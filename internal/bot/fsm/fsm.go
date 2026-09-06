// Package fsm implements the in-memory per-user state machine with TTL
// eviction — port of app/states/forms.py (the state names) and
// app/utils/fsm_storage.py's IdleEvictingMemoryStorage.
//
// Unlike aiogram's MemoryStorage (a defaultdict that permanently allocates a
// record for every user who ever triggers a state read, even a pure filter
// check on an update from a user who never entered a flow), Store never
// creates a record on read and evicts records untouched for maxAge — the
// bounded-memory behavior is the default here, not a workaround grafted on
// top of a leaky base class.
//
// FSM state never touches the database — it's purely in-process and lost on
// restart, exactly like the Python bot's MemoryStorage — so state string
// values only need to be internally consistent, not byte-identical to
// aiogram's "GroupName:state_name" wire format.
package fsm

import (
	"sync"
	"time"
)

// State identifies one step of a multi-step flow.
type State string

// Onboarding flow (app/states/forms.py's Onboarding group).
const (
	OnboardingLanguage         State = "onboarding:language"
	OnboardingTimezone         State = "onboarding:timezone"
	OnboardingTimezoneCustom   State = "onboarding:timezone_custom"
	OnboardingNotifyTime       State = "onboarding:notify_time"
	OnboardingNotifyTimeCustom State = "onboarding:notify_time_custom"
)

// AddEvent flow.
const (
	AddEventName     State = "add_event:name"
	AddEventDate     State = "add_event:date"
	AddEventHebMonth State = "add_event:heb_month"
	AddEventHebDay   State = "add_event:heb_day"
	AddEventHebYear  State = "add_event:heb_year"
	// AddEventSecondaryPrompt/Date offer a hebrew-primary event a second,
	// gregorian-calendar recurring date (SPEC "dual hebrew/gregorian dates"
	// feature). Never entered from the gregorian-primary branch.
	AddEventSecondaryPrompt State = "add_event:secondary_prompt"
	AddEventSecondaryDate   State = "add_event:secondary_date"
)

// EditEvent flow.
const (
	EditEventChoosingField State = "edit_event:choosing_field"
	EditEventEnteringValue State = "edit_event:entering_value"
	EditEventHebMonth      State = "edit_event:heb_month"
	EditEventHebDay        State = "edit_event:heb_day"
	EditEventHebYear       State = "edit_event:heb_year"
)

// Search flow.
const (
	SearchQuery State = "search:query"
)

// SettingsFlow.
const (
	SettingsFlowCustomTime     State = "settings_flow:custom_time"
	SettingsFlowCustomTimezone State = "settings_flow:custom_timezone"
	SettingsFlowWipeConfirm    State = "settings_flow:wipe_confirm"
)

// TemplateFlow.
const (
	TemplateFlowBody State = "template_flow:body"
	TemplateFlowTone State = "template_flow:tone"
)

// ImportFlow.
const (
	ImportFlowWaitingFile State = "import_flow:waiting_file"
	ImportFlowConfirm     State = "import_flow:confirm"
)

// AdminFlow.
const (
	AdminFlowBroadcastText    State = "admin_flow:broadcast_text"
	AdminFlowBroadcastConfirm State = "admin_flow:broadcast_confirm"
)

type record struct {
	state     State
	data      map[string]any
	lastWrite time.Time
}

// Store is a concurrency-safe, TTL-evicting FSM store keyed by Telegram user id.
type Store struct {
	mu          sync.Mutex
	records     map[int64]*record
	maxAge      time.Duration
	sweepEvery  int
	writesSweep int
}

// NewStore creates a Store that evicts records untouched for maxAge, checked
// every sweepEvery writes (matching IdleEvictingMemoryStorage's amortized
// sweep so eviction never costs a full-map scan on every single write).
func NewStore(maxAge time.Duration, sweepEvery int) *Store {
	if sweepEvery <= 0 {
		sweepEvery = 200
	}
	return &Store{records: make(map[int64]*record), maxAge: maxAge, sweepEvery: sweepEvery}
}

func (s *Store) touch(userID int64) *record {
	r, ok := s.records[userID]
	if !ok {
		r = &record{data: make(map[string]any)}
		s.records[userID] = r
	}
	r.lastWrite = time.Now()
	s.writesSweep++
	if s.writesSweep >= s.sweepEvery {
		s.writesSweep = 0
		s.evictStaleLocked()
	}
	return r
}

func (s *Store) evictStaleLocked() {
	cutoff := time.Now().Add(-s.maxAge)
	for key, r := range s.records {
		if r.lastWrite.Before(cutoff) {
			delete(s.records, key)
		}
	}
}

// SetState sets the current state for userID.
func (s *Store) SetState(userID int64, state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch(userID).state = state
}

// GetState returns the current state for userID, or ("", false) if none —
// a pure lookup that never allocates a record (matching
// IdleEvictingMemoryStorage.get_state).
func (s *Store) GetState(userID int64) (State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[userID]
	if !ok {
		return "", false
	}
	return r.state, r.state != ""
}

// ClearState removes userID's state (but keeps its data, matching aiogram's
// FSMContext.clear semantics used by callers that call SetData separately).
func (s *Store) ClearState(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.records[userID]; ok {
		r.state = ""
	}
}

// SetData replaces userID's flow data.
func (s *Store) SetData(userID int64, data map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[string]any, len(data))
	for k, v := range data {
		cp[k] = v
	}
	s.touch(userID).data = cp
}

// UpdateData merges updates into userID's existing flow data (the common
// case: set one or two keys without clobbering the rest of the flow's
// accumulated data).
func (s *Store) UpdateData(userID int64, updates map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.touch(userID)
	if r.data == nil {
		r.data = make(map[string]any, len(updates))
	}
	for k, v := range updates {
		r.data[k] = v
	}
}

// GetData returns a copy of userID's flow data, or an empty map if none —
// a pure lookup that never allocates a record.
func (s *Store) GetData(userID int64) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[userID]
	if !ok || r.data == nil {
		return map[string]any{}
	}
	cp := make(map[string]any, len(r.data))
	for k, v := range r.data {
		cp[k] = v
	}
	return cp
}

// Clear removes all state and data for userID (end of flow / cancel).
func (s *Store) Clear(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, userID)
}

// Len reports the number of currently-tracked users (test/metrics use).
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}
