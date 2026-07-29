package router

import (
	"database/sql"
	"log/slog"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/config"
)

// NewDispatcher builds a Dispatcher with the fixed middleware chain
// pre-registered (logging, db, user, throttle, debounce — see the Group*
// constants). Feature handlers should be added afterward via
// AddHandlerToGroup(h, 0) or higher.
func NewDispatcher(db *sql.DB, cfg *config.Config, logger *slog.Logger) *ext.Dispatcher {
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Logger: logger,
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			logger.Error("handler error", "error", err)
			return ext.DispatcherActionNoop
		},
	})

	dispatcher.AddHandlerToGroup(loggingHandler(logger), GroupLogging)
	dispatcher.AddHandlerToGroup(dbHandler(db), GroupDB)
	dispatcher.AddHandlerToGroup(userHandler(db, cfg), GroupUser)
	dispatcher.AddHandlerToGroup(newThrottleHandler(cfg), GroupThrottle)
	dispatcher.AddHandlerToGroup(debounceHandler(cfg), GroupDebounce)

	return dispatcher
}
