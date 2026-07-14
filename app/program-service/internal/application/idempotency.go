package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

type IdempotencyState int

const (
	IdempotencyAcquired IdempotencyState = iota + 1
	IdempotencyProcessing
	IdempotencyCompleted
)

type IdempotencyResult struct {
	State       IdempotencyState
	OrderNumber int64
}

type IdempotencyStore interface {
	Begin(ctx context.Context, key string, fingerprint string, processingTTL time.Duration) (IdempotencyResult, error)
	Complete(ctx context.Context, key string, fingerprint string, orderNumber int64, resultTTL time.Duration) error
	Abort(ctx context.Context, key string, fingerprint string) error
}

func purchaseFingerprint(cmd program.CreateOrderCommand) (string, error) {
	payload, err := json.Marshal(struct {
		UserID           int64   `json:"user_id"`
		ProgramID        int64   `json:"program_id"`
		TicketCategoryID int64   `json:"ticket_category_id"`
		SeatIDs          []int64 `json:"seat_ids"`
		TicketUserIDs    []int64 `json:"ticket_user_ids"`
	}{cmd.UserID, cmd.ProgramID, cmd.TicketCategoryID, cmd.SeatIDs, cmd.TicketUserIDs})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func validateIdempotencyKey(key string) error {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 128 {
		return therrors.New(therrors.CodeInvalidArgument, "idempotency key must contain 8 to 128 characters")
	}
	return nil
}
