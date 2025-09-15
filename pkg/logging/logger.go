package logging

import (
	"io"
	"log/slog"

	"gopkg.in/natefinch/lumberjack.v2"
)

// ----------- SLOG -----------

// RollingFile creates a default logger for the engine
func RollingFile(path string) io.WriteCloser {
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    500,
		MaxBackups: 10,
		MaxAge:     14,
		Compress:   true,
	}
}

// Logger creates a default logger for the engine
func Logger(out io.Writer, source bool, lvl slog.Level) *slog.Logger {
	h := ContextHandler{Handler: slog.NewJSONHandler(out, &slog.HandlerOptions{
		AddSource: source,
		Level:     lvl,
	})}
	return slog.New(&h)
}
