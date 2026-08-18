CREATE TABLE IF NOT EXISTS links (
    slug VARCHAR(64) PRIMARY KEY,
    destination_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    visit_count BIGINT NOT NULL DEFAULT 0 CHECK (visit_count >= 0),
    CONSTRAINT links_slug_format CHECK (slug ~ '^[a-z0-9_-]{3,64}$')
);

CREATE TABLE IF NOT EXISTS visits (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug VARCHAR(64) NOT NULL REFERENCES links(slug) ON DELETE CASCADE,
    visited_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    referer TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS visits_slug_visited_at_idx
    ON visits (slug, visited_at DESC);
