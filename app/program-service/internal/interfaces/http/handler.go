package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/app/program-service/internal/application"
	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/auth"
	"tickethub/pkg/httpx"
)

type Handler struct {
	createOrders application.CreateOrderUsecase
	queries      *application.ProgramQueryService
	admin        *application.ProgramAdminService
}

func (h Handler) WithAdmin(admin application.ProgramAdminService) Handler {
	h.admin = &admin
	return h
}

func NewHandler(createOrders application.CreateOrderUsecase, queries ...application.ProgramQueryService) Handler {
	h := Handler{createOrders: createOrders}
	if len(queries) > 0 {
		h.queries = &queries[0]
	}
	return h
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/v1/program/orders", h.createOrder)
	server.HandleFunc("/v1/programs/search", h.search)
	server.HandleFunc("/v1/programs/detail", h.detail)
	server.HandleFunc("/v1/programs/suggest", h.suggest)
	server.HandleFunc("/v1/admin/programs", h.adminPrograms)
	server.HandleFunc("/v1/admin/programs/status", h.adminProgramStatus)
}

func (h Handler) suggest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.queries == nil {
		httpx.WriteError(w, httpx.InvalidArgument("program query service is not configured"))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.queries.Suggest(r.Context(), r.URL.Query().Get("prefix"), limit)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"suggestions": items})
}

func (h Handler) adminPrograms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.admin == nil || strings.ToLower(r.Header.Get(auth.RoleHeader)) != "admin" {
		httpx.WriteError(w, httpx.Forbidden("admin role is required"))
		return
	}
	var req struct {
		ID         int64  `json:"id"`
		Title      string `json:"title"`
		City       string `json:"city"`
		Place      string `json:"place"`
		ShowTime   string `json:"show_time"`
		Categories []struct {
			ID        int64  `json:"id"`
			Name      string `json:"name"`
			PriceCent int64  `json:"price_cent"`
			Total     int64  `json:"total"`
		} `json:"ticket_categories"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	showTime, err := time.Parse(time.RFC3339, req.ShowTime)
	if err != nil {
		httpx.WriteError(w, httpx.InvalidArgument("show_time must use RFC3339"))
		return
	}
	categories := make([]program.TicketCategory, 0, len(req.Categories))
	for _, item := range req.Categories {
		categories = append(categories, program.TicketCategory{ID: item.ID, ProgramID: req.ID, Name: item.Name, PriceCent: item.PriceCent, Total: item.Total})
	}
	if err := h.admin.SaveDraft(r.Context(), program.Program{ID: req.ID, Title: req.Title, City: req.City, Place: req.Place, ShowTime: showTime}, categories); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"program_id": strconv.FormatInt(req.ID, 10), "status": "DRAFT"})
}

func (h Handler) adminProgramStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.admin == nil || strings.ToLower(r.Header.Get(auth.RoleHeader)) != "admin" {
		httpx.WriteError(w, httpx.Forbidden("admin role is required"))
		return
	}
	var req struct {
		ProgramID int64  `json:"program_id"`
		Status    string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.admin.ChangeStatus(r.Context(), req.ProgramID, req.Status); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"program_id": strconv.FormatInt(req.ProgramID, 10), "status": strings.ToUpper(req.Status)})
}

func (h Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		UserID           int64   `json:"user_id"`
		ProgramID        int64   `json:"program_id"`
		TicketCategoryID int64   `json:"ticket_category_id"`
		SeatIDs          []int64 `json:"seat_ids"`
		TicketUserIDs    []int64 `json:"ticket_user_ids"`
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
	result, err := h.createOrders.CreateAsync(r.Context(), program.CreateOrderCommand{
		RequestID:        r.Header.Get("Idempotency-Key"),
		UserID:           req.UserID,
		ProgramID:        req.ProgramID,
		TicketCategoryID: req.TicketCategoryID,
		SeatIDs:          req.SeatIDs,
		TicketUserIDs:    req.TicketUserIDs,
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, result)
}

func (h Handler) search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.queries == nil {
		httpx.WriteError(w, httpx.InvalidArgument("program query service is not configured"))
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	result, err := h.queries.SearchPage(r.Context(), r.URL.Query().Get("keyword"), r.URL.Query().Get("city"), r.URL.Query().Get("cursor"), page, pageSize)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"programs": result.Items, "next_cursor": result.NextCursor})
}

func (h Handler) detail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.queries == nil {
		httpx.WriteError(w, httpx.InvalidArgument("program query service is not configured"))
		return
	}
	programID, err := strconv.ParseInt(r.URL.Query().Get("program_id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, httpx.InvalidArgument("invalid query: program_id"))
		return
	}
	ticketCategoryID, _ := strconv.ParseInt(r.URL.Query().Get("ticket_category_id"), 10, 64)
	detail, err := h.queries.Detail(r.Context(), programID, ticketCategoryID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, detail)
}
