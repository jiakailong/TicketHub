package httpapi

import (
	"net/http"
	"strconv"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/app/user-service/internal/application"
	"tickethub/pkg/auth"
	"tickethub/pkg/httpx"
	"tickethub/pkg/privacy"
)

type Handler struct {
	users     application.UserCommandService
	clientIPs httpx.ClientIPResolver
}

func NewHandler(users application.UserCommandService) Handler {
	resolver, _ := httpx.NewClientIPResolver(nil)
	return Handler{users: users, clientIPs: resolver}
}

func (h Handler) WithClientIPResolver(resolver httpx.ClientIPResolver) Handler {
	h.clientIPs = resolver
	return h
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/v1/users/register", h.register)
	server.HandleFunc("/v1/users/register/captcha", h.createRegisterCaptcha)
	server.HandleFunc("/v1/users/login", h.login)
	server.HandleFunc("/v1/users/detail", h.getUser)
	server.HandleFunc("/v1/users/ticket-users", h.ticketUsers)
}

func (h Handler) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Mobile        string `json:"mobile"`
		Password      string `json:"password"`
		CaptchaID     string `json:"captcha_id"`
		CaptchaAnswer string `json:"captcha_answer"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	created, err := h.users.Register(r.Context(), application.RegisterCommand{
		Mobile:        req.Mobile,
		Password:      req.Password,
		CaptchaID:     req.CaptchaID,
		CaptchaAnswer: req.CaptchaAnswer,
		ClientIP:      h.clientIPs.Resolve(r),
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"id":               created.ID,
		"mobile":           privacy.MaskMobile(created.Mobile),
		"real_name_status": created.RealNameStatus,
	})
}

func (h Handler) createRegisterCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Mobile string `json:"mobile"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	challenge, err := h.users.CreateRegisterCaptcha(r.Context(), application.RegisterCaptchaCommand{
		Mobile:   req.Mobile,
		ClientIP: h.clientIPs.Resolve(r),
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"captcha_id":         challenge.ID,
		"image":              challenge.Image,
		"expires_in_seconds": int(challenge.ExpiresIn.Seconds()),
	})
}

func (h Handler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Mobile   string `json:"mobile"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	result, err := h.users.Login(r.Context(), application.LoginCommand{Mobile: req.Mobile, Password: req.Password})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"access_token": result.AccessToken,
		"user": map[string]any{
			"id":               result.User.ID,
			"mobile":           privacy.MaskMobile(result.User.Mobile),
			"real_name_status": result.User.RealNameStatus,
		},
	})
}

func (h Handler) getUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, err := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	if raw := r.Header.Get(auth.UserIDHeader); raw != "" {
		userID, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil {
		httpx.WriteError(w, errInvalidQuery("user_id"))
		return
	}
	current, err := h.users.GetUser(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"id":               current.ID,
		"mobile":           privacy.MaskMobile(current.Mobile),
		"real_name_status": current.RealNameStatus,
	})
}

func (h Handler) ticketUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTicketUsers(w, r)
	case http.MethodPost:
		h.addTicketUser(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h Handler) listTicketUsers(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	if raw := r.Header.Get(auth.UserIDHeader); raw != "" {
		userID, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil {
		httpx.WriteError(w, errInvalidQuery("user_id"))
		return
	}
	items, err := h.users.ListTicketUsers(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	masked := make([]map[string]any, 0, len(items))
	for _, item := range items {
		masked = append(masked, map[string]any{
			"id":             item.ID,
			"name":           privacy.MaskName(item.Name),
			"certificate_no": privacy.MaskCertificate(item.CertificateNo),
		})
	}
	httpx.WriteOK(w, map[string]any{"ticket_users": masked})
}

func (h Handler) addTicketUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID        int64  `json:"user_id"`
		Name          string `json:"name"`
		CertificateNo string `json:"certificate_no"`
		Mobile        string `json:"mobile"`
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
	item, err := h.users.AddTicketUser(r.Context(), application.AddTicketUserCommand{
		UserID:        req.UserID,
		Name:          req.Name,
		CertificateNo: req.CertificateNo,
		Mobile:        req.Mobile,
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"id":             item.ID,
		"name":           privacy.MaskName(item.Name),
		"certificate_no": privacy.MaskCertificate(item.CertificateNo),
	})
}

func errInvalidQuery(name string) error {
	return httpx.InvalidArgument("invalid query: " + name)
}
