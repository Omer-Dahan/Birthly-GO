package handlers

import (
	"bytes"
	"context"
	"fmt"
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
	"birthly/internal/services"
)

// RegisterAdmin wires the admin panel — port of app/handlers/admin.py.
// Every handler is admin-gated: Python restricts the whole router with a
// Filter subclass (router.message.filter(IsAdminFilter())); gotgbot has no
// router-level filter concept, so requireAdmin wraps each Response and
// returns ext.ContinueGroups for a non-admin caller — letting the update
// fall through to other handlers, exactly like Python's filter causing this
// router's handlers to simply not match.
//
// Text here is hardcoded Hebrew throughout, matching Python's admin.py
// exactly — this screen was never wired into i18n in the original either.
func RegisterAdmin(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCommand("admin", requireAdminMsg(cmdAdmin)))
	dispatcher.AddHandler(handlers.NewCallback(adminActionFilter("home"), requireAdminCB(cbAdminHome)))

	dispatcher.AddHandler(handlers.NewCallback(adminActionFilter("stats"), requireAdminCB(cbAdminStatsCallback)))
	dispatcher.AddHandler(handlers.NewCommand("dbstats", requireAdminMsg(cmdDbstats)))

	dispatcher.AddHandler(handlers.NewCallback(adminActionFilter("bc"), requireAdminCB(cbAdminBcCallback)))
	dispatcher.AddHandler(handlers.NewCommand("broadcast", requireAdminMsg(cmdBroadcast)))
	dispatcher.AddHandler(handlers.NewMessage(anyText, requireAdminMsgCtx(msgBcText)))
	dispatcher.AddHandler(handlers.NewCallback(adminActionFilter("bc_ok"), requireAdminCB(cbAdminBcOk)))

	dispatcher.AddHandler(handlers.NewCallback(adminActionFilter("logs"), requireAdminCB(cbAdminLogsCallback)))
	dispatcher.AddHandler(handlers.NewCommand("logs", requireAdminMsg(cmdLogs)))

	dispatcher.AddHandler(handlers.NewCallback(adminActionFilter("u"), requireAdminCB(cbAdminUserSearchStart)))
	dispatcher.AddHandler(handlers.NewCommand("userinfo", requireAdminMsg(cmdUserinfo)))
	dispatcher.AddHandler(handlers.NewCommand("block", requireAdminMsg(cmdBlock)))
	dispatcher.AddHandler(handlers.NewCommand("unblock", requireAdminMsg(cmdUnblock)))

	dispatcher.AddHandler(handlers.NewCallback(adminActionFilter("backup"), requireAdminCB(cbAdminBackupCallback)))
	dispatcher.AddHandler(handlers.NewCommand("forcebackup", requireAdminMsg(cmdForcebackup)))
}

func adminActionFilter(action string) filters.CallbackQuery {
	return func(cq *gotgbot.CallbackQuery) bool {
		if callbacks.Prefix(cq.Data) != callbacks.PrefixAdmin {
			return false
		}
		a, err := callbacks.DecodeAdmin(cq.Data)
		return err == nil && a.Action == action
	}
}

func requireAdminMsg(fn func(b *gotgbot.Bot, ctx *ext.Context) error) handlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if user := User(ctx); user == nil || !user.IsAdmin {
			return ext.ContinueGroups
		}
		return fn(b, ctx)
	}
}

// requireAdminMsgCtx is requireAdminMsg for handlers that also need to fall
// through on their own (FSM state mismatch) — same signature, kept as a
// distinct name only for readability at call sites.
func requireAdminMsgCtx(fn func(b *gotgbot.Bot, ctx *ext.Context) error) handlers.Response {
	return requireAdminMsg(fn)
}

func requireAdminCB(fn func(b *gotgbot.Bot, ctx *ext.Context) error) handlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if user := User(ctx); user == nil || !user.IsAdmin {
			return ext.ContinueGroups
		}
		return fn(b, ctx)
	}
}

