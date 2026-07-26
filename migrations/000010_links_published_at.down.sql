DROP TRIGGER IF EXISTS trigger_sync_links_published_at ON articles;
DROP FUNCTION IF EXISTS sync_links_published_at();
DROP INDEX IF EXISTS idx_links_user_published;
ALTER TABLE links DROP COLUMN IF EXISTS published_at;
