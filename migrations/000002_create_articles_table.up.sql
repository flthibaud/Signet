CREATE TABLE IF NOT EXISTS articles (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  url TEXT NOT NULL,
	hash TEXT NOT NULL,
  
  -- Métadonnées du contenu
  title TEXT NOT NULL,
  description TEXT,
  author TEXT,
  image_url TEXT,
  page_type TEXT,
  reading_time_minutes REAL NOT NULL DEFAULT 0,
  
  -- Le contenu brut
  content TEXT, -- Le HTML épuré
  text_content TEXT NOT NULL, -- Le texte brut

  created_at timestamp with time zone DEFAULT now(),
  published_at timestamp with time zone
);

ALTER TABLE articles
ADD CONSTRAINT chk_reading_time CHECK (reading_time_minutes >= 0 AND reading_time_minutes <= 100);