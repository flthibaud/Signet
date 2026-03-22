DROP INDEX IF EXISTS idx_feeds_sync;
ALTER TABLE feeds DROP COLUMN IF EXISTS http_etag;
ALTER TABLE feeds DROP COLUMN IF EXISTS http_last_modified;
ALTER TABLE feeds DROP COLUMN IF EXISTS fetching_since;
ALTER TABLE feeds DROP COLUMN IF EXISTS consecutive_failures;
