// ── internal/repository/user-repository.go ───────────────────────────────────
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"meily/internal/domain"
	"time"
)

// UserRepository работает со всеми таблицами: just, client, loto, geo.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository создаёт новый UserRepository.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

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

// IsClientPaid возвращает true, если в последней записи клиента флаг checks = 1.
func (r *UserRepository) IsClientPaid(ctx context.Context, userID int64) (bool, error) {
	const q = `
		SELECT checks 
		FROM client 
		WHERE id_user = ? 
		ORDER BY id DESC 
		LIMIT 1;
	`
	var paid bool
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&paid)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("no client record for user %d", userID)
	}
	return paid, err
}

// GetClientByUserID возвращает данные клиента по id_user.
func (r *UserRepository) GetClientByUserID(ctx context.Context, userID int64) (*domain.ClientEntry, error) {
	const q = `
		SELECT id_user, userName, fio, contact, address, dateRegister, dataPay, checks
		FROM client 
		WHERE id_user = ? 
		ORDER BY id DESC 
		LIMIT 1;
	`

	var client domain.ClientEntry
	err := r.db.QueryRowContext(ctx, q, userID).Scan(
		&client.UserID,
		&client.UserName,
		&client.Fio,
		&client.Contact,
		&client.Address,
		&client.DateRegister,
		&client.DatePay,
		&client.Checks,
	)

	if err != nil {
		return nil, err
	}

	return &client, nil
}

// UpdateClientDeliveryData обновляет данные доставки клиента (ФИО, адрес, координаты) и устанавливает checks = true.
func (r *UserRepository) UpdateClientDeliveryData(ctx context.Context, userID int64, fio, address string, latitude, longitude float64) error {
	const q = `
		UPDATE client 
		SET fio = ?, 
		    address = ?, 
		    dateRegister = ?,
		    checks = true,
		    latitude = ?,
		    longitude = ?
		WHERE id_user = ?;
	`

	currentTime := time.Now().Format("2006-01-02 15:04:05")

	result, err := r.db.ExecContext(ctx, q, fio, address, currentTime, latitude, longitude, userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no client record found for user %d", userID)
	}

	return nil
}

// UpdateClientChecksStatus обновляет статус checks для клиента.
func (r *UserRepository) UpdateClientChecksStatus(ctx context.Context, userID int64, checks bool) error {
	const q = `
		UPDATE client 
		SET checks = ? 
		WHERE id_user = ?;
	`

	result, err := r.db.ExecContext(ctx, q, checks, userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no client record found for user %d", userID)
	}

	return nil
}

// GetAllClientsWithDeliveryData возвращает всех клиентов с заполненными данными доставки.
func (r *UserRepository) GetAllClientsWithDeliveryData(ctx context.Context) ([]domain.ClientEntry, error) {
	const q = `
		SELECT id_user, userName, fio, contact, address, dateRegister, dataPay, checks
		FROM client 
		WHERE fio IS NOT NULL AND fio != '' 
		  AND address IS NOT NULL AND address != ''
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
			&client.UserID,
			&client.UserName,
			&client.Fio,
			&client.Contact,
			&client.Address,
			&client.DateRegister,
			&client.DatePay,
			&client.Checks,
		)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return clients, nil
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
