package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/config"
	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// fakeBotClient intercepts every gotgbot API call so tests never touch the
// network. sendResult/sendErr control what "sendMessage" returns; every
// call (method + chat_id) is recorded for assertions.
type fakeBotClient struct {
	mu       sync.Mutex
	calls    []map[string]any
	sendErr  error // if set, every sendMessage call fails with this error
	sendErrN int   // if >0, only the first N sendMessage calls fail with sendErr
}

func (f *fakeBotClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, map[string]any{"method": method, "params": params})
	callIdx := len(f.calls)
	f.mu.Unlock()

	if method != "sendMessage" && method != "sendPhoto" {
		return json.RawMessage(`{}`), nil
	}
	if f.sendErr != nil && (f.sendErrN == 0 || callIdx <= f.sendErrN) {
		return nil, f.sendErr
	}
	return json.Marshal(gotgbot.Message{MessageId: 1, Date: int64(time.Now().Unix())})
}

func (f *fakeBotClient) GetAPIURL(opts *gotgbot.RequestOpts) string { return "https://example.invalid" }
func (f *fakeBotClient) FileURL(token, path string, opts *gotgbot.RequestOpts) string {
	return "https://example.invalid/" + path
}

func (f *fakeBotClient) sendCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c["method"] == "sendMessage" || c["method"] == "sendPhoto" {
			n++
		}
	}
	return n
}

func (f *fakeBotClient) lastChatID() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i]["method"] != "sendMessage" && f.calls[i]["method"] != "sendPhoto" {
			continue
		}
		params := f.calls[i]["params"].(map[string]any)
		if id, ok := params["chat_id"].(int64); ok {
			return id
		}
	}
	return 0
}

func testBot(client *fakeBotClient) *gotgbot.Bot {
	return &gotgbot.Bot{Token: "test-token", BotClient: client}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testConfig() *config.Config {
	return &config.Config{
		ReminderGraceHours:  6,
		MaxUpcomingDays:     45,
		BroadcastRatePerSec: 20,
	}
}

func seedUserEventRule(t *testing.T, db *sql.DB, userID int64, tz, notifyTime string, offsetDays *int, sendTime *string, eventID *int64) (*models.User, *models.Event) {
	t.Helper()
	ctx := context.Background()
	users := repo.NewUserRepo(db)
	name := "User"
	user, _, err := users.GetOrCreate(ctx, userID, nil, name, nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate user: %v", err)
	}
	user.Timezone = tz
	user.DefaultNotifyTime = notifyTime
	if err := users.UpdateSettings(ctx, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	occ := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	year := 1990
	events := repo.NewEventRepo(db, userID)
	event, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Dana", Category: "other",
		CalendarType: "gregorian", Month: 8, Day: 1, Year: &year,
		NextOccurrence: &occ, IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	rules := repo.NewReminderRuleRepo(db, userID)
	rule := &models.ReminderRule{OffsetDays: offsetDays, SendTime: sendTime, Enabled: true, EventID: eventID}
	if _, err := rules.Create(ctx, rule); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	return user, event
}

func TestTickReminders_FiresExactlyOnce(t *testing.T) {
	db := testDB(t)
	offset := 1
	sendTime := "09:00"
	seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, &sendTime, nil)

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()
	logger := testLogger()
	ctx := context.Background()

	// fires 2026-07-31 09:00 local (Asia/Jerusalem, UTC+3 in summer) = 06:00 UTC
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount = %d, want 1", client.sendCount())
	}

	// Second tick in the same minute-ish window must NOT resend.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 7, 31, 6, 1, 30, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders (second): %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount after second tick = %d, want still 1", client.sendCount())
	}
}

