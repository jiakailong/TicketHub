package httpapi

import (
	"net/http"
	"strconv"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/app/base-data-service/internal/application"
	"tickethub/pkg/httpx"
)

type Handler struct {
	base application.BaseQueryService
}

func NewHandler(base application.BaseQueryService) Handler {
	return Handler{base: base}
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/v1/base/areas", h.areas)
	server.HandleFunc("/v1/base/channels", h.channels)
}

func (h Handler) areas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parentID, _ := strconv.ParseInt(r.URL.Query().Get("parent_id"), 10, 64)
	items, err := h.base.ListAreas(r.Context(), parentID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"areas": items})
}

func (h Handler) channels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items, err := h.base.ListChannelData(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"channels": items})
}
