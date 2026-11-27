package indexing

import (
	"testing"
	"time"

	"github.com/gtsteffaniak/filebrowser/backend/pkg/settings"
)

func TestForkRescanClock(t *testing.T) {
	cases := map[string]bool{"": false, "03:30": true, "23:59": true, "25:00": false, "nightly": false}
	for in, wantOK := range cases {
		_, _, ok, err := settings.BackgroundScanConfig{FullRescanAt: in}.RescanClock()
		if ok != wantOK || (in != "" && !wantOK && err == nil) {
			t.Errorf("%q: ok=%v err=%v", in, ok, err)
		}
	}
}

func TestForkNextDailyRescan(t *testing.T) {
	loc := time.Local
	defer func() { time.Local = loc }()
	time.Local = time.FixedZone("CET", 3600)

	now := time.Date(2026, 11, 27, 22, 0, 0, 0, time.Local)
	if got, want := nextDailyRescan(now, 3, 30), time.Date(2026, 11, 28, 3, 30, 0, 0, time.Local); !got.Equal(want) {
		t.Errorf("evening: got %v want %v", got, want)
	}
	now = time.Date(2026, 11, 28, 1, 0, 0, 0, time.Local)
	if got, want := nextDailyRescan(now, 3, 30), time.Date(2026, 11, 28, 3, 30, 0, 0, time.Local); !got.Equal(want) {
		t.Errorf("before time same day: got %v want %v", got, want)
	}
	now = time.Date(2026, 11, 28, 3, 30, 0, 0, time.Local)
	if got, want := nextDailyRescan(now, 3, 30), time.Date(2026, 11, 29, 3, 30, 0, 0, time.Local); !got.Equal(want) {
		t.Errorf("exactly at time: got %v want %v", got, want)
	}
}

func TestForkAdaptiveToggle(t *testing.T) {
	idx := &Index{}
	if !idx.useAdaptiveScheduling() {
		t.Fatal("default must stay adaptive")
	}
	idx.Config.BackgroundScan.FullRescanAt = "03:30"
	if idx.useAdaptiveScheduling() {
		t.Fatal("fullRescanAt alone must turn adaptive scans off")
	}
	idx.Config.BackgroundScan.OnlyWhenDiskAwake = true
	if !idx.useAdaptiveScheduling() {
		t.Fatal("with onlyWhenDiskAwake adaptive scans keep running")
	}
}

func TestForkWake(t *testing.T) {
	now := time.Now()
	slot := now.Add(10 * time.Minute)

	idx := &Index{}
	if got := idx.forkWake(now, slot); !got.Equal(slot) {
		t.Fatal("unset: wake must be unchanged")
	}

	idx = &Index{}
	idx.Config.BackgroundScan.FullRescanAt = "03:30"
	idx.forkState().nextRescan = now.Add(5 * time.Hour)
	if got := idx.forkWake(now, now.Add(time.Minute)); !got.Equal(now.Add(5 * time.Hour)) {
		t.Fatal("fixed-only: wake at the rescan, not every minute")
	}

	idx.Config.BackgroundScan.OnlyWhenDiskAwake = true
	if got := idx.forkWake(now, slot); !got.Equal(slot) {
		t.Fatal("combined: earlier slot wins")
	}
	idx.forkState().nextRescan = now.Add(time.Minute)
	if got := idx.forkWake(now, slot); !got.Equal(now.Add(time.Minute)) {
		t.Fatal("combined: earlier rescan wins")
	}
}

func TestForkDeferIfDiskAsleep(t *testing.T) {
	idx := &Index{}
	s := idx.createScanner("/a")
	if idx.forkDeferIfDiskAsleep([]*Scanner{s}) {
		t.Fatal("option off: never defer")
	}
	idx.Config.BackgroundScan.OnlyWhenDiskAwake = true
	sleeping := &diskProbe{name: "fake", rotational: true}
	idx.forkState().disks = &diskWatch{disks: []*diskProbe{sleeping}}
	if !idx.forkDeferIfDiskAsleep([]*Scanner{s}) {
		t.Fatal("no recent I/O: must defer")
	}
	if s.nextRun.Before(time.Now()) || s.calendarSlotSec == 0 {
		t.Fatalf("deferred scanner must be re-queued in the future, nextRun=%v slot=%d", s.nextRun, s.calendarSlotSec)
	}
	sleeping.lastIO = time.Now()
	if idx.forkDeferIfDiskAsleep([]*Scanner{s}) {
		t.Fatal("recent I/O: must not defer")
	}
	idx.forkState().disks = &diskWatch{disks: []*diskProbe{{name: "ssd", rotational: false}}}
	if idx.forkDeferIfDiskAsleep([]*Scanner{s}) {
		t.Fatal("non-rotational disk is always awake")
	}
}
