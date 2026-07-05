-- 003_outbox.down.sql
-- Fury SMS Gateway - Revert outbox, DLR fields, metrics, and price fix

DROP TABLE IF EXISTS metrics_counters CASCADE;
DROP TABLE IF EXISTS outbox_events CASCADE;
DROP TYPE IF EXISTS outbox_status;

ALTER TABLE dlr_logs DROP COLUMN IF EXISTS connector_name;
ALTER TABLE dlr_logs DROP COLUMN IF EXISTS remote_ip;
ALTER TABLE dlr_logs DROP COLUMN IF EXISTS headers;
ALTER TABLE dlr_logs DROP COLUMN IF EXISTS raw_payload;

ALTER TABLE messages ALTER COLUMN price TYPE NUMERIC(12,5) USING (price::NUMERIC / 100000);
ALTER TABLE messages ALTER COLUMN cost TYPE NUMERIC(12,5) USING (cost::NUMERIC / 100000);
