// Command birthly runs the Birthly Telegram bot.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/bot/fsm"
	bothandlers "birthly/internal/bot/handlers"
	"birthly/internal/bot/router"
	"birthly/internal/config"
	"birthly/internal/i18n"
	"birthly/internal/store"
)

// fsmMaxIdleSeconds abandons in-progress flows/state after 6h of
// inactivity — matches app/main.py's _FSM_MAX_IDLE_SECONDS.
const fsmMaxIdle = 6 * time.Hour

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	if err := i18n.LoadLocales(); err != nil {
		return fmt.Errorf("loading locales: %w", err)
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	bot, err := gotgbot.NewBot(cfg.BotToken, nil)
	if err != nil {
		return fmt.Errorf("creating bot: %w", err)
	}

	fsmStore := fsm.NewStore(fsmMaxIdle, 200)
	dispatcher := router.NewDispatcher(db, fsmStore, cfg, logger)
	registerHandlers(dispatcher)

	updater := ext.NewUpdater(dispatcher, &ext.UpdaterOpts{Logger: logger})

	if err := updater.StartPolling(bot, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: 10,
			},
		},
	}); err != nil {
		return fmt.Errorf("starting polling: %w", err)
	}

	logger.Info("bot_starting", "username", bot.Username)
	updater.Idle()
	return nil
}

// registerHandlers wires the feature handlers into the dispatcher (group 0+,
// after the middleware chain router.NewDispatcher already registered).
//
// /start + the full onboarding flow, and the home/help menu screens, are
// wired end-to-end (app/handlers/start.py, app/handlers/menu.py). The other
// ~100 handler functions from app/handlers/*.py (event add/edit/list/card,
// reminders, settings, templates, search, stats, backup, admin,
// errors/fallback) are stage 8 of the migration plan and are not yet ported.
func registerHandlers(dispatcher *ext.Dispatcher) {
	bothandlers.RegisterStart(dispatcher)
	bothandlers.RegisterMenu(dispatcher)
	bothandlers.RegisterEventAdd(dispatcher)
	bothandlers.RegisterEventCard(dispatcher)
	bothandlers.RegisterEventList(dispatcher)
	bothandlers.RegisterEventEdit(dispatcher)
	bothandlers.RegisterReminders(dispatcher)
	bothandlers.RegisterSettings(dispatcher)
	bothandlers.RegisterTemplates(dispatcher)
	bothandlers.RegisterSearch(dispatcher)
	bothandlers.RegisterStats(dispatcher)
	bothandlers.RegisterBackup(dispatcher)
	bothandlers.RegisterAdmin(dispatcher)
	// RegisterFallback must be last: its handlers match any message/callback,
	// and gotgbot only runs the first match per group (all of the above
	// share the default group 0) — see RegisterFallback's doc comment.
	bothandlers.RegisterFallback(dispatcher)
}