func adminHomeText(stats *services.SystemStats) string {
	return fmt.Sprintf(
		"🛠 <b>ניהול</b>\n\n"+
			"👥 משתמשים: %d  (פעילים 30 יום: %d)\n"+
			"🎂 אירועים: %d\n"+
			"🔔 תזכורות נשלחו היום: %d  (נכשלו: %d)\n"+
			"💾 DB: %.2f MB  ·  גיבוי אחרון: %s\n"+
			"⏱ טיק אחרון: %s\n",
		stats.TotalUsers, stats.ActiveUsers, stats.TotalEvents,
		stats.SentToday, stats.FailedToday, stats.DBSizeMB, stats.LastBackup, stats.LastTickAt,
	)
}

func cmdAdmin(b *gotgbot.Bot, ctx *ext.Context) error {
	db := router.DBFromContext(ctx)
	cfg := router.ConfigFromContext(ctx)
	stats, err := services.GetSystemStats(context.Background(), db, cfg.DBPath)
	if err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, adminHomeText(stats), &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.AdminHomeKeyboard(), ParseMode: "HTML"})
	return err
}

func cbAdminHome(b *gotgbot.Bot, ctx *ext.Context) error {
	router.FSMFromContext(ctx).Clear(ctx.EffectiveUser.Id)
	db := router.DBFromContext(ctx)
	cfg := router.ConfigFromContext(ctx)
	stats, err := services.GetSystemStats(context.Background(), db, cfg.DBPath)
	if err != nil {
		return err
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, adminHomeText(stats), keyboards.AdminHomeKeyboard()); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func adminStatsFullText(stats *services.SystemStats) string {
	lines := []string{
		"total_users: " + strconv.Itoa(stats.TotalUsers),
		"active_users: " + strconv.Itoa(stats.ActiveUsers),
		"total_events: " + strconv.Itoa(stats.TotalEvents),
		"sent_today: " + strconv.Itoa(stats.SentToday),
		"failed_today: " + strconv.Itoa(stats.FailedToday),
		"db_size_mb: " + strconv.FormatFloat(stats.DBSizeMB, 'f', 2, 64),
		"last_backup: " + stats.LastBackup,
		"last_tick_at: " + stats.LastTickAt,
	}
	return "📊 <b>סטטיסטיקות מתקדמות</b>\n\n" + strings.Join(lines, "\n")
}

func cbAdminStatsCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	db := router.DBFromContext(ctx)
	cfg := router.ConfigFromContext(ctx)
	stats, err := services.GetSystemStats(context.Background(), db, cfg.DBPath)
	if err != nil {
		return err
	}
	if err := EditOrIgnore(b, ctx.CallbackQuery, adminStatsFullText(stats), keyboards.BackToAdminKeyboard()); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cmdDbstats(b *gotgbot.Bot, ctx *ext.Context) error {
	db := router.DBFromContext(ctx)
	cfg := router.ConfigFromContext(ctx)
	stats, err := services.GetSystemStats(context.Background(), db, cfg.DBPath)
	if err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, adminStatsFullText(stats), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

const broadcastPromptText = "📢 <b>הודעה לכולם</b>\n\nאנא הקלד את תוכן ההודעה שתרצה לשלוח לכל המשתמשים הפעילים:"

func cbAdminBcCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := EditOrIgnore(b, ctx.CallbackQuery, broadcastPromptText, keyboards.BackToAdminKeyboard()); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	router.FSMFromContext(ctx).SetState(ctx.EffectiveUser.Id, fsm.AdminFlowBroadcastText)
	return nil
}

func cmdBroadcast(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := ctx.EffectiveMessage.Reply(b, broadcastPromptText, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.BackToAdminKeyboard(), ParseMode: "HTML"})
	if err != nil {
		return err
	}
	router.FSMFromContext(ctx).SetState(ctx.EffectiveUser.Id, fsm.AdminFlowBroadcastText)
	return nil
}

func msgBcText(b *gotgbot.Bot, ctx *ext.Context) error {
	store := router.FSMFromContext(ctx)
	userID := ctx.EffectiveUser.Id
	if state, ok := store.GetState(userID); !ok || state != fsm.AdminFlowBroadcastText {
		return ext.ContinueGroups
	}

	text := ctx.EffectiveMessage.Text
	if text == "" {
		_, err := ctx.EffectiveMessage.Reply(b, "אנא שלח טקסט בלבד.", nil)
		return err
	}

	store.UpdateData(userID, map[string]any{"bc_text": text})
	preview := "📢 <b>תצוגה מקדימה:</b>\n\n" + text + "\n\nהאם לשלוח הודעה זו?"
	_, err := ctx.EffectiveMessage.Reply(b, preview, &gotgbot.SendMessageOpts{ReplyMarkup: keyboards.BroadcastConfirmKeyboard(), ParseMode: "HTML"})
	if err != nil {
		return err
	}
	store.SetState(userID, fsm.AdminFlowBroadcastConfirm)
	return nil
}

