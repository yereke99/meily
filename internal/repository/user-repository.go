// ── internal/repository/user-repository.go ───────────────────────────────────
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"meily/internal/domain"
	"strconv"
	"strings"
	"time"
)

// Enhanced ClientEntryWithGeo for admin dashboard with geolocation data
type ClientEntryWithGeo struct {
	domain.ClientEntry
	HasGeo    bool     `json:"hasGeo"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

// LottoStats represents statistics for lotto entries
type LottoStats struct {
	Paid   int `json:"paid"`
	Unpaid int `json:"unpaid"`
}

// GeoStats represents geographical distribution statistics
type GeoStats struct {
	Almaty    int `json:"almaty"`
	Nursultan int `json:"nursultan"`
	Shymkent  int `json:"shymkent"`
	Karaganda int `json:"karaganda"`
	Others    int `json:"others"`
}

// AdminClientEntry represents enhanced client data for admin dashboard with geolocation
type AdminClientEntry struct {
	UserID       int64    `json:"userID"`
	UserName     string   `json:"userName"`
	Fio          string   `json:"fio"`
	Contact      string   `json:"contact"`
	Address      string   `json:"address"`
	DateRegister string   `json:"dateRegister"`
	DatePay      string   `json:"dataPay"`
	Checks       bool     `json:"checks"`
	HasGeo       bool     `json:"hasGeo"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
}

// UserRepository работает со всеми таблицами: just, client, loto, geo.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository создаёт новый UserRepository.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// ═══════════════════════════════════════════════════════════════════════════════
//                                 EXISTING METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// InsertJust вставляет запись в таблицу just.
func (r *UserRepository) InsertJust(ctx context.Context, e domain.JustEntry) error {
	const q = `
		INSERT INTO just (id_user, userName, dataRegistred)
		VALUES (?, ?, ?);
	`
	_, err := r.db.ExecContext(ctx, q, e.UserID, e.UserName, e.DateRegistered)
	return err
}

