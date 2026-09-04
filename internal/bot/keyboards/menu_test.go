package keyboards

import (
	"testing"
)

func TestHomeKeyboard_Hebrew(t *testing.T) {
	kb := HomeKeyboard("he")
	if len(kb.InlineKeyboard) != 4 {
		t.Fatalf("HomeKeyboard row count = %d, want 4", len(kb.InlineKeyboard))
	}
	for i, row := range kb.InlineKeyboard {
		if len(row) != 2 {
			t.Errorf("row %d has %d buttons, want 2", i, len(row))
		}
	}

	channelBtn := kb.InlineKeyboard[3][1]
	wantText := "📣 הערוץ שלנו"
	if channelBtn.Text != wantText {
		t.Errorf("channel button text = %q, want %q", channelBtn.Text, wantText)
	}
	if channelBtn.Url != channelURL {
		t.Errorf("channel button url = %q, want %q", channelBtn.Url, channelURL)
	}
	if channelBtn.CallbackData != "" {
		t.Errorf("channel button callback_data = %q, want empty", channelBtn.CallbackData)
	}
}

func TestHomeKeyboard_English(t *testing.T) {
	kb := HomeKeyboard("en")
	if len(kb.InlineKeyboard) != 4 {
		t.Fatalf("HomeKeyboard row count = %d, want 4", len(kb.InlineKeyboard))
	}

	channelBtn := kb.InlineKeyboard[3][1]
	wantText := "📣 Our Channel"
	if channelBtn.Text != wantText {
		t.Errorf("channel button text = %q, want %q", channelBtn.Text, wantText)
	}
	if channelBtn.Url != channelURL {
		t.Errorf("channel button url = %q, want %q", channelBtn.Url, channelURL)
	}
	if channelBtn.CallbackData != "" {
		t.Errorf("channel button callback_data = %q, want empty", channelBtn.CallbackData)
	}
}

func TestHelpKeyboard_ChannelButton(t *testing.T) {
	kb := HelpKeyboard("he", "test_bot")
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("HelpKeyboard row count = %d, want 2", len(kb.InlineKeyboard))
	}

	channelBtn := kb.InlineKeyboard[1][1]
	if channelBtn.Url != channelURL {
		t.Errorf("help channel button url = %q, want %q", channelBtn.Url, channelURL)
	}
}
