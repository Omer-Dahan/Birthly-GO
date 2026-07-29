package services

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExportImportJSON_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 7001)

	year := 1990
	if _, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "Dana", LastName: strPtr("Cohen"), Month: 3, Day: 15, Year: &year}, 1000); err != nil {
		t.Fatalf("CreateMinimalEvent: %v", err)
	}

	data, err := ExportJSON(ctx, db, user)
	if err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	if !bytes.Contains(data, []byte("Dana")) {
		t.Errorf("exported JSON missing event data: %s", data)
	}
	if bytes.Contains(data, []byte(`\u`)) {
		t.Errorf("exported JSON should not escape unicode: %s", data)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("exported JSON is not valid JSON: %v", err)
	}
	if decoded["schema_version"].(float64) != 1 {
		t.Errorf("schema_version = %v, want 1", decoded["schema_version"])
	}

	// Import into a second, fresh user — full round trip.
	user2 := mustUser(t, ctx, db, 7002)
	payload, parseResult, err := ParseImportJSON(data)
	if err != nil {
		t.Fatalf("ParseImportJSON: %v", err)
	}
	if parseResult.TotalParsed != 1 {
		t.Fatalf("TotalParsed = %d, want 1", parseResult.TotalParsed)
	}

	importResult, err := DoImport(ctx, db, user2, payload, "add")
	if err != nil {
		t.Fatalf("DoImport: %v", err)
	}
	if importResult.Imported != 1 || importResult.Duplicates != 0 || len(importResult.Errors) != 0 {
		t.Errorf("unexpected import result: %+v", importResult)
	}

	events, err := GetUserStats(ctx, db, user2)
	if err != nil {
		t.Fatalf("GetUserStats: %v", err)
	}
	if events.Total != 1 {
		t.Errorf("user2 total events = %d, want 1", events.Total)
	}
}

func TestDoImport_DuplicateDetection(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 7003)

	if _, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "Yossi", Month: 5, Day: 5}, 1000); err != nil {
		t.Fatal(err)
	}

	payload := map[string]any{
		"events": []any{
			map[string]any{"first_name": "yossi", "month": float64(5), "day": float64(5)}, // case-insensitive dup
			map[string]any{"first_name": "Miri", "month": float64(6), "day": float64(6)},  // new
		},
	}

	result, err := DoImport(ctx, db, user, payload, "add")
	if err != nil {
		t.Fatalf("DoImport: %v", err)
	}
	if result.Duplicates != 1 || result.Imported != 1 {
		t.Errorf("result = %+v, want 1 duplicate, 1 imported", result)
	}
}

func TestDoImport_BadRowsAccumulateErrors(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 7004)

	payload := map[string]any{
		"events": []any{
			map[string]any{"first_name": "", "month": float64(1), "day": float64(1)},                              // missing name
			map[string]any{"first_name": "X", "month": float64(99), "day": float64(1)},                            // bad month
			map[string]any{"first_name": "Y", "month": float64(1), "day": float64(1), "calendar_type": "martian"}, // bad calendar
			map[string]any{"first_name": "Good", "month": float64(1), "day": float64(1)},                          // valid
		},
	}

	result, err := DoImport(ctx, db, user, payload, "add")
	if err != nil {
		t.Fatalf("DoImport: %v", err)
	}
	if result.Imported != 1 {
		t.Errorf("Imported = %d, want 1", result.Imported)
	}
	if len(result.Errors) != 3 {
		t.Errorf("Errors = %v, want 3 entries", result.Errors)
	}
}

func TestDoImport_ReplaceModeSoftDeletesExisting(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 7005)

	if _, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "Old", Month: 1, Day: 1}, 1000); err != nil {
		t.Fatal(err)
	}

	payload := map[string]any{
		"events": []any{
			map[string]any{"first_name": "New", "month": float64(2), "day": float64(2)},
		},
	}
	if _, err := DoImport(ctx, db, user, payload, "replace"); err != nil {
		t.Fatalf("DoImport replace: %v", err)
	}

	stats, err := GetUserStats(ctx, db, user)
	if err != nil {
		t.Fatalf("GetUserStats: %v", err)
	}
	if stats.Total != 1 {
		t.Errorf("Total after replace = %d, want 1 (old soft-deleted, not counted)", stats.Total)
	}
}

func TestExportCSV_HasBOMAndHebrewHeaders(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 7006)
	if _, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000); err != nil {
		t.Fatal(err)
	}

	data, err := ExportCSV(ctx, db, user)
	if err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		t.Error("CSV export missing UTF-8 BOM prefix")
	}
	if !strings.Contains(string(data), csvHeaders[0]) {
		t.Errorf("CSV export missing Hebrew header %q", csvHeaders[0])
	}
	if !strings.Contains(string(data), "Dana") {
		t.Error("CSV export missing event data")
	}
}

func TestExportXLSX_ProducesValidWorkbook(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, ctx, db, 7007)
	if _, err := CreateMinimalEvent(ctx, db, user, NewEventInput{FirstName: "Dana", Month: 3, Day: 15}, 1000); err != nil {
		t.Fatal(err)
	}

	data, err := ExportXLSX(ctx, db, user)
	if err != nil {
		t.Fatalf("ExportXLSX: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("exported XLSX is not a valid workbook: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("sheet count = %d, want 2", len(sheets))
	}

	rows, err := f.GetRows("אירועים")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) != 2 { // header + 1 event
		t.Fatalf("events sheet rows = %d, want 2", len(rows))
	}
	if rows[1][0] != "Dana" {
		t.Errorf("events sheet row 2 col 1 = %q, want Dana", rows[1][0])
	}
}

func strPtr(s string) *string { return &s }
