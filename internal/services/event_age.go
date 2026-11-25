package services

import (
	"time"

	"birthly/internal/core"
	"birthly/internal/store/models"
)

// civilBirthYear returns the Gregorian year a Hebrew-primary event was born
// in. The stored civil month/day decides when it exists: a birth after sunset
// is recorded on the civil day before the Hebrew date, so one day of slack is
// allowed (26 Tevet 5744 is 1 Jan 1984, but stored 31/12 it is 1983).
func civilBirthYear(event *models.Event) (int, bool) {
	if event.Year == nil {
		return 0, false
	}
	birth, err := core.ToGregorian(*event.Year, event.Month, event.Day)
	if err != nil {
		return 0, false
	}
	if event.SecondaryMonth != nil && event.SecondaryDay != nil {
		sm, sd := *event.SecondaryMonth, *event.SecondaryDay
		if int(birth.Month()) != sm || birth.Day() != sd {
			if prev := birth.AddDate(0, 0, -1); int(prev.Month()) == sm && prev.Day() == sd {
				birth = prev
			}
		}
	}
	return birth.Year(), true
}

// AgeForTrack is the age the given calendar track celebrates on the given
// date. The Gregorian track of a Hebrew-primary event counts civil years from
// the stored civil birth date; every other combination is core.AgeAt.
func AgeForTrack(event *models.Event, on time.Time, track string) (int, bool) {
	if event.CalendarType == core.CalendarTypeHebrew && track == core.CalendarTypeGregorian {
		if y, ok := civilBirthYear(event); ok {
			return on.Year() - y, true
		}
	}
	return core.AgeAt(event.CalendarType, event.Year, on)
}

// greetingAge is the age a greeting should state. A dual-date event is
// greeted for the track whose occurrence is nearest, the one the reminder
// that opened the greeting fired for.
func greetingAge(event *models.Event, today time.Time) (int, bool) {
	if event.CalendarType == core.CalendarTypeHebrew && event.SecondaryNextOccurrence != nil && event.NextOccurrence != nil {
		primary := absDays(event.NextOccurrence.Sub(today))
		secondary := absDays(event.SecondaryNextOccurrence.Sub(today))
		if secondary < primary {
			return AgeForTrack(event, *event.SecondaryNextOccurrence, core.CalendarTypeGregorian)
		}
	}
	return core.AgeAt(event.CalendarType, event.Year, today)
}

func absDays(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
