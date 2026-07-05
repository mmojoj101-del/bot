-- 003_outbox.up.sql
-- Fury SMS Gateway - Price type fix, Outbox pattern, DLR fields, Metrics

-- ============================================================
-- FIX: Convert price/cost from NUMERIC to BIGINT (thousandths of a cent)
-- ============================================================
ALTER TABLE messages ALTER COLUMN price TYPE BIGINT USING (ROUND(price * 100000)::BIGINT);
ALTER TABLE messages ALTER COLUMN cost TYPE BIGINT USING (ROUND(cost * 100000)::BIGINT);

-- ============================================================
-- OUTBOX EVENTS
-- ============================================================
CREATE TYPE outbox_status AS ENUM ('pending', 'published', 'failed');

CREATE TABLE outbox_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type  VARCHAR(100) NOT NULL,
    tenant_id   UUID REFERENCES tenants(id) ON DELETE SET NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    status      outbox_status NOT NULL DEFAULT 'pending',
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    published_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_outbox_status ON outbox_events(status, created_at ASC) WHERE status = 'pending' AND deleted_at IS NULL;

-- ============================================================
-- Updated DLR LOGS (add new columns)
-- ============================================================
ALTER TABLE dlr_logs ADD COLUMN IF NOT EXISTS connector_name VARCHAR(255);
ALTER TABLE dlr_logs ADD COLUMN IF NOT EXISTS remote_ip VARCHAR(50);
ALTER TABLE dlr_logs ADD COLUMN IF NOT EXISTS headers JSONB;
ALTER TABLE dlr_logs ADD COLUMN IF NOT EXISTS raw_payload JSONB;

-- ============================================================
-- METRICS COUNTERS
-- ============================================================
CREATE TABLE metrics_counters (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID REFERENCES tenants(id) ON DELETE CASCADE,
    connector_id UUID REFERENCES connectors(id) ON DELETE SET NULL,
    metric_name VARCHAR(100) NOT NULL,
    value       BIGINT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_metrics_tenant ON metrics_counters(tenant_id, metric_name, recorded_at DESC);
