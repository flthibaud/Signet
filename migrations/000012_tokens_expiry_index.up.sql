-- Supports the scheduler's hourly sweep of expired tokens.
CREATE INDEX IF NOT EXISTS idx_tokens_expiry ON tokens(expiry);
