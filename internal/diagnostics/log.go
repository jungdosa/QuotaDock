// Package diagnostics owns QuotaDock's bounded, local-only diagnostic logs.
package diagnostics

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jungdosa/QuotaDock/internal/security"
)

const (
	NormalLogName   = "quotadock.log"
	CrashLogName    = "crash.log"
	WatchLogName    = "watch.log"
	SessionFileName = "session.marker"

	NormalMaxBytes int64 = 1 << 20
	CrashMaxBytes  int64 = 256 << 10
	WatchMaxBytes  int64 = 1 << 20
)

const timestampLayout = "2006-01-02T15:04:05.000Z07:00"

var appEvents = []string{
	"app.start",
	"app.exit",
	"provider.refresh",
	"provider.state",
	"session.invalidate",
	"session.reconnect",
	"display.change",
	"window.fit",
	"render.paint",
	"render.blank",
}

var watchEvents = []string{
	"watch.start",
	"watch.gone",
	"watch.display",
	"watch.sysevent",
}

// NewRunID returns the short random identifier shared by every event from one
// process. It contains no machine or user identity.
func NewRunID() (string, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("create diagnostic run id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// LevelFromEnvironment keeps the default at Info without extending the
// settings schema. Only the explicit debug value enables Debug events.
func LevelFromEnvironment(getenv func(string) string) slog.Level {
	if getenv != nil && strings.EqualFold(strings.TrimSpace(getenv("QUOTADOCK_LOG_LEVEL")), "debug") {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// FileLogger writes one masked JSON object per line to one bounded file.
type FileLogger struct {
	writer *cappedWriter
	logger *slog.Logger
}

func newFileLogger(path, version, run string, maximum int64, level slog.Level, allowed []string) (*FileLogger, error) {
	if path == "" || version == "" || run == "" {
		return nil, fmt.Errorf("diagnostic path, version, and run id are required")
	}
	writer, err := newCappedWriter(path, maximum)
	if err != nil {
		return nil, err
	}
	base := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey && attr.Value.Kind() == slog.KindTime {
				return slog.String(slog.TimeKey, attr.Value.Time().Format(timestampLayout))
			}
			return maskAttr(attr)
		},
	})
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, message := range allowed {
		allowedSet[message] = struct{}{}
	}
	handler := &eventHandler{next: base, allowed: allowedSet}
	logger := slog.New(handler).With("ver", security.MaskSecrets(version), "run", security.MaskSecrets(run))
	return &FileLogger{writer: writer, logger: logger}, nil
}

// NewWatchLogger creates the development watcher's local JSONL logger.
func NewWatchLogger(directory, version, run string) (*FileLogger, error) {
	return newFileLogger(filepath.Join(directory, WatchLogName), version, run, WatchMaxBytes, slog.LevelInfo, watchEvents)
}

func (l *FileLogger) Logger() *slog.Logger { return l.logger }

func (l *FileLogger) Close() error {
	if l == nil || l.writer == nil {
		return nil
	}
	return l.writer.Close()
}

func (l *FileLogger) Sync() error {
	if l == nil || l.writer == nil {
		return nil
	}
	return l.writer.Sync()
}

type eventHandler struct {
	next    slog.Handler
	allowed map[string]struct{}
}

func (h *eventHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *eventHandler) Handle(ctx context.Context, record slog.Record) error {
	if _, ok := h.allowed[record.Message]; !ok {
		return nil
	}
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	masked := slog.NewRecord(timestamp, record.Level, security.MaskSecrets(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		masked.AddAttrs(maskAttr(attr))
		return true
	})
	return h.next.Handle(ctx, masked)
}

func (h *eventHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	masked := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		masked = append(masked, maskAttr(attr))
	}
	return &eventHandler{next: h.next.WithAttrs(masked), allowed: h.allowed}
}

func (h *eventHandler) WithGroup(name string) slog.Handler {
	return &eventHandler{next: h.next.WithGroup(security.MaskSecrets(name)), allowed: h.allowed}
}

func maskAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	switch attr.Value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(security.MaskSecrets(attr.Value.String()))
	case slog.KindAny:
		switch value := attr.Value.Any().(type) {
		case error:
			attr.Value = slog.StringValue(security.MaskSecrets(value.Error()))
		case fmt.Stringer:
			attr.Value = slog.StringValue(security.MaskSecrets(value.String()))
		case string:
			attr.Value = slog.StringValue(security.MaskSecrets(value))
		default:
			if encoded, err := json.Marshal(value); err == nil {
				attr.Value = slog.AnyValue(json.RawMessage(security.MaskSecrets(string(encoded))))
			}
		}
	case slog.KindGroup:
		group := attr.Value.Group()
		masked := make([]slog.Attr, 0, len(group))
		for _, child := range group {
			masked = append(masked, maskAttr(child))
		}
		attr.Value = slog.GroupValue(masked...)
	}
	attr.Key = security.MaskSecrets(attr.Key)
	return attr
}

type cappedWriter struct {
	mu      sync.Mutex
	file    *os.File
	maximum int64
	size    int64
}

func newCappedWriter(path string, maximum int64) (*cappedWriter, error) {
	if maximum <= 0 {
		return nil, fmt.Errorf("diagnostic log limit must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create diagnostic directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open diagnostic log: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("protect diagnostic log: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat diagnostic log: %w", err)
	}
	writer := &cappedWriter{file: file, maximum: maximum, size: info.Size()}
	if writer.size > maximum {
		if err := writer.trim(nil); err != nil {
			file.Close()
			return nil, err
		}
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		file.Close()
		return nil, fmt.Errorf("seek diagnostic log: %w", err)
	}
	return writer, nil
}

func (w *cappedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size+int64(len(data)) <= w.maximum {
		written, err := w.file.Write(data)
		w.size += int64(written)
		return written, err
	}
	if err := w.trim(data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *cappedWriter) trim(extra []byte) error {
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek diagnostic log for trim: %w", err)
	}
	current, err := io.ReadAll(w.file)
	if err != nil {
		return fmt.Errorf("read diagnostic log for trim: %w", err)
	}
	combined := append(current, extra...)
	// Trim to 75% instead of rewriting the whole capped file for every later
	// event. This preserves the hard cap while keeping steady-state disk writes
	// proportional to the few new JSON lines.
	target := w.maximum - w.maximum/4
	kept := completeJSONLTail(combined, target)
	if err := w.file.Truncate(0); err != nil {
		return fmt.Errorf("truncate diagnostic log: %w", err)
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind diagnostic log: %w", err)
	}
	if len(kept) > 0 {
		if _, err := w.file.Write(kept); err != nil {
			return fmt.Errorf("rewrite diagnostic log: %w", err)
		}
	}
	w.size = int64(len(kept))
	_, err = w.file.Seek(0, io.SeekEnd)
	return err
}

func completeJSONLTail(data []byte, maximum int64) []byte {
	if int64(len(data)) <= maximum {
		return data
	}
	start := len(data) - int(maximum)
	if start > 0 && data[start-1] != '\n' {
		newline := bytes.IndexByte(data[start:], '\n')
		if newline < 0 {
			return nil
		}
		start += newline + 1
	}
	return data[start:]
}

func (w *cappedWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return os.ErrClosed
	}
	return w.file.Sync()
}

func (w *cappedWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
