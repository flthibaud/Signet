DROP TRIGGER IF EXISTS trigger_set_link_published_at ON links;
DROP FUNCTION IF EXISTS set_link_published_at();

-- Restauration de la version 000010 de la synchronisation.
CREATE OR REPLACE FUNCTION sync_links_published_at()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE links
    SET published_at = NEW.published_at
    WHERE article_id = NEW.id;
    RETURN NULL;
END;
$$ LANGUAGE 'plpgsql';

DROP TRIGGER IF EXISTS trigger_sync_links_published_at ON articles;

CREATE TRIGGER trigger_sync_links_published_at
  AFTER UPDATE OF published_at ON articles
  FOR EACH ROW
  WHEN (NEW.published_at IS NOT NULL AND OLD.published_at IS DISTINCT FROM NEW.published_at)
  EXECUTE FUNCTION sync_links_published_at();
