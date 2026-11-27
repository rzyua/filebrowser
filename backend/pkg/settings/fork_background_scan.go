package settings

// fork: per-source control over when background indexing scans run, for sources on
// disks that spin down. See FORK.md ("Background scan schedule").

import (
	"fmt"
	"strings"
	"time"
)

// BackgroundScanConfig is the `backgroundScan` block of a source's config.
// Leaving it out keeps upstream's adaptive schedule unchanged.
type BackgroundScanConfig struct {
	// FullRescanAt runs a full rescan of the source every day at this local time ("HH:MM", 24h).
	// The time zone is the server's (the TZ environment variable in Docker).
	// On its own it replaces the adaptive schedule: no other background scans run.
	// Combined with onlyWhenDiskAwake, adaptive scans keep running in between, but only while the disk is awake.
	FullRescanAt string `json:"fullRescanAt,omitempty"`
	// OnlyWhenDiskAwake skips adaptive background scans while the source's disk is in standby,
	// and runs them once it is spinning again. The nightly full rescan (fullRescanAt) always runs.
	OnlyWhenDiskAwake bool `json:"onlyWhenDiskAwake,omitempty"`
	// Disks overrides the block devices checked by onlyWhenDiskAwake (eg. "/dev/sda").
	// By default they are found from the source path (partitions, LVM and mdraid are followed).
	Disks []string `json:"disks,omitempty"`
}

// RescanClock returns the hour and minute of FullRescanAt. ok is false when it is unset.
func (c BackgroundScanConfig) RescanClock() (hour, minute int, ok bool, err error) {
	s := strings.TrimSpace(c.FullRescanAt)
	if s == "" {
		return 0, 0, false, nil
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, false, fmt.Errorf("backgroundScan.fullRescanAt must be a 24h time like \"03:30\", got %q", c.FullRescanAt)
	}
	return t.Hour(), t.Minute(), true, nil
}

func validateBackgroundScan(source *Source) error {
	_, _, _, err := source.Config.BackgroundScan.RescanClock()
	if err != nil {
		return fmt.Errorf("source %q: %w", source.Path, err)
	}
	return nil
}
