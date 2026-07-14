package httpapi

import (
	"net/http"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/app/admin-service/internal/application"
	"tickethub/pkg/httpx"
)

type Handler struct {
	query   application.DashboardQuery
	command application.ReconciliationCommand
}

func NewHandler(query application.DashboardQuery, command application.ReconciliationCommand) Handler {
	return Handler{query: query, command: command}
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/v1/admin/dashboard", h.dashboard)
	server.HandleFunc("/v1/admin/reconciliation/run", h.reconcile)
}

func (h Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	stats, err := h.query.Dashboard(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, stats)
}

func (h Handler) reconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProgramID       int64 `json:"program_id"`
		RepairInventory bool  `json:"repair_inventory"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	result, err := h.command.Run(r.Context(), req.ProgramID, req.RepairInventory)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	stats, err := h.query.Dashboard(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"triggered":                     true,
		"program_id":                    result.ProgramID,
		"mismatch_count":                result.MismatchCount,
		"record_mismatch_count":         result.RecordMismatchCount,
		"inventory_mismatch_count":      result.InventoryMismatchCount,
		"repaired_inventory_count":      result.RepairedInventoryCount,
		"inventory_differences":         result.InventoryDifferences,
		"dashboard_mismatch_count":      stats.ReconciliationMismatchCount,
		"discard_order_count":           stats.DiscardOrderCount,
		"reconciliation_command_source": "order-service",
	})
}
