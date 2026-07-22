package httpapi

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	adminv1 "tickethub/api/proto/admin/v1"
	basev1 "tickethub/api/proto/base/v1"
	customizev1 "tickethub/api/proto/customize/v1"
	migratev1 "tickethub/api/proto/migrate/v1"
	orderv1 "tickethub/api/proto/order/v1"
	payv1 "tickethub/api/proto/pay/v1"
	programv1 "tickethub/api/proto/program/v1"
	userv1 "tickethub/api/proto/user/v1"
	"tickethub/pkg/auth"
	"tickethub/pkg/httpx"
	"tickethub/pkg/privacy"
)

type Handler struct {
	client    *http.Client
	upstreams map[string]string
	grpc      *GRPCClients
	tokens    auth.TokenManager
	clientIPs httpx.ClientIPResolver
}

func NewHandler(upstreams map[string]string, tokens auth.TokenManager) Handler {
	handler, _ := NewHandlerWithGRPC(upstreams, nil, tokens)
	return handler
}

func NewHandlerWithGRPC(upstreams map[string]string, grpcUpstreams map[string]string, tokens auth.TokenManager) (Handler, error) {
	copied := make(map[string]string, len(defaultUpstreams())+len(upstreams))
	for key, value := range defaultUpstreams() {
		copied[key] = value
	}
	for key, value := range upstreams {
		if strings.TrimSpace(value) != "" {
			copied[key] = strings.TrimRight(value, "/")
		}
	}
	grpcClients, err := NewGRPCClients(grpcUpstreams)
	if err != nil {
		return Handler{}, err
	}
	resolver, _ := httpx.NewClientIPResolver(nil)
	return Handler{
		client:    &http.Client{Timeout: 5 * time.Second},
		upstreams: copied,
		grpc:      grpcClients,
		tokens:    tokens,
		clientIPs: resolver,
	}, nil
}

func (h Handler) WithClientIPResolver(resolver httpx.ClientIPResolver) Handler {
	h.clientIPs = resolver
	return h
}

type GRPCClients struct {
	conns   []*grpc.ClientConn
	user    userv1.UserServiceClient
	program programv1.ProgramServiceClient
	order   orderv1.OrderServiceClient
	pay     payv1.PayServiceClient
	base    basev1.BaseDataServiceClient
	custom  customizev1.CustomizeServiceClient
	admin   adminv1.AdminServiceClient
	migrate migratev1.MigrateServiceClient
}

func NewGRPCClients(upstreams map[string]string) (*GRPCClients, error) {
	if len(upstreams) == 0 {
		return nil, nil
	}
	clients := &GRPCClients{}
	dial := func(service string) (*grpc.ClientConn, error) {
		addr := strings.TrimSpace(upstreams[service])
		if addr == "" {
			return nil, nil
		}
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		clients.conns = append(clients.conns, conn)
		return conn, nil
	}
	if conn, err := dial("user-service"); err != nil {
		return nil, err
	} else if conn != nil {
		clients.user = userv1.NewUserServiceClient(conn)
	}
	if conn, err := dial("program-service"); err != nil {
		return nil, err
	} else if conn != nil {
		clients.program = programv1.NewProgramServiceClient(conn)
	}
	if conn, err := dial("order-service"); err != nil {
		return nil, err
	} else if conn != nil {
		clients.order = orderv1.NewOrderServiceClient(conn)
	}
	if conn, err := dial("pay-service"); err != nil {
		return nil, err
	} else if conn != nil {
		clients.pay = payv1.NewPayServiceClient(conn)
	}
	if conn, err := dial("base-data-service"); err != nil {
		return nil, err
	} else if conn != nil {
		clients.base = basev1.NewBaseDataServiceClient(conn)
	}
	if conn, err := dial("customize-service"); err != nil {
		return nil, err
	} else if conn != nil {
		clients.custom = customizev1.NewCustomizeServiceClient(conn)
	}
	if conn, err := dial("admin-service"); err != nil {
		return nil, err
	} else if conn != nil {
		clients.admin = adminv1.NewAdminServiceClient(conn)
	}
	if conn, err := dial("migrate-service"); err != nil {
		return nil, err
	} else if conn != nil {
		clients.migrate = migratev1.NewMigrateServiceClient(conn)
	}
	return clients, nil
}

