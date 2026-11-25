package version

import "testing"

func TestGet_UnstampedReportsDev(t *testing.T) {
	Version, Commit, Date = "", "", ""
	got := Get()
	if got.Version != "dev" || got.Commit != "dev" || got.Date != "dev" {
		t.Errorf("unstamped Get() = %+v, want all \"dev\"", got)
	}
}

func TestGet_ReportsInjectedValues(t *testing.T) {
	Version, Commit, Date = "v1.2.3", "abc1234", "2026-11-25T10:00:00Z"
	t.Cleanup(func() { Version, Commit, Date = "", "", "" })
	got := Get()
	if got.Version != "v1.2.3" || got.Commit != "abc1234" || got.Date != "2026-11-25T10:00:00Z" {
		t.Errorf("Get() = %+v", got)
	}
}