func TestTickReminders_NotificationsDisabledNoSend(t *testing.T) {
	db := testDB(t)
	offset := 1
	sendTime := "09:00"
	user, _ := seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, &sendTime, nil)

	user.NotificationsEnabled = false
	if err := repo.NewUserRepo(db).UpdateSettings(context.Background(), user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()

	// Same fire time as TestTickReminders_FiresExactlyOnce, which sends.
	// The only difference here is notifications_enabled=false.
	if err := TickReminders(context.Background(), bot, db, cfg, time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC), testLogger()); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 0 {
		t.Fatalf("sendCount = %d, want 0 (notifications disabled)", client.sendCount())
	}
}

func TestTickReminders_DowntimeWithinGraceSends(t *testing.T) {
	db := testDB(t)
	offset := 1
	sendTime := "09:00"
	seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, &sendTime, nil)

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()

	// Fire time is 06:00 UTC; resume at 09:00 UTC: 3h late, within 6h grace.
	if err := TickReminders(context.Background(), bot, db, cfg, time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC), testLogger()); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount = %d, want 1", client.sendCount())
	}
}

func TestTickReminders_DowntimeOutsideGraceSkips(t *testing.T) {
	db := testDB(t)
	offset := 1
	sendTime := "09:00"
	seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, &sendTime, nil)

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()

	// Fire time is 06:00 UTC; resume at 16:00 UTC: 10h late, outside 6h grace.
	if err := TickReminders(context.Background(), bot, db, cfg, time.Date(2026, 7, 31, 16, 0, 0, 0, time.UTC), testLogger()); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 0 {
		t.Fatalf("sendCount = %d, want 0 (outside grace)", client.sendCount())
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM notifications_log LIMIT 1`).Scan(&status); err != nil {
		t.Fatalf("querying notifications_log: %v", err)
	}
	if status != "skipped" {
		t.Errorf("status = %q, want skipped", status)
	}
}

func TestTickReminders_TwoUsersDifferentTimezones(t *testing.T) {
	db := testDB(t)
	offset := 1
	seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, nil, nil)
	seedUserEventRule(t, db, 2, "America/New_York", "09:00", &offset, nil, nil)

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()
	logger := testLogger()
	ctx := context.Background()

	// 06:01 UTC = 09:01 Israel time, 02:01 NY time: only IL user fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount = %d, want 1", client.sendCount())
	}
	if client.lastChatID() != 1 {
		t.Errorf("lastChatID = %d, want 1 (IL user)", client.lastChatID())
	}

	// 13:01 UTC = 09:01 NY time: NY user now fires too.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 7, 31, 13, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 2 {
		t.Fatalf("sendCount = %d, want 2", client.sendCount())
	}
}

func TestTickReminders_PerEventRuleOverridesGlobal(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	users := repo.NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 1, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	user.Timezone = "Asia/Jerusalem"
	if err := users.UpdateSettings(ctx, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	occ := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	year := 1990
	events := repo.NewEventRepo(db, 1)
	event, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Dana", Category: "other",
		CalendarType: "gregorian", Month: 8, Day: 1, Year: &year,
		NextOccurrence: &occ, IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	rules := repo.NewReminderRuleRepo(db, 1)
	offset := 1
	globalSendTime, eventSendTime := "09:00", "12:00"
	if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &offset, SendTime: &globalSendTime, Enabled: true}); err != nil {
		t.Fatalf("create global rule: %v", err)
	}
	if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &offset, SendTime: &eventSendTime, Enabled: true, EventID: &event.ID}); err != nil {
		t.Fatalf("create event rule: %v", err)
	}

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()
	logger := testLogger()

	// local 09:01 (06:01 UTC): global would fire but is overridden -> no send.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 0 {
		t.Fatalf("sendCount after global-time tick = %d, want 0 (overridden)", client.sendCount())
	}

	// local 12:01 (09:01 UTC): event-specific rule fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 7, 31, 9, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount after event-time tick = %d, want 1", client.sendCount())
	}
}

func TestTickReminders_BotForbiddenSetsBlocked(t *testing.T) {
	db := testDB(t)
	offset := 1
	seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, nil, nil)

	client := &fakeBotClient{sendErr: &gotgbot.TelegramError{Code: 403, Description: "Forbidden"}}
	bot := testBot(client)
	cfg := testConfig()

	if err := TickReminders(context.Background(), bot, db, cfg, time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC), testLogger()); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}

	user, err := repo.NewUserRepo(db).Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get user: %v", err)
	}
	if !user.BotBlockedByUser {
		t.Error("expected BotBlockedByUser=true after 403")
	}
}

func TestTickReminders_RecordsLastTickAt(t *testing.T) {
	db := testDB(t)
	offset := 1
	seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, nil, nil)

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()

	tickTime := time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC)
	if err := TickReminders(context.Background(), bot, db, cfg, tickTime, testLogger()); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}

	v, err := repo.NewAppMetaRepo(db).Get(context.Background(), "last_tick_at")
	if err != nil || v == nil {
		t.Fatalf("last_tick_at not recorded: v=%v err=%v", v, err)
	}
}

func TestTickReminders_SameMinuteDeduplication(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	users := repo.NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 1, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	user.Timezone = "Asia/Jerusalem"
	if err := users.UpdateSettings(ctx, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	occ := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	events := repo.NewEventRepo(db, 1)
	event, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Dana", Category: "other",
		CalendarType: "gregorian", Month: 8, Day: 1,
		NextOccurrence: &occ, IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	rules := repo.NewReminderRuleRepo(db, 1)
	offsetOne, offsetZero := 1, 0
	sendTime := "09:00"
	if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &offsetOne, SendTime: &sendTime, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &offsetZero, SendTime: &sendTime, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_ = event

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()
	logger := testLogger()

	// Day before at 09:01 local (06:01 UTC): only the offset=1 rule fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount = %d, want 1", client.sendCount())
	}

	// On occurrence day at 09:01 local (06:01 UTC): only the offset=0 rule fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 1, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders (day 2): %v", err)
	}
	if client.sendCount() != 2 {
		t.Fatalf("sendCount after day-of tick = %d, want 2", client.sendCount())
	}
}

// TestTickReminders_OffsetZero_FullDaySimulation tests the exact failure mode:
// an event on 2026-08-31 with offset_days=0 and send_time=09:00 in Asia/Jerusalem.
// Ticks run at 00:00, 03:00, 09:01, 12:00, 23:59, and next day 00:01.
func TestTickReminders_OffsetZero_FullDaySimulation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	users := repo.NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 1, nil, "Omer", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	user.Timezone = "Asia/Jerusalem"
	user.DefaultNotifyTime = "09:00"
	if err := users.UpdateSettings(ctx, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	occ := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	year := 2002
	events := repo.NewEventRepo(db, 1)
	event, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Omer", Category: "other",
		CalendarType: "gregorian", Month: 8, Day: 31, Year: &year,
		NextOccurrence: &occ, IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	rules := repo.NewReminderRuleRepo(db, 1)
	offsetZero := 0
	sendTime := "09:00"
	if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &offsetZero, SendTime: &sendTime, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()
	logger := testLogger()

	// 1. Midnight tick at start of birthday: 2026-08-31 00:01 local = 2026-08-30 21:01 UTC.
	// Must NOT send yet, and critically must NOT advance NextOccurrence to 2027.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 30, 21, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick at midnight: %v", err)
	}
	if client.sendCount() != 0 {
		t.Fatalf("sendCount at midnight = %d, want 0", client.sendCount())
	}
	ev, err := events.GetOwned(ctx, event.ID)
	if err != nil || ev == nil {
		t.Fatalf("get event: %v", err)
	}
	if ev.NextOccurrence == nil || !ev.NextOccurrence.Equal(occ) {
		t.Fatalf("NextOccurrence was prematurely changed at midnight: got %v, want %v", ev.NextOccurrence, occ)
	}

	// 2. Early morning tick: 2026-08-31 06:00 local = 2026-08-31 03:00 UTC.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 31, 3, 0, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick at 06:00 local: %v", err)
	}
	if client.sendCount() != 0 {
		t.Fatalf("sendCount at 06:00 local = %d, want 0", client.sendCount())
	}

	// 3. Fire time tick: 2026-08-31 09:01 local = 2026-08-31 06:01 UTC.
	// The offset=0 reminder MUST fire now.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 31, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick at fire time: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount at fire time = %d, want 1", client.sendCount())
	}

	// Verify occurrence is still 2026-08-31 so UI displays birthday today.
	ev, err = events.GetOwned(ctx, event.ID)
	if err != nil || ev == nil {
		t.Fatalf("get event: %v", err)
	}
	if ev.NextOccurrence == nil || !ev.NextOccurrence.Equal(occ) {
		t.Fatalf("NextOccurrence was prematurely changed after send: got %v, want %v", ev.NextOccurrence, occ)
	}

	// 4. Afternoon tick: 2026-08-31 15:00 local = 2026-08-31 12:00 UTC.
	// Must not send duplicate.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick at afternoon: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount at afternoon = %d, want still 1", client.sendCount())
	}

	// 5. Late night tick: 2026-08-31 23:59 local = 2026-08-31 20:59 UTC.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 31, 20, 59, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick at late night: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount at late night = %d, want still 1", client.sendCount())
	}

	// 6. Day after tick: 2026-09-01 00:01 local = 2026-08-31 21:01 UTC.
	// Now that birthday has passed, NextOccurrence must advance to next year (2027-08-31).
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 31, 21, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick day after: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount day after = %d, want still 1", client.sendCount())
	}
	ev, err = events.GetOwned(ctx, event.ID)
	if err != nil || ev == nil {
		t.Fatalf("get event: %v", err)
	}
	expectedNext := time.Date(2027, 8, 31, 0, 0, 0, 0, time.UTC)
	if ev.NextOccurrence == nil || !ev.NextOccurrence.Equal(expectedNext) {
		t.Fatalf("NextOccurrence was not advanced on the day after: got %v, want %v", ev.NextOccurrence, expectedNext)
	}
}

// TestTickReminders_MultipleRulesOffsetOneAndZero tests that offset=1 (day before)
// and multiple offset=0 rules (different hours on event day) all fire in sequence.
func TestTickReminders_MultipleRulesOffsetOneAndZero(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	users := repo.NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 1, nil, "MultiRuleUser", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	user.Timezone = "Asia/Jerusalem"
	if err := users.UpdateSettings(ctx, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	occ := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	year := 1995
	events := repo.NewEventRepo(db, 1)
	event, err := events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Talia", Category: "family",
		CalendarType: "gregorian", Month: 8, Day: 31, Year: &year,
		NextOccurrence: &occ, IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	rules := repo.NewReminderRuleRepo(db, 1)
	offsetOne, offsetZero := 1, 0
	time10, time12 := "10:00", "12:00"

	// Rule 1: 1 day before at 10:00 (fire 2026-08-30 07:00 UTC)
	if _, err := rules.Create(ctx, &models.ReminderRule{EventID: &event.ID, OffsetDays: &offsetOne, SendTime: &time10, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Rule 2: day of at 10:00 (fire 2026-08-31 07:00 UTC)
	if _, err := rules.Create(ctx, &models.ReminderRule{EventID: &event.ID, OffsetDays: &offsetZero, SendTime: &time10, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Rule 3: day of at 12:00 (fire 2026-08-31 09:00 UTC)
	if _, err := rules.Create(ctx, &models.ReminderRule{EventID: &event.ID, OffsetDays: &offsetZero, SendTime: &time12, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()
	logger := testLogger()

	// Day before at 10:01 local (07:01 UTC): Rule 1 fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 30, 7, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick day before: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount after offset=1 = %d, want 1", client.sendCount())
	}

	// Day of event at 00:01 local (2026-08-30 21:01 UTC): nothing fires yet.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 30, 21, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick midnight: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount at midnight = %d, want 1", client.sendCount())
	}

	// Day of event at 10:01 local (07:01 UTC): Rule 2 fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 31, 7, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick 10:00 rule: %v", err)
	}
	if client.sendCount() != 2 {
		t.Fatalf("sendCount after 10:00 rule = %d, want 2", client.sendCount())
	}

	// Day of event at 12:01 local (09:01 UTC): Rule 3 fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 31, 9, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick 12:00 rule: %v", err)
	}
	if client.sendCount() != 3 {
		t.Fatalf("sendCount after 12:00 rule = %d, want 3", client.sendCount())
	}
}

// TestRecoverPending_ExpiredGraceMarkedSkipped verifies that a pending notification
// created long ago (e.g. from an interrupted run before a server crash) is skipped.
func TestRecoverPending_ExpiredGraceMarkedSkipped(t *testing.T) {
	db := testDB(t)
	offset := 1
	sendTime := "09:00"
	user, event := seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, &sendTime, nil)

	ctx := context.Background()
	notifRepo := repo.NewNotificationRepo(db)

	// Scheduled 30 days ago, never completed (still pending).
	oldDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	oldSched := time.Date(2026, 6, 30, 6, 0, 0, 0, time.UTC)
	pendingLog, err := notifRepo.CreatePending(ctx, user.ID, event.ID, nil, oldDate, oldSched)
	if err != nil || pendingLog == nil {
		t.Fatalf("CreatePending: %v", err)
	}

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()

	nowUTC := time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC)
	if err := TickReminders(ctx, bot, db, cfg, nowUTC, testLogger()); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}

	// Must NOT send the expired reminder.
	// Only the new current reminder for 2026-08-01 event will send.
	var status string
	if err := db.QueryRow(`SELECT status FROM notifications_log WHERE id = ?`, pendingLog.ID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != models.NotificationStatusSkipped {
		t.Errorf("old pending log status = %q, want skipped", status)
	}
}

// TestRecoverPending_WithinGraceSentSuccessfully verifies that a pending notification
// created right before a restart (within grace) is recovered and sent on next tick.
func TestRecoverPending_WithinGraceSentSuccessfully(t *testing.T) {
	db := testDB(t)
	offset := 1
	sendTime := "09:00"
	user, event := seedUserEventRule(t, db, 1, "Asia/Jerusalem", "09:00", &offset, &sendTime, nil)

	ctx := context.Background()
	notifRepo := repo.NewNotificationRepo(db)

	rules, err := notifRepo.GetRulesForUser(ctx, user.ID)
	if err != nil || len(rules) == 0 {
		t.Fatalf("GetRulesForUser: %v", err)
	}
	ruleID := rules[0].ID

	// Scheduled 10 minutes ago, stuck in pending due to simulated process interruption.
	occDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	schedAt := time.Date(2026, 7, 31, 6, 0, 0, 0, time.UTC)
	pendingLog, err := notifRepo.CreatePending(ctx, user.ID, event.ID, &ruleID, occDate, schedAt)
	if err != nil || pendingLog == nil {
		t.Fatalf("CreatePending: %v", err)
	}

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()

	nowUTC := time.Date(2026, 7, 31, 6, 10, 0, 0, time.UTC)
	if err := TickReminders(ctx, bot, db, cfg, nowUTC, testLogger()); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}

	if client.sendCount() != 1 {
		t.Fatalf("sendCount = %d, want 1 recovered send", client.sendCount())
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM notifications_log WHERE id = ?`, pendingLog.ID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != models.NotificationStatusSent {
		t.Errorf("pending log status = %q, want sent", status)
	}
}