func (h Handler) Register(server *khttp.Server) {
	server.HandleFunc("/api/users/register", h.userRegister)
	server.HandleFunc("/api/users/register/captcha", h.userRegisterCaptcha)
	server.HandleFunc("/api/users/login", h.userLogin)
	server.HandleFunc("/api/users/detail", h.userDetail)
	server.HandleFunc("/api/users/ticket-users", h.ticketUsers)
	server.HandleFunc("/api/programs/search", h.programSearch)
	server.HandleFunc("/api/programs/detail", h.programDetail)
	server.HandleFunc("/api/programs/suggest", h.publicProxy("program-service", "/v1/programs/suggest"))
	server.HandleFunc("/api/orders", h.orders)
	server.HandleFunc("/api/orders/cancel", h.orderCancel)
	server.HandleFunc("/api/payments", h.paymentCreate)
	server.HandleFunc("/api/payments/check", h.paymentCheck)
	server.HandleFunc("/api/payments/callback", h.publicProxy("pay-service", "/v1/payments/callback"))
	server.HandleFunc("/api/base/areas", h.baseAreas)
	server.HandleFunc("/api/base/channels", h.baseChannels)
	server.HandleFunc("/api/customize/messages", h.customizeMessages)
	server.HandleFunc("/api/customize/messages/failed", h.adminProxy("customize-service", "/v1/customize/messages/failed"))
	server.HandleFunc("/api/admin/dashboard", h.adminDashboard)
	server.HandleFunc("/api/admin/reconciliation/run", h.adminReconciliation)
	server.HandleFunc("/api/admin/programs", h.adminProxy("program-service", "/v1/admin/programs"))
	server.HandleFunc("/api/admin/programs/status", h.adminProxy("program-service", "/v1/admin/programs/status"))
	server.HandleFunc("/api/admin/discard-orders", h.adminProxy("order-service", "/v1/orders/discard-orders"))
	server.HandleFunc("/api/admin/discard-orders/retry", h.adminProxy("order-service", "/v1/orders/discard-orders/retry"))
	server.HandleFunc("/api/migrate/tasks", h.migrateTasks)
	server.HandleFunc("/api/migrate/tasks/resume", h.migrateResumeTask)
	server.HandleFunc("/api/migrate/shard-mappings", h.migrateShardMappings)
}

func (h Handler) userRegister(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.user == nil {
		h.publicProxy("user-service", "/v1/users/register")(w, r)
		return
	}
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
	reply, err := h.grpc.user.Register(r.Context(), &userv1.RegisterRequest{
		Mobile:        req.Mobile,
		Password:      req.Password,
		CaptchaId:     req.CaptchaID,
		CaptchaAnswer: req.CaptchaAnswer,
		ClientIp:      h.clientIPs.Resolve(r),
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"id":               strconv.FormatInt(reply.GetUserId(), 10),
		"mobile":           privacy.MaskMobile(reply.GetMobile()),
		"real_name_status": reply.GetRealNameStatus(),
	})
}

func (h Handler) userRegisterCaptcha(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.user == nil {
		h.publicProxy("user-service", "/v1/users/register/captcha")(w, r)
		return
	}
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
	reply, err := h.grpc.user.CreateRegisterCaptcha(r.Context(), &userv1.RegisterCaptchaRequest{
		Mobile:   req.Mobile,
		ClientIp: h.clientIPs.Resolve(r),
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"captcha_id":         reply.GetCaptchaId(),
		"image":              reply.GetImage(),
		"expires_in_seconds": reply.GetExpiresInSeconds(),
	})
}

func (h Handler) userLogin(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.user == nil {
		h.publicProxy("user-service", "/v1/users/login")(w, r)
		return
	}
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
	reply, err := h.grpc.user.Login(r.Context(), &userv1.LoginRequest{Mobile: req.Mobile, Password: req.Password})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"access_token": reply.GetAccessToken(),
		"user": map[string]any{
			"id":               strconv.FormatInt(reply.GetUser().GetUserId(), 10),
			"mobile":           privacy.MaskMobile(reply.GetUser().GetMobile()),
			"real_name_status": reply.GetUser().GetRealNameStatus(),
		},
	})
}

