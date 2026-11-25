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
	// SecondaryMonth/SecondaryDay add a second, gregorian-calendar recurring
	// date to the same event (SPEC "dual dates" feature). Only honored when
	// CalendarType is hebrew: ignored otherwise, since the add flow never
	// offers the reverse direction.
	SecondaryMonth *int
	SecondaryDay   *int
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
	if in.SecondaryMonth != nil && in.SecondaryDay != nil && calendarType == core.CalendarTypeHebrew {
		secondaryCalendarType := core.CalendarTypeGregorian
		event.SecondaryCalendarType = &secondaryCalendarType
		event.SecondaryMonth = in.SecondaryMonth
		event.SecondaryDay = in.SecondaryDay
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

// UpdateEventYear changes only the year of an event, and only if its year is
// still oldYear. Nothing else on the row is written, so a concurrent change to
// any other column (a secondary date, a phone, ...) survives. next_occurrence
// does not depend on the year, so it is not touched either. A changed year
// yields *ConflictErr, a missing event *NotFoundErr.
func UpdateEventYear(ctx context.Context, db repo.DBTX, user *models.User, eventID int64, oldYear, newYear *int) (*models.Event, error) {
	events := repo.NewEventRepo(db, user.ID)
	ok, err := events.UpdateYear(ctx, eventID, oldYear, newYear)
	if err != nil {
		return nil, err
	}
	event, err := events.GetOwned(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if event == nil || event.DeletedAt != nil {
		return nil, &NotFoundErr{Message: fmt.Sprintf("event %d not found for user %d", eventID, user.ID)}
	}
	if !ok {
		return nil, &ConflictErr{Message: fmt.Sprintf("event %d year changed concurrently", eventID)}
	}
	return event, nil
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

// UpdateEventColumns writes only the named non-date columns of an event (see
// repo.EventRepo.UpdateColumns) and returns the refreshed row. Year, dates and
// every column not named stay as stored, whatever snapshot the caller read.
func UpdateEventColumns(ctx context.Context, db repo.DBTX, user *models.User, eventID int64, set map[string]any) (*models.Event, error) {
	if len(set) == 0 {
		return GetOwnedEvent(ctx, db, user, eventID)
	}
	events := repo.NewEventRepo(db, user.ID)
	ok, err := events.UpdateColumns(ctx, eventID, set)
	if err != nil {
		return nil, err
	}
	return reloadAfterWrite(ctx, events, user, eventID, ok)
}

// ToggleMute flips event_id's is_active flag, writing only that column.
func ToggleMute(ctx context.Context, db repo.DBTX, user *models.User, eventID int64) (*models.Event, error) {
	events := repo.NewEventRepo(db, user.ID)
	ok, err := events.ToggleActive(ctx, eventID)
	if err != nil {
		return nil, err
	}
	return reloadAfterWrite(ctx, events, user, eventID, ok)
}

func reloadAfterWrite(ctx context.Context, events *repo.EventRepo, user *models.User, eventID int64, ok bool) (*models.Event, error) {
	event, err := events.GetOwned(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if !ok || event == nil || event.DeletedAt != nil {
		return nil, &NotFoundErr{Message: fmt.Sprintf("event %d not found for user %d", eventID, user.ID)}
	}
	return event, nil
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

	if event.SecondaryMonth == nil || event.SecondaryDay == nil {
		event.SecondaryNextOccurrence = nil
		return nil
	}
	secondaryCalendarType := core.CalendarTypeGregorian
	if event.SecondaryCalendarType != nil {
		secondaryCalendarType = *event.SecondaryCalendarType
	}
	secOcc, err := core.NextOccurrence(
		secondaryCalendarType, *event.SecondaryMonth, *event.SecondaryDay, UserToday(user),
		user.AdarPolicy, user.Feb29Policy,
	)
	if err != nil {
		return err
	}
	event.SecondaryNextOccurrence = &secOcc
	return nil
}