// ExistsJust проверяет, есть ли запись в just по id_user.
func (r *UserRepository) ExistsJust(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM just WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// InsertClient вставляет запись в таблицу client.
func (r *UserRepository) InsertClient(ctx context.Context, e domain.ClientEntry) error {
	const q = `
		INSERT INTO client (id_user, userName, fio, contact, address, dateRegister, dataPay, checks)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := r.db.ExecContext(ctx, q,
		e.UserID, e.UserName, e.Fio, e.Contact,
		e.Address, e.DateRegister, e.DatePay, e.Checks,
	)
	return err
}

// ExistsClient проверяет, есть ли запись в client по id_user.
func (r *UserRepository) ExistsClient(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM client WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// IsClientUnique возвращает true, если в client нет записи с данным id_user.
func (r *UserRepository) IsClientUnique(ctx context.Context, userID int64) (bool, error) {
	const q = `
		SELECT COUNT(1)
		FROM client
		WHERE id_user = ?;
	`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	// уникальный — значит, записей нет
	return cnt == 0, nil
}

// IsClientPaid проверяет, оплачен ли клиент (существует ли запись и checks = true)
func (r *UserRepository) IsClientPaid(ctx context.Context, userID int64) (bool, error) {
	const q = `
		SELECT checks
		FROM client
		WHERE id_user = ?;
	`
	var checks bool
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&checks)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // No client record found
		}
		return false, err
	}
	return checks, nil
}

// GetClientByUserID получает данные клиента по user ID
func (r *UserRepository) GetClientByUserID(ctx context.Context, userID int64) (*domain.ClientEntry, error) {
	const q = `
		SELECT id_user, userName, fio, contact, address, dateRegister, dataPay, checks
		FROM client
		WHERE id_user = ?;
	`
	var client domain.ClientEntry
	err := r.db.QueryRowContext(ctx, q, userID).Scan(
		&client.UserID, &client.UserName,
		&client.Fio, &client.Contact, &client.Address,
		&client.DateRegister, &client.DatePay, &client.Checks,
	)
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// UpdateClientDeliveryData обновляет данные доставки клиента
func (r *UserRepository) UpdateClientDeliveryData(ctx context.Context, userID int64, fio, address string, latitude, longitude float64) error {
	const q = `
		UPDATE client 
		SET fio = ?, address = ?, checks = true
		WHERE id_user = ?;
	`
	_, err := r.db.ExecContext(ctx, q, fio, address, userID)
	if err != nil {
		return err
	}

	// Also insert/update geo data
	const geoQ = `
		INSERT OR REPLACE INTO geo (id_user, location, dataReg)
		VALUES (?, ?, ?);
	`
	location := fmt.Sprintf("%.6f,%.6f", latitude, longitude)
	_, err = r.db.ExecContext(ctx, geoQ, userID, location, time.Now().Format("2006-01-02 15:04:05"))
	return err
}

// GetAllClientsWithDeliveryData получает всех клиентов с данными доставки
func (r *UserRepository) GetAllClientsWithDeliveryData(ctx context.Context) ([]domain.ClientEntry, error) {
	const q = `
		SELECT id_user, userName, fio, contact, address, dateRegister, dataPay, checks
		FROM client
		WHERE checks = true
		ORDER BY dataPay DESC;
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []domain.ClientEntry
	for rows.Next() {
		var client domain.ClientEntry
		err := rows.Scan(
			&client.UserID, &client.UserName,
			&client.Fio, &client.Contact, &client.Address,
			&client.DateRegister, &client.DatePay, &client.Checks,
		)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return clients, rows.Err()
}

// InsertLoto вставляет запись в таблицу loto.
func (r *UserRepository) InsertLoto(ctx context.Context, e domain.LotoEntry) error {
	const q = `
		INSERT INTO loto (id_user, id_loto, qr, who_paid, receipt, fio, contact, address, dataPay)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := r.db.ExecContext(ctx, q,
		e.UserID, e.LotoID, e.QR, e.WhoPaid,
		e.Receipt, e.Fio, e.Contact, e.Address, e.DatePay,
	)
	return err
}

// ExistsLoto проверяет, есть ли запись в loto по id_user.
func (r *UserRepository) ExistsLoto(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM loto WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// IsLotoPaid возвращает true, если для данного id_user и id_loto есть непустое who_paid.
func (r *UserRepository) IsLotoPaid(ctx context.Context, userID int64, lotoID int) (bool, error) {
	const q = `
		SELECT COUNT(1) > 0
		FROM loto
		WHERE id_user = ? AND id_loto = ? AND who_paid != '';
	`
	var paid bool
	err := r.db.QueryRowContext(ctx, q, userID, lotoID).Scan(&paid)
	return paid, err
}

// InsertGeo вставляет запись в таблицу geo.
func (r *UserRepository) InsertGeo(ctx context.Context, e domain.GeoEntry) error {
	const q = `
		INSERT INTO geo (id_user, location, dataReg)
		VALUES (?, ?, ?);
	`
	_, err := r.db.ExecContext(ctx, q, e.UserID, e.Location, e.DataReg)
	return err
}

// ExistsGeo проверяет, есть ли запись в geo по id_user.
func (r *UserRepository) ExistsGeo(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM geo WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            ADMIN DASHBOARD METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// GetTotalUsers возвращает общее количество пользователей
func (r *UserRepository) GetTotalUsers(ctx context.Context) int {
	const q = `SELECT COUNT(*) FROM just;`
	var count int
	if err := r.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0
	}
	return count
}

// GetTotalClients возвращает общее количество клиентов
func (r *UserRepository) GetTotalClients(ctx context.Context) int {
	const q = `SELECT COUNT(*) FROM client;`
	var count int
	if err := r.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0
	}
	return count
}

// GetTotalLotto возвращает общее количество участников лотереи
func (r *UserRepository) GetTotalLotto(ctx context.Context) int {
	const q = `SELECT COUNT(*) FROM loto;`
	var count int
	if err := r.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0
	}
	return count
}

// GetTotalGeo возвращает общее количество записей геолокации
func (r *UserRepository) GetTotalGeo(ctx context.Context) int {
	const q = `SELECT COUNT(*) FROM geo;`
	var count int
	if err := r.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0
	}
	return count
}

// GetRecentJustEntries возвращает последние записи из таблицы just
func (r *UserRepository) GetRecentJustEntries(ctx context.Context, limit int) ([]domain.JustEntry, error) {
	const q = `
		SELECT id_user, userName, dataRegistred
		FROM just
		ORDER BY dataRegistred DESC
		LIMIT ?;
	`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []domain.JustEntry
	for rows.Next() {
		var entry domain.JustEntry
		err := rows.Scan(&entry.UserID, &entry.UserName, &entry.DateRegistered)
		if err != nil {
			continue // Skip invalid rows
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetRecentLotoEntries возвращает последние записи из таблицы loto
func (r *UserRepository) GetRecentLotoEntries(ctx context.Context, limit int) ([]domain.LotoEntry, error) {
	const q = `
		SELECT id_user, id_loto, qr, who_paid, receipt, fio, contact, address, dataPay
		FROM loto
		ORDER BY dataPay DESC
		LIMIT ?;
	`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []domain.LotoEntry
	for rows.Next() {
		var entry domain.LotoEntry
		if err := rows.Scan(&entry.UserID, &entry.LotoID, &entry.QR, &entry.WhoPaid,
			&entry.Receipt, &entry.Fio, &entry.Contact, &entry.Address, &entry.DatePay); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetRecentGeoEntries возвращает последние записи из таблицы geo
func (r *UserRepository) GetRecentGeoEntries(ctx context.Context, limit int) ([]domain.GeoEntry, error) {
	const q = `
		SELECT id_user, location, dataReg
		FROM geo
		ORDER BY dataReg DESC
		LIMIT ?;
	`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []domain.GeoEntry
	for rows.Next() {
		var entry domain.GeoEntry
		if err := rows.Scan(&entry.UserID, &entry.Location, &entry.DataReg); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetClientsWithGeo возвращает клиентов с геолокацией для админ панели
func (r *UserRepository) GetClientsWithGeo(ctx context.Context) ([]AdminClientEntry, error) {
	const q = `
		SELECT 
			c.id_user, 
			c.userName, 
			COALESCE(c.fio, '') as fio,
			COALESCE(c.contact, '') as contact, 
			COALESCE(c.address, '') as address,
			COALESCE(c.dateRegister, '') as dateRegister,
			COALESCE(c.dataPay, '') as dataPay,
			COALESCE(c.checks, 0) as checks,
			g.location
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		ORDER BY c.dataPay DESC;
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []AdminClientEntry
	for rows.Next() {
		var client AdminClientEntry
		var geoLocation sql.NullString

		if err := rows.Scan(
			&client.UserID,
			&client.UserName,
			&client.Fio,
			&client.Contact,
			&client.Address,
			&client.DateRegister,
			&client.DatePay,
			&client.Checks,
			&geoLocation,
		); err != nil {
			continue
		}

		// Parse geolocation if available
		client.HasGeo = false
		if geoLocation.Valid && geoLocation.String != "" {
			lat, lon := parseCoordinates(geoLocation.String)
			if lat != nil && lon != nil {
				client.HasGeo = true
				client.Latitude = lat
				client.Longitude = lon
			}
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// parseCoordinates парсит координаты из строки местоположения
func parseCoordinates(location string) (*float64, *float64) {
	// Try different coordinate formats
	// Format 1: "lat,lon"
	if strings.Contains(location, ",") {
		parts := strings.Split(location, ",")
		if len(parts) >= 2 {
			lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 == nil && err2 == nil {
				// Validate coordinate ranges
				if lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 {
					return &lat, &lon
				}
			}
		}
	}

	// Format 2: "lat:43.2,lon:76.8"
	if strings.Contains(location, "lat:") && strings.Contains(location, "lon:") {
		parts := strings.Split(location, ",")
		if len(parts) >= 2 {
			latStr := strings.TrimPrefix(strings.TrimSpace(parts[0]), "lat:")
			lonStr := strings.TrimPrefix(strings.TrimSpace(parts[1]), "lon:")

			lat, err1 := strconv.ParseFloat(latStr, 64)
			lon, err2 := strconv.ParseFloat(lonStr, 64)
			if err1 == nil && err2 == nil {
				if lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 {
					return &lat, &lon
				}
			}
		}
	}

	// Format 3: JSON-like format {"lat": 43.2, "lon": 76.8}
	if strings.Contains(location, "{") && strings.Contains(location, "}") {
		var coords map[string]float64
		if err := json.Unmarshal([]byte(location), &coords); err == nil {
			if lat, ok1 := coords["lat"]; ok1 {
				if lon, ok2 := coords["lon"]; ok2 {
					if lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 {
						return &lat, &lon
					}
				}
			}
			// Also try latitude/longitude keys
			if lat, ok1 := coords["latitude"]; ok1 {
				if lon, ok2 := coords["longitude"]; ok2 {
					if lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 {
						return &lat, &lon
					}
				}
			}
		}
	}

	return nil, nil
}

// GetLottoStats возвращает статистику лотереи с правильной обработкой NULL значений
func (r *UserRepository) GetLottoStats(ctx context.Context) *LottoStats {
	const q = `
		SELECT 
			COUNT(CASE WHEN who_paid IS NOT NULL AND who_paid != '' THEN 1 END) as paid,
			COUNT(CASE WHEN who_paid IS NULL OR who_paid = '' THEN 1 END) as unpaid
		FROM loto;
	`

	var stats LottoStats
	if err := r.db.QueryRowContext(ctx, q).Scan(&stats.Paid, &stats.Unpaid); err != nil {
		return &LottoStats{Paid: 0, Unpaid: 0}
	}

	return &stats
}

// GetGeoStats возвращает географическую статистику
func (r *UserRepository) GetGeoStats(ctx context.Context) *GeoStats {
	const q = `
		SELECT location, COUNT(*) as count
		FROM geo
		WHERE location IS NOT NULL AND location != ''
		GROUP BY location;
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return &GeoStats{Almaty: 0, Nursultan: 0, Shymkent: 0, Karaganda: 0, Others: 0}
	}
	defer rows.Close()

	var stats GeoStats
	for rows.Next() {
		var location string
		var count int
		if err := rows.Scan(&location, &count); err != nil {
			continue
		}

		// Parse coordinates and categorize by city
		lat, lon := parseCoordinates(location)
		if lat != nil && lon != nil {
			// Categorize by approximate coordinates for Kazakhstan cities
			if *lat >= 43.0 && *lat <= 43.5 && *lon >= 76.5 && *lon <= 77.2 {
				stats.Almaty += count // Almaty region
			} else if *lat >= 51.0 && *lat <= 51.5 && *lon >= 71.0 && *lon <= 71.8 {
				stats.Nursultan += count // Nur-Sultan/Astana region
			} else if *lat >= 42.0 && *lat <= 42.5 && *lon >= 69.0 && *lon <= 70.0 {
				stats.Shymkent += count // Shymkent region
			} else if *lat >= 49.5 && *lat <= 50.0 && *lon >= 72.5 && *lon <= 73.5 {
				stats.Karaganda += count // Karaganda region
			} else {
				stats.Others += count
			}
		} else {
			stats.Others += count
		}
	}

	return &stats
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            ENHANCED GEO ANALYTICS METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// GetClientsByLocationRadius возвращает клиентов в радиусе от заданной точки
func (r *UserRepository) GetClientsByLocationRadius(ctx context.Context, centerLat, centerLon float64, radiusKm int) ([]AdminClientEntry, error) {
	const q = `
		SELECT 
			c.id_user, 
			c.userName, 
			COALESCE(c.fio, '') as fio,
			COALESCE(c.contact, '') as contact, 
			COALESCE(c.address, '') as address,
			COALESCE(c.dateRegister, '') as dateRegister,
			COALESCE(c.dataPay, '') as dataPay,
			COALESCE(c.checks, 0) as checks,
			g.location
		FROM client c
		INNER JOIN geo g ON c.id_user = g.id_user
		WHERE g.location IS NOT NULL AND g.location != ''
		ORDER BY c.dataPay DESC;
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AdminClientEntry
	for rows.Next() {
		var entry AdminClientEntry
		var geoLocation string
		if err := rows.Scan(
			&entry.UserID, &entry.UserName,
			&entry.Fio, &entry.Contact, &entry.Address,
			&entry.DateRegister, &entry.DatePay, &entry.Checks,
			&geoLocation,
		); err != nil {
			continue
		}

		// Parse coordinates
		lat, lon := parseCoordinates(geoLocation)
		if lat != nil && lon != nil {
			// Calculate distance using Haversine formula
			distance := calculateDistance(centerLat, centerLon, *lat, *lon)
			if distance <= float64(radiusKm) {
				entry.HasGeo = true
				entry.Latitude = lat
				entry.Longitude = lon
				entries = append(entries, entry)
			}
		}
	}

	return entries, nil
}

// calculateDistance вычисляет расстояние между двумя точками (формула Haversine)
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Earth's radius in kilometers

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// GetDeliveryHeatmapData возвращает данные для тепловой карты доставок
func (r *UserRepository) GetDeliveryHeatmapData(ctx context.Context) ([]map[string]interface{}, error) {
	const q = `
		SELECT c.id_user, c.dataPay, c.checks, g.location
		FROM client c
		INNER JOIN geo g ON c.id_user = g.id_user
		WHERE g.location IS NOT NULL AND g.location != '' AND c.checks = true
		ORDER BY c.dataPay DESC;
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heatmapData []map[string]interface{}
	for rows.Next() {
		var userID int64
		var dataPay string
		var checks bool
		var location string

		if err := rows.Scan(&userID, &dataPay, &checks, &location); err != nil {
			continue
		}

		// Parse coordinates
		lat, lon := parseCoordinates(location)
		if lat != nil && lon != nil {
			heatmapData = append(heatmapData, map[string]interface{}{
				"userID":  userID,
				"lat":     *lat,
				"lon":     *lon,
				"dataPay": dataPay,
				"checks":  checks,
				"weight":  1, // Can be adjusted based on order value, etc.
			})
		}
	}

	return heatmapData, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            LEGACY COMPATIBILITY METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// GetTotalJustCount - legacy method for compatibility
func (r *UserRepository) GetTotalJustCount(ctx context.Context) (int, error) {
	return r.GetTotalUsers(ctx), nil
}

// GetTotalClientCount - legacy method for compatibility
func (r *UserRepository) GetTotalClientCount(ctx context.Context) (int, error) {
	return r.GetTotalClients(ctx), nil
}

// GetTotalLottoCount - legacy method for compatibility
func (r *UserRepository) GetTotalLottoCount(ctx context.Context) (int, error) {
	return r.GetTotalLotto(ctx), nil
}

// GetTotalGeoCount - legacy method for compatibility
func (r *UserRepository) GetTotalGeoCount(ctx context.Context) (int, error) {
	return r.GetTotalGeo(ctx), nil
}

// GetClientsWithGeoCount возвращает количество клиентов с геолокацией
func (r *UserRepository) GetClientsWithGeoCount(ctx context.Context) (int, error) {
	const q = `
		SELECT COUNT(DISTINCT c.id_user) 
		FROM client c 
		INNER JOIN geo g ON c.id_user = g.id_user 
		WHERE g.location IS NOT NULL AND g.location != '';
	`
	var count int
	err := r.db.QueryRowContext(ctx, q).Scan(&count)
	return count, err
}

// GetRecentClientEntries возвращает последние записи из таблицы client (original method)
func (r *UserRepository) GetRecentClientEntries(ctx context.Context, limit int) ([]domain.ClientEntry, error) {
	const q = `
		SELECT c.id_user, c.userName, c.fio, c.contact, c.address, c.dateRegister, c.dataPay, c.checks,
		       COALESCE(g.location, '') as geo_location
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		ORDER BY c.dataPay DESC
		LIMIT ?;
	`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []domain.ClientEntry
	for rows.Next() {
		var entry domain.ClientEntry
		var geoLocation string
		err := rows.Scan(
			&entry.UserID, &entry.UserName,
			&entry.Fio, &entry.Contact, &entry.Address,
			&entry.DateRegister, &entry.DatePay, &entry.Checks,
			&geoLocation,
		)
		if err != nil {
			continue
		}

		// If we have geo location data and address doesn't contain coordinates, add them
		if geoLocation != "" && !strings.Contains(entry.Address.String, "lat:") {
			if entry.Address.Valid {
				entry.Address.String = entry.Address.String + " " + geoLocation
			} else {
				entry.Address.String = geoLocation
				entry.Address.Valid = true
			}
		}

		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// GetRecentClientEntriesWithGeo возвращает последние записи клиентов с расширенной геолокацией для админ панели
func (r *UserRepository) GetRecentClientEntriesWithGeo(ctx context.Context, limit int) ([]ClientEntryWithGeo, error) {
	const q = `
		SELECT c.id_user, c.userName, c.fio, c.contact, c.address, c.dateRegister, c.dataPay, c.checks,
		       COALESCE(g.location, '') as geo_location
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		ORDER BY c.dataPay DESC
		LIMIT ?;
	`
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ClientEntryWithGeo
	for rows.Next() {
		var entry ClientEntryWithGeo
		var geoLocation string
		err := rows.Scan(
			&entry.UserID, &entry.UserName,
			&entry.Fio, &entry.Contact, &entry.Address,
			&entry.DateRegister, &entry.DatePay, &entry.Checks,
			&geoLocation,
		)
		if err != nil {
			continue
		}

		// Parse geolocation data
		entry.HasGeo = false
		entry.Latitude = nil
		entry.Longitude = nil

		if geoLocation != "" {
			lat, lon := parseCoordinates(geoLocation)
			if lat != nil && lon != nil {
				entry.HasGeo = true
				entry.Latitude = lat
				entry.Longitude = lon

				// Enhance address with coordinates if not already present
				if !strings.Contains(entry.Address.String, "lat:") {
					coordsText := fmt.Sprintf(" (lat:%.6f,lon:%.6f)", *lat, *lon)
					if entry.Address.Valid {
						entry.Address.String = entry.Address.String + coordsText
					} else {
						entry.Address.String = geoLocation + coordsText
						entry.Address.Valid = true
					}
				}
			}
		}

		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// GetGeoStatsByCity возвращает статистику по городам на основе геолокации
func (r *UserRepository) GetGeoStatsByCity(ctx context.Context) (map[string]int, error) {
	const q = `
		SELECT location, COUNT(*) as count
		FROM geo
		WHERE location IS NOT NULL AND location != ''
		GROUP BY location
		ORDER BY count DESC;
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cityStats := make(map[string]int)
	almaty := 0
	nursultan := 0
	shymkent := 0
	karaganda := 0
	others := 0

	for rows.Next() {
		var location string
		var count int
		err := rows.Scan(&location, &count)
		if err != nil {
			continue
		}

		// Parse coordinates and categorize
		lat, lon := parseCoordinates(location)
		if lat != nil && lon != nil {
			// Categorize by approximate coordinates for Kazakhstan cities
			if *lat >= 43.0 && *lat <= 43.5 && *lon >= 76.5 && *lon <= 77.2 {
				almaty += count // Almaty region
			} else if *lat >= 51.0 && *lat <= 51.5 && *lon >= 71.0 && *lon <= 71.8 {
				nursultan += count // Nur-Sultan/Astana region
			} else if *lat >= 42.0 && *lat <= 42.5 && *lon >= 69.0 && *lon <= 70.0 {
				shymkent += count // Shymkent region
			} else if *lat >= 49.5 && *lat <= 50.0 && *lon >= 72.5 && *lon <= 73.5 {
				karaganda += count // Karaganda region
			} else {
				others += count
			}
		} else {
			others += count
		}
	}

	cityStats["almaty"] = almaty
	cityStats["nursultan"] = nursultan
	cityStats["shymkent"] = shymkent
	cityStats["karaganda"] = karaganda
	cityStats["others"] = others

	return cityStats, rows.Err()
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            SEARCH AND FILTER METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// SearchClientsByGeoRadius ищет клиентов в радиусе от координат
func (r *UserRepository) SearchClientsByGeoRadius(ctx context.Context, lat, lon float64, radiusKm int) ([]AdminClientEntry, error) {
	return r.GetClientsByLocationRadius(ctx, lat, lon, radiusKm)
}

// GetClientsByStatus возвращает клиентов по статусу доставки
func (r *UserRepository) GetClientsByStatus(ctx context.Context, delivered bool) ([]AdminClientEntry, error) {
	const q = `
		SELECT 
			c.id_user, 
			c.userName, 
			COALESCE(c.fio, '') as fio,
			COALESCE(c.contact, '') as contact, 
			COALESCE(c.address, '') as address,
			COALESCE(c.dateRegister, '') as dateRegister,
			COALESCE(c.dataPay, '') as dataPay,
			COALESCE(c.checks, 0) as checks,
			COALESCE(g.location, '') as location
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		WHERE c.checks = ?
		ORDER BY c.dataPay DESC;
	`

	rows, err := r.db.QueryContext(ctx, q, delivered)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []AdminClientEntry
	for rows.Next() {
		var client AdminClientEntry
		var geoLocation string

		if err := rows.Scan(
			&client.UserID, &client.UserName,
			&client.Fio, &client.Contact, &client.Address,
			&client.DateRegister, &client.DatePay, &client.Checks,
			&geoLocation,
		); err != nil {
			continue
		}

		// Parse geolocation if available
		client.HasGeo = false
		if geoLocation != "" {
			lat, lon := parseCoordinates(geoLocation)
			if lat != nil && lon != nil {
				client.HasGeo = true
				client.Latitude = lat
				client.Longitude = lon
			}
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// GetClientsWithRecentPayments возвращает клиентов с недавними платежами
func (r *UserRepository) GetClientsWithRecentPayments(ctx context.Context, days int) ([]AdminClientEntry, error) {
	const q = `
		SELECT 
			c.id_user, 
			c.userName, 
			COALESCE(c.fio, '') as fio,
			COALESCE(c.contact, '') as contact, 
			COALESCE(c.address, '') as address,
			COALESCE(c.dateRegister, '') as dateRegister,
			COALESCE(c.dataPay, '') as dataPay,
			COALESCE(c.checks, 0) as checks,
			COALESCE(g.location, '') as location
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		WHERE c.dataPay IS NOT NULL 
		AND c.dataPay != ''
		AND datetime(c.dataPay) >= datetime('now', '-' || ? || ' days')
		ORDER BY c.dataPay DESC;
	`

	rows, err := r.db.QueryContext(ctx, q, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []AdminClientEntry
	for rows.Next() {
		var client AdminClientEntry
		var geoLocation string

		if err := rows.Scan(
			&client.UserID, &client.UserName,
			&client.Fio, &client.Contact, &client.Address,
			&client.DateRegister, &client.DatePay, &client.Checks,
			&geoLocation,
		); err != nil {
			continue
		}

		// Parse geolocation if available
		client.HasGeo = false
		if geoLocation != "" {
			lat, lon := parseCoordinates(geoLocation)
			if lat != nil && lon != nil {
				client.HasGeo = true
				client.Latitude = lat
				client.Longitude = lon
			}
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            UTILITY AND HELPER METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// ValidateCoordinates проверяет валидность координат
func ValidateCoordinates(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// FormatLocationString форматирует координаты в строку
func FormatLocationString(lat, lon float64) string {
	return fmt.Sprintf("%.6f,%.6f", lat, lon)
}

// InsertGeoWithCoordinates вставляет гео-запись с координатами
func (r *UserRepository) InsertGeoWithCoordinates(ctx context.Context, userID int64, lat, lon float64) error {
	if !ValidateCoordinates(lat, lon) {
		return fmt.Errorf("invalid coordinates: lat=%f, lon=%f", lat, lon)
	}

	location := FormatLocationString(lat, lon)
	geoEntry := domain.GeoEntry{
		UserID:   userID,
		Location: location,
		DataReg:  time.Now().Format("2006-01-02 15:04:05"),
	}

	return r.InsertGeo(ctx, geoEntry)
}

// UpdateGeoLocation обновляет геолокацию пользователя
func (r *UserRepository) UpdateGeoLocation(ctx context.Context, userID int64, lat, lon float64) error {
	if !ValidateCoordinates(lat, lon) {
		return fmt.Errorf("invalid coordinates: lat=%f, lon=%f", lat, lon)
	}

	const q = `
		INSERT OR REPLACE INTO geo (id_user, location, dataReg)
		VALUES (?, ?, ?);
	`
	location := FormatLocationString(lat, lon)
	_, err := r.db.ExecContext(ctx, q, userID, location, time.Now().Format("2006-01-02 15:04:05"))
	return err
}

// GetLatestGeoLocation возвращает последнюю геолокацию пользователя
func (r *UserRepository) GetLatestGeoLocation(ctx context.Context, userID int64) (*float64, *float64, error) {
	const q = `
		SELECT location
		FROM geo
		WHERE id_user = ?
		ORDER BY dataReg DESC
		LIMIT 1;
	`

	var location string
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&location)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil // No location found
		}
		return nil, nil, err
	}

	lat, lon := parseCoordinates(location)
	return lat, lon, nil
}
