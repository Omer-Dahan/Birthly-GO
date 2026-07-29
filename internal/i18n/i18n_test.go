package i18n

import "testing"

func TestT_ReturnsHebrewByDefault(t *testing.T) {
	if got := T("common.home", "he", nil); got != "🏠 בית" {
		t.Errorf("T(common.home, he) = %q, want 🏠 בית", got)
	}
}

func TestT_FallsBackToHebrewForUnknownLanguage(t *testing.T) {
	if got := T("common.home", "fr", nil); got != "🏠 בית" {
		t.Errorf("T(common.home, fr) = %q, want 🏠 בית (Hebrew fallback)", got)
	}
}

func TestT_MissingKeyReturnsKeyItself(t *testing.T) {
	if got := T("does.not.exist", "he", nil); got != "does.not.exist" {
		t.Errorf("T(does.not.exist, he) = %q, want does.not.exist", got)
	}
}

func TestT_FormatsKwargs(t *testing.T) {
	got := T("error.too_long", "he", map[string]any{"max": 64})
	want := "זה ארוך מדי — עד 64 תווים."
	if got != want {
		t.Errorf("T(error.too_long, he, max=64) = %q, want %q", got, want)
	}
}

// TestHeEnKeyParity documents a pre-existing gap copied as-is from the
// Python locale files (not introduced by this port): he.json has 12
// gendered keys (_m/_f suffixes on add.date.title, card.age, delete.done,
// delete.restored, stats.oldest, stats.youngest) with no en.json
// counterpart, so an English-speaking user hitting one of those keys with a
// gender kwarg silently gets the Hebrew fallback text instead of English.
// This assertion intentionally records the *known* mismatch rather than
// failing the suite; app/i18n/translator.py has no bare-key fallback within
// the same language for these gendered entries either, so the original
// Python bot has the identical behavior today.
func TestHeEnKeyParity(t *testing.T) {
	ensureLoaded()
	mu.RLock()
	he, en := catalogs["he"], catalogs["en"]
	mu.RUnlock()

	wantMissingFromEn := map[string]bool{
		"add.date.title_f": true, "add.date.title_m": true,
		"card.age_f": true, "card.age_m": true,
		"delete.done_f": true, "delete.done_m": true,
		"delete.restored_f": true, "delete.restored_m": true,
		"stats.oldest_f": true, "stats.oldest_m": true,
		"stats.youngest_f": true, "stats.youngest_m": true,
	}

	var unexpectedlyMissing []string
	for k := range he {
		if _, inEn := en[k]; !inEn && !wantMissingFromEn[k] {
			unexpectedlyMissing = append(unexpectedlyMissing, k)
		}
	}
	if len(unexpectedlyMissing) > 0 {
		t.Errorf("he.json keys missing from en.json beyond the known gendered gap: %v", unexpectedlyMissing)
	}

	var unexpectedlyPresent []string
	for k := range wantMissingFromEn {
		if _, stillMissing := he[k]; !stillMissing {
			continue
		}
		if _, inEn := en[k]; inEn {
			unexpectedlyPresent = append(unexpectedlyPresent, k)
		}
	}
	if len(unexpectedlyPresent) > 0 {
		t.Errorf("locale files changed since this test was written — en.json now has: %v (update the known-gap list)", unexpectedlyPresent)
	}
}
