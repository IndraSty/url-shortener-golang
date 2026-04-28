-- Users table
-- api_key is stored as SHA-256 hash — plaintext is shown once at creation and never stored
-- plan controls feature access (free tier: limited links, no A/B testing)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,              -- bcrypt hash
    api_key       VARCHAR(64)  NOT NULL UNIQUE,       -- SHA-256 hex hash (64 chars)
    plan          VARCHAR(20)  NOT NULL DEFAULT 'free'
                  CHECK (plan IN ('free', 'pro')),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Index for API key lookups on every authenticated request
CREATE INDEX IF NOT EXISTS idx_users_api_key ON users (api_key);