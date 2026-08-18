package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

func TestPostgresStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("QH8Z_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("QH8Z_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	slug := fmt.Sprintf("test-%d", time.Now().UnixNano())
	created := core.Link{Slug: slug, URL: "https://example.com/durable", CreatedAt: time.Now().UTC()}
	if err := s.CreateLink(ctx, created); err != nil {
		t.Fatalf("create link: %v", err)
	}
	got, err := s.GetLink(ctx, slug)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if got.URL != created.URL || got.Visits != 0 {
		t.Fatalf("unexpected link: %+v", got)
	}

	count, err := s.RecordVisit(ctx, core.Visit{
		Slug:      slug,
		VisitedAt: time.Now().UTC(),
		Referer:   "https://ref.example/",
		UserAgent: "qh8z-test",
	})
	if err != nil {
		t.Fatalf("record visit: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	stats, err := s.Stats(ctx, slug, 10)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalVisits != 1 || len(stats.Recent) != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// Opening a second store re-runs the migration manager and proves migrations are idempotent.
	s2, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s2.Close()
	if _, err := s2.GetLink(ctx, slug); err != nil {
		t.Fatalf("link did not persist across connections: %v", err)
	}
}
