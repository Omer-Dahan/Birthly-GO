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
	"birthly/internal/bot/fsm"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/core"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// RegisterTemplates wires the S14 greeting/template flow — port of
// app/handlers/templates.py.
func RegisterTemplates(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(eventCardActionFilter("gr"), cbGreetingStyle))
	dispatcher.AddHandler(handlers.NewCallback(templateActionFilter("style"), cbBackToStyle))
	dispatcher.AddHandler(handlers.NewCallback(templateActionFilter("pick"), cbGreetingPick))
	dispatcher.AddHandler(handlers.NewCallback(templateActionFilter("ai"), cbGreetingAI))
	dispatcher.AddHandler(handlers.NewCallback(templateActionFilter("list"), cbListPersonal))
	dispatcher.AddHandler(handlers.NewCallback(templateActionFilter("use"), cbUsePersonalTemplate))
	dispatcher.AddHandler(handlers.NewCallback(templateActionFilter("new"), cbTemplateNew))
	dispatcher.AddHandler(handlers.NewCallback(templateActionFilter("new_tone"), cbTemplateNewTone))
	dispatcher.AddHandler(handlers.NewCallback(templateActionFilter("del"), cbTemplateDelete))
	dispatcher.AddHandler(handlers.NewMessage(anyText, msgTemplateBody))
}

var validTones = map[string]bool{"warm": true, "funny": true, "formal": true, "short": true}

func templateActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixTemplate {
			return false
		}
		tc, err := callbacks.DecodeTemplate(cq.Data)
		return err == nil && tc.Action == action
	}
}

func decodeTemplateCB(ctx *ext.Context) (callbacks.Template, error) {
	return callbacks.DecodeTemplate(ctx.CallbackQuery.Data)
}

func templatePreview(body string) string {
	runes := []rune(body)
	if len(runes) <= 40 {
		return body
	}
	return string(runes[:40]) + "…"
}

func renderPersonalList(ctx context.Context, b *gotgbot.Bot, cq *gotgbot.CallbackQuery, db repo.DBTX, user *models.User, eventID int64) error {
	list, err := repo.NewTemplateRepo(db, user.ID).ListUserTemplates(ctx)
	if err != nil {
		return err
	}
	rows := make([]keyboards.TemplateRow, len(list))
	for i, tpl := range list {
		rows[i] = keyboards.TemplateRow{ID: tpl.ID, Preview: templatePreview(tpl.Body)}
	}

	text := i18n.T("greeting.new.personal_title", user.Language, nil)
	if len(rows) == 0 {
		text = i18n.T("greeting.new.no_personal", user.Language, nil)
	}
	return EditOrIgnore(b, cq, text, keyboards.PersonalTemplatesKeyboard(user.Language, eventID, rows))
}

