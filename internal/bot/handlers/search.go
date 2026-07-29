package handlers

import (
	"context"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/fsm"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
)

// RegisterSearch wires the S10 free-text search flow — port of
// app/handlers/search.py.
func RegisterSearch(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(menuActionFilter("srch"), cbMenuSearch))
	dispatcher.AddHandler(handlers.NewMessage(anyText, msgSearchQuery))
}

func searchAgainHomeRow(lang string) []gotgbot.InlineKeyboardButton {
	again := gotgbot.InlineKeyboardButton{Text: i18n.T("search.again", lang, nil), CallbackData: callbacks.Menu{Action: "srch"}.Encode()}
	return []gotgbot.InlineKeyboardButton{again, keyboards.HomeButton(i18n.T("common.home", lang, nil))}
}

func searchResultsKeyboard(user *models.User, events []*models.Event) *gotgbot.InlineKeyboardMarkup {
	rows := make([][]gotgbot.InlineKeyboardButton, 0, len(events)+1)
	for _, e := range events {
		btn := gotgbot.InlineKeyboardButton{
			Text:         core.Esc(rowLabel(user, e)),
			CallbackData: callbacks.EventCard{Action: "v", EventID: e.ID}.Encode(),
		}
		rows = append(rows, []gotgbot.InlineKeyboardButton{btn})
	}
	rows = append(rows, searchAgainHomeRow(user.Language))
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func cbMenuSearch(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	router.FSMFromContext(ctx).SetState(user.ID, fsm.SearchQuery)

	text := i18n.T("search.title", user.Language, nil) + "\n<i>" + i18n.T("search.hint", user.Language, nil) + "</i>"
	kb := keyboards.SingleRowKeyboard(keyboards.CancelButton(i18n.T("common.cancel", user.Language, nil)))
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, kb); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func msgSearchQuery(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.SearchQuery {
		return ext.ContinueGroups
	}
	db := router.DBFromContext(ctx)

	query := strings.TrimSpace(ctx.EffectiveMessage.Text)
	if query == "" {
		return nil
	}

	events, err := services.SearchEvents(context.Background(), db, user, query)
	if err != nil {
		return err
	}
	store.Clear(user.ID)

	lang := user.Language
	if len(events) == 0 {
		text := i18n.T("search.no_results", lang, map[string]any{"query": core.Esc(query)})
		kb := &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{searchAgainHomeRow(lang)}}
		_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: kb, ParseMode: "HTML"})
		return sendErr
	}

	text := i18n.T("search.results_title", lang, map[string]any{"count": len(events)})
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ReplyMarkup: searchResultsKeyboard(user, events), ParseMode: "HTML"})
	return sendErr
}
