package mysql

import (
	"context"
	"database/sql"

	"tickethub/app/base-data-service/internal/domain/base"
	therrors "tickethub/pkg/errors"
)

type BaseRepository struct {
	db *sql.DB
}

func NewBaseRepository(db *sql.DB) BaseRepository {
	return BaseRepository{db: db}
}

func (r BaseRepository) ListAreas(ctx context.Context, parentID int64) ([]base.Area, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, parent_id, name, level, hot
FROM areas
WHERE parent_id = ?
ORDER BY hot DESC, id`, parentID)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "query areas failed", err)
	}
	defer rows.Close()

	var result []base.Area
	for rows.Next() {
		var item base.Area
		if err := rows.Scan(&item.ID, &item.ParentID, &item.Name, &item.Level, &item.Hot); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan area failed", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate areas failed", err)
	}
	return result, nil
}

func (r BaseRepository) ListChannelData(ctx context.Context) ([]base.ChannelData, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, code, value
FROM channel_data
ORDER BY id`)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "query channel data failed", err)
	}
	defer rows.Close()

	var result []base.ChannelData
	for rows.Next() {
		var item base.ChannelData
		if err := rows.Scan(&item.ID, &item.Code, &item.Value); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan channel data failed", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate channel data failed", err)
	}
	return result, nil
}
