package indexing

// fork: per-source background scan schedule (settings.BackgroundScanConfig):
//   - fullRescanAt: one full rescan of every scanner per day at a fixed local time.
//   - onlyWhenDiskAwake: adaptive scans are deferred while the source's disks are in standby.
// The hooks into upstream code are marked "// fork:" in indexingSchedulerLoop.go and indexingFiles.go.

import (
	"strings"
	"sync"
	"time"

	"github.com/gtsteffaniak/go-logger/logger"
)

const (
	// diskDeferRetry is how often a scan deferred by a sleeping disk checks the disk again.
	// Checks never touch the disk, so this only bounds how late a scan runs after the disk wakes.
	diskDeferRetry = 5 * time.Minute
	// diskActivityWindow: with the I/O-activity fallback, a disk that completed I/O this recently
	// counts as spinning. Keep it below the drive's spin-down timeout.
	diskActivityWindow = 5 * time.Minute
	diskSampleInterval = 30 * time.Second
)

type forkScanState struct {
	mu         sync.Mutex
	nextRescan time.Time // zero when fullRescanAt is unset
	disks      *diskWatch
}

var forkStates sync.Map // *Index -> *forkScanState

func (idx *Index) forkState() *forkScanState {
	v, _ := forkStates.LoadOrStore(idx, &forkScanState{})
	st, ok := v.(*forkScanState)
	if !ok {
		st = &forkScanState{}
		forkStates.Store(idx, st)
	}
	return st
}

func (idx *Index) forkRescanEnabled() bool {
	_, _, ok, _ := idx.Config.BackgroundScan.RescanClock()
	return ok
}

// forkFixedOnly: the daily rescan is the only background scan (adaptive scheduling off).
func (idx *Index) forkFixedOnly() bool {
	return idx.forkRescanEnabled() && !idx.Config.BackgroundScan.OnlyWhenDiskAwake
}

// nextDailyRescan returns the first time strictly after now at hour:minute in the server's time zone.
func nextDailyRescan(now time.Time, hour, minute int) time.Time {
	local := now.In(time.Local)
	t := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, time.Local)
	if !t.After(now) {
		t = time.Date(local.Year(), local.Month(), local.Day()+1, hour, minute, 0, 0, time.Local)
	}
	return t
}

// forkSchedulerStart is called once when the index scheduler goroutine starts.
func (idx *Index) forkSchedulerStart() {
	cfg := idx.Config.BackgroundScan
	st := idx.forkState()
	var parts []string
	if h, m, ok, _ := cfg.RescanClock(); ok {
		st.mu.Lock()
		st.nextRescan = nextDailyRescan(time.Now(), h, m)
		next := st.nextRescan
		st.mu.Unlock()
		parts = append(parts, "full rescan daily at "+cfg.FullRescanAt+" "+next.Format("MST")+" (next "+next.Format(time.RFC3339)+")")
		if !cfg.OnlyWhenDiskAwake {
			parts = append(parts, "no other background scans")
		}
	}
	if cfg.OnlyWhenDiskAwake {
		st.disks = newDiskWatch(idx)
		parts = append(parts, "adaptive scans only while the disk is awake")
		go st.disks.sample(idx.schedulerStop)
	}
	if len(parts) > 0 {
		logger.Infof("[%s] background scans: %s", idx.Name, strings.Join(parts, "; "))
	}
}

// forkWake adjusts the scheduler's next wake-up for the daily rescan.
func (idx *Index) forkWake(now, next time.Time) time.Time {
	st := idx.forkState()
	st.mu.Lock()
	rescan := st.nextRescan
	st.mu.Unlock()
	if rescan.IsZero() {
		return next
	}
	if idx.forkFixedOnly() || rescan.Before(next) {
		return rescan
	}
	return next
}

// forkRunDueFullRescan runs the daily full rescan when it is due. Reports whether it ran.
// It runs regardless of disk state: it is scheduled for when the disks are awake anyway.
func (idx *Index) forkRunDueFullRescan() bool {
	h, m, ok, _ := idx.Config.BackgroundScan.RescanClock()
	if !ok {
		return false
	}
	st := idx.forkState()
	now := time.Now()
	st.mu.Lock()
	due := !st.nextRescan.IsZero() && !now.Before(st.nextRescan)
	if due {
		st.nextRescan = nextDailyRescan(now, h, m)
	}
	st.mu.Unlock()
	if !due {
		return false
	}
	logger.Infof("[%s] scheduled full rescan starting", idx.Name)
	start := time.Now()
	idx.runSerialScanPass()
	// The root scan only reads the top level and registers new top-level folders as
	// unscanned scanners; scan those now instead of a day later.
	idx.forkScanUnscanned()
	logger.Infof("[%s] scheduled full rescan finished in %v", idx.Name, time.Since(start).Round(time.Second))
	return true
}

