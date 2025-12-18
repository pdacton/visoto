package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	Level  string // "DEBUG", "INFO", "WARN", "ERROR"
	Format string // "json" or "text"
	Output string // "stdout", "stderr", or file path
}

// local logger instance (package level singleton)
var log *slog.Logger

// initialize the logger based on the provided configuration
// exits the program if initialization fails (that's the Must pattern)
func MustInit(cfg Config) {
	// Parse level
	level := slog.LevelInfo
	switch strings.ToUpper(cfg.Level) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}

	// Determine output
	var output io.Writer = os.Stdout
	if cfg.Output == "stderr" {
		output = os.Stderr
	} else if cfg.Output != "" && cfg.Output != "stdout" {
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: Failed to open log file: %v\n", err)
			os.Exit(1)
		}
		output = file
	}

	// Create handler
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	}
	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(output, opts)
	} else {
		handler = slog.NewTextHandler(output, opts)
	}

	log = slog.New(handler)
}

func Get() *slog.Logger {
	if log == nil {
		log = slog.Default()
	}
	return log
}
