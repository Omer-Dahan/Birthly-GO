package core

import (
	"encoding/csv"
	"os"
	"strconv"
	"testing"
	"time"
)

// goldenRefNow matches the wall-clock date the Python exporter ran under
// (see the session context: today was 2026-07-29) — the two-digit-year
// branch of ParseGregorian depends on "today" at call time, so table tests
// pin it to the same instant the CSV was generated under, exactly like
// production code passing time.Now() would if run on that same date.
var goldenRefNow = time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)

func readCSV(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open("../../testdata/" + name)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	if len(rows) < 1 {
		t.Fatalf("%s: no rows", name)
	}
	return rows[1:] // skip header
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("atoi(%q): %v", s, err)
	}
	return v
}

func parseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parseDate(%q): %v", s, err)
	}
	return d
}

func TestGolden_Gematria(t *testing.T) {
	for _, row := range readCSV(t, "gematria.csv") {
		num, want := atoi(t, row[0]), row[1]
		if got := Gematria(num); got != want {
			t.Errorf("Gematria(%d) = %q, want %q", num, got, want)
		}
	}
}

func TestGolden_HebrewYearGematria(t *testing.T) {
	for _, row := range readCSV(t, "hebrew_year_gematria.csv") {
		y, want := atoi(t, row[0]), row[1]
		if got := HebrewYearGematria(y); got != want {
			t.Errorf("HebrewYearGematria(%d) = %q, want %q", y, got, want)
		}
	}
}

func TestGolden_LeapYears(t *testing.T) {
	for _, row := range readCSV(t, "hebrew_leap_years.csv") {
		y := atoi(t, row[0])
		want := row[1] == "True"
		if got := IsLeapYear(y); got != want {
			t.Errorf("IsLeapYear(%d) = %v, want %v", y, got, want)
		}
	}
}

func TestGolden_MonthLengths(t *testing.T) {
	for _, row := range readCSV(t, "hebrew_month_lengths.csv") {
		y, m, want := atoi(t, row[0]), atoi(t, row[1]), atoi(t, row[2])
		got, err := MonthLength(y, m)
		if err != nil {
			t.Errorf("MonthLength(%d,%d) error: %v", y, m, err)
			continue
		}
		if got != want {
			t.Errorf("MonthLength(%d,%d) = %d, want %d", y, m, got, want)
		}
	}
}

func TestGolden_ResolveMonth(t *testing.T) {
	for _, row := range readCSV(t, "hebrew_resolve_month.csv") {
		y, src, policy, want := atoi(t, row[0]), atoi(t, row[1]), row[2], row[3]
		if got := ResolveMonth(y, src, policy); strconv.Itoa(got) != want {
			t.Errorf("ResolveMonth(%d,%d,%q) = %d, want %s", y, src, policy, got, want)
		}
	}
}

func TestGolden_FormatHebrewDate(t *testing.T) {
	for _, row := range readCSV(t, "format_hebrew_date.csv") {
		y, m, d := atoi(t, row[0]), atoi(t, row[1]), atoi(t, row[2])
		wantWithYear, wantWithoutYear := row[3], row[4]
		if got := FormatHebrewDate(y, m, d, true); got != wantWithYear {
			t.Errorf("FormatHebrewDate(%d,%d,%d,true) = %q, want %q", y, m, d, got, wantWithYear)
		}
		if got := FormatHebrewDate(y, m, d, false); got != wantWithoutYear {
			t.Errorf("FormatHebrewDate(%d,%d,%d,false) = %q, want %q", y, m, d, got, wantWithoutYear)
		}
	}
}

