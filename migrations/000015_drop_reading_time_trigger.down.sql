CREATE OR REPLACE FUNCTION calculate_reading_time()
RETURNS TRIGGER AS $$
DECLARE
    words_per_minute CONSTANT INTEGER := 225;
    word_count INTEGER;
BEGIN
    IF NEW.text_content IS NULL OR LENGTH(NEW.text_content) = 0 THEN
        NEW.reading_time_minutes := 0;
        RETURN NEW;
    END IF;

    word_count := array_length(regexp_split_to_array(TRIM(NEW.text_content), '\s+'), 1);
    NEW.reading_time_minutes := CEIL(word_count::FLOAT / words_per_minute);

    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE TRIGGER trigger_calc_reading_time
  BEFORE INSERT OR UPDATE ON articles
  FOR EACH ROW EXECUTE FUNCTION calculate_reading_time();
