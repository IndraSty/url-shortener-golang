-- Click events — the analytics fact table
-- This table grows unboundedly — every redirect appends a row
-- ip_address is masked at application layer for GDPR compliance (stored as x.x.0.0)
-- referrer stores domain only, no path/query — strips tracking params at app layer
-- ab_test_id is nullable — only set when click was served by an A/B variant
CREATE TABLE IF NOT EXISTS click_events (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    link_id       BIGINT      NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    ab_test_id    UUID        REFERENCES ab_tests(id) ON DELETE SET NULL,
    ip_address    VARCHAR(45) NOT NULL DEFAULT '',   -- masked: "192.168.0.0" format
    country_code  CHAR(2)     NOT NULL DEFAULT '',
    city          VARCHAR(100)NOT NULL DEFAULT '',
    device        VARCHAR(20) NOT NULL DEFAULT '',   -- desktop | mobile | tablet | bot
    os            VARCHAR(50) NOT NULL DEFAULT '',
    browser       VARCHAR(50) NOT NULL DEFAULT '',
    referrer      VARCHAR(255)NOT NULL DEFAULT '',   -- domain only
    user_agent    TEXT        NOT NULL DEFAULT '',
    clicked_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Composite index for all analytics queries — link_id + time range
CREATE INDEX IF NOT EXISTS idx_click_events_link_time
    ON click_events (link_id, clicked_at DESC);

-- Index for A/B test performance analytics
CREATE INDEX IF NOT EXISTS idx_click_events_ab_test
    ON click_events (ab_test_id)
    WHERE ab_test_id IS NOT NULL;

-- Index for country breakdown queries
CREATE INDEX IF NOT EXISTS idx_click_events_country
    ON click_events (link_id, country_code);

-- Index for device breakdown queries
CREATE INDEX IF NOT EXISTS idx_click_events_device
    ON click_events (link_id, device);

-- Partial index for recent clicks (last 30 days) — most analytics queries are recent
CREATE INDEX IF NOT EXISTS idx_click_events_recent
    ON click_events (link_id, clicked_at DESC)
    WHERE clicked_at > NOW() - INTERVAL '30 days';