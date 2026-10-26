package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// testLogger returns a Debug-level JSON logger writing into buf.
func testLogger(buf *bytes.Buffer) *Logger {
	return NewWithHandler(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// record decodes the single JSON line written to buf.
func record(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected a log line, got none")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("log line is not valid JSON: %v (line: %s)", err, line)
	}

	return payload
}

func TestConfigureLevels(t *testing.T) {
	cases := map[string]struct {
		wantInfo  bool
		wantDebug bool
	}{
		"":      {wantInfo: true, wantDebug: false},
		"debug": {wantInfo: true, wantDebug: true},
		"info":  {wantInfo: true, wantDebug: false},
		"warn":  {wantInfo: false, wantDebug: false},
		"error": {wantInfo: false, wantDebug: false},
	}

	ctx := context.Background()

	for in, want := range cases {
		if err := Configure(in, "text"); err != nil {
			t.Fatalf("Configure(%q) error = %v", in, err)
		}

		l := New("test")
		if got := l.Enabled(ctx, slog.LevelInfo); got != want.wantInfo {
			t.Errorf("Configure(%q).Enabled(Info) = %v, want %v", in, got, want.wantInfo)
		}
		if got := l.Enabled(ctx, slog.LevelDebug); got != want.wantDebug {
			t.Errorf("Configure(%q).Enabled(Debug) = %v, want %v", in, got, want.wantDebug)
		}
	}

	t.Cleanup(func() { _ = Configure("info", "json") })
}

func TestConfigureInvalidArgs(t *testing.T) {
	before := active

	if err := Configure("verbose", "json"); err == nil {
		t.Error("Configure() with invalid level expected error, got nil")
	}

	if err := Configure("info", "xml"); err == nil {
		t.Error("Configure() with invalid format expected error, got nil")
	}

	if active != before {
		t.Errorf("invalid Configure must not change the active config, got %+v, want %+v", active, before)
	}
}

func TestParseLevel(t *testing.T) {
	lvl, err := parseLevel("warn")
	if err != nil {
		t.Fatalf("parseLevel() error = %v", err)
	}
	if lvl != slog.LevelWarn {
		t.Fatalf("parseLevel(warn) = %v, want %v", lvl, slog.LevelWarn)
	}

	if _, err := parseLevel("nope"); err == nil {
		t.Fatal("parseLevel() expected error for invalid level")
	}
}

func TestLoggerEmitsInfoByDefault(t *testing.T) {
	if err := Configure("", ""); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if !New("test").Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("logger must enable Info level with the default configuration")
	}

	t.Cleanup(func() { _ = Configure("info", "json") })
}

func TestNewTagsComponent(t *testing.T) {
	var buf bytes.Buffer
	newWith(config{level: slog.LevelDebug, format: "json"}, "cron", &buf).
		Info().Msg("Scheduler started")

	p := record(t, &buf)
	if p[AttrComponent] != "cron" {
		t.Errorf("%s = %v, want cron", AttrComponent, p[AttrComponent])
	}
}

func TestNewWithoutComponent(t *testing.T) {
	var buf bytes.Buffer
	newWith(config{level: slog.LevelInfo, format: "json"}, "", &buf).
		Info().Msg("bootstrap")

	p := record(t, &buf)
	if _, ok := p[AttrComponent]; ok {
		t.Errorf("empty component must not add %q", AttrComponent)
	}
}

