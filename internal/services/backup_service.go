package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"birthly/internal/core"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// Backup, export, and import — port of app/services/backup_service.py
// (SPEC.md chapter 23). Export formats: JSON (full round-trip), CSV
// (events only, UTF-8 BOM, Hebrew headers), XLSX (events + summary sheet,
// RTL). Import only accepts JSON.
//
// app/services/backup_service.py also had a standalone auto_backup()/
// _vacuum_into() pair that nothing in the codebase called (the scheduler's
// actual auto-backup job has its own separate inline VACUUM INTO in
// app/scheduler/jobs.py, already ported in internal/scheduler/jobs.go) —
// that dead pair is not ported here.

const exportSchemaVersion = 1

var csvHeaders = []string{
	"שם פרטי", "שם משפחה", "כינוי", "תאריך (יום)", "תאריך (חודש)", "תאריך (שנה)",
	"לוח", "סוג אירוע", "קטגוריה", "קשר", "מין", "טלפון", "טלגרם", "הערות",
	"שעת אירוע", "פעיל", "תאריך משני (יום)", "תאריך משני (חודש)", "לוח משני",
}

type exportPayload struct {
	SchemaVersion       int              `json:"schema_version"`
	ExportedAt          string           `json:"exported_at"`
	User                exportUser       `json:"user"`
	Events              []exportEvent    `json:"events"`
	GlobalReminderRules []exportRule     `json:"global_reminder_rules"`
	PersonalTemplates   []exportTemplate `json:"personal_templates"`
}

type exportUser struct {
	Language           string `json:"language"`
	Timezone           string `json:"timezone"`
	DateFormat         string `json:"date_format"`
	TimeFormat         string `json:"time_format"`
	DefaultNotifyTime  string `json:"default_notify_time"`
	ShowHebrewDate     bool   `json:"show_hebrew_date"`
	DailyDigestEnabled bool   `json:"daily_digest_enabled"`
	DigestTime         string `json:"digest_time"`
	AdarPolicy         string `json:"adar_policy"`
	Feb29Policy        string `json:"feb29_policy"`
}

type exportEvent struct {
	FirstName    string  `json:"first_name"`
	LastName     *string `json:"last_name"`
	Nickname     *string `json:"nickname"`
	Month        int     `json:"month"`
	Day          int     `json:"day"`
	Year         *int    `json:"year"`
	CalendarType string  `json:"calendar_type"`
	// Secondary date: an optional second calendar track (SPEC "dual
	// hebrew/gregorian dates" feature). Omitted when the event has no
	// secondary date, matching every other optional field in this struct.
	SecondaryCalendarType *string `json:"secondary_calendar_type,omitempty"`
	SecondaryMonth        *int    `json:"secondary_month,omitempty"`
	SecondaryDay          *int    `json:"secondary_day,omitempty"`
	EventType             string  `json:"event_type"`
	CustomTypeLabel       *string `json:"custom_type_label"`
	Category              string  `json:"category"`
	Gender                *string `json:"gender"`
	Relation              *string `json:"relation"`
	Phone                 *string `json:"phone"`
	TelegramUsername      *string `json:"telegram_username"`
	Notes                 *string `json:"notes"`
	EventTime             *string `json:"event_time"`
	IsActive              bool    `json:"is_active"`
	// photo_file_id intentionally omitted (SPEC §23)
	ReminderRules []exportRule `json:"reminder_rules"`
}

type exportRule struct {
	OffsetDays    *int    `json:"offset_days"`
	OffsetMinutes *int    `json:"offset_minutes"`
	SendTime      *string `json:"send_time"`
	Enabled       bool    `json:"enabled"`
}

type exportTemplate struct {
	EventType string  `json:"event_type"`
	Tone      string  `json:"tone"`
	Gender    *string `json:"gender"`
	Language  string  `json:"language"`
	Body      string  `json:"body"`
}

func eventToExport(e *models.Event) exportEvent {
	return exportEvent{
		FirstName: e.FirstName, LastName: e.LastName, Nickname: e.Nickname,
		Month: e.Month, Day: e.Day, Year: e.Year, CalendarType: e.CalendarType,
		SecondaryCalendarType: e.SecondaryCalendarType, SecondaryMonth: e.SecondaryMonth, SecondaryDay: e.SecondaryDay,
		EventType: e.EventType, CustomTypeLabel: e.CustomTypeLabel, Category: e.Category,
		Gender: e.Gender, Relation: e.Relation, Phone: e.Phone,
		TelegramUsername: e.TelegramUsername, Notes: e.Notes, EventTime: e.EventTime,
		IsActive: e.IsActive, ReminderRules: []exportRule{},
	}
}

func ruleToExport(r *models.ReminderRule) exportRule {
	return exportRule{OffsetDays: r.OffsetDays, OffsetMinutes: r.OffsetMinutes, SendTime: r.SendTime, Enabled: r.Enabled}
}

func templateToExport(t *models.GreetingTemplate) exportTemplate {
	return exportTemplate{EventType: t.EventType, Tone: t.Tone, Gender: t.Gender, Language: t.Language, Body: t.Body}
}

// ExportJSON exports all user data as a JSON blob suitable for re-import.
func ExportJSON(ctx context.Context, db repo.DBTX, user *models.User) ([]byte, error) {
	events := repo.NewEventRepo(db, user.ID)
	rules := repo.NewReminderRuleRepo(db, user.ID)
	templates := repo.NewTemplateRepo(db, user.ID)

	eventList, err := events.ListNotDeleted(ctx)
	if err != nil {
		return nil, err
	}
	globalRules, err := rules.ListGlobal(ctx)
	if err != nil {
		return nil, err
	}
	tplList, err := templates.ListUserTemplates(ctx)
	if err != nil {
		return nil, err
	}

	payload := exportPayload{
		SchemaVersion: exportSchemaVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		User: exportUser{
			Language: user.Language, Timezone: user.Timezone, DateFormat: user.DateFormat,
			TimeFormat: user.TimeFormat, DefaultNotifyTime: user.DefaultNotifyTime,
			ShowHebrewDate: user.ShowHebrewDate, DailyDigestEnabled: user.DailyDigestEnabled,
			DigestTime: user.DigestTime, AdarPolicy: user.AdarPolicy, Feb29Policy: user.Feb29Policy,
		},
		Events:              make([]exportEvent, len(eventList)),
		GlobalReminderRules: make([]exportRule, len(globalRules)),
		PersonalTemplates:   make([]exportTemplate, len(tplList)),
	}
	for i, r := range globalRules {
		payload.GlobalReminderRules[i] = ruleToExport(r)
	}
	for i, t := range tplList {
		payload.PersonalTemplates[i] = templateToExport(t)
	}
	for i, e := range eventList {
		exp := eventToExport(e)
		perEventRules, err := rules.ListForEvent(ctx, e.ID)
		if err != nil {
			return nil, err
		}
		exp.ReminderRules = make([]exportRule, len(perEventRules))
		for j, r := range perEventRules {
			exp.ReminderRules[j] = ruleToExport(r)
		}
		payload.Events[i] = exp
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func eventCSVRow(e *models.Event) []string {
	calendarLabel := "לועזי"
	if e.CalendarType == core.CalendarTypeHebrew {
		calendarLabel = "עברי"
	}
	activeLabel := "לא"
	if e.IsActive {
		activeLabel = "כן"
	}
	year := ""
	if e.Year != nil {
		year = strconv.Itoa(*e.Year)
	}
	secDay, secMonth, secCalendarLabel := "", "", ""
	if e.SecondaryMonth != nil && e.SecondaryDay != nil {
		secDay = strconv.Itoa(*e.SecondaryDay)
		secMonth = strconv.Itoa(*e.SecondaryMonth)
		secCalendarLabel = "לועזי"
		if e.SecondaryCalendarType != nil && *e.SecondaryCalendarType == core.CalendarTypeHebrew {
			secCalendarLabel = "עברי"
		}
	}
	return []string{
		e.FirstName, deref(e.LastName), deref(e.Nickname),
		strconv.Itoa(e.Day), strconv.Itoa(e.Month), year,
		calendarLabel, e.EventType, e.Category, deref(e.Relation), deref(e.Gender),
		deref(e.Phone), deref(e.TelegramUsername), deref(e.Notes), deref(e.EventTime),
		activeLabel, secDay, secMonth, secCalendarLabel,
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ExportCSV exports events as CSV with a UTF-8 BOM and Hebrew headers.
func ExportCSV(ctx context.Context, db repo.DBTX, user *models.User) ([]byte, error) {
	events, err := repo.NewEventRepo(db, user.ID).ListNotDeleted(ctx)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString("\ufeff") // BOM
	w := csv.NewWriter(&buf)
	if err := w.Write(csvHeaders); err != nil {
		return nil, err
	}
	for _, e := range events {
		if err := w.Write(eventCSVRow(e)); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ExportXLSX exports events as an XLSX workbook with an events sheet and a
// summary sheet, both RTL.
func ExportXLSX(ctx context.Context, db repo.DBTX, user *models.User) ([]byte, error) {
	events, err := repo.NewEventRepo(db, user.ID).ListNotDeleted(ctx)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer f.Close()

	const eventsSheet = "אירועים"
	f.SetSheetName("Sheet1", eventsSheet)
	rtl := true
	if err := f.SetSheetView(eventsSheet, 0, &excelize.ViewOptions{RightToLeft: &rtl}); err != nil {
		return nil, err
	}

	boldStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}

	colWidths := make([]int, len(csvHeaders))
	for col, header := range csvHeaders {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		if err := f.SetCellStr(eventsSheet, cell, header); err != nil {
			return nil, err
		}
		colWidths[col] = utf8.RuneCountInString(header)
	}
	endCell, _ := excelize.CoordinatesToCellName(len(csvHeaders), 1)
	if err := f.SetCellStyle(eventsSheet, "A1", endCell, boldStyle); err != nil {
		return nil, err
	}

	for rowIdx, e := range events {
		row := rowIdx + 2
		for col, val := range eventCSVRow(e) {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			if err := f.SetCellStr(eventsSheet, cell, val); err != nil {
				return nil, err
			}
			if n := utf8.RuneCountInString(val); n > colWidths[col] {
				colWidths[col] = n
			}
		}
	}
	for col, w := range colWidths {
		width := float64(w + 2)
		if width > 40 {
			width = 40
		}
		colLetter, _ := excelize.ColumnNumberToName(col + 1)
		if err := f.SetColWidth(eventsSheet, colLetter, colLetter, width); err != nil {
			return nil, err
		}
	}

	const summarySheet = "סיכום"
	if _, err := f.NewSheet(summarySheet); err != nil {
		return nil, err
	}
	if err := f.SetSheetView(summarySheet, 0, &excelize.ViewOptions{RightToLeft: &rtl}); err != nil {
		return nil, err
	}
	total := len(events)
	active := 0
	for _, e := range events {
		if e.IsActive {
			active++
		}
	}
	summaryRows := [][2]string{
		{`סה"כ אירועים`, strconv.Itoa(total)},
		{"פעילים", strconv.Itoa(active)},
		{"מושתקים", strconv.Itoa(total - active)},
		{"יוצא בתאריך", time.Now().UTC().Format("02/01/2006 15:04")},
	}
	for i, r := range summaryRows {
		if err := f.SetCellStr(summarySheet, "A"+strconv.Itoa(i+1), r[0]); err != nil {
			return nil, err
		}
		if err := f.SetCellStr(summarySheet, "B"+strconv.Itoa(i+1), r[1]); err != nil {
			return nil, err
		}
	}

	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportResult tallies the outcome of DoImport.
type ImportResult struct {
	TotalParsed int      `json:"total_parsed"`
	Imported    int      `json:"imported"`
	Duplicates  int      `json:"duplicates"`
	Errors      []string `json:"errors"`
}

// ParseImportJSON parses raw bytes into a validated import payload map.
// Returns (payload, result) where result carries just total_parsed; errors
// are populated during DoImport's per-row parsing. Returns an error if the
// bytes aren't valid JSON at all, or the root isn't an object with an
// "events" array.
func ParseImportJSON(data []byte) (map[string]any, ImportResult, error) {
	var result ImportResult
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, result, fmt.Errorf("root element must be a JSON object: %w", err)
	}

	eventsRaw, ok := payload["events"].([]any)
	if !ok {
		if payload["events"] == nil {
			eventsRaw = nil
		} else {
			return nil, result, fmt.Errorf("'events' must be a list")
		}
	}
	result.TotalParsed = len(eventsRaw)
	return payload, result, nil
}

// DoImport executes the import. mode is "add" (merge) or "replace" (soft-
// delete everything first). Duplicate detection key: (first_name, last_name,
// month, day, calendar_type), case-insensitive on the name parts. Bad rows
// accumulate into result.Errors and never abort the import.
func DoImport(ctx context.Context, db repo.DBTX, user *models.User, payload map[string]any, mode string) (ImportResult, error) {
	var result ImportResult
	eventsRaw, _ := payload["events"].([]any)
	result.TotalParsed = len(eventsRaw)

	events := repo.NewEventRepo(db, user.ID)
	today := UserToday(user)

	if mode == "replace" {
		existing, err := events.ListNotDeleted(ctx)
		if err != nil {
			return result, err
		}
		for _, e := range existing {
			if err := events.SoftDelete(ctx, e.ID); err != nil {
				return result, err
			}
		}
	}

	existingEvents, err := events.ListNotDeleted(ctx)
	if err != nil {
		return result, err
	}
	dupKeys := make(map[string]bool, len(existingEvents))
	dupKey := func(firstName, lastName string, month, day int, calendarType string) string {
		return strings.ToLower(firstName) + "\x00" + strings.ToLower(lastName) + "\x00" +
			strconv.Itoa(month) + "\x00" + strconv.Itoa(day) + "\x00" + calendarType
	}
	for _, e := range existingEvents {
		dupKeys[dupKey(e.FirstName, deref(e.LastName), e.Month, e.Day, e.CalendarType)] = true
	}

	for i, raw := range eventsRaw {
		rowMap, ok := raw.(map[string]any)
		if !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: not an object", i+1))
			continue
		}
		event, err := dictToEvent(rowMap, today, user)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: %v", i+1, err))
			continue
		}

		key := dupKey(event.FirstName, deref(event.LastName), event.Month, event.Day, event.CalendarType)
		if dupKeys[key] {
			result.Duplicates++
			continue
		}
		dupKeys[key] = true

		if _, err := events.Create(ctx, event); err != nil {
			return result, err
		}
		result.Imported++
	}

	return result, nil
}

var validCalendarTypes = map[string]bool{core.CalendarTypeGregorian: true, core.CalendarTypeHebrew: true}
var validEventTypes = map[string]bool{
	core.EventTypeBirthday: true, core.EventTypeAnniversary: true, core.EventTypeWedding: true,
	core.EventTypeMemorial: true, core.EventTypeCustom: true,
}
var validCategories = map[string]bool{
	core.CategoryFamily: true, core.CategoryFriends: true, core.CategoryWork: true,
	core.CategoryClients: true, core.CategorySchool: true, core.CategoryOther: true,
}

// dictToEvent converts a raw JSON object (already decoded to map[string]any)
// to an Event, raising an error on invalid data — port of _dict_to_event.
func dictToEvent(raw map[string]any, today time.Time, user *models.User) (*models.Event, error) {
	firstName := strings.TrimSpace(stringField(raw, "first_name"))
	if firstName == "" {
		return nil, fmt.Errorf("missing first_name")
	}

	month, err := intField(raw, "month")
	if err != nil {
		return nil, fmt.Errorf("invalid month: %w", err)
	}
	day, err := intField(raw, "day")
	if err != nil {
		return nil, fmt.Errorf("invalid day: %w", err)
	}
	if month < 1 || month > 13 {
		return nil, fmt.Errorf("invalid month: %d", month)
	}
	if day < 1 || day > 31 {
		return nil, fmt.Errorf("invalid day: %d", day)
	}

	var year *int
	if raw["year"] != nil {
		y, err := intField(raw, "year")
		if err != nil {
			return nil, fmt.Errorf("invalid year: %w", err)
		}
		year = &y
	}

	calendarType := stringFieldDefault(raw, "calendar_type", core.CalendarTypeGregorian)
	if !validCalendarTypes[calendarType] {
		return nil, fmt.Errorf("invalid calendar_type: %s", calendarType)
	}

	eventType := stringFieldDefault(raw, "event_type", core.EventTypeBirthday)
	if !validEventTypes[eventType] {
		return nil, fmt.Errorf("invalid event_type: %s", eventType)
	}

	category := stringFieldDefault(raw, "category", core.CategoryOther)
	if !validCategories[category] {
		category = core.CategoryOther
	}

	occ, err := core.NextOccurrence(calendarType, month, day, today, user.AdarPolicy, user.Feb29Policy)
	if err != nil {
		return nil, err
	}

	// Secondary date: only honored when the primary is hebrew, matching the
	// one-directional invariant the add flow enforces (see NewEventInput).
	// Silently ignored otherwise rather than erroring the whole row, since a
	// stray secondary_month/day on a gregorian row is harmless to drop.
	var secondaryCalendarType *string
	var secondaryMonth, secondaryDay *int
	var secondaryOcc *time.Time
	if calendarType == core.CalendarTypeHebrew && raw["secondary_month"] != nil && raw["secondary_day"] != nil {
		sm, err := intField(raw, "secondary_month")
		if err != nil {
			return nil, fmt.Errorf("invalid secondary_month: %w", err)
		}
		sd, err := intField(raw, "secondary_day")
		if err != nil {
			return nil, fmt.Errorf("invalid secondary_day: %w", err)
		}
		if sm < 1 || sm > 12 {
			return nil, fmt.Errorf("invalid secondary_month: %d", sm)
		}
		if sd < 1 || sd > 31 {
			return nil, fmt.Errorf("invalid secondary_day: %d", sd)
		}
		ct := core.CalendarTypeGregorian
		secOcc, err := core.NextOccurrence(ct, sm, sd, today, user.AdarPolicy, user.Feb29Policy)
		if err != nil {
			return nil, err
		}
		secondaryCalendarType, secondaryMonth, secondaryDay, secondaryOcc = &ct, &sm, &sd, &secOcc
	}

	return &models.Event{
		FirstName: firstName, LastName: optStringField(raw, "last_name"),
		Nickname: optStringField(raw, "nickname"), Month: month, Day: day, Year: year,
		CalendarType: calendarType, EventType: eventType,
		CustomTypeLabel: optStringField(raw, "custom_type_label"), Category: category,
		Gender: optStringField(raw, "gender"), Relation: optStringField(raw, "relation"),
		Phone: optStringField(raw, "phone"), TelegramUsername: optStringField(raw, "telegram_username"),
		Notes: optStringField(raw, "notes"), EventTime: optStringField(raw, "event_time"),
		IsActive: boolFieldDefault(raw, "is_active", true), NextOccurrence: &occ,
		SecondaryCalendarType: secondaryCalendarType, SecondaryMonth: secondaryMonth, SecondaryDay: secondaryDay,
		SecondaryNextOccurrence: secondaryOcc,
	}, nil
}

func stringField(raw map[string]any, key string) string {
	if v, ok := raw[key].(string); ok {
		return v
	}
	return ""
}

func stringFieldDefault(raw map[string]any, key, def string) string {
	if v, ok := raw[key].(string); ok && v != "" {
		return v
	}
	return def
}

func optStringField(raw map[string]any, key string) *string {
	v, ok := raw[key].(string)
	if !ok || v == "" {
		return nil
	}
	return &v
}

func boolFieldDefault(raw map[string]any, key string, def bool) bool {
	if v, ok := raw[key].(bool); ok {
		return v
	}
	return def
}

// intField reads a numeric field that arrived via encoding/json as float64
// (JSON has no separate int type), matching Python's int(raw["month"])
// coercion (which also accepts numeric strings — replicated here too).
func intField(raw map[string]any, key string) (int, error) {
	switch v := raw[key].(type) {
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(v))
	default:
		return 0, fmt.Errorf("missing or non-numeric %q", key)
	}
}
