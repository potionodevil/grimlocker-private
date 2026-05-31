// Package errors (logging.go) defines StructuredLogger — the logging interface
// used across all Grimlocker modules — and StdLogger, its standard-library
// implementation.
//
// Modules should accept a StructuredLogger as a dependency rather than calling
// log.Printf directly. This decouples business logic from log formatting and
// makes log output testable.
//
// To attach a GrimlockError to a log entry with its full context (code,
// operation, block ID, stacktrace), use the Log method on the error itself:
//
//	if ge, ok := err.(*errors.GrimlockError); ok {
//	    ge.WithModule("storage").Log(logger)
//	}
//
// To replace StdLogger with zerolog, zap, or slog, implement the three-method
// StructuredLogger interface and inject it into the module constructors.
package errors

import (
	"fmt"
	"log"
	"strings"
)

// ─── StructuredLogger interface ───────────────────────────────────────────────

// StructuredLogger is the logging interface used across all Grimlocker modules.
// Pass it as a dependency — never call log.Printf directly from module code.
type StructuredLogger interface {
	Debug(msg string, fields map[string]any)
	Info(msg string, fields map[string]any)
	Warn(msg string, fields map[string]any)
	Error(msg string, err error, fields map[string]any)
	Fatal(msg string, err error, fields map[string]any)
}

// ─── Log method on GrimlockError ─────────────────────────────────────────────

// Log writes a structured representation of this error to the provided logger.
// Call this at the top of a handler after receiving a GrimlockError.
func (e *GrimlockError) Log(logger StructuredLogger) {
	fields := map[string]any{
		"error_code": e.Code,
		"module":     e.ModuleID,
		"operation":  e.Ctx.Operation,
	}
	if e.Ctx.BlockID != "" {
		fields["block_id"] = e.Ctx.BlockID
	}
	for k, v := range e.Ctx.Details {
		fields["detail_"+k] = v
	}
	if e.EventType != "" {
		fields["event_type"] = e.EventType
	}
	if len(e.Stack) > 0 {
		frames := make([]string, 0, len(e.Stack))
		for _, f := range e.Stack {
			frames = append(frames, f.String())
		}
		fields["stacktrace"] = frames
	}

	logger.Error(e.Message, e.Cause, fields)
}

// ─── StdLogger — wraps the standard library log package ──────────────────────

// StdLogger wraps the standard library log package as a StructuredLogger.
// Each field is appended as key=value to the log line.
// Replace with a structured logging library (zerolog, zap, slog) in production.
type StdLogger struct {
	// Prefix is prepended to every log line, e.g. "[security]".
	Prefix string
	// DebugEnabled controls whether Debug messages are emitted.
	DebugEnabled bool
}

func (l *StdLogger) format(level, msg string, fields map[string]any) string {
	var b strings.Builder
	if l.Prefix != "" {
		b.WriteString(l.Prefix)
		b.WriteString(" ")
	}
	b.WriteString(fmt.Sprintf("[%s] %s", level, msg))
	for k, v := range fields {
		b.WriteString(fmt.Sprintf(" %s=%v", k, v))
	}
	return b.String()
}

func (l *StdLogger) Debug(msg string, fields map[string]any) {
	if !l.DebugEnabled {
		return
	}
	log.Print(l.format("DEBUG", msg, fields))
}

func (l *StdLogger) Info(msg string, fields map[string]any) {
	log.Print(l.format("INFO", msg, fields))
}

func (l *StdLogger) Warn(msg string, fields map[string]any) {
	log.Print(l.format("WARN", msg, fields))
}

func (l *StdLogger) Error(msg string, err error, fields map[string]any) {
	if err != nil {
		if fields == nil {
			fields = make(map[string]any)
		}
		fields["error"] = err.Error()
	}
	log.Print(l.format("ERROR", msg, fields))
}

func (l *StdLogger) Fatal(msg string, err error, fields map[string]any) {
	if err != nil {
		if fields == nil {
			fields = make(map[string]any)
		}
		fields["error"] = err.Error()
	}
	log.Fatal(l.format("FATAL", msg, fields))
}

// NewStdLogger creates a StdLogger for the given module prefix.
func NewStdLogger(modulePrefix string) *StdLogger {
	return &StdLogger{Prefix: modulePrefix}
}

// NewDebugLogger creates a StdLogger with debug output enabled.
func NewDebugLogger(modulePrefix string) *StdLogger {
	return &StdLogger{Prefix: modulePrefix, DebugEnabled: true}
}
