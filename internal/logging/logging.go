// Package logging builds the application logger. Logs go to stderr so they
// never mix with exported or piped output.
package logging

import (
	"io"
	"log/slog"
)

func New(w io.Writer, level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "info":
		l = slog.LevelInfo
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelWarn
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: l}))
}
