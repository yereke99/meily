// ── internal/database/schema.go ───────────────────────────────────────────────
package database

import (
	"database/sql"
	"fmt"
	"log"
)

// CreateTables создаёт все необходимые таблицы с индексами, если они не существуют.
func CreateTables(db *sql.DB) error {
	tables := []struct {
		name string
		fn   func(*sql.DB) error
	}{
		{"just", createJustTable},
		{"client", createClientTable},
		{"loto", createLotoTable},
		{"geo", createGeoTable},
		{"bot_sessions", createBotSessionsTable},
		{"admin_logs", createAdminLogsTable},
	}

	for _, table := range tables {
		log.Printf("Creating table: %s", table.name)
		if err := table.fn(db); err != nil {
			return fmt.Errorf("create %s table: %w", table.name, err)
		}
	}

	// Создаем индексы после создания всех таблиц
	if err := createIndexes(db); err != nil {
		return fmt.Errorf("create indexes: %w", err)
	}

	// Создаем представления
	if err := createViews(db); err != nil {
		return fmt.Errorf("create views: %w", err)
	}

	log.Println("All tables, indexes and views created successfully")
	return nil
}

func createJustTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS just (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		id_user BIGINT NOT NULL UNIQUE,
		userName VARCHAR(255) NOT NULL,
		dataRegistred VARCHAR(50) NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(stmt)
	return err
}

func createClientTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS client (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		id_user BIGINT NOT NULL UNIQUE,
		userName VARCHAR(255) NOT NULL,
		fio TEXT NULL,
		contact VARCHAR(50) NOT NULL,
		address TEXT NULL,
		dateRegister VARCHAR(50) NULL,
		dataPay VARCHAR(50) NOT NULL,
		checks BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(stmt)
	return err
}

func createLotoTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS loto (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		id_user BIGINT NOT NULL,
		id_loto INT NOT NULL,
		qr TEXT NULL,
		who_paid VARCHAR(255) DEFAULT '',
		receipt TEXT NULL,
		fio TEXT NULL,
		contact VARCHAR(50),
		address TEXT NULL,
		dataPay VARCHAR(50) NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(id_user, id_loto)
	);
	`
	_, err := db.Exec(stmt)
	return err
}

func createGeoTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS geo (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		id_user INTEGER NOT NULL UNIQUE,
		location TEXT NOT NULL,
		dataReg VARCHAR(50) NOT NULL,
		latitude REAL,
		longitude REAL,
		accuracy_meters INTEGER,
		address_components TEXT,
		city VARCHAR(100),
		country VARCHAR(100) DEFAULT 'Kazakhstan',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(stmt); err != nil {
		return err
	}

	// Триггер для обновления updated_at при любом UPDATE
	const trigger = `
	CREATE TRIGGER IF NOT EXISTS geo_updated_at 
	AFTER UPDATE ON geo
	FOR EACH ROW
	BEGIN
	  UPDATE geo SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id;
	END;
	`
	_, err := db.Exec(trigger)
	return err
}

func createBotSessionsTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS bot_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id BIGINT NOT NULL,
		session_id VARCHAR(100) NOT NULL,
		state VARCHAR(50) NOT NULL DEFAULT 'start',
		data TEXT NULL,
		last_activity DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, session_id)
	);
	`
	_, err := db.Exec(stmt)
	return err
}

func createAdminLogsTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS admin_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		admin_user_id BIGINT NOT NULL,
		action VARCHAR(100) NOT NULL,
		target_user_id BIGINT NULL,
		details TEXT NULL,
		ip_address VARCHAR(45) NULL,
		user_agent TEXT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(stmt)
	return err
}

