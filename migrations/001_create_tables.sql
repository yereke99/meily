-- Enhanced database schema for Meily Cosmetics Bot
-- File: migrations/001_create_tables.sql

-- Users table (just)
CREATE TABLE IF NOT EXISTS just (
    id INT AUTO_INCREMENT PRIMARY KEY,
    id_user BIGINT NOT NULL UNIQUE,
    userName VARCHAR(255) NOT NULL,
    dataRegistred VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (id_user),
    INDEX idx_date (dataRegistred)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Clients table
CREATE TABLE IF NOT EXISTS client (
    id INT AUTO_INCREMENT PRIMARY KEY,
    id_user BIGINT NOT NULL UNIQUE,
    userName VARCHAR(255) NOT NULL,
    fio TEXT NULL,
    contact VARCHAR(50) NOT NULL,
    address TEXT NULL,
    dateRegister VARCHAR(50) NULL,
    dataPay VARCHAR(50) NOT NULL,
    checks BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (id_user),
    INDEX idx_contact (contact),
    INDEX idx_date_pay (dataPay),
    INDEX idx_checks (checks)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Lottery table (loto)
CREATE TABLE IF NOT EXISTS loto (
    id INT AUTO_INCREMENT PRIMARY KEY,
    id_user BIGINT NOT NULL,
    id_loto INT NOT NULL,
    qr TEXT NULL,
    who_paid VARCHAR(255) DEFAULT '',
    receipt TEXT NULL,
    fio TEXT NULL,
    contact VARCHAR(50) NOT NULL,
    address TEXT NULL,
    dataPay VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (id_user),
    INDEX idx_loto_id (id_loto),
    INDEX idx_who_paid (who_paid),
    INDEX idx_date_pay (dataPay),
    UNIQUE KEY unique_user_loto (id_user, id_loto)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Enhanced geolocation table
CREATE TABLE IF NOT EXISTS geo (
    id INT AUTO_INCREMENT PRIMARY KEY,
    id_user BIGINT NOT NULL,
    location TEXT NOT NULL,
    dataReg VARCHAR(50) NOT NULL,
    latitude DECIMAL(10, 8) NULL,
    longitude DECIMAL(11, 8) NULL,
    accuracy_meters INT NULL,
    address_components JSON NULL,
    city VARCHAR(100) NULL,
    country VARCHAR(100) DEFAULT 'Kazakhstan',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (id_user),
    INDEX idx_coordinates (latitude, longitude),
    INDEX idx_city (city),
    INDEX idx_date (dataReg),
    UNIQUE KEY unique_user_geo (id_user)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Sessions table for bot state management
CREATE TABLE IF NOT EXISTS bot_sessions (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    session_id VARCHAR(100) NOT NULL,
    state VARCHAR(50) NOT NULL DEFAULT 'start',
    data JSON NULL,
    last_activity TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_session_id (session_id),
    INDEX idx_state (state),
    INDEX idx_last_activity (last_activity),
    UNIQUE KEY unique_user_session (user_id, session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Admin logs table
CREATE TABLE IF NOT EXISTS admin_logs (
    id INT AUTO_INCREMENT PRIMARY KEY,
    admin_user_id BIGINT NOT NULL,
    action VARCHAR(100) NOT NULL,
    target_user_id BIGINT NULL,
    details JSON NULL,
    ip_address VARCHAR(45) NULL,
    user_agent TEXT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_admin_user (admin_user_id),
    INDEX idx_action (action),
    INDEX idx_target_user (target_user_id),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- File: migrations/002_add_foreign_keys.sql

-- Add foreign key constraints
ALTER TABLE client 
ADD CONSTRAINT fk_client_user 
FOREIGN KEY (id_user) REFERENCES just(id_user) 
ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE loto 
ADD CONSTRAINT fk_loto_user 
FOREIGN KEY (id_user) REFERENCES just(id_user) 
ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE geo 
ADD CONSTRAINT fk_geo_user 
FOREIGN KEY (id_user) REFERENCES just(id_user) 
ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE bot_sessions 
ADD CONSTRAINT fk_session_user 
FOREIGN KEY (user_id) REFERENCES just(id_user) 
ON DELETE CASCADE ON UPDATE CASCADE;

-- File: migrations/003_create_views.sql

-- View for clients with geolocation data
CREATE OR REPLACE VIEW clients_with_geo AS
SELECT 
    c.id,
    c.id_user,
    c.userName,
    c.fio,
    c.contact,
    c.address,
    c.dateRegister,
    c.dataPay,
    c.checks,
    c.created_at as client_created_at,
    c.updated_at as client_updated_at,
    g.location,
    g.latitude,
    g.longitude,
    g.accuracy_meters,
    g.city,
    g.dataReg as geo_date,
    g.created_at as geo_created_at,
    CASE WHEN g.id_user IS NOT NULL THEN TRUE ELSE FALSE END as has_geo,
    CASE WHEN g.latitude IS NOT NULL AND g.longitude IS NOT NULL THEN TRUE ELSE FALSE END as has_coordinates
FROM client c
LEFT JOIN geo g ON c.id_user = g.id_user;

-- View for dashboard statistics
CREATE OR REPLACE VIEW dashboard_stats AS
SELECT 
    (SELECT COUNT(*) FROM just) as total_users,
    (SELECT COUNT(*) FROM client) as total_clients,
    (SELECT COUNT(*) FROM loto) as total_lotto,
    (SELECT COUNT(*) FROM geo) as total_geo,
    (SELECT COUNT(*) FROM client c INNER JOIN geo g ON c.id_user = g.id_user) as clients_with_geo,
    (SELECT COUNT(*) FROM just WHERE DATE(created_at) = CURDATE()) as new_users_today,
    (SELECT COUNT(*) FROM client WHERE DATE(created_at) = CURDATE()) as new_clients_today,
    (SELECT COUNT(*) FROM loto WHERE DATE(created_at) = CURDATE()) as new_lotto_today,
    (SELECT COUNT(*) FROM geo WHERE DATE(created_at) = CURDATE()) as new_geo_today;

-- File: migrations/004_insert_sample_data.sql

-- Sample test data for development (only run in development)
INSERT IGNORE INTO just (id_user, userName, dataRegistred) VALUES
(123456789, 'Test User 1', '2024-01-15 10:00:00'),
(234567890, 'Test User 2', '2024-01-16 11:00:00'),
(345678901, 'Test User 3', '2024-01-17 12:00:00'),
(456789012, 'Test User 4', '2024-01-18 13:00:00'),
(567890123, 'Test User 5', '2024-01-19 14:00:00');

INSERT IGNORE INTO client (id_user, userName, fio, contact, address, dateRegister, dataPay, checks) VALUES
(123456789, 'Test User 1', 'Айжан Сәбитова', '+77001234567', 'Алматы қ., Абай даңғылы 123', '2024-01-15 10:00:00', '2024-01-15 10:30:00', TRUE),
(234567890, 'Test User 2', 'Бекзат Нұрланов', '+77007654321', 'Нұр-Сұлтан қ., Мәңгілік Ел даңғылы 55', '2024-01-16 11:00:00', '2024-01-16 11:30:00', FALSE),
(345678901, 'Test User 3', 'Гүлнара Қасымова', '+77009876543', 'Шымкент қ., Тәуке хан даңғылы 78', '2024-01-17 12:00:00', '2024-01-17 12:30:00', TRUE),
(456789012, 'Test User 4', 'Дәурен Бейсенов', '+77005432109', 'Қарағанды қ., Бұқар жырау даңғылы 91', '2024-01-18 13:00:00', '2024-01-18 13:30:00', FALSE),
(567890123, 'Test User 5', 'Еркежан Төлеген', '+77008765432', 'Алматы қ., Сәтпаев көшесі 15', '2024-01-19 14:00:00', '2024-01-19 14:30:00', TRUE);

INSERT IGNORE INTO geo (id_user, location, dataReg, latitude, longitude, city) VALUES
(123456789, 'latitude: 43.238949, longitude: 76.889709', '2024-01-15 10:00:00', 43.238949, 76.889709, 'Алматы'),
(234567890, 'latitude: 51.160523, longitude: 71.470356', '2024-01-16 11:00:00', 51.160523, 71.470356, 'Нұр-Сұлтан'),
(345678901, 'latitude: 42.317738, longitude: 69.586945', '2024-01-17 12:00:00', 42.317738, 69.586945, 'Шымкент'),
(456789012, 'latitude: 49.806406, longitude: 73.087328', '2024-01-18 13:00:00', 49.806406, 73.087328, 'Қарағанды'),
(567890123, 'latitude: 43.250000, longitude: 76.900000', '2024-01-19 14:00:00', 43.250000, 76.900000, 'Алматы');

-- File: migrations/005_create_indexes.sql

-- Additional performance indexes
CREATE INDEX idx_client_geo_lookup ON client(id_user, dataPay DESC);
CREATE INDEX idx_geo_coordinates_lookup ON geo(latitude, longitude, city);
CREATE INDEX idx_loto_payment_status ON loto(who_paid, dataPay DESC);
CREATE INDEX idx_just_registration_date ON just(dataRegistred DESC);

-- Composite indexes for common queries
CREATE INDEX idx_client_status_date ON client(checks, dataPay DESC);
CREATE INDEX idx_geo_user_location ON geo(id_user, latitude, longitude);

-- Full-text search indexes
ALTER TABLE client ADD FULLTEXT(fio, address);
ALTER TABLE geo ADD FULLTEXT(location);

-- File: migrations/006_create_procedures.sql

DELIMITER //

-- Procedure to get dashboard statistics
CREATE PROCEDURE GetDashboardStats()
BEGIN
    SELECT * FROM dashboard_stats;
END //

-- Procedure to get clients with geolocation in a radius
CREATE PROCEDURE GetClientsInRadius(
    IN center_lat DECIMAL(10, 8),
    IN center_lng DECIMAL(11, 8),
    IN radius_km INT
)
BEGIN
    SELECT c.*, g.latitude, g.longitude,
           (6371 * acos(cos(radians(center_lat)) 
           * cos(radians(g.latitude)) 
           * cos(radians(g.longitude) - radians(center_lng)) 
           + sin(radians(center_lat)) 
           * sin(radians(g.latitude)))) AS distance_km
    FROM clients_with_geo c
    WHERE c.has_coordinates = TRUE
    HAVING distance_km <= radius_km
    ORDER BY distance_km;
END //

-- Procedure to cleanup old sessions
CREATE PROCEDURE CleanupOldSessions()
BEGIN
    DELETE FROM bot_sessions 
    WHERE expires_at IS NOT NULL AND expires_at < NOW()
       OR last_activity < DATE_SUB(NOW(), INTERVAL 24 HOUR);
END //

DELIMITER ;