// TestTickReminders_NegativeTimezoneWindow verifies that users in negative UTC
// timezones (e.g. America/Los_Angeles UTC-7) are correctly queried and processed
// even when nowUTC has crossed into the next calendar day.
func TestTickReminders_NegativeTimezoneWindow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	users := repo.NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 1, nil, "LAUser", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	user.Timezone = "America/Los_Angeles"
	user.DefaultNotifyTime = "19:00"
	if err := users.UpdateSettings(ctx, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	occ := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	year := 1992
	events := repo.NewEventRepo(db, 1)
	_, err = events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Alex", Category: "friends",
		CalendarType: "gregorian", Month: 8, Day: 31, Year: &year,
		NextOccurrence: &occ, IsActive: true,
	})
	if err != nil {
		t.Fatalf("Create event: %v", err)
	}

	rules := repo.NewReminderRuleRepo(db, 1)
	offsetZero := 0
	sendTime := "19:00"
	if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &offsetZero, SendTime: &sendTime, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()
	logger := testLogger()

	// 19:01 local in LA on 2026-08-31 is 2026-09-01 02:01 UTC.
	// nowUTC is 2026-09-01, but event occurrence is 2026-08-31.
	nowUTC := time.Date(2026, 9, 1, 2, 1, 0, 0, time.UTC)
	if err := TickReminders(ctx, bot, db, cfg, nowUTC, logger); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount = %d, want 1", client.sendCount())
	}
}

