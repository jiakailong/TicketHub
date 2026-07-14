package mysql

import (
	"context"
	"database/sql"

	"tickethub/app/user-service/internal/domain/user"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/privacy"
)

type TicketUserRepository struct {
	db        *sql.DB
	protector *privacy.Protector
}

func NewTicketUserRepository(db *sql.DB, protector *privacy.Protector) TicketUserRepository {
	return TicketUserRepository{db: db, protector: protector}
}

func (r TicketUserRepository) Save(ctx context.Context, item user.TicketUser) error {
	name := privacy.NormalizeName(item.Name)
	certificate := privacy.NormalizeCertificate(item.CertificateNo)
	mobile := privacy.NormalizeMobile(item.Mobile)
	nameCiphertext, version, err := r.protector.Encrypt(name, privateAAD("ticket_users", "name", item.ID))
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "encrypt ticket user name failed", err)
	}
	certificateCiphertext, _, err := r.protector.Encrypt(certificate, privateAAD("ticket_users", "certificate", item.ID))
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "encrypt ticket user certificate failed", err)
	}
	mobileCiphertext, _, err := r.protector.Encrypt(mobile, privateAAD("ticket_users", "mobile", item.ID))
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "encrypt ticket user mobile failed", err)
	}
	_, err = r.db.ExecContext(ctx, `
	INSERT INTO ticket_users (id, user_id, name_ciphertext, certificate_ciphertext, certificate_lookup, mobile_ciphertext, privacy_key_version, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(3))`,
		item.ID,
		item.UserID,
		nameCiphertext,
		certificateCiphertext,
		r.protector.Lookup(certificate),
		nullBytes(mobileCiphertext),
		version,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save ticket user failed", err)
	}
	return nil
}

func (r TicketUserRepository) ListByUserID(ctx context.Context, userID int64) ([]user.TicketUser, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT id, user_id, name_ciphertext, certificate_ciphertext, mobile_ciphertext, privacy_key_version
	FROM ticket_users
	WHERE user_id = ?
	ORDER BY id DESC`, userID)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "query ticket users failed", err)
	}
	defer rows.Close()

	var result []user.TicketUser
	for rows.Next() {
		var item user.TicketUser
		var nameCiphertext []byte
		var certificateCiphertext []byte
		var mobileCiphertext []byte
		var version string
		if err := rows.Scan(&item.ID, &item.UserID, &nameCiphertext, &certificateCiphertext, &mobileCiphertext, &version); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan ticket user failed", err)
		}
		item.Name, err = r.protector.Decrypt(nameCiphertext, version, privateAAD("ticket_users", "name", item.ID))
		if err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "decrypt ticket user name failed", err)
		}
		item.CertificateNo, err = r.protector.Decrypt(certificateCiphertext, version, privateAAD("ticket_users", "certificate", item.ID))
		if err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "decrypt ticket user certificate failed", err)
		}
		item.Mobile, err = r.protector.Decrypt(mobileCiphertext, version, privateAAD("ticket_users", "mobile", item.ID))
		if err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "decrypt ticket user mobile failed", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate ticket users failed", err)
	}
	return result, nil
}
