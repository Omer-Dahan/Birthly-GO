package core

import (
	"testing"
	"time"
)

// TestToGregorian_MatchesKnownHebrewDate pins one hand-checked conversion so
// a regression in the underlying hdate library or ResolveMonth wiring shows
// up as a test failure: 20 Av 5781 (month 5 = Av in the SPEC.md 1=Nisan..
// 13=Adar II numbering) is the exact civil date hebcal resolves it to.
func TestToGregorian_MatchesKnownHebrewDate(t *testing.T) {
	got, err := ToGregorian(5781, 5, 20)
	if err != nil {
		t.Fatalf("ToGregorian: %v", err)
	}
	want := time.Date(2021, time.July, 29, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ToGregorian(5781, Av, 20) = %v, want %v", got, want)
	}
}

// TestToGregorian_RoundTripsThroughToHebrew guards the pairing this feature
// relies on: converting a hebrew date to gregorian and back must recover the
// same hebrew date, for both a leap and a non-leap year.
func TestToGregorian_RoundTripsThroughToHebrew(t *testing.T) {
	cases := []struct {
		year, month, day int
	}{
		{5781, 5, 20},  // 5781 is not a leap year
		{5782, 12, 15}, // 5782 is a leap year (has Adar I/Adar II)
	}
	for _, c := range cases {
		greg, err := ToGregorian(c.year, c.month, c.day)
		if err != nil {
			t.Fatalf("ToGregorian(%d, %d, %d): %v", c.year, c.month, c.day, err)
		}
		gotYear, gotMonth, gotDay := ToHebrew(greg)
		if gotYear != c.year || gotMonth != c.month || gotDay != c.day {
			t.Errorf("round trip of hebrew (%d,%d,%d) via %v = (%d,%d,%d)", c.year, c.month, c.day, greg, gotYear, gotMonth, gotDay)
		}
	}
}
