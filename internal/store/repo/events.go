package repo

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"birthly/internal/store/models"
)

// EventRepo is scoped to a single user_id — every query filters on it.
// There is deliberately no method that fetches a row by id alone; see
// SPEC.md chapter 27 (IDOR protection) and app/db/repositories/base.py.
type EventRepo struct {
	db     DBTX
	userID int64
}

func NewEventRepo(db DBTX, userID int64) *EventRepo {
	return &EventRepo{db: db, userID: userID}
}

const eventColumns = `id, user_id, event_type, custom_type_label, first_name, last_name,
	nickname, gender, relation, category, calendar_type, year, month, day,
	event_time, phone, telegram_username, photo_file_id, notes, next_occurrence,
	secondary_calendar_type, secondary_month, secondary_day, secondary_next_occurrence,
	is_active, deleted_at, created_at, updated_at`

func scanEvent(row interface{ Scan(...any) error }) (*models.Event, error) {
	var e models.Event
	err := row.Scan(
		&e.ID, &e.UserID, &e.EventType, &e.CustomTypeLabel, &e.FirstName, &e.LastName,
		&e.Nickname, &e.Gender, &e.Relation, &e.Category, &e.CalendarType, &e.Year, &e.Month, &e.Day,
		&e.EventTime, &e.Phone, &e.TelegramUsername, &e.PhotoFileID, &e.Notes, &e.NextOccurrence,
		&e.SecondaryCalendarType, &e.SecondaryMonth, &e.SecondaryDay, &e.SecondaryNextOccurrence,
		&e.IsActive, &e.DeletedAt, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func scanEvents(rows *sql.Rows) ([]*models.Event, error) {
	defer rows.Close()
	var events []*models.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetOwned fetches a row by id, scoped to this repo's user_id. Returns
// (nil, nil) if not found or not owned by this user.
func (r *EventRepo) GetOwned(ctx context.Context, id int64) (*models.Event, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events WHERE id = ? AND user_id = ?`, id, r.userID)
	e, err := scanEvent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// CountNotDeleted counts events toward MAX_EVENTS_PER_USER (excludes trash only).
func (r *EventRepo) CountNotDeleted(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE user_id = ? AND deleted_at IS NULL`, r.userID,
	).Scan(&count)
	return count, err
}

// ListNotDeleted returns all events NOT in the trash (used for export).
func (r *EventRepo) ListNotDeleted(ctx context.Context) ([]*models.Event, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE user_id = ? AND deleted_at IS NULL ORDER BY id`, r.userID,
	)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

func (r *EventRepo) ListActive(ctx context.Context) ([]*models.Event, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE user_id = ? AND deleted_at IS NULL AND is_active = 1`, r.userID,
	)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

func (r *EventRepo) UpcomingBetween(ctx context.Context, start, end time.Time) ([]*models.Event, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events
		 WHERE user_id = ? AND deleted_at IS NULL AND is_active = 1
		   AND next_occurrence IS NOT NULL AND next_occurrence >= ? AND next_occurrence <= ?
		 ORDER BY next_occurrence`,
		r.userID, formatDate(start), formatDate(end),
	)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// ListPage returns (events for this page, total count) for the given view/sort.
// view: upcoming (active, not deleted) | all (not deleted, incl. muted) |
// muted (is_active=false) | trash (soft-deleted).
func (r *EventRepo) ListPage(ctx context.Context, sort, view string, page, pageSize int) ([]*models.Event, int, error) {
	where := "user_id = ?"
	args := []any{r.userID}

	switch view {
	case models.ListViewTrash:
		where += " AND deleted_at IS NOT NULL"
	default:
		where += " AND deleted_at IS NULL"
		switch view {
		case models.ListViewUpcoming:
			where += " AND is_active = 1"
		case models.ListViewMuted:
			where += " AND is_active = 0"
		}
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "next_occurrence IS NULL, next_occurrence"
	switch sort {
	case models.SortName:
		orderBy = "first_name, last_name"
	case models.SortCreated:
		orderBy = "created_at DESC"
	case models.SortAge:
		orderBy = "year IS NULL, year"
	}

	queryArgs := append(append([]any{}, args...), pageSize, page*pageSize)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE `+where+` ORDER BY `+orderBy+` LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	events, err := scanEvents(rows)
	return events, total, err
}

// Search implements the S10 free-text search: LIKE on name/nickname/phone/
// notes/relation, or filter by resolved category/month. text and
// (category or month) are mutually exclusive per the caller's own
// resolution — this just applies whichever is given, checked in that order.
func (r *EventRepo) Search(ctx context.Context, text, category *string, month *int) ([]*models.Event, error) {
	where := "user_id = ? AND deleted_at IS NULL"
	args := []any{r.userID}

	switch {
	case category != nil:
		where += " AND category = ?"
		args = append(args, *category)
	case month != nil:
		where += " AND month = ?"
		args = append(args, *month)
	case text != nil:
		// Matches SQLAlchemy's .ilike(): lower(column) LIKE lower(pattern), no
		// wildcard escaping (a literal % or _ in the search text acts as a
		// wildcard on both sides, by design parity).
		like := "%" + strings.ToLower(*text) + "%"
		where += ` AND (
			LOWER(first_name) LIKE ? OR
			LOWER(last_name) LIKE ? OR
			LOWER(nickname) LIKE ? OR
			LOWER(phone) LIKE ? OR
			LOWER(notes) LIKE ? OR
			LOWER(relation) LIKE ?
		)`
		args = append(args, like, like, like, like, like, like)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE `+where+` ORDER BY next_occurrence IS NULL, next_occurrence`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// Create inserts a new event owned by this repo's user_id, ignoring any
// UserID/ID already set on e, and returns the persisted row (with id,
// timestamps, and computed defaults filled in).
func (r *EventRepo) Create(ctx context.Context, e *models.Event) (*models.Event, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO events (user_id, event_type, custom_type_label, first_name, last_name,
			nickname, gender, relation, category, calendar_type, year, month, day,
			event_time, phone, telegram_username, photo_file_id, notes, next_occurrence,
			secondary_calendar_type, secondary_month, secondary_day, secondary_next_occurrence, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.userID, e.EventType, e.CustomTypeLabel, e.FirstName, e.LastName,
		e.Nickname, e.Gender, e.Relation, e.Category, e.CalendarType, e.Year, e.Month, e.Day,
		e.EventTime, e.Phone, e.TelegramUsername, e.PhotoFileID, e.Notes, formatDatePtr(e.NextOccurrence),
		e.SecondaryCalendarType, e.SecondaryMonth, e.SecondaryDay, formatDatePtr(e.SecondaryNextOccurrence), e.IsActive,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetOwned(ctx, id)
}

// Update persists every mutable column of e (identified by e.ID), scoped to
// this repo's user_id, and returns the refreshed row.
func (r *EventRepo) Update(ctx context.Context, e *models.Event) (*models.Event, error) {
	_, err := r.db.ExecContext(ctx,
		`UPDATE events SET event_type=?, custom_type_label=?, first_name=?, last_name=?,
			nickname=?, gender=?, relation=?, category=?, calendar_type=?, year=?, month=?, day=?,
			event_time=?, phone=?, telegram_username=?, photo_file_id=?, notes=?, next_occurrence=?,
			secondary_calendar_type=?, secondary_month=?, secondary_day=?, secondary_next_occurrence=?,
			is_active=?, updated_at=?
		 WHERE id = ? AND user_id = ?`,
		e.EventType, e.CustomTypeLabel, e.FirstName, e.LastName,
		e.Nickname, e.Gender, e.Relation, e.Category, e.CalendarType, e.Year, e.Month, e.Day,
		e.EventTime, e.Phone, e.TelegramUsername, e.PhotoFileID, e.Notes, formatDatePtr(e.NextOccurrence),
		e.SecondaryCalendarType, e.SecondaryMonth, e.SecondaryDay, formatDatePtr(e.SecondaryNextOccurrence),
		e.IsActive, formatDateTime(time.Now()),
		e.ID, r.userID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetOwned(ctx, e.ID)
}

func (r *EventRepo) SoftDelete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE events SET deleted_at = ? WHERE id = ? AND user_id = ?`,
		formatDateTime(time.Now()), id, r.userID,
	)
	return err
}

func (r *EventRepo) Restore(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE events SET deleted_at = NULL WHERE id = ? AND user_id = ?`, id, r.userID,
	)
	return err
}

// PurgeDeletedBefore hard-deletes trashed events older than cutoff, returning the count removed.
func (r *EventRepo) PurgeDeletedBefore(ctx context.Context, cutoff time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM events WHERE user_id = ? AND deleted_at IS NOT NULL AND deleted_at < ?`,
		r.userID, formatDateTime(cutoff),
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}