// TestTickReminders_DualDateEventFiresBothTracksIndependently covers the
// dual hebrew/gregorian dates feature: a hebrew-primary event with a
// gregorian secondary date must fire a separate reminder for each track on
// its own occurrence day, and neither track's dedup should suppress the
// other's send even though they share one global rule.
func TestTickReminders_DualDateEventFiresBothTracksIndependently(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	users := repo.NewUserRepo(db)
	user, _, err := users.GetOrCreate(ctx, 1, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	user.Timezone = "Asia/Jerusalem"
	user.DefaultNotifyTime = "09:00"
	if err := users.UpdateSettings(ctx, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	primaryOcc := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	secondaryOcc := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	secondaryCalendarType := "gregorian"
	secMonth, secDay := 8, 10
	events := repo.NewEventRepo(db, 1)
	_, err = events.Create(ctx, &models.Event{
		EventType: "birthday", FirstName: "Dana", Category: "other",
		CalendarType: "hebrew", Month: 5, Day: 1,
		NextOccurrence:          &primaryOcc,
		SecondaryCalendarType:   &secondaryCalendarType,
		SecondaryMonth:          &secMonth,
		SecondaryDay:            &secDay,
		SecondaryNextOccurrence: &secondaryOcc,
		IsActive:                true,
	})
	if err != nil {
		t.Fatalf("Create dual-date event: %v", err)
	}

	rules := repo.NewReminderRuleRepo(db, 1)
	offsetZero := 0
	sendTime := "09:00"
	if _, err := rules.Create(ctx, &models.ReminderRule{OffsetDays: &offsetZero, SendTime: &sendTime, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := testConfig()
	logger := testLogger()

	// Primary occurrence day at 09:01 local (06:01 UTC): only the primary track fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 1, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick on primary day: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount after primary day = %d, want 1", client.sendCount())
	}

	// Same day, later tick: must not resend the primary track.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 1, 7, 0, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick later same day: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount after re-tick on primary day = %d, want still 1", client.sendCount())
	}

	// Secondary occurrence day at 09:01 local (06:01 UTC): the secondary
	// track fires too, independently of the primary track's own send.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 10, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick on secondary day: %v", err)
	}
	if client.sendCount() != 2 {
		t.Fatalf("sendCount after secondary day = %d, want 2", client.sendCount())
	}

	// Same secondary day, later tick: must not resend the secondary track either.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 8, 10, 7, 0, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("Tick later on secondary day: %v", err)
	}
	if client.sendCount() != 2 {
		t.Fatalf("sendCount after re-tick on secondary day = %d, want still 2", client.sendCount())
	}
}
