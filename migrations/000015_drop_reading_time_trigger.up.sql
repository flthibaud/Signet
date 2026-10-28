-- Le temps de lecture est calculé par le fetcher (estimateReadingTime : 200 mots
-- par minute, plafonné à 500). Ce trigger l'écrasait en silence avec sa propre
-- formule, sans plafond : au-delà de 112 500 mots, CEIL(mots / 225) dépassait la
-- contrainte chk_reading_time, l'INSERT échouait, et l'item était re-scrapé à
-- chaque synchronisation sans jamais être enregistré.
DROP TRIGGER IF EXISTS trigger_calc_reading_time ON articles;
DROP FUNCTION IF EXISTS calculate_reading_time();
