package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"

	orderapp "tickethub/app/order-service/internal/application"
	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/sharding"
)

var shardIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type ShardedOrderRepository struct {
	databases sharding.DatabaseResolver
	router    sharding.OrderRouter
}

func NewShardedOrderRepository(databases sharding.DatabaseResolver, router sharding.OrderRouter) *ShardedOrderRepository {
	return &ShardedOrderRepository{databases: databases, router: router}
}

func (r *ShardedOrderRepository) Save(ctx context.Context, item order.Order) error {
	if err := r.ensureFreshMapping(); err != nil {
		return err
	}
	for _, location := range r.writeLocations(item.OrderNumber, item.UserID) {
		if err := r.saveAt(ctx, location, item); err != nil {
			return err
		}
	}
	return nil
}

func (r *ShardedOrderRepository) Update(ctx context.Context, item order.Order) error {
	if err := r.ensureFreshMapping(); err != nil {
		return err
	}
	for _, location := range r.writeLocations(item.OrderNumber, item.UserID) {
		// During dual write the shadow row may not have been copied yet. INSERT
		// IGNORE creates that row, while updateAt still enforces transitions on
		// every row that already exists.
		if err := r.insertIgnoreAt(ctx, location, item); err != nil {
			return err
		}
		if err := r.updateAt(ctx, location, item); err != nil {
			return err
		}
	}
	return nil
}

func (r *ShardedOrderRepository) FindByOrderNumber(ctx context.Context, orderNumber int64, userID int64) (order.Order, error) {
	if err := r.ensureFreshMapping(); err != nil {
		return order.Order{}, err
	}
	return r.findAt(ctx, r.router.RouteOrder(orderNumber, userID), orderNumber, userID, true)
}

func (r *ShardedOrderRepository) FindByOrderNumberSystem(ctx context.Context, orderNumber int64) (order.Order, error) {
	if err := r.ensureFreshMapping(); err != nil {
		return order.Order{}, err
	}
	return r.findAt(ctx, r.router.RouteOrder(orderNumber, 0), orderNumber, 0, false)
}

func (r *ShardedOrderRepository) ListByUserID(ctx context.Context, userID int64, limit int) ([]order.Order, error) {
	return r.ListByUserIDPage(ctx, userID, "", orderapp.OrderListCursor{}, limit)
}

func (r *ShardedOrderRepository) ListByUserIDPage(ctx context.Context, userID int64, status order.Status, before orderapp.OrderListCursor, limit int) ([]order.Order, error) {
	if err := r.ensureFreshMapping(); err != nil {
		return nil, err
	}
	return r.listByUserAt(ctx, r.router.RouteOrder(0, userID), userID, status, before, limit)
}

func (r *ShardedOrderRepository) ListInventoryUsage(ctx context.Context, programID int64) ([]orderapp.InventoryUsage, error) {
	if err := r.ensureFreshMapping(); err != nil {
		return nil, err
	}
	catalog, ok := r.router.(sharding.LocationCatalog)
	if !ok {
		return nil, therrors.New(therrors.CodeInfrastructure, "order shard router does not expose primary locations")
	}
	counts := make(map[int64]int64)
	seen := make(map[string]struct{})
	for _, location := range catalog.PrimaryLocations() {
		key := location.Database + "." + location.Table
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := r.addInventoryUsage(ctx, location, programID, counts); err != nil {
			return nil, err
		}
	}
	result := make([]orderapp.InventoryUsage, 0, len(counts))
	for categoryID, occupiedCount := range counts {
		result = append(result, orderapp.InventoryUsage{TicketCategoryID: categoryID, OccupiedCount: occupiedCount})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TicketCategoryID < result[j].TicketCategoryID })
	return result, nil
}

func (r *ShardedOrderRepository) ensureFreshMapping() error {
	if freshness, ok := r.router.(sharding.MappingFreshness); ok && !freshness.MappingFresh(sharding.DefaultMappingMaxStaleness) {
		return therrors.New(therrors.CodeInfrastructure, "order shard mapping is stale")
	}
	return nil
}

func (r *ShardedOrderRepository) saveAt(ctx context.Context, location sharding.Location, item order.Order) error {
	database, table, err := r.resolve(location)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
INSERT INTO %s (order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  order_number = VALUES(order_number)`, table)
	_, err = database.ExecContext(ctx, query,
		item.OrderNumber,
		item.ProgramID,
		item.UserID,
		nullInt64(item.TicketCategoryID),
		jsonInt64Slice(item.SeatIDs),
		jsonInt64Slice(item.TicketUserIDs),
		item.AmountCent,
		string(item.Status),
		item.CreatedAt,
		nullTime(item.PaidAt),
		nullTime(item.CanceledAt),
		nullTime(item.RefundedAt),
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save order to shard "+location.Database+"."+location.Table+" failed", err)
	}
	return nil
}

func (r *ShardedOrderRepository) insertIgnoreAt(ctx context.Context, location sharding.Location, item order.Order) error {
	database, table, err := r.resolve(location)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
INSERT IGNORE INTO %s (order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, table)
	_, err = database.ExecContext(ctx, query,
		item.OrderNumber, item.ProgramID, item.UserID, nullInt64(item.TicketCategoryID),
		jsonInt64Slice(item.SeatIDs), jsonInt64Slice(item.TicketUserIDs), item.AmountCent,
		string(item.Status), item.CreatedAt, nullTime(item.PaidAt), nullTime(item.CanceledAt), nullTime(item.RefundedAt),
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "insert order transition row at shard failed", err)
	}
	return nil
}

func (r *ShardedOrderRepository) updateAt(ctx context.Context, location sharding.Location, item order.Order) error {
	database, table, err := r.resolve(location)
	if err != nil {
		return err
	}
	previous, err := allowedPreviousStatus(item.Status)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
UPDATE %s
SET status = ?, paid_at = ?, canceled_at = ?, refunded_at = ?
WHERE order_number = ? AND user_id = ? AND status IN (?, ?)`, table)
	result, err := database.ExecContext(ctx, query,
		string(item.Status), nullTime(item.PaidAt), nullTime(item.CanceledAt), nullTime(item.RefundedAt),
		item.OrderNumber, item.UserID, string(previous), string(item.Status),
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "update order transition at shard failed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "read sharded order update result failed", err)
	}
	if affected > 0 {
		return nil
	}
	var current string
	if err := database.QueryRowContext(ctx, fmt.Sprintf(`SELECT status FROM %s WHERE order_number = ? AND user_id = ?`, table), item.OrderNumber, item.UserID).Scan(&current); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "verify sharded order update conflict failed", err)
	}
	if order.Status(current) != item.Status {
		return therrors.New(therrors.CodeOrderStateConflict, "order status changed concurrently")
	}
	return nil
}

