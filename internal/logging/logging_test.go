package logging

import (
	"os"
	"path/filepath"
	"testing"

	"birthly/internal/config"
)

func TestParseLevel(t *testing.T) {
	tests := map[string]string{
		"DEBUG": "DEBUG", "debug": "DEBUG",
		"WARNING": "WARN", "warn": "WARN",
		"ERROR": "ERROR", "CRITICAL": "ERROR",
		"INFO": "INFO", "nonsense": "INFO", "": "INFO",
	}
	for raw, wantLevelText := range tests {
		if got := parseLevel(raw).String(); got != wantLevelText {
			t.Errorf("parseLevel(%q).String() = %q, want %q", raw, got, wantLevelText)
		}
	}
}

func TestMaxSizeMB(t *testing.T) {
	tests := []struct {
		bytes int64
		want  int
	}{
		{0, 10},
		{1024 * 1024, 1},
		{5 * 1024 * 1024, 5},
		{5*1024*1024 + 1, 6}, // rounds up
	}
	for _, tc := range tests {
		if got := maxSizeMB(tc.bytes); got != tc.want {
			t.Errorf("maxSizeMB(%d) = %d, want %d", tc.bytes, got, tc.want)
		}
	}
}

// tempLogDir creates its own temp directory instead of t.TempDir(), because
// lumberjack.Logger keeps its file handle open (closed only on rotation or
// process exit) — on Windows, t.TempDir()'s automatic RemoveAll cleanup
// fails hard on an open file and fails the test. RemoveAll errors here are
// tolerated instead of fatal, same as the OS would do at process exit.
func tempLogDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "birthly-logging-test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("cleanup: could not remove %s (likely still open on Windows): %v", dir, err)
		}
	})
	return dir
}

func TestSetup_CreatesLogDirAndWritesToFile(t *testing.T) {
	logDir := filepath.Join(tempLogDir(t), "logs")
	cfg := &config.Config{LogDir: logDir, LogLevel: "INFO", LogMaxBytes: 10 * 1024 * 1024, LogBackupCount: 3}

	logger, err := Setup(cfg)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	logger.Info("test_message", "key", "value")

	logPath := filepath.Join(logDir, "bot.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading %s: %v", logPath, err)
	}
	if len(data) == 0 {
		t.Error("bot.log is empty after logging a message")
	}
}

func TestSetup_RespectsLogLevel(t *testing.T) {
	logDir := filepath.Join(tempLogDir(t), "logs")
	cfg := &config.Config{LogDir: logDir, LogLevel: "ERROR", LogMaxBytes: 10 * 1024 * 1024, LogBackupCount: 3}

	logger, err := Setup(cfg)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	logger.Debug("should_be_filtered")
	logger.Info("also_filtered")

	// Below the configured level, lumberjack never even opens the file —
	// no bytes get written, so "file doesn't exist yet" is the expected
	// outcome here, not an error.
	data, err := os.ReadFile(filepath.Join(logDir, "bot.log"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading bot.log: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected no output below ERROR level, got: %s", data)
	}
}
