package application

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
	"tickethub/pkg/observability"
)

const CreateOrderTopic = "ticket_hub.create_order"

type OrderNumberGenerator interface {
	NextOrderNumber(userID int64) (int64, error)
}

type InventoryLocker interface {
	LockSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) ([]program.Seat, error)
	RollbackSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) error
}

type EventPublisher interface {
	Publish(ctx context.Context, event mq.Event) error
}

type OrderPricingService interface {
	CalculateAmount(ctx context.Context, cmd program.CreateOrderCommand) (int64, error)
}

type CreateOrderUsecase struct {
	orderNumbers  OrderNumberGenerator
	identifierIDs interface{ NextID() (int64, error) }
	inventory     InventoryLocker
	events        EventPublisher
	pricing       OrderPricingService
	validator     PurchaseValidator
	idempotency   IdempotencyStore
	processingTTL time.Duration
	resultTTL     time.Duration
	rateLimiter   PurchaseRateLimiter
	nowFunc       func() time.Time
}

func NewCreateOrderUsecase(orderNumbers OrderNumberGenerator, identifierIDs interface{ NextID() (int64, error) }, inventory InventoryLocker, events EventPublisher, pricing OrderPricingService) CreateOrderUsecase {
	return CreateOrderUsecase{
		orderNumbers:  orderNumbers,
		identifierIDs: identifierIDs,
		inventory:     inventory,
		events:        events,
		pricing:       pricing,
		processingTTL: 5 * time.Minute,
		resultTTL:     24 * time.Hour,
		nowFunc:       time.Now,
	}
}

func (u CreateOrderUsecase) WithIdempotency(store IdempotencyStore) CreateOrderUsecase {
	u.idempotency = store
	return u
}

func (u CreateOrderUsecase) WithRateLimiter(limiter PurchaseRateLimiter) CreateOrderUsecase {
	u.rateLimiter = limiter
	return u
}

func (u CreateOrderUsecase) WithValidator(validator PurchaseValidator) CreateOrderUsecase {
	u.validator = validator
	return u
}

func (u CreateOrderUsecase) CreateAsync(ctx context.Context, cmd program.CreateOrderCommand) (program.CreateOrderResult, error) {
	startedAt := time.Now()
	observability.IncCounter("ticket_hub_create_order_total", map[string]string{"result": "attempt"})
	defer func() {
		observability.ObserveHistogram("ticket_hub_create_order_latency_seconds", nil, time.Since(startedAt).Seconds())
	}()
	if u.validator != nil {
		if err := u.validator.Validate(ctx, cmd); err != nil {
			return program.CreateOrderResult{}, err
		}
	}
	if u.rateLimiter != nil {
		allowed, err := u.rateLimiter.Allow(ctx, cmd)
		if err != nil {
			return program.CreateOrderResult{}, err
		}
		if !allowed {
			return program.CreateOrderResult{}, therrors.New(therrors.CodeTooManyRequests, "purchase request rate exceeded")
		}
	}
	if err := validateIdempotencyKey(cmd.RequestID); err != nil {
		return program.CreateOrderResult{}, err
	}
	fingerprint, err := purchaseFingerprint(cmd)
	if err != nil {
		return program.CreateOrderResult{}, err
	}
	idempotency, err := u.idempotency.Begin(ctx, cmd.RequestID, fingerprint, u.processingTTL)
	if err != nil {
		return program.CreateOrderResult{}, err
	}
	if idempotency.State == IdempotencyCompleted {
		return program.CreateOrderResult{OrderNumber: idempotency.OrderNumber}, nil
	}
	if idempotency.State == IdempotencyProcessing {
		return program.CreateOrderResult{}, therrors.New(therrors.CodeDuplicateSubmission, "order request is already processing")
	}
	abortIdempotency := true
	defer func() {
		if abortIdempotency {
			_ = u.idempotency.Abort(context.WithoutCancel(ctx), cmd.RequestID, fingerprint)
		}
	}()
	amountCent, err := u.pricing.CalculateAmount(ctx, cmd)
	if err != nil {
		return program.CreateOrderResult{}, err
	}
	orderNumber, err := u.orderNumbers.NextOrderNumber(cmd.UserID)
	if err != nil {
		return program.CreateOrderResult{}, err
	}
	identifierID, err := u.identifierIDs.NextID()
	if err != nil {
		return program.CreateOrderResult{}, err
	}
	payload, err := json.Marshal(mq.CreateOrderEvent{
		OrderNumber:      orderNumber,
		ProgramID:        cmd.ProgramID,
		UserID:           cmd.UserID,
		TicketCategoryID: cmd.TicketCategoryID,
		SeatIDs:          append([]int64(nil), cmd.SeatIDs...),
		TicketUserIDs:    append([]int64(nil), cmd.TicketUserIDs...),
		AmountCent:       amountCent,
		IdentifierID:     identifierID,
		OrderVersion:     "async-kafka",
		CreatedAt:        u.nowFunc(),
	})
	if err != nil {
		return program.CreateOrderResult{}, err
	}
	if _, err := u.inventory.LockSeats(ctx, cmd, orderNumber, identifierID); err != nil {
		return program.CreateOrderResult{}, err
	}
	if err := u.events.Publish(ctx, mq.Event{
		Topic: CreateOrderTopic,
		Key:   strconv.FormatInt(orderNumber, 10),
		Header: mq.Header{
			EventID:       strconv.FormatInt(identifierID, 10),
			SchemaVersion: "v1",
			OccurredAt:    u.nowFunc(),
		},
		Payload: payload,
	}); err != nil {
		observability.IncCounter("ticket_hub_kafka_produce_fail_total", map[string]string{"topic": CreateOrderTopic})
		if rollbackErr := u.inventory.RollbackSeats(ctx, cmd, orderNumber, identifierID); rollbackErr != nil {
			return program.CreateOrderResult{}, errors.Join(err, rollbackErr)
		}
		return program.CreateOrderResult{}, err
	}
	abortIdempotency = false
	if err := u.idempotency.Complete(ctx, cmd.RequestID, fingerprint, orderNumber, u.resultTTL); err != nil {
		return program.CreateOrderResult{}, err
	}
	observability.IncCounter("ticket_hub_create_order_total", map[string]string{"result": "accepted"})
	return program.CreateOrderResult{OrderNumber: orderNumber}, nil
}
