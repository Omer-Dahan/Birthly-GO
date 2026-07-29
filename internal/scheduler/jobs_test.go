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

	if method != "sendMessage" {
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
		if c["method"] == "sendMessage" {
			n++
		}
	}
	return n
}

func (f *fakeBotClient) lastChatID() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i]["method"] != "sendMessage" {
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

	// Same fire time as TestTickReminders_FiresExactlyOnce, which sends —
	// the only difference here is notifications_enabled=false.
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

	// Fire time is 06:00 UTC; resume at 09:00 UTC — 3h late, within 6h grace.
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

	// Fire time is 06:00 UTC; resume at 16:00 UTC — 10h late, outside 6h grace.
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

	// 06:01 UTC = 09:01 Israel time, 02:01 NY time — only IL user fires.
	if err := TickReminders(ctx, bot, db, cfg, time.Date(2026, 7, 31, 6, 1, 0, 0, time.UTC), logger); err != nil {
		t.Fatalf("TickReminders: %v", err)
	}
	if client.sendCount() != 1 {
		t.Fatalf("sendCount = %d, want 1", client.sendCount())
	}
	if client.lastChatID() != 1 {
		t.Errorf("lastChatID = %d, want 1 (IL user)", client.lastChatID())
	}

	// 13:01 UTC = 09:01 NY time — NY user now fires too.
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
