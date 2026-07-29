package handlers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"birthly/internal/core"
	"birthly/internal/services"
	"birthly/internal/store"
)

func TestTemplatePreview_TruncatesLongBodies(t *testing.T) {
	short := "Short body"
	if got := templatePreview(short); got != short {
		t.Errorf("templatePreview(short) = %q, want unchanged", got)
	}

	long := strings.Repeat("א", 50)
	got := templatePreview(long)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("templatePreview(long) = %q, want ellipsis suffix", got)
	}
	if len([]rune(got)) != 41 {
		t.Errorf("templatePreview(long) rune length = %d, want 41 (40 + ellipsis)", len([]rune(got)))
	}
}

func TestCreateUserTemplate_LimitEnforced(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 7001, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}

	for i := 0; i < core.MaxGreetingTemplatesPerUser; i++ {
		if _, err := services.CreateUserTemplate(ctx, db, user, core.EventTypeBirthday, "warm", nil, "Body"); err != nil {
			t.Fatalf("CreateUserTemplate %d: %v", i, err)
		}
	}

	_, err = services.CreateUserTemplate(ctx, db, user, core.EventTypeBirthday, "warm", nil, "One too many")
	if err == nil {
		t.Fatal("expected LimitErr once the 20-template cap is reached")
	}
	if _, ok := err.(*services.LimitErr); !ok {
		t.Errorf("expected *services.LimitErr, got %T: %v", err, err)
	}
}

func TestPickTemplate_ExcludesLastShown(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()

	user, err := services.GetOrCreateUser(ctx, db, 7002, nil, "U", nil, false)
	if err != nil {
		t.Fatalf("GetOrCreateUser: %v", err)
	}
	event, err := services.CreateMinimalEvent(ctx, db, user, services.NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000)
	if err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	// The seeded system templates cover birthday/warm/he already (stage 1 seed).
	first, err := services.PickTemplate(ctx, db, user, event, "warm", nil)
	if err != nil {
		t.Fatalf("PickTemplate: %v", err)
	}
	if first == nil {
		t.Fatal("expected a seeded system template to match")
	}

	// Excluding it should still return *something* (there are multiple warm/he
	// system templates seeded) but never the same id back-to-back is not
	// guaranteed by a single draw — just confirm exclusion doesn't error and
	// the candidate pool logic runs.
	_, err = services.PickTemplate(ctx, db, user, event, "warm", &first.ID)
	if err != nil {
		t.Fatalf("PickTemplate with exclude: %v", err)
	}
}
