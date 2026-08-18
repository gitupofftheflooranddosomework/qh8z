package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

func TestMemoryStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	created := core.Link{Slug: "abc123", URL: "https://example.com/one", CreatedAt: time.Now().UTC()}
	if err := s.CreateLink(ctx, created); err != nil {
		t.Fatalf("create link: %v", err)
	}
	if err := s.CreateLink(ctx, created); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("duplicate create error = %v, want conflict", err)
	}

	got, err := s.GetLink(ctx, created.Slug)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if got.URL != created.URL || got.Visits != 0 {
		t.Fatalf("unexpected link: %+v", got)
	}

	visit := core.Visit{Slug: created.Slug, VisitedAt: time.Now().UTC(), Referer: "https://ref.example/", UserAgent: "test-agent"}
	count, err := s.RecordVisit(ctx, visit)
	if err != nil {
		t.Fatalf("record visit: %v", err)
	}
	if count != 1 {
		t.Fatalf("visit count = %d, want 1", count)
	}

	stats, err := s.Stats(ctx, created.Slug, 10)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalVisits != 1 || len(stats.Recent) != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
