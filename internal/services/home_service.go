package services

import (
	"context"

	"birthly/internal/core"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// HomeSummary holds live counts for the S1 home screen banner.
type HomeSummary struct {
	TodayCount       int
	WeekCount        int
	NearestEvent     *models.Event
	NearestDaysUntil *int
}

// GetHomeSummary computes today/this-week counts and the nearest upcoming event.
func GetHomeSummary(ctx context.Context, db repo.DBTX, user *models.User) (*HomeSummary, error) {
	events := repo.NewEventRepo(db, user.ID)
	today := UserToday(user)
	weekEnd := today.AddDate(0, 0, 7)

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
		TodayCount:       todayCount,
		WeekCount:        len(upcoming),
		NearestEvent:     nearest,
		NearestDaysUntil: nearestDays,
	}, nil
}
