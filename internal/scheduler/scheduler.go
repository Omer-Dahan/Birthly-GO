package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/robfig/cron/v3"

	"birthly/internal/config"
)

// safe wraps a job so a panic or slow overlapping run never crashes the
// process or stacks up concurrent executions — port of app/scheduler/
// scheduler.py's _safe() (panic recovery) plus APScheduler's
// max_instances=1 (the running flag below), which Go needs explicitly since
// robfig/cron has no built-in overlap guard.
func safe(name string, logger *slog.Logger, fn func()) func() {
	var running int32
	return func() {
		if !atomic.CompareAndSwapInt32(&running, 0, 1) {
			logger.Warn("scheduler_job_overlap_skipped", "job", name)
			return
		}
		defer atomic.StoreInt32(&running, 0)
		defer func() {
			if r := recover(); r != nil {
				logger.Error("scheduler_job_panic", "job", name, "panic", r, "stack", string(debug.Stack()))
			}
		}()
		fn()
	}
}

// Scheduler owns the cron runner and every registered job.
type Scheduler struct {
	cron *cron.Cron
}

// Build creates and configures the Scheduler with all six jobs registered.
// Jobs are NOT started here — call Start() after this returns, matching
// build_scheduler()/scheduler.start() being separate steps in Python.
//
// robfig/cron's "@every <duration>" spec takes a Go duration string (parsed
// via time.ParseDuration), and standard 5-field cron expressions run in the
// scheduler's configured location (UTC here) — matching every trigger in
// app/scheduler/scheduler.py's build_scheduler exactly: interval jobs stay
// interval jobs, "hour=H, minute=M, timezone=UTC" cron jobs become
// "M H * * *" in UTC, and the Sunday-05:00 weekly job becomes "0 5 * * 0".
func Build(bot *gotgbot.Bot, db *sql.DB, cfg *config.Config, logger *slog.Logger) (*Scheduler, error) {
	c := cron.New(cron.WithLocation(time.UTC))

	autoBackupHour, autoBackupMinute, err := parseHHMM(cfg.AutoBackupTime)
	if err != nil {
		autoBackupHour, autoBackupMinute = 3, 30 // config default
	}

	jobs := []struct {
		name string
		spec string
		fn   func()
	}{
		{"tick_reminders", fmt.Sprintf("@every %ds", cfg.SchedulerTickSeconds), func() {
			if err := TickReminders(context.Background(), bot, db, cfg, time.Time{}, logger); err != nil {
				logger.Error("tick_reminders failed", "error", err)
			}
		}},
		{"recompute_occurrences", "0 2 * * *", func() {
			if err := RecomputeOccurrences(context.Background(), db, logger); err != nil {
				logger.Error("recompute_occurrences failed", "error", err)
			}
		}},
		{"daily_digest", "@every 15m", func() {
			if err := DailyDigest(context.Background(), bot, db, logger); err != nil {
				logger.Error("daily_digest failed", "error", err)
			}
		}},
		{"auto_backup", fmt.Sprintf("%d %d * * *", autoBackupMinute, autoBackupHour), func() {
			if err := AutoBackup(context.Background(), db, cfg, logger); err != nil {
				logger.Error("auto_backup failed", "error", err)
			}
		}},
		{"purge_soft_deleted", "0 4 * * *", func() {
			if err := PurgeSoftDeleted(context.Background(), db, logger); err != nil {
				logger.Error("purge_soft_deleted failed", "error", err)
			}
		}},
		{"cleanup_logs", "0 5 * * 0", func() {
			if err := CleanupLogs(context.Background(), db, logger); err != nil {
				logger.Error("cleanup_logs failed", "error", err)
			}
		}},
	}

	for _, j := range jobs {
		if _, err := c.AddFunc(j.spec, safe(j.name, logger, j.fn)); err != nil {
			return nil, fmt.Errorf("scheduling job %s (%q): %w", j.name, j.spec, err)
		}
	}

	logger.Info("scheduler_built", "job_count", len(jobs), "tick_seconds", cfg.SchedulerTickSeconds)
	return &Scheduler{cron: c}, nil
}

// Start begins running scheduled jobs in the background.
func (s *Scheduler) Start() { s.cron.Start() }

// Stop waits for running jobs to complete (graceful shutdown), matching
// scheduler.shutdown(wait=True).
func (s *Scheduler) Stop() { <-s.cron.Stop().Done() }
