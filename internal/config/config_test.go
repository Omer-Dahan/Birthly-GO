package config

import (
	"os"
	"path/filepath"
	"testing"
)

// configEnvKeys is every env var Load() reads — required or defaulted.
// Tests clear all of them before and after each run so leftover process
// environment (from the developer's shell, or an earlier test writing via
// loadDotenv's raw os.Setenv) never leaks into another test.
func configEnvKeys() []string {
	keys := append([]string{}, requiredKeys...)
	keys = append(keys, "ADMIN_IDS")
	for k := range defaults {
		keys = append(keys, k)
	}
	return keys
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := configEnvKeys()
	for _, k := range keys {
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range keys {
			os.Unsetenv(k)
		}
	})
}

// chdirTemp switches the working directory to a fresh temp dir (Load()
// reads ".env" relative to cwd) and restores it after the test.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}

func TestLoad_MissingRequiredKeysFails(t *testing.T) {
	clearConfigEnv(t)
	chdirTemp(t)

	_, err := Load()
	if err == nil {
		t.Fatal("expected an error when BOT_TOKEN/BOT_USERNAME are unset")
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	clearConfigEnv(t)
	chdirTemp(t)
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("BOT_USERNAME", "test_bot")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Spot-check defaults across every parsed type, including the
	// RATE_LIMIT_MESSAGES/CALLBACKS values a stage-1 audit caught drifting
	// from the committed .env.example (12/25 is correct; 20/40 was the
	// stale, wrong value) — this pins that fix down as a regression test.
	if cfg.DBPath != "data/birthly.db" {
		t.Errorf("DBPath = %q, want data/birthly.db", cfg.DBPath)
	}
	if cfg.DefaultLanguage != "he" {
		t.Errorf("DefaultLanguage = %q, want he", cfg.DefaultLanguage)
	}
	if cfg.RateLimitMessages != 12 {
		t.Errorf("RateLimitMessages = %d, want 12", cfg.RateLimitMessages)
	}
	if cfg.RateLimitCallbacks != 25 {
		t.Errorf("RateLimitCallbacks = %d, want 25", cfg.RateLimitCallbacks)
	}
	if cfg.MaxEventsPerUser != 1000 {
		t.Errorf("MaxEventsPerUser = %d, want 1000", cfg.MaxEventsPerUser)
	}
	if !cfg.AutoBackupEnabled {
		t.Error("AutoBackupEnabled = false, want true")
	}
	if cfg.NewAccountGraceHours != 1.0 {
		t.Errorf("NewAccountGraceHours = %v, want 1.0", cfg.NewAccountGraceHours)
	}
	if cfg.LogMaxBytes != 10485760 {
		t.Errorf("LogMaxBytes = %d, want 10485760", cfg.LogMaxBytes)
	}
	if len(cfg.AdminIDs) != 0 {
		t.Errorf("AdminIDs = %v, want empty", cfg.AdminIDs)
	}
}

func TestLoad_OSEnvOverridesDotenvFile(t *testing.T) {
	clearConfigEnv(t)
	dir := chdirTemp(t)
	envFile := "BOT_TOKEN=from-file\nBOT_USERNAME=from_file_bot\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envFile), 0o644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	t.Setenv("BOT_TOKEN", "from-os-env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BotToken != "from-os-env" {
		t.Errorf("BotToken = %q, want from-os-env (OS env must win over .env file)", cfg.BotToken)
	}
	if cfg.BotUsername != "from_file_bot" {
		t.Errorf("BotUsername = %q, want from_file_bot (from .env, no OS override set)", cfg.BotUsername)
	}
}

func TestLoad_ReadsDotenvFile(t *testing.T) {
	clearConfigEnv(t)
	dir := chdirTemp(t)
	envFile := "BOT_TOKEN=tok\nBOT_USERNAME=user\nRATE_LIMIT_MESSAGES=99\nADMIN_IDS=111,222\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envFile), 0o644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RateLimitMessages != 99 {
		t.Errorf("RateLimitMessages = %d, want 99 (from .env file)", cfg.RateLimitMessages)
	}
	if len(cfg.AdminIDs) != 2 || cfg.AdminIDs[0] != 111 || cfg.AdminIDs[1] != 222 {
		t.Errorf("AdminIDs = %v, want [111 222]", cfg.AdminIDs)
	}
}

func TestLoad_InvalidIntFieldReturnsError(t *testing.T) {
	clearConfigEnv(t)
	chdirTemp(t)
	t.Setenv("BOT_TOKEN", "tok")
	t.Setenv("BOT_USERNAME", "user")
	t.Setenv("MAX_EVENTS_PER_USER", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a non-numeric MAX_EVENTS_PER_USER")
	}
}

func TestLoad_InvalidBoolFieldReturnsError(t *testing.T) {
	clearConfigEnv(t)
	chdirTemp(t)
	t.Setenv("BOT_TOKEN", "tok")
	t.Setenv("BOT_USERNAME", "user")
	t.Setenv("AUTO_BACKUP_ENABLED", "maybe")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an invalid AUTO_BACKUP_ENABLED value")
	}
}

func TestParseAdminIDs(t *testing.T) {
	tests := []struct {
		in      string
		want    []int64
		wantErr bool
	}{
		{"", []int64{}, false},
		{"123", []int64{123}, false},
		{"123,456", []int64{123, 456}, false},
		{" 123 , 456 ", []int64{123, 456}, false},
		{"123,,456", []int64{123, 456}, false}, // empty entries skipped
		{"123,abc", nil, true},
	}
	for _, tc := range tests {
		got, err := parseAdminIDs(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseAdminIDs(%q) expected an error, got %v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAdminIDs(%q): unexpected error %v", tc.in, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseAdminIDs(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseAdminIDs(%q) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

func TestParseBool(t *testing.T) {
	tests := map[string]bool{
		"1": true, "true": true, "yes": true, "on": true, "TRUE": true,
		"0": false, "false": false, "no": false, "off": false, "": false,
	}
	for in, want := range tests {
		got, err := parseBool(in)
		if err != nil {
			t.Errorf("parseBool(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseBool(%q) = %v, want %v", in, got, want)
		}
	}

	if _, err := parseBool("maybe"); err == nil {
		t.Error("parseBool(maybe) expected an error")
	}
}
