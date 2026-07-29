package services

import (
	"context"

	"birthly/internal/core"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// EventAge pairs an event with the age it reaches on its next occurrence.
type EventAge struct {
	Event *models.Event
	Age   int
}

// UserStats holds the SPEC.md ch.21 aggregate stats for the S13 screen.
type UserStats struct {
	Total            int
	Today            int
	ThisWeek         int
	ThisMonth        int
	Next30Days       int
	Nearest          *models.Event
	NearestDaysUntil *int
	ByCategory       map[string]int
	// ByCategoryOrder preserves first-appearance order while scanning events
	// (ascending id, same order the repo query uses) — matching Python's
	// dict insertion-order iteration in stats.py's `for cat, count in
	// stats.by_category.items()`. A plain map range in Go would iterate in
	// randomized order instead.
	ByCategoryOrder []string
	ByType          map[string]int
	ByTypeOrder     []string
	ByMonth         map[int]int
	Youngest        *EventAge
	Oldest          *EventAge
	AvgAge          *float64
	WithoutYear     int
	Muted           int
}

// GetUserStats loads every non-deleted event once and computes all fields
// client-side — fine at this scale (max_events_per_user caps well under a
// page-fault concern), and keeps the whole calculation in one readable pass.
func GetUserStats(ctx context.Context, db repo.DBTX, user *models.User) (*UserStats, error) {
	events := repo.NewEventRepo(db, user.ID)
	list, err := events.ListNotDeleted(ctx)
	if err != nil {
		return nil, err
	}
	today := UserToday(user)
	weekEnd := today.AddDate(0, 0, 7)
	monthEnd := today.AddDate(0, 0, 30)
	next30End := today.AddDate(0, 0, 30)

	stats := &UserStats{
		ByCategory: map[string]int{},
		ByType:     map[string]int{},
		ByMonth:    map[int]int{},
	}
	var ages []EventAge

	for _, event := range list {
		occ := event.NextOccurrence
		if occ != nil {
			if occ.Equal(today) {
				stats.Today++
			}
			if !occ.Before(today) && !occ.After(weekEnd) {
				stats.ThisWeek++
			}
			if !occ.Before(today) && !occ.After(monthEnd) {
				stats.ThisMonth++
			}
			if !occ.Before(today) && !occ.After(next30End) {
				stats.Next30Days++
			}

			if stats.Nearest == nil || occ.Before(*stats.Nearest.NextOccurrence) {
				stats.Nearest = event
				d := core.DaysUntil(*occ, today)
				stats.NearestDaysUntil = &d
			}

			stats.ByMonth[int(occ.Month())]++
		}

		if _, seen := stats.ByCategory[event.Category]; !seen {
			stats.ByCategoryOrder = append(stats.ByCategoryOrder, event.Category)
		}
		stats.ByCategory[event.Category]++
		if _, seen := stats.ByType[event.EventType]; !seen {
			stats.ByTypeOrder = append(stats.ByTypeOrder, event.EventType)
		}
		stats.ByType[event.EventType]++

		if event.Year == nil {
			stats.WithoutYear++
		}
		if !event.IsActive {
			stats.Muted++
		}

		if event.Year != nil && occ != nil {
			if age, ok := core.AgeAt(event.CalendarType, event.Year, *occ); ok {
				ages = append(ages, EventAge{Event: event, Age: age})
			}
		}
	}

	stats.Total = len(list)

	if len(ages) > 0 {
		youngest, oldest := ages[0], ages[0]
		sum := 0
		for _, a := range ages {
			if a.Age < youngest.Age {
				youngest = a
			}
			if a.Age > oldest.Age {
				oldest = a
			}
			sum += a.Age
		}
		yCopy, oCopy := youngest, oldest
		stats.Youngest = &yCopy
		stats.Oldest = &oCopy
		avg := float64(sum) / float64(len(ages))
		stats.AvgAge = &avg
	}

	return stats, nil
}
