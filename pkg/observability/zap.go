package observability

import (
	"context"

	"go.uber.org/zap"
)

type ZapLogger struct {
	logger *zap.Logger
}

func NewZapLogger(service string, level string) (ZapLogger, error) {
	cfg := zap.NewProductionConfig()
	if level == "debug" {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	logger, err := cfg.Build()
	if err != nil {
		return ZapLogger{}, err
	}
	return ZapLogger{logger: logger.With(zap.String("service", service))}, nil
}

func (l ZapLogger) Info(ctx context.Context, msg string, fields ...any) {
	l.logger.Info(msg, appendTrace(ctx, fields...)...)
}

func (l ZapLogger) Error(ctx context.Context, msg string, fields ...any) {
	l.logger.Error(msg, appendTrace(ctx, fields...)...)
}

func appendTrace(ctx context.Context, fields ...any) []zap.Field {
	result := make([]zap.Field, 0, len(fields)/2+1)
	if traceID := TraceID(ctx); traceID != "" {
		result = append(result, zap.String("trace_id", traceID))
	}
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		result = append(result, zap.Any(key, fields[i+1]))
	}
	return result
}
