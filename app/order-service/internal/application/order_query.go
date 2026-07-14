package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
)

type OrderQueryService struct {
	orders order.Repository
}

type OrderListCursor struct {
	CreatedAt   time.Time
	OrderNumber int64
}

type OrderPage struct {
	Items      []order.Order
	NextCursor string
}

type orderPageRepository interface {
	ListByUserIDPage(ctx context.Context, userID int64, status order.Status, before OrderListCursor, limit int) ([]order.Order, error)
}

func NewOrderQueryService(orders order.Repository) OrderQueryService {
	return OrderQueryService{orders: orders}
}

func (s OrderQueryService) Get(ctx context.Context, orderNumber int64, userID int64) (order.Order, error) {
	if orderNumber <= 0 || userID <= 0 {
		return order.Order{}, therrors.New(therrors.CodeInvalidArgument, "order_number and user_id are required")
	}
	return s.orders.FindByOrderNumber(ctx, orderNumber, userID)
}

func (s OrderQueryService) List(ctx context.Context, userID int64, limit int) ([]order.Order, error) {
	page, err := s.ListPage(ctx, userID, "", "", limit)
	return page.Items, err
}

func (s OrderQueryService) ListPage(ctx context.Context, userID int64, status string, cursor string, limit int) (OrderPage, error) {
	if userID <= 0 {
		return OrderPage{}, therrors.New(therrors.CodeInvalidArgument, "user_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	parsedStatus, err := parseOrderStatus(status)
	if err != nil {
		return OrderPage{}, err
	}
	before, err := decodeOrderCursor(cursor)
	if err != nil {
		return OrderPage{}, err
	}
	repository, ok := s.orders.(orderPageRepository)
	if !ok {
		return OrderPage{}, therrors.New(therrors.CodeInfrastructure, "order page repository is not configured")
	}
	items, err := repository.ListByUserIDPage(ctx, userID, parsedStatus, before, limit+1)
	if err != nil {
		return OrderPage{}, err
	}
	page := OrderPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeOrderCursor(OrderListCursor{CreatedAt: last.CreatedAt, OrderNumber: last.OrderNumber})
	}
	return page, nil
}

func parseOrderStatus(value string) (order.Status, error) {
	status := order.Status(strings.ToUpper(strings.TrimSpace(value)))
	if status == "" {
		return "", nil
	}
	switch status {
	case order.StatusNoPay, order.StatusPaid, order.StatusCancel, order.StatusRefund:
		return status, nil
	default:
		return "", therrors.New(therrors.CodeInvalidArgument, "invalid order status")
	}
}

func encodeOrderCursor(cursor OrderListCursor) string {
	payload, _ := json.Marshal([]any{cursor.CreatedAt.UTC().Format(time.RFC3339Nano), cursor.OrderNumber})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeOrderCursor(value string) (OrderListCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return OrderListCursor{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return OrderListCursor{}, therrors.New(therrors.CodeInvalidArgument, "invalid order cursor")
	}
	var fields []json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil || len(fields) != 2 {
		return OrderListCursor{}, therrors.New(therrors.CodeInvalidArgument, "invalid order cursor")
	}
	var timestamp string
	var orderNumber int64
	if json.Unmarshal(fields[0], &timestamp) != nil || json.Unmarshal(fields[1], &orderNumber) != nil {
		return OrderListCursor{}, therrors.New(therrors.CodeInvalidArgument, "invalid order cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil || orderNumber <= 0 {
		return OrderListCursor{}, therrors.New(therrors.CodeInvalidArgument, "invalid order cursor")
	}
	return OrderListCursor{CreatedAt: createdAt, OrderNumber: orderNumber}, nil
}
