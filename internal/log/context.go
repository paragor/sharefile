package log

import (
	"context"
	"log/slog"
	"os"
)

type logContext struct{}

var logContextValue = logContext{}

var logInstance = slog.New(slog.NewJSONHandler(os.Stderr, nil))

func FromContext(ctx context.Context) *slog.Logger {
	value := ctx.Value(logContextValue)
	if value == nil {
		return logInstance
	}
	logger, ok := value.(*slog.Logger)
	if !ok {
		return logInstance
	}
	return logger
}

func PutIntoContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, logContextValue, logger)
}
