CREATE TABLE IF NOT EXISTS links (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    article_id bigint NOT NULL REFERENCES articles(id) ON DELETE CASCADE,

    slug TEXT NOT NULL,
    article_hash TEXT NOT NULL,
    article_url TEXT NOT NULL,
    
    -- États de lecture
    is_read boolean DEFAULT false,
    is_starred boolean DEFAULT false,
    article_reading_progress_anchor_index INTEGER NOT NULL DEFAULT 0,
    archived_at timestamp with time zone,
    saved_at timestamp with time zone DEFAULT now(),

    feed_id bigint REFERENCES feeds(id) ON DELETE CASCADE,

    created_at timestamp with time zone DEFAULT now(),
    published_at timestamp with time zone,
    
    UNIQUE(user_id, article_id)
);

-- Index de performance
CREATE INDEX idx_links_user_feed ON links(user_id, feed_id);
CREATE INDEX idx_links_user_saved ON links(user_id, saved_at DESC);
CREATE INDEX idx_links_user_unread ON links(user_id) WHERE is_read = FALSE;