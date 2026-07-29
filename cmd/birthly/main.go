// Command birthly runs the Birthly Telegram bot.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"birthly/internal/bot/router"
	"birthly/internal/config"
	"birthly/internal/i18n"
	"birthly/internal/store"
)

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

	dispatcher := router.NewDispatcher(db, cfg, logger)
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
// Only a minimal /start acknowledgement is wired so far — this proves the
// full chain (config -> store -> middleware -> handler -> i18n) works
// end-to-end. The other ~108 handler functions from app/handlers/*.py
// (menu, event add/edit/list/card, reminders, settings, templates, search,
// stats, backup, admin, errors/fallback) are stage 8 of the migration plan
// and are not yet ported.
func registerHandlers(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(handlers.NewCommand("start", cmdStart))
}

func cmdStart(b *gotgbot.Bot, ctx *ext.Context) error {
	user := router.UserFromContext(ctx)
	lang := "he"
	if user != nil {
		lang = user.Language
	}
	_, err := ctx.EffectiveMessage.Reply(b, i18n.T("onboarding.welcome", lang, nil), nil)
	return err
}
