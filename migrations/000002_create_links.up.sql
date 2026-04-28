-- Links table
-- id is bigserial — the numeric ID is base62-encoded to produce the short slug
-- slug can also be a custom value provided by the user
-- click_count is a denormalized counter for fast read; authoritative count is in click_events
-- password_hash is nullable — only set when link is password-protected
CREATE TABLE IF NOT EXISTS links (
    id               BIGSERIAL    PRIMARY KEY,
    user_id          UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    slug             VARCHAR(20)  NOT NULL UNIQUE,
    destination_url  TEXT         NOT NULL,
    title            VARCHAR(255),
    password_hash    VARCHAR(255),                    -- bcrypt hash, nullable
    is_active        BOOLEAN      NOT NULL DEFAULT TRUE,
    click_count      BIGINT       NOT NULL DEFAULT 0, -- denormalized for fast dashboard reads
    expired_at       TIMESTAMPTZ,                     -- NULL means no expiration
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Primary lookup index for redirect path — this is the hottest query in the system
CREATE UNIQUE INDEX IF NOT EXISTS idx_links_slug ON links (slug);

-- Index for user's link listing
CREATE INDEX IF NOT EXISTS idx_links_user_id ON links (user_id);

-- Partial index for active links only — smaller index, faster redirect lookup
CREATE INDEX IF NOT EXISTS idx_links_slug_active ON links (slug)
    WHERE is_active = TRUE;