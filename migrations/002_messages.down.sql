-- 002_messages.down.sql
-- Fury SMS Gateway - Drop messages and DLR tracking

DROP TABLE IF EXISTS dlr_logs CASCADE;
DROP TABLE IF EXISTS messages CASCADE;

DROP TYPE IF EXISTS dlr_status;
DROP TYPE IF EXISTS message_priority;
DROP TYPE IF EXISTS message_encoding;
DROP TYPE IF EXISTS message_direction;
DROP TYPE IF EXISTS message_status;
