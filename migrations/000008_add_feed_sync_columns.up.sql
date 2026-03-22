ALTER TABLE feeds ADD COLUMN http_etag TEXT NOT NULL DEFAULT '';
ALTER TABLE feeds ADD COLUMN http_last_modified TEXT NOT NULL DEFAULT '';
ALTER TABLE feeds ADD COLUMN fetching_since TIMESTAMP WITH TIME ZONE;
ALTER TABLE feeds ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_feeds_sync ON feeds(last_fetched_at ASC NULLS FIRST)
  WHERE is_active = TRUE AND fetching_since IS NULL;
