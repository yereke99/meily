// Update your domain/entries.go file with these complete structures

package domain

import (
	"database/sql"
)

type PdfResult struct {
	Total       int
	ActualPrice int
	Bin         string
	Qr          string
}

type UserState struct {
	State         string `json:"state"`
	BroadCastType string `json:"broadcast_type"`
	Count         int    `json:"count"`
	Contact       string `json:"contact"`
	IsPaid        bool   `json:"is_paid"`
}

// JustEntry represents a user registration in the just table
type JustEntry struct {
	ID             int64  `json:"id" db:"id"`
	UserID         int64  `json:"userID" db:"id_user"`
	UserName       string `json:"userName" db:"userName"`
	DateRegistered string `json:"dateRegistered" db:"dataRegistred"`
}

// ClientEntry represents a paying client in the client table
type ClientEntry struct {
	ID           int64          `json:"id" db:"id"`
	UserID       int64          `json:"userID" db:"id_user"`
	UserName     string         `json:"userName" db:"userName"`
	Fio          sql.NullString `json:"fio" db:"fio"`
	Contact      string         `json:"contact" db:"contact"`
	Address      sql.NullString `json:"address" db:"address"`
	DateRegister sql.NullString `json:"dateRegister" db:"dateRegister"`
	DatePay      string         `json:"dataPay" db:"dataPay"`
	Checks       bool           `json:"checks" db:"checks"`
}

// LotoEntry represents a lottery participant in the loto table
type LotoEntry struct {
	ID      int64          `json:"id" db:"id"`
	UserID  int64          `json:"userID" db:"id_user"`
	LotoID  int            `json:"lotoID" db:"id_loto"`
	QR      sql.NullString `json:"qr" db:"qr"`
	WhoPaid sql.NullString `json:"whoPaid" db:"who_paid"`
	Receipt sql.NullString `json:"receipt" db:"receipt"`
	Fio     sql.NullString `json:"fio" db:"fio"`
	Contact sql.NullString `json:"contact" db:"contact"`
	Address sql.NullString `json:"address" db:"address"`
	DatePay sql.NullString `json:"datePay" db:"dataPay"`
}

// GeoEntry represents geolocation data in the geo table
type GeoEntry struct {
	ID       int64  `json:"id" db:"id"`
	UserID   int64  `json:"userID" db:"id_user"`
	Location string `json:"location" db:"location"`
	DataReg  string `json:"dataReg" db:"dataReg"`
}
