package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"tickethub/pkg/privacy"
)

type PrivacyMigrationResult struct {
	Users       int64
	TicketUsers int64
}

type LegacyPrivacyMigrator struct {
	db        *sql.DB
	protector *privacy.Protector
}

func NewLegacyPrivacyMigrator(db *sql.DB, protector *privacy.Protector) LegacyPrivacyMigrator {
	return LegacyPrivacyMigrator{db: db, protector: protector}
}

func (m LegacyPrivacyMigrator) Run(ctx context.Context) (PrivacyMigrationResult, error) {
	legacyUsers, err := m.columnExists(ctx, "users", "mobile")
	if err != nil {
		return PrivacyMigrationResult{}, err
	}
	legacyTicketUsers, err := m.columnExists(ctx, "ticket_users", "certificate_no")
	if err != nil {
		return PrivacyMigrationResult{}, err
	}
	result := PrivacyMigrationResult{}
	if legacyUsers {
		result.Users, err = m.migrateUsers(ctx)
		if err != nil {
			return result, err
		}
	}
	if legacyTicketUsers {
		result.TicketUsers, err = m.migrateTicketUsers(ctx)
		if err != nil {
			return result, err
		}
	}
	if err := m.verify(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func (m LegacyPrivacyMigrator) migrateUsers(ctx context.Context) (int64, error) {
	type legacyUser struct {
		id     int64
		mobile string
		email  string
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id, mobile, COALESCE(email, '') FROM users WHERE mobile_ciphertext IS NULL`)
	if err != nil {
		return 0, err
	}
	var items []legacyUser
	for rows.Next() {
		var item legacyUser
		if err := rows.Scan(&item.id, &item.mobile, &item.email); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, item := range items {
		mobile := privacy.NormalizeMobile(item.mobile)
		email := privacy.NormalizeEmail(item.email)
		mobileCiphertext, version, err := m.protector.Encrypt(mobile, privateAAD("users", "mobile", item.id))
		if err != nil {
			return 0, err
		}
		emailCiphertext, _, err := m.protector.Encrypt(email, privateAAD("users", "email", item.id))
		if err != nil {
			return 0, err
		}
		if _, err := m.db.ExecContext(ctx, `UPDATE users SET mobile_ciphertext=?, mobile_lookup=?, email_ciphertext=?, email_lookup=?, privacy_key_version=? WHERE id=?`,
			mobileCiphertext, m.protector.Lookup(mobile), nullBytes(emailCiphertext), nullBytes(lookupOptional(m.protector, email)), version, item.id); err != nil {
			return 0, err
		}
	}
	return int64(len(items)), nil
}

func (m LegacyPrivacyMigrator) migrateTicketUsers(ctx context.Context) (int64, error) {
	type legacyTicketUser struct {
		id          int64
		name        string
		certificate string
		mobile      string
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id, name, certificate_no, COALESCE(mobile, '') FROM ticket_users WHERE certificate_ciphertext IS NULL`)
	if err != nil {
		return 0, err
	}
	var items []legacyTicketUser
	for rows.Next() {
		var item legacyTicketUser
		if err := rows.Scan(&item.id, &item.name, &item.certificate, &item.mobile); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, item := range items {
		name := privacy.NormalizeName(item.name)
		certificate := privacy.NormalizeCertificate(item.certificate)
		mobile := privacy.NormalizeMobile(item.mobile)
		nameCiphertext, version, err := m.protector.Encrypt(name, privateAAD("ticket_users", "name", item.id))
		if err != nil {
			return 0, err
		}
		certificateCiphertext, _, err := m.protector.Encrypt(certificate, privateAAD("ticket_users", "certificate", item.id))
		if err != nil {
			return 0, err
		}
		mobileCiphertext, _, err := m.protector.Encrypt(mobile, privateAAD("ticket_users", "mobile", item.id))
		if err != nil {
			return 0, err
		}
		if _, err := m.db.ExecContext(ctx, `UPDATE ticket_users SET name_ciphertext=?, certificate_ciphertext=?, certificate_lookup=?, mobile_ciphertext=?, privacy_key_version=? WHERE id=?`,
			nameCiphertext, certificateCiphertext, m.protector.Lookup(certificate), nullBytes(mobileCiphertext), version, item.id); err != nil {
			return 0, err
		}
	}
	return int64(len(items)), nil
}

func (m LegacyPrivacyMigrator) verify(ctx context.Context) error {
	checks := []struct {
		table string
		where string
	}{
		{"users", "mobile_ciphertext IS NULL OR mobile_lookup IS NULL OR privacy_key_version IS NULL"},
		{"ticket_users", "name_ciphertext IS NULL OR certificate_ciphertext IS NULL OR certificate_lookup IS NULL OR privacy_key_version IS NULL"},
	}
	for _, check := range checks {
		var count int64
		if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+check.table+" WHERE "+check.where).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("privacy migration incomplete: %s has %d unencrypted rows", check.table, count)
		}
	}
	return nil
}

func (m LegacyPrivacyMigrator) columnExists(ctx context.Context, table string, column string) (bool, error) {
	var count int
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, column).Scan(&count)
	return count > 0, err
}
