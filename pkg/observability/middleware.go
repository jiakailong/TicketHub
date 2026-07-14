package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"

	therrors "tickethub/pkg/errors"
)

func ServerMiddleware() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			startedAt := time.Now()
			kind := "unknown"
			operation := "unknown"
			if tr, ok := transport.FromServerContext(ctx); ok {
				kind = tr.Kind().String()
				operation = tr.Operation()
				traceID := tr.RequestHeader().Get("X-Request-ID")
				if traceID == "" {
					traceID = tr.RequestHeader().Get("X-Trace-ID")
				}
				if traceID == "" {
					traceID = newTraceID()
				}
				ctx = WithTraceID(ctx, traceID)
				tr.ReplyHeader().Set("X-Request-ID", traceID)
			}
			response, err := next(ctx, req)
			code := string(therrors.CodeOf(err))
			labels := map[string]string{"transport": kind, "operation": operation, "code": code}
			IncCounter("ticket_hub_server_requests_total", labels)
			ObserveHistogram("ticket_hub_server_request_duration_seconds", labels, time.Since(startedAt).Seconds())
			return response, err
		}
	}
}

func newTraceID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(value[:])
}
