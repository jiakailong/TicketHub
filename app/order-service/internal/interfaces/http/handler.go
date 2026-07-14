package httpapi

import (
	"net/http"
	"strconv"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/app/order-service/internal/application"
	"tickethub/pkg/auth"
	"tickethub/pkg/httpx"
)

type Handler struct {
	orders         application.OrderCommandService
	queries        *application.OrderQueryService
	compensation   *application.DiscardOrderCompensationService
	reconciliation *application.ReconciliationService
}

func NewHandler(orders application.OrderCommandService, queries application.OrderQueryService, compensation application.DiscardOrderCompensationService, reconciliation application.ReconciliationService) Handler {
	h := Handler{orders: orders}
	h.queries = &queries
	h.compensation = &compensation
	h.reconciliation = &reconciliation
	return h
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/v1/orders/detail", h.detail)
	server.HandleFunc("/v1/orders/cancel", h.cancel)
	server.HandleFunc("/v1/orders/discard-orders", h.listDiscardOrders)
	server.HandleFunc("/v1/orders/discard-orders/retry", h.retryDiscardOrders)
	server.HandleFunc("/v1/orders/reconciliation/program", h.reconcileProgram)
}

func (h Handler) detail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.queries == nil {
		httpx.WriteError(w, httpx.InvalidArgument("order query service is not configured"))
		return
	}
	orderNumber, err := strconv.ParseInt(r.URL.Query().Get("order_number"), 10, 64)
	if err != nil {
		httpx.WriteError(w, httpx.InvalidArgument("invalid query: order_number"))
		return
	}
	userID, err := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	if raw := r.Header.Get(auth.UserIDHeader); raw != "" {
		userID, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil {
		httpx.WriteError(w, httpx.InvalidArgument("invalid query: user_id"))
		return
	}
	current, err := h.queries.Get(r.Context(), orderNumber, userID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, current)
}

func (h Handler) cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		OrderNumber int64 `json:"order_number"`
		UserID      int64 `json:"user_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if raw := r.Header.Get(auth.UserIDHeader); raw != "" {
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			httpx.WriteError(w, httpx.Unauthenticated("invalid authenticated user"))
			return
		}
		req.UserID = userID
	}
	if err := h.orders.Cancel(r.Context(), req.OrderNumber, req.UserID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"order_number": req.OrderNumber})
}

func (h Handler) listDiscardOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.compensation == nil {
		httpx.WriteError(w, httpx.InvalidArgument("discard order compensation service is not configured"))
		return
	}
	programID, _ := strconv.ParseInt(r.URL.Query().Get("program_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.compensation.ListPending(r.Context(), programID, limit)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"discard_orders": items})
}

func (h Handler) retryDiscardOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.compensation == nil {
		httpx.WriteError(w, httpx.InvalidArgument("discard order compensation service is not configured"))
		return
	}
	var req struct {
		ID        int64 `json:"id"`
		ProgramID int64 `json:"program_id"`
		Limit     int   `json:"limit"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	var (
		result application.CompensationResult
		err    error
	)
	if req.ID > 0 {
		result, err = h.compensation.RetryByID(r.Context(), req.ID)
	} else {
		result, err = h.compensation.RetryProgram(r.Context(), req.ProgramID, req.Limit)
	}
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, result)
}

func (h Handler) reconcileProgram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.reconciliation == nil {
		httpx.WriteError(w, httpx.InvalidArgument("reconciliation service is not configured"))
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
	result, err := h.reconciliation.ReconcileProgram(r.Context(), req.ProgramID, req.RepairInventory)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, result)
}
