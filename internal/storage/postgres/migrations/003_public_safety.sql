ALTER TABLE links
    ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS suspension_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS rate_limit_windows (
    bucket_key TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    request_count INTEGER NOT NULL CHECK (request_count >= 0),
    reset_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (bucket_key, window_start)
);

CREATE INDEX IF NOT EXISTS rate_limit_windows_reset_idx
    ON rate_limit_windows (reset_at);

CREATE TABLE IF NOT EXISTS url_rules (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    action TEXT NOT NULL CHECK (action IN ('allow', 'block')),
    match_type TEXT NOT NULL CHECK (match_type IN ('host', 'domain')),
    pattern TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (match_type, pattern)
);

CREATE TABLE IF NOT EXISTS abuse_reports (
    id TEXT PRIMARY KEY,
    slug VARCHAR(64) NOT NULL REFERENCES links(slug) ON DELETE CASCADE,
    destination_url TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('malware', 'phishing', 'scam', 'spam', 'other')),
    details TEXT NOT NULL DEFAULT '',
    reporter_email TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reviewed', 'resolved')),
    review_notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS abuse_reports_status_created_idx
    ON abuse_reports (status, created_at DESC, id DESC);
