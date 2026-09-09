package core

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDate(t *testing.T) {
	d := time.Date(2026, time.March, 5, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		format string
		want   string
	}{
		{DateFormatDMYSlash, "05/03/2026"},
		{DateFormatDMYDot, "05.03.2026"},
		{DateFormatISO, "2026-03-05"},
		{"unknown-format", "05/03/2026"}, // falls back to DD/MM/YYYY
	}
	for _, tc := range tests {
		if got := FormatDate(d, tc.format); got != tc.want {
			t.Errorf("FormatDate(%s) = %q, want %q", tc.format, got, tc.want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		hour, minute int
		format       string
		want         string
	}{
		{14, 5, TimeFormat24h, "14:05"},
		{0, 0, TimeFormat24h, "00:00"},
		{9, 30, TimeFormat12h, "9:30 AM"},
		{14, 30, TimeFormat12h, "2:30 PM"},
		{0, 0, TimeFormat12h, "12:00 AM"},  // midnight
		{12, 0, TimeFormat12h, "12:00 PM"}, // noon
		{23, 59, TimeFormat12h, "11:59 PM"},
	}
	for _, tc := range tests {
		if got := FormatTime(tc.hour, tc.minute, tc.format); got != tc.want {
			t.Errorf("FormatTime(%d,%d,%s) = %q, want %q", tc.hour, tc.minute, tc.format, got, tc.want)
		}
	}
}

func TestFormatCountdown(t *testing.T) {
	tests := []struct {
		days int
		want string
	}{
		{0, "היום"},
		{1, "מחר"},
		{2, "עוד יומיים"},
		{5, "עוד 5 ימים"},
		{7, "עוד שבוע"},
		{13, "עוד שבוע"},
		{14, "עוד " + PluralizeHebrew(2, "week")}, // 14 days = 2 weeks (dual form)
		{60, "עוד חודשיים"},
		{90, "עוד " + PluralizeHebrew(3, "month")},
	}
	for _, tc := range tests {
		if got := FormatCountdown(tc.days); got != tc.want {
			t.Errorf("FormatCountdown(%d) = %q, want %q", tc.days, got, tc.want)
		}
	}
}

func TestFormatCountdown_NeverEmpty(t *testing.T) {
	for days := 0; days <= 400; days++ {
		if got := FormatCountdown(days); got == "" {
			t.Errorf("FormatCountdown(%d) returned empty string", days)
		}
	}
}

func TestFormatPhone(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"+972501234567", "050-1234567"}, // E.164
		{"0501234567", "050-1234567"},    // local, leading zero
		{"501234567", "050-1234567"},     // no leading zero
		{"021234567", "02-1234567"},      // landline, already has leading zero (9 digits)
		{"21234567", "02-1234567"},       // landline, no leading zero (8 digits)
	}
	for _, tc := range tests {
		if got := FormatPhone(tc.in); got != tc.want {
			t.Errorf("FormatPhone(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatPhone_NonStandardLengthReturnsDigitsOnly(t *testing.T) {
	// Not 9 or 10 digits after normalization -> returned as plain digits,
	// no hyphen inserted.
	got := FormatPhone("123")
	if got != "0123" {
		t.Errorf("FormatPhone(123) = %q, want %q (leading 0 added, too short for hyphenation)", got, "0123")
	}
}

func TestFormatName(t *testing.T) {
	lastName := "Cohen"
	empty := ""
	if got := FormatName("Dana", &lastName); got != "Dana Cohen" {
		t.Errorf("FormatName with last name = %q, want %q", got, "Dana Cohen")
	}
	if got := FormatName("Dana", nil); got != "Dana" {
		t.Errorf("FormatName with nil last name = %q, want %q", got, "Dana")
	}
	if got := FormatName("Dana", &empty); got != "Dana" {
		t.Errorf("FormatName with empty last name = %q, want %q", got, "Dana")
	}
}

func TestFormatName_TruncatesOverlongNames(t *testing.T) {
	// Guards the card layout against names that predate the current
	// NameMaxLen or arrived via backup restore, which bypasses ValidateName.
	longLast := strings.Repeat("א", DisplayNameMaxLen)
	got := FormatName("Dana", &longLast)
	gotRunes := []rune(got)
	if len(gotRunes) != DisplayNameMaxLen+1 { // +1 for the trailing ellipsis
		t.Fatalf("FormatName length = %d, want %d", len(gotRunes), DisplayNameMaxLen+1)
	}
	if gotRunes[len(gotRunes)-1] != '…' {
		t.Errorf("FormatName(long) = %q, want it to end with an ellipsis", got)
	}
}

func TestRTL(t *testing.T) {
	got := RTL("hello")
	if !strings.HasPrefix(got, rlm) || !strings.HasSuffix(got, "hello") {
		t.Errorf("RTL(hello) = %q, want RLM-prefixed", got)
	}
}

func TestLTR(t *testing.T) {
	got := LTR("050-1234567")
	if !strings.HasPrefix(got, lrm) || !strings.HasSuffix(got, lrm) {
		t.Errorf("LTR(...) = %q, want LRM on both sides", got)
	}
	if !strings.Contains(got, "050-1234567") {
		t.Errorf("LTR(...) = %q, want it to contain the original text", got)
	}
}
