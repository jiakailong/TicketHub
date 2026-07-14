package observability

import (
	"context"
	"log"
)

type Logger interface {
	Info(ctx context.Context, msg string, fields ...any)
	Error(ctx context.Context, msg string, fields ...any)
}

type StdLogger struct{}

func (StdLogger) Info(ctx context.Context, msg string, fields ...any) {
	log.Println(append([]any{"INFO", msg}, fields...)...)
}

func (StdLogger) Error(ctx context.Context, msg string, fields ...any) {
	log.Println(append([]any{"ERROR", msg}, fields...)...)
}
