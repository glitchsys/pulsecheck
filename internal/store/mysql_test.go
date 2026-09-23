package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"pulsecheck/internal/config"
	"pulsecheck/internal/store"
)

func TestMySQLSubmitDedupeAndDelete(t *testing.T) {
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN not set")
	}
	cfg := config.Config{
		Driver:        "mysql",
		DatabaseURL:   dsn,
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

	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	sub := store.Submission{
		Slug:         "api",
		Category:     "slow",
		Description:  "mysql private text",
		Email:        "mysql-reporter@example.com",
		IPHash:       "mysql-hash-a",
		Now:          now,
		DedupeWindow: 10 * time.Minute,
		EmailStatus:  store.EmailPending,
	}
	first, err := st.Submit(ctx, sub)
	if err != nil || first.Duplicate {
		t.Fatalf("first: %+v %v", first, err)
	}
	dup, err := st.Submit(ctx, sub)
	if err != nil || !dup.Duplicate {
		t.Fatalf("duplicate: %+v %v", dup, err)
	}
	if err := st.SetEmailStatus(ctx, first.ID, store.EmailFailed, "dial failed"); err != nil {
		t.Fatal(err)
	}
	dash, err := st.Dashboard(ctx, "api", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range dash.Services {
		if svc.Slug == "api" && svc.LastDay != 1 {
			t.Fatalf("api count = %d, want 1", svc.LastDay)
		}
	}
	if err := st.DeleteReport(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	dash, err = st.Dashboard(ctx, "", now)
	if err != nil {
		t.Fatal(err)
	}
	var day int
	for _, svc := range dash.Services {
		day += svc.LastDay
	}
	if day != 0 {
		t.Fatalf("count after delete = %d", day)
	}
}
