package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	mysqldriver "github.com/go-sql-driver/mysql"

	"tickethub/app/user-service/internal/domain/user"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/privacy"
)

type UserRepository struct {
	db        *sql.DB
	protector *privacy.Protector
}

func NewUserRepository(db *sql.DB, protector *privacy.Protector) UserRepository {
	return UserRepository{db: db, protector: protector}
}

func (r UserRepository) Save(ctx context.Context, item user.User) error {
	mobile := privacy.NormalizeMobile(item.Mobile)
	email := privacy.NormalizeEmail(item.Email)
	mobileCiphertext, version, err := r.protector.Encrypt(mobile, privateAAD("users", "mobile", item.ID))
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "encrypt user mobile failed", err)
	}
	emailCiphertext, _, err := r.protector.Encrypt(email, privateAAD("users", "email", item.ID))
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "encrypt user email failed", err)
	}
	_, err = r.db.ExecContext(ctx, `
	INSERT INTO users (id, mobile_ciphertext, mobile_lookup, password_hash, email_ciphertext, email_lookup, privacy_key_version, real_name_status, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID,
		mobileCiphertext,
		r.protector.Lookup(mobile),
		item.PasswordHash,
		nullBytes(emailCiphertext),
		nullBytes(lookupOptional(r.protector, email)),
		version,
		string(item.RealNameStatus),
		item.CreatedAt,
	)
	if err != nil {
		var mysqlErr *mysqldriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return therrors.New(therrors.CodeConflict, "mobile already registered")
		}
		return therrors.Wrap(therrors.CodeInfrastructure, "save user failed", err)
	}
	return nil
}

func (r UserRepository) FindByID(ctx context.Context, id int64) (user.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx, `
	SELECT id, mobile_ciphertext, password_hash, email_ciphertext, privacy_key_version, real_name_status, created_at
	FROM users
	WHERE id = ?`, id))
}

func (r UserRepository) FindByMobile(ctx context.Context, mobile string) (user.User, error) {
	mobile = privacy.NormalizeMobile(mobile)
	return r.scanUser(r.db.QueryRowContext(ctx, `
	SELECT id, mobile_ciphertext, password_hash, email_ciphertext, privacy_key_version, real_name_status, created_at
	FROM users
	WHERE mobile_lookup = ?`, r.protector.Lookup(mobile)))
}

func (r UserRepository) scanUser(row *sql.Row) (user.User, error) {
	var item user.User
	var mobileCiphertext []byte
	var emailCiphertext []byte
	var version string
	var status string
	err := row.Scan(
		&item.ID,
		&mobileCiphertext,
		&item.PasswordHash,
		&emailCiphertext,
		&version,
		&status,
		&item.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return user.User{}, therrors.New(therrors.CodeNotFound, "user not found")
	}
	if err != nil {
		return user.User{}, therrors.Wrap(therrors.CodeInfrastructure, "query user failed", err)
	}
	item.Mobile, err = r.protector.Decrypt(mobileCiphertext, version, privateAAD("users", "mobile", item.ID))
	if err != nil {
		return user.User{}, therrors.Wrap(therrors.CodeInfrastructure, "decrypt user mobile failed", err)
	}
	item.Email, err = r.protector.Decrypt(emailCiphertext, version, privateAAD("users", "email", item.ID))
	if err != nil {
		return user.User{}, therrors.Wrap(therrors.CodeInfrastructure, "decrypt user email failed", err)
	}
	item.RealNameStatus = user.RealNameStatus(status)
	return item, nil
}

func privateAAD(table string, field string, id int64) []byte {
	return []byte(fmt.Sprintf("%s:%s:%d", table, field, id))
}

func lookupOptional(protector *privacy.Protector, value string) []byte {
	if value == "" {
		return nil
	}
	return protector.Lookup(value)
}

func nullBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
