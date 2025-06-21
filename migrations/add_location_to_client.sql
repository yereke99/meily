-- ── migrations/add_location_to_client.sql ────────────────────────────────────

-- Add latitude and longitude columns to client table
ALTER TABLE client ADD COLUMN latitude REAL;
ALTER TABLE client ADD COLUMN longitude REAL;

-- Create index on coordinates for faster location-based queries
CREATE INDEX idx_client_location ON client(latitude, longitude);

-- Update the client table structure to ensure it matches expected schema
-- (This is the complete table structure for reference)

-- If you need to recreate the table from scratch, use this:
/*
DROP TABLE IF EXISTS client;

CREATE TABLE IF NOT EXISTS client (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    id_user BIGINT NOT NULL,
    userName VARCHAR(255),
    fio VARCHAR(255),
    contact VARCHAR(255),
    address VARCHAR(255),
    dateRegister VARCHAR(255),
    dataPay VARCHAR(255),
    checks BOOLEAN DEFAULT FALSE,
    latitude REAL,
    longitude REAL
);

-- Create indexes for better performance
CREATE INDEX idx_client_user_id ON client(id_user);
CREATE INDEX idx_client_checks ON client(checks);
CREATE INDEX idx_client_location ON client(latitude, longitude);
CREATE INDEX idx_client_date_pay ON client(dataPay);
*/

-- Create the other required tables if they don't exist

CREATE TABLE IF NOT EXISTS just (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    id_user BIGINT NOT NULL,
    userName VARCHAR(255),
    dataRegistred VARCHAR(255)
);

CREATE INDEX IF NOT EXISTS idx_just_user_id ON just(id_user);

CREATE TABLE IF NOT EXISTS loto (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    id_user BIGINT NOT NULL,
    id_loto INTEGER,
    qr VARCHAR(255),
    who_paid VARCHAR(255),
    receipt VARCHAR(255),
    fio VARCHAR(255),
    contact VARCHAR(255),
    address VARCHAR(255),
    dataPay VARCHAR(255)
);

CREATE INDEX IF NOT EXISTS idx_loto_user_id ON loto(id_user);
CREATE INDEX IF NOT EXISTS idx_loto_id_loto ON loto(id_loto);

CREATE TABLE IF NOT EXISTS geo (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    id_user BIGINT NOT NULL,
    location VARCHAR(255),
    dataReg VARCHAR(255)
);

CREATE INDEX IF NOT EXISTS idx_geo_user_id ON geo(id_user);