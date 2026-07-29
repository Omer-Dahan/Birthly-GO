package core

import "testing"

func TestEsc(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"Tom & Jerry", "Tom &amp; Jerry"},
		{"", ""},
		{"דנה כהן", "דנה כהן"},           // plain Hebrew unchanged
		{`he said "hi"`, `he said "hi"`}, // quotes NOT escaped (body text, not attribute context)
	}
	for _, tc := range tests {
		if got := Esc(tc.in); got != tc.want {
			t.Errorf("Esc(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPluralizeHebrew(t *testing.T) {
	tests := []struct {
		count int
		unit  string
		want  string
	}{
		{1, "day", "1 יום"},
		{2, "day", "יומיים"},
		{5, "day", "5 ימים"},
		{1, "year", "1 שנה"},
		{2, "year", "שנתיים"},
		{3, "year", "3 שנים"},
		{2, "week", "שבועיים"},
		{2, "month", "חודשיים"},
	}
	for _, tc := range tests {
		if got := PluralizeHebrew(tc.count, tc.unit); got != tc.want {
			t.Errorf("PluralizeHebrew(%d,%s) = %q, want %q", tc.count, tc.unit, got, tc.want)
		}
	}
}

func TestPluralizeHebrew_UnknownUnitPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("PluralizeHebrew with an unknown unit should panic, but it did not")
		}
	}()
	PluralizeHebrew(1, "century")
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		value  string
		maxLen int
		suffix string
		want   string
	}{
		{"short", 10, "…", "short"},           // unchanged, under limit
		{"hello world", 8, "…", "hello w…"},   // truncated, suffix appended
		{"exactlyten", 10, "…", "exactlyten"}, // exact length, unchanged
		{"hello world", 1, "…", "…"},          // maxLen == len(suffix)
	}
	for _, tc := range tests {
		if got := Truncate(tc.value, tc.maxLen, tc.suffix); got != tc.want {
			t.Errorf("Truncate(%q,%d,%q) = %q, want %q", tc.value, tc.maxLen, tc.suffix, got, tc.want)
		}
	}
}

func TestSplitName(t *testing.T) {
	first, last := SplitName("Dana Cohen")
	if first != "Dana" || last == nil || *last != "Cohen" {
		t.Errorf("SplitName(Dana Cohen) = (%q, %v), want (Dana, Cohen)", first, last)
	}

	first, last = SplitName("Dana")
	if first != "Dana" || last != nil {
		t.Errorf("SplitName(Dana) = (%q, %v), want (Dana, nil)", first, last)
	}

	first, last = SplitName("Dana  Maria  Cohen")
	if first != "Dana" || last == nil || *last != "Maria  Cohen" {
		t.Errorf("SplitName(multi-space) = (%q, %v), want (Dana, \"Maria  Cohen\")", first, last)
	}

	// Trailing whitespace after the first name with nothing else yields no
	// last name at all — the Go implementation trims and re-checks for empty.
	first, last = SplitName("Dana   ")
	if first != "Dana" || last != nil {
		t.Errorf("SplitName(trailing whitespace) = (%q, %v), want (Dana, nil)", first, last)
	}
}
