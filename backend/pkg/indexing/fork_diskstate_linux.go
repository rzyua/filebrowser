//go:build linux

package indexing

// fork: find the disks behind a source and tell whether they are spinning, without waking them.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// resolveSourceDisks returns the kernel names (eg. "sda") of the whole disks that hold path,
// following partitions and stacked devices (LVM, dm-crypt, mdraid) down to their members.
func resolveSourceDisks(path string) ([]string, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return nil, err
	}
	major, minor := unix.Major(uint64(st.Dev)), unix.Minor(uint64(st.Dev))
	if major == 0 {
		return nil, fmt.Errorf("%s is on a filesystem without a single block device (eg. btrfs, ZFS, overlay); set backgroundScan.disks", path)
	}
	real, err := filepath.EvalSymlinks(fmt.Sprintf("/sys/dev/block/%d:%d", major, minor))
	if err != nil {
		return nil, fmt.Errorf("no block device %d:%d in /sys for %s: %w", major, minor, path, err)
	}
	return expandDisk(filepath.Base(real), 0)
}

// disksFromConfig maps configured device paths ("/dev/sda", "/dev/disk/by-id/...", "sda") to kernel names.
func disksFromConfig(devices []string) ([]string, error) {
	var out []string
	for _, d := range devices {
		if strings.Contains(d, "/") {
			if real, err := filepath.EvalSymlinks(d); err == nil {
				d = real
			}
		}
		disks, err := expandDisk(filepath.Base(d), 0)
		if err != nil {
			return nil, err
		}
		out = append(out, disks...)
	}
	return out, nil
}

func expandDisk(name string, depth int) ([]string, error) {
	if depth > 8 {
		return nil, fmt.Errorf("device stack too deep at %s", name)
	}
	sysPath, err := filepath.EvalSymlinks("/sys/class/block/" + name)
	if err != nil {
		return nil, fmt.Errorf("unknown block device %q: %w", name, err)
	}
	if _, err := os.Stat(filepath.Join(sysPath, "partition")); err == nil {
		return expandDisk(filepath.Base(filepath.Dir(sysPath)), depth+1)
	}
	slaves, _ := os.ReadDir(filepath.Join(sysPath, "slaves"))
	if len(slaves) == 0 {
		return []string{name}, nil
	}
	var out []string
	for _, s := range slaves {
		disks, err := expandDisk(s.Name(), depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, disks...)
	}
	return out, nil
}

// diskIsRotational reports whether the disk spins; SSDs and NVMe are treated as always awake.
func diskIsRotational(name string) bool {
	b, err := os.ReadFile("/sys/block/" + name + "/queue/rotational")
	return err != nil || strings.TrimSpace(string(b)) != "0"
}

// diskIOCounters returns the completed read+write counts from /sys/block/<name>/stat.
// Reading it is free: it never touches the disk and needs no privileges.
func diskIOCounters(name string) (string, error) {
	b, err := os.ReadFile("/sys/block/" + name + "/stat")
	if err != nil {
		return "", err
	}
	f := strings.Fields(string(b))
	if len(f) < 5 {
		return "", errors.New("short stat line")
	}
	return f[0] + "/" + f[4], nil // reads completed / writes completed
}

const (
	sgIO            = 0x2285
	hdioDriveCmd    = 0x031f
	ataCheckPower   = 0xe5
	sgDxferNone     = -1
	sgInterfaceID   = 'S'
	ataPassThru16   = 0x85
	senseBufferSize = 32
)

// sgIOHdr mirrors struct sg_io_hdr from <scsi/sg.h>.
type sgIOHdr struct {
	interfaceID    int32
	dxferDirection int32
	cmdLen         uint8
	mxSbLen        uint8
	iovecCount     uint16
	dxferLen       uint32
	dxferp         unsafe.Pointer
	cmdp           unsafe.Pointer
	sbp            unsafe.Pointer
	timeout        uint32
	flags          uint32
	packID         int32
	usrPtr         unsafe.Pointer
	status         uint8
	maskedStatus   uint8
	msgStatus      uint8
	sbLenWr        uint8
	hostStatus     uint16
	driverStatus   uint16
	resid          int32
	duration       uint32
	info           uint32
}

// ataPowerMode sends ATA CHECK POWER MODE, which drives answer from standby without spinning up
// (it is what `hdparm -C` does). Returns true while the disk is spinning (active or idle).
// Needs read access to /dev/<name> and, for the SCSI pass-through, CAP_SYS_RAWIO (root).
func ataPowerMode(name string) (awake bool, err error) {
	fd, err := unix.Open("/dev/"+name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, err
	}
	defer unix.Close(fd)

	count, sgErr := checkPowerSGIO(fd)
	if sgErr != nil {
		args := [4]byte{ataCheckPower, 0, 0, 0}
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), hdioDriveCmd, uintptr(unsafe.Pointer(&args))); errno != 0 {
			return false, fmt.Errorf("ATA pass-through: %v; HDIO_DRIVE_CMD: %v", sgErr, errno)
		}
		count = args[2]
	}
	// 0x00: standby, 0x01: standby (ACS-3 "standby_y"); everything else is idle or active.
	return count != 0x00 && count != 0x01, nil
}

func checkPowerSGIO(fd int) (byte, error) {
	cdb := [16]byte{
		0:  ataPassThru16,
		1:  3 << 1, // protocol 3: non-data
		2:  0x20,   // ck_cond: return the ATA registers in the sense data
		14: ataCheckPower,
	}
	sense := make([]byte, senseBufferSize)
	hdr := sgIOHdr{
		interfaceID:    sgInterfaceID,
		dxferDirection: sgDxferNone,
		cmdLen:         uint8(len(cdb)),
		mxSbLen:        uint8(len(sense)),
		cmdp:           unsafe.Pointer(&cdb[0]),
		sbp:            unsafe.Pointer(&sense[0]),
		timeout:        5000,
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), sgIO, uintptr(unsafe.Pointer(&hdr)))
	runtime.KeepAlive(&cdb)
	runtime.KeepAlive(sense)
	if errno != 0 {
		return 0, errno
	}
	if hdr.hostStatus != 0 || (hdr.driverStatus&^0x08) != 0 { // 0x08 = DRIVER_SENSE, expected with ck_cond
		return 0, fmt.Errorf("SG_IO host=%#x driver=%#x", hdr.hostStatus, hdr.driverStatus)
	}
	return parseATAReturnSense(sense[:hdr.sbLenWr])
}

// parseATAReturnSense pulls the ATA COUNT register out of descriptor (0x72) or fixed (0x70) sense data.
func parseATAReturnSense(sb []byte) (byte, error) {
	if len(sb) < 8 {
		return 0, errors.New("no sense data returned")
	}
	switch sb[0] & 0x7f {
	case 0x72:
		desc := sb[8:]
		for len(desc) >= 2 {
			l := int(desc[1]) + 2
			if desc[0] == 0x09 && len(desc) >= 14 {
				return desc[5], nil
			}
			if l > len(desc) {
				break
			}
			desc = desc[l:]
		}
	case 0x70:
		if len(sb) >= 7 {
			return sb[6], nil
		}
	}
	return 0, fmt.Errorf("no ATA status in sense data % x", bytes.TrimRight(sb, "\x00"))
}
