package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trading-go/config"
)

func TestSetupWritesTraceLogFile(t *testing.T) {
	t.Cleanup(func() {
		_ = Close()
	})

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "trading-go.log")
	traceDir := filepath.Join(tempDir, "traces")

	logger, err := Setup(config.LogConfig{
		Level:    "info",
		Path:     logPath,
		TraceDir: traceDir,
	})
	if err != nil {
		t.Fatalf("Setup error: %v", err)
	}

	logger.WithField("trace_id", "trace-123").Info("hello trace log")

	if err := Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	mainLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile main log error: %v", err)
	}
	if !strings.Contains(string(mainLog), "hello trace log") {
		t.Fatalf("expected aggregate log to contain message, got %s", string(mainLog))
	}

	traceLog, err := os.ReadFile(filepath.Join(traceDir, "trace-123.log"))
	if err != nil {
		t.Fatalf("ReadFile trace log error: %v", err)
	}
	if !strings.Contains(string(traceLog), "hello trace log") {
		t.Fatalf("expected trace log to contain message, got %s", string(traceLog))
	}
}
