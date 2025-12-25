package logging

import (
	"context"
	"log/slog"
)

var _ slog.Handler = &ContextHandler{}

// define a non-public type to key this logger
type loggerKeyType int

// define a non-public const for keying this logger in the context
const loggerKey loggerKeyType = iota

type ContextHandler struct {
	slog.Handler
}

// Handle adds contextual attributes to the Record before calling the underlying
// handler
func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(loggerKey).([]slog.Attr); ok {
		for _, v := range attrs {
			r.AddAttrs(v)
		}
	}
	return h.Handler.Handle(ctx, r)
}

// AppendCtx adds an slog attribute to the provided context so that it will be
// included in any Record created with such context
func AppendCtx(parent context.Context, attr ...slog.Attr) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	var tmp []slog.Attr
	if v, ok := parent.Value(loggerKey).([]slog.Attr); ok {
		tmp = append(tmp, v...)
	}
	tmp = append(tmp, attr...)
	return context.WithValue(parent, loggerKey, tmp)
}
