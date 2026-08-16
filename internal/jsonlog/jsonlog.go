// Package jsonlog writes one JSON object per log entry, so a deployment's logs
// can be shipped to any collector without a parsing rule written for this app in
// particular.
//
// It is deliberately small: the app logs from a handful of places (the request
// error path, the scheduler, startup and shutdown), and every entry is a
// message plus a flat map of string properties. There is no level filtering
// beyond a minimum severity, no context plumbing, and no sub-loggers.
package jsonlog

import (
	"encoding/json"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

// Level is the severity of a log entry. Entries below a Logger's minimum level
// are dropped.
type Level int8

// The severity levels, in increasing order. LevelOff is above every real level,
// so a logger created with it writes nothing.
const (
	LevelInfo Level = iota
	LevelError
	LevelFatal
	LevelOff
)

// String returns the name written to the "level" field. LevelOff has no name:
// it is a threshold, never the level of an actual entry.
func (l Level) String() string {
	switch l {
	case LevelInfo:
		return "INFO"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return ""
	}
}

// Logger writes JSON entries to a destination, serialized by its own mutex so
// that concurrent callers cannot interleave halves of two entries on one line.
// The zero value is not usable; call New.
type Logger struct {
	out      io.Writer
	minLevel Level
	mu       sync.Mutex
}

// New returns a Logger writing entries of at least minLevel to out.
func New(out io.Writer, minLevel Level) *Logger {
	return &Logger{
		out:      out,
		minLevel: minLevel,
	}
}

// PrintInfo records a normal event. properties may be nil; when set, its pairs
// are written under "properties" — that is where anything worth grepping for
// later belongs (a feed ID, a request URL), rather than interpolated into the
// message, which would make the message unaggregatable.
func (l *Logger) PrintInfo(message string, properties map[string]string) {
	l.print(LevelInfo, message, properties)
}

// PrintError records a failure the app recovered from. The entry carries a
// stack trace.
func (l *Logger) PrintError(err error, properties map[string]string) {
	l.print(LevelError, err.Error(), properties)
}

// PrintFatal records a failure the app cannot continue past and then terminates
// the process with status 1. It never returns, so it is only for startup wiring
// and for the server's own exit path — never for a request handler, where it
// would take every other in-flight request down with it.
func (l *Logger) PrintFatal(err error, properties map[string]string) {
	l.print(LevelFatal, err.Error(), properties)
	os.Exit(1)
}

func (l *Logger) print(level Level, message string, properties map[string]string) (int, error) {
	if level < l.minLevel {
		return 0, nil
	}

	aux := struct {
		Level      string            `json:"level"`
		Time       string            `json:"time"`
		Message    string            `json:"message"`
		Properties map[string]string `json:"properties,omitempty"`
		Trace      string            `json:"trace,omitempty"`
	}{
		Level:      level.String(),
		Time:       time.Now().UTC().Format(time.RFC3339),
		Message:    message,
		Properties: properties,
	}

	if level >= LevelError {
		aux.Trace = string(debug.Stack())
	}

	// A marshalling failure still has to produce a line: dropping the entry
	// would lose the error that was being reported in the first place.
	line, err := json.Marshal(aux)
	if err != nil {
		line = []byte(LevelError.String() + ": unable to marshal log message: " + err.Error())
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.out.Write(append(line, '\n'))
}

// Write makes Logger an io.Writer, so it can be handed to APIs that expect one
// — http.Server.ErrorLog in particular. Everything arriving this way is logged
// at ERROR with no properties.
func (l *Logger) Write(message []byte) (n int, err error) {
	return l.print(LevelError, string(message), nil)
}
