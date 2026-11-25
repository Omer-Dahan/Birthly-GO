package core

import (
	"testing"
	"time"
)

func TestParseHebrewYearInputFor_RoshHashanaBoundary(t *testing.T) {
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	// Hebrew month numbering: derive the month/day from the Gregorian date.
	for _, c := range []struct {
		greg    time.Time
		wantAge int
	}{
		{time.Date(1983, 11, 30, 0, 0, 0, 0, time.UTC), 43},
		{time.Date(1983, 10, 1, 0, 0, 0, 0, time.UTC), 43},
		{time.Date(1983, 12, 5, 0, 0, 0, 0, time.UTC), 43},
		{time.Date(1984, 1, 5, 0, 0, 0, 0, time.UTC), 43},
		{time.Date(1984, 8, 20, 0, 0, 0, 0, time.UTC), 43},
	} {
		hy, hm, hd := ToHebrew(c.greg)
		got, err := ParseHebrewYearInputFor(itoa(c.greg.Year()), hm, hd, AdarPolicyAdarII)
		if err != nil || got != hy {
			t.Fatalf("%s: got (%d,%v), want %d", c.greg.Format("2006-01-02"), got, err, hy)
		}
		if age, _ := AgeAt(CalendarTypeHebrew, &got, now); age != c.wantAge {
			t.Errorf("%s: age %d, want %d", c.greg.Format("2006-01-02"), age, c.wantAge)
		}
		sec, _ := ToGregorian(got, ResolveMonth(got, hm, AdarPolicyAdarII), hd)
		if !sec.Equal(c.greg) {
			t.Errorf("%s: secondary %s", c.greg.Format("2006-01-02"), sec.Format("2006-01-02"))
		}
	}
}

func itoa(n int) string { return time.Date(n, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006") }
