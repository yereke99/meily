// ── internal/database/schema.go ───────────────────────────────────────────────
package database

import (
	"database/sql"
	"fmt"
)

// CreateTables создаёт все необходимые таблицы, если они не существуют.
func CreateTables(db *sql.DB) error {
	for name, fn := range map[string]func(*sql.DB) error{
		"just":   createJustTable,
		"client": createClientTable,
		"loto":   createLotoTable,
		"geo":    createGeoTable,
	} {
		if err := fn(db); err != nil {
			return fmt.Errorf("create %s table: %w", name, err)
		}
	}
	return nil
}

func CreateTabless(db *sql.DB) error {
	for name, fn := range map[string]func(*sql.DB) error{
		"just":   createJustTable,
		"client": createClientTable,
		"loto":   createLotoTable,
		"geo":    createGeoTable,
	} {
		if err := fn(db); err != nil {
			return fmt.Errorf("create %s table: %w", name, err)
		}
	}

	return nil
}

func createJustTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS just (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		id_user BIGINT,
		userName VARCHAR(255),
		dataRegistred VARCHAR(255)
	);
	`
	_, err := db.Exec(stmt)
	return err
}

func createClientTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS client (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		id_user BIGINT,
		userName VARCHAR(255),
		fio VARCHAR(255),
		contact VARCHAR(255),
		address VARCHAR(255),
		dateRegister VARCHAR(255),
		dataPay VARCHAR(255),
		checks BOOLEAN
	);
	`
	_, err := db.Exec(stmt)
	return err
}

func createLotoTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS loto (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		id_user BIGINT,
		id_loto INTEGER,
		qr VARCHAR(255),
		who_paid VARCHAR(255),
		receipt VARCHAR(255),
		fio VARCHAR(255),
		contact VARCHAR(255),
		address VARCHAR(255),
		dataPay VARCHAR(255)
	);
	`
	_, err := db.Exec(stmt)
	return err
}

func createGeoTable(db *sql.DB) error {
	const stmt = `
	CREATE TABLE IF NOT EXISTS geo (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		id_user BIGINT,
		location VARCHAR(512),
		dataReg VARCHAR(512)
	);
	`
	_, err := db.Exec(stmt)
	return err
}
