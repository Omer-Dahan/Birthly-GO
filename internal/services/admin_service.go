package services

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"birthly/internal/core"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

// SystemStats is the admin dashboard snapshot (SPEC.md ch.33-ish).
type SystemStats struct {
	TotalUsers  int
	ActiveUsers int
	TotalEvents int
	SentToday   int
	FailedToday int
	DBSizeMB    float64
	LastBackup  string
	LastTickAt  string
}

// GetSystemStats aggregates counts + app_meta bookkeeping for the admin
// dashboard. dbPath is the live SQLite file path (for on-disk size).
func GetSystemStats(ctx context.Context, db repo.DBTX, dbPath string) (*SystemStats, error) {
	users := repo.NewUserRepo(db)
	totalUsers, err := users.Count(ctx)
	if err != nil {
		return nil, err
	}
	activeUsers, err := users.CountActiveSince(ctx, time.Now().AddDate(0, 0, -30))
	if err != nil {
		return nil, err
	}

	var totalEvents int
	if err := queryRowScan(ctx, db, `SELECT COUNT(*) FROM events`, &totalEvents); err != nil {
		return nil, err
	}

	today := time.Now().UTC().Format("2006-01-02")
	var sentToday, failedToday int
	if err := queryRowScan(ctx, db,
		`SELECT COUNT(*) FROM notifications_log WHERE status = 'sent' AND occurrence_date = ?`, &sentToday, today); err != nil {
		return nil, err
	}
	if err := queryRowScan(ctx, db,
		`SELECT COUNT(*) FROM notifications_log WHERE status = 'failed' AND occurrence_date = ?`, &failedToday, today); err != nil {
		return nil, err
	}

	dbSizeMB := 0.0
	if info, err := os.Stat(dbPath); err == nil {
		dbSizeMB = float64(info.Size()) / (1024 * 1024)
	}

	meta := repo.NewAppMetaRepo(db)
	lastBackup := "Never"
	if v, err := meta.Get(ctx, "last_auto_backup"); err == nil && v != nil {
		lastBackup = *v
	}
	lastTick := "Never"
	if v, err := meta.Get(ctx, "last_tick_at"); err == nil && v != nil {
		lastTick = *v
	}

	return &SystemStats{
		TotalUsers: totalUsers, ActiveUsers: activeUsers, TotalEvents: totalEvents,
		SentToday: sentToday, FailedToday: failedToday,
		DBSizeMB: roundTo2(dbSizeMB), LastBackup: lastBackup, LastTickAt: lastTick,
	}, nil
}

func roundTo2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// queryRowScan is a tiny helper for one-off aggregate queries that don't
// warrant their own repo method.
func queryRowScan(ctx context.Context, db repo.DBTX, query string, dst any, args ...any) error {
	return db.QueryRowContext(ctx, query, args...).Scan(dst)
}

// GetBroadcastTargets returns active, non-blocked users to broadcast to,
// logging the attempt to the audit trail. Sending itself touches the bot
// API, so it stays in the handler layer — services stay framework-agnostic.
func GetBroadcastTargets(ctx context.Context, db repo.DBTX, adminID int64, text string) ([]*models.User, error) {
	users, err := repo.NewUserRepo(db).ListBroadcastTargets(ctx)
	if err != nil {
		return nil, err
	}
	audit := repo.NewAuditRepo(db, adminID)
	_, err = audit.Add(ctx, core.AuditActionAdminBroadcast, "system", nil, map[string]any{
		"text": text, "target_count": len(users),
	})
	if err != nil {
		return nil, err
	}
	return users, nil
}

// UserInfo is the admin "user lookup" result (admin_service.get_user_info).
type UserInfo struct {
	ID          int64
	Name        string
	Username    *string
	EventsCount int
	CreatedAt   time.Time
	LastSeenAt  time.Time
	IsBlocked   bool
	BotBlocked  bool
}

// GetUserInfo looks up a user by numeric id or @username (case-insensitive).
// Returns (nil, nil) if not found.
func GetUserInfo(ctx context.Context, db repo.DBTX, identifier string) (*UserInfo, error) {
	users := repo.NewUserRepo(db)
	var user *models.User
	var err error

	if isAllDigits(identifier) {
		var id int64
		fmt.Sscanf(identifier, "%d", &id)
		user, err = users.Get(ctx, id)
	} else {
		user, err = users.GetByUsername(ctx, strings.TrimPrefix(identifier, "@"))
	}
	if err != nil || user == nil {
		return nil, err
	}

	var eventsCount int
	if err := queryRowScan(ctx, db, `SELECT COUNT(*) FROM events WHERE user_id = ?`, &eventsCount, user.ID); err != nil {
		return nil, err
	}

	name := user.FirstName
	if user.LastName != nil && *user.LastName != "" {
		name = strings.TrimSpace(name + " " + *user.LastName)
	}

	return &UserInfo{
		ID: user.ID, Name: name, Username: user.Username, EventsCount: eventsCount,
		CreatedAt: user.CreatedAt, LastSeenAt: user.LastSeenAt,
		IsBlocked: user.IsBlocked, BotBlocked: user.BotBlockedByUser,
	}, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ToggleBlockUser sets a user's blocked flag and logs the action. Returns
// false if the user doesn't exist.
func ToggleBlockUser(ctx context.Context, db repo.DBTX, adminID, userID int64, block bool) (bool, error) {
	users := repo.NewUserRepo(db)
	user, err := users.Get(ctx, userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, nil
	}

	if err := users.SetBlocked(ctx, userID, block); err != nil {
		return false, err
	}

	action := core.AuditActionUserBlocked
	if !block {
		action = "user_unblocked"
	}
	entityID := userID
	audit := repo.NewAuditRepo(db, adminID)
	if _, err := audit.Add(ctx, action, "users", &entityID, nil); err != nil {
		return false, err
	}
	return true, nil
}

// ForceBackup runs VACUUM INTO to produce a hot backup and logs it.
// db must be the live *sql.DB (VACUUM cannot run inside a transaction).
func ForceBackup(ctx context.Context, db *sql.DB, adminID int64, backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	nowStr := time.Now().UTC().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("birthly_manual_%s.db", nowStr))
	absPath, err := filepath.Abs(backupPath)
	if err != nil {
		return "", err
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(absPath, "'", "''"))); err != nil {
		return "", err
	}

	audit := repo.NewAuditRepo(db, adminID)
	if _, err := audit.Add(ctx, core.AuditActionExport, "system", nil, map[string]any{
		"format": "db", "type": "manual_vacuum", "path": backupPath,
	}); err != nil {
		return "", err
	}

	return backupPath, nil
}

// GetRecentLogs returns the last n lines of the bot's log file.
func GetRecentLogs(logDir string, n int) string {
	logPath := filepath.Join(logDir, "bot.log")
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "Log file not found."
		}
		return fmt.Sprintf("Error reading logs: %v", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("Error reading logs: %v", err)
	}
	return strings.Join(lines, "\n")
}
