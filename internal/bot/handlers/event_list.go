package handlers

import (
	"context"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// RegisterEventList wires the S3 event list screen — port of
// app/handlers/event_list.py.
func RegisterEventList(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(menuActionFilter("list"), cbMenuList))
	dispatcher.AddHandler(handlers.NewCallback(listActionFilter("p"), cbListPage))
	dispatcher.AddHandler(handlers.NewCallback(listActionFilter("sort"), cbSortMenu))
	dispatcher.AddHandler(handlers.NewCallback(listActionFilter("filt"), cbFilterMenu))
	dispatcher.AddHandler(handlers.NewCallback(listActionFilter("fset"), cbListSet))
}

func listActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixList {
			return false
		}
		l, err := callbacks.DecodeList(cq.Data)
		return err == nil && l.Action == action
	}
}

func rowLabel(user *models.User, event *models.Event) string {
	today := services.UserToday(user)
	name := core.FormatName(event.FirstName, event.LastName)
	countdown := core.FormatCountdown(core.DaysUntil(*event.NextOccurrence, today))

	if age, ok := core.AgeAt(event.CalendarType, event.Year, *event.NextOccurrence); ok {
		return name + " — " + countdown + " (" + strconv.Itoa(age) + ")"
	}
	return name + " — " + countdown
}

func renderList(ctx context.Context, db repo.DBTX, user *models.User, page, pageSize int) (string, *gotgbot.InlineKeyboardMarkup, error) {
	sort := user.ListSort
	view := core.ListViewUpcoming
	if user.ListFilter != nil && *user.ListFilter != "" {
		view = *user.ListFilter
	}

	events, total, err := repo.NewEventRepo(db, user.ID).ListPage(ctx, sort, view, page, pageSize)
	if err != nil {
		return "", nil, err
	}

	lang := user.Language
	var text string
	if total == 0 {
		text = i18n.T("list.title", lang, map[string]any{"count": 0}) + "\n\n" + i18n.T("list.empty", lang, nil)
	} else {
		header := i18n.T("list.title", lang, map[string]any{"count": total})
		sortLine := i18n.T("list.sort_"+sort, lang, nil)
		filterKey := "list.filter_all"
		if view == core.ListViewMuted {
			filterKey = "list.filter_muted"
		}
		filterLine := i18n.T(filterKey, lang, nil)
		text = header + "\n" + sortLine + "  ·  " + filterLine
	}

	rowLabels := make([]keyboards.EventRowLabel, len(events))
	for i, e := range events {
		rowLabels[i] = keyboards.EventRowLabel{EventID: e.ID, Label: core.Esc(rowLabel(user, e))}
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	kb := keyboards.ListKeyboard(lang, rowLabels, page, totalPages)
	return text, kb, nil
}

func cbMenuList(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	pageSize := router.ConfigFromContext(ctx).PageSize

	text, kb, err := renderList(context.Background(), db, user, 0, pageSize)
	if err != nil {
		return err
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, kb); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbListPage(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	pageSize := router.ConfigFromContext(ctx).PageSize

	l, err := callbacks.DecodeList(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}
	page, err := strconv.Atoi(l.Value)
	if err != nil {
		page = 0
	}

	text, kb, err := renderList(context.Background(), db, user, page, pageSize)
	if err != nil {
		return err
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, kb); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbSortMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("list.sort_menu_title", user.Language, nil), keyboards.SortMenuKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbFilterMenu(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("list.filter_menu_title", user.Language, nil), keyboards.FilterMenuKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

var validSortValues = map[string]bool{core.ListSortUpcoming: true, core.ListSortName: true, core.ListSortAge: true, core.ListSortCreated: true}
var validFilterValues = map[string]bool{"all": true, core.ListViewMuted: true}

func cbListSet(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	pageSize := router.ConfigFromContext(ctx).PageSize

	l, err := callbacks.DecodeList(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}
	kind, value, _ := strings.Cut(l.Value, ":")

	switch {
	case kind == "sort" && validSortValues[value]:
		user.ListSort = value
	case kind == "filt" && validFilterValues[value]:
		if value == "all" {
			user.ListFilter = nil
		} else {
			v := value
			user.ListFilter = &v
		}
	default:
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	if err := services.UpdateSettings(context.Background(), db, user); err != nil {
		return err
	}

	text, kb, err := renderList(context.Background(), db, user, 0, pageSize)
	if err != nil {
		return err
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, kb); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("common.saved", user.Language, nil))
	return nil
}
