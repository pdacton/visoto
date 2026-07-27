package logger

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestMustInit(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name:   "default config",
			config: Config{Level: "INFO", Format: "text", Output: "stdout"},
		},
		{
			name:   "json format",
			config: Config{Level: "DEBUG", Format: "json", Output: "stderr"},
		},
		{
			name:   "warn level",
			config: Config{Level: "WARN", Format: "text", Output: "stdout"},
		},
		{
			name:   "error level",
			config: Config{Level: "ERROR", Format: "json", Output: "stderr"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// MustInit doesn't return error, it exits on failure
			// So we just test that it doesn't panic for valid configs
			MustInit(tt.config)

			// Verify logger was initialized
			logger := Get()
			if logger == nil {
				t.Error("MustInit() did not initialize logger")
			}
		})
	}
}

func TestGet(t *testing.T) {
	log = nil
	logger := Get()
	if logger == nil {
		t.Error("Get() returned nil logger")
	}
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	log = slog.New(slog.NewTextHandler(&buf, opts))

	logger := Get()
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")

	output := buf.String()
	if output == "" {
		t.Error("expected log output, got empty string")
	}
}
