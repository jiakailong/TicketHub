package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"tickethub/app/migrate-service/internal/domain/migrate"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/lock"
)

func TestMigrationServiceValidatesAndDefaultsTask(t *testing.T) {
	repository := &fakeMigrationRepository{}
	service := NewMigrationService(repository)
	task := migrate.MigrationTask{
		ID: 1, VirtualShard: 0,
		SourceShard: "tickethub_order_0.orders_0",
		TargetShard: "tickethub_order_2.orders_0",
	}

	if err := service.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if repository.saved.BatchSize != 500 || repository.saved.Status != migrate.TaskPending || repository.saved.CreatedAt.IsZero() {
		t.Fatalf("saved task = %+v", repository.saved)
	}

	task.TargetShard = task.SourceShard
	if err := service.SaveTask(context.Background(), task); !therrors.IsCode(err, therrors.CodeInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestMigrationServiceRejectsUnconfiguredTarget(t *testing.T) {
	service := NewMigrationService(&fakeMigrationRepository{}).WithAllowedTargets([]string{"tickethub_order_2.orders_0"})
	err := service.SaveTask(context.Background(), migrate.MigrationTask{
		ID: 1, VirtualShard: 0,
		SourceShard: "tickethub_order_0.orders_0",
		TargetShard: "tickethub_order_9.orders_0",
	})
	if !therrors.IsCode(err, therrors.CodeInvalidArgument) {
		t.Fatalf("expected invalid target, got %v", err)
	}
}

func TestMigrationWorkerCopiesThenCutsOver(t *testing.T) {
	repository := &fakeMigrationRepository{
		nextTask:    migrate.MigrationTask{ID: 1, CreatedAt: time.Now().Add(-DualWritePropagationDelay - time.Second)},
		copyResults: []int64{2, 0},
	}
	worker := NewMigrationWorker(repository, lock.NewMemoryLocker())

	if err := worker.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.completed {
		t.Fatal("task must not cut over while a batch was copied")
	}
	if err := worker.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !repository.completed {
		t.Fatal("expected cutover after source is exhausted")
	}
}

func TestMigrationWorkerMarksCopyFailure(t *testing.T) {
	repository := &fakeMigrationRepository{
		nextTask: migrate.MigrationTask{ID: 9, CreatedAt: time.Now().Add(-DualWritePropagationDelay - time.Second)},
		copyErr:  errors.New("copy failed"),
	}
	worker := NewMigrationWorker(repository, lock.NewMemoryLocker())

	if err := worker.Poll(context.Background()); err == nil {
		t.Fatal("expected copy error")
	}
	if repository.failedTaskID != 9 {
		t.Fatalf("failed task id = %d", repository.failedTaskID)
	}
}

type fakeMigrationRepository struct {
	saved        migrate.MigrationTask
	nextTask     migrate.MigrationTask
	copyResults  []int64
	copyErr      error
	completed    bool
	failedTaskID int64
}

func (r *fakeMigrationRepository) SaveTask(ctx context.Context, task migrate.MigrationTask) error {
	r.saved = task
	return nil
}

func (r *fakeMigrationRepository) LoadShardMappings(ctx context.Context) ([]migrate.ShardMapping, error) {
	return nil, nil
}

func (r *fakeMigrationRepository) LoadNextTask(ctx context.Context) (migrate.MigrationTask, bool, error) {
	return r.nextTask, r.nextTask.ID > 0, nil
}

func (r *fakeMigrationRepository) CopyBatch(ctx context.Context, task migrate.MigrationTask) (int64, int64, error) {
	if r.copyErr != nil {
		return 0, task.CursorOrderNumber, r.copyErr
	}
	if len(r.copyResults) == 0 {
		return 0, task.CursorOrderNumber, nil
	}
	result := r.copyResults[0]
	r.copyResults = r.copyResults[1:]
	return result, task.CursorOrderNumber + result, nil
}

func (r *fakeMigrationRepository) CompleteTask(ctx context.Context, task migrate.MigrationTask) error {
	r.completed = true
	return nil
}

func (r *fakeMigrationRepository) FailTask(ctx context.Context, taskID int64, reason string) error {
	r.failedTaskID = taskID
	return nil
}

func (r *fakeMigrationRepository) ResumeTask(ctx context.Context, taskID int64) error {
	return nil
}
