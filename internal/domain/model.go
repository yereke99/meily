// ── internal/domain/models.go ────────────────────────────────────────────────
package domain

import (
	"database/sql"
)

// JustEntry представляет запись в таблице just.
type JustEntry struct {
	ID             int    `json:"id" db:"id"`
	UserID         int64  `json:"user_id" db:"id_user"`
	UserName       string `json:"user_name" db:"userName"`
	DateRegistered string `json:"date_registered" db:"dataRegistred"`
}

// ClientEntry представляет запись в таблице client.
type ClientEntry struct {
	ID           int             `json:"id" db:"id"`
	UserID       int64           `json:"user_id" db:"id_user"`
	UserName     string          `json:"user_name" db:"userName"`
	Fio          sql.NullString  `json:"fio" db:"fio"`
	Contact      string          `json:"contact" db:"contact"`
	Address      sql.NullString  `json:"address" db:"address"`
	DateRegister sql.NullString  `json:"date_register" db:"dateRegister"`
	DatePay      string          `json:"date_pay" db:"dataPay"`
	Checks       bool            `json:"checks" db:"checks"`
	Latitude     sql.NullFloat64 `json:"latitude,omitempty" db:"latitude"`
	Longitude    sql.NullFloat64 `json:"longitude,omitempty" db:"longitude"`
}

// LotoEntry представляет запись в таблице loto.
type LotoEntry struct {
	ID      int            `json:"id" db:"id"`
	UserID  int64          `json:"user_id" db:"id_user"`
	LotoID  int            `json:"loto_id" db:"id_loto"`
	QR      sql.NullString `json:"qr" db:"qr"`
	WhoPaid sql.NullString `json:"who_paid" db:"who_paid"`
	Receipt sql.NullString `json:"receipt" db:"receipt"`
	Fio     sql.NullString `json:"fio" db:"fio"`
	Contact sql.NullString `json:"contact" db:"contact"`
	Address sql.NullString `json:"address" db:"address"`
	DatePay sql.NullString `json:"date_pay" db:"dataPay"`
}

// GeoEntry представляет запись в таблице geo.
type GeoEntry struct {
	ID       int            `json:"id" db:"id"`
	UserID   int64          `json:"user_id" db:"id_user"`
	Location sql.NullString `json:"location" db:"location"`
	DataReg  sql.NullString `json:"data_reg" db:"dataReg"`
}

// DeliveryLocation представляет координаты доставки.
type DeliveryLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Address   string  `json:"address"`
}

// ClientDeliveryData представляет полные данные клиента для доставки.
type ClientDeliveryData struct {
	UserID       int64            `json:"user_id"`
	UserName     string           `json:"user_name"`
	Fio          string           `json:"fio"`
	Contact      string           `json:"contact"`
	Address      string           `json:"address"`
	Location     DeliveryLocation `json:"location"`
	DateRegister string           `json:"date_register"`
	DatePay      string           `json:"date_pay"`
	Checks       bool             `json:"checks"`
}
