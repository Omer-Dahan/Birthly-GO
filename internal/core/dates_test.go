package core

import (
	"testing"
	"time"
)

func TestDaysUntil(t *testing.T) {
	today := time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		target time.Time
		want   int
	}{
		{today, 0},
		{today.AddDate(0, 0, 1), 1},
		{today.AddDate(0, 0, -1), -1},
		{today.AddDate(0, 0, 30), 30},
	}
	for _, tc := range tests {
		if got := DaysUntil(tc.target, today); got != tc.want {
			t.Errorf("DaysUntil(%v, %v) = %d, want %d", tc.target, today, got, tc.want)
		}
	}
}
