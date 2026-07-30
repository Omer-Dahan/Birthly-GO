// Package config parses and validates the application's environment configuration.
//
// 1:1 mapping of the settings in Birthly's app/config.py. Values come from the
// process environment, optionally pre-loaded from a .env file in the working
// directory (same semantics as pydantic-settings' env_file=".env": present
// OS environment variables always win over the file).
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config mirrors app.config.Settings field-for-field.
type Config struct {
	// Telegram
	BotToken    string
	BotUsername string
	AdminIDs    []int64

	// Database
	DBPath string

	// Defaults
	DefaultLanguage   string
	DefaultTimezone   string
	DefaultNotifyTime string
	DefaultDateFormat string
	DefaultTimeFormat string

	// Scheduler
	SchedulerTickSeconds int
	ReminderGraceHours   int
	MaxUpcomingDays      int

	// Limits
	MaxEventsPerUser    int
	RateLimitMessages   int
	RateLimitCallbacks  int
	BroadcastRatePerSec int
	PageSize            int

	// Anti-spam: stricter limits for accounts still within their grace period
	NewAccountGraceHours         float64
	NewAccountRateLimitMessages  int
	NewAccountRateLimitCallbacks int

	// Anti-spam: minimum gap between two button taps from the same user
	CallbackDebounceMS int

	// Backup
	AutoBackupEnabled   bool
	AutoBackupTime      string
	BackupRetentionDays int
	BackupDir           string

	// Logging
	LogLevel       string
	LogDir         string
	LogJSON        bool
	LogMaxBytes    int64
	LogBackupCount int

	// Misc
	Env                 string
	ReportErrorsToAdmin bool
}

// defaults holds every non-empty default from app/config.py, keyed by the
// same env var names used in .env / .env.example.
var defaults = map[string]string{
	"DB_PATH":                          "data/birthly.db",
	"DEFAULT_LANGUAGE":                 "he",
	"DEFAULT_TIMEZONE":                 "Asia/Jerusalem",
	"DEFAULT_NOTIFY_TIME":              "09:00",
	"DEFAULT_DATE_FORMAT":              "DD/MM/YYYY",
	"DEFAULT_TIME_FORMAT":              "24h",
	"SCHEDULER_TICK_SECONDS":           "60",
	"REMINDER_GRACE_HOURS":             "6",
	"MAX_UPCOMING_DAYS":                "45",
	"MAX_EVENTS_PER_USER":              "250",
	"RATE_LIMIT_MESSAGES":              "12",
	"RATE_LIMIT_CALLBACKS":             "25",
	"BROADCAST_RATE_PER_SEC":           "20",
	"PAGE_SIZE":                        "6",
	"NEW_ACCOUNT_GRACE_HOURS":          "1.0",
	"NEW_ACCOUNT_RATE_LIMIT_MESSAGES":  "6",
	"NEW_ACCOUNT_RATE_LIMIT_CALLBACKS": "12",
	"CALLBACK_DEBOUNCE_MS":             "400",
	"AUTO_BACKUP_ENABLED":              "true",
	"AUTO_BACKUP_TIME":                 "03:30",
	"BACKUP_RETENTION_DAYS":            "30",
	"BACKUP_DIR":                       "data/backups",
	"LOG_LEVEL":                        "INFO",
	"LOG_DIR":                          "data/logs",
	"LOG_JSON":                         "true",
	"LOG_MAX_BYTES":                    "10485760",
	"LOG_BACKUP_COUNT":                 "5",
	"ENV":                              "production",
	"REPORT_ERRORS_TO_ADMIN":           "true",
}

// requiredKeys have no default in app/config.py — loading fails without them.
var requiredKeys = []string{"BOT_TOKEN", "BOT_USERNAME"}

// loadDotenv reads KEY=VALUE pairs from path into the process environment,
// skipping keys already set (OS env always takes precedence, matching
// pydantic-settings' env_file behavior). Missing file is not an error.
func loadDotenv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, value)
	}
	return scanner.Err()
}

func getEnv(key string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaults[key]
}

func parseAdminIDs(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int64{}, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ADMIN_IDS entry %q: %w", p, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", raw)
	}
}

