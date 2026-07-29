package router

import (
	"database/sql"
	"log/slog"

	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/bot/fsm"
	"birthly/internal/config"
)

// NewDispatcher builds a Dispatcher with the fixed middleware chain
// pre-registered (logging, db, fsm, user, throttle, debounce — see the
// Group* constants). Feature handlers should be added afterward via
// AddHandlerToGroup(h, 0) or higher.
func NewDispatcher(db *sql.DB, store *fsm.Store, cfg *config.Config, logger *slog.Logger) *ext.Dispatcher {
	reporter := newErrorReporter(cfg, logger)
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Logger: logger,
		Error:  reporter.handle,
		Panic:  reporter.handlePanic,
	})

	dispatcher.AddHandlerToGroup(loggingHandler(logger), GroupLogging)
	dispatcher.AddHandlerToGroup(configHandler(cfg), GroupConfig)
	dispatcher.AddHandlerToGroup(dbHandler(db), GroupDB)
	dispatcher.AddHandlerToGroup(fsmHandler(store), GroupFSM)
	dispatcher.AddHandlerToGroup(userHandler(db, cfg), GroupUser)
	dispatcher.AddHandlerToGroup(newThrottleHandler(cfg), GroupThrottle)
	dispatcher.AddHandlerToGroup(debounceHandler(cfg), GroupDebounce)

	return dispatcher
}
