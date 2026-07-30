package services

import (
	"context"

	"birthly/internal/core"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// HomeSummary holds live counts for the S1 home screen banner.
type HomeSummary struct {
	TotalCount       int
	TodayCount       int
	WeekCount        int
	NearestEvent     *models.Event
	NearestDaysUntil *int
}

// GetHomeSummary computes today/this-week counts and the nearest upcoming event.
//
// TotalCount (every non-deleted event, regardless of date) is what decides
// the empty state — NearestEvent only covers the next 7 days, so a user
// whose events (freshly added or backup-imported) all fall further out than
// that window would otherwise see the "you haven't added anyone yet" empty
// message despite having events on file.
func GetHomeSummary(ctx context.Context, db repo.DBTX, user *models.User) (*HomeSummary, error) {
	events := repo.NewEventRepo(db, user.ID)
	today := UserToday(user)
	weekEnd := today.AddDate(0, 0, 7)

	total, err := events.CountNotDeleted(ctx)
	if err != nil {
		return nil, err
	}

	upcoming, err := events.UpcomingBetween(ctx, today, weekEnd)
	if err != nil {
		return nil, err
	}

	todayCount := 0
	for _, e := range upcoming {
		if e.NextOccurrence != nil && e.NextOccurrence.Equal(today) {
			todayCount++
		}
	}

	var nearest *models.Event
	var nearestDays *int
	if len(upcoming) > 0 {
		nearest = upcoming[0]
		d := core.DaysUntil(*nearest.NextOccurrence, today)
		nearestDays = &d
	}

	return &HomeSummary{
		TotalCount:       total,
		TodayCount:       todayCount,
		WeekCount:        len(upcoming),
		NearestEvent:     nearest,
		NearestDaysUntil: nearestDays,
	}, nil
}
