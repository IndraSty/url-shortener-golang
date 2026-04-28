-- Geo-targeting rules — redirect to different URL based on visitor's country
-- country_code is ISO 3166-1 alpha-2 (e.g. 'ID', 'US', 'SG')
-- priority allows ordering when multiple rules could match (lower = higher priority)
-- If no geo rule matches, falls back to A/B test or destination_url
CREATE TABLE IF NOT EXISTS geo_rules (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    link_id         BIGINT      NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    country_code    CHAR(2)     NOT NULL,             -- ISO 3166-1 alpha-2
    destination_url TEXT        NOT NULL,
    priority        INT         NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One rule per country per link
    UNIQUE (link_id, country_code)
);

-- Index for geo rule lookup during redirect
CREATE INDEX IF NOT EXISTS idx_geo_rules_link_id ON geo_rules (link_id);