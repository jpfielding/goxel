package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandler(t *testing.T) {
	ctx := context.Background()
	ctx = AppendCtx(ctx, slog.String("number", "42"))
	ctx = AppendCtx(ctx, slog.String("pet", "donkey"))
	var b bytes.Buffer
	h := ContextHandler{
		Handler: slog.NewJSONHandler(&b, &slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelInfo,
		})}
	logger := slog.New(h)
	logger.ErrorContext(ctx, "test")
	tmp := map[string]string{}
	assert.Nil(t, json.Unmarshal(b.Bytes(), &tmp))
	assert.Equal(t, "ERROR", tmp["level"])
	assert.Equal(t, "test", tmp["msg"])
	assert.Equal(t, "42", tmp["number"])
	assert.Equal(t, "donkey", tmp["pet"])
}

func TestHandlerWithPreventsContext(t *testing.T) {
	ctx := context.Background()
	ctx = AppendCtx(ctx, slog.String("number", "42"))
	ctx = AppendCtx(ctx, slog.String("pet", "donkey"))
	var b bytes.Buffer
	h := ContextHandler{
		Handler: slog.NewJSONHandler(&b, &slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelInfo,
		})}
	logger := slog.New(h)
	logger = logger.With(slog.String("with", "value"))
	logger.ErrorContext(ctx, "test")
	tmp := map[string]string{}
	assert.Nil(t, json.Unmarshal(b.Bytes(), &tmp))
	assert.Equal(t, "ERROR", tmp["level"])
	assert.Equal(t, "test", tmp["msg"])
	assert.Equal(t, "value", tmp["with"])
	assert.Equal(t, "", tmp["pet"])
}
