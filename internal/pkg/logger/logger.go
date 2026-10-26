// Package logger builds the service-wide structured logger (log/slog) and
// exposes the chained API used across every component:
//
//	lg.Info().
//		Str(logger.AttrRunID, runID).
//		Int(logger.AttrAttempt, attempt).
//		Msg("Step attempt failed")
//
// Configuration lives in one place: main calls Configure once with the
// `logging` config block, and every package builds its own logger during
// construction with New("<component>"), which tags the component at
// initialization:
//
//	lg := logger.New("cron")
//
// It also defines the canonical attribute keys and run origins so that log
// lines stay consistent and greppable.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Logger is the type each component builds for itself with New("<component>")
// and stores in its own struct. A nil *Logger is usable: every event it
// produces is discarded, so callers never need a nil check.
type Logger struct {
	inner *slog.Logger
}

// Event is a chained log record under construction. Field methods (Str, Int,
// Err, ...) accumulate attributes and return the same event; Msg emits it.
type Event struct {
	l     *Logger
	level slog.Level
	fatal bool
	args  []any
}

// Canonical attribute keys.
const (
	AttrComponent   = "component"
	AttrPackage     = "package"
	AttrRunID       = "run_id"
	AttrTestID      = "test_id"
	AttrSource      = "source"
	AttrStep        = "step"
	AttrAttempt     = "attempt"
	AttrMaxAttempts = "max_attempts"
	AttrStatus      = "status"
	AttrRespStatus  = "resp_status"
	AttrExpected    = "expected_status"
	AttrDurationMS  = "duration_ms"
	AttrDelayMS     = "delay_ms"
	AttrMethod      = "method"
	AttrPath        = "path"
	AttrURL         = "url"
	AttrProvider    = "provider"
	AttrReceiver    = "receiver_type"
	AttrSchedule    = "schedule"
	AttrTestsCount  = "tests_count"
	AttrFailedCount = "failed_count"
	AttrCallIndex   = "call_index"
	AttrStoreType   = "store_type"
	AttrHost        = "host"
	AttrMailbox     = "mailbox"
	AttrTriggerType = "trigger_type"
	AttrAsync       = "async"
	AttrName        = "name"
	AttrReason      = "reason"
	IsEnabled       = "is_enabled"
	AttrError       = "error"
)

// Run origins.
type Flow = string

const (
	ApiFlow      Flow = "api"
	SequenceFlow Flow = "sequence"
	CronFlow     Flow = "cron"
)

// exit is swapped in tests so Event.Msg can be exercised without killing the
// test process.
var exit = os.Exit

// config is the process-wide logging configuration; Configure replaces it and
// New reads it. The zero value keeps the documented defaults (info/json).
type config struct {
	level  slog.Level
	format string
}

var (
	configMu sync.RWMutex
	active   = config{level: slog.LevelInfo, format: "json"}
)

// Configure sets the process-wide logging configuration applied by every
// logger built afterwards with New. It is meant to be called once from main
// with the values of the `logging` config block.
// level: debug|info|warn|error (empty defaults to info).
// format: json|text (empty defaults to json).
func Configure(level, format string) error {
	lvl, err := parseLevel(level)
	if err != nil {
		return err
	}

	switch format {
	case "":
		format = "json"
	case "json", "text":
	default:
		return fmt.Errorf("unknown log format %q (supported: json, text)", format)
	}

	configMu.Lock()
	active = config{level: lvl, format: format}
	configMu.Unlock()

	return nil
}

// New builds the logger of the calling component: it applies the current
// configuration (info/json until Configure runs) and tags every event with
// component=<name>. Each package creates its own logger at construction:
//
//	lg := logger.New("cron")
func New(name string) *Logger {
	configMu.RLock()
	cfg := active
	configMu.RUnlock()

	return newWith(cfg, name, os.Stdout)
}

