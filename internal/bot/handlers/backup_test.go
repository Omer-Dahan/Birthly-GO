package handlers

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
)

func TestBackupActionFilter(t *testing.T) {
	filter := backupActionFilter("exp")
	format := "json"
	data := callbacks.Backup{Action: "exp", Value: &format}.Encode()

	if !filter(&gotgbot.CallbackQuery{Data: data}) {
		t.Errorf("backupActionFilter(exp) should match %q", data)
	}
	if filter(&gotgbot.CallbackQuery{Data: callbacks.Backup{Action: "imp"}.Encode()}) {
		t.Error("backupActionFilter(exp) should not match an imp payload")
	}
	if filter(&gotgbot.CallbackQuery{Data: callbacks.Menu{Action: "add"}.Encode()}) {
		t.Error("backupActionFilter(exp) should not match a different prefix entirely")
	}
}

func TestHasDocumentFilter(t *testing.T) {
	if hasDocument(&gotgbot.Message{}) {
		t.Error("hasDocument should be false with no Document field")
	}
	if !hasDocument(&gotgbot.Message{Document: &gotgbot.Document{FileId: "abc"}}) {
		t.Error("hasDocument should be true when Document is set")
	}
}

func TestMaxImportBytesIsFiveMB(t *testing.T) {
	if maxImportBytes != 5*1024*1024 {
		t.Errorf("maxImportBytes = %d, want 5MB", maxImportBytes)
	}
}
