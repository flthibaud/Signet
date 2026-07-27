-- links.published_at duplicates articles.published_at so the article river can
-- sort straight off idx_links_user_published. Two holes were left in that copy:
-- nothing filled it on INSERT except every caller remembering to, and the sync
-- trigger ignored an article whose date was cleared.

-- Remplissage de la copie à l'insertion, en un seul endroit.
CREATE OR REPLACE FUNCTION set_link_published_at()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.published_at IS NULL THEN
        SELECT a.published_at INTO NEW.published_at
        FROM articles a
        WHERE a.id = NEW.article_id;
    END IF;

    -- articles.published_at is nullable, this copy is NOT NULL. Fall back to the
    -- save time rather than a bare NOW(): saved_at is already on the row, so a
    -- re-run or a backfill lands on the same value instead of drifting.
    NEW.published_at := COALESCE(NEW.published_at, NEW.saved_at, NOW());

    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE TRIGGER trigger_set_link_published_at
  BEFORE INSERT ON links
  FOR EACH ROW EXECUTE FUNCTION set_link_published_at();

-- Resynchronisation sur tout changement de la colonne source, effacement inclus.
CREATE OR REPLACE FUNCTION sync_links_published_at()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE links
    SET published_at = COALESCE(NEW.published_at, saved_at)
    WHERE article_id = NEW.id
      AND published_at IS DISTINCT FROM COALESCE(NEW.published_at, saved_at);
    RETURN NULL;
END;
$$ LANGUAGE 'plpgsql';

DROP TRIGGER IF EXISTS trigger_sync_links_published_at ON articles;

CREATE TRIGGER trigger_sync_links_published_at
  AFTER UPDATE OF published_at ON articles
  FOR EACH ROW
  WHEN (OLD.published_at IS DISTINCT FROM NEW.published_at)
  EXECUTE FUNCTION sync_links_published_at();

-- Réalignement des lignes que l'ancien trigger ne pouvait pas rattraper.
UPDATE links l
SET published_at = COALESCE(a.published_at, l.saved_at)
FROM articles a
WHERE a.id = l.article_id
  AND l.published_at IS DISTINCT FROM COALESCE(a.published_at, l.saved_at);