// newWith builds the logger for a component over an arbitrary writer
// (New passes os.Stdout; tests pass a buffer).
func newWith(cfg config, name string, w io.Writer) *Logger {
	l := NewWithHandler(newHandler(cfg, w))
	if name != "" && l != nil {
		l = l.With(AttrComponent, name)
	}

	return l
}

// NewWithHandler builds a Logger over a custom slog.Handler (tests capture
// output in a buffer this way).
func NewWithHandler(h slog.Handler) *Logger {
	if h == nil {
		return nil
	}

	return &Logger{inner: slog.New(h)}
}

func newHandler(cfg config, w io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{Level: cfg.level}

	if cfg.format == "text" {
		return slog.NewTextHandler(w, opts)
	}

	return slog.NewJSONHandler(w, opts)
}

// Enabled reports whether records at the given level are emitted.
func (l *Logger) Enabled(ctx context.Context, level slog.Level) bool {
	return l != nil && l.inner != nil && l.inner.Enabled(ctx, level)
}

// With returns a child logger that adds the given attributes (typically
// AttrComponent) to every event.
func (l *Logger) With(args ...any) *Logger {
	if l == nil || l.inner == nil {
		return l
	}

	return &Logger{inner: l.inner.With(args...)}
}

// Debug starts an event at Debug level (diagnostics, noisy paths).
func (l *Logger) Debug() *Event {
	return l.Event(slog.LevelDebug)
}

// Info starts an event at Info level (normal lifecycle).
func (l *Logger) Info() *Event {
	return l.Event(slog.LevelInfo)
}

// Warn starts an event at Warn level (handled degradation, retries before
// the last attempt, rejected input).
func (l *Logger) Warn() *Event {
	return l.Event(slog.LevelWarn)
}

// Error starts an event at Error level (last retry attempt, failed runs,
// dispatch failures).
func (l *Logger) Error() *Event {
	return l.Event(slog.LevelError)
}

// Fatal starts an event that is emitted at Error level and then exits the
// process with code 1 when Msg is called.
func (l *Logger) Fatal() *Event {
	e := l.Event(slog.LevelError)
	e.fatal = true

	return e
}

// Event starts an event at an arbitrary level, for call sites that select the
// level dynamically (e.g. derived from a run status).
func (l *Logger) Event(level slog.Level) *Event {
	return &Event{l: l, level: level}
}

// Str adds a string attribute.
func (e *Event) Str(key, value string) *Event {
	return e.append(key, value)
}

// Int adds an integer attribute.
func (e *Event) Int(key string, value int) *Event {
	return e.append(key, value)
}

// Bool adds a boolean attribute.
func (e *Event) Bool(key string, value bool) *Event {
	return e.append(key, value)
}

// Duration adds a duration attribute expressed in milliseconds, so callers
// can use AttrDurationMS/AttrDelayMS directly.
func (e *Event) Duration(key string, value time.Duration) *Event {
	return e.append(key, value.Milliseconds())
}

// Any adds an attribute of any slog-compatible type.
func (e *Event) Any(key string, value any) *Event {
	return e.append(key, value)
}

// Err adds the error message under AttrError. A nil error is skipped, so
// successful paths can pass err unconditionally.
func (e *Event) Err(err error) *Event {
	if err == nil {
		return e
	}

	return e.append(AttrError, err.Error())
}

// Msg emits the event. It is a no-op for a nil logger or nil event, and
// terminates the process for events started with Fatal.
func (e *Event) Msg(msg string) {
	if e == nil || e.l == nil || e.l.inner == nil {
		return
	}

	e.l.inner.Log(context.Background(), e.level, msg, e.args...)

	if e.fatal {
		exit(1)
	}
}

func (e *Event) append(key string, value any) *Event {
	if e == nil {
		return nil
	}

	e.args = append(e.args, key, value)

	return e
}

func parseLevel(level string) (slog.Level, error) {
	switch level {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q (supported: debug, info, warn, error)", level)
	}
}
