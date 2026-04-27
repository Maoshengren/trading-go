package logx

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"trading-go/config"
)

type traceIDKey struct{}

var (
	resourceMu      sync.Mutex
	managedClosers  []io.Closer
	activeTraceHook *traceFileHook
)

func Setup(cfg config.LogConfig) (*logrus.Logger, error) {
	logger := logrus.StandardLogger()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.ReplaceHooks(make(logrus.LevelHooks))

	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	logger.SetLevel(level)

	writer, closer, err := buildWriter(cfg.Path)
	if err != nil {
		return nil, err
	}
	logger.SetOutput(writer)

	resetManagedResources()
	registerCloser(closer)

	if strings.TrimSpace(cfg.TraceDir) != "" {
		hook, err := newTraceFileHook(cfg.TraceDir, &logrus.JSONFormatter{})
		if err != nil {
			return nil, err
		}
		logger.AddHook(hook)
		activeTraceHook = hook
	}

	return logger, nil
}

func Close() error {
	resourceMu.Lock()
	closers := append([]io.Closer(nil), managedClosers...)
	managedClosers = nil
	activeTraceHook = nil
	resourceMu.Unlock()

	var errs []string
	for _, closer := range closers {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("close log resources: %s", strings.Join(errs, "; "))
	}
	return nil
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

func TraceIDFromContext(ctx context.Context) string {
	v := ctx.Value(traceIDKey{})
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func Logger(ctx context.Context) *logrus.Entry {
	entry := logrus.StandardLogger().WithContext(ctx)
	if traceID := TraceIDFromContext(ctx); traceID != "" {
		entry = entry.WithField("trace_id", traceID)
	}
	return entry
}

func parseLevel(level string) (logrus.Level, error) {
	if strings.TrimSpace(level) == "" {
		return logrus.InfoLevel, nil
	}
	parsed, err := logrus.ParseLevel(strings.TrimSpace(level))
	if err != nil {
		return 0, fmt.Errorf("parse log level: %w", err)
	}
	return parsed, nil
}

func buildWriter(path string) (io.Writer, io.Closer, error) {
	if strings.TrimSpace(path) == "" {
		return os.Stdout, nil, nil
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("create log dir: %w", err)
		}
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	return io.MultiWriter(os.Stdout, file), file, nil
}

func resetManagedResources() {
	resourceMu.Lock()
	defer resourceMu.Unlock()
	managedClosers = nil
	activeTraceHook = nil
}

func registerCloser(closer io.Closer) {
	if closer == nil {
		return
	}
	resourceMu.Lock()
	defer resourceMu.Unlock()
	managedClosers = append(managedClosers, closer)
}

type traceFileHook struct {
	dir       string
	formatter logrus.Formatter

	mu    sync.Mutex
	files map[string]*os.File
}

func newTraceFileHook(dir string, formatter logrus.Formatter) (*traceFileHook, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create trace log dir: %w", err)
	}
	hook := &traceFileHook{
		dir:       dir,
		formatter: formatter,
		files:     make(map[string]*os.File),
	}
	registerCloser(hook)
	return hook, nil
}

func (h *traceFileHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *traceFileHook) Fire(entry *logrus.Entry) error {
	traceID, _ := entry.Data["trace_id"].(string)
	traceID = sanitizeTraceID(traceID)
	if traceID == "" {
		return nil
	}

	serialized, err := h.formatter.Format(entry)
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	file, ok := h.files[traceID]
	if !ok {
		path := filepath.Join(h.dir, traceID+".log")
		file, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open trace log file: %w", err)
		}
		h.files[traceID] = file
	}
	if _, err := file.Write(serialized); err != nil {
		return fmt.Errorf("write trace log file: %w", err)
	}
	return nil
}

func (h *traceFileHook) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var errs []string
	for traceID, file := range h.files {
		if err := file.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", traceID, err))
		}
	}
	h.files = make(map[string]*os.File)
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("close trace log files: %s", strings.Join(errs, "; "))
	}
	return nil
}

func sanitizeTraceID(traceID string) string {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(traceID)
}
