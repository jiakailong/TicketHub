package httpapi

import (
	"net/http"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/app/migrate-service/internal/application"
	"tickethub/app/migrate-service/internal/domain/migrate"
	"tickethub/pkg/httpx"
)

type IDGenerator interface {
	NextID() (int64, error)
}

type Handler struct {
	migrations application.MigrationService
	ids        IDGenerator
}

func NewHandler(migrations application.MigrationService, ids IDGenerator) Handler {
	return Handler{migrations: migrations, ids: ids}
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/v1/migrate/tasks", h.tasks)
	server.HandleFunc("/v1/migrate/tasks/resume", h.resumeTask)
	server.HandleFunc("/v1/migrate/shard-mappings", h.mappings)
}

func (h Handler) resumeTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TaskID int64 `json:"task_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.migrations.ResumeTask(r.Context(), req.TaskID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"task_id": req.TaskID, "status": migrate.TaskRunning})
}

func (h Handler) tasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		VirtualShard int    `json:"virtual_shard"`
		SourceShard  string `json:"source_shard"`
		TargetShard  string `json:"target_shard"`
		BatchSize    int    `json:"batch_size"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	id, err := h.ids.NextID()
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	task := migrate.MigrationTask{
		ID:           id,
		VirtualShard: req.VirtualShard,
		SourceShard:  req.SourceShard,
		TargetShard:  req.TargetShard,
		Status:       migrate.TaskPending,
		BatchSize:    req.BatchSize,
	}
	if task.BatchSize <= 0 {
		task.BatchSize = 500
	}
	if err := h.migrations.SaveTask(r.Context(), task); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, task)
}

func (h Handler) mappings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mappings, err := h.migrations.LoadShardMappings(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"shard_mappings": mappings})
}
