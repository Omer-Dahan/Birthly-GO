package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// InvalidTimezoneErr is returned when a user-typed IANA timezone name doesn't exist.
type InvalidTimezoneErr struct{ TZ string }

func (e *InvalidTimezoneErr) Error() string { return fmt.Sprintf("invalid timezone: %s", e.TZ) }

// UpdateSettings persists whatever fields the caller already mutated on
// user (SPEC.md ch.20: "שינוי נשמר מיד — אין 'שמור'"). Go has no equivalent
// to Python's update_setting(**fields)/setattr; the caller sets the Go
// struct field(s) directly, then calls this to persist the whole row —
// same net effect as Python's dynamic version, since both write back every
// current field value.
func UpdateSettings(ctx context.Context, db repo.DBTX, user *models.User) error {
	return repo.NewUserRepo(db).UpdateSettings(ctx, user)
}

// ValidateTimezone checks tz against the IANA tzdata the Go runtime has
// loaded (time.LoadLocation succeeding is the practical equivalent of
// Python's `tz in zoneinfo.available_timezones()` — both defer to the
// same underlying tzdata).
func ValidateTimezone(tz string) (string, error) {
	cleaned := strings.TrimSpace(tz)
	if _, err := time.LoadLocation(cleaned); err != nil {
		return "", &InvalidTimezoneErr{TZ: cleaned}
	}
	return cleaned, nil
}

// WipeAccount permanently deletes the user row — ON DELETE CASCADE removes
// events, rules, etc.
func WipeAccount(ctx context.Context, db repo.DBTX, user *models.User) error {
	return repo.NewUserRepo(db).Delete(ctx, user.ID)
}
