CREATE TABLE IF NOT EXISTS feeds (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  url TEXT NOT NULL UNIQUE,
  original_title TEXT,
  site_url TEXT,
  image_url TEXT,
  last_fetched_at TIMESTAMP WITH TIME ZONE,
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_feeds_url ON feeds(url);
CREATE INDEX idx_feeds_last_fetched ON feeds(last_fetched_at) WHERE is_active = TRUE;