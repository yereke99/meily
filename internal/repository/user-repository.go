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
	HasGeo         bool     `json:"hasGeo"`
	Latitude       *float64 `json:"latitude,omitempty"`
	Longitude      *float64 `json:"longitude,omitempty"`
	AccuracyMeters *int     `json:"accuracyMeters,omitempty"`
	City           *string  `json:"city,omitempty"`
	Country        string   `json:"country"`
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
	UserID         int64     `json:"userID"`
	UserName       string    `json:"userName"`
	Fio            string    `json:"fio"`
	Contact        string    `json:"contact"`
	Address        string    `json:"address"`
	DateRegister   string    `json:"dateRegister"`
	DatePay        string    `json:"dataPay"`
	Checks         bool      `json:"checks"`
	HasGeo         bool      `json:"hasGeo"`
	Latitude       *float64  `json:"latitude"`
	Longitude      *float64  `json:"longitude"`
	AccuracyMeters *int      `json:"accuracyMeters"`
	City           *string   `json:"city"`
	Country        string    `json:"country"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// BotSession represents bot session data
type BotSession struct {
	ID           int             `json:"id"`
	UserID       int64           `json:"userID"`
	SessionID    string          `json:"sessionID"`
	State        string          `json:"state"`
	Data         json.RawMessage `json:"data,omitempty"`
	LastActivity time.Time       `json:"lastActivity"`
	ExpiresAt    *time.Time      `json:"expiresAt,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

