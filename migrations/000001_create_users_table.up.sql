-- Active l'extension pour des recherches textuelles
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
-- Active l'extension pour les UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "citext";

CREATE TABLE IF NOT EXISTS users (
  id UUID NOT NULL DEFAULT uuid_generate_v1mc() PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  email citext NOT NULL UNIQUE,
  password_hash BYTEA NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
