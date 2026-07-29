CREATE TABLE IF NOT EXISTS folders (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  UNIQUE (user_id, name)
);

CREATE INDEX idx_folders_user ON folders(user_id);

-- ON DELETE SET NULL et non CASCADE : supprimer un dossier ne doit jamais
-- désabonner. Un abonnement sans dossier (folder_id NULL) est affiché sous
-- "Uncategorized" ; ce n'est pas une ligne de la table.
ALTER TABLE subscriptions
  ADD COLUMN folder_id BIGINT REFERENCES folders(id) ON DELETE SET NULL;

CREATE INDEX idx_subscriptions_folder ON subscriptions(folder_id);
