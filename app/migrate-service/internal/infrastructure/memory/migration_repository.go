package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"tickethub/app/migrate-service/internal/domain/migrate"
	therrors "tickethub/pkg/errors"
)

type MigrationRepository struct {
	mu       sync.RWMutex
	tasks    map[int64]migrate.MigrationTask
	mappings []migrate.ShardMapping
}

func NewMigrationRepository() *MigrationRepository {
	return &MigrationRepository{
		tasks: make(map[int64]migrate.MigrationTask),
		mappings: []migrate.ShardMapping{
			{VirtualShard: 0, PhysicalDB: "tickethub_order_0", PhysicalTable: "orders_0", WriteMode: "PRIMARY_ONLY", Version: 1},
			{VirtualShard: 1, PhysicalDB: "tickethub_order_0", PhysicalTable: "orders_1", WriteMode: "PRIMARY_ONLY", Version: 1},
		},
	}
}

func (r *MigrationRepository) SaveTask(ctx context.Context, task migrate.MigrationTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sourceDatabase, sourceTable, _ := migrate.ParseShardLocation(task.SourceShard)
	targetDatabase, targetTable, _ := migrate.ParseShardLocation(task.TargetShard)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	for _, mapping := range r.mappings {
		if mapping.VirtualShard != task.VirtualShard && ((mapping.PhysicalDB == targetDatabase && mapping.PhysicalTable == targetTable) || (mapping.ShadowDB == targetDatabase && mapping.ShadowTable == targetTable)) {
			return therrors.New(therrors.CodeConflict, "target shard is already assigned")
		}
	}
	found := false
	for index := range r.mappings {
		mapping := &r.mappings[index]
		if mapping.VirtualShard != task.VirtualShard || mapping.PhysicalDB != sourceDatabase || mapping.PhysicalTable != sourceTable {
			continue
		}
		mapping.ShadowDB = targetDatabase
		mapping.ShadowTable = targetTable
		mapping.WriteMode = "DUAL_WRITE"
		mapping.Version++
		found = true
		break
	}
	if !found {
		return therrors.New(therrors.CodeConflict, "source shard does not match current mapping")
	}
	r.tasks[task.ID] = task
	return nil
}

func (r *MigrationRepository) LoadNextTask(ctx context.Context) (migrate.MigrationTask, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]int64, 0, len(r.tasks))
	for id, task := range r.tasks {
		if task.Status == migrate.TaskPending || task.Status == migrate.TaskRunning {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return migrate.MigrationTask{}, false, nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	task := r.tasks[ids[0]]
	task.Status = migrate.TaskRunning
	r.tasks[task.ID] = task
	return task, true, nil
}

func (r *MigrationRepository) CopyBatch(ctx context.Context, task migrate.MigrationTask) (int64, int64, error) {
	return 0, task.CursorOrderNumber, nil
}

func (r *MigrationRepository) CompleteTask(ctx context.Context, task migrate.MigrationTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range r.mappings {
		mapping := &r.mappings[index]
		if mapping.VirtualShard != task.VirtualShard || mapping.WriteMode != "DUAL_WRITE" {
			continue
		}
		mapping.PhysicalDB = mapping.ShadowDB
		mapping.PhysicalTable = mapping.ShadowTable
		mapping.ShadowDB = ""
		mapping.ShadowTable = ""
		mapping.WriteMode = "PRIMARY_ONLY"
		mapping.Version++
		stored := r.tasks[task.ID]
		stored.Status = migrate.TaskDone
		r.tasks[task.ID] = stored
		return nil
	}
	return therrors.New(therrors.CodeConflict, "dual-write mapping not found")
}

func (r *MigrationRepository) FailTask(ctx context.Context, taskID int64, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return therrors.New(therrors.CodeNotFound, "migration task not found")
	}
	task.Status = migrate.TaskPaused
	task.ErrorMessage = reason
	r.tasks[taskID] = task
	return nil
}

func (r *MigrationRepository) ResumeTask(ctx context.Context, taskID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || (task.Status != migrate.TaskPaused && task.Status != migrate.TaskFailed) {
		return therrors.New(therrors.CodeConflict, "migration task is not resumable")
	}
	task.Status = migrate.TaskRunning
	task.ErrorMessage = ""
	r.tasks[taskID] = task
	return nil
}

func (r *MigrationRepository) LoadShardMappings(ctx context.Context) ([]migrate.ShardMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]migrate.ShardMapping(nil), r.mappings...), nil
}
