CREATE TABLE IF NOT EXISTS subscriptions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  feed_id BIGINT NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
  
  -- Personnalisation
  custom_title TEXT,   -- Si NULL, on affiche feeds.original_title
  custom_icon TEXT,    -- Emoji ou URL d'icone
  category TEXT,       -- Optionnel: "Tech", "News", "Dev"
  
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  
  -- Un utilisateur ne peut s'abonner qu'une fois au même flux
  UNIQUE(user_id, feed_id)
);