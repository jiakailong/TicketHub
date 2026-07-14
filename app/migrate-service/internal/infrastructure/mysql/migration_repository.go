package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"tickethub/app/migrate-service/internal/domain/migrate"
	therrors "tickethub/pkg/errors"
)

type MigrationRepository struct {
	db *sql.DB
}

func NewMigrationRepository(db *sql.DB) MigrationRepository {
	return MigrationRepository{db: db}
}

func (r MigrationRepository) SaveTask(ctx context.Context, task migrate.MigrationTask) error {
	sourceDatabase, sourceTable, err := migrate.ParseShardLocation(task.SourceShard)
	if err != nil {
		return therrors.New(therrors.CodeInvalidArgument, err.Error())
	}
	targetDatabase, targetTable, err := migrate.ParseShardLocation(task.TargetShard)
	if err != nil {
		return therrors.New(therrors.CodeInvalidArgument, err.Error())
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "begin migration task transaction failed", err)
	}
	defer tx.Rollback()
	var targetTableExists int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.tables
WHERE table_schema = ? AND table_name = ?`, targetDatabase, targetTable).Scan(&targetTableExists); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "check migration target table failed", err)
	}
	if targetTableExists == 0 {
		return therrors.New(therrors.CodeInvalidArgument, "migration target table does not exist")
	}
	var targetAssignments int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM shard_mappings
WHERE virtual_shard <> ?
  AND ((physical_db = ? AND physical_table = ?) OR (shadow_db = ? AND shadow_table = ?))`,
		task.VirtualShard, targetDatabase, targetTable, targetDatabase, targetTable,
	).Scan(&targetAssignments); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "check migration target assignment failed", err)
	}
	if targetAssignments > 0 {
		return therrors.New(therrors.CodeConflict, "target shard is already assigned")
	}
	result, err := tx.ExecContext(ctx, `
UPDATE shard_mappings
SET shadow_db = ?, shadow_table = ?, write_mode = 'DUAL_WRITE', version = version + 1, updated_at = CURRENT_TIMESTAMP(3)
WHERE virtual_shard = ? AND physical_db = ? AND physical_table = ? AND write_mode = 'PRIMARY_ONLY'`,
		targetDatabase, targetTable, task.VirtualShard, sourceDatabase, sourceTable,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "activate shard dual write failed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "read shard mapping update result failed", err)
	}
	if affected == 0 {
		return therrors.New(therrors.CodeConflict, "source shard does not match a primary-only mapping")
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO migration_tasks (id, virtual_shard, source_shard, target_shard, status, batch_size, created_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  virtual_shard = VALUES(virtual_shard),
  source_shard = VALUES(source_shard),
  target_shard = VALUES(target_shard),
  status = VALUES(status),
  batch_size = VALUES(batch_size),
  updated_at = CURRENT_TIMESTAMP(3)`,
		task.ID,
		task.VirtualShard,
		task.SourceShard,
		task.TargetShard,
		string(task.Status),
		task.BatchSize,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save migration task failed", err)
	}
	if err := tx.Commit(); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "commit migration task failed", err)
	}
	return nil
}

func (r MigrationRepository) LoadNextTask(ctx context.Context) (migrate.MigrationTask, bool, error) {
	var task migrate.MigrationTask
	var status string
	var errorMessage sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT id, virtual_shard, source_shard, target_shard, status, batch_size, cursor_order_number, copied_rows, error_message, created_at
FROM migration_tasks
WHERE status IN ('PENDING', 'RUNNING')
ORDER BY created_at
LIMIT 1`).Scan(
		&task.ID,
		&task.VirtualShard,
		&task.SourceShard,
		&task.TargetShard,
		&status,
		&task.BatchSize,
		&task.CursorOrderNumber,
		&task.CopiedRows,
		&errorMessage,
		&task.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return migrate.MigrationTask{}, false, nil
	}
	if err != nil {
		return migrate.MigrationTask{}, false, therrors.Wrap(therrors.CodeInfrastructure, "load next migration task failed", err)
	}
	task.Status = migrate.TaskStatus(status)
	task.ErrorMessage = errorMessage.String
	if _, err := r.db.ExecContext(ctx, `UPDATE migration_tasks SET status = 'RUNNING', updated_at = CURRENT_TIMESTAMP(3) WHERE id = ?`, task.ID); err != nil {
		return migrate.MigrationTask{}, false, therrors.Wrap(therrors.CodeInfrastructure, "mark migration task running failed", err)
	}
	task.Status = migrate.TaskRunning
	return task, true, nil
}

