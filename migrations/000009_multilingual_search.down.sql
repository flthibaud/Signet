-- Retour au tsvector `french` maintenu par trigger (état de la migration 000007).

DROP INDEX IF EXISTS idx_articles_tsv;
ALTER TABLE articles DROP COLUMN IF EXISTS tsv;
ALTER TABLE articles DROP COLUMN IF EXISTS language;

ALTER TABLE articles ADD COLUMN tsv TSVECTOR;

CREATE OR REPLACE FUNCTION update_article_tsv()
RETURNS TRIGGER AS $$
BEGIN
    NEW.tsv :=
        setweight(to_tsvector('french', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('french', COALESCE(NEW.description, '')), 'B') ||
        setweight(to_tsvector('french', COALESCE(NEW.text_content, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE TRIGGER trigger_update_tsv
  BEFORE INSERT OR UPDATE ON articles
  FOR EACH ROW EXECUTE FUNCTION update_article_tsv();

-- Le trigger ne s'applique qu'aux écritures : il faut repeupler l'existant.
UPDATE articles SET tsv = NULL;

CREATE INDEX idx_articles_tsv ON articles USING GIN(tsv);

DROP TEXT SEARCH CONFIGURATION IF EXISTS public.simple_ua;

DO $$
DECLARE
    cfg text;
BEGIN
    FOREACH cfg IN ARRAY ARRAY(
        SELECT cfgname::text FROM pg_ts_config
        WHERE cfgname::text LIKE '%\_ua'
        ORDER BY cfgname::text
    )
    LOOP
        EXECUTE format('DROP TEXT SEARCH CONFIGURATION IF EXISTS public.%I', cfg);
    END LOOP;
END $$;

DROP EXTENSION IF EXISTS unaccent;
