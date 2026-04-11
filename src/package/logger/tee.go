package logger

import (
	"context"
	"log/slog"
)

// teeHandler fans out each log record to two handlers.
type teeHandler struct {
	a, b slog.Handler
}

func newTeeHandler(a, b slog.Handler) slog.Handler {
	return &teeHandler{a: a, b: b}
}

func (t *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return t.a.Enabled(ctx, level) || t.b.Enabled(ctx, level)
}

func (t *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	if err := t.a.Handle(ctx, r); err != nil {
		return err
	}
	return t.b.Handle(ctx, r)
}

func (t *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &teeHandler{a: t.a.WithAttrs(attrs), b: t.b.WithAttrs(attrs)}
}

func (t *teeHandler) WithGroup(name string) slog.Handler {
	return &teeHandler{a: t.a.WithGroup(name), b: t.b.WithGroup(name)}
}
