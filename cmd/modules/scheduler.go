package modules

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const taskTimeout = 5 * time.Minute

// task is a scheduled job.
type task struct {
	name     string
	interval time.Duration
	nextRun  time.Time
	fn       func()
}

// Scheduler owns a queue of tasks and fires each one on its interval using
// UTC time. Only one task runs at a time; if a task exceeds taskTimeout it is
// cancelled and an error is logged.
type Scheduler struct {
	cfg    *Config
	fm     *FileManager
	logger *Logger

	tasks  []*task
	queue  chan *task
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler constructs a Scheduler but does not start it.
func NewScheduler(cfg *Config, fm *FileManager, logger *Logger) *Scheduler {
	return &Scheduler{
		cfg:    cfg,
		fm:     fm,
		logger: logger,
		queue:  make(chan *task, 16),
	}
}

// Start launches the scheduler background goroutines.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	now := time.Now().UTC()

	s.tasks = make([]*task, 0, 3)

	if s.cfg.Schedule.PingSeconds > 0 {
		s.tasks = append(s.tasks, &task{
			name:     "ping",
			interval: time.Duration(s.cfg.Schedule.PingSeconds) * time.Second,
			nextRun:  alignTime(now, time.Duration(s.cfg.Schedule.PingSeconds)*time.Second),
			fn: func() {
				RunPing(s.cfg, s.fm, s.logger, time.Now().UTC())
			},
		})
	}

	if s.cfg.Schedule.SpeedtestSeconds > 0 {
		s.tasks = append(s.tasks, &task{
			name:     "speedtest",
			interval: time.Duration(s.cfg.Schedule.SpeedtestSeconds) * time.Second,
			nextRun:  alignTime(now, time.Duration(s.cfg.Schedule.SpeedtestSeconds)*time.Second),
			fn: func() {
				RunSpeedtest(s.fm, s.logger, time.Now().UTC())
			},
		})
	}

	if s.cfg.Schedule.ArchivingSeconds > 0 {
		s.tasks = append(s.tasks, &task{
			name:     "archive",
			interval: time.Duration(s.cfg.Schedule.ArchivingSeconds) * time.Second,
			nextRun:  alignTime(now, time.Duration(s.cfg.Schedule.ArchivingSeconds)*time.Second),
			fn: func() {
				ts := time.Now().UTC()
				fmt.Printf("[archive] starting archiving run at %s\n", ts.Format("15:04:05Z"))
				if err := s.fm.Archive(s.cfg.Schedule.ArchivingSeconds); err != nil {
					s.logger.Error("archiving failed", "error", err)
					fmt.Printf("[archive] FAILED: %v\n", err)
				} else {
					s.logger.Info("archiving completed")
					fmt.Println("[archive] run complete")
				}
			},
		})
	}

	s.printNextRuns()

	// Worker: drains the queue one task at a time.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-s.queue:
				s.runWithTimeout(t)
			}
		}
	}()

	// Ticker: enqueues tasks when their time arrives.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				utcNow := now.UTC()
				for _, t := range s.tasks {
					if !utcNow.Before(t.nextRun) {
						t.nextRun = alignTime(utcNow, t.interval)
						select {
						case s.queue <- t:
						default:
							s.logger.Warn("queue full, skipping task", "task", t.name)
						}
					}
				}
			}
		}
	}()
}

// Stop signals the scheduler to finish the current task and exit cleanly.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// runWithTimeout runs a task inside a goroutine with a hard timeout.
// After the task finishes (or times out) it prints the next scheduled run time.
func (s *Scheduler) runWithTimeout(t *task) {
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		t.fn()
	}()

	select {
	case <-done:
		// completed normally — print next run so it appears right after task output
		s.printNextRun(t, time.Now().UTC())
	case <-ctx.Done():
		s.logger.Error("task exceeded timeout and was cancelled",
			"task", t.name, "timeout", taskTimeout.String())
		s.printNextRun(t, time.Now().UTC())
	}
}

func (s *Scheduler) printNextRuns() {
	now := time.Now().UTC()
	for _, t := range s.tasks {
		s.printNextRun(t, now)
	}
}

func (s *Scheduler) printNextRun(t *task, now time.Time) {
	mins := t.nextRun.Sub(now).Minutes()
	fmt.Printf("[scheduler] %-12s next run in %.1f minutes (at %s UTC)\n",
		t.name, mins, t.nextRun.Format("15:04:05"))
}

// alignTime returns the next clock-aligned time for a given interval.
// It ensures that the returned time is strictly after 'now'.
func alignTime(now time.Time, interval time.Duration) time.Time {
	aligned := now.Truncate(interval)
	if !aligned.After(now) {
		aligned = aligned.Add(interval)
	}
	return aligned
}
