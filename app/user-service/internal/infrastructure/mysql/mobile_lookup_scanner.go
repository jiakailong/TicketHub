package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"tickethub/app/user-service/internal/application"
)

type MobileLookupScanner struct {
	db *sql.DB
}

func NewMobileLookupScanner(db *sql.DB) MobileLookupScanner {
	return MobileLookupScanner{db: db}
}

var _ application.MobileLookupScanner = MobileLookupScanner{}

func (s MobileLookupScanner) ScanMobileLookups(ctx context.Context, batchSize int, visit func(context.Context, []byte) error) error {
	if s.db == nil || visit == nil {
		return fmt.Errorf("mobile lookup scanner is not configured")
	}
	if batchSize <= 0 {
		batchSize = 1_000
	}
	var lastID int64
	for {
		rows, err := s.db.QueryContext(ctx, `
SELECT id, mobile_lookup
FROM users
WHERE id > ?
ORDER BY id
LIMIT ?`, lastID, batchSize)
		if err != nil {
			return err
		}
		count := 0
		for rows.Next() {
			var id int64
			var lookup []byte
			if err := rows.Scan(&id, &lookup); err != nil {
				_ = rows.Close()
				return err
			}
			if err := visit(ctx, lookup); err != nil {
				_ = rows.Close()
				return err
			}
			lastID = id
			count++
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if count < batchSize {
			return nil
		}
	}
}
