CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Fonction de calcul du temps de lecture
CREATE OR REPLACE FUNCTION calculate_reading_time()
RETURNS TRIGGER AS $$
DECLARE
    words_per_minute constant integer := 225;
    word_count integer;
BEGIN


    -- Si le contenu est vide ou NULL
    IF NEW.text_content IS NULL OR length(NEW.text_content) = 0 THEN
        NEW.reading_time_minutes := 0;
        RETURN NEW;
    END IF;

    -- Compte des mots
    word_count := array_length(regexp_split_to_array(TRIM(NEW.text_content), '\s+'), 1);

    -- Calcul et assignation
    NEW.reading_time_minutes := CEIL(word_count::float / words_per_minute);

    RETURN NEW;
END;
$$ language 'plpgsql';

-- Application des triggers sur les tables
CREATE TRIGGER update_users_modtime BEFORE UPDATE ON users FOR EACH ROW EXECUTE PROCEDURE update_updated_at_column();
CREATE TRIGGER update_articles_modtime BEFORE UPDATE ON articles FOR EACH ROW EXECUTE PROCEDURE update_updated_at_column();
CREATE TRIGGER update_links_modtime BEFORE UPDATE ON links FOR EACH ROW EXECUTE PROCEDURE update_updated_at_column();
-- Se déclenche avant INSERT ou UPDATE sur la table 'articles'
CREATE TRIGGER trigger_calc_reading_time 
BEFORE INSERT OR UPDATE ON articles 
FOR EACH ROW EXECUTE PROCEDURE calculate_reading_time();