// Load reads configuration from the OS environment, pre-populated from a
// ".env" file in the current directory if present. It fails fast (matching
// _load_settings' SystemExit(1) behavior) by returning an error the caller
// should treat as fatal at startup.
func Load() (*Config, error) {
	if err := loadDotenv(".env"); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	for _, key := range requiredKeys {
		if strings.TrimSpace(getEnv(key)) == "" {
			return nil, fmt.Errorf("failed to load configuration: missing required env var %s", key)
		}
	}

	adminIDs, err := parseAdminIDs(getEnv("ADMIN_IDS"))
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	c := &Config{
		BotToken:    getEnv("BOT_TOKEN"),
		BotUsername: getEnv("BOT_USERNAME"),
		AdminIDs:    adminIDs,

		DBPath: getEnv("DB_PATH"),

		DefaultLanguage:   getEnv("DEFAULT_LANGUAGE"),
		DefaultTimezone:   getEnv("DEFAULT_TIMEZONE"),
		DefaultNotifyTime: getEnv("DEFAULT_NOTIFY_TIME"),
		DefaultDateFormat: getEnv("DEFAULT_DATE_FORMAT"),
		DefaultTimeFormat: getEnv("DEFAULT_TIME_FORMAT"),

		BackupDir: getEnv("BACKUP_DIR"),
		LogDir:    getEnv("LOG_DIR"),
		LogLevel:  getEnv("LOG_LEVEL"),
		Env:       getEnv("ENV"),

		AutoBackupTime: getEnv("AUTO_BACKUP_TIME"),
	}

	intFields := []struct {
		key string
		dst *int
	}{
		{"SCHEDULER_TICK_SECONDS", &c.SchedulerTickSeconds},
		{"REMINDER_GRACE_HOURS", &c.ReminderGraceHours},
		{"MAX_UPCOMING_DAYS", &c.MaxUpcomingDays},
		{"MAX_EVENTS_PER_USER", &c.MaxEventsPerUser},
		{"RATE_LIMIT_MESSAGES", &c.RateLimitMessages},
		{"RATE_LIMIT_CALLBACKS", &c.RateLimitCallbacks},
		{"BROADCAST_RATE_PER_SEC", &c.BroadcastRatePerSec},
		{"PAGE_SIZE", &c.PageSize},
		{"NEW_ACCOUNT_RATE_LIMIT_MESSAGES", &c.NewAccountRateLimitMessages},
		{"NEW_ACCOUNT_RATE_LIMIT_CALLBACKS", &c.NewAccountRateLimitCallbacks},
		{"CALLBACK_DEBOUNCE_MS", &c.CallbackDebounceMS},
		{"BACKUP_RETENTION_DAYS", &c.BackupRetentionDays},
		{"LOG_BACKUP_COUNT", &c.LogBackupCount},
	}
	for _, f := range intFields {
		v, err := strconv.Atoi(strings.TrimSpace(getEnv(f.key)))
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration: invalid %s: %w", f.key, err)
		}
		*f.dst = v
	}

	logMaxBytes, err := strconv.ParseInt(strings.TrimSpace(getEnv("LOG_MAX_BYTES")), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: invalid LOG_MAX_BYTES: %w", err)
	}
	c.LogMaxBytes = logMaxBytes

	graceHours, err := strconv.ParseFloat(strings.TrimSpace(getEnv("NEW_ACCOUNT_GRACE_HOURS")), 64)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: invalid NEW_ACCOUNT_GRACE_HOURS: %w", err)
	}
	c.NewAccountGraceHours = graceHours

	boolFields := []struct {
		key string
		dst *bool
	}{
		{"AUTO_BACKUP_ENABLED", &c.AutoBackupEnabled},
		{"LOG_JSON", &c.LogJSON},
		{"REPORT_ERRORS_TO_ADMIN", &c.ReportErrorsToAdmin},
	}
	for _, f := range boolFields {
		v, err := parseBool(getEnv(f.key))
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration: invalid %s: %w", f.key, err)
		}
		*f.dst = v
	}

	return c, nil
}
