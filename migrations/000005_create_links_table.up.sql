CREATE TABLE IF NOT EXISTS links (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
  feed_id BIGINT REFERENCES feeds(id) ON DELETE SET NULL, -- Optionnel si sauvé manuellement

  slug TEXT NOT NULL,
  
  -- États de lecture
  is_read BOOLEAN DEFAULT FALSE,
  is_starred BOOLEAN DEFAULT FALSE,
  reading_progress REAL DEFAULT 0 CHECK (reading_progress >= 0 AND reading_progress <= 1), -- 0.0 à 1.0
  reading_progress_anchor_index INTEGER NOT NULL DEFAULT 0, -- Index du paragraphe
  
  -- Timestamps
  saved_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  archived_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  
  UNIQUE(user_id, article_id),
  UNIQUE(user_id, slug)
);

-- Index de performance
CREATE INDEX idx_links_user_feed ON links(user_id, feed_id);
CREATE INDEX idx_links_user_saved ON links(user_id, saved_at DESC);
CREATE INDEX idx_links_user_unread ON links(user_id) WHERE is_read = FALSE;
CREATE INDEX idx_links_user_starred ON links(user_id) WHERE is_starred = TRUE;
CREATE INDEX idx_links_article ON links(article_id);