//go:build linux

package indexing

import "testing"

func TestForkParseATAReturnSense(t *testing.T) {
	// descriptor format with ATA Status Return descriptor, COUNT = 0xff (active/idle)
	desc := []byte{0x72, 0x01, 0x00, 0x1d, 0, 0, 0, 0x0e, 0x09, 0x0c, 0x00, 0x00, 0x00, 0xff, 0, 0, 0, 0, 0, 0, 0x40, 0x50}
	if c, err := parseATAReturnSense(desc); err != nil || c != 0xff {
		t.Fatalf("descriptor: c=%#x err=%v", c, err)
	}
	// fixed format: INFORMATION = error, status, device, count(=0x00 standby)
	fixed := []byte{0x70, 0x00, 0x01, 0x00, 0x50, 0x40, 0x00, 0x0a, 0, 0, 0, 0}
	if c, err := parseATAReturnSense(fixed); err != nil || c != 0x00 {
		t.Fatalf("fixed: c=%#x err=%v", c, err)
	}
	if _, err := parseATAReturnSense([]byte{0x72, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("no descriptor must be an error")
	}
}
