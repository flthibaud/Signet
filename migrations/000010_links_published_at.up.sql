ALTER TABLE links ADD COLUMN published_at TIMESTAMP WITH TIME ZONE;

UPDATE links l
SET published_at = COALESCE(a.published_at, l.saved_at, NOW())
FROM articles a
WHERE a.id = l.article_id;

ALTER TABLE links ALTER COLUMN published_at SET NOT NULL;

CREATE INDEX idx_links_user_published ON links(user_id, published_at DESC);

CREATE OR REPLACE FUNCTION sync_links_published_at()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE links
    SET published_at = NEW.published_at
    WHERE article_id = NEW.id;
    RETURN NULL;
END;
$$ LANGUAGE 'plpgsql';

CREATE TRIGGER trigger_sync_links_published_at
  AFTER UPDATE OF published_at ON articles
  FOR EACH ROW
  WHEN (NEW.published_at IS NOT NULL AND OLD.published_at IS DISTINCT FROM NEW.published_at)
  EXECUTE FUNCTION sync_links_published_at();
