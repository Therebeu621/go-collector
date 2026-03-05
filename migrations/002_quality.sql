-- 002_quality.sql
-- Adds checksum-based idempotency and quality gate columns to the products table.

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS checksum        TEXT,
    ADD COLUMN IF NOT EXISTS quality_status   TEXT    NOT NULL DEFAULT 'ok',
    ADD COLUMN IF NOT EXISTS quality_reasons  TEXT[]  NOT NULL DEFAULT '{}';
