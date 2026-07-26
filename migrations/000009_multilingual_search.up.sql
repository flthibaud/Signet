CREATE EXTENSION IF NOT EXISTS unaccent;

DO $$
DECLARE
    cfg  text;
    stem text;
BEGIN
    FOREACH cfg IN ARRAY ARRAY(
        SELECT cfgname::text FROM pg_ts_config
        WHERE cfgname::text NOT LIKE '%\_ua'
        ORDER BY cfgname::text
    )
    LOOP
        CONTINUE WHEN EXISTS (
            SELECT 1 FROM pg_ts_config WHERE cfgname::text = cfg || '_ua'
        );

        stem := CASE WHEN cfg = 'simple' THEN 'simple' ELSE cfg || '_stem' END;

        EXECUTE format(
            'CREATE TEXT SEARCH CONFIGURATION public.%I (COPY = pg_catalog.%I)',
            cfg || '_ua', cfg);
        EXECUTE format(
            'ALTER TEXT SEARCH CONFIGURATION public.%I
                 ALTER MAPPING FOR hword, hword_part, word WITH unaccent, %I',
            cfg || '_ua', stem);
    END LOOP;
END $$;

ALTER TABLE articles
    ADD COLUMN language regconfig NOT NULL DEFAULT 'simple_ua';

DROP TRIGGER IF EXISTS trigger_update_tsv ON articles;
DROP FUNCTION IF EXISTS update_article_tsv();
DROP INDEX IF EXISTS idx_articles_tsv;
ALTER TABLE articles DROP COLUMN tsv;

ALTER TABLE articles
    ADD COLUMN tsv tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector(language, coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple_ua', coalesce(title, '')), 'A') ||
        setweight(to_tsvector(language, coalesce(description, '')), 'B') ||
        setweight(to_tsvector('simple_ua', coalesce(description, '')), 'B') ||
        setweight(to_tsvector(language, coalesce(text_content, '')), 'C') ||
        setweight(to_tsvector('simple_ua', coalesce(text_content, '')), 'C')
    ) STORED;

CREATE INDEX idx_articles_tsv ON articles USING GIN(tsv);
