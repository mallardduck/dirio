package logging

import (
	"io"
	"log/slog"
	"os"
)

var (
	defaultLogger   *slog.Logger
	globalVerbosity Verbosity
)

// Config holds logger configuration
type Config struct {
	Level     string    // debug, info, warn, error
	Format    string    // text, json
	Verbosity string    // quiet, normal, verbose
	Output    io.Writer // defaults to os.Stderr
}

// Setup initializes the global logger with the given configuration.
// This should be called once at application startup.
func Setup(cfg Config) {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	level := parseLevel(cfg.Level)
	globalVerbosity = ParseVerbosity(cfg.Verbosity)

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(cfg.Output, opts)
	} else {
		handler = slog.NewTextHandler(cfg.Output, opts)
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

// parseLevel converts a string log level to slog.Level.
// slog levels: Debug (-4), Info (0), Warn (4), Error (8)
func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Default returns the default logger.
// If Setup has not been called, returns slog's default logger.
func Default() *slog.Logger {
	if defaultLogger == nil {
		return slog.Default()
	}
	return defaultLogger
}

// GetVerbosity returns the global verbosity level.
func GetVerbosity() Verbosity {
	return globalVerbosity
}
