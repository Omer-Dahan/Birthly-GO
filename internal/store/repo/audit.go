package repo

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"birthly/internal/store/models"
)

// AuditRepo is scoped to a single user_id (the actor).
type AuditRepo struct {
	db     *sql.DB
	userID int64
}

func NewAuditRepo(db *sql.DB, userID int64) *AuditRepo {
	return &AuditRepo{db: db, userID: userID}
}

// Add records an audit log entry. payload, if non-nil, is JSON-encoded
// without ASCII-escaping non-Latin text (matches json.dumps(..., ensure_ascii=False)).
func (r *AuditRepo) Add(ctx context.Context, action, entity string, entityID *int64, payload map[string]any) (*models.AuditLog, error) {
	var payloadStr *string
	if payload != nil {
		encoded, err := marshalNoEscapeHTML(payload)
		if err != nil {
			return nil, err
		}
		payloadStr = &encoded
	}

	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_logs (user_id, action, entity, entity_id, payload) VALUES (?, ?, ?, ?, ?)`,
		r.userID, action, entity, entityID, payloadStr,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, action, entity, entity_id, payload, created_at FROM audit_logs WHERE id = ?`, id,
	)
	var a models.AuditLog
	if err := row.Scan(&a.ID, &a.UserID, &a.Action, &a.Entity, &a.EntityID, &a.Payload, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

// marshalNoEscapeHTML JSON-encodes v without HTML-escaping (<,>,&), matching
// Python's json.dumps(ensure_ascii=False): encoding/json's Marshal escapes
// those three runes by default even though it never escapes non-ASCII text,
// so Encoder.SetEscapeHTML(false) is required for a faithful port.
func marshalNoEscapeHTML(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}