func TestGolden_RoundTripGregorianToHebrew(t *testing.T) {
	for _, row := range readCSV(t, "round_trip_gregorian_to_hebrew.csv") {
		g := parseDate(t, row[0])
		wantY, wantM, wantD := atoi(t, row[1]), atoi(t, row[2]), atoi(t, row[3])
		gotY, gotM, gotD := ToHebrew(g)
		if gotY != wantY || gotM != wantM || gotD != wantD {
			t.Errorf("ToHebrew(%s) = (%d,%d,%d), want (%d,%d,%d)", row[0], gotY, gotM, gotD, wantY, wantM, wantD)
		}
		// and back
		back, err := ToGregorian(gotY, gotM, gotD)
		if err != nil {
			t.Errorf("ToGregorian(%d,%d,%d) error: %v", gotY, gotM, gotD, err)
			continue
		}
		if !back.Equal(g) {
			t.Errorf("round trip %s -> (%d,%d,%d) -> %s, want %s", row[0], gotY, gotM, gotD, back.Format("2006-01-02"), row[0])
		}
	}
}

func TestGolden_NextOccurrenceGregorian(t *testing.T) {
	for _, row := range readCSV(t, "next_occurrence_gregorian.csv") {
		today := parseDate(t, row[0])
		month, day, feb29Policy, want := atoi(t, row[1]), atoi(t, row[2]), row[3], row[4]
		got, err := NextOccurrence(CalendarTypeGregorian, month, day, today, AdarPolicyAdarII, feb29Policy)
		if err != nil {
			t.Errorf("NextOccurrence(gregorian,%d,%d,%s,%s) error: %v", month, day, row[0], feb29Policy, err)
			continue
		}
		if got.Format("2006-01-02") != want {
			t.Errorf("NextOccurrence(gregorian,%d,%d,today=%s,%s) = %s, want %s", month, day, row[0], feb29Policy, got.Format("2006-01-02"), want)
		}
	}
}

func TestGolden_NextOccurrenceHebrew(t *testing.T) {
	for _, row := range readCSV(t, "next_occurrence_hebrew.csv") {
		today := parseDate(t, row[0])
		month, day, adarPolicy, want := atoi(t, row[1]), atoi(t, row[2]), row[3], row[4]
		got, err := NextOccurrence(CalendarTypeHebrew, month, day, today, adarPolicy, Feb29PolicyFeb28)
		if err != nil {
			t.Errorf("NextOccurrence(hebrew,%d,%d,%s,%s) error: %v", month, day, row[0], adarPolicy, err)
			continue
		}
		if got.Format("2006-01-02") != want {
			t.Errorf("NextOccurrence(hebrew,%d,%d,today=%s,%s) = %s, want %s", month, day, row[0], adarPolicy, got.Format("2006-01-02"), want)
		}
	}
}

func TestGolden_AgeAt(t *testing.T) {
	for _, row := range readCSV(t, "age_at.csv") {
		calendarType := row[0]
		var year *int
		if row[1] != "" {
			y := atoi(t, row[1])
			year = &y
		}
		on := parseDate(t, row[4])
		wantStr := row[5]

		got, ok := AgeAt(calendarType, year, on)
		if wantStr == "" {
			if ok {
				t.Errorf("AgeAt(%s,%v,%s) = %d, want None", calendarType, row[1], row[4], got)
			}
			continue
		}
		want := atoi(t, wantStr)
		if !ok || got != want {
			t.Errorf("AgeAt(%s,%v,%s) = (%d,%v), want %d", calendarType, row[1], row[4], got, ok, want)
		}
	}
}

