package services

import (
	"time"

	"birthly/internal/store/models"
)

// UserToday returns today's date in the user's configured timezone
// (SPEC.md chapter 9), represented as a UTC-midnight time.Time — the
// project's "pure date" convention (matches how Date columns round-trip
// through SQLite, see internal/store/repo/scan.go).
func UserToday(user *models.User) time.Time {
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
