DROP TRIGGER IF EXISTS update_opml_imports_modtime ON opml_imports;

DROP INDEX IF EXISTS idx_opml_imports_user;

DROP TABLE IF EXISTS opml_imports;
