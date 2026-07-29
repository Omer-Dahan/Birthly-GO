package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/fsm"
	"birthly/internal/bot/keyboards"
	"birthly/internal/bot/router"
	"birthly/internal/i18n"
	"birthly/internal/services"
	"birthly/internal/store/repo"
)

// RegisterBackup wires the S15 backup/export/import screens — port of
// app/handlers/backup.py.
func RegisterBackup(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCallback(backupActionFilter("home"), cbBackupHome))
	dispatcher.AddHandler(handlers.NewCallback(backupActionFilter("exp"), cbExport))
	dispatcher.AddHandler(handlers.NewCallback(backupActionFilter("imp"), cbImportStart))
	dispatcher.AddHandler(handlers.NewCallback(backupActionFilter("imp_do"), cbImportDo))
	dispatcher.AddHandler(handlers.NewCallback(backupActionFilter("auto"), cbAutoBackupToggle))
	dispatcher.AddHandler(handlers.NewMessage(hasDocument, msgImportFile))
}

const maxImportBytes = 5 * 1024 * 1024 // 5 MB

func backupActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixBackup {
			return false
		}
		bc, err := callbacks.DecodeBackup(cq.Data)
		return err == nil && bc.Action == action
	}
}

func hasDocument(msg *gotgbot.Message) bool { return msg.Document != nil }

func sendBackupHome(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	cfg := router.ConfigFromContext(ctx)

	count, err := repo.NewEventRepo(db, user.ID).CountNotDeleted(context.Background())
	if err != nil {
		return err
	}
	lang := user.Language
	text := "💾 <b>" + i18n.T("backup.home.title", lang, nil) + "</b>\n\n" +
		"📦 " + strconv.Itoa(count) + " " + i18n.T("backup.home.records", lang, nil) + "\n" +
		"🕐 " + i18n.T("backup.home.auto_last", lang, map[string]any{"time": cfg.AutoBackupTime})
	kb := keyboards.BackupHomeKeyboard(lang, cfg.AutoBackupEnabled)
	return EditOrIgnore(b, ctx.CallbackQuery, text, kb)
}

func cbBackupHome(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := sendBackupHome(b, ctx); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cbExport(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	bc, err := callbacks.DecodeBackup(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}
	format := ""
	if bc.Value != nil {
		format = *bc.Value
	}
	if format != "json" && format != "csv" && format != "xlsx" {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	AnswerCallback(b, ctx.CallbackQuery, i18n.T("backup.exporting", user.Language, nil))

	timestamp := time.Now().UTC().Format("20060102_1504")
	filename := "birthly_" + strconv.FormatInt(user.ID, 10) + "_" + timestamp + "." + format

	var data []byte
	switch format {
	case "json":
		data, err = services.ExportJSON(context.Background(), db, user)
	case "csv":
		data, err = services.ExportCSV(context.Background(), db, user)
	default:
		data, err = services.ExportXLSX(context.Background(), db, user)
	}
	if err != nil {
		_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.generic", user.Language, nil), nil)
		return sendErr
	}

	doc := &gotgbot.FileReader{Name: filename, Data: bytes.NewReader(data)}
	caption := i18n.T("backup.export_caption", user.Language, map[string]any{"fmt": strings.ToUpper(format)})
	if _, err := b.SendDocument(ctx.EffectiveChat.Id, doc, &gotgbot.SendDocumentOpts{Caption: caption}); err != nil {
		return err
	}
	if format == "json" {
		if _, err := b.SendMessage(ctx.EffectiveChat.Id, i18n.T("backup.export_no_photos", user.Language, nil), nil); err != nil {
			return err
		}
	}
	return nil
}

func cbImportStart(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	router.FSMFromContext(ctx).SetState(user.ID, fsm.ImportFlowWaitingFile)

	cancelKb := &gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{keyboards.CancelButton(i18n.T("common.cancel", user.Language, nil))}},
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, i18n.T("backup.import_send_file", user.Language, nil), cancelKb); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func msgImportFile(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.ImportFlowWaitingFile {
		return ext.ContinueGroups
	}

	doc := ctx.EffectiveMessage.Document
	if doc == nil {
		_, err := ctx.EffectiveMessage.Reply(b, i18n.T("backup.import_no_file", user.Language, nil), nil)
		return err
	}

	if doc.MimeType != "application/json" && doc.MimeType != "text/plain" && !strings.HasSuffix(doc.FileName, ".json") {
		_, err := ctx.EffectiveMessage.Reply(b, i18n.T("backup.import_wrong_format", user.Language, nil), nil)
		return err
	}
	if doc.FileSize > maxImportBytes {
		_, err := ctx.EffectiveMessage.Reply(b, i18n.T("backup.import_too_large", user.Language, map[string]any{"max_mb": 5}), nil)
		return err
	}

	file, err := b.GetFile(doc.FileId, nil)
	if err != nil {
		return err
	}
	data, err := downloadFile(file.URL(b, nil))
	if err != nil {
		return err
	}

	payload, result, err := services.ParseImportJSON(data)
	if err != nil {
		_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("backup.import_parse_error", user.Language, map[string]any{"error": err.Error()}), nil)
		store.Clear(user.ID)
		return sendErr
	}

	store.SetState(user.ID, fsm.ImportFlowConfirm)
	store.UpdateData(user.ID, map[string]any{"import_payload": payload})

	previewText := i18n.T("backup.import_preview.title", user.Language, nil) + "\n\n" +
		"📦 " + i18n.T("backup.import_preview.records", user.Language, map[string]any{"count": result.TotalParsed}) + "\n"
	if len(result.Errors) > 0 {
		previewText += "⚠️ " + i18n.T("backup.import_preview.errors", user.Language, map[string]any{"count": len(result.Errors)}) + "\n"
	}

	_, sendErr := ctx.EffectiveMessage.Reply(b, previewText, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.ImportModeKeyboard(user.Language)})
	return sendErr
}

