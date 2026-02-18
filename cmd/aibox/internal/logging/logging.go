package logging

import (
	"io"
	"log/slog"
	"os"
)

// Setup configures the global slog logger based on the desired format and verbosity.
func Setup(format string, verbose bool) {
	var w io.Writer = os.Stderr
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	slog.SetDefault(slog.New(handler))
}
