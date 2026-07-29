package repo

import (
	"context"
	"database/sql"

	"birthly/internal/store/models"
)

// ReminderRuleRepo is scoped to a single user_id.
type ReminderRuleRepo struct {
	db     *sql.DB
	userID int64
}

func NewReminderRuleRepo(db *sql.DB, userID int64) *ReminderRuleRepo {
	return &ReminderRuleRepo{db: db, userID: userID}
}

const reminderRuleColumns = `id, user_id, event_id, offset_days, offset_minutes, send_time, enabled, created_at`

func scanReminderRule(row interface{ Scan(...any) error }) (*models.ReminderRule, error) {
	var r models.ReminderRule
	err := row.Scan(&r.ID, &r.UserID, &r.EventID, &r.OffsetDays, &r.OffsetMinutes, &r.SendTime, &r.Enabled, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func scanReminderRules(rows *sql.Rows) ([]*models.ReminderRule, error) {
	defer rows.Close()
	var rules []*models.ReminderRule
	for rows.Next() {
		r, err := scanReminderRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (r *ReminderRuleRepo) ListGlobal(ctx context.Context) ([]*models.ReminderRule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+reminderRuleColumns+` FROM reminder_rules WHERE user_id = ? AND event_id IS NULL`, r.userID,
	)
	if err != nil {
		return nil, err
	}
	return scanReminderRules(rows)
}

func (r *ReminderRuleRepo) ListForEvent(ctx context.Context, eventID int64) ([]*models.ReminderRule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+reminderRuleColumns+` FROM reminder_rules WHERE user_id = ? AND event_id = ?`, r.userID, eventID,
	)
	if err != nil {
		return nil, err
	}
	return scanReminderRules(rows)
}

func (r *ReminderRuleRepo) CountGlobal(ctx context.Context) (int, error) {
	rules, err := r.ListGlobal(ctx)
	return len(rules), err
}

func (r *ReminderRuleRepo) CountForEvent(ctx context.Context, eventID int64) (int, error) {
	rules, err := r.ListForEvent(ctx, eventID)
	return len(rules), err
}

// GetOwned fetches a rule by id, scoped to this repo's user_id.
func (r *ReminderRuleRepo) GetOwned(ctx context.Context, id int64) (*models.ReminderRule, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+reminderRuleColumns+` FROM reminder_rules WHERE id = ? AND user_id = ?`, id, r.userID,
	)
	rule, err := scanReminderRule(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return rule, err
}

func (r *ReminderRuleRepo) Create(ctx context.Context, rule *models.ReminderRule) (*models.ReminderRule, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO reminder_rules (user_id, event_id, offset_days, offset_minutes, send_time, enabled)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.userID, rule.EventID, rule.OffsetDays, rule.OffsetMinutes, rule.SendTime, rule.Enabled,
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

func (r *ReminderRuleRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM reminder_rules WHERE id = ? AND user_id = ?`, id, r.userID)
	return err
}

func (r *ReminderRuleRepo) Update(ctx context.Context, rule *models.ReminderRule) (*models.ReminderRule, error) {
	_, err := r.db.ExecContext(ctx,
		`UPDATE reminder_rules SET event_id=?, offset_days=?, offset_minutes=?, send_time=?, enabled=?
		 WHERE id = ? AND user_id = ?`,
		rule.EventID, rule.OffsetDays, rule.OffsetMinutes, rule.SendTime, rule.Enabled,
		rule.ID, r.userID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetOwned(ctx, rule.ID)
}

func (r *ReminderRuleRepo) Toggle(ctx context.Context, id int64) (*models.ReminderRule, error) {
	_, err := r.db.ExecContext(ctx,
		`UPDATE reminder_rules SET enabled = NOT enabled WHERE id = ? AND user_id = ?`, id, r.userID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetOwned(ctx, id)
}