func createIndexes(db *sql.DB) error {
	indexes := []string{
		// Индексы для таблицы just
		"CREATE INDEX IF NOT EXISTS idx_just_user_id ON just(id_user)",
		"CREATE INDEX IF NOT EXISTS idx_just_date ON just(dataRegistred)",

		// Индексы для таблицы client
		"CREATE INDEX IF NOT EXISTS idx_client_user_id ON client(id_user)",
		"CREATE INDEX IF NOT EXISTS idx_client_contact ON client(contact)",
		"CREATE INDEX IF NOT EXISTS idx_client_date_pay ON client(dataPay)",
		"CREATE INDEX IF NOT EXISTS idx_client_checks ON client(checks)",

		// Индексы для таблицы loto
		"CREATE INDEX IF NOT EXISTS idx_loto_user_id ON loto(id_user)",
		"CREATE INDEX IF NOT EXISTS idx_loto_id ON loto(id_loto)",
		"CREATE INDEX IF NOT EXISTS idx_loto_who_paid ON loto(who_paid)",
		"CREATE INDEX IF NOT EXISTS idx_loto_date_pay ON loto(dataPay)",

		// Индексы для таблицы geo
		"CREATE INDEX IF NOT EXISTS idx_geo_user_id ON geo(id_user)",
		"CREATE INDEX IF NOT EXISTS idx_geo_coordinates ON geo(latitude, longitude)",
		"CREATE INDEX IF NOT EXISTS idx_geo_city ON geo(city)",
		"CREATE INDEX IF NOT EXISTS idx_geo_date ON geo(dataReg)",

		// Индексы для таблицы bot_sessions
		"CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON bot_sessions(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_sessions_session_id ON bot_sessions(session_id)",
		"CREATE INDEX IF NOT EXISTS idx_sessions_state ON bot_sessions(state)",
		"CREATE INDEX IF NOT EXISTS idx_sessions_last_activity ON bot_sessions(last_activity)",

		// Индексы для таблицы admin_logs
		"CREATE INDEX IF NOT EXISTS idx_admin_logs_admin_user ON admin_logs(admin_user_id)",
		"CREATE INDEX IF NOT EXISTS idx_admin_logs_action ON admin_logs(action)",
		"CREATE INDEX IF NOT EXISTS idx_admin_logs_target_user ON admin_logs(target_user_id)",
		"CREATE INDEX IF NOT EXISTS idx_admin_logs_created_at ON admin_logs(created_at)",
	}

	for _, indexStmt := range indexes {
		if _, err := db.Exec(indexStmt); err != nil {
			log.Printf("Warning: failed to create index: %s, error: %v", indexStmt, err)
			// Не возвращаем ошибку, так как индекс может уже существовать
		}
	}

	return nil
}

func createViews(db *sql.DB) error {
	// SQLite doesn't support all MySQL view features, so we'll skip views for now
	// You can implement these as methods in your repository instead
	log.Println("Views skipped for SQLite compatibility")
	return nil
}

// CleanupOldSessions удаляет старые сессии (SQLite version)
func CleanupOldSessions(db *sql.DB) error {
	const stmt = `
	DELETE FROM bot_sessions 
	WHERE expires_at IS NOT NULL AND expires_at < datetime('now')
	   OR last_activity < datetime('now', '-24 hours')
	`
	_, err := db.Exec(stmt)
	return err
}

// GetDashboardStats получает статистику для дашборда (SQLite version)
func GetDashboardStats(db *sql.DB) (*DashboardStats, error) {
	stats := &DashboardStats{}

	// Get individual counts since we don't have the view
	var err error

	// Total users
	err = db.QueryRow("SELECT COUNT(*) FROM just").Scan(&stats.TotalUsers)
	if err != nil {
		return nil, err
	}

	// Total clients
	err = db.QueryRow("SELECT COUNT(*) FROM client").Scan(&stats.TotalClients)
	if err != nil {
		return nil, err
	}

	// Total lotto
	err = db.QueryRow("SELECT COUNT(*) FROM loto").Scan(&stats.TotalLotto)
	if err != nil {
		return nil, err
	}

	// Total geo
	err = db.QueryRow("SELECT COUNT(*) FROM geo").Scan(&stats.TotalGeo)
	if err != nil {
		return nil, err
	}

	// Clients with geo
	err = db.QueryRow("SELECT COUNT(*) FROM client c INNER JOIN geo g ON c.id_user = g.id_user").Scan(&stats.ClientsWithGeo)
	if err != nil {
		return nil, err
	}

	// New users today
	err = db.QueryRow("SELECT COUNT(*) FROM just WHERE DATE(created_at) = DATE('now')").Scan(&stats.NewUsersToday)
	if err != nil {
		return nil, err
	}

	// New clients today
	err = db.QueryRow("SELECT COUNT(*) FROM client WHERE DATE(created_at) = DATE('now')").Scan(&stats.NewClientsToday)
	if err != nil {
		return nil, err
	}

	// New lotto today
	err = db.QueryRow("SELECT COUNT(*) FROM loto WHERE DATE(created_at) = DATE('now')").Scan(&stats.NewLottoToday)
	if err != nil {
		return nil, err
	}

	// New geo today
	err = db.QueryRow("SELECT COUNT(*) FROM geo WHERE DATE(created_at) = DATE('now')").Scan(&stats.NewGeoToday)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// DashboardStats структура для статистики дашборда
type DashboardStats struct {
	TotalUsers      int `json:"total_users"`
	TotalClients    int `json:"total_clients"`
	TotalLotto      int `json:"total_lotto"`
	TotalGeo        int `json:"total_geo"`
	ClientsWithGeo  int `json:"clients_with_geo"`
	NewUsersToday   int `json:"new_users_today"`
	NewClientsToday int `json:"new_clients_today"`
	NewLottoToday   int `json:"new_lotto_today"`
	NewGeoToday     int `json:"new_geo_today"`
}
