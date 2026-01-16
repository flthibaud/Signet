CREATE TABLE IF NOT EXISTS feeds (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    original_title TEXT, -- Titre fourni par le XML (ex: "Le Monde - Une")
    site_url TEXT,       -- Lien vers le site web (pas le RSS)
    last_fetched_at TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT TRUE,
    image_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);