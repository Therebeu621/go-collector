-- 001_init.sql
-- Creates the products table for the collector.

CREATE TABLE IF NOT EXISTS products (
    id         INTEGER PRIMARY KEY,
    title      TEXT        NOT NULL,
    brand      TEXT,
    category   TEXT        NOT NULL DEFAULT 'unknown',
    price      NUMERIC     NOT NULL CHECK (price > 0),
    rating     NUMERIC,
    stock      INTEGER,
    updated_at TIMESTAMP   NOT NULL DEFAULT now()
);
