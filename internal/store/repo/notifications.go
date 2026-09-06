package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"birthly/internal/store/models"
)

// NotificationRepo is NOT scoped to a single user_id: the scheduler tick
// processes all users in one pass.
type NotificationRepo struct {
	db DBTX
}

func NewNotificationRepo(db DBTX) *NotificationRepo {
	return &NotificationRepo{db: db}
}

const notificationLogColumns = `id, user_id, event_id, rule_id, occurrence_date, scheduled_at, sent_at, status, error, attempts`

func scanNotificationLog(row interface{ Scan(...any) error }) (*models.NotificationLog, error) {
	var n models.NotificationLog
	err := row.Scan(&n.ID, &n.UserID, &n.EventID, &n.RuleID, &n.OccurrenceDate, &n.ScheduledAt, &n.SentAt, &n.Status, &n.Error, &n.Attempts)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// Exists reports whether a log row already exists for this (event, rule, date) triple.
func (r *NotificationRepo) Exists(ctx context.Context, eventID int64, ruleID *int64, occurrenceDate time.Time) (bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM notifications_log WHERE event_id = ? AND rule_id IS ? AND occurrence_date = ?`,
		eventID, ruleID, formatDate(occurrenceDate),
	).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// CreatePending inserts a pending row before sending. Returns (nil, nil) if
// the UNIQUE(event_id, rule_id, occurrence_date) constraint fires — callers
// must treat that as "already handled, skip", not an error.
func (r *NotificationRepo) CreatePending(ctx context.Context, userID, eventID int64, ruleID *int64, occurrenceDate, scheduledAt time.Time) (*models.NotificationLog, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications_log (user_id, event_id, rule_id, occurrence_date, scheduled_at, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, eventID, ruleID, formatDate(occurrenceDate), formatDateTime(scheduledAt), models.NotificationStatusPending,
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return nil, nil
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `SELECT `+notificationLogColumns+` FROM notifications_log WHERE id = ?`, id)
	return scanNotificationLog(row)
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

var ErrNotFound = errors.New("not found")

func (r *NotificationRepo) MarkSent(ctx context.Context, logID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications_log SET status = ?, sent_at = ?, attempts = attempts + 1 WHERE id = ?`,
		models.NotificationStatusSent, formatDateTime(time.Now()), logID,
	)
	return checkAffected(res, err)
}

func (r *NotificationRepo) MarkFailed(ctx context.Context, logID int64, errText string) error {
	if len(errText) > 500 {
		errText = errText[:500]
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications_log SET status = ?, error = ?, attempts = attempts + 1 WHERE id = ?`,
		models.NotificationStatusFailed, errText, logID,
	)
	return checkAffected(res, err)
}

func (r *NotificationRepo) MarkSkipped(ctx context.Context, logID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications_log SET status = ?, attempts = attempts + 1 WHERE id = ?`,
		models.NotificationStatusSkipped, logID,
	)
	return checkAffected(res, err)
}

// checkAffected mirrors the Python repo's silent no-op when session.get()
// returns None (row doesn't exist): if the row is missing, do nothing rather
// than erroring, since callers never branch on this.
func checkAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	_, err = res.RowsAffected()
	return err
}

// ListActiveUsersWithEvents returns distinct users whose active events fall
// in [windowStart, windowEnd], excluding users who won't receive anything
// (notifications disabled, blocked, or bot-blocked).
func (r *NotificationRepo) ListActiveUsersWithEvents(ctx context.Context, windowStart, windowEnd time.Time) ([]*models.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT `+prefixColumns("u", userColumns)+`
		 FROM users u
		 JOIN events e ON e.user_id = u.id
		 WHERE u.notifications_enabled = 1 AND u.is_blocked = 0 AND u.bot_blocked_by_user = 0
		   AND e.deleted_at IS NULL AND e.is_active = 1
		   AND (
		     (e.next_occurrence IS NOT NULL AND e.next_occurrence >= ? AND e.next_occurrence <= ?)
		     OR (e.secondary_next_occurrence IS NOT NULL AND e.secondary_next_occurrence >= ? AND e.secondary_next_occurrence <= ?)
		   )`,
		formatDate(windowStart), formatDate(windowEnd), formatDate(windowStart), formatDate(windowEnd),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// GetActiveEventsForUser returns active events for a specific user in the occurrence window.
func (r *NotificationRepo) GetActiveEventsForUser(ctx context.Context, userID int64, windowStart, windowEnd time.Time) ([]*models.Event, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events
		 WHERE user_id = ? AND deleted_at IS NULL AND is_active = 1
		   AND (
		     (next_occurrence IS NOT NULL AND next_occurrence >= ? AND next_occurrence <= ?)
		     OR (secondary_next_occurrence IS NOT NULL AND secondary_next_occurrence >= ? AND secondary_next_occurrence <= ?)
		   )`,
		userID, formatDate(windowStart), formatDate(windowEnd), formatDate(windowStart), formatDate(windowEnd),
	)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// GetRulesForUser returns all enabled rules (global + per-event) for the user.
func (r *NotificationRepo) GetRulesForUser(ctx context.Context, userID int64) ([]*models.ReminderRule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+reminderRuleColumns+` FROM reminder_rules WHERE user_id = ? AND enabled = 1`, userID,
	)
	if err != nil {
		return nil, err
	}
	return scanReminderRules(rows)
}

func (r *NotificationRepo) DeleteOldLogs(ctx context.Context, cutoff time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM notifications_log WHERE scheduled_at < ?`, formatDateTime(cutoff))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// GetPendingLogsBefore returns pending logs whose scheduled_at is in the
// past (for recovery on restart).
func (r *NotificationRepo) GetPendingLogsBefore(ctx context.Context, cutoff time.Time) ([]*models.NotificationLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+notificationLogColumns+` FROM notifications_log WHERE status = ? AND scheduled_at <= ?`,
		models.NotificationStatusPending, formatDateTime(cutoff),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []*models.NotificationLog
	for rows.Next() {
		n, err := scanNotificationLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, n)
	}
	return logs, rows.Err()
}

func (r *NotificationRepo) DeleteOldAuditLogs(ctx context.Context, cutoff time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM audit_logs WHERE created_at < ?`, formatDateTime(cutoff))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// prefixColumns rewrites a bare "a, b, c" column list to "alias.a, alias.b, alias.c".
func prefixColumns(alias, columns string) string {
	parts := strings.Split(columns, ",")
	for i, p := range parts {
		parts[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}
