package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required for postgres storage")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version := name
		var applied bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			continue
		}
		sqlBytes, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
	}
	return nil
}

func (s *Store) CreateLink(ctx context.Context, link core.Link) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO links (slug, destination_url, created_at, visit_count) VALUES ($1, $2, $3, $4)`,
		link.Slug, link.URL, link.CreatedAt, link.Visits,
	)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return core.ErrConflict
	}
	return fmt.Errorf("create link: %w", err)
}

func (s *Store) GetLink(ctx context.Context, slug string) (core.Link, error) {
	var link core.Link
	err := s.db.QueryRowContext(ctx,
		`SELECT slug, destination_url, created_at, visit_count FROM links WHERE slug = $1`, slug,
	).Scan(&link.Slug, &link.URL, &link.CreatedAt, &link.Visits)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Link{}, core.ErrNotFound
	}
	if err != nil {
		return core.Link{}, fmt.Errorf("get link: %w", err)
	}
	return link, nil
}

func (s *Store) RecordVisit(ctx context.Context, visit core.Visit) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin visit transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int64
	err = tx.QueryRowContext(ctx,
		`UPDATE links SET visit_count = visit_count + 1 WHERE slug = $1 RETURNING visit_count`, visit.Slug,
	).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, core.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("increment visit count: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO visits (slug, visited_at, referer, user_agent) VALUES ($1, $2, $3, $4)`,
		visit.Slug, visit.VisitedAt, visit.Referer, visit.UserAgent,
	); err != nil {
		return 0, fmt.Errorf("insert visit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit visit: %w", err)
	}
	return count, nil
}

func (s *Store) Stats(ctx context.Context, slug string, recentLimit int) (core.Stats, error) {
	if recentLimit < 0 {
		recentLimit = 0
	}
	if recentLimit > 100 {
		recentLimit = 100
	}
	var stats core.Stats
	if err := s.db.QueryRowContext(ctx, `SELECT visit_count FROM links WHERE slug = $1`, slug).Scan(&stats.TotalVisits); errors.Is(err, sql.ErrNoRows) {
		return core.Stats{}, core.ErrNotFound
	} else if err != nil {
		return core.Stats{}, fmt.Errorf("read visit count: %w", err)
	}
	if recentLimit == 0 {
		return stats, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT visited_at, referer, user_agent FROM visits WHERE slug = $1 ORDER BY visited_at DESC, id DESC LIMIT $2`,
		slug, recentLimit,
	)
	if err != nil {
		return core.Stats{}, fmt.Errorf("read visits: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var visit core.Visit
		visit.Slug = slug
		if err := rows.Scan(&visit.VisitedAt, &visit.Referer, &visit.UserAgent); err != nil {
			return core.Stats{}, fmt.Errorf("scan visit: %w", err)
		}
		stats.Recent = append(stats.Recent, visit)
	}
	if err := rows.Err(); err != nil {
		return core.Stats{}, fmt.Errorf("iterate visits: %w", err)
	}
	return stats, nil
}

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Close() error                   { return s.db.Close() }
