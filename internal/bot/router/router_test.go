package router

import (
	"log/slog"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/bot/fsm"
	"birthly/internal/config"
)

// TestNewDispatcher_InjectsConfigAndLoggerIntoContext is the regression test
// for a defect where configHandler and a separate loggerHandler were
// registered as two "check: always" handlers in the same dispatcher group
// (GroupConfig). gotgbot's dispatcher runs at most one matching handler per
// group and then breaks to the next group (ext/dispatcher.go,
// iterateOverHandlerGroups), so the second handler in that group never ran:
// DataKeyLogger was never set, and every feature handler's
// LoggerFromContext silently fell back to slog.Default() instead of the
// process logger passed to NewDispatcher.
//
// This drives a real synthetic update through the full dispatcher built by
// NewDispatcher (not a bare ext.Context), so it exercises the actual group
// registration, not just the handler function in isolation.
func TestNewDispatcher_InjectsConfigAndLoggerIntoContext(t *testing.T) {
	cfg := &config.Config{}
	logger := testLogger()
	fsmStore := fsm.NewStore(0, 200)

	// db is nil on purpose: the update below has no EffectiveUser (Message
	// with no From), so userHandler returns before touching the DB, and no
	// other middleware in the chain queries it either.
	dispatcher := NewDispatcher(nil, fsmStore, cfg, logger)

	var probed bool
	var gotCfg *config.Config
	var gotLogger *slog.Logger
	dispatcher.AddHandler(simpleHandler{
		name:  "probe",
		check: always,
		run: func(b *gotgbot.Bot, ctx *ext.Context) error {
			probed = true
			gotCfg = ConfigFromContext(ctx)
			gotLogger = LoggerFromContext(ctx)
			return nil
		},
	})

	update := &gotgbot.Update{
		Message: &gotgbot.Message{Chat: gotgbot.Chat{Id: 555}, Text: "hi"},
	}
	bot := testBot(&fakeBotClient{})

	if err := dispatcher.ProcessUpdate(bot, update, nil); err != nil {
		t.Fatalf("ProcessUpdate: %v", err)
	}
	if !probed {
		t.Fatal("probe handler never ran; the update did not reach group 0")
	}
	if gotCfg != cfg {
		t.Errorf("ConfigFromContext returned %p, want the exact cfg passed to NewDispatcher (%p)", gotCfg, cfg)
	}
	if gotLogger != logger {
		t.Errorf("LoggerFromContext returned %p, want the exact logger passed to NewDispatcher (%p, likely fell back to slog.Default() because DataKeyLogger was never set)", gotLogger, logger)
	}
}
