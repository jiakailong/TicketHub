package httpapi

import (
	"net/http"
	"strconv"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/app/pay-service/internal/application"
	"tickethub/pkg/auth"
	"tickethub/pkg/httpx"
)

type Handler struct {
	payments application.PayUsecase
}

func NewHandler(payments application.PayUsecase) Handler {
	return Handler{payments: payments}
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/v1/payments", h.pay)
	server.HandleFunc("/v1/payments/check", h.check)
	server.HandleFunc("/v1/payments/refund", h.refund)
	server.HandleFunc("/v1/payments/callback", h.callback)
}

func (h Handler) pay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		OrderNumber httpx.Int64 `json:"order_number"`
		AmountCent  int64       `json:"amount_cent"`
		Channel     string      `json:"channel"`
		UserID      int64       `json:"user_id"`
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
	payURL, err := h.payments.CommonPay(r.Context(), req.OrderNumber.Value(), req.UserID, req.AmountCent, req.Channel)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"pay_url": payURL})
}

func (h Handler) check(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	orderNumber, err := strconv.ParseInt(r.URL.Query().Get("order_number"), 10, 64)
	if err != nil {
		httpx.WriteError(w, httpx.InvalidArgument("invalid query: order_number"))
		return
	}
	userID, _ := strconv.ParseInt(r.Header.Get(auth.UserIDHeader), 10, 64)
	trade, err := h.payments.TradeCheck(r.Context(), orderNumber, userID, r.URL.Query().Get("channel"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, trade)
}

func (h Handler) refund(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		OrderNumber httpx.Int64 `json:"order_number"`
		AmountCent  int64       `json:"amount_cent"`
		Reason      string      `json:"reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.payments.Refund(r.Context(), req.OrderNumber.Value(), req.AmountCent, req.Reason); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"success": true})
}

func (h Handler) callback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		OrderNumber httpx.Int64 `json:"order_number"`
		AmountCent  int64       `json:"amount_cent"`
		Channel     string      `json:"channel"`
		Paid        bool        `json:"paid"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.payments.Callback(r.Context(), req.OrderNumber.Value(), req.AmountCent, req.Channel, req.Paid); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"success": true})
}
