package httpapi

import (
	"net/http"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/app/customize-service/internal/application"
	"tickethub/app/customize-service/internal/domain/customize"
	"tickethub/pkg/httpx"
)

type Handler struct {
	messages application.MessageRecordService
}

func NewHandler(messages application.MessageRecordService) Handler {
	return Handler{messages: messages}
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/v1/customize/messages", h.messagesHandler)
	server.HandleFunc("/v1/customize/messages/failed", h.markFailed)
}

func (h Handler) messagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID        int64  `json:"id"`
		MessageID string `json:"message_id"`
		Topic     string `json:"topic"`
		Status    string `json:"status"`
		Reason    string `json:"reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.messages.Save(r.Context(), customize.MessageRecord{
		ID:        req.ID,
		MessageID: req.MessageID,
		Topic:     req.Topic,
		Status:    customize.MessageStatus(req.Status),
		Reason:    req.Reason,
	}); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"success": true})
}

func (h Handler) markFailed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		MessageID string `json:"message_id"`
		Topic     string `json:"topic"`
		Reason    string `json:"reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.messages.MarkFailed(r.Context(), req.MessageID, req.Topic, req.Reason); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"success": true})
}