func (r *ShardedOrderRepository) findAt(ctx context.Context, location sharding.Location, orderNumber int64, userID int64, enforceUser bool) (order.Order, error) {
	database, table, err := r.resolve(location)
	if err != nil {
		return order.Order{}, err
	}
	query := fmt.Sprintf(`
SELECT order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at
FROM %s
WHERE order_number = ?`, table)
	args := []any{orderNumber}
	if enforceUser {
		query += " AND user_id = ?"
		args = append(args, userID)
	}
	return scanOrder(database.QueryRowContext(ctx, query, args...))
}

func (r *ShardedOrderRepository) addInventoryUsage(ctx context.Context, location sharding.Location, programID int64, counts map[int64]int64) error {
	database, table, err := r.resolve(location)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
SELECT ticket_category_id,
       SUM(CASE
         WHEN COALESCE(JSON_LENGTH(seat_ids), 0) > 0 THEN JSON_LENGTH(seat_ids)
         ELSE COALESCE(JSON_LENGTH(ticket_user_ids), 0)
       END) AS occupied_count
FROM %s
WHERE program_id = ?
  AND status IN ('NO_PAY', 'PAY')
  AND ticket_category_id IS NOT NULL
GROUP BY ticket_category_id`, table)
	rows, err := database.QueryContext(ctx, query, programID)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "list inventory usage from shard "+location.Database+"."+location.Table+" failed", err)
	}
	defer rows.Close()
	for rows.Next() {
		var categoryID int64
		var occupiedCount int64
		if err := rows.Scan(&categoryID, &occupiedCount); err != nil {
			return therrors.Wrap(therrors.CodeInfrastructure, "scan sharded inventory usage failed", err)
		}
		counts[categoryID] += occupiedCount
	}
	if err := rows.Err(); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "iterate sharded inventory usage failed", err)
	}
	return nil
}

func (r *ShardedOrderRepository) listByUserAt(ctx context.Context, location sharding.Location, userID int64, status order.Status, before orderapp.OrderListCursor, limit int) ([]order.Order, error) {
	database, table, err := r.resolve(location)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
SELECT order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at
FROM %s
WHERE user_id = ?`, table)
	args := []any{userID}
	if status != "" {
		query += " AND status = ?"
		args = append(args, string(status))
	}
	if !before.CreatedAt.IsZero() {
		query += " AND (created_at < ? OR (created_at = ? AND order_number < ?))"
		args = append(args, before.CreatedAt, before.CreatedAt, before.OrderNumber)
	}
	query += " ORDER BY created_at DESC, order_number DESC LIMIT ?"
	args = append(args, limit)
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "list orders from shard "+location.Database+"."+location.Table+" failed", err)
	}
	defer rows.Close()
	result := make([]order.Order, 0)
	for rows.Next() {
		item, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate sharded orders failed", err)
	}
	return result, nil
}

func (r *ShardedOrderRepository) writeLocations(orderNumber int64, userID int64) []sharding.Location {
	if router, ok := r.router.(sharding.OrderWriteRouter); ok {
		locations := router.RouteOrderWrites(orderNumber, userID)
		if len(locations) > 0 {
			return uniqueLocations(locations)
		}
	}
	return []sharding.Location{r.router.RouteOrder(orderNumber, userID)}
}

func (r *ShardedOrderRepository) resolve(location sharding.Location) (*sql.DB, string, error) {
	if !shardIdentifierPattern.MatchString(location.Database) || !shardIdentifierPattern.MatchString(location.Table) {
		return nil, "", therrors.New(therrors.CodeInfrastructure, "invalid order shard identifier")
	}
	database, err := r.databases.Resolve(location.Database)
	if err != nil {
		return nil, "", therrors.Wrap(therrors.CodeInfrastructure, "resolve order shard database failed", err)
	}
	return database, "`" + location.Table + "`", nil
}

func uniqueLocations(locations []sharding.Location) []sharding.Location {
	result := make([]sharding.Location, 0, len(locations))
	seen := make(map[string]struct{}, len(locations))
	for _, location := range locations {
		key := location.Database + "." + location.Table
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, location)
	}
	return result
}
