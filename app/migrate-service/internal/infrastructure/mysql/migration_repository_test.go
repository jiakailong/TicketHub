package mysql

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"tickethub/app/migrate-service/internal/domain/migrate"
)

func TestMigrationRepositorySaveTaskActivatesDualWriteAtomically(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := NewMigrationRepository(database)
	task := migrate.MigrationTask{
		ID: 1, VirtualShard: 0,
		SourceShard: "tickethub_order_0.orders_0",
		TargetShard: "tickethub_order_2.orders_0",
		Status:      migrate.TaskPending, BatchSize: 500,
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\).*FROM information_schema.tables").
		WithArgs("tickethub_order_2", "orders_0").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\).*FROM shard_mappings").
		WithArgs(task.VirtualShard, "tickethub_order_2", "orders_0", "tickethub_order_2", "orders_0").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("UPDATE shard_mappings.*write_mode = 'DUAL_WRITE'").
		WithArgs("tickethub_order_2", "orders_0", task.VirtualShard, "tickethub_order_0", "orders_0").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO migration_tasks").
		WithArgs(task.ID, task.VirtualShard, task.SourceShard, task.TargetShard, string(task.Status), task.BatchSize).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRepositoryCopiesBatchAndAdvancesCursor(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := NewMigrationRepository(database)
	task := migrate.MigrationTask{
		ID: 1, SourceShard: "tickethub_order_0.orders_0", TargetShard: "tickethub_order_2.orders_0",
		BatchSize: 2, CursorOrderNumber: 10,
	}

	mock.ExpectQuery(regexp.QuoteMeta("FROM `tickethub_order_0`.`orders_0`")).
		WithArgs(task.CursorOrderNumber, task.BatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"max", "count"}).AddRow(20, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `tickethub_order_2`.`orders_0`")).
		WithArgs(task.CursorOrderNumber, int64(20)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE migration_tasks.*cursor_order_number").
		WithArgs(int64(20), int64(2), task.ID).WillReturnResult(sqlmock.NewResult(0, 1))

	copied, cursor, err := repository.CopyBatch(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 2 || cursor != 20 {
		t.Fatalf("copied=%d cursor=%d", copied, cursor)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRepositoryCompletesCutoverAtomically(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := NewMigrationRepository(database)
	task := migrate.MigrationTask{ID: 1, VirtualShard: 0, TargetShard: "tickethub_order_2.orders_0"}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE shard_mappings.*physical_db = shadow_db").
		WithArgs(task.VirtualShard, "tickethub_order_2", "orders_0").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE migration_tasks.*status = 'DONE'").
		WithArgs(task.ID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repository.CompleteTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRepositoryResumesPausedTask(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := NewMigrationRepository(database)

	mock.ExpectExec("UPDATE migration_tasks task.*status = 'RUNNING'").
		WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repository.ResumeTask(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
