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

const tevet = 10

func TestParseHebrewYearInputFor_DecemberTevet(t *testing.T) {
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	// 20 Tevet fell on 2 Jan 1983 (5743) and on 26 Dec 1983 (5744); a bare
	// "1983" matches both and must not silently pick the January one.
	if _, err := ParseHebrewYearInputFor("1983", tevet, 20, AdarPolicyAdarII); err == nil {
		t.Fatal("want ambiguity error for 20 Tevet 1983 without a stored date")
	} else if _, ok := err.(*AmbiguousHebrewYearError); !ok {
		t.Fatalf("got %T, want *AmbiguousHebrewYearError", err)
	}
	dm, dd := 12, 26
	got, err := ParseHebrewYearInputAnchored("1983", tevet, 20, AdarPolicyAdarII, &dm, &dd)
	if err != nil || got != 5744 {
		t.Fatalf("december anchor: got (%d,%v), want 5744", got, err)
	}
	if age, _ := AgeAt(CalendarTypeHebrew, &got, now); age != 43 {
		t.Errorf("age %d, want 43", age)
	}
	jm, jd := 1, 5
	got, err = ParseHebrewYearInputAnchored("1983", tevet, 20, AdarPolicyAdarII, &jm, &jd)
	if err != nil || got != 5743 {
		t.Fatalf("january anchor: got (%d,%v), want 5743 (January year)", got, err)
	}
}

func TestParseHebrewYearInputAnchored_Sweep(t *testing.T) {
	for greg := 1900; greg <= 2099; greg++ {
		for hm := 1; hm <= 13; hm++ {
			for hd := 1; hd <= 30; hd++ {
				raw := itoa(greg)
				var cands []int
				var dates []time.Time
				for _, h := range [2]int{greg + 3760, greg + 3761} {
					d, err := ToGregorian(h, ResolveMonth(h, hm, AdarPolicyAdarII), hd)
					if err == nil && d.Year() == greg {
						cands = append(cands, h)
						dates = append(dates, d)
					}
				}
				got, err := ParseHebrewYearInputFor(raw, hm, hd, AdarPolicyAdarII)
				switch len(cands) {
				case 1:
					if err != nil || got != cands[0] {
						t.Fatalf("%d m%d d%d: got (%d,%v), want %d", greg, hm, hd, got, err, cands[0])
					}
				case 2:
					if _, ok := err.(*AmbiguousHebrewYearError); !ok {
						t.Fatalf("%d m%d d%d: both match but got (%d,%v)", greg, hm, hd, got, err)
					}
					for i, d := range dates {
						sm, sd := int(d.Month()), d.Day()
						g2, err := ParseHebrewYearInputAnchored(raw, hm, hd, AdarPolicyAdarII, &sm, &sd)
						if err != nil || g2 != cands[i] {
							t.Fatalf("%d m%d d%d anchored %s: got (%d,%v), want %d", greg, hm, hd, d.Format("01-02"), g2, err, cands[i])
						}
					}
				}
			}
		}
	}
}
