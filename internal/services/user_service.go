package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// GetOrCreateUser fetches the user's row, creating it on first contact and
// refreshing profile fields otherwise.
//
// A brand-new user gets two default global reminder rules (SPEC.md chapter
// 8): "the day before" and "the day of", both at the user's default time.
// User creation + the two default rules run inside one transaction — Python
// gets this atomicity for free from a single session.commit(); Go needs it
// explicit, hence taking *sql.DB (not repo.DBTX) here specifically to own
// the transaction boundary.
func GetOrCreateUser(ctx context.Context, db *sql.DB, userID int64, username *string, firstName string, lastName *string, isAdmin bool) (*models.User, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get_or_create_user: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	users := repo.NewUserRepo(tx)
	user, created, err := users.GetOrCreate(ctx, userID, username, firstName, lastName, isAdmin)
	if err != nil {
		return nil, err
	}

	if created {
		rules := repo.NewReminderRuleRepo(tx, user.ID)
		dayBefore, dayOf := 1, 0
		if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &dayBefore, Enabled: true}); err != nil {
			return nil, err
		}
		if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &dayOf, Enabled: true}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("get_or_create_user: commit: %w", err)
	}
	return user, nil
}

// SyncAdmins promotes every user id in adminIDs to is_admin=1 — port of
// app/main.py's _sync_admins, run once at process startup. This is
// deliberately one-directional (promote only, matching Python exactly): a
// user removed from ADMIN_IDS keeps is_admin=1 until explicitly demoted
// elsewhere. Per-request traffic already sets is_admin correctly for a
// brand-new user at creation time (userHandler passes the same computed
// flag into GetOrCreateUser's INSERT) — this startup sync exists only to
// catch a user who was already in the DB *before* being added to
// ADMIN_IDS, since GetOrCreate's existing-user branch never touches
// is_admin on subsequent contacts.
func SyncAdmins(ctx context.Context, db repo.DBTX, adminIDs []int64) error {
	if len(adminIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(adminIDs))
	args := make([]any, len(adminIDs))
	for i, id := range adminIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := "UPDATE users SET is_admin = 1 WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

func SetLanguage(ctx context.Context, db repo.DBTX, user *models.User, language string) error {
	user.Language = language
	return repo.NewUserRepo(db).UpdateSettings(ctx, user)
}

func SetTimezone(ctx context.Context, db repo.DBTX, user *models.User, timezone string) error {
	user.Timezone = timezone
	return repo.NewUserRepo(db).UpdateSettings(ctx, user)
}

func SetDefaultNotifyTime(ctx context.Context, db repo.DBTX, user *models.User, hhmm string) error {
	user.DefaultNotifyTime = hhmm
	return repo.NewUserRepo(db).UpdateSettings(ctx, user)
}

func CompleteOnboarding(ctx context.Context, db repo.DBTX, user *models.User) error {
	user.Onboarded = true
	return repo.NewUserRepo(db).UpdateSettings(ctx, user)
}
