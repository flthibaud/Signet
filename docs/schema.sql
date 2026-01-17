-- Active l'extension pour des recherches textuelles
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
-- Active l'extension pour les UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "citext";

-- ============================================================================
-- USERS & AUTH
-- ============================================================================

CREATE TABLE IF NOT EXISTS users (
  id UUID NOT NULL DEFAULT uuid_generate_v1mc() PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  email citext NOT NULL UNIQUE,
  password_hash BYTEA NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tokens (
  hash BYTEA PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expiry TIMESTAMP(0) WITH TIME ZONE NOT NULL,
  scope TEXT NOT NULL
);

-- ============================================================================
-- ARTICLES (contenu partagé entre tous les users)
-- ============================================================================

CREATE TABLE IF NOT EXISTS articles (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  url TEXT NOT NULL,
  hash TEXT NOT NULL UNIQUE, -- Hash de l'URL pour déduplication
  
  -- Métadonnées du contenu
  title TEXT NOT NULL,
  description TEXT,
  author TEXT,
  image_url TEXT,
  page_type TEXT, -- 'article', 'video', 'pdf', etc.
  reading_time_minutes REAL NOT NULL DEFAULT 0,
  
  -- Le contenu
  original_html TEXT, -- HTML brut (pour debug/fallback)
  content TEXT, -- HTML épuré/nettoyé
  text_content TEXT NOT NULL, -- Texte brut pour recherche
  
  -- Recherche full-text
  tsv TSVECTOR,
  
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  published_at TIMESTAMP WITH TIME ZONE
);

-- ============================================================================
-- FEEDS RSS/ATOM
-- ============================================================================

CREATE TABLE IF NOT EXISTS feeds (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  url TEXT NOT NULL UNIQUE,
  original_title TEXT, -- Titre fourni par le XML (ex: "Le Monde - Une")
  site_url TEXT, -- Lien vers le site web (pas le RSS)
  image_url TEXT,
  last_fetched_at TIMESTAMP WITH TIME ZONE,
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subscriptions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  feed_id BIGINT NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
  
  -- Personnalisation
  custom_title TEXT, -- Si NULL, on affiche feeds.original_title
  custom_icon TEXT, -- Emoji ou URL d'icône
  category TEXT, -- Optionnel: "Tech", "News", "Dev"
  
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  
  -- Un utilisateur ne peut s'abonner qu'une fois au même flux
  UNIQUE(user_id, feed_id)
);

-- ============================================================================
-- LINKS (articles sauvés par user)
-- ============================================================================

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
  
  UNIQUE(user_id, article_id)
);

-- ============================================================================
-- LABELS (tags)
-- ============================================================================

CREATE TABLE IF NOT EXISTS labels (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  color TEXT DEFAULT '#808080',
  description TEXT,
  position INTEGER NOT NULL DEFAULT 0, -- Pour l'ordre d'affichage
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  
  UNIQUE(user_id, name)
);

CREATE TABLE IF NOT EXISTS link_labels (
  link_id BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  label_id BIGINT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  
  PRIMARY KEY(link_id, label_id)
);

-- ============================================================================
-- HIGHLIGHTS (annotations)
-- ============================================================================

CREATE TABLE IF NOT EXISTS highlights (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  link_id BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  
  quote TEXT NOT NULL, -- Texte surligné
  annotation TEXT, -- Note personnelle de l'utilisateur
  color TEXT DEFAULT '#FFEB3B', -- Couleur du surlignage
  
  -- Position dans le texte (optionnel, pour ancrage précis)
  position_start INTEGER,
  position_end INTEGER,
  
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ============================================================================
-- CONSTRAINTS
-- ============================================================================

ALTER TABLE articles
  ADD CONSTRAINT chk_reading_time 
  CHECK (reading_time_minutes >= 0 AND reading_time_minutes <= 500);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Articles
CREATE INDEX idx_articles_hash ON articles(hash);
CREATE INDEX idx_articles_tsv ON articles USING GIN(tsv);

-- Feeds
CREATE INDEX idx_feeds_url ON feeds(url);
CREATE INDEX idx_feeds_last_fetched ON feeds(last_fetched_at) WHERE is_active = TRUE;

-- Subscriptions
CREATE INDEX idx_subscriptions_user ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_feed ON subscriptions(feed_id);

-- Links
CREATE INDEX idx_links_user_feed ON links(user_id, feed_id);
CREATE INDEX idx_links_user_saved ON links(user_id, saved_at DESC);
CREATE INDEX idx_links_user_unread ON links(user_id) WHERE is_read = FALSE;
CREATE INDEX idx_links_user_starred ON links(user_id) WHERE is_starred = TRUE;
CREATE INDEX idx_links_article ON links(article_id);

-- Labels
CREATE INDEX idx_labels_user ON labels(user_id);
CREATE INDEX idx_link_labels_link ON link_labels(link_id);
CREATE INDEX idx_link_labels_label ON link_labels(label_id);

-- Highlights
CREATE INDEX idx_highlights_user ON highlights(user_id);
CREATE INDEX idx_highlights_link ON highlights(link_id);

-- Tokens
CREATE INDEX idx_tokens_user_scope ON tokens(user_id, scope);

-- ============================================================================
-- FUNCTIONS
-- ============================================================================

-- Fonction de mise à jour automatique de updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

-- Fonction de calcul du temps de lecture
CREATE OR REPLACE FUNCTION calculate_reading_time()
RETURNS TRIGGER AS $$
DECLARE
    words_per_minute CONSTANT INTEGER := 225;
    word_count INTEGER;
BEGIN
    -- Si le contenu est vide ou NULL
    IF NEW.text_content IS NULL OR LENGTH(NEW.text_content) = 0 THEN
        NEW.reading_time_minutes := 0;
        RETURN NEW;
    END IF;

    -- Compte des mots
    word_count := array_length(regexp_split_to_array(TRIM(NEW.text_content), '\s+'), 1);

    -- Calcul et assignation
    NEW.reading_time_minutes := CEIL(word_count::FLOAT / words_per_minute);

    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

-- Fonction pour mettre à jour le vecteur de recherche full-text
CREATE OR REPLACE FUNCTION update_article_tsv()
RETURNS TRIGGER AS $$
BEGIN
    NEW.tsv := 
        setweight(to_tsvector('french', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('french', COALESCE(NEW.description, '')), 'B') ||
        setweight(to_tsvector('french', COALESCE(NEW.text_content, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Mise à jour automatique de updated_at
CREATE TRIGGER update_users_modtime 
  BEFORE UPDATE ON users 
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_articles_modtime 
  BEFORE UPDATE ON articles 
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_links_modtime 
  BEFORE UPDATE ON links 
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_highlights_modtime 
  BEFORE UPDATE ON highlights 
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Calcul automatique du temps de lecture
CREATE TRIGGER trigger_calc_reading_time 
  BEFORE INSERT OR UPDATE ON articles 
  FOR EACH ROW EXECUTE FUNCTION calculate_reading_time();

-- Mise à jour automatique du vecteur de recherche
CREATE TRIGGER trigger_update_tsv
  BEFORE INSERT OR UPDATE ON articles
  FOR EACH ROW EXECUTE FUNCTION update_article_tsv();