func cbAdminBcOk(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	store := router.FSMFromContext(ctx)
	if state, ok := store.GetState(user.ID); !ok || state != fsm.AdminFlowBroadcastConfirm {
		return ext.ContinueGroups
	}
	db := router.DBFromContext(ctx)

	data := store.GetData(user.ID)
	bcText, _ := data["bc_text"].(string)
	if bcText == "" {
		AnswerCallbackAlert(b, ctx.CallbackQuery, "שגיאה, נסה שוב.")
		return nil
	}

	if err := EditOrIgnore(b, ctx.CallbackQuery, "⏳ שולח... אנא המתן.", nil); err != nil {
		return err
	}

	targets, err := services.GetBroadcastTargets(context.Background(), db, user.ID, bcText)
	if err != nil {
		return err
	}

	// Fixed-interval pacing at 20/sec — a simpler stand-in for Python's
	// AsyncRateLimiter token bucket (which allows an initial burst up to
	// capacity then throttles). For a broadcast loop the steady-state rate
	// is what matters; this admin-only, rarely-used path doesn't need the
	// burst nuance replicated exactly.
	const ratePerSec = 20
	interval := time.Second / ratePerSec
	sent, blocked := 0, 0
	for _, target := range targets {
		time.Sleep(interval)
		if _, err := b.SendMessage(target.ID, bcText, nil); err != nil {
			blocked++
		} else {
			sent++
		}
	}

	doneText := fmt.Sprintf("✅ <b>סיום שליחה</b>\n\nנשלח בהצלחה: %d\nנכשלו/חסמו: %d", sent, blocked)
	if err := EditOrIgnore(b, ctx.CallbackQuery, doneText, keyboards.BackToAdminKeyboard()); err != nil {
		return err
	}
	store.Clear(user.ID)
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func adminLogsCaption(n int) string {
	return fmt.Sprintf("הנה %d השורות האחרונות מהלוג.", n)
}

func cbAdminLogsCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cfg := router.ConfigFromContext(ctx)
	n := 50
	logsText := services.GetRecentLogs(cfg.LogDir, n)
	AnswerCallback(b, ctx.CallbackQuery, "")

	msg, ok := ctx.CallbackQuery.Message.(gotgbot.Message)
	if !ok {
		return nil
	}
	logFile := &gotgbot.FileReader{Name: "bot_logs.txt", Data: bytes.NewReader([]byte(logsText))}
	_, err := b.SendDocument(msg.Chat.Id, logFile, &gotgbot.SendDocumentOpts{Caption: adminLogsCaption(n)})
	return err
}

func cmdLogs(b *gotgbot.Bot, ctx *ext.Context) error {
	cfg := router.ConfigFromContext(ctx)
	n := 50
	parts := strings.Fields(ctx.EffectiveMessage.Text)
	if len(parts) > 1 {
		if v, err := strconv.Atoi(parts[1]); err == nil {
			n = v
		}
	}
	logsText := services.GetRecentLogs(cfg.LogDir, n)
	logFile := &gotgbot.FileReader{Name: "bot_logs.txt", Data: bytes.NewReader([]byte(logsText))}
	_, err := b.SendDocument(ctx.EffectiveChat.Id, logFile, &gotgbot.SendDocumentOpts{Caption: adminLogsCaption(n)})
	return err
}

const userSearchHelpText = "👤 לחיפוש משתמש אנא שלח את הפקודה:\n" +
	"<code>/userinfo &lt;id or username&gt;</code>\n" +
	"או <code>/block &lt;id&gt;</code> ו-<code>/unblock &lt;id&gt;</code>."

