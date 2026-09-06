// Package models contains the database row structs, one per table in
// migrations/00001_baseline.sql. Nullable columns are Go pointers; a nil
// pointer is SQL NULL.
package models

import "time"

type User struct {
	ID                   int64
	Username             *string
	FirstName            string
	LastName             *string
	Language             string
	Timezone             string
	DateFormat           string
	TimeFormat           string
	DefaultNotifyTime    string
	NotificationsEnabled bool
	SilentNotifications  bool
	ShowHebrewDate       bool
	DailyDigestEnabled   bool
	DigestTime           string
	ListSort             string
	ListFilter           *string
	AdarPolicy           string
	Feb29Policy          string
	IsAdmin              bool
	IsBlocked            bool
	BotBlockedByUser     bool
	Onboarded            bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	LastSeenAt           time.Time
}

type Event struct {
	ID               int64
	UserID           int64
	EventType        string
	CustomTypeLabel  *string
	FirstName        string
	LastName         *string
	Nickname         *string
	Gender           *string
	Relation         *string
	Category         string
	CalendarType     string
	Year             *int
	Month            int
	Day              int
	EventTime        *string
	Phone            *string
	TelegramUsername *string
	PhotoFileID      *string
	Notes            *string
	NextOccurrence   *time.Time
	// Secondary date: an optional second calendar track on the same event
	// (SPEC "dual hebrew/gregorian dates" feature). Only ever populated when
	// CalendarType is hebrew — the reverse direction (gregorian primary +
	// hebrew secondary) is not offered by the add flow. SecondaryCalendarType
	// is always "gregorian" when set.
	SecondaryCalendarType   *string
	SecondaryMonth          *int
	SecondaryDay            *int
	SecondaryNextOccurrence *time.Time
	IsActive                bool
	DeletedAt               *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type ReminderRule struct {
	ID            int64
	UserID        int64
	EventID       *int64
	OffsetDays    *int
	OffsetMinutes *int
	SendTime      *string
	Enabled       bool
	CreatedAt     time.Time
}

type NotificationLog struct {
	ID             int64
	UserID         int64
	EventID        int64
	RuleID         *int64
	OccurrenceDate time.Time
	ScheduledAt    time.Time
	SentAt         *time.Time
	Status         string
	Error          *string
	Attempts       int
}

type GreetingTemplate struct {
	ID        int64
	UserID    *int64
	EventType string
	Tone      string
	Gender    *string
	Language  string
	Body      string
	IsActive  bool
}

type AuditLog struct {
	ID        int64
	UserID    int64
	Action    string
	Entity    string
	EntityID  *int64
	Payload   *string
	CreatedAt time.Time
}

type Backup struct {
	ID          int64
	UserID      *int64
	Kind        string
	Format      string
	Path        string
	SizeBytes   int64
	EventsCount int
	CreatedAt   time.Time
}

type AppMeta struct {
	Key   string
	Value *string
}

// Notification statuses (matches the notif_status_valid CHECK constraint).
const (
	NotificationStatusPending = "pending"
	NotificationStatusSent    = "sent"
	NotificationStatusFailed  = "failed"
	NotificationStatusSkipped = "skipped"
)

// List views for EventRepo.ListPage (matches app/db/repositories/events.py).
const (
	ListViewUpcoming = "upcoming"
	ListViewAll      = "all"
	ListViewMuted    = "muted"
	ListViewTrash    = "trash"
)

// Sort orders for EventRepo.ListPage.
const (
	SortUpcoming = "upcoming"
	SortName     = "name"
	SortAge      = "age"
	SortCreated  = "created"
)
