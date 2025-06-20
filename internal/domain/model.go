package domain

import "database/sql"

// JustEntry представляет запись в таблице just
// dataRegistred хранится в поле DateRegistered
// поле DateRegistered соответствует колонке dataRegistred в БД
// id_user — Telegram user ID
// userName — имя пользователя
// dataRegistred — дата регистрации (строка)
type JustEntry struct {
	UserID         int64  // corresponds to id_user
	UserName       string // corresponds to userName
	DateRegistered string // corresponds to dataRegistred
}

// ClientEntry представляет запись в таблице client
// fio — ФИО клиента
// checks — флаг оплаты
// dateRegister — дата регистрации, dataPay — дата оплаты (строки)
type ClientEntry struct {
	UserID       int64
	UserName     string
	Fio          sql.NullString
	Contact      string
	Address      sql.NullString
	DateRegister sql.NullString
	DatePay      string
	Checks       bool
}

// LotoEntry представляет запись в таблице loto
// id_loto — номер лото, who_paid — оплативший, receipt — номер квитанции
type LotoEntry struct {
	UserID  int64  // corresponds to id_user
	LotoID  int    // corresponds to id_loto
	QR      string // corresponds to qr
	WhoPaid string // corresponds to who_paid
	Receipt string // corresponds to receipt
	Fio     string // corresponds to fio
	Contact string // corresponds to contact
	Address string // corresponds to address
	DatePay string // corresponds to dataPay
}

// GeoEntry представляет запись в таблице geo
// location — координаты или описание локации
type GeoEntry struct {
	UserID   int64  // corresponds to id_user
	Location string // corresponds to location
	DataReg  string // corresponds to dataReg
}