func TestEventChainsFieldsAndMsg(t *testing.T) {
	var buf bytes.Buffer
	lg := testLogger(&buf)

	lg.Info().
		Str(AttrRunID, "run-1").
		Str(AttrTestID, "test-1").
		Int(AttrAttempt, 2).
		Int(AttrMaxAttempts, 3).
		Bool(AttrAsync, false).
		Duration(AttrDurationMS, 1500*time.Millisecond).
		Any(AttrTestsCount, 7).
		Msg("Test run started")

	p := record(t, &buf)

	if p["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", p["level"])
	}
	if p["msg"] != "Test run started" {
		t.Errorf("msg = %v", p["msg"])
	}
	if p[AttrRunID] != "run-1" || p[AttrTestID] != "test-1" {
		t.Errorf("string attrs = %v / %v", p[AttrRunID], p[AttrTestID])
	}
	if p[AttrAttempt] != float64(2) || p[AttrMaxAttempts] != float64(3) {
		t.Errorf("int attrs = %v / %v", p[AttrAttempt], p[AttrMaxAttempts])
	}
	if p[AttrAsync] != false {
		t.Errorf("%s = %v, want false", AttrAsync, p[AttrAsync])
	}
	if p[AttrDurationMS] != float64(1500) {
		t.Errorf("%s = %v, want 1500 (milliseconds)", AttrDurationMS, p[AttrDurationMS])
	}
	if p[AttrTestsCount] != float64(7) {
		t.Errorf("%s = %v, want 7", AttrTestsCount, p[AttrTestsCount])
	}
}

func TestEventLevelMapping(t *testing.T) {
	cases := map[string]func(*Logger) *Event{
		"DEBUG": func(l *Logger) *Event { return l.Debug() },
		"INFO":  func(l *Logger) *Event { return l.Info() },
		"WARN":  func(l *Logger) *Event { return l.Warn() },
		"ERROR": func(l *Logger) *Event { return l.Error() },
	}

	for want, start := range cases {
		var buf bytes.Buffer
		lg := testLogger(&buf)

		start(lg).Msg("event")

		if got := record(t, &buf)["level"]; got != want {
			t.Errorf("level = %v, want %v", got, want)
		}
	}
}

func TestEventErrSkipsNil(t *testing.T) {
	var buf bytes.Buffer
	lg := testLogger(&buf)

	lg.Error().Err(nil).Msg("no error attached")

	if _, ok := record(t, &buf)[AttrError]; ok {
		t.Errorf("nil Err() must not add %q", AttrError)
	}

	buf.Reset()
	lg.Error().Err(errors.New("boom")).Msg("failed")

	p := record(t, &buf)
	if p[AttrError] != "boom" {
		t.Errorf("%s = %v, want boom", AttrError, p[AttrError])
	}
}

func TestWithAddsAttributesToEveryEvent(t *testing.T) {
	var buf bytes.Buffer
	lg := testLogger(&buf).With(AttrComponent, "api")

	lg.Info().Msg("first")
	p := record(t, &buf)
	if p[AttrComponent] != "api" {
		t.Errorf("%s = %v, want api", AttrComponent, p[AttrComponent])
	}

	buf.Reset()
	lg.Info().Msg("second")
	if p := record(t, &buf); p[AttrComponent] != "api" {
		t.Errorf("%s = %v, want api", AttrComponent, p[AttrComponent])
	}
}

func TestNilLoggerDiscardsEvents(t *testing.T) {
	var lg *Logger

	lg.Info().Str("key", "value").Msg("discarded")
	lg.With(AttrComponent, "x").Warn().Msg("also discarded")

	var nilEvent *Event
	nilEvent.Int("key", 1).Msg("discarded")
}

func TestEventDynamicLevel(t *testing.T) {
	var buf bytes.Buffer
	lg := testLogger(&buf)

	lg.Event(slog.LevelWarn).Str(AttrStatus, "failed").Msg("Test run finished")

	p := record(t, &buf)
	if p["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", p["level"])
	}
}

func TestFatalExitsAfterLogging(t *testing.T) {
	var buf bytes.Buffer
	lg := testLogger(&buf)

	code := -1
	original := exit
	exit = func(c int) { code = c }
	t.Cleanup(func() { exit = original })

	lg.Fatal().Str(AttrReason, "bad config").Msg("Service startup failed")

	p := record(t, &buf)
	if p["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", p["level"])
	}
	if p[AttrReason] != "bad config" {
		t.Errorf("%s = %v", AttrReason, p[AttrReason])
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestNewWithHandlerNilReturnsNil(t *testing.T) {
	if NewWithHandler(nil) != nil {
		t.Error("NewWithHandler(nil) must return nil")
	}
}
