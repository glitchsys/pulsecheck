package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"pulsecheck/internal/config"
	"pulsecheck/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	cfg := config.Config{
		Driver:        "sqlite",
		SQLitePath:    filepath.Join(t.TempDir(), "pulsecheck.db"),
		BucketMinutes: 15,
		Services: []config.Service{
			{Slug: "api", Name: "API"},
			{Slug: "web", Name: "Web"},
		},
	}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestBucketStart(t *testing.T) {
	at := func(s string) int64 {
		t.Helper()
		tm, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return store.BucketStart(tm, 15)
	}
	if at("2026-09-23T12:00:00Z") != at("2026-09-23T12:14:59Z") {
		t.Fatal("expected the same 15-minute bucket")
	}
	if at("2026-09-23T12:14:59Z") == at("2026-09-23T12:15:00Z") {
		t.Fatal("expected a new bucket at :15")
	}
}

func TestSignalLevel(t *testing.T) {
	class, label := store.SignalLevel(2, 3, 10)
	if class != "normal" || label == "" {
		t.Fatal(class, label)
	}
	class, _ = store.SignalLevel(3, 3, 10)
	if class != "elevated" {
		t.Fatal(class)
	}
	class, _ = store.SignalLevel(10, 3, 10)
	if class != "high" {
		t.Fatal(class)
	}
}

func TestSubmitDedupeDashboardAndDelete(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	base := store.Submission{
		Slug:         "api",
		Category:     "down",
		Description:  "database is private",
		Email:        "hidden@example.com",
		IPHash:       "hash-a",
		Now:          now,
		DedupeWindow: 10 * time.Minute,
		EmailStatus:  store.EmailSkipped,
	}
	first, err := st.Submit(ctx, base)
	if err != nil || first.Duplicate || first.ID == 0 {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := st.Submit(ctx, base)
	if err != nil || !second.Duplicate {
		t.Fatalf("duplicate: %+v %v", second, err)
	}
	other := base
	other.Slug = "web"
	other.IPHash = "hash-b"
	if _, err := st.Submit(ctx, other); err != nil {
		t.Fatal(err)
	}
	later := base
	later.Now = now.Add(10 * time.Minute)
	later.IPHash = "hash-a"
	third, err := st.Submit(ctx, later)
	if err != nil || third.Duplicate {
		t.Fatalf("after window: %+v %v", third, err)
	}

	dash, err := st.Dashboard(ctx, "api", later.Now)
	if err != nil {
		t.Fatal(err)
	}
	var day int
	for _, svc := range dash.Services {
		if svc.Slug == "api" {
			day = svc.LastDay
		}
	}
	if day != 2 {
		t.Fatalf("api reports = %d, want 2 (duplicate must not count)", day)
	}

	if err := st.DeleteReport(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	dash, err = st.Dashboard(ctx, "api", later.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range dash.Services {
		if svc.Slug == "api" && svc.LastDay != 1 {
			t.Fatalf("after delete api reports = %d, want 1", svc.LastDay)
		}
	}

	reports, err := st.ListReports(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, rep := range reports {
		if rep.ID == first.ID {
			t.Fatal("deleted report still listed")
		}
	}
}
