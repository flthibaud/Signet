package env

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Loader reads environment variables and collects the problems it finds. The
// zero value is not usable; call NewLoader.
type Loader struct {
	errs []error
}

func NewLoader() *Loader {
	return &Loader{}
}

// lookup returns the raw value of key and whether it carries anything. An empty
// variable is treated as unset, so `FOO=` in a .env falls back to the default
// rather than failing to parse.
func lookup(key string) (string, bool) {
	v := os.Getenv(key)
	return v, v != ""
}

// record notes that key could not be used, quoting what was actually set.
func (l *Loader) record(key, value, msg string) {
	l.errs = append(l.errs, fmt.Errorf("%s: %s (got %q)", key, msg, value))
}

// String returns the value of key, or def when it is unset.
func (l *Loader) String(key, def string) string {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	return v
}

// Required returns the value of key and records an error when it is unset.
func (l *Loader) Required(key string) string {
	v, ok := lookup(key)
	if !ok {
		l.errs = append(l.errs, fmt.Errorf("%s: must be set", key))
	}
	return v
}

// Int returns the value of key parsed as an integer, or def when it is unset.
//
// A value that fails to parse also yields def — the caller carries on and the
// remaining variables still get read, which is the whole point of accumulating.
// The returned value is never used for real, since Err() stops the program.
func (l *Loader) Int(key string, def int) int {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.record(key, v, "must be an integer")
		return def
	}
	return n
}

// Float returns the value of key parsed as a float, or def when it is unset.
func (l *Loader) Float(key string, def float64) float64 {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		l.record(key, v, "must be a number")
		return def
	}
	return f
}

// Bool returns the value of key parsed as a boolean, or def when it is unset.
func (l *Loader) Bool(key string, def bool) bool {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.record(key, v, "must be a boolean (true or false)")
		return def
	}
	return b
}

// Duration returns the value of key parsed as a duration, or def when it is
// unset.
func (l *Loader) Duration(key string, def time.Duration) time.Duration {
	v, ok := lookup(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.record(key, v, `must be a duration such as "720h", "15m" or "30s"`)
		return def
	}
	return d
}

// Check records a constraint on an already-parsed value, mirroring the
// Check idiom internal/validator uses on the HTTP side. The raw environment
// value is read back from key so the message can quote what the operator set.
func (l *Loader) Check(ok bool, key, msg string) {
	if ok {
		return
	}
	if v, set := lookup(key); set {
		l.record(key, v, msg)
		return
	}
	l.errs = append(l.errs, fmt.Errorf("%s: %s", key, msg))
}

// Err returns every problem collected so far, or nil when there were none.
func (l *Loader) Err() error {
	if len(l.errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration: %w", errors.Join(l.errs...))
}
