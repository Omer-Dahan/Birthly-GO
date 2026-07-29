package keyboards

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"birthly/internal/bot/callbacks"
)

func btn(label string) gotgbot.InlineKeyboardButton {
	return gotgbot.InlineKeyboardButton{Text: label}
}

func TestBuildGrid(t *testing.T) {
	buttons := []gotgbot.InlineKeyboardButton{btn("a"), btn("b"), btn("c"), btn("d"), btn("e")}

	rows := BuildGrid(buttons, 2)
	if len(rows) != 3 {
		t.Fatalf("BuildGrid(5,cols=2) = %d rows, want 3", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 || len(rows[2]) != 1 {
		t.Errorf("BuildGrid(5,cols=2) row sizes = [%d,%d,%d], want [2,2,1]", len(rows[0]), len(rows[1]), len(rows[2]))
	}
	if rows[2][0].Text != "e" {
		t.Errorf("last row's button = %q, want e", rows[2][0].Text)
	}
}

func TestBuildGrid_ExactMultiple(t *testing.T) {
	buttons := []gotgbot.InlineKeyboardButton{btn("a"), btn("b"), btn("c"), btn("d")}
	rows := BuildGrid(buttons, 2)
	if len(rows) != 2 || len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("BuildGrid(4,cols=2) = %v, want 2 full rows of 2", rows)
	}
}

func TestBuildGrid_EmptyInput(t *testing.T) {
	rows := BuildGrid(nil, 2)
	if len(rows) != 0 {
		t.Errorf("BuildGrid(empty) = %d rows, want 0", len(rows))
	}
}

func TestBuildGrid_NonPositiveColumnsDefaultsToTwo(t *testing.T) {
	buttons := []gotgbot.InlineKeyboardButton{btn("a"), btn("b"), btn("c")}
	rows := BuildGrid(buttons, 0)
	if len(rows) != 2 || len(rows[0]) != 2 || len(rows[1]) != 1 {
		t.Errorf("BuildGrid(3,cols=0) = %v, want the columns=2 default layout", rows)
	}
}

func TestPageRow_FirstPage(t *testing.T) {
	row := PageRow(0, 3)
	if row[0].CallbackData != callbacks.EncodeNoop() {
		t.Errorf("first page's prev button = %q, want a noop (no page before 0)", row[0].CallbackData)
	}
	if row[1].Text != "1/3" {
		t.Errorf("page indicator = %q, want 1/3", row[1].Text)
	}
	wantNext := callbacks.List{Action: "p", Value: "1"}.Encode()
	if row[2].CallbackData != wantNext {
		t.Errorf("next button data = %q, want %q", row[2].CallbackData, wantNext)
	}
}

func TestPageRow_LastPage(t *testing.T) {
	row := PageRow(2, 3) // 0-indexed page 2 of 3 total = the last page
	wantPrev := callbacks.List{Action: "p", Value: "1"}.Encode()
	if row[0].CallbackData != wantPrev {
		t.Errorf("prev button data = %q, want %q", row[0].CallbackData, wantPrev)
	}
	if row[2].CallbackData != callbacks.EncodeNoop() {
		t.Errorf("last page's next button = %q, want a noop (no page after the last)", row[2].CallbackData)
	}
	if row[1].Text != "3/3" {
		t.Errorf("page indicator = %q, want 3/3", row[1].Text)
	}
}

func TestPageRow_ZeroTotalPagesDisplaysOne(t *testing.T) {
	row := PageRow(0, 0)
	if row[1].Text != "1/1" {
		t.Errorf("PageRow(0,0) indicator = %q, want 1/1 (never display a zero total)", row[1].Text)
	}
}

func TestSingleRowKeyboard(t *testing.T) {
	kb := SingleRowKeyboard(btn("a"), btn("b"))
	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 2 {
		t.Errorf("SingleRowKeyboard(2 buttons) = %v, want 1 row of 2", kb.InlineKeyboard)
	}
}
