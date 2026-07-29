DROP INDEX IF EXISTS idx_subscriptions_folder;

ALTER TABLE subscriptions DROP COLUMN IF EXISTS folder_id;

DROP INDEX IF EXISTS idx_folders_user;

DROP TABLE IF EXISTS folders;
