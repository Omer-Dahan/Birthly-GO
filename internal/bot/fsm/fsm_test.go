package fsm

import (
	"testing"
	"time"
)

func TestGetState_PureReadNeverAllocates(t *testing.T) {
	s := NewStore(time.Hour, 200)
	if _, ok := s.GetState(123); ok {
		t.Error("GetState on unknown user should be (_, false)")
	}
	if s.Len() != 0 {
		t.Errorf("Len() = %d after pure read, want 0 (no record should be allocated)", s.Len())
	}
	if data := s.GetData(123); len(data) != 0 {
		t.Errorf("GetData on unknown user = %v, want empty", data)
	}
	if s.Len() != 0 {
		t.Errorf("Len() = %d after pure GetData, want 0", s.Len())
	}
}

func TestSetGetState(t *testing.T) {
	s := NewStore(time.Hour, 200)
	s.SetState(1, AddEventName)
	got, ok := s.GetState(1)
	if !ok || got != AddEventName {
		t.Errorf("GetState(1) = (%v,%v), want (%v,true)", got, ok, AddEventName)
	}

	s.UpdateData(1, map[string]any{"first_name": "Dana"})
	s.UpdateData(1, map[string]any{"month": 3})
	data := s.GetData(1)
	if data["first_name"] != "Dana" || data["month"] != 3 {
		t.Errorf("GetData(1) = %v, want first_name=Dana month=3", data)
	}

	s.Clear(1)
	if _, ok := s.GetState(1); ok {
		t.Error("GetState after Clear should be false")
	}
}

func TestTTLEviction(t *testing.T) {
	s := NewStore(10*time.Millisecond, 1) // sweep on every write
	s.SetState(1, AddEventName)
	if s.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", s.Len())
	}
	time.Sleep(20 * time.Millisecond)
	// Trigger a sweep via another user's write (sweepEvery=1).
	s.SetState(2, AddEventName)
	if _, ok := s.GetState(1); ok {
		t.Error("user 1's state should have been evicted after maxAge elapsed")
	}
}

func TestDataCopyIsolation(t *testing.T) {
	s := NewStore(time.Hour, 200)
	s.SetData(1, map[string]any{"x": 1})
	got := s.GetData(1)
	got["x"] = 999 // mutating the returned copy must not affect the store
	got2 := s.GetData(1)
	if got2["x"] != 1 {
		t.Errorf("GetData(1)[\"x\"] = %v after external mutation of a prior copy, want 1", got2["x"])
	}
}