func (idx *Index) forkScanUnscanned() {
	var pending []*Scanner
	for _, p := range sortedScannerPaths(idx) {
		idx.mu.RLock()
		s, ok := idx.scanners[p]
		idx.mu.RUnlock()
		if !ok {
			continue
		}
		var never bool
		s.withStatsRLock(func() { never = s.lastScanned.IsZero() })
		if never {
			pending = append(pending, s)
		}
	}
	if len(pending) == 0 {
		return
	}
	idx.mu.Lock()
	idx.schedulerBatch++
	idx.mu.Unlock()
	defer func() {
		idx.mu.Lock()
		idx.schedulerBatch--
		idx.mu.Unlock()
		if err := idx.PostScan(); err != nil {
			logger.Errorf("PostScan after scanning new folders: %v", err)
		}
	}()
	for _, s := range pending {
		if !s.directoryExists() {
			s.removeSelf()
			continue
		}
		s.withStatsLock(func() { s.fullScanCounter = 0 })
		s.executeScan()
	}
}

// forkDeferIfDiskAsleep re-queues a due batch without scanning when onlyWhenDiskAwake is set and
// a disk of the source is in standby. Reports whether the batch was deferred.
func (idx *Index) forkDeferIfDiskAsleep(batch []*Scanner) bool {
	if !idx.Config.BackgroundScan.OnlyWhenDiskAwake {
		return false
	}
	st := idx.forkState()
	if st.disks == nil || st.disks.awake() {
		return false
	}
	now := time.Now()
	retry := computeNextSlotTime(now, now, diskDeferRetry)
	for _, s := range batch {
		s.withStatsLock(func() { s.nextRun = retry })
		idx.registerScannerNextRun(s, retry)
	}
	logger.Debugf("[%s] scheduler: disk asleep, deferred %d scanner(s) to %v", idx.Name, len(batch), retry.UTC().Format(time.RFC3339))
	return true
}

type diskProbe struct {
	name       string
	rotational bool
	useATA     bool // false after the ATA power query failed once; then I/O activity is used
	counters   string
	lastIO     time.Time // zero: no I/O seen since startup
	lastAwake  *bool
}

type diskWatch struct {
	idxName string
	mu      sync.Mutex
	disks   []*diskProbe
}

func newDiskWatch(idx *Index) *diskWatch {
	w := &diskWatch{idxName: idx.Name}
	cfg := idx.Config.BackgroundScan
	var names []string
	var err error
	if len(cfg.Disks) > 0 {
		names, err = disksFromConfig(cfg.Disks)
	} else {
		names, err = resolveSourceDisks(idx.Path)
	}
	if err != nil {
		logger.Warningf("[%s] onlyWhenDiskAwake: cannot find the disk to check, background scans will run as usual: %v", idx.Name, err)
		return w
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		p := &diskProbe{name: n, rotational: diskIsRotational(n), useATA: true}
		p.counters, _ = diskIOCounters(n)
		w.disks = append(w.disks, p)
	}
	var desc []string
	for _, p := range w.disks {
		if p.rotational {
			desc = append(desc, p.name)
		} else {
			desc = append(desc, p.name+" (not rotational, always awake)")
		}
	}
	logger.Infof("[%s] onlyWhenDiskAwake: checking %s", idx.Name, strings.Join(desc, ", "))
	return w
}

// sample records when each disk last completed I/O, for the activity fallback.
func (w *diskWatch) sample(stop <-chan struct{}) {
	t := time.NewTicker(diskSampleInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		now := time.Now()
		w.mu.Lock()
		for _, p := range w.disks {
			c, err := diskIOCounters(p.name)
			if err == nil && c != p.counters {
				if p.counters != "" {
					p.lastIO = now
				}
				p.counters = c
			}
		}
		w.mu.Unlock()
	}
}

// awake reports whether every disk of the source is spinning. Unknown disks count as awake.
func (w *diskWatch) awake() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	all := true
	for _, p := range w.disks {
		if !p.rotational {
			continue
		}
		var awake bool
		method := "power mode"
		if p.useATA {
			a, err := ataPowerMode(p.name)
			if err != nil {
				p.useATA = false
				logger.Warningf("[%s] onlyWhenDiskAwake: cannot query the power mode of /dev/%s (%v); "+
					"judging by recent disk activity instead. For the exact check, run as root with the device available "+
					"(Docker: devices: [/dev/%s], cap_add: [SYS_RAWIO], user: root).", w.idxName, p.name, err, p.name)
			} else {
				awake = a
			}
		}
		if !p.useATA {
			method = "recent I/O"
			awake = !p.lastIO.IsZero() && time.Since(p.lastIO) < diskActivityWindow
		}
		if p.lastAwake == nil || *p.lastAwake != awake {
			state := "asleep"
			if awake {
				state = "awake"
			}
			logger.Debugf("[%s] disk %s is %s (%s)", w.idxName, p.name, state, method)
			p.lastAwake = &awake
		}
		if !awake {
			all = false
		}
	}
	return all
}
