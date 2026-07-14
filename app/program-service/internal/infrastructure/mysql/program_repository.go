package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
)

type ProgramRepository struct {
	db *sql.DB
}

func (r ProgramRepository) SaveProgramWithEvent(ctx context.Context, item program.Program, categories []program.TicketCategory, event mq.Event) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "begin program change failed", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO programs (id, title, city, place, show_time, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  title = VALUES(title), city = VALUES(city), place = VALUES(place),
  show_time = VALUES(show_time), status = VALUES(status), updated_at = CURRENT_TIMESTAMP(3)`,
		item.ID, item.Title, item.City, item.Place, item.ShowTime, item.Status,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save program failed", err)
	}
	for _, category := range categories {
		_, err = tx.ExecContext(ctx, `
INSERT INTO ticket_categories (id, program_id, name, price_cent, total, remain, sell_started)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  program_id = VALUES(program_id), name = VALUES(name), price_cent = VALUES(price_cent),
  total = VALUES(total), remain = VALUES(remain), sell_started = VALUES(sell_started)`,
			category.ID, item.ID, category.Name, category.PriceCent, category.Total, category.Remain, category.SellStarted,
		)
		if err != nil {
			return therrors.Wrap(therrors.CodeInfrastructure, "save program ticket category failed", err)
		}
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO program_outbox
  (event_id, topic, event_key, trace_id, schema_version, occurred_at, payload, status, available_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE event_id = VALUES(event_id)`,
		event.Header.EventID, event.Topic, event.Key,
		sql.NullString{String: event.Header.TraceID, Valid: event.Header.TraceID != ""},
		event.Header.SchemaVersion, event.Header.OccurredAt, event.Payload,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save program change outbox event failed", err)
	}
	if err := tx.Commit(); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "commit program change failed", err)
	}
	return nil
}

func (r ProgramRepository) FindProgramsByIDs(ctx context.Context, programIDs []int64) ([]program.Program, error) {
	if len(programIDs) == 0 {
		return []program.Program{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(programIDs)), ",")
	args := make([]any, len(programIDs))
	for index, id := range programIDs {
		args[index] = id
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT id, title, city, place, show_time, status
FROM programs
WHERE id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "batch query programs failed", err)
	}
	defer rows.Close()
	byID := make(map[int64]program.Program, len(programIDs))
	for rows.Next() {
		var item program.Program
		if err := rows.Scan(&item.ID, &item.Title, &item.City, &item.Place, &item.ShowTime, &item.Status); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan batch program failed", err)
		}
		byID[item.ID] = item
	}
	items := make([]program.Program, 0, len(byID))
	for _, id := range programIDs {
		if item, ok := byID[id]; ok {
			items = append(items, item)
		}
	}
	return items, rows.Err()
}

