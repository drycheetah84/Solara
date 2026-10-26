package snapshot

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"solara/internal/docdb"
	"solara/internal/logging"
)

// SchedulerConfig controls the automatic snapshots taken while the server runs.
type SchedulerConfig struct {
	Dir      string        // snapshots directory
	EOSDir   string        // "" = do not capture EOS files
	Interval time.Duration // 0 disables periodic snapshots
	Keep     int           // prune to this many after each snapshot; 0 = no pruning
	OnStart  bool          // take a "startup" snapshot when the scheduler starts
}

// ConfigFromEnv reads SOLARA_SNAPSHOT_DIR, SOLARA_SNAPSHOT_INTERVAL_HOURS
// (default 6, 0 disables; fractions allowed), SOLARA_SNAPSHOT_KEEP (default
// 28, 0 disables pruning) and SOLARA_SNAPSHOT_ON_START (default on, "0" /
// "false" disables). Invalid values fall back to the default with a warning.
func ConfigFromEnv(log *logging.Logger) SchedulerConfig {
	c := SchedulerConfig{Dir: DefaultDir(), Interval: DefaultIntervalHours * time.Hour, Keep: DefaultKeep, OnStart: true}
	if v := strings.TrimSpace(os.Getenv("SOLARA_SNAPSHOT_INTERVAL_HOURS")); v != "" {
		if h, err := strconv.ParseFloat(v, 64); err == nil && h >= 0 {
			c.Interval = time.Duration(h * float64(time.Hour))
		} else if log != nil {
			log.Warning("Ignoring invalid SOLARA_SNAPSHOT_INTERVAL_HOURS {Value}", v)
		}
	}
	if v := strings.TrimSpace(os.Getenv("SOLARA_SNAPSHOT_KEEP")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			c.Keep = n
		} else if log != nil {
			log.Warning("Ignoring invalid SOLARA_SNAPSHOT_KEEP {Value}", v)
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SOLARA_SNAPSHOT_ON_START"))) {
	case "0", "false", "no", "off":
		c.OnStart = false
	}
	return c
}

// Scheduler takes automatic snapshots of an open database.
type Scheduler struct {
	db   *docdb.Database
	cfg  SchedulerConfig
	log  *logging.Logger
	quit chan struct{}
	done chan struct{}
	once sync.Once

	mu    sync.Mutex
	taken []string // names of snapshots taken (tests / diagnostics)
}

// StartScheduler starts the scheduler goroutine. The returned Scheduler's
// Stop waits for an in-progress snapshot and must be called before the
// database is closed. Failures are logged and never stop the server.
func StartScheduler(db *docdb.Database, cfg SchedulerConfig) *Scheduler {
	s := &Scheduler{db: db, cfg: cfg, log: logging.New("Snapshots"), quit: make(chan struct{}), done: make(chan struct{})}
	go s.run()
	return s
}

func (s *Scheduler) run() {
	defer close(s.done)
	if s.cfg.OnStart {
		s.take("startup", "automatic snapshot at server start")
	}
	if s.cfg.Interval <= 0 {
		<-s.quit
		return
	}
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			s.take("auto", fmt.Sprintf("scheduled snapshot (every %s)", s.cfg.Interval))
		}
	}
}

// take creates, verifies and prunes; it never panics out of the goroutine.
func (s *Scheduler) take(label, reason string) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("Snapshot '{Label}' panicked: {Panic}", label, fmt.Sprint(r))
		}
	}()
	select {
	case <-s.quit:
		return
	default:
	}
	start := time.Now()
	snap, err := Create(s.db, Options{Dir: s.cfg.Dir, EOSDir: s.cfg.EOSDir, Label: label, Reason: reason})
	if err != nil {
		s.log.Error("Snapshot '{Label}' failed: {Message}", label, err.Error())
		return
	}
	v := Verify(snap.Path, s.db.Descriptor().Password)
	if !v.OK {
		s.log.Error("Snapshot {Name} failed verification: {Problems}", snap.Name, strings.Join(v.Problems, "; "))
	} else {
		var size int64
		for _, f := range snap.Manifest.Files {
			size += f.Size
		}
		s.log.Information("Snapshot {Name} written ({Bytes} bytes, {Elapsed}) to {Dir}", snap.Name, size, time.Since(start).Round(time.Millisecond).String(), s.cfg.Dir)
	}
	s.mu.Lock()
	s.taken = append(s.taken, snap.Name)
	s.mu.Unlock()
	if s.cfg.Keep > 0 {
		deleted, err := Prune(s.cfg.Dir, s.cfg.Keep, false)
		if err != nil {
			s.log.Error("Snapshot pruning failed: {Message}", err.Error())
		} else if len(deleted) > 0 {
			s.log.Information("Pruned {Count} old snapshot(s): {Names}", len(deleted), strings.Join(deleted, ", "))
		}
	}
}

// Taken returns the names of snapshots this scheduler created.
func (s *Scheduler) Taken() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.taken...)
}

// Stop stops the scheduler and waits for it (idempotent).
func (s *Scheduler) Stop() {
	s.once.Do(func() { close(s.quit) })
	<-s.done
}
