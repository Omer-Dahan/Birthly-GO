// Command birthly runs the Birthly Telegram bot.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/bot/fsm"
	bothandlers "birthly/internal/bot/handlers"
	"birthly/internal/bot/router"
	"birthly/internal/config"
	"birthly/internal/i18n"
	"birthly/internal/logging"
	"birthly/internal/scheduler"
	"birthly/internal/services"
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
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	logger, err := logging.Setup(cfg)
	if err != nil {
		return fmt.Errorf("setting up logging: %w", err)
	}

	if err := i18n.LoadLocales(); err != nil {
		return fmt.Errorf("loading locales: %w", err)
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Promote any pre-existing user whose id is in ADMIN_IDS but who
	// contacted the bot before being added to it — port of app/main.py's
	// _sync_admins(). Brand-new users already get is_admin set correctly at
	// creation time via the per-request userHandler middleware.
	if err := services.SyncAdmins(context.Background(), db, cfg.AdminIDs); err != nil {
		return fmt.Errorf("syncing admins: %w", err)
	}

	bot, err := gotgbot.NewBot(cfg.BotToken, nil)
	if err != nil {
		return fmt.Errorf("creating bot: %w", err)
	}

	fsmStore := fsm.NewStore(fsmMaxIdle, 200)
	dispatcher := router.NewDispatcher(db, fsmStore, cfg, logger)
	registerHandlers(dispatcher)

	sched, err := scheduler.Build(bot, db, cfg, logger)
	if err != nil {
		return fmt.Errorf("building scheduler: %w", err)
	}
	sched.Start()
	// Graceful shutdown, matching app/main.py's try/finally: wait for any
	// in-flight job to finish before the process exits, rather than killing
	// it mid-write. Only runs if Idle() below returns normally, which the
	// signal handler goroutine ensures happens on SIGINT/SIGTERM instead of
	// the process dying immediately with pending work interrupted.
	defer sched.Stop()

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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("shutdown_signal_received", "signal", sig.String())
		if err := updater.Stop(); err != nil {
			logger.Error("updater_stop_failed", "error", err)
		}
	}()

	logger.Info("bot_starting", "username", bot.Username)
	updater.Idle()
	logger.Info("bot_stopped")
	return nil
}

// registerHandlers wires every feature handler package into the dispatcher
// (group 0, after the middleware chain router.NewDispatcher already
// registered in its own lower-numbered groups). RegisterFallback must stay
// last — see its doc comment.
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
