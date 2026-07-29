package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"birthly/internal/services"
	"birthly/internal/store"
	"birthly/internal/store/repo"
)

func TestToggleSettingsPersist(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 6001, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	if !user.ShowHebrewDate {
		t.Fatal("expected ShowHebrewDate default true")
	}

	user.ShowHebrewDate = !user.ShowHebrewDate
	if err := services.UpdateSettings(ctx, db, user); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	reloaded, err := repo.NewUserRepo(db).Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reloaded.ShowHebrewDate {
		t.Error("expected ShowHebrewDate=false after toggle+persist")
	}
}

func TestWipeAccount_RemovesUser(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 6002, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	if _, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 1, Day: 1}, 1000); err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	if err := services.WipeAccount(ctx, db, user); err != nil {
		t.Fatalf("WipeAccount: %v", err)
	}

	gone, err := repo.NewUserRepo(db).Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gone != nil {
		t.Error("expected user row to be gone after wipe")
	}

	var eventCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE user_id = ?`, user.ID).Scan(&eventCount); err != nil {
		t.Fatalf("querying events: %v", err)
	}
	if eventCount != 0 {
		t.Errorf("expected events cascade-deleted, found %d", eventCount)
	}
}

func TestValidateTimezone_ValidAndInvalid(t *testing.T) {
	if _, err := services.ValidateTimezone("Not/AZone"); err == nil {
		t.Error("expected error for invalid timezone")
	}
	if tz, err := services.ValidateTimezone("Europe/London"); err != nil || tz != "Europe/London" {
		t.Errorf("ValidateTimezone(Europe/London) = (%q, %v)", tz, err)
	}
}
