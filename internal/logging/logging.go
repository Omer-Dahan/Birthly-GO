// Package logging wires up the process-wide structured logger.
//
// Port of app/utils/logger.py's setup_logging(): two sinks — JSON lines to a
// size-rotated file under log_dir (read back by the admin /logs command via
// services.GetRecentLogs), plain text to stdout (picked up by journald under
// systemd, matching the deploy/birthly-go.service unit's
// StandardOutput=journal).
package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"

	"birthly/internal/config"
)

// parseLevel maps the LOG_LEVEL env var (Python's logging module names) to
// an slog.Level, defaulting to Info for an unrecognized value.
func parseLevel(raw string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARNING", "WARN":
		return slog.LevelWarn
	case "ERROR", "CRITICAL":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Setup configures the rotating-file + stdout logger described above and
// returns it as the process-wide slog.Logger. The log directory is created
// if missing (matching Python's log_dir.mkdir(parents=True, exist_ok=True)).
func Setup(cfg *config.Config) (*slog.Logger, error) {
	if err := os.MkdirAll(cfg.LogDir, 0o755); err != nil {
		return nil, err
	}

	level := parseLevel(cfg.LogLevel)
	opts := &slog.HandlerOptions{Level: level}

	fileWriter := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, "bot.log"),
		MaxSize:    maxSizeMB(cfg.LogMaxBytes),
		MaxBackups: cfg.LogBackupCount,
		Compress:   false, // Python's RotatingFileHandler doesn't compress either
	}

	handlers := []slog.Handler{
		slog.NewJSONHandler(fileWriter, opts),
		slog.NewTextHandler(os.Stdout, opts),
	}

	return slog.New(fanOutHandler{handlers: handlers}), nil
}

// maxSizeMB converts LOG_MAX_BYTES (bytes, matching Python's
// RotatingFileHandler(maxBytes=...)) to lumberjack's MB-granularity MaxSize,
// rounding up so a configured byte value never rotates more eagerly than
// intended.
func maxSizeMB(maxBytes int64) int {
	const mb = 1024 * 1024
	if maxBytes <= 0 {
		return 10 // lumberjack's own default
	}
	sizeMB := int((maxBytes + mb - 1) / mb)
	if sizeMB < 1 {
		return 1
	}
	return sizeMB
}

// fanOutHandler implements slog.Handler by forwarding every call to each
// wrapped handler in turn — the direct equivalent of Python's root logger
// carrying two logging.Handler instances (file + stdout).
type fanOutHandler struct {
	handlers []slog.Handler
}

func (f fanOutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanOutHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, h := range f.handlers {
		if !h.Enabled(ctx, record.Level) {
			continue
		}
		if err := h.Handle(ctx, record.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (f fanOutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return fanOutHandler{handlers: next}
}

func (f fanOutHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithGroup(name)
	}
	return fanOutHandler{handlers: next}
}
