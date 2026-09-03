package modules

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// taskTimeout is the maximum duration an individual task (ping, speedtest, archive)
// may run before its context is cancelled to prevent hung processes from blocking the worker.
const taskTimeout = 5 * time.Minute

// task represents an individual scheduled job with its own recurrence interval,
// thread-safe next execution time, and worker function.
type task struct {
	mu       sync.RWMutex
	name     string
	interval time.Duration
	nextRun  time.Time
	fn       func(ctx context.Context)
}

// getNextRun returns the target execution timestamp in a thread-safe manner.
func (t *task) getNextRun() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nextRun
}

// setNextRun updates the target execution timestamp in a thread-safe manner.
func (t *task) setNextRun(next time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextRun = next
}

// Scheduler coordinates recurring monitoring tasks.
// Key architectural design:
//  1. Clock alignment: tasks snap to natural clock boundaries (e.g. 5m intervals trigger at :00, :05, :10).
//  2. Serial worker queue: tasks are pushed onto a buffered channel and processed one-at-a-time by a dedicated
//     worker goroutine, preventing concurrent disk writes or network congestion from overlapping tasks.
//  3. Execution deadline: each task execution is wrapped with taskTimeout and cancellation context.
type Scheduler struct {
	cfg    *Config
	fm     *FileManager
	logger *Logger

	tasks  []*task
	queue  chan *task
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler creates a Scheduler with a buffered queue channel.
func NewScheduler(cfg *Config, fm *FileManager, logger *Logger) *Scheduler {
	return &Scheduler{
		cfg:    cfg,
		fm:     fm,
		logger: logger,
		queue:  make(chan *task, 16),
	}
}

// Start registers active tasks from configuration and launches the scheduler worker and ticker goroutines.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	now := time.Now().UTC()

	s.tasks = make([]*task, 0, 4)

	addTask := func(name string, seconds int64, fn func(context.Context)) {
		if seconds <= 0 {
			return
		}

		interval := time.Duration(seconds) * time.Second

		s.tasks = append(s.tasks, &task{
			name:     name,
			interval: interval,
			nextRun:  alignTime(now, interval),
			fn:       fn,
		})
	}

	addTask("ping", s.cfg.Schedule.PingSeconds, func(ctx context.Context) {
		RunPing(ctx, s.cfg, s.fm, s.logger, time.Now().UTC())
	})

	addTask("speedtest", s.cfg.Schedule.SpeedtestSeconds, func(ctx context.Context) {
		RunSpeedtest(ctx, s.cfg.Speedtest, s.fm, s.logger, time.Now().UTC())
	})

	addTask("archive", s.cfg.Schedule.ArchivingSeconds, func(ctx context.Context) {
		ts := time.Now().UTC()
		fmt.Printf("[archive] starting archiving run at %s\n", formatConsoleTime(ts))

		if err := s.fm.Archive(s.cfg.Schedule.ArchivingSeconds); err != nil {
			s.logger.Error("archiving failed", "error", err)
			fmt.Printf("[archive] FAILED: %v\n", err)
		} else {
			s.logger.Info("archiving completed")
			fmt.Println("[archive] run complete")
		}
	})

	addTask("log rotate", s.cfg.Schedule.LogRotationSeconds, func(ctx context.Context) {
		ts := time.Now().UTC()
		fmt.Printf("[log rotate] starting log rotation run at %s\n", formatConsoleTime(ts))

		if err := s.logger.Rotate(); err != nil {
			s.logger.Error("log rotation failed", "error", err)
			fmt.Printf("[log rotate] FAILED: %v\n", err)
		} else {
			s.logger.Info("log rotation completed")
			fmt.Println("[log rotate] run complete")
		}
	})

	// Display initial scheduled next run times for all registered tasks.
	s.printNextRuns()

	// Worker goroutine: drains the queue and executes tasks serially.
	s.wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-s.queue:
				s.runWithTimeout(t)
			}
		}
	})

	// Ticker goroutine: checks every second if any task is due, updates its next run time,
	// and pushes it onto the worker queue.
	s.wg.Go(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				utcNow := now.UTC()

				for _, t := range s.tasks {
					if !utcNow.Before(t.getNextRun()) {
						t.setNextRun(alignTime(utcNow, t.interval))

						select {
						case s.queue <- t:
						default:
							s.logger.Warn("queue full, skipping task", "task", t.name)
						}
					}
				}
			}
		}
	})
}

// Stop signals cancellation to the worker/ticker goroutines and waits for in-flight executions to finish.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()
}

// runWithTimeout runs an individual task with a hard execution deadline (taskTimeout).
// When completed or cancelled, it calculates and prints the next scheduled execution time.
func (s *Scheduler) runWithTimeout(t *task) {
	ctx, cancel := context.WithTimeout(s.ctx, taskTimeout)
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)
		t.fn(ctx)
	}()

	select {
	case <-done:
		// Completed normally — print next scheduled run time.
		s.printNextRun(t, time.Now().UTC())

	case <-ctx.Done():
		s.logger.Error(
			"task exceeded timeout and was cancelled",
			"task", t.name,
			"timeout", taskTimeout.String(),
		)

		s.printNextRun(t, time.Now().UTC())
	}
}

// printNextRuns prints upcoming run times for all configured tasks.
func (s *Scheduler) printNextRuns() {
	now := time.Now().UTC()

	for _, t := range s.tasks {
		s.printNextRun(t, now)
	}
}

// printNextRun prints a human-readable summary of when a task will execute next.
func (s *Scheduler) printNextRun(t *task, now time.Time) {
	next := t.getNextRun()
	d := next.Sub(now)

	fmt.Printf(
		"[scheduler] %-12s next run in %s (at %s)\n",
		t.name,
		formatDuration(d),
		formatConsoleTime(next),
	)
}

// alignTime calculates the next natural clock-aligned time boundary for an interval.
// Example: If interval is 5 minutes and now is 12:03:15, Truncate returns 12:00:00,
// and alignTime advances it to 12:05:00.
func alignTime(now time.Time, interval time.Duration) time.Time {
	aligned := now.Truncate(interval)

	if !aligned.After(now) {
		aligned = aligned.Add(interval)
	}

	return aligned
}

// formatConsoleTime formats a timestamp in the host machine's local timezone with offset (e.g. "15:04:05 UTC+10:00").
func formatConsoleTime(t time.Time) string {
	return t.Local().Format("15:04:05 UTC-07:00")
}

// formatDuration formats a duration into an intuitive human-readable string (e.g. "5.0 minutes", "1 hour", "14 days").
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	if d >= 24*time.Hour {
		return formatUnit(d.Hours()/24.0, "day", "days")
	}

	if d >= time.Hour {
		return formatUnit(d.Hours(), "hour", "hours")
	}

	return fmt.Sprintf("%.1f minutes", d.Minutes())
}

// formatUnit formats an integer or single-decimal unit count with proper singular/plural suffix.
func formatUnit(val float64, singular, plural string) string {
	rounded := math.Round(val*10) / 10

	if rounded == math.Floor(rounded) {
		if int(rounded) == 1 {
			return "1 " + singular
		}

		return fmt.Sprintf("%d %s", int(rounded), plural)
	}

	return fmt.Sprintf("%.1f %s", rounded, plural)
}
