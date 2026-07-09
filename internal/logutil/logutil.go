// Package logutil centralizes structured logging to stderr and carries a logger
// on a context so call chains can reach it without extra parameters.
package logutil

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// ctxKey is the private context key type for the stored logger.
type ctxKey struct{}

// New returns a logger writing to stderr at the given level. Unknown levels
// fall back to info. Valid levels are debug, info, warn, and error.
func New(level string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

// WithLogger returns a context carrying the given logger.
func WithLogger(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, log)
}

// FromContext returns the logger stored on the context, or a logger that
// discards output when none is present.
func FromContext(ctx context.Context) *slog.Logger {
	if log, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// parseLevel maps a level name to a slog level, defaulting to info.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