func TestGolden_ParseGregorian(t *testing.T) {
	for _, row := range readCSV(t, "validators_parse_gregorian.csv") {
		input, ok := row[0], row[1] == "True"
		got, err := ParseGregorian(input, goldenRefNow)
		if !ok {
			if err == nil {
				t.Errorf("ParseGregorian(%q) = %+v, want error", input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseGregorian(%q) error: %v, want ok", input, err)
			continue
		}
		wantDay, wantMonth := atoi(t, row[2]), atoi(t, row[3])
		if got.Day != wantDay || got.Month != wantMonth {
			t.Errorf("ParseGregorian(%q) = day=%d month=%d, want day=%d month=%d", input, got.Day, got.Month, wantDay, wantMonth)
		}
		if row[4] == "" {
			if got.Year != nil {
				t.Errorf("ParseGregorian(%q).Year = %d, want nil", input, *got.Year)
			}
		} else {
			wantYear := atoi(t, row[4])
			if got.Year == nil || *got.Year != wantYear {
				t.Errorf("ParseGregorian(%q).Year = %v, want %d", input, got.Year, wantYear)
			}
		}
	}
}

func TestGolden_ValidateName(t *testing.T) {
	for _, row := range readCSV(t, "validators_name.csv") {
		input, ok, want := row[0], row[1] == "True", row[2]
		got, err := ValidateName(input)
		if ok && (err != nil || got != want) {
			t.Errorf("ValidateName(%q) = (%q,%v), want (%q,nil)", input, got, err, want)
		}
		if !ok && err == nil {
			t.Errorf("ValidateName(%q) = %q, want error", input, got)
		}
	}
}

func testOptionalValidator(t *testing.T, csvName string, fn func(string) (string, error)) {
	t.Helper()
	for _, row := range readCSV(t, csvName) {
		input, ok, want := row[0], row[1] == "True", row[2]
		got, err := fn(input)
		if ok && (err != nil || got != want) {
			t.Errorf("%s(%q) = (%q,%v), want (%q,nil)", csvName, input, got, err, want)
		}
		if !ok && err == nil {
			t.Errorf("%s(%q) = %q, want error", csvName, input, got)
		}
	}
}

func TestGolden_ValidateNickname(t *testing.T) {
	testOptionalValidator(t, "validators_nickname.csv", ValidateNickname)
}
func TestGolden_ValidateNotes(t *testing.T) {
	testOptionalValidator(t, "validators_notes.csv", ValidateNotes)
}
func TestGolden_ValidateRelation(t *testing.T) {
	testOptionalValidator(t, "validators_relation.csv", ValidateRelation)
}
func TestGolden_ValidatePhone(t *testing.T) {
	testOptionalValidator(t, "validators_phone.csv", ValidatePhone)
}

func TestGolden_ValidateYear(t *testing.T) {
	for _, row := range readCSV(t, "validators_year.csv") {
		year, hebrew, ok := atoi(t, row[0]), row[1] == "True", row[2] == "True"
		got, err := ValidateYear(year, hebrew)
		if ok {
			want := atoi(t, row[3])
			if err != nil || got != want {
				t.Errorf("ValidateYear(%d,%v) = (%d,%v), want (%d,nil)", year, hebrew, got, err, want)
			}
		} else if err == nil {
			t.Errorf("ValidateYear(%d,%v) = %d, want error", year, hebrew, got)
		}
	}
}

func TestGolden_ParseHebrewYearInput(t *testing.T) {
	for _, row := range readCSV(t, "validators_hebrew_year_input.csv") {
		input, ok := row[0], row[1] == "True"
		got, err := ParseHebrewYearInput(input)
		if ok {
			want := atoi(t, row[2])
			if err != nil || got != want {
				t.Errorf("ParseHebrewYearInput(%q) = (%d,%v), want (%d,nil)", input, got, err, want)
			}
		} else if err == nil {
			t.Errorf("ParseHebrewYearInput(%q) = %d, want error", input, got)
		}
	}
}

func TestGolden_ContainsOnlySymbols(t *testing.T) {
	for _, row := range readCSV(t, "validators_contains_only_symbols.csv") {
		input, want := row[0], row[1] == "True"
		if got := ContainsOnlySymbols(input); got != want {
			t.Errorf("ContainsOnlySymbols(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestGolden_GenderDetection(t *testing.T) {
	mismatches := 0
	rows := readCSV(t, "gender_detection.csv")
	for _, row := range rows {
		name, want := row[0], row[1]
		got, ok := DetectGender(name)
		gotStr := ""
		if ok {
			gotStr = got
		}
		if gotStr != want {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("DetectGender(%q) = %q, want %q", name, gotStr, want)
			}
		}
	}
	if mismatches > 20 {
		t.Errorf("... and %d more mismatches (%d/%d total)", mismatches-20, mismatches, len(rows))
	}
}
