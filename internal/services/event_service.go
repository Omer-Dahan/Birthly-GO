package services

import (
	"context"
	"fmt"

	"birthly/internal/core"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// NewEventInput holds the fields the simplicity contract requires up front;
// everything else keeps its column default and can be filled in later.
type NewEventInput struct {
	FirstName    string
	LastName     *string
	Month        int
	Day          int
	Year         *int
	CalendarType string // defaults to core.CalendarTypeGregorian if empty
	Gender       *string
}

// CreateMinimalEvent creates an event with only the fields the simplicity
// contract requires. maxEventsPerUser is the caller's configured per-user cap.
func CreateMinimalEvent(ctx context.Context, db repo.DBTX, user *models.User, in NewEventInput, maxEventsPerUser int) (*models.Event, error) {
	events := repo.NewEventRepo(db, user.ID)
	count, err := events.CountNotDeleted(ctx)
	if err != nil {
		return nil, err
	}
	if count >= maxEventsPerUser {
		return nil, &LimitErr{Message: fmt.Sprintf("reached the %d-event limit", maxEventsPerUser)}
	}

	calendarType := in.CalendarType
	if calendarType == "" {
		calendarType = core.CalendarTypeGregorian
	}

	event := &models.Event{
		EventType:    core.EventTypeBirthday,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		Category:     core.CategoryOther,
		CalendarType: calendarType,
		Year:         in.Year,
		Month:        in.Month,
		Day:          in.Day,
		Gender:       in.Gender,
		IsActive:     true,
	}
	if err := recomputeOccurrence(event, user); err != nil {
		return nil, err
	}
	return events.Create(ctx, event)
}

// GetOwnedEvent fetches event_id, scoped to user, treating a soft-deleted
// row the same as "not found" (matches Python's deleted_at check).
func GetOwnedEvent(ctx context.Context, db repo.DBTX, user *models.User, eventID int64) (*models.Event, error) {
	events := repo.NewEventRepo(db, user.ID)
	event, err := events.GetOwned(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if event == nil || event.DeletedAt != nil {
		return nil, &NotFoundErr{Message: fmt.Sprintf("event %d not found for user %d", eventID, user.ID)}
	}
	return event, nil
}

// UpdateEvent persists event (already mutated by the caller — Go has no
// equivalent to Python's dynamic **fields/setattr, so callers set the one or
// two struct fields they're changing directly) and recomputes next_occurrence
// unconditionally. That's safe even when no date field changed: recomputing
// from the same month/day/year/calendar_type/policies/today is deterministic
// and yields the identical value, so there's no need to track "did a date
// field change" the way update_event_field does.
func UpdateEvent(ctx context.Context, db repo.DBTX, user *models.User, event *models.Event) (*models.Event, error) {
	if err := recomputeOccurrence(event, user); err != nil {
		return nil, err
	}
	events := repo.NewEventRepo(db, user.ID)
	return events.Update(ctx, event)
}

// DeleteEvent soft-deletes event_id (30-day trash, SPEC.md chapter 20).
func DeleteEvent(ctx context.Context, db repo.DBTX, user *models.User, eventID int64) (*models.Event, error) {
	event, err := GetOwnedEvent(ctx, db, user, eventID)
	if err != nil {
		return nil, err
	}
	events := repo.NewEventRepo(db, user.ID)
	if err := events.SoftDelete(ctx, eventID); err != nil {
		return nil, err
	}
	return event, nil
}

// RestoreEvent undoes a soft delete. Unlike GetOwnedEvent, it does NOT
// reject an already-deleted row (that's the whole point).
func RestoreEvent(ctx context.Context, db repo.DBTX, user *models.User, eventID int64) (*models.Event, error) {
	events := repo.NewEventRepo(db, user.ID)
	event, err := events.GetOwned(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, &NotFoundErr{Message: fmt.Sprintf("event %d not found for user %d", eventID, user.ID)}
	}
	if err := events.Restore(ctx, eventID); err != nil {
		return nil, err
	}
	event.DeletedAt = nil
	return event, nil
}

// ToggleMute flips event_id's is_active flag.
func ToggleMute(ctx context.Context, db repo.DBTX, user *models.User, eventID int64) (*models.Event, error) {
	event, err := GetOwnedEvent(ctx, db, user, eventID)
	if err != nil {
		return nil, err
	}
	event.IsActive = !event.IsActive
	events := repo.NewEventRepo(db, user.ID)
	return events.Update(ctx, event)
}

func recomputeOccurrence(event *models.Event, user *models.User) error {
	occ, err := core.NextOccurrence(
		event.CalendarType, event.Month, event.Day, UserToday(user),
		user.AdarPolicy, user.Feb29Policy,
	)
	if err != nil {
		return err
	}
	event.NextOccurrence = &occ
	return nil
}