func (r MigrationRepository) CopyBatch(ctx context.Context, task migrate.MigrationTask) (int64, int64, error) {
	sourceDatabase, sourceTable, err := migrate.ParseShardLocation(task.SourceShard)
	if err != nil {
		return 0, task.CursorOrderNumber, therrors.New(therrors.CodeInvalidArgument, err.Error())
	}
	targetDatabase, targetTable, err := migrate.ParseShardLocation(task.TargetShard)
	if err != nil {
		return 0, task.CursorOrderNumber, therrors.New(therrors.CodeInvalidArgument, err.Error())
	}
	source := quotedShard(sourceDatabase, sourceTable)
	target := quotedShard(targetDatabase, targetTable)
	batchSize := task.BatchSize
	if batchSize <= 0 {
		batchSize = 500
	}
	var nextCursor int64
	var copied int64
	boundaryQuery := fmt.Sprintf(`
SELECT COALESCE(MAX(order_number), 0), COUNT(*)
FROM (
  SELECT order_number
  FROM %s
  WHERE order_number > ?
  ORDER BY order_number
  LIMIT ?
) migration_batch`, source)
	if err := r.db.QueryRowContext(ctx, boundaryQuery, task.CursorOrderNumber, batchSize).Scan(&nextCursor, &copied); err != nil {
		return 0, task.CursorOrderNumber, therrors.Wrap(therrors.CodeInfrastructure, "read migration batch boundary failed", err)
	}
	if copied == 0 {
		return 0, task.CursorOrderNumber, nil
	}
	copyQuery := fmt.Sprintf(`
INSERT INTO %s (order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at, reconciliation_status)
SELECT order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at, reconciliation_status
FROM %s
WHERE order_number > ? AND order_number <= ?
ON DUPLICATE KEY UPDATE
  program_id = VALUES(program_id),
  user_id = VALUES(user_id),
  ticket_category_id = VALUES(ticket_category_id),
  seat_ids = VALUES(seat_ids),
  ticket_user_ids = VALUES(ticket_user_ids),
  amount_cent = VALUES(amount_cent),
  status = VALUES(status),
  paid_at = VALUES(paid_at),
  canceled_at = VALUES(canceled_at),
  refunded_at = VALUES(refunded_at),
  reconciliation_status = VALUES(reconciliation_status)`, target, source)
	if _, err := r.db.ExecContext(ctx, copyQuery, task.CursorOrderNumber, nextCursor); err != nil {
		return 0, task.CursorOrderNumber, therrors.Wrap(therrors.CodeInfrastructure, "copy migration order batch failed", err)
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE migration_tasks
SET cursor_order_number = ?, copied_rows = copied_rows + ?, status = 'RUNNING', updated_at = CURRENT_TIMESTAMP(3)
WHERE id = ?`, nextCursor, copied, task.ID); err != nil {
		return 0, task.CursorOrderNumber, therrors.Wrap(therrors.CodeInfrastructure, "advance migration task cursor failed", err)
	}
	return copied, nextCursor, nil
}

func (r MigrationRepository) CompleteTask(ctx context.Context, task migrate.MigrationTask) error {
	targetDatabase, targetTable, err := migrate.ParseShardLocation(task.TargetShard)
	if err != nil {
		return therrors.New(therrors.CodeInvalidArgument, err.Error())
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "begin migration cutover failed", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
UPDATE shard_mappings
SET physical_db = shadow_db,
    physical_table = shadow_table,
    shadow_db = NULL,
    shadow_table = NULL,
    write_mode = 'PRIMARY_ONLY',
    version = version + 1,
    updated_at = CURRENT_TIMESTAMP(3)
WHERE virtual_shard = ? AND write_mode = 'DUAL_WRITE' AND shadow_db = ? AND shadow_table = ?`,
		task.VirtualShard, targetDatabase, targetTable,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "cut over shard mapping failed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "read shard cutover result failed", err)
	}
	if affected == 0 {
		return therrors.New(therrors.CodeConflict, "dual-write shard mapping not found")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE migration_tasks
SET status = 'DONE', error_message = NULL, updated_at = CURRENT_TIMESTAMP(3)
WHERE id = ?`, task.ID); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "complete migration task failed", err)
	}
	if err := tx.Commit(); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "commit migration cutover failed", err)
	}
	return nil
}

func (r MigrationRepository) FailTask(ctx context.Context, taskID int64, reason string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE migration_tasks
SET status = 'PAUSED', error_message = ?, updated_at = CURRENT_TIMESTAMP(3)
WHERE id = ?`, reason, taskID)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "fail migration task failed", err)
	}
	return nil
}

func (r MigrationRepository) ResumeTask(ctx context.Context, taskID int64) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE migration_tasks task
JOIN shard_mappings mapping ON mapping.virtual_shard = task.virtual_shard
SET task.status = 'RUNNING', task.error_message = NULL, task.updated_at = CURRENT_TIMESTAMP(3)
WHERE task.id = ?
  AND task.status IN ('PAUSED', 'FAILED')
  AND mapping.write_mode = 'DUAL_WRITE'`, taskID)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "resume migration task failed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "read resume migration result failed", err)
	}
	if affected == 0 {
		return therrors.New(therrors.CodeConflict, "migration task is not resumable")
	}
	return nil
}

func quotedShard(database string, table string) string {
	return "`" + database + "`.`" + table + "`"
}

func (r MigrationRepository) LoadShardMappings(ctx context.Context) ([]migrate.ShardMapping, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT virtual_shard, physical_db, physical_table, COALESCE(shadow_db, ''), COALESCE(shadow_table, ''), write_mode, version
FROM shard_mappings
ORDER BY virtual_shard`)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "query shard mappings failed", err)
	}
	defer rows.Close()

	var result []migrate.ShardMapping
	for rows.Next() {
		var item migrate.ShardMapping
		if err := rows.Scan(&item.VirtualShard, &item.PhysicalDB, &item.PhysicalTable, &item.ShadowDB, &item.ShadowTable, &item.WriteMode, &item.Version); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan shard mapping failed", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate shard mappings failed", err)
	}
	return result, nil
}
