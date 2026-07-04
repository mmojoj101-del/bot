-- 001_initial_schema.down.sql
-- Fury SMS Gateway - Drop all tables

DROP TABLE IF EXISTS audit_logs CASCADE;
DROP TABLE IF EXISTS refresh_tokens CASCADE;
DROP TABLE IF EXISTS routes CASCADE;
DROP TABLE IF EXISTS connectors CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS tenant_members CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;
DROP TABLE IF EXISTS users CASCADE;

DROP TYPE IF EXISTS audit_action;
DROP TYPE IF EXISTS route_type;
DROP TYPE IF EXISTS connector_type;
DROP TYPE IF EXISTS user_status;
DROP TYPE IF EXISTS tenant_status;
DROP TYPE IF EXISTS member_role;