// AdminLog represents admin action log
type AdminLog struct {
	ID           int             `json:"id"`
	AdminUserID  int64           `json:"adminUserID"`
	Action       string          `json:"action"`
	TargetUserID *int64          `json:"targetUserID,omitempty"`
	Details      json.RawMessage `json:"details,omitempty"`
	IPAddress    *string         `json:"ipAddress,omitempty"`
	UserAgent    *string         `json:"userAgent,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// UserRepository работает со всеми таблицами: just, client, loto, geo, bot_sessions, admin_logs.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository создаёт новый UserRepository.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            ENHANCED GEO ANALYTICS METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// GetClientsByLocationRadius возвращает клиентов в радиусе от заданной точки
func (r *UserRepository) GetClientsByLocationRadius(ctx context.Context, centerLat, centerLon float64, radiusKm int) ([]AdminClientEntry, error) {
	const q = `
		SELECT 
			c.id_user, c.userName, 
			COALESCE(c.fio, '') as fio,
			COALESCE(c.contact, '') as contact, 
			COALESCE(c.address, '') as address,
			COALESCE(c.dateRegister, '') as dateRegister,
			COALESCE(c.dataPay, '') as dataPay,
			COALESCE(c.checks, 0) as checks,
			c.created_at, c.updated_at,
			g.latitude, g.longitude, g.accuracy_meters, g.city, g.country
		FROM client c
		INNER JOIN geo g ON c.id_user = g.id_user
		WHERE g.latitude IS NOT NULL AND g.longitude IS NOT NULL
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
		var lat, lon sql.NullFloat64
		var accuracy sql.NullInt64
		var city sql.NullString
		var country sql.NullString

		if err := rows.Scan(
			&entry.UserID, &entry.UserName,
			&entry.Fio, &entry.Contact, &entry.Address,
			&entry.DateRegister, &entry.DatePay, &entry.Checks,
			&entry.CreatedAt, &entry.UpdatedAt,
			&lat, &lon, &accuracy, &city, &country,
		); err != nil {
			continue
		}

		// Parse coordinates and calculate distance
		if lat.Valid && lon.Valid {
			distance := calculateDistance(centerLat, centerLon, lat.Float64, lon.Float64)
			if distance <= float64(radiusKm) {
				entry.HasGeo = true
				entry.Latitude = &lat.Float64
				entry.Longitude = &lon.Float64

				if accuracy.Valid {
					accuracyInt := int(accuracy.Int64)
					entry.AccuracyMeters = &accuracyInt
				}
				if city.Valid {
					entry.City = &city.String
				}
				if country.Valid {
					entry.Country = country.String
				} else {
					entry.Country = "Kazakhstan"
				}

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
		SELECT c.id_user, c.dataPay, c.checks, g.latitude, g.longitude
		FROM client c
		INNER JOIN geo g ON c.id_user = g.id_user
		WHERE g.latitude IS NOT NULL AND g.longitude IS NOT NULL AND c.checks = true
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
		var lat, lon float64

		if err := rows.Scan(&userID, &dataPay, &checks, &lat, &lon); err != nil {
			continue
		}

		heatmapData = append(heatmapData, map[string]interface{}{
			"userID":  userID,
			"lat":     lat,
			"lon":     lon,
			"dataPay": dataPay,
			"checks":  checks,
			"weight":  1,
		})
	}

	return heatmapData, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            BOT SESSIONS METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// CreateBotSession создает новую сессию бота (SQLite version)
func (r *UserRepository) CreateBotSession(ctx context.Context, userID int64, sessionID, state string, data json.RawMessage, expiresAt *time.Time) error {
	const q = `
		INSERT OR REPLACE INTO bot_sessions (user_id, session_id, state, data, expires_at, last_activity, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'));
	`
	_, err := r.db.ExecContext(ctx, q, userID, sessionID, state, data, expiresAt)
	return err
}

// GetBotSession получает сессию бота
func (r *UserRepository) GetBotSession(ctx context.Context, userID int64, sessionID string) (*BotSession, error) {
	const q = `
		SELECT id, user_id, session_id, state, data, last_activity, expires_at, created_at, updated_at
		FROM bot_sessions
		WHERE user_id = ? AND session_id = ?;
	`

	var session BotSession
	err := r.db.QueryRowContext(ctx, q, userID, sessionID).Scan(
		&session.ID, &session.UserID, &session.SessionID, &session.State,
		&session.Data, &session.LastActivity, &session.ExpiresAt,
		&session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &session, nil
}

// UpdateBotSession обновляет сессию бота (SQLite version)
func (r *UserRepository) UpdateBotSession(ctx context.Context, userID int64, sessionID, state string, data json.RawMessage) error {
	const q = `
		UPDATE bot_sessions 
		SET state = ?, data = ?, last_activity = datetime('now'), updated_at = datetime('now')
		WHERE user_id = ? AND session_id = ?;
	`
	_, err := r.db.ExecContext(ctx, q, state, data, userID, sessionID)
	return err
}

// DeleteBotSession удаляет сессию бота
func (r *UserRepository) DeleteBotSession(ctx context.Context, userID int64, sessionID string) error {
	const q = `DELETE FROM bot_sessions WHERE user_id = ? AND session_id = ?;`
	_, err := r.db.ExecContext(ctx, q, userID, sessionID)
	return err
}

// CleanupExpiredSessions удаляет истекшие сессии (SQLite version)
func (r *UserRepository) CleanupExpiredSessions(ctx context.Context) error {
	const q = `
		DELETE FROM bot_sessions 
		WHERE expires_at IS NOT NULL AND expires_at < datetime('now')
		   OR last_activity < datetime('now', '-24 hours');
	`
	_, err := r.db.ExecContext(ctx, q)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            ADMIN LOGS METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// CreateAdminLog создает запись в логе администратора
func (r *UserRepository) CreateAdminLog(ctx context.Context, adminUserID int64, action string, targetUserID *int64, details json.RawMessage, ipAddress, userAgent *string) error {
	const q = `
		INSERT INTO admin_logs (admin_user_id, action, target_user_id, details, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?);
	`
	_, err := r.db.ExecContext(ctx, q, adminUserID, action, targetUserID, details, ipAddress, userAgent)
	return err
}

// GetAdminLogs получает логи администратора
func (r *UserRepository) GetAdminLogs(ctx context.Context, limit int) ([]AdminLog, error) {
	const q = `
		SELECT id, admin_user_id, action, target_user_id, details, ip_address, user_agent, created_at
		FROM admin_logs
		ORDER BY created_at DESC
		LIMIT ?;
	`

	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AdminLog
	for rows.Next() {
		var log AdminLog
		err := rows.Scan(
			&log.ID, &log.AdminUserID, &log.Action, &log.TargetUserID,
			&log.Details, &log.IPAddress, &log.UserAgent, &log.CreatedAt,
		)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            ENHANCED GEO METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// InsertGeoWithEnhancements вставляет расширенную гео-запись (SQLite version)
func (r *UserRepository) InsertGeoWithEnhancements(ctx context.Context, userID int64, location string, lat, lon *float64, accuracyMeters *int, addressComponents json.RawMessage, city, country *string) error {
	const q = `
		INSERT OR REPLACE INTO geo (id_user, location, dataReg, latitude, longitude, accuracy_meters, address_components, city, country, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'));
	`

	now := time.Now().Format("2006-01-02 15:04:05")
	countryVal := "Kazakhstan"
	if country != nil {
		countryVal = *country
	}

	// Convert JSON to string for SQLite
	var addressComponentsStr *string
	if addressComponents != nil {
		str := string(addressComponents)
		addressComponentsStr = &str
	}

	_, err := r.db.ExecContext(ctx, q, userID, location, now, lat, lon, accuracyMeters, addressComponentsStr, city, countryVal)
	return err
}

// GetGeoWithEnhancements получает расширенную гео-информацию
func (r *UserRepository) GetGeoWithEnhancements(ctx context.Context, userID int64) (*domain.GeoEntry, *float64, *float64, *int, *string, error) {
	const q = `
		SELECT location, dataReg, latitude, longitude, accuracy_meters, city
		FROM geo
		WHERE id_user = ?
		ORDER BY updated_at DESC
		LIMIT 1;
	`

	var geo domain.GeoEntry
	var lat, lon sql.NullFloat64
	var accuracy sql.NullInt64
	var city sql.NullString

	err := r.db.QueryRowContext(ctx, q, userID).Scan(
		&geo.Location, &geo.DataReg, &lat, &lon, &accuracy, &city,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	geo.UserID = userID

	var latPtr, lonPtr *float64
	var accuracyPtr *int
	var cityPtr *string

	if lat.Valid {
		latPtr = &lat.Float64
	}
	if lon.Valid {
		lonPtr = &lon.Float64
	}
	if accuracy.Valid {
		accuracyInt := int(accuracy.Int64)
		accuracyPtr = &accuracyInt
	}
	if city.Valid {
		cityPtr = &city.String
	}

	return &geo, latPtr, lonPtr, accuracyPtr, cityPtr, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            UPDATED EXISTING METHODS TO MATCH SCHEMA
// ═══════════════════════════════════════════════════════════════════════════════

// InsertJust вставляет запись в таблицу just с учетом новых полей (SQLite version)
func (r *UserRepository) InsertJust(ctx context.Context, e domain.JustEntry) error {
	const q = `
		INSERT OR REPLACE INTO just (id_user, userName, dataRegistred, updated_at)
		VALUES (?, ?, ?, datetime('now'));
	`
	_, err := r.db.ExecContext(ctx, q, e.UserID, e.UserName, e.DateRegistered)
	return err
}

// InsertClient вставляет запись в таблицу client с учетом новых полей (SQLite version)
func (r *UserRepository) InsertClient(ctx context.Context, e domain.ClientEntry) error {
	const q = `
		INSERT OR REPLACE INTO client (id_user, userName, fio, contact, address, dateRegister, dataPay, checks, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'));
	`
	_, err := r.db.ExecContext(ctx, q,
		e.UserID, e.UserName, e.Fio, e.Contact,
		e.Address, e.DateRegister, e.DatePay, e.Checks,
	)
	return err
}

// InsertLoto вставляет запись в таблицу loto с учетом уникального ключа (SQLite version)
func (r *UserRepository) InsertLoto(ctx context.Context, e domain.LotoEntry) error {
	const q = `
		INSERT OR REPLACE INTO loto (id_user, id_loto, qr, who_paid, receipt, fio, contact, address, dataPay, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'));
	`
	_, err := r.db.ExecContext(ctx, q,
		e.UserID, e.LotoID, e.QR, e.WhoPaid,
		e.Receipt, e.Fio, e.Contact, e.Address, e.DatePay,
	)
	return err
}

// InsertGeo вставляет запись в таблицу geo (legacy support)
func (r *UserRepository) InsertGeo(ctx context.Context, e domain.GeoEntry) error {
	// Parse coordinates from location string if possible
	lat, lon := parseCoordinates(e.Location)

	return r.InsertGeoWithEnhancements(ctx, e.UserID, e.Location, lat, lon, nil, nil, nil, nil)
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            ALL REMAINING METHODS (PRESERVED)
// ═══════════════════════════════════════════════════════════════════════════════

// [All other existing methods remain the same - ExistsJust, ExistsClient, etc.]
// [Preserving all legacy compatibility and existing functionality]

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

// ExistsJust проверяет, есть ли запись в just по id_user
func (r *UserRepository) ExistsJust(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM just WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// ExistsClient проверяет, есть ли запись в client по id_user
func (r *UserRepository) ExistsClient(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM client WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// ExistsLoto проверяет, есть ли запись в loto по id_user
func (r *UserRepository) ExistsLoto(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM loto WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// ExistsGeo проверяет, есть ли запись в geo по id_user
func (r *UserRepository) ExistsGeo(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM geo WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// IsClientUnique возвращает true, если в client нет записи с данным id_user
func (r *UserRepository) IsClientUnique(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT COUNT(1) FROM client WHERE id_user = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, userID).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt == 0, nil
}

func (r *UserRepository) IsQrUnique(ctx context.Context, qrCode string) (bool, error) {
	const q = `SELECT COUNT(1) FROM loto WHERE qr = ?;`
	var cnt int
	if err := r.db.QueryRowContext(ctx, q, qrCode).Scan(&cnt); err != nil {
		return false, err
	}
	return cnt == 0, nil
}

// IsClientPaid проверяет, оплачен ли клиент
func (r *UserRepository) IsClientPaid(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT checks FROM client WHERE id_user = ?;`
	var checks bool
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&checks)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return checks, nil
}

// IsLotoPaid возвращает true, если для данного id_user и id_loto есть непустое who_paid
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

// parseCoordinates парсит координаты из строки местоположения (enhanced version)
func parseCoordinates(location string) (*float64, *float64) {
	// Format 1: "lat,lon"
	if strings.Contains(location, ",") && !strings.Contains(location, ":") {
		parts := strings.Split(location, ",")
		if len(parts) >= 2 {
			lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 == nil && err2 == nil && ValidateCoordinates(lat, lon) {
				return &lat, &lon
			}
		}
	}

	// Format 2: "latitude: 43.2, longitude: 76.8"
	if strings.Contains(location, "latitude:") && strings.Contains(location, "longitude:") {
		latStart := strings.Index(location, "latitude:") + 9
		lonStart := strings.Index(location, "longitude:") + 10

		latEnd := strings.Index(location[latStart:], ",")
		if latEnd == -1 {
			latEnd = len(location) - latStart
		}

		lonEnd := len(location) - lonStart
		if commaIndex := strings.Index(location[lonStart:], ","); commaIndex != -1 {
			lonEnd = commaIndex
		}

		latStr := strings.TrimSpace(location[latStart : latStart+latEnd])
		lonStr := strings.TrimSpace(location[lonStart : lonStart+lonEnd])

		lat, err1 := strconv.ParseFloat(latStr, 64)
		lon, err2 := strconv.ParseFloat(lonStr, 64)
		if err1 == nil && err2 == nil && ValidateCoordinates(lat, lon) {
			return &lat, &lon
		}
	}

	// Format 3: JSON-like format {"lat": 43.2, "lon": 76.8}
	if strings.Contains(location, "{") && strings.Contains(location, "}") {
		var coords map[string]float64
		if err := json.Unmarshal([]byte(location), &coords); err == nil {
			if lat, ok1 := coords["lat"]; ok1 {
				if lon, ok2 := coords["lon"]; ok2 {
					if ValidateCoordinates(lat, lon) {
						return &lat, &lon
					}
				}
			}
			if lat, ok1 := coords["latitude"]; ok1 {
				if lon, ok2 := coords["longitude"]; ok2 {
					if ValidateCoordinates(lat, lon) {
						return &lat, &lon
					}
				}
			}
		}
	}

	return nil, nil
}

// ValidateCoordinates проверяет валидность координат
func ValidateCoordinates(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// FormatLocationString форматирует координаты в строку
func FormatLocationString(lat, lon float64) string {
	return fmt.Sprintf("%.6f,%.6f", lat, lon)
}

// ═══════════════════════════════════════════════════════════════════════════════
//                            REMAINING ENHANCED METHODS
// ═══════════════════════════════════════════════════════════════════════════════

// GetAllJustUserIDs returns all user IDs from just table
func (r *UserRepository) GetAllJustUserIDs(ctx context.Context) ([]int64, error) {
	const q = `SELECT id_user FROM just ORDER BY created_at DESC;`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, nil
}

// GetClientByUserID получает данные клиента по user ID
func (r *UserRepository) GetClientByUserID(ctx context.Context, userID int64) (*domain.ClientEntry, error) {
	const q = `
		SELECT id_user, userName, fio, contact, address, dateRegister, dataPay, checks
		FROM client
		WHERE id_user = ? AND checks = false;
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

// UpdateClientDeliveryData обновляет данные доставки клиента (SQLite version)
func (r *UserRepository) UpdateClientDeliveryData(ctx context.Context, userID int64, fio, address string, latitude, longitude float64) error {
	const q = `
		UPDATE client 
		SET fio = ?, address = ?, checks = true, updated_at = datetime('now')
		WHERE id_user = ?;
	`
	_, err := r.db.ExecContext(ctx, q, fio, address, userID)
	if err != nil {
		return err
	}

	// Also insert/update geo data with enhanced fields
	return r.InsertGeoWithEnhancements(ctx, userID,
		FormatLocationString(latitude, longitude),
		&latitude, &longitude, nil, nil, nil, nil)
}

// GetAllClientsWithDeliveryData получает всех клиентов с данными доставки
func (r *UserRepository) GetAllClientsWithDeliveryData(ctx context.Context) ([]domain.ClientEntry, error) {
	const q = `
		SELECT id_user, userName, fio, contact, address, dateRegister, dataPay, checks
		FROM client
		WHERE checks = true
		ORDER BY updated_at DESC;
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

// GetRecentJustEntries возвращает последние записи из таблицы just
func (r *UserRepository) GetRecentJustEntries(ctx context.Context, limit int) ([]domain.JustEntry, error) {
	const q = `
		SELECT id_user, userName, dataRegistred
		FROM just
		ORDER BY created_at DESC
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
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetRecentClientEntries возвращает последние записи из таблицы client
func (r *UserRepository) GetRecentClientEntries(ctx context.Context, limit int) ([]domain.ClientEntry, error) {
	const q = `
		SELECT c.id_user, c.userName, c.fio, c.contact, c.address, c.dateRegister, c.dataPay, c.checks,
		       COALESCE(g.location, '') as geo_location
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		ORDER BY c.updated_at DESC
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

// GetRecentClientEntriesWithGeo возвращает последние записи клиентов с расширенной геолокацией
func (r *UserRepository) GetRecentClientEntriesWithGeo(ctx context.Context, limit int) ([]ClientEntryWithGeo, error) {
	const q = `
		SELECT c.id_user, c.userName, c.fio, c.contact, c.address, c.dateRegister, c.dataPay, c.checks,
		       g.latitude, g.longitude, g.accuracy_meters, g.city, g.country
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		ORDER BY c.updated_at DESC
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
		var lat, lon sql.NullFloat64
		var accuracy sql.NullInt64
		var city, country sql.NullString

		err := rows.Scan(
			&entry.UserID, &entry.UserName,
			&entry.Fio, &entry.Contact, &entry.Address,
			&entry.DateRegister, &entry.DatePay, &entry.Checks,
			&lat, &lon, &accuracy, &city, &country,
		)
		if err != nil {
			continue
		}

		// Parse geolocation data
		entry.HasGeo = false
		if lat.Valid && lon.Valid {
			entry.HasGeo = true
			entry.Latitude = &lat.Float64
			entry.Longitude = &lon.Float64

			if accuracy.Valid {
				accuracyInt := int(accuracy.Int64)
				entry.AccuracyMeters = &accuracyInt
			}
			if city.Valid {
				entry.City = &city.String
			}
			if country.Valid {
				entry.Country = country.String
			} else {
				entry.Country = "Kazakhstan"
			}

			// Enhance address with coordinates if not already present
			if !strings.Contains(entry.Address.String, "lat:") {
				coordsText := fmt.Sprintf(" (lat:%.6f,lon:%.6f)", lat.Float64, lon.Float64)
				if entry.Address.Valid {
					entry.Address.String = entry.Address.String + coordsText
				} else {
					entry.Address.String = coordsText
					entry.Address.Valid = true
				}
			}
		}

		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// GetRecentLotoEntries возвращает последние записи из таблицы loto
func (r *UserRepository) GetRecentLotoEntries(ctx context.Context, limit int) ([]domain.LotoEntry, error) {
	const q = `
		SELECT id_user, id_loto, qr, who_paid, receipt, fio, contact, address, dataPay
		FROM loto
		ORDER BY updated_at DESC
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
		ORDER BY updated_at DESC
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

// GetAllGeoEntries возвращает ВСЕ записи из таблицы geo для карты
func (r *UserRepository) GetAllGeoEntries(ctx context.Context) ([]domain.GeoEntry, error) {
	const q = `
		SELECT id_user, location, dataReg
		FROM geo
		WHERE location IS NOT NULL AND location != ''
		ORDER BY updated_at DESC;
	`
	rows, err := r.db.QueryContext(ctx, q)
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
			c.id_user, c.userName, 
			COALESCE(c.fio, '') as fio,
			COALESCE(c.contact, '') as contact, 
			COALESCE(c.address, '') as address,
			COALESCE(c.dateRegister, '') as dateRegister,
			COALESCE(c.dataPay, '') as dataPay,
			COALESCE(c.checks, 0) as checks,
			c.created_at, c.updated_at,
			g.latitude, g.longitude, g.accuracy_meters, g.city, g.country
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		ORDER BY c.updated_at DESC;
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []AdminClientEntry
	for rows.Next() {
		var client AdminClientEntry
		var lat, lon sql.NullFloat64
		var accuracy sql.NullInt64
		var city, country sql.NullString

		if err := rows.Scan(
			&client.UserID, &client.UserName,
			&client.Fio, &client.Contact, &client.Address,
			&client.DateRegister, &client.DatePay, &client.Checks,
			&client.CreatedAt, &client.UpdatedAt,
			&lat, &lon, &accuracy, &city, &country,
		); err != nil {
			continue
		}

		// Parse geolocation if available
		client.HasGeo = false
		if lat.Valid && lon.Valid {
			client.HasGeo = true
			client.Latitude = &lat.Float64
			client.Longitude = &lon.Float64

			if accuracy.Valid {
				accuracyInt := int(accuracy.Int64)
				client.AccuracyMeters = &accuracyInt
			}
			if city.Valid {
				client.City = &city.String
			}
			if country.Valid {
				client.Country = country.String
			} else {
				client.Country = "Kazakhstan"
			}
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// GetClientsWithGeoCount возвращает количество клиентов с геолокацией
func (r *UserRepository) GetClientsWithGeoCount(ctx context.Context) (int, error) {
	const q = `
		SELECT COUNT(DISTINCT c.id_user) 
		FROM client c 
		INNER JOIN geo g ON c.id_user = g.id_user 
		WHERE g.latitude IS NOT NULL AND g.longitude IS NOT NULL;
	`
	var count int
	err := r.db.QueryRowContext(ctx, q).Scan(&count)
	return count, err
}

// GetGeoStatsByCity возвращает статистику по городам на основе геолокации
func (r *UserRepository) GetGeoStatsByCity(ctx context.Context) (map[string]int, error) {
	const q = `
		SELECT city, COUNT(*) as count
		FROM geo
		WHERE city IS NOT NULL AND city != ''
		GROUP BY city
		ORDER BY count DESC;
	`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cityStats := make(map[string]int)
	for rows.Next() {
		var city string
		var count int
		err := rows.Scan(&city, &count)
		if err != nil {
			continue
		}
		cityStats[strings.ToLower(city)] = count
	}

	return cityStats, rows.Err()
}

// SearchClientsByGeoRadius ищет клиентов в радиусе от координат
func (r *UserRepository) SearchClientsByGeoRadius(ctx context.Context, lat, lon float64, radiusKm int) ([]AdminClientEntry, error) {
	return r.GetClientsByLocationRadius(ctx, lat, lon, radiusKm)
}

// GetClientsByStatus возвращает клиентов по статусу доставки
func (r *UserRepository) GetClientsByStatus(ctx context.Context, delivered bool) ([]AdminClientEntry, error) {
	const q = `
		SELECT 
			c.id_user, c.userName, 
			COALESCE(c.fio, '') as fio,
			COALESCE(c.contact, '') as contact, 
			COALESCE(c.address, '') as address,
			COALESCE(c.dateRegister, '') as dateRegister,
			COALESCE(c.dataPay, '') as dataPay,
			COALESCE(c.checks, 0) as checks,
			c.created_at, c.updated_at,
			g.latitude, g.longitude, g.accuracy_meters, g.city, g.country
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		WHERE c.checks = ?
		ORDER BY c.updated_at DESC;
	`

	rows, err := r.db.QueryContext(ctx, q, delivered)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []AdminClientEntry
	for rows.Next() {
		var client AdminClientEntry
		var lat, lon sql.NullFloat64
		var accuracy sql.NullInt64
		var city, country sql.NullString

		if err := rows.Scan(
			&client.UserID, &client.UserName,
			&client.Fio, &client.Contact, &client.Address,
			&client.DateRegister, &client.DatePay, &client.Checks,
			&client.CreatedAt, &client.UpdatedAt,
			&lat, &lon, &accuracy, &city, &country,
		); err != nil {
			continue
		}

		// Parse geolocation if available
		client.HasGeo = false
		if lat.Valid && lon.Valid {
			client.HasGeo = true
			client.Latitude = &lat.Float64
			client.Longitude = &lon.Float64

			if accuracy.Valid {
				accuracyInt := int(accuracy.Int64)
				client.AccuracyMeters = &accuracyInt
			}
			if city.Valid {
				client.City = &city.String
			}
			if country.Valid {
				client.Country = country.String
			} else {
				client.Country = "Kazakhstan"
			}
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// GetClientsWithRecentPayments возвращает клиентов с недавними платежами (SQLite version)
func (r *UserRepository) GetClientsWithRecentPayments(ctx context.Context, days int) ([]AdminClientEntry, error) {
	const q = `
		SELECT 
			c.id_user, c.userName, 
			COALESCE(c.fio, '') as fio,
			COALESCE(c.contact, '') as contact, 
			COALESCE(c.address, '') as address,
			COALESCE(c.dateRegister, '') as dateRegister,
			COALESCE(c.dataPay, '') as dataPay,
			COALESCE(c.checks, 0) as checks,
			c.created_at, c.updated_at,
			g.latitude, g.longitude, g.accuracy_meters, g.city, g.country
		FROM client c
		LEFT JOIN geo g ON c.id_user = g.id_user
		WHERE c.updated_at >= datetime('now', '-' || ? || ' days')
		ORDER BY c.updated_at DESC;
	`

	rows, err := r.db.QueryContext(ctx, q, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []AdminClientEntry
	for rows.Next() {
		var client AdminClientEntry
		var lat, lon sql.NullFloat64
		var accuracy sql.NullInt64
		var city, country sql.NullString

		if err := rows.Scan(
			&client.UserID, &client.UserName,
			&client.Fio, &client.Contact, &client.Address,
			&client.DateRegister, &client.DatePay, &client.Checks,
			&client.CreatedAt, &client.UpdatedAt,
			&lat, &lon, &accuracy, &city, &country,
		); err != nil {
			continue
		}

		// Parse geolocation if available
		client.HasGeo = false
		if lat.Valid && lon.Valid {
			client.HasGeo = true
			client.Latitude = &lat.Float64
			client.Longitude = &lon.Float64

			if accuracy.Valid {
				accuracyInt := int(accuracy.Int64)
				client.AccuracyMeters = &accuracyInt
			}
			if city.Valid {
				client.City = &city.String
			}
			if country.Valid {
				client.Country = country.String
			} else {
				client.Country = "Kazakhstan"
			}
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// InsertGeoWithCoordinates вставляет гео-запись с координатами
func (r *UserRepository) InsertGeoWithCoordinates(ctx context.Context, userID int64, lat, lon float64) error {
	if !ValidateCoordinates(lat, lon) {
		return fmt.Errorf("invalid coordinates: lat=%f, lon=%f", lat, lon)
	}

	location := FormatLocationString(lat, lon)
	return r.InsertGeoWithEnhancements(ctx, userID, location, &lat, &lon, nil, nil, nil, nil)
}

// UpdateGeoLocation обновляет геолокацию пользователя
func (r *UserRepository) UpdateGeoLocation(ctx context.Context, userID int64, lat, lon float64) error {
	if !ValidateCoordinates(lat, lon) {
		return fmt.Errorf("invalid coordinates: lat=%f, lon=%f", lat, lon)
	}

	location := FormatLocationString(lat, lon)
	return r.InsertGeoWithEnhancements(ctx, userID, location, &lat, &lon, nil, nil, nil, nil)
}

// GetLatestGeoLocation возвращает последнюю геолокацию пользователя
func (r *UserRepository) GetLatestGeoLocation(ctx context.Context, userID int64) (*float64, *float64, error) {
	const q = `
		SELECT latitude, longitude
		FROM geo
		WHERE id_user = ?
		ORDER BY updated_at DESC
		LIMIT 1;
	`

	var lat, lon sql.NullFloat64
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&lat, &lon)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var latPtr, lonPtr *float64
	if lat.Valid {
		latPtr = &lat.Float64
	}
	if lon.Valid {
		lonPtr = &lon.Float64
	}

	return latPtr, lonPtr, nil
}

// GetLottoStats возвращает статистику лотереи
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
		SELECT latitude, longitude, COUNT(*) as count
		FROM geo
		WHERE latitude IS NOT NULL AND longitude IS NOT NULL
		GROUP BY ROUND(latitude, 1), ROUND(longitude, 1);
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return &GeoStats{Almaty: 0, Nursultan: 0, Shymkent: 0, Karaganda: 0, Others: 0}
	}
	defer rows.Close()

	var stats GeoStats
	for rows.Next() {
		var lat, lon float64
		var count int
		if err := rows.Scan(&lat, &lon, &count); err != nil {
			continue
		}

		// Categorize by approximate coordinates for Kazakhstan cities
		if lat >= 43.0 && lat <= 43.5 && lon >= 76.5 && lon <= 77.2 {
			stats.Almaty += count // Almaty region
		} else if lat >= 51.0 && lat <= 51.5 && lon >= 71.0 && lon <= 71.8 {
			stats.Nursultan += count // Nur-Sultan/Astana region
		} else if lat >= 42.0 && lat <= 42.5 && lon >= 69.0 && lon <= 70.0 {
			stats.Shymkent += count // Shymkent region
		} else if lat >= 49.5 && lat <= 50.0 && lon >= 72.5 && lon <= 73.5 {
			stats.Karaganda += count // Karaganda region
		} else {
			stats.Others += count
		}
	}

	return &stats
}

// Legacy compatibility methods
func (r *UserRepository) GetTotalJustCount(ctx context.Context) (int, error) {
	return r.GetTotalUsers(ctx), nil
}

func (r *UserRepository) GetTotalClientCount(ctx context.Context) (int, error) {
	return r.GetTotalClients(ctx), nil
}

func (r *UserRepository) GetTotalLottoCount(ctx context.Context) (int, error) {
	return r.GetTotalLotto(ctx), nil
}

func (r *UserRepository) GetTotalGeoCount(ctx context.Context) (int, error) {
	return r.GetTotalGeo(ctx), nil
}
