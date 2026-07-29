// Package core holds date logic (Gregorian and Hebrew calendars), validators,
// text formatting, and gender detection — no third-party dependencies beyond
// github.com/hebcal/hdate for the Hebrew calendar math itself.
package core

// Event/domain enums (app/constants.py's StrEnum classes). Represented as
// plain strings, matching the CHECK constraint values in the DB schema —
// there is no behavior attached to these beyond the literal value.
const (
	EventTypeBirthday    = "birthday"
	EventTypeAnniversary = "anniversary"
	EventTypeWedding     = "wedding"
	EventTypeMemorial    = "memorial"
	EventTypeCustom      = "custom"

	CalendarTypeGregorian = "gregorian"
	CalendarTypeHebrew    = "hebrew"

	CategoryFamily  = "family"
	CategoryFriends = "friends"
	CategoryWork    = "work"
	CategoryClients = "clients"
	CategorySchool  = "school"
	CategoryOther   = "other"

	GenderMale   = "m"
	GenderFemale = "f"
	GenderOther  = "other"

	ToneWarm   = "warm"
	ToneFunny  = "funny"
	ToneFormal = "formal"
	ToneShort  = "short"

	NotificationStatusPending = "pending"
	NotificationStatusSent    = "sent"
	NotificationStatusFailed  = "failed"
	NotificationStatusSkipped = "skipped"

	LanguageHe = "he"
	LanguageEn = "en"

	DateFormatDMYSlash = "DD/MM/YYYY"
	DateFormatDMYDot   = "DD.MM.YYYY"
	DateFormatISO      = "YYYY-MM-DD"

	TimeFormat24h = "24h"
	TimeFormat12h = "12h"

	ListSortUpcoming = "upcoming"
	ListSortName     = "name"
	ListSortAge      = "age"
	ListSortCreated  = "created"

	ListViewUpcoming = "upcoming"
	ListViewAll      = "all"
	ListViewMuted    = "muted"
	ListViewTrash    = "trash"

	AdarPolicyAdarI  = "adar_i"
	AdarPolicyAdarII = "adar_ii"

	Feb29PolicyFeb28 = "feb28"
	Feb29PolicyMar01 = "mar01"

	BackupKindAuto   = "auto"
	BackupKindManual = "manual"
	BackupKindExport = "export"

	BackupFormatDB   = "db"
	BackupFormatJSON = "json"
	BackupFormatCSV  = "csv"
	BackupFormatXLSX = "xlsx"

	AuditActionStart            = "start"
	AuditActionEventCreate      = "event_create"
	AuditActionEventUpdate      = "event_update"
	AuditActionEventDelete      = "event_delete"
	AuditActionEventRestore     = "event_restore"
	AuditActionRuleCreate       = "rule_create"
	AuditActionRuleDelete       = "rule_delete"
	AuditActionSettingsUpdate   = "settings_update"
	AuditActionExport           = "export"
	AuditActionImport           = "import"
	AuditActionNotificationSent = "notification_sent"
	AuditActionNotificationFail = "notification_failed"
	AuditActionAdminBroadcast   = "admin_broadcast"
	AuditActionUserBlocked      = "user_blocked"
	AuditActionError            = "error"
)

// Field length limits (SPEC.md chapter 27).
const (
	NameMaxLen            = 64
	NicknameMaxLen        = 32
	NotesMaxLen           = 500
	PhoneMaxLen           = 20
	RelationMaxLen        = 32
	CustomTypeLabelMaxLen = 32
)

// Domain limits.
const (
	MinYearGregorian            = 1900
	MaxYearGregorian            = 2100
	MinYearHebrew               = 5660
	MaxYearHebrew               = 5860
	MaxReminderRulesGlobal      = 5
	MaxReminderRulesPerEvent    = 5
	MaxGreetingTemplatesPerUser = 20
	SoftDeleteRetentionDays     = 30
)

// ReminderOffsetChoices are the offsets available in the quick reminder-add
// flow (SPEC.md chapter 15, S11).
var ReminderOffsetChoices = [...]int{0, 1, 2, 3, 7, 14, 30}

// Ordered enum values, matching app/constants.py's StrEnum declaration
// order exactly — several keyboards iterate `for c in Category` etc. and
// depend on that order for button layout.
var (
	CategoryValues  = []string{CategoryFamily, CategoryFriends, CategoryWork, CategoryClients, CategorySchool, CategoryOther}
	EventTypeValues = []string{EventTypeBirthday, EventTypeAnniversary, EventTypeWedding, EventTypeMemorial, EventTypeCustom}
	GenderValues    = []string{GenderMale, GenderFemale, GenderOther}
)

// HebrewMonthNames is indexed by (h_month - 1); h_month uses the SPEC.md
// numbering (1=Nisan .. 12=Adar/Adar I, 13=Adar II).
var HebrewMonthNames = [13]string{
	"ניסן",
	"אייר",
	"סיוון",
	"תמוז",
	"אב",
	"אלול",
	"תשרי",
	"חשוון",
	"כסלו",
	"טבת",
	"שבט",
	"אדר",
	"אדר ב׳",
}
