CREATE TABLE IF NOT EXISTS tokens (
  hash BYTEA PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expiry TIMESTAMP(0) WITH TIME ZONE NOT NULL,
  scope TEXT NOT NULL
);

CREATE INDEX idx_tokens_user_scope ON tokens(user_id, scope);