func (r ProgramRepository) MinPricesByProgramIDs(ctx context.Context, programIDs []int64) (map[int64]int64, error) {
	result := make(map[int64]int64, len(programIDs))
	if len(programIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(programIDs)), ",")
	args := make([]any, len(programIDs))
	for index, id := range programIDs {
		args[index] = id
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT program_id, MIN(price_cent)
FROM ticket_categories
WHERE program_id IN (%s) AND sell_started = 1
GROUP BY program_id`, placeholders), args...)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "batch query minimum ticket prices failed", err)
	}
	defer rows.Close()
	for rows.Next() {
		var programID, price int64
		if err := rows.Scan(&programID, &price); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan minimum ticket price failed", err)
		}
		result[programID] = price
	}
	return result, rows.Err()
}

func NewProgramRepository(db *sql.DB) ProgramRepository {
	return ProgramRepository{db: db}
}

func (r ProgramRepository) FindProgram(ctx context.Context, programID int64) (program.Program, error) {
	var item program.Program
	err := r.db.QueryRowContext(ctx, `
SELECT id, title, city, place, show_time, status
FROM programs
WHERE id = ?`, programID).Scan(
		&item.ID,
		&item.Title,
		&item.City,
		&item.Place,
		&item.ShowTime,
		&item.Status,
	)
	if err == sql.ErrNoRows {
		return program.Program{}, therrors.New(therrors.CodeNotFound, "program not found")
	}
	if err != nil {
		return program.Program{}, therrors.Wrap(therrors.CodeInfrastructure, "query program failed", err)
	}
	return item, nil
}

func (r ProgramRepository) SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	args := make([]any, 0, 4)
	where := "WHERE 1 = 1"
	if strings.TrimSpace(keyword) != "" {
		where += " AND title LIKE ?"
		args = append(args, "%"+strings.TrimSpace(keyword)+"%")
	}
	if strings.TrimSpace(city) != "" {
		where += " AND city = ?"
		args = append(args, strings.TrimSpace(city))
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
SELECT id, title, city, place, show_time, status
FROM programs
`+where+`
ORDER BY show_time
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "search programs failed", err)
	}
	defer rows.Close()

	var result []program.Program
	for rows.Next() {
		var item program.Program
		if err := rows.Scan(&item.ID, &item.Title, &item.City, &item.Place, &item.ShowTime, &item.Status); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan program failed", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate programs failed", err)
	}
	return result, nil
}

func (r ProgramRepository) ListProgramsAfterID(ctx context.Context, afterID int64, limit int) ([]program.Program, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, title, city, place, show_time, status
FROM programs
WHERE id > ?
ORDER BY id
LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "list programs for search index failed", err)
	}
	defer rows.Close()

	items := make([]program.Program, 0, limit)
	for rows.Next() {
		var item program.Program
		if err := rows.Scan(&item.ID, &item.Title, &item.City, &item.Place, &item.ShowTime, &item.Status); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan program for search index failed", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate programs for search index failed", err)
	}
	return items, nil
}

func (r ProgramRepository) ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, program_id, name, price_cent, total, remain, sell_started
FROM ticket_categories
WHERE program_id = ?
ORDER BY price_cent`, programID)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "query ticket categories failed", err)
	}
	defer rows.Close()

	var result []program.TicketCategory
	for rows.Next() {
		var item program.TicketCategory
		if err := rows.Scan(&item.ID, &item.ProgramID, &item.Name, &item.PriceCent, &item.Total, &item.Remain, &item.SellStarted); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan ticket category failed", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate ticket categories failed", err)
	}
	return result, nil
}

func (r ProgramRepository) ListTicketCategoriesAfterID(ctx context.Context, afterID int64, limit int) ([]program.TicketCategory, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, program_id, name, price_cent, total, remain, sell_started
FROM ticket_categories
WHERE id > ?
ORDER BY id
LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "list ticket categories for inventory bootstrap failed", err)
	}
	defer rows.Close()
	items := make([]program.TicketCategory, 0, limit)
	for rows.Next() {
		var item program.TicketCategory
		if err := rows.Scan(&item.ID, &item.ProgramID, &item.Name, &item.PriceCent, &item.Total, &item.Remain, &item.SellStarted); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan inventory bootstrap category failed", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate inventory bootstrap categories failed", err)
	}
	return items, nil
}

func (r ProgramRepository) UpdateTicketCategoryRemain(ctx context.Context, ticketCategoryID int64, remain int64) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE ticket_categories
SET remain = ?
WHERE id = ?`, remain, ticketCategoryID)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "update ticket category remain failed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "read ticket category update result failed", err)
	}
	if affected == 0 {
		return therrors.New(therrors.CodeNotFound, "ticket category not found")
	}
	return nil
}

func (r ProgramRepository) ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, program_id, ticket_category_id, row_code, col_code, price_cent, status
FROM seats
WHERE program_id = ? AND ticket_category_id = ?
ORDER BY row_code, col_code`, programID, ticketCategoryID)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "query seats failed", err)
	}
	defer rows.Close()

	var result []program.Seat
	for rows.Next() {
		var item program.Seat
		var status string
		if err := rows.Scan(&item.ID, &item.ProgramID, &item.TicketCategoryID, &item.RowCode, &item.ColCode, &item.PriceCent, &status); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan seat failed", err)
		}
		item.Status = program.SeatStatus(status)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate seats failed", err)
	}
	return result, nil
}