func downloadFile(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:gosec // URL is Telegram's own file API, not user-controlled
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func cbImportDo(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	store := router.FSMFromContext(ctx)
	bc, err := callbacks.DecodeBackup(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}
	mode := ""
	if bc.Value != nil {
		mode = *bc.Value
	}
	if mode != "add" && mode != "replace" && mode != "replace_confirmed" {
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	if mode == "replace" {
		text := i18n.T("backup.import_replace_warning", user.Language, nil)
		if err := EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.ImportReplaceConfirmKeyboard(user.Language)); err != nil {
			return err
		}
		AnswerCallback(b, ctx.CallbackQuery, "")
		return nil
	}

	data := store.GetData(user.ID)
	payload, ok := data["import_payload"].(map[string]any)
	if !ok {
		AnswerCallbackAlert(b, ctx.CallbackQuery, i18n.T("error.generic", user.Language, nil))
		store.Clear(user.ID)
		return nil
	}

	AnswerCallback(b, ctx.CallbackQuery, i18n.T("backup.importing", user.Language, nil))

	actualMode := "add"
	if mode == "replace_confirmed" {
		actualMode = "replace"
	}
	result, err := services.DoImport(context.Background(), db, user, payload, actualMode)
	if err != nil {
		store.Clear(user.ID)
		_, sendErr := ctx.EffectiveMessage.Reply(b, i18n.T("error.generic", user.Language, nil), nil)
		return sendErr
	}
	store.Clear(user.ID)

	summary := "✅ " + i18n.T("backup.import_done.title", user.Language, nil) + "\n\n" +
		"📦 " + i18n.T("backup.import_done.imported", user.Language, map[string]any{"count": result.Imported}) + "\n" +
		"🔁 " + i18n.T("backup.import_done.duplicates", user.Language, map[string]any{"count": result.Duplicates}) + "\n"
	if len(result.Errors) > 0 {
		limit := len(result.Errors)
		if limit > 5 {
			limit = 5
		}
		errorRows := strings.Join(result.Errors[:limit], ", ")
		summary += "⚠️ " + i18n.T("backup.import_done.errors", user.Language, map[string]any{"count": len(result.Errors), "rows": errorRows}) + "\n"
	}

	_, sendErr := ctx.EffectiveMessage.Reply(b, summary, nil)
	return sendErr
}

// cbAutoBackupToggle: auto-backup is a global server setting, not per-user
// (SPEC.md — settings is a global object). Matches Python's own handler,
// which only acknowledges and re-renders the home screen with the current
// (unchanged) state — the toggle isn't actually persisted anywhere.
func cbAutoBackupToggle(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	bc, err := callbacks.DecodeBackup(ctx.CallbackQuery.Data)
	if err != nil {
		return err
	}
	enabled := bc.Value != nil && *bc.Value == "1"
	state := "common.no"
	if enabled {
		state = "common.yes"
	}
	AnswerCallback(b, ctx.CallbackQuery, i18n.T("backup.auto_toggled", user.Language, map[string]any{"state": i18n.T(state, user.Language, nil)}))
	return sendBackupHome(b, ctx)
}
