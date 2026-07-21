package main

import (
	"log/slog"
	"os"
	"strings"
)

// initLogger configures the global slog logger based on environment variables.
//
//	LOG_FORMAT=json   → JSON output (production / log aggregation)
//	LOG_FORMAT=text   → human-readable output (development, default)
//	LOG_LEVEL=debug   → show debug messages (development)
//	LOG_LEVEL=info    → only info and above (production, default)
func initLogger() {
	level := parseLogLevel(os.Getenv("LOG_LEVEL"))
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
