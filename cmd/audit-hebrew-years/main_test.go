package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"birthly/internal/core"
	"birthly/internal/store"
	"birthly/internal/store/models"
	"birthly/internal/store/repo"
)

func seed(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	u, _, err := repo.NewUserRepo(db).GetOrCreate(ctx, 5001, nil, "T", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	add := func(name string, g time.Time) {
		_, hm, hd := core.ToHebrew(g)
		// Stored year as the old conversion wrote it: Gregorian year + 3760.
		y := g.Year() + 3760
		occ, err := core.NextOccurrence(core.CalendarTypeHebrew, hm, hd, time.Now().UTC(), core.AdarPolicyAdarII, core.Feb29PolicyFeb28)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.NewEventRepo(db, u.ID).Create(ctx, &models.Event{
			EventType: core.EventTypeBirthday, FirstName: name, Category: core.CategoryOther,
			CalendarType: core.CalendarTypeHebrew, Year: &y, Month: hm, Day: hd, IsActive: true, NextOccurrence: &occ,
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("Tevet", time.Date(1983, 12, 30, 0, 0, 0, 0, time.UTC)) // Tevet, December
	add("Elul", time.Date(1984, 8, 30, 0, 0, 0, 0, time.UTC))   // Elul, before Rosh Hashana
}

func TestReportClassifiesBothClasses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.db")
	seed(t, path)
	var buf bytes.Buffer
	if err := run([]string{"--db-path", path}, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	var tevet, elul string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, `"Tevet"`) {
			tevet = l
		}
		if strings.Contains(l, `"Elul"`) {
			elul = l
		}
	}
	if !strings.Contains(tevet, "undecidable") || !strings.Contains(tevet, "candidates") || strings.Count(tevet, "-") < 4 {
		t.Fatalf("tevet row: %q", tevet)
	}
	if !strings.Contains(elul, "trustworthy") || strings.Contains(elul, "candidates") {
		t.Fatalf("elul row: %q", elul)
	}
	if !strings.Contains(out, "summary: trustworthy=1 undecidable=1") {
		t.Fatalf("summary: %s", out)
	}
}

func TestReadOnlyConnectionRejectsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.db")
	seed(t, path)
	db, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE events SET year = year + 1`); err == nil {
		t.Fatal("write succeeded on read-only connection")
	}
	if _, err := db.Exec(`CREATE TABLE x (a int)`); err == nil {
		t.Fatal("DDL succeeded on read-only connection")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("read: n=%d err=%v", n, err)
	}
}