func cbGreetingStyle(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	eventID := mustEventID(ctx)

	event, err := services.GetOwnedEvent(context.Background(), db, user, eventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	if event.EventType == core.EventTypeMemorial {
		AnswerCallbackAlert(b, ctx.CallbackQuery, i18n.T("greeting.no_memorial", user.Language, nil))
		return nil
	}

	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	text := i18n.T("greeting.style.title", user.Language, map[string]any{"name": name})
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.GreetingStyleKeyboard(user.Language, eventID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbBackToStyle(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	tc, err := decodeTemplateCB(ctx)
	if err != nil {
		return err
	}
	if tc.EventID == nil {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	event, err := services.GetOwnedEvent(context.Background(), db, user, *tc.EventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}

	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	text := i18n.T("greeting.style.title", user.Language, map[string]any{"name": name})
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.GreetingStyleKeyboard(user.Language, *tc.EventID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func renderGreetingResult(b *gotgbot.Bot, cq *gotgbot.CallbackQuery, user *models.User, event *models.Event, tpl *models.GreetingTemplate, botUsername string) error {
	greetingText := services.RenderTemplate(tpl, event, user)
	name := core.Esc(core.FormatName(event.FirstName, event.LastName))
	body := "💌 " + i18n.T("greeting.result.title", user.Language, map[string]any{"name": name}) +
		"\n\n<code>" + core.Esc(greetingText) + "</code>\n\n<i>" + i18n.T("greeting.result.hint", user.Language, nil) + "</i>"
	kb := keyboards.GreetingResultKeyboard(user.Language, event.ID, tpl.Tone, tpl.ID, greetingText, botUsername)
	return EditOrIgnore(b, cq, body, kb)
}

func cbGreetingPick(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	tc, err := decodeTemplateCB(ctx)
	if err != nil {
		return err
	}
	if tc.EventID == nil || tc.Value == nil {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}
	tone := *tc.Value
	if !validTones[tone] {
		AnswerCallbackAlert(b, ctx.CallbackQuery, i18n.T("error.generic", user.Language, nil))
		return nil
	}

	event, err := services.GetOwnedEvent(context.Background(), db, user, *tc.EventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}

	tpl, err := services.PickTemplate(context.Background(), db, user, event, tone, tc.ExcludeID)
	if err != nil {
		return err
	}
	if tpl == nil {
		AnswerCallbackAlert(b, ctx.CallbackQuery, i18n.T("greeting.no_templates", user.Language, nil))
		return nil
	}

	botUsername := router.ConfigFromContext(ctx).BotUsername
	if err := renderGreetingResult(b, ctx.CallbackQuery, user, event, tpl, botUsername); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbGreetingAI(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	tc, err := decodeTemplateCB(ctx)
	if err != nil {
		return err
	}
	if tc.EventID == nil {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	event, err := services.GetOwnedEvent(context.Background(), db, user, *tc.EventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}

	prompt := services.BuildAIPrompt(event, user, "warm")
	text := "🤖 " + i18n.T("greeting.ai.title", user.Language, nil) + "\n\n<code>" + core.Esc(prompt) + "</code>\n\n<i>" + i18n.T("greeting.ai.hint", user.Language, nil) + "</i>"
	if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.AIPromptKeyboard(user.Language, *tc.EventID)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbListPersonal(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	tc, err := decodeTemplateCB(ctx)
	if err != nil {
		return err
	}
	if tc.EventID == nil {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}
	if _, err := services.GetOwnedEvent(context.Background(), db, user, *tc.EventID); err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	if err := renderPersonalList(context.Background(), b, ctx.CallbackQuery, db, user, *tc.EventID); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbUsePersonalTemplate(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	tc, err := decodeTemplateCB(ctx)
	if err != nil {
		return err
	}
	if tc.EventID == nil || tc.Value == nil {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}
	tplID, _ := strconv.ParseInt(*tc.Value, 10, 64)

	tpl, err := repo.NewTemplateRepo(db, user.ID).GetOwned(context.Background(), tplID)
	if err != nil {
		return err
	}
	if tpl == nil {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}

	event, err := services.GetOwnedEvent(context.Background(), db, user, *tc.EventID)
	if err != nil {
		if isNotFound(err) {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}

	botUsername := router.ConfigFromContext(ctx).BotUsername
	if err := renderGreetingResult(b, ctx.CallbackQuery, user, event, tpl, botUsername); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbTemplateNew(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	tc, err := decodeTemplateCB(ctx)
	if err != nil {
		return err
	}
	if tc.EventID == nil {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}
	router.FSMFromContext(ctx).UpdateData(user.ID, map[string]any{"new_event_id": *tc.EventID})

	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("greeting.new.pick_tone", user.Language, nil), keyboards.TemplateNewKeyboard(user.Language)); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbTemplateNewTone(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	tc, err := decodeTemplateCB(ctx)
	if err != nil {
		return err
	}
	if tc.Value == nil || !validTones[*tc.Value] {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	store := router.FSMFromContext(ctx)
	store.SetState(user.ID, fsm.TemplateFlowBody)
	store.UpdateData(user.ID, map[string]any{"new_tone": *tc.Value})

	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("greeting.new.body_prompt", user.Language, nil), nil); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func msgTemplateBody(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.TemplateFlowBody {
		return ext.ContinueGroups
	}
	db := router.DBFromContext(ctx)

	text := strings.TrimSpace(ctx.EffectiveMessage.Text)
	if text == "" {
		_, err := ctx.EffectiveMessage.Reply(b, i18n.T("greeting.new.body_empty", user.Language, nil), nil)
		return err
	}

	data := store.GetData(user.ID)
	tone, _ := data["new_tone"].(string)
	if tone == "" {
		tone = "warm"
	}
	eventID, hasEventID := data["new_event_id"].(int64)

	_, err := services.CreateUserTemplate(context.Background(), db, user, core.EventTypeBirthday, tone, nil, text)
	if err != nil {
		if _, ok := err.(*services.LimitErr); ok {
			store.Clear(user.ID)
			_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.limit_reached", user.Language, map[string]any{"max": core.MaxGreetingTemplatesPerUser}), nil)
			return sendErr
		}
		return err
	}

	store.Clear(user.ID)
	textOut := i18n.T("greeting.new.saved", user.Language, nil)
	if hasEventID {
		list, err := repo.NewTemplateRepo(db, user.ID).ListUserTemplates(context.Background())
		if err != nil {
			return err
		}
		rows := make([]keyboards.TemplateRow, len(list))
		for i, tpl := range list {
			rows[i] = keyboards.TemplateRow{ID: tpl.ID, Preview: templatePreview(tpl.Body)}
		}
		_, sendErr := ctx.EffectiveMessage.Reply(b, textOut, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.PersonalTemplatesKeyboard(user.Language, eventID, rows)})
		return sendErr
	}
	_, sendErr := ctx.EffectiveMessage.Reply(b, textOut, nil)
	return sendErr
}

func cbTemplateDelete(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	tc, err := decodeTemplateCB(ctx)
	if err != nil {
		return err
	}
	if tc.Value == nil {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}
	tplID, err := strconv.ParseInt(*tc.Value, 10, 64)
	if err != nil {
		NotFoundAlert(b, ctx.CallbackQuery, user.Language)
		return nil
	}

	if err := services.DeleteUserTemplate(context.Background(), db, user, tplID); err != nil {
		if _, ok := err.(*services.NotFoundErr); ok {
			NotFoundAlert(b, ctx.CallbackQuery, user.Language)
			return nil
		}
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("common.deleted", user.Language, nil))

	if tc.EventID != nil {
		return renderPersonalList(context.Background(), b, ctx.CallbackQuery, db, user, *tc.EventID)
	}
	return nil
}
