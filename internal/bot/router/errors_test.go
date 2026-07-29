package router

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/config"
)

// fakeBotClient intercepts every gotgbot API call so tests never touch the
// network — same shape as the fake used in internal/scheduler/jobs_test.go.
type fakeBotClient struct {
	mu    sync.Mutex
	calls []map[string]any
}

func (f *fakeBotClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, map[string]any{"method": method, "params": params})
	f.mu.Unlock()
	if method == "sendMessage" {
		return json.Marshal(gotgbot.Message{MessageId: 1, Date: int64(time.Now().Unix())})
	}
	return json.RawMessage(`true`), nil
}

func (f *fakeBotClient) GetAPIURL(opts *gotgbot.RequestOpts) string { return "https://example.invalid" }
func (f *fakeBotClient) FileURL(token, path string, opts *gotgbot.RequestOpts) string {
	return "https://example.invalid/" + path
}

func (f *fakeBotClient) callsFor(method string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, c := range f.calls {
		if c["method"] == method {
			out = append(out, c)
		}
	}
	return out
}

func testBot(client *fakeBotClient) *gotgbot.Bot {
	return &gotgbot.Bot{Token: "test-token", BotClient: client}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestShortErrorID_Format(t *testing.T) {
	id := shortErrorID()
	if len(id) != 4 {
		t.Fatalf("shortErrorID() = %q, want length 4", id)
	}
	for _, r := range id {
		isHexUpper := (r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')
		if !isHexUpper {
			t.Fatalf("shortErrorID() = %q, want uppercase hex only", id)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Errorf("truncateRunes(short) = %q, want unchanged", got)
	}
	if got := truncateRunes("hello world", 5); got != "hello" {
		t.Errorf("truncateRunes(long) = %q, want %q", got, "hello")
	}
	// Hebrew text is multi-byte in UTF-8; truncation must count runes, not
	// bytes, or it would split a character mid-encoding.
	hebrew := "שגיאה חדשה במערכת"
	if got := truncateRunes(hebrew, 3); got != "שגי" {
		t.Errorf("truncateRunes(hebrew,3) = %q, want %q", got, "שגי")
	}
}

func TestNotifyAdmins_DedupedWithinWindow(t *testing.T) {
	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := &config.Config{ReportErrorsToAdmin: true, AdminIDs: []int64{111, 222}}
	r := newErrorReporter(cfg, testLogger())

	r.notifyAdmins(bot, "AAAA", "SomeError:boom", errors.New("boom"))
	if got := len(client.callsFor("sendMessage")); got != 2 {
		t.Fatalf("first notifyAdmins: got %d sendMessage calls, want 2 (one per admin)", got)
	}

	r.notifyAdmins(bot, "BBBB", "SomeError:boom", errors.New("boom again"))
	if got := len(client.callsFor("sendMessage")); got != 2 {
		t.Fatalf("second notifyAdmins within dedup window: got %d sendMessage calls, want still 2 (deduped)", got)
	}
}

func TestNotifyAdmins_DisabledOrNoAdmins(t *testing.T) {
	client := &fakeBotClient{}
	bot := testBot(client)

	r := newErrorReporter(&config.Config{ReportErrorsToAdmin: false, AdminIDs: []int64{111}}, testLogger())
	r.notifyAdmins(bot, "AAAA", "Err:x", errors.New("x"))

	r2 := newErrorReporter(&config.Config{ReportErrorsToAdmin: true, AdminIDs: nil}, testLogger())
	r2.notifyAdmins(bot, "AAAA", "Err:x", errors.New("x"))

	if got := len(client.callsFor("sendMessage")); got != 0 {
		t.Fatalf("expected no admin notifications, got %d", got)
	}
}

func TestErrorReporter_Handle_NotifiesUserFromMessage(t *testing.T) {
	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := &config.Config{}
	r := newErrorReporter(cfg, testLogger())

	update := &gotgbot.Update{Message: &gotgbot.Message{Chat: gotgbot.Chat{Id: 555}}}
	ctx := &ext.Context{Update: update, Data: map[string]any{}}

	action := r.handle(bot, ctx, errors.New("boom"))
	if action != ext.DispatcherActionNoop {
		t.Errorf("handle() action = %v, want DispatcherActionNoop", action)
	}

	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("got %d sendMessage calls, want 1", len(calls))
	}
	if chatID := calls[0]["params"].(map[string]any)["chat_id"]; chatID != int64(555) {
		t.Errorf("notified chat_id = %v, want 555", chatID)
	}
}

func TestErrorReporter_Handle_AnswersCallbackAndNotifiesUser(t *testing.T) {
	client := &fakeBotClient{}
	bot := testBot(client)
	cfg := &config.Config{}
	r := newErrorReporter(cfg, testLogger())

	cq := &gotgbot.CallbackQuery{Id: "cq1", Message: gotgbot.Message{Chat: gotgbot.Chat{Id: 777}}}
	update := &gotgbot.Update{CallbackQuery: cq}
	ctx := &ext.Context{Update: update, Data: map[string]any{}}

	r.handle(bot, ctx, errors.New("boom"))

	if got := len(client.callsFor("answerCallbackQuery")); got != 1 {
		t.Errorf("got %d answerCallbackQuery calls, want 1", got)
	}
	if got := len(client.callsFor("sendMessage")); got != 1 {
		t.Errorf("got %d sendMessage calls, want 1", got)
	}
}
