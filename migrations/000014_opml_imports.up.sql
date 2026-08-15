-- Un import OPML dure trop longtemps pour tenir dans une requête HTTP : le job
-- est persisté ici et le front interroge sa progression. La ligne survit à la
-- fin du job, c'est elle qui porte le bilan affiché à l'utilisateur.
CREATE TABLE IF NOT EXISTS opml_imports (
  id          UUID NOT NULL DEFAULT uuid_generate_v1mc() PRIMARY KEY,
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status      TEXT NOT NULL DEFAULT 'running', -- running | completed | interrupted
  total       INT NOT NULL DEFAULT 0,
  processed   INT NOT NULL DEFAULT 0,
  imported    INT NOT NULL DEFAULT 0,
  skipped     INT NOT NULL DEFAULT 0,
  failed      INT NOT NULL DEFAULT 0,
  results     JSONB NOT NULL DEFAULT '[]'::jsonb, -- [{url, title, status, reason}]
  created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  finished_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_opml_imports_user ON opml_imports(user_id, created_at DESC);

-- updated_at sert de battement de cœur : FailStale s'en sert pour repérer les
-- jobs dont le processus est mort en cours de route.
CREATE TRIGGER update_opml_imports_modtime
  BEFORE UPDATE ON opml_imports
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
