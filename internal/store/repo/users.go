package repo

import (
	"context"
	"database/sql"
	"time"

	"birthly/internal/store/models"
)

// UserRepo is NOT user-scoped like the other repos: users manage their own row.
type UserRepo struct {
	db DBTX
}

func NewUserRepo(db DBTX) *UserRepo {
	return &UserRepo{db: db}
}

const userColumns = `id, username, first_name, last_name, language, timezone, date_format,
	time_format, default_notify_time, notifications_enabled, silent_notifications,
	show_hebrew_date, daily_digest_enabled, digest_time, list_sort, list_filter,
	adar_policy, feb29_policy, is_admin, is_blocked, bot_blocked_by_user, onboarded,
	created_at, updated_at, last_seen_at`

func scanUser(row interface{ Scan(...any) error }) (*models.User, error) {
	var u models.User
	err := row.Scan(
		&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.Language, &u.Timezone, &u.DateFormat,
		&u.TimeFormat, &u.DefaultNotifyTime, &u.NotificationsEnabled, &u.SilentNotifications,
		&u.ShowHebrewDate, &u.DailyDigestEnabled, &u.DigestTime, &u.ListSort, &u.ListFilter,
		&u.AdarPolicy, &u.Feb29Policy, &u.IsAdmin, &u.IsBlocked, &u.BotBlockedByUser, &u.Onboarded,
		&u.CreatedAt, &u.UpdatedAt, &u.LastSeenAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) Get(ctx context.Context, userID int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, userID)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// GetOrCreate returns (user, created) — created is true only on first contact.
func (r *UserRepo) GetOrCreate(ctx context.Context, userID int64, username *string, firstName string, lastName *string, isAdmin bool) (*models.User, bool, error) {
	existing, err := r.Get(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	now := time.Now()
	if existing != nil {
		_, err := r.db.ExecContext(ctx,
			`UPDATE users SET username = ?, first_name = ?, last_name = ?, last_seen_at = ? WHERE id = ?`,
			username, firstName, lastName, formatDateTime(now), userID,
		)
		if err != nil {
			return nil, false, err
		}
		updated, err := r.Get(ctx, userID)
		return updated, false, err
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO users (id, username, first_name, last_name, is_admin) VALUES (?, ?, ?, ?, ?)`,
		userID, username, firstName, lastName, isAdmin,
	)
	if err != nil {
		return nil, false, err
	}
	created, err := r.Get(ctx, userID)
	return created, true, err
}

func (r *UserRepo) TouchLastSeen(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET last_seen_at = ? WHERE id = ?`, formatDateTime(time.Now()), userID,
	)
	return err
}

// UpdateSettings persists the mutable settings fields of u (identified by u.ID).
func (r *UserRepo) UpdateSettings(ctx context.Context, u *models.User) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET language=?, timezone=?, date_format=?, time_format=?, default_notify_time=?,
			notifications_enabled=?, silent_notifications=?, show_hebrew_date=?, daily_digest_enabled=?,
			digest_time=?, list_sort=?, list_filter=?, adar_policy=?, feb29_policy=?, onboarded=?,
			updated_at=?
		 WHERE id = ?`,
		u.Language, u.Timezone, u.DateFormat, u.TimeFormat, u.DefaultNotifyTime,
		u.NotificationsEnabled, u.SilentNotifications, u.ShowHebrewDate, u.DailyDigestEnabled,
		u.DigestTime, u.ListSort, u.ListFilter, u.AdarPolicy, u.Feb29Policy, u.Onboarded,
		formatDateTime(time.Now()),
		u.ID,
	)
	return err
}

func (r *UserRepo) SetBlocked(ctx context.Context, userID int64, blocked bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET is_blocked = ? WHERE id = ?`, blocked, userID)
	return err
}

func (r *UserRepo) SetBotBlockedByUser(ctx context.Context, userID int64, blocked bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET bot_blocked_by_user = ? WHERE id = ?`, blocked, userID)
	return err
}

// Delete permanently removes the user row; ON DELETE CASCADE removes their
// events, rules, templates, and audit logs. Matches settings_service.wipe_account.
func (r *UserRepo) Delete(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	return err
}

// GetByUsername looks up a user by @username, case-insensitively (matches
// admin_service.get_user_info's non-numeric identifier branch).
func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE LOWER(username) = LOWER(?)`, username,
	)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// ListBroadcastTargets returns non-blocked, non-bot-blocked users (admin_service.get_broadcast_targets).
func (r *UserRepo) ListBroadcastTargets(ctx context.Context) ([]*models.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE is_blocked = 0 AND bot_blocked_by_user = 0 ORDER BY id`,
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

func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// CountActiveSince counts users whose last_seen_at is on/after cutoff (stats_service).
func (r *UserRepo) CountActiveSince(ctx context.Context, cutoff time.Time) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE last_seen_at >= ?`, formatDateTime(cutoff),
	).Scan(&count)
	return count, err
}
