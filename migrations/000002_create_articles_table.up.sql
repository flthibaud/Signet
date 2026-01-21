CREATE TABLE IF NOT EXISTS articles (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  url TEXT NOT NULL,
  hash TEXT NOT NULL UNIQUE,
  
  -- Métadonnées du contenu
  title TEXT NOT NULL,
  description TEXT,
  author TEXT,
  image_url TEXT,
  page_type TEXT, -- 'article', 'video', 'pdf', etc.
  reading_time_minutes REAL NOT NULL DEFAULT 0,
  
  -- Le contenu
  original_html TEXT, -- HTML brut (pour debug/fallback)
  text_content TEXT NOT NULL, -- Texte brut pour recherche
  
  -- Recherche full-text
  tsv TSVECTOR,
  
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  published_at TIMESTAMP WITH TIME ZONE
);

ALTER TABLE articles
  ADD CONSTRAINT chk_reading_time 
  CHECK (reading_time_minutes >= 0 AND reading_time_minutes <= 500);

CREATE INDEX idx_articles_hash ON articles(hash);
CREATE INDEX idx_articles_tsv ON articles USING GIN(tsv);