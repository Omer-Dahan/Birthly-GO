package core

import "time"

func mustDate(year int, month time.Month, day int) (time.Time, bool) {
	if day < 1 {
		return time.Time{}, false
	}
	d := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	// time.Date normalizes out-of-range days/months instead of erroring;
	// detect that by checking the result reports back the same components,
	// mirroring Python's date(...) raising ValueError on an invalid date.
	if d.Year() != year || d.Month() != month || d.Day() != day {
		return time.Time{}, false
	}
	return d, true
}

func nextGregorianOccurrence(month time.Month, day int, today time.Time, feb29Policy string) time.Time {
	resolvedMonth, resolvedDay := month, day
	if month == time.February && day == 29 {
		if _, ok := mustDate(today.Year(), time.February, 29); !ok {
			if feb29Policy == Feb29PolicyFeb28 {
				resolvedMonth, resolvedDay = time.February, 28
			} else {
				resolvedMonth, resolvedDay = time.March, 1
			}
		}
	}

	candidate, ok := mustDate(today.Year(), resolvedMonth, resolvedDay)
	if !ok {
		candidate, _ = mustDate(today.Year(), resolvedMonth, resolvedDay-1)
	}

	if !candidate.Before(today) {
		return candidate
	}

	nextYear := today.Year() + 1
	targetMonth, targetDay := month, day
	if month == time.February && day == 29 {
		if _, ok := mustDate(nextYear, time.February, 29); !ok {
			if feb29Policy == Feb29PolicyFeb28 {
				targetMonth, targetDay = time.February, 28
			} else {
				targetMonth, targetDay = time.March, 1
			}
		}
	}

	if result, ok := mustDate(nextYear, targetMonth, targetDay); ok {
		return result
	}
	result, _ := mustDate(nextYear, targetMonth, targetDay-1)
	return result
}

func nextHebrewOccurrence(hMonth, hDay int, today time.Time, adarPolicy string) (time.Time, error) {
	hYearToday, _, _ := ToHebrew(today)

	for _, hYear := range [2]int{hYearToday, hYearToday + 1} {
		resolvedMonth := ResolveMonth(hYear, hMonth, adarPolicy)
		candidate, err := ToGregorian(hYear, resolvedMonth, hDay)
		if err != nil {
			return time.Time{}, err
		}
		if !candidate.Before(today) {
			return candidate, nil
		}
	}

	panic("unreachable: next Hebrew occurrence not found within one year")
}

// NextOccurrence returns the next Gregorian date this recurring event falls
// on, on or after today. month/day use the Gregorian calendar's own numbering
// (time.Month 1-12) for calendarType "gregorian", or the SPEC.md Hebrew
// numbering (1-13) for "hebrew".
func NextOccurrence(calendarType string, month, day int, today time.Time, adarPolicy, feb29Policy string) (time.Time, error) {
	if calendarType == CalendarTypeHebrew {
		return nextHebrewOccurrence(month, day, today, adarPolicy)
	}
	return nextGregorianOccurrence(time.Month(month), day, today, feb29Policy), nil
}

// DaysUntil returns the number of days from today to target (0 if today,
// negative if past). Both arguments are expected to be UTC midnight (pure
// dates, no time-of-day component), matching Python's (date - date).days.
func DaysUntil(target, today time.Time) int {
	return int(target.Sub(today) / (24 * time.Hour))
}

// AgeAt returns the age reached on occurrence date on. Returns (0, false)
// if the birth year is unknown (matching Python's age_at returning None).
func AgeAt(calendarType string, year *int, on time.Time) (int, bool) {
	if year == nil {
		return 0, false
	}
	if calendarType == CalendarTypeHebrew {
		occurrenceHYear, _, _ := ToHebrew(on)
		return occurrenceHYear - *year, true
	}
	return on.Year() - *year, true
}
