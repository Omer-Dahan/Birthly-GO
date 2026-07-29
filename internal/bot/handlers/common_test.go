package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// fakeEditClient intercepts editMessageText calls so EditOrIgnore can be
// tested without touching the network. editErr controls what the call
// returns; every call is recorded for assertions.
type fakeEditClient struct {
	editErr   error
	editCalls int
}

func (f *fakeEditClient) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	if method != "editMessageText" {
		return json.RawMessage(`true`), nil
	}
	f.editCalls++
	if f.editErr != nil {
		return nil, f.editErr
	}
	return json.Marshal(gotgbot.Message{MessageId: 1, Date: int64(time.Now().Unix())})
}

func (f *fakeEditClient) GetAPIURL(opts *gotgbot.RequestOpts) string {
	return "https://example.invalid"
}
func (f *fakeEditClient) FileURL(token, path string, opts *gotgbot.RequestOpts) string {
	return "https://example.invalid/" + path
}

func testEditBot(client *fakeEditClient) *gotgbot.Bot {
	return &gotgbot.Bot{Token: "test-token", BotClient: client}
}

func editableCallback() *gotgbot.CallbackQuery {
	return &gotgbot.CallbackQuery{
		Id:      "cq1",
		Message: gotgbot.Message{MessageId: 42, Chat: gotgbot.Chat{Id: 555}},
	}
}

func TestEditOrIgnore_SwallowsMessageNotModified(t *testing.T) {
	client := &fakeEditClient{editErr: &gotgbot.TelegramError{Description: "Bad Request: message is not modified"}}
	bot := testEditBot(client)

	err := EditOrIgnore(bot, editableCallback(), "same text", nil)
	if err != nil {
		t.Errorf("EditOrIgnore should swallow 'message is not modified', got %v", err)
	}
	if client.editCalls != 1 {
		t.Errorf("editCalls = %d, want 1", client.editCalls)
	}
}

func TestEditOrIgnore_ReraisesOtherTelegramErrors(t *testing.T) {
	client := &fakeEditClient{editErr: &gotgbot.TelegramError{Description: "Bad Request: chat not found"}}
	bot := testEditBot(client)

	err := EditOrIgnore(bot, editableCallback(), "text", nil)
	if err == nil {
		t.Error("EditOrIgnore should re-raise a TelegramError other than 'message is not modified'")
	}
}

func TestEditOrIgnore_ReraisesNonTelegramErrors(t *testing.T) {
	client := &fakeEditClient{editErr: errors.New("network blip")}
	bot := testEditBot(client)

	err := EditOrIgnore(bot, editableCallback(), "text", nil)
	if err == nil {
		t.Error("EditOrIgnore should re-raise a non-TelegramError")
	}
}

func TestEditOrIgnore_CallsEditNormallyOnSuccess(t *testing.T) {
	client := &fakeEditClient{}
	bot := testEditBot(client)

	if err := EditOrIgnore(bot, editableCallback(), "new text", nil); err != nil {
		t.Errorf("EditOrIgnore unexpected error: %v", err)
	}
	if client.editCalls != 1 {
		t.Errorf("editCalls = %d, want 1", client.editCalls)
	}
}

func TestEditOrIgnore_NoOpWhenMessageInaccessible(t *testing.T) {
	client := &fakeEditClient{}
	bot := testEditBot(client)

	// An InaccessibleMessage (or any non-gotgbot.Message) means the type
	// assertion in EditOrIgnore fails — it must no-op, not panic or error.
	cq := &gotgbot.CallbackQuery{Id: "cq2", Message: gotgbot.InaccessibleMessage{Chat: gotgbot.Chat{Id: 555}}}

	if err := EditOrIgnore(bot, cq, "text", nil); err != nil {
		t.Errorf("EditOrIgnore with inaccessible message should no-op, got error %v", err)
	}
	if client.editCalls != 0 {
		t.Errorf("editCalls = %d, want 0 (should not call the API at all)", client.editCalls)
	}
}