func cbAdminUserSearchStart(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := EditOrIgnore(b, ctx.CallbackQuery, userSearchHelpText, keyboards.BackToAdminKeyboard()); err != nil {
		return err
	}
	AnswerCallback(b, ctx.CallbackQuery, "")
	return nil
}

func cmdUserinfo(b *gotgbot.Bot, ctx *ext.Context) error {
	db := router.DBFromContext(ctx)
	parts := strings.Fields(ctx.EffectiveMessage.Text)
	if len(parts) < 2 {
		_, err := ctx.EffectiveMessage.Reply(b, "שימוש: /userinfo <id או שם משתמש>", nil)
		return err
	}

	info, err := services.GetUserInfo(context.Background(), db, parts[1])
	if err != nil {
		return err
	}
	if info == nil {
		_, err := ctx.EffectiveMessage.Reply(b, "לא נמצא משתמש כזה.", nil)
		return err
	}

	username := "—"
	if info.Username != nil && *info.Username != "" {
		username = *info.Username
	}
	blockedMark, botBlockedMark := "❌ לא", "❌ לא"
	if info.IsBlocked {
		blockedMark = "✅ כן"
	}
	if info.BotBlocked {
		botBlockedMark = "✅ כן"
	}

	text := fmt.Sprintf(
		"👤 <b>פרטי משתמש</b>\n"+
			"ID: <code>%d</code>\n"+
			"שם: %s\n"+
			"יוזרניים: @%s\n"+
			"אירועים: %d\n"+
			"נוצר ב: %s\n"+
			"נראה לאחרונה: %s\n"+
			"חסום ע״י אדמין: %s\n"+
			"חסם את הבוט: %s",
		info.ID, info.Name, username, info.EventsCount,
		info.CreatedAt.Format("2006-01-02 15:04"), info.LastSeenAt.Format("2006-01-02 15:04"),
		blockedMark, botBlockedMark,
	)
	_, err = ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func cmdBlock(b *gotgbot.Bot, ctx *ext.Context) error   { return adminToggleBlock(b, ctx, true) }
func cmdUnblock(b *gotgbot.Bot, ctx *ext.Context) error { return adminToggleBlock(b, ctx, false) }

func adminToggleBlock(b *gotgbot.Bot, ctx *ext.Context, block bool) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	parts := strings.Fields(ctx.EffectiveMessage.Text)

	usage := "שימוש: /block <id>"
	if !block {
		usage = "שימוש: /unblock <id>"
	}
	if len(parts) < 2 {
		_, err := ctx.EffectiveMessage.Reply(b, usage, nil)
		return err
	}
	targetID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		_, sendErr := ctx.EffectiveMessage.Reply(b, usage, nil)
		return sendErr
	}

	success, err := services.ToggleBlockUser(context.Background(), db, user.ID, targetID, block)
	if err != nil {
		return err
	}
	if !success {
		_, sendErr := ctx.EffectiveMessage.Reply(b, "משתמש לא נמצא.", nil)
		return sendErr
	}

	verb := "נחסם"
	if !block {
		verb = "שוחרר"
	}
	_, sendErr := ctx.EffectiveMessage.Reply(b, fmt.Sprintf("משתמש %d %s בהצלחה.", targetID, verb), nil)
	return sendErr
}

func cbAdminBackupCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	AnswerCallback(b, ctx.CallbackQuery, "מבצע גיבוי...")
	db := router.DBFromContext(ctx)
	cfg := router.ConfigFromContext(ctx)

	path, err := services.ForceBackup(context.Background(), db, user.ID, cfg.BackupDir)
	if err != nil {
		return err
	}
	text := "✅ גיבוי ידני בוצע בהצלחה:\n<code>" + path + "</code>"
	return EditOrIgnore(b, ctx.CallbackQuery, text, keyboards.BackToAdminKeyboard())
}

func cmdForcebackup(b *gotgbot.Bot, ctx *ext.Context) error {
	user := User(ctx)
	db := router.DBFromContext(ctx)
	cfg := router.ConfigFromContext(ctx)

	path, err := services.ForceBackup(context.Background(), db, user.ID, cfg.BackupDir)
	if err != nil {
		return err
	}
	text := "✅ גיבוי ידני בוצע בהצלחה:\n<code>" + path + "</code>"
	_, sendErr := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return sendErr
}