func (h Handler) userDetail(w http.ResponseWriter, r *http.Request) {
	claims, err := h.parseBearer(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if h.grpc == nil || h.grpc.user == nil {
		h.authProxy("user-service", "/v1/users/detail")(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID := claims.UserID
	if raw := r.URL.Query().Get("user_id"); raw != "" && claims.Role == "admin" {
		userID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			httpx.WriteError(w, httpx.InvalidArgument("invalid query: user_id"))
			return
		}
	}
	reply, err := h.grpc.user.GetUser(r.Context(), &userv1.GetUserRequest{UserId: userID})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"id":               strconv.FormatInt(reply.GetUserId(), 10),
		"mobile":           privacy.MaskMobile(reply.GetMobile()),
		"real_name_status": reply.GetRealNameStatus(),
	})
}

func (h Handler) ticketUsers(w http.ResponseWriter, r *http.Request) {
	claims, err := h.parseBearer(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if h.grpc == nil || h.grpc.user == nil {
		r.Header.Set(auth.UserIDHeader, strconv.FormatInt(claims.UserID, 10))
		r.Header.Set(auth.RoleHeader, claims.Role)
		h.proxy(w, r, "user-service", "/v1/users/ticket-users")
		return
	}
	switch r.Method {
	case http.MethodGet:
		reply, err := h.grpc.user.ListTicketUsers(r.Context(), &userv1.ListTicketUsersRequest{UserId: claims.UserID})
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		items := make([]map[string]any, 0, len(reply.GetTicketUsers()))
		for _, item := range reply.GetTicketUsers() {
			items = append(items, map[string]any{
				"id":             strconv.FormatInt(item.GetId(), 10),
				"name":           privacy.MaskName(item.GetName()),
				"certificate_no": privacy.MaskCertificate(item.GetCertificateNo()),
			})
		}
		httpx.WriteOK(w, map[string]any{"ticket_users": items})
	case http.MethodPost:
		var req struct {
			Name          string `json:"name"`
			CertificateNo string `json:"certificate_no"`
			Mobile        string `json:"mobile"`
		}
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, err)
			return
		}
		reply, err := h.grpc.user.AddTicketUser(r.Context(), &userv1.AddTicketUserRequest{
			UserId:        claims.UserID,
			Name:          req.Name,
			CertificateNo: req.CertificateNo,
			Mobile:        req.Mobile,
		})
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteOK(w, map[string]any{
			"id":             strconv.FormatInt(reply.GetId(), 10),
			"name":           privacy.MaskName(reply.GetName()),
			"certificate_no": privacy.MaskCertificate(reply.GetCertificateNo()),
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h Handler) programSearch(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.program == nil {
		h.publicProxy("program-service", "/v1/programs/search")(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	reply, err := h.grpc.program.SearchProgram(r.Context(), &programv1.SearchProgramRequest{
		Keyword:  r.URL.Query().Get("keyword"),
		City:     r.URL.Query().Get("city"),
		Page:     int32(page),
		PageSize: int32(pageSize),
		Cursor:   r.URL.Query().Get("cursor"),
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(reply.GetPrograms()))
	for _, item := range reply.GetPrograms() {
		items = append(items, publicProgramSummary(item))
	}
	httpx.WriteOK(w, map[string]any{"programs": items, "next_cursor": reply.GetNextCursor()})
}

func (h Handler) programDetail(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.program == nil {
		h.publicProxy("program-service", "/v1/programs/detail")(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	programID, err := strconv.ParseInt(r.URL.Query().Get("program_id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, httpx.InvalidArgument("invalid query: program_id"))
		return
	}
	ticketCategoryID, _ := strconv.ParseInt(r.URL.Query().Get("ticket_category_id"), 10, 64)
	reply, err := h.grpc.program.GetProgramDetail(r.Context(), &programv1.GetProgramDetailRequest{
		ProgramId:        programID,
		TicketCategoryId: ticketCategoryID,
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, publicProgramDetail(reply))
}

func (h Handler) orders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if h.grpc != nil && h.grpc.program != nil {
			h.orderCreateGRPC(w, r)
			return
		}
		h.authProxy("program-service", "/v1/program/orders")(w, r)
	case http.MethodGet:
		if h.grpc != nil && h.grpc.order != nil {
			if r.URL.Query().Get("order_number") == "" {
				h.orderListGRPC(w, r)
			} else {
				h.orderDetailGRPC(w, r)
			}
			return
		}
		h.authProxy("order-service", "/v1/orders/detail")(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h Handler) orderCreateGRPC(w http.ResponseWriter, r *http.Request) {
	claims, err := h.parseBearer(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var req struct {
		ProgramID        httpx.Int64      `json:"program_id"`
		TicketCategoryID httpx.Int64      `json:"ticket_category_id"`
		SeatIDs          httpx.Int64Slice `json:"seat_ids"`
		TicketUserIDs    httpx.Int64Slice `json:"ticket_user_ids"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	reply, err := h.grpc.program.CreateOrder(r.Context(), &programv1.CreateOrderRequest{
		RequestId:        strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		UserId:           claims.UserID,
		ProgramId:        req.ProgramID.Value(),
		TicketCategoryId: req.TicketCategoryID.Value(),
		SeatIds:          req.SeatIDs.Values(),
		TicketUserIds:    req.TicketUserIDs.Values(),
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"order_number": strconv.FormatInt(reply.GetOrderNumber(), 10)})
}

func (h Handler) orderListGRPC(w http.ResponseWriter, r *http.Request) {
	claims, err := h.parseBearer(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	reply, err := h.grpc.order.ListOrders(r.Context(), &orderv1.ListOrdersRequest{
		UserId: claims.UserID,
		Limit:  int32(limit),
		Status: r.URL.Query().Get("status"),
		Cursor: r.URL.Query().Get("cursor"),
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(reply.GetOrders()))
	for _, item := range reply.GetOrders() {
		items = append(items, publicOrder(item))
	}
	httpx.WriteOK(w, map[string]any{"orders": items, "next_cursor": reply.GetNextCursor()})
}

func (h Handler) orderDetailGRPC(w http.ResponseWriter, r *http.Request) {
	claims, err := h.parseBearer(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	orderNumber, err := strconv.ParseInt(r.URL.Query().Get("order_number"), 10, 64)
	if err != nil {
		httpx.WriteError(w, httpx.InvalidArgument("invalid query: order_number"))
		return
	}
	reply, err := h.grpc.order.GetOrder(r.Context(), &orderv1.GetOrderRequest{OrderNumber: orderNumber, UserId: claims.UserID})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, publicOrder(reply))
}

func (h Handler) orderCancel(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.order == nil {
		h.authProxy("order-service", "/v1/orders/cancel")(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	claims, err := h.parseBearer(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var req struct {
		OrderNumber httpx.Int64 `json:"order_number"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	reply, err := h.grpc.order.CancelOrder(r.Context(), &orderv1.CancelOrderRequest{OrderNumber: req.OrderNumber.Value(), UserId: claims.UserID})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, publicOrder(reply))
}

func (h Handler) paymentCreate(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.pay == nil {
		h.authProxy("pay-service", "/v1/payments")(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	claims, err := h.parseBearer(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var req struct {
		OrderNumber httpx.Int64 `json:"order_number"`
		AmountCent  int64       `json:"amount_cent"`
		Channel     string      `json:"channel"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	reply, err := h.grpc.pay.CommonPay(r.Context(), &payv1.CommonPayRequest{
		OrderNumber: req.OrderNumber.Value(),
		AmountCent:  req.AmountCent,
		Channel:     req.Channel,
		UserId:      claims.UserID,
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"pay_url": reply.GetPayUrl()})
}

func (h Handler) paymentCheck(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.pay == nil {
		h.authProxy("pay-service", "/v1/payments/check")(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	claims, err := h.parseBearer(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	orderNumber, err := strconv.ParseInt(r.URL.Query().Get("order_number"), 10, 64)
	if err != nil {
		httpx.WriteError(w, httpx.InvalidArgument("invalid query: order_number"))
		return
	}
	reply, err := h.grpc.pay.TradeCheck(r.Context(), &payv1.TradeCheckRequest{OrderNumber: orderNumber, Channel: r.URL.Query().Get("channel"), UserId: claims.UserID})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, reply)
}

func (h Handler) baseAreas(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.base == nil {
		h.publicProxy("base-data-service", "/v1/base/areas")(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	parentID, _ := strconv.ParseInt(r.URL.Query().Get("parent_id"), 10, 64)
	reply, err := h.grpc.base.ListAreas(r.Context(), &basev1.ListAreasRequest{ParentId: parentID})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, reply)
}

func (h Handler) baseChannels(w http.ResponseWriter, r *http.Request) {
	if h.grpc == nil || h.grpc.base == nil {
		h.publicProxy("base-data-service", "/v1/base/channels")(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	reply, err := h.grpc.base.ListChannelData(r.Context(), &basev1.ListChannelDataRequest{})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, reply)
}

func (h Handler) customizeMessages(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireAdmin(r); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if h.grpc == nil || h.grpc.custom == nil {
		h.adminProxy("customize-service", "/v1/customize/messages")(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		MessageID string `json:"message_id"`
		Topic     string `json:"topic"`
		Status    string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	reply, err := h.grpc.custom.RecordMessage(r.Context(), &customizev1.RecordMessageRequest{
		MessageId: req.MessageID,
		Topic:     req.Topic,
		Status:    req.Status,
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, reply)
}

func (h Handler) adminDashboard(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireAdmin(r); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if h.grpc == nil || h.grpc.admin == nil {
		h.adminProxy("admin-service", "/v1/admin/dashboard")(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	reply, err := h.grpc.admin.Dashboard(r.Context(), &adminv1.DashboardRequest{})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, reply)
}

func (h Handler) adminReconciliation(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireAdmin(r); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if h.grpc == nil || h.grpc.admin == nil {
		h.adminProxy("admin-service", "/v1/admin/reconciliation/run")(w, r)
		return
	}
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
	reply, err := h.grpc.admin.RunReconciliation(r.Context(), &adminv1.RunReconciliationRequest{
		ProgramId:       req.ProgramID,
		RepairInventory: req.RepairInventory,
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{
		"triggered":                reply.GetTriggered(),
		"program_id":               reply.GetProgramId(),
		"mismatch_count":           reply.GetMismatchCount(),
		"record_mismatch_count":    reply.GetRecordMismatchCount(),
		"inventory_mismatch_count": reply.GetInventoryMismatchCount(),
		"repaired_inventory_count": reply.GetRepairedInventoryCount(),
		"inventory_differences":    reply.GetInventoryDifferences(),
		"discard_order_count":      reply.GetDiscardOrderCount(),
	})
}

func (h Handler) migrateTasks(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireAdmin(r); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if h.grpc == nil || h.grpc.migrate == nil {
		h.adminProxy("migrate-service", "/v1/migrate/tasks")(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		VirtualShard int32  `json:"virtual_shard"`
		SourceShard  string `json:"source_shard"`
		TargetShard  string `json:"target_shard"`
		BatchSize    int32  `json:"batch_size"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	reply, err := h.grpc.migrate.CreateTask(r.Context(), &migratev1.CreateTaskRequest{
		VirtualShard: req.VirtualShard,
		SourceShard:  req.SourceShard,
		TargetShard:  req.TargetShard,
		BatchSize:    req.BatchSize,
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, reply)
}

func (h Handler) migrateShardMappings(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireAdmin(r); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if h.grpc == nil || h.grpc.migrate == nil {
		h.adminProxy("migrate-service", "/v1/migrate/shard-mappings")(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	reply, err := h.grpc.migrate.ListShardMappings(r.Context(), &migratev1.ListShardMappingsRequest{})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, map[string]any{"shard_mappings": reply.GetMappings()})
}

func (h Handler) migrateResumeTask(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireAdmin(r); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if h.grpc == nil || h.grpc.migrate == nil {
		h.adminProxy("migrate-service", "/v1/migrate/tasks/resume")(w, r)
		return
	}
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
	reply, err := h.grpc.migrate.ResumeTask(r.Context(), &migratev1.ResumeTaskRequest{TaskId: req.TaskID})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteOK(w, reply)
}

func publicProgramSummary(item *programv1.ProgramSummary) map[string]any {
	return map[string]any{
		"id":             strconv.FormatInt(item.GetId(), 10),
		"title":          item.GetTitle(),
		"city":           item.GetCity(),
		"place":          item.GetPlace(),
		"show_time":      item.GetShowTime(),
		"status":         item.GetStatus(),
		"min_price_cent": item.GetMinPriceCent(),
	}
}

func publicProgramDetail(item *programv1.ProgramDetail) map[string]any {
	categories := make([]map[string]any, 0, len(item.GetTicketCategories()))
	for _, category := range item.GetTicketCategories() {
		categories = append(categories, map[string]any{
			"id":         strconv.FormatInt(category.GetId(), 10),
			"name":       category.GetName(),
			"price_cent": category.GetPriceCent(),
			"remain":     category.GetRemain(),
		})
	}
	seats := make([]map[string]any, 0, len(item.GetSeats()))
	for _, seat := range item.GetSeats() {
		seats = append(seats, map[string]any{
			"id":                 strconv.FormatInt(seat.GetId(), 10),
			"ticket_category_id": strconv.FormatInt(seat.GetTicketCategoryId(), 10),
			"row_code":           seat.GetRowCode(),
			"col_code":           seat.GetColCode(),
			"price_cent":         seat.GetPriceCent(),
			"status":             seat.GetStatus(),
		})
	}
	return map[string]any{
		"id":                strconv.FormatInt(item.GetId(), 10),
		"title":             item.GetTitle(),
		"city":              item.GetCity(),
		"place":             item.GetPlace(),
		"show_time":         item.GetShowTime(),
		"status":            item.GetStatus(),
		"ticket_categories": categories,
		"seats":             seats,
	}
}

func publicOrder(item *orderv1.OrderReply) map[string]any {
	return map[string]any{
		"order_number":       strconv.FormatInt(item.GetOrderNumber(), 10),
		"status":             item.GetStatus(),
		"amount_cent":        item.GetAmountCent(),
		"program_id":         strconv.FormatInt(item.GetProgramId(), 10),
		"ticket_category_id": strconv.FormatInt(item.GetTicketCategoryId(), 10),
		"seat_ids":           publicIDs(item.GetSeatIds()),
		"ticket_user_ids":    publicIDs(item.GetTicketUserIds()),
		"created_at":         item.GetCreatedAt(),
		"paid_at":            item.GetPaidAt(),
		"canceled_at":        item.GetCanceledAt(),
		"refunded_at":        item.GetRefundedAt(),
	}
}

func publicIDs(values []int64) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.FormatInt(value, 10))
	}
	return result
}

func (h Handler) publicProxy(service string, targetPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.proxy(w, r, service, targetPath)
	}
}

func (h Handler) authProxy(service string, targetPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := h.parseBearer(r)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		r.Header.Set(auth.UserIDHeader, strconv.FormatInt(claims.UserID, 10))
		r.Header.Set(auth.RoleHeader, claims.Role)
		h.proxy(w, r, service, targetPath)
	}
}

func (h Handler) adminProxy(service string, targetPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := h.requireAdmin(r)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		r.Header.Set(auth.UserIDHeader, strconv.FormatInt(claims.UserID, 10))
		r.Header.Set(auth.RoleHeader, claims.Role)
		h.proxy(w, r, service, targetPath)
	}
}

func (h Handler) requireAdmin(r *http.Request) (auth.Claims, error) {
	claims, err := h.parseBearer(r)
	if err != nil {
		return auth.Claims{}, err
	}
	if claims.Role != "admin" {
		return auth.Claims{}, httpx.Forbidden("admin role is required")
	}
	return claims, nil
}

func (h Handler) parseBearer(r *http.Request) (auth.Claims, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return auth.Claims{}, httpx.Unauthenticated("missing bearer token")
	}
	return h.tokens.Parse(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
}

func (h Handler) proxy(w http.ResponseWriter, r *http.Request, service string, targetPath string) {
	baseURL, ok := h.upstreams[service]
	if !ok {
		httpx.WriteError(w, httpx.InvalidArgument("unknown upstream: "+service))
		return
	}
	target, err := url.Parse(baseURL + targetPath)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	target.RawQuery = r.URL.RawQuery
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	req.Header = r.Header.Clone()
	resp, err := h.client.Do(req)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func defaultUpstreams() map[string]string {
	return map[string]string{
		"user-service":      "http://127.0.0.1:8001",
		"program-service":   "http://127.0.0.1:8002",
		"order-service":     "http://127.0.0.1:8003",
		"pay-service":       "http://127.0.0.1:8004",
		"base-data-service": "http://127.0.0.1:8005",
		"customize-service": "http://127.0.0.1:8006",
		"admin-service":     "http://127.0.0.1:8007",
		"migrate-service":   "http://127.0.0.1:8008",
	}
}
