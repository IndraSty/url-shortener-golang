-- A/B test variants for a link
-- Each link can have multiple destination URLs with weighted distribution
-- weights across all variants for a link MUST sum to 100 (enforced in application layer)
-- When no A/B tests exist, the link's destination_url is used directly
CREATE TABLE IF NOT EXISTS ab_tests (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    link_id         BIGINT       NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    destination_url TEXT         NOT NULL,
    weight          INT          NOT NULL CHECK (weight > 0 AND weight <= 100),
    label           VARCHAR(100) NOT NULL DEFAULT '',  -- human-readable variant name
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Index for fetching all variants of a link (called on every redirect with A/B enabled)
CREATE INDEX IF NOT EXISTS idx_ab_tests_link_id ON ab_tests (link_id);