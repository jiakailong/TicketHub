package observability

import "context"

type traceIDKey struct{}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

func TraceID(ctx context.Context) string {
	value, _ := ctx.Value(traceIDKey{}).(string)
	return value
}
