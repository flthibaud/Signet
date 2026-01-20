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

-- Calcul automatique du temps de lecture
CREATE TRIGGER trigger_calc_reading_time 
  BEFORE INSERT OR UPDATE ON articles 
  FOR EACH ROW EXECUTE FUNCTION calculate_reading_time();

-- Mise à jour automatique du vecteur de recherche
CREATE TRIGGER trigger_update_tsv
  BEFORE INSERT OR UPDATE ON articles
  FOR EACH ROW EXECUTE FUNCTION update_article_tsv();