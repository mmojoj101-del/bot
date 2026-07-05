-- 002_messages.up.sql
-- Fury SMS Gateway - Messages and DLR tracking

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- Message Status ENUM
-- ============================================================
CREATE TYPE message_status AS ENUM (
    'accepted', 'queued', 'sending', 'sent', 'delivered', 'failed', 'retrying'
);

CREATE TYPE message_direction AS ENUM ('outbound', 'inbound');
CREATE TYPE message_encoding AS ENUM ('gsm7', 'ucs2', 'latin1', 'ascii');
CREATE TYPE message_priority AS ENUM ('low', 'normal', 'high', 'urgent');
CREATE TYPE dlr_status AS ENUM ('pending', 'delivered', 'failed', 'expired', 'rejected', 'unknown');

-- ============================================================
-- MESSAGES
-- ============================================================
CREATE TABLE messages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    connector_id     UUID REFERENCES connectors(id) ON DELETE SET NULL,
    route_id         UUID REFERENCES routes(id) ON DELETE SET NULL,
    client_id        VARCHAR(255) NOT NULL,
    direction        message_direction NOT NULL DEFAULT 'outbound',
    status           message_status NOT NULL DEFAULT 'accepted',
    previous_status  message_status,
    source           VARCHAR(50) NOT NULL,
    destination      VARCHAR(50) NOT NULL,
    text             TEXT NOT NULL,
    encoding         message_encoding NOT NULL DEFAULT 'gsm7',
    priority         message_priority NOT NULL DEFAULT 'normal',
    parts            INTEGER NOT NULL DEFAULT 1,
    dlr_status       dlr_status,
    dlr_url          TEXT,
    dlr_id           VARCHAR(255),
    external_id      VARCHAR(255),
    client_ref       VARCHAR(255),
    retry_count      INTEGER NOT NULL DEFAULT 0,
    max_retries      INTEGER NOT NULL DEFAULT 3,
    price            NUMERIC(12,4) NOT NULL DEFAULT 0,
    cost             NUMERIC(12,4) NOT NULL DEFAULT 0,
    sent_at          TIMESTAMPTZ,
    delivered_at     TIMESTAMPTZ,
    failed_at        TIMESTAMPTZ,
    error_code       VARCHAR(50),
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

-- Indexes for efficient querying
CREATE INDEX idx_messages_tenant_created ON messages(tenant_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_messages_status ON messages(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_messages_connector ON messages(connector_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_messages_direction ON messages(tenant_id, direction) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_messages_client_ref ON messages(tenant_id, client_ref) WHERE client_ref IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_messages_external_id ON messages(external_id) WHERE external_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_messages_source_dest ON messages(tenant_id, source, destination) WHERE deleted_at IS NULL;

-- ============================================================
-- DLR LOGS
-- ============================================================
CREATE TABLE dlr_logs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id     UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    status         dlr_status NOT NULL,
    external_id    VARCHAR(255),
    error_code     VARCHAR(50),
    description    TEXT,
    raw_response   JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

CREATE INDEX idx_dlr_logs_message ON dlr_logs(message_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_dlr_logs_tenant ON dlr_logs(tenant_id, created_at DESC) WHERE deleted_at IS NULL;
