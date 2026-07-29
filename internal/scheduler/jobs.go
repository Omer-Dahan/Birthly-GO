package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/config"
	"birthly/internal/core"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

func parseHHMM(s string) (int, int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid HH:MM %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return h, m, nil
}

type sendTask struct {
	user           *models.User
	event          *models.Event
	rule           *models.ReminderRule
	logID          int64
	occurrenceYear int
}

type fireKey struct {
	eventID         int64
	y, mo, d, h, mi int
}

// TickReminders is the main reminder engine tick — runs every
// SchedulerTickSeconds seconds. Algorithm (SPEC.md ch.18), ported line for
// line from app/scheduler/jobs.py's tick_reminders since a silent bug here
// is a reminder sent (or not sent) on the wrong date:
//
//  1. Query users with events in the next MaxUpcomingDays window.
//  2. For each user, compute fire_utc for every (event × rule) pair.
//  3. If fire_utc <= now_utc and within grace, insert pending log row
//     (UNIQUE prevents double-send), then send.
//  4. Recompute next_occurrence for events whose occurrence == today.
//
// nowUTC defaults to time.Now().UTC() in production; tests pass it
// explicitly for determinism (matching the Python parameter's purpose,
// though Go has no freezegun-corruption concern to avoid).
func TickReminders(ctx context.Context, bot *gotgbot.Bot, db *sql.DB, cfg *config.Config, nowUTC time.Time, logger *slog.Logger) error {
	if nowUTC.IsZero() {
		nowUTC = time.Now().UTC()
	}
	grace := time.Duration(cfg.ReminderGraceHours) * time.Hour

	notifRepo := repo.NewNotificationRepo(db)

	windowStart := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
	windowEnd := windowStart.AddDate(0, 0, cfg.MaxUpcomingDays)

	users, err := notifRepo.ListActiveUsersWithEvents(ctx, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("tick_reminders: list active users: %w", err)
	}
	logger.Debug("tick_start", "user_count", len(users), "now_utc", nowUTC)

	var sendTasks []sendTask

	for _, user := range users {
		loc, err := time.LoadLocation(user.Timezone)
		if err != nil {
			loc = time.UTC
		}
		nowLocal := nowUTC.In(loc)
		todayLocal := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.UTC)

		events, err := notifRepo.GetActiveEventsForUser(ctx, user.ID, windowStart, windowEnd)
		if err != nil {
			return fmt.Errorf("tick_reminders: get active events for user %d: %w", user.ID, err)
		}
		rules, err := notifRepo.GetRulesForUser(ctx, user.ID)
		if err != nil {
			return fmt.Errorf("tick_reminders: get rules for user %d: %w", user.ID, err)
		}

		var globalRules []*models.ReminderRule
		perEventRules := map[int64][]*models.ReminderRule{}
		for _, r := range rules {
			if r.EventID == nil {
				globalRules = append(globalRules, r)
			} else {
				perEventRules[*r.EventID] = append(perEventRules[*r.EventID], r)
			}
		}

		for _, event := range events {
			occ := event.NextOccurrence
			if occ == nil {
				continue
			}

			eventSpecific := perEventRules[event.ID]
			overriddenOffsets := map[int]bool{}
			for _, r := range eventSpecific {
				if r.OffsetDays != nil {
					overriddenOffsets[*r.OffsetDays] = true
				}
			}
			applicableRules := append([]*models.ReminderRule{}, eventSpecific...)
			for _, gr := range globalRules {
				if gr.OffsetDays == nil || !overriddenOffsets[*gr.OffsetDays] {
					applicableRules = append(applicableRules, gr)
				}
			}

			seenFireMinutes := map[fireKey]bool{}

			for _, rule := range applicableRules {
				var fireLocal time.Time
				switch {
				case rule.OffsetDays != nil:
					sendTimeStr := user.DefaultNotifyTime
					if rule.SendTime != nil {
						sendTimeStr = *rule.SendTime
					}
					sh, sm, err := parseHHMM(sendTimeStr)
					if err != nil {
						logger.Warn("tick_reminders: bad send_time", "user_id", user.ID, "value", sendTimeStr)
						continue
					}
					fireDate := occ.AddDate(0, 0, -*rule.OffsetDays)
					fireLocal = time.Date(fireDate.Year(), fireDate.Month(), fireDate.Day(), sh, sm, 0, 0, loc)

				case rule.OffsetMinutes != nil:
					if event.EventTime == nil || *event.EventTime == "" {
						continue // only meaningful when event has a time
					}
					eh, em, err := parseHHMM(*event.EventTime)
					if err != nil {
						continue
					}
					eventLocal := time.Date(occ.Year(), occ.Month(), occ.Day(), eh, em, 0, 0, loc)
					fireLocal = eventLocal.Add(-time.Duration(*rule.OffsetMinutes) * time.Minute)

				default:
					continue
				}

				fireUTC := fireLocal.UTC()

				if fireUTC.After(nowUTC) {
					continue // not yet
				}

				if nowUTC.Sub(fireUTC) > grace {
					log, err := notifRepo.CreatePending(ctx, user.ID, event.ID, &rule.ID, *occ, fireUTC)
					if err != nil {
						return fmt.Errorf("tick_reminders: create_pending (grace-skip): %w", err)
					}
					if log != nil {
						_ = notifRepo.MarkSkipped(ctx, log.ID)
						logger.Warn("reminder_skipped_grace", "user_id", user.ID, "event_id", event.ID, "fire_utc", fireUTC)
					}
					continue
				}

				fk := fireKey{event.ID, fireUTC.Year(), int(fireUTC.Month()), fireUTC.Day(), fireUTC.Hour(), fireUTC.Minute()}
				if seenFireMinutes[fk] {
					continue
				}
				seenFireMinutes[fk] = true

				log, err := notifRepo.CreatePending(ctx, user.ID, event.ID, &rule.ID, *occ, fireUTC)
				if err != nil {
					return fmt.Errorf("tick_reminders: create_pending: %w", err)
				}
				if log == nil {
					logger.Debug("reminder_already_logged", "user_id", user.ID, "event_id", event.ID)
					continue
				}

				sendTasks = append(sendTasks, sendTask{user, event, rule, log.ID, occ.Year()})
			}
		}

		// After processing all rules, recompute next_occurrence for events
		// where occ == today (the birthday has passed for this year).
		events2 := repo.NewEventRepo(db, user.ID)
		for _, event := range events {
			if event.NextOccurrence != nil && event.NextOccurrence.Equal(todayLocal) {
				newOcc, err := core.NextOccurrence(
					event.CalendarType, event.Month, event.Day,
					todayLocal.AddDate(0, 0, 1),
					user.AdarPolicy, user.Feb29Policy,
				)
				if err != nil {
					logger.Error("tick_reminders: recompute occurrence failed", "event_id", event.ID, "error", err)
					continue
				}
				event.NextOccurrence = &newOcc
				if _, err := events2.Update(ctx, event); err != nil {
					return fmt.Errorf("tick_reminders: update recomputed occurrence: %w", err)
				}
			}
		}
	}

	for _, task := range sendTasks {
		SendReminder(ctx, bot, db, task.user, task.event, task.rule, task.logID, task.occurrenceYear, cfg.BroadcastRatePerSec)
	}

	if err := recordLastTick(ctx, db, nowUTC); err != nil {
		return fmt.Errorf("tick_reminders: record last tick: %w", err)
	}

	logger.Info("tick_done", "sends_queued", len(sendTasks))
	return nil
}

func recordLastTick(ctx context.Context, db *sql.DB, nowUTC time.Time) error {
	return repo.NewAppMetaRepo(db).Set(ctx, "last_tick_at", nowUTC.Format(time.RFC3339))
}

// RecomputeOccurrences recalculates next_occurrence for every active event.
// Called daily at 02:00 UTC and also triggered after any event create/update
// (event_service already does the latter inline; this job is the daily
// safety net for drift/edge cases).
func RecomputeOccurrences(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	logger.Info("recompute_occurrences_start")
	// Every user, not just broadcast targets: a blocked user's events still
	// need correct next_occurrence for when they're unblocked.
	users, err := listAllUsers(ctx, db)
	if err != nil {
		return err
	}

	updated := 0
	for _, user := range users {
		loc, err := time.LoadLocation(user.Timezone)
		if err != nil {
			loc = time.UTC
		}
		now := time.Now().In(loc)
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

		events := repo.NewEventRepo(db, user.ID)
		list, err := events.ListActive(ctx)
		if err != nil {
			return err
		}
		for _, event := range list {
			newOcc, err := core.NextOccurrence(event.CalendarType, event.Month, event.Day, today, user.AdarPolicy, user.Feb29Policy)
			if err != nil {
				logger.Error("recompute_occurrences: failed", "event_id", event.ID, "error", err)
				continue
			}
			if event.NextOccurrence == nil || !event.NextOccurrence.Equal(newOcc) {
				event.NextOccurrence = &newOcc
				if _, err := events.Update(ctx, event); err != nil {
					return err
				}
				updated++
			}
		}
	}

	logger.Info("recompute_occurrences_done", "updated", updated)
	return nil
}

// listAllUsers is a small helper since UserRepo has no unfiltered "all
// users" method (every existing caller wanted a filtered subset).
func listAllUsers(ctx context.Context, db *sql.DB) ([]*models.User, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	users := repo.NewUserRepo(db)
	result := make([]*models.User, 0, len(ids))
	for _, id := range ids {
		u, err := users.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if u != nil {
			result = append(result, u)
		}
	}
	return result, nil
}

// DailyDigest is a stub — full implementation is a later milestone in the
// Python app too (app/scheduler/jobs.py: "Full implementation: M6"). Ported
// as a stub, not invented.
func DailyDigest(ctx context.Context, bot *gotgbot.Bot, db *sql.DB, logger *slog.Logger) error {
	logger.Debug("daily_digest_tick")
	return nil
}

// AutoBackup creates a VACUUM INTO snapshot of the DB and prunes backups
// older than the retention window. VACUUM INTO is safe during live
// operation (unlike a file copy).
func AutoBackup(ctx context.Context, db *sql.DB, cfg *config.Config, logger *slog.Logger) error {
	if err := os.MkdirAll(cfg.BackupDir, 0o755); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102")
	dest := filepath.Join(cfg.BackupDir, "birthly_"+stamp+".db")

	logger.Info("auto_backup_start", "dest", dest)

	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	escaped := strings.ReplaceAll(absDest, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		logger.Error("auto_backup_failed", "error", err)
		return nil // matches Python: log and return, don't propagate
	}

	cutoffStamp := time.Now().UTC().AddDate(0, 0, -cfg.BackupRetentionDays).Format("20060102")
	pruned := 0
	entries, err := os.ReadDir(cfg.BackupDir)
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, "birthly_") || !strings.HasSuffix(name, ".db") {
				continue
			}
			stem := strings.TrimSuffix(name, ".db")
			parts := strings.Split(stem, "_")
			fileStamp := parts[len(parts)-1]
			if fileStamp < cutoffStamp {
				if err := os.Remove(filepath.Join(cfg.BackupDir, name)); err == nil {
					pruned++
				}
			}
		}
	}

	logger.Info("auto_backup_done", "dest", dest, "pruned", pruned)
	return nil
}

// PurgeSoftDeleted hard-deletes events with deleted_at older than
// SoftDeleteRetentionDays.
func PurgeSoftDeleted(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -core.SoftDeleteRetentionDays)
	logger.Info("purge_soft_deleted_start", "cutoff", cutoff)

	users, err := listAllUsers(ctx, db)
	if err != nil {
		return err
	}
	total := 0
	for _, user := range users {
		n, err := repo.NewEventRepo(db, user.ID).PurgeDeletedBefore(ctx, cutoff)
		if err != nil {
			return err
		}
		total += n
	}
	logger.Info("purge_soft_deleted_done", "purged", total)
	return nil
}

// CleanupLogs deletes old audit_logs (>90d) and notifications_log (>180d).
func CleanupLogs(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	now := time.Now().UTC()
	auditCutoff := now.AddDate(0, 0, -90)
	notifCutoff := now.AddDate(0, 0, -180)

	notifRepo := repo.NewNotificationRepo(db)
	deletedAudit, err := notifRepo.DeleteOldAuditLogs(ctx, auditCutoff)
	if err != nil {
		return err
	}
	deletedNotif, err := notifRepo.DeleteOldLogs(ctx, notifCutoff)
	if err != nil {
		return err
	}

	logger.Info("cleanup_logs_done", "audit_deleted", deletedAudit, "notif_deleted", deletedNotif)
	return nil
}
