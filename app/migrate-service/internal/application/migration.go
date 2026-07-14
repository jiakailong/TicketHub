package application

import (
	"context"
	"errors"
	"time"

	"tickethub/app/migrate-service/internal/domain/migrate"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/lock"
	"tickethub/pkg/observability"
	"tickethub/pkg/sharding"
)

type MigrationRepository interface {
	SaveTask(ctx context.Context, task migrate.MigrationTask) error
	LoadShardMappings(ctx context.Context) ([]migrate.ShardMapping, error)
	LoadNextTask(ctx context.Context) (migrate.MigrationTask, bool, error)
	CopyBatch(ctx context.Context, task migrate.MigrationTask) (copied int64, nextCursor int64, error error)
	CompleteTask(ctx context.Context, task migrate.MigrationTask) error
	FailTask(ctx context.Context, taskID int64, reason string) error
	ResumeTask(ctx context.Context, taskID int64) error
}

type MigrationWorker struct {
	repo   MigrationRepository
	locker lock.Locker
}

const DualWritePropagationDelay = sharding.DefaultMappingMaxStaleness + 5*time.Second

func NewMigrationWorker(repo MigrationRepository, locker lock.Locker) MigrationWorker {
	return MigrationWorker{repo: repo, locker: locker}
}

func (w MigrationWorker) Poll(ctx context.Context) error {
	guard, err := w.locker.Acquire(ctx, "tickethub:migrate:worker", 30*time.Second)
	if err != nil {
		if errors.Is(err, lock.ErrNotAcquired) {
			return nil
		}
		return err
	}
	defer guard.Release(ctx)

	task, found, err := w.repo.LoadNextTask(ctx)
	if err != nil || !found {
		return err
	}
	if task.CreatedAt.IsZero() || time.Since(task.CreatedAt) < DualWritePropagationDelay {
		return nil
	}
	copied, _, err := w.repo.CopyBatch(ctx, task)
	if err != nil {
		_ = w.repo.FailTask(ctx, task.ID, err.Error())
		observability.IncCounter("ticket_hub_shard_migration_batch_total", map[string]string{"result": "failed"})
		return err
	}
	if copied > 0 {
		observability.AddCounter("ticket_hub_shard_migration_rows_total", nil, float64(copied))
		return nil
	}
	if err := w.repo.CompleteTask(ctx, task); err != nil {
		_ = w.repo.FailTask(ctx, task.ID, err.Error())
		return err
	}
	observability.IncCounter("ticket_hub_shard_migration_task_total", map[string]string{"result": "completed"})
	return nil
}

type MigrationService struct {
	repo           MigrationRepository
	allowedTargets map[string]struct{}
}

func (s MigrationService) WithAllowedTargets(targets []string) MigrationService {
	s.allowedTargets = make(map[string]struct{}, len(targets))
	for _, target := range targets {
		s.allowedTargets[target] = struct{}{}
	}
	return s
}

func NewMigrationService(repo MigrationRepository) MigrationService {
	return MigrationService{repo: repo}
}

func (s MigrationService) SaveTask(ctx context.Context, task migrate.MigrationTask) error {
	if task.ID <= 0 || task.VirtualShard < 0 {
		return therrors.New(therrors.CodeInvalidArgument, "migration task id and virtual shard are required")
	}
	if _, _, err := migrate.ParseShardLocation(task.SourceShard); err != nil {
		return therrors.New(therrors.CodeInvalidArgument, err.Error())
	}
	if _, _, err := migrate.ParseShardLocation(task.TargetShard); err != nil {
		return therrors.New(therrors.CodeInvalidArgument, err.Error())
	}
	if task.SourceShard == task.TargetShard {
		return therrors.New(therrors.CodeInvalidArgument, "source and target shards must differ")
	}
	if len(s.allowedTargets) > 0 {
		if _, ok := s.allowedTargets[task.TargetShard]; !ok {
			return therrors.New(therrors.CodeInvalidArgument, "target shard is not configured")
		}
	}
	if task.BatchSize <= 0 {
		task.BatchSize = 500
	}
	if task.Status == "" {
		task.Status = migrate.TaskPending
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	return s.repo.SaveTask(ctx, task)
}

func (s MigrationService) LoadShardMappings(ctx context.Context) ([]migrate.ShardMapping, error) {
	return s.repo.LoadShardMappings(ctx)
}

func (s MigrationService) ResumeTask(ctx context.Context, taskID int64) error {
	if taskID <= 0 {
		return therrors.New(therrors.CodeInvalidArgument, "task_id is required")
	}
	return s.repo.ResumeTask(ctx, taskID)
}
