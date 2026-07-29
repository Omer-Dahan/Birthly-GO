package handlers

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"birthly/internal/bot/callbacks"
	"birthly/internal/bot/router"
	"birthly/internal/services"
	"birthly/internal/store"
	"birthly/internal/store/models"
)

func ctxWithUser(user *models.User) *ext.Context {
	return &ext.Context{Update: &gotgbot.Update{}, Data: map[string]any{router.DataKeyUser: user}}
}

func TestRequireAdminMsg_RejectsNonAdmin(t *testing.T) {
	called := false
	wrapped := requireAdminMsg(func(b *gotgbot.Bot, ctx *ext.Context) error {
		called = true
		return nil
	})

	err := wrapped(nil, ctxWithUser(&models.User{ID: 1, IsAdmin: false}))
	if err != ext.ContinueGroups {
		t.Errorf("requireAdminMsg for non-admin = %v, want ext.ContinueGroups", err)
	}
	if called {
		t.Error("requireAdminMsg should not invoke the wrapped handler for a non-admin")
	}
}

func TestRequireAdminMsg_PassesAdmin(t *testing.T) {
	called := false
	wrapped := requireAdminMsg(func(b *gotgbot.Bot, ctx *ext.Context) error {
		called = true
		return nil
	})

	err := wrapped(nil, ctxWithUser(&models.User{ID: 1, IsAdmin: true}))
	if err != nil {
		t.Errorf("requireAdminMsg for admin returned %v, want nil", err)
	}
	if !called {
		t.Error("requireAdminMsg should invoke the wrapped handler for an admin")
	}
}

func TestRequireAdminCB_RejectsNonAdmin(t *testing.T) {
	called := false
	wrapped := requireAdminCB(func(b *gotgbot.Bot, ctx *ext.Context) error {
		called = true
		return nil
	})

	err := wrapped(nil, ctxWithUser(&models.User{ID: 1, IsAdmin: false}))
	if err != ext.ContinueGroups {
		t.Errorf("requireAdminCB for non-admin = %v, want ext.ContinueGroups", err)
	}
	if called {
		t.Error("requireAdminCB should not invoke the wrapped handler for a non-admin")
	}
}

func TestRequireAdminCB_PassesAdmin(t *testing.T) {
	called := false
	wrapped := requireAdminCB(func(b *gotgbot.Bot, ctx *ext.Context) error {
		called = true
		return nil
	})

	err := wrapped(nil, ctxWithUser(&models.User{ID: 1, IsAdmin: true}))
	if err != nil {
		t.Errorf("requireAdminCB for admin returned %v, want nil", err)
	}
	if !called {
		t.Error("requireAdminCB should invoke the wrapped handler for an admin")
	}
}

func TestAdminActionFilter(t *testing.T) {
	filter := adminActionFilter("stats")

	if !filter(&gotgbot.CallbackQuery{Data: callbacks.Admin{Action: "stats"}.Encode()}) {
		t.Error("adminActionFilter(stats) should match a stats payload")
	}
	if filter(&gotgbot.CallbackQuery{Data: callbacks.Admin{Action: "bc"}.Encode()}) {
		t.Error("adminActionFilter(stats) should not match a different admin action")
	}
	if filter(&gotgbot.CallbackQuery{Data: callbacks.Menu{Action: "add"}.Encode()}) {
		t.Error("adminActionFilter(stats) should not match a different prefix entirely")
	}
}

func TestAdminHomeText_ContainsCounts(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	if _, err := services.GetOrCreateUser(ctx, db, 5001, nil, "Admin", nil, false); err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	stats, err := services.GetSystemStats(ctx, db, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("GetSystemStats: %v", err)
	}

	text := adminHomeText(stats)
	if !strings.Contains(text, strconv.Itoa(stats.TotalUsers)) {
		t.Errorf("adminHomeText missing total user count: %q", text)
	}
	if !strings.Contains(text, "ניהול") {
		t.Errorf("adminHomeText missing expected Hebrew heading: %q", text)
	}
}

func TestAdminStatsFullText_ContainsKeyValueLines(t *testing.T) {
	stats := &services.SystemStats{
		TotalUsers: 3, ActiveUsers: 2, TotalEvents: 7,
		SentToday: 4, FailedToday: 1, DBSizeMB: 1.23,
		LastBackup: "2026-07-28 03:00", LastTickAt: "2026-07-29 08:00",
	}
	text := adminStatsFullText(stats)

	for _, want := range []string{
		"total_users: 3", "active_users: 2", "total_events: 7",
		"sent_today: 4", "failed_today: 1", "db_size_mb: 1.23",
		"last_backup: 2026-07-28 03:00", "last_tick_at: 2026-07-29 08:00",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("adminStatsFullText missing %q in:\n%s", want, text)
		}
	}
}

func TestAdminLogsCaption(t *testing.T) {
	if got := adminLogsCaption(50); !strings.Contains(got, "50") {
		t.Errorf("adminLogsCaption(50) = %q, want it to mention 50", got)
	}
}

func TestToggleBlockUser_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	admin, err := services.GetOrCreateUser(ctx, db, 5002, nil, "Admin", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser(admin): %v", err)
	}
	target, err := services.GetOrCreateUser(ctx, db, 5003, nil, "Target", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser(target): %v", err)
	}

	ok, err := services.ToggleBlockUser(ctx, db, admin.ID, target.ID, true)
	if err != nil || !ok {
		t.Fatalf("ToggleBlockUser(block) = %v, %v", ok, err)
	}
	info, err := services.GetUserInfo(ctx, db, strconv.FormatInt(target.ID, 10))
	if err != nil || info == nil || !info.IsBlocked {
		t.Fatalf("GetUserInfo after block: info=%+v err=%v", info, err)
	}

	ok, err = services.ToggleBlockUser(ctx, db, admin.ID, target.ID, false)
	if err != nil || !ok {
		t.Fatalf("ToggleBlockUser(unblock) = %v, %v", ok, err)
	}
	info, err = services.GetUserInfo(ctx, db, strconv.FormatInt(target.ID, 10))
	if err != nil || info == nil || info.IsBlocked {
		t.Fatalf("GetUserInfo after unblock: info=%+v err=%v", info, err)
	}

	ok, err = services.ToggleBlockUser(ctx, db, admin.ID, 999999, true)
	if err != nil || ok {
		t.Fatalf("ToggleBlockUser(missing user) = %v, %v, want false,nil", ok, err)
	}
}
