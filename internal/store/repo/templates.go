package repo

import (
	"context"
	"database/sql"

	"birthly/internal/store/models"
)

// TemplateRepo is scoped to a single user_id for personal-template
// operations; system templates (user_id IS NULL) are readable by everyone.
type TemplateRepo struct {
	db     DBTX
	userID int64
}

func NewTemplateRepo(db DBTX, userID int64) *TemplateRepo {
	return &TemplateRepo{db: db, userID: userID}
}

const templateColumns = `id, user_id, event_type, tone, gender, language, body, is_active`

func scanTemplate(row interface{ Scan(...any) error }) (*models.GreetingTemplate, error) {
	var t models.GreetingTemplate
	err := row.Scan(&t.ID, &t.UserID, &t.EventType, &t.Tone, &t.Gender, &t.Language, &t.Body, &t.IsActive)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func scanTemplates(rows *sql.Rows) ([]*models.GreetingTemplate, error) {
	defer rows.Close()
	var templates []*models.GreetingTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// ListMatching returns active templates matching the query: both system
// templates (user_id IS NULL) and this user's personal templates. A nil
// gender matches only gender-neutral templates left unfiltered by SQL — the
// gender filter itself is applied only when gender != nil, matching
// TemplateRepository.list_matching's `if gender:` branch.
func (r *TemplateRepo) ListMatching(ctx context.Context, eventType, tone string, gender *string, language string) ([]*models.GreetingTemplate, error) {
	query := `SELECT ` + templateColumns + ` FROM greeting_templates
		WHERE is_active = 1 AND event_type = ? AND tone = ? AND language = ?
		  AND (user_id IS NULL OR user_id = ?)`
	args := []any{eventType, tone, language, r.userID}

	if gender != nil && *gender != "" {
		query += ` AND (gender IS NULL OR gender = ?)`
		args = append(args, *gender)
	}
	query += ` ORDER BY id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanTemplates(rows)
}

// ListUserTemplates returns all active personal templates for this user (any type/tone).
func (r *TemplateRepo) ListUserTemplates(ctx context.Context) ([]*models.GreetingTemplate, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+templateColumns+` FROM greeting_templates WHERE user_id = ? AND is_active = 1 ORDER BY id`,
		r.userID,
	)
	if err != nil {
		return nil, err
	}
	return scanTemplates(rows)
}

// CountUserTemplates counts active personal templates (for the 20-template limit check).
func (r *TemplateRepo) CountUserTemplates(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM greeting_templates WHERE user_id = ? AND is_active = 1`, r.userID,
	).Scan(&count)
	return count, err
}

// GetOwned fetches a personal template by id, scoped to this repo's user_id.
func (r *TemplateRepo) GetOwned(ctx context.Context, id int64) (*models.GreetingTemplate, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+templateColumns+` FROM greeting_templates WHERE id = ? AND user_id = ?`, id, r.userID,
	)
	t, err := scanTemplate(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// Create persists a new personal template owned by this repo's user_id.
func (r *TemplateRepo) Create(ctx context.Context, t *models.GreetingTemplate) (*models.GreetingTemplate, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO greeting_templates (user_id, event_type, tone, gender, language, body, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, 1)`,
		r.userID, t.EventType, t.Tone, t.Gender, t.Language, t.Body,
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

// Delete soft-deletes a personal template (marks inactive, keeps history intact).
func (r *TemplateRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE greeting_templates SET is_active = 0 WHERE id = ? AND user_id = ?`, id, r.userID,
	)
	return err
}
