package env

import (
	"strings"
	"testing"
	"time"
)

// Each getter follows the same contract: unset or empty yields the default, a
// valid value is parsed, and an unparseable one yields the default *and* records
// an error. The table drives all four cases per type.
func TestGettersParseAndRecord(t *testing.T) {
	const key = "SIGNET_TEST_VALUE"

	tests := []struct {
		name    string
		set     bool
		value   string
		get     func(l *Loader) any
		want    any
		wantErr bool
	}{
		{
			name: "string unset returns default",
			get:  func(l *Loader) any { return l.String(key, "fallback") },
			want: "fallback",
		},
		{
			name:  "string empty returns default",
			set:   true,
			value: "",
			get:   func(l *Loader) any { return l.String(key, "fallback") },
			want:  "fallback",
		},
		{
			name:  "string set returns value",
			set:   true,
			value: "production",
			get:   func(l *Loader) any { return l.String(key, "fallback") },
			want:  "production",
		},
		{
			name: "int unset returns default",
			get:  func(l *Loader) any { return l.Int(key, 8000) },
			want: 8000,
		},
		{
			name:  "int set returns value",
			set:   true,
			value: "9090",
			get:   func(l *Loader) any { return l.Int(key, 8000) },
			want:  9090,
		},
		{
			name:    "int invalid records error and returns default",
			set:     true,
			value:   "abc",
			get:     func(l *Loader) any { return l.Int(key, 8000) },
			want:    8000,
			wantErr: true,
		},
		{
			name: "float unset returns default",
			get:  func(l *Loader) any { return l.Float(key, 5) },
			want: float64(5),
		},
		{
			name:  "float set returns value",
			set:   true,
			value: "2.5",
			get:   func(l *Loader) any { return l.Float(key, 5) },
			want:  2.5,
		},
		{
			name:    "float invalid records error and returns default",
			set:     true,
			value:   "fast",
			get:     func(l *Loader) any { return l.Float(key, 5) },
			want:    float64(5),
			wantErr: true,
		},
		{
			name: "bool unset returns default",
			get:  func(l *Loader) any { return l.Bool(key, true) },
			want: true,
		},
		{
			name:  "bool set returns value",
			set:   true,
			value: "false",
			get:   func(l *Loader) any { return l.Bool(key, true) },
			want:  false,
		},
		{
			name:    "bool invalid records error and returns default",
			set:     true,
			value:   "yes please",
			get:     func(l *Loader) any { return l.Bool(key, true) },
			want:    true,
			wantErr: true,
		},
		{
			name: "duration unset returns default",
			get:  func(l *Loader) any { return l.Duration(key, 15*time.Minute) },
			want: 15 * time.Minute,
		},
		{
			name:  "duration set returns value",
			set:   true,
			value: "720h",
			get:   func(l *Loader) any { return l.Duration(key, 15*time.Minute) },
			want:  720 * time.Hour,
		},
		{
			// "30d" is the tempting-but-invalid spelling operators reach for.
			name:    "duration invalid records error and returns default",
			set:     true,
			value:   "30d",
			get:     func(l *Loader) any { return l.Duration(key, 15*time.Minute) },
			want:    15 * time.Minute,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(key, tt.value)
			} else {
				t.Setenv(key, "")
			}

			l := NewLoader()
			got := tt.get(l)

			if got != tt.want {
				t.Errorf("value: got %v, want %v", got, tt.want)
			}
			if gotErr := l.Err() != nil; gotErr != tt.wantErr {
				t.Errorf("error: got %v, want %v (%v)", gotErr, tt.wantErr, l.Err())
			}
		})
	}
}

// A failing getter must name the variable and quote what was set — that is what
// makes a grouped error actionable.
func TestErrorMentionsKeyAndValue(t *testing.T) {
	const key = "SIGNET_TEST_VALUE"
	t.Setenv(key, "30d")

	l := NewLoader()
	l.Duration(key, time.Minute)

	err := l.Err()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), key) {
		t.Errorf("error should name the key, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"30d"`) {
		t.Errorf("error should quote the value, got: %v", err)
	}
}

func TestRequired(t *testing.T) {
	const key = "SIGNET_TEST_REQUIRED"

	t.Run("set returns value", func(t *testing.T) {
		t.Setenv(key, "postgres://localhost/signet")

		l := NewLoader()
		got := l.Required(key)

		if got != "postgres://localhost/signet" {
			t.Errorf("got %q", got)
		}
		if l.Err() != nil {
			t.Errorf("unexpected error: %v", l.Err())
		}
	})

	t.Run("unset records error", func(t *testing.T) {
		t.Setenv(key, "")

		l := NewLoader()
		l.Required(key)

		if l.Err() == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(l.Err().Error(), key) {
			t.Errorf("error should name the key, got: %v", l.Err())
		}
	})
}

func TestCheck(t *testing.T) {
	const key = "SIGNET_TEST_VALUE"

	t.Run("satisfied records nothing", func(t *testing.T) {
		t.Setenv(key, "5")

		l := NewLoader()
		l.Check(true, key, "must be greater than 0")

		if l.Err() != nil {
			t.Errorf("unexpected error: %v", l.Err())
		}
	})

	t.Run("violated quotes the value that was set", func(t *testing.T) {
		t.Setenv(key, "-1")

		l := NewLoader()
		l.Check(false, key, "must be greater than 0")

		err := l.Err()
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "must be greater than 0") {
			t.Errorf("unexpected message: %v", err)
		}
		if !strings.Contains(err.Error(), `"-1"`) {
			t.Errorf("error should quote the offending value, got: %v", err)
		}
	})

	t.Run("violated on an unset variable omits the value", func(t *testing.T) {
		t.Setenv(key, "")

		l := NewLoader()
		l.Check(false, key, "must be greater than 0")

		err := l.Err()
		if err == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(err.Error(), "got") {
			t.Errorf("nothing was set, so no value should be quoted: %v", err)
		}
	})
}

// The reason the Loader exists: one pass must surface every problem, not just
// the first.
func TestErrCollectsEveryProblem(t *testing.T) {
	t.Setenv("SIGNET_TEST_A", "abc")
	t.Setenv("SIGNET_TEST_B", "30d")
	t.Setenv("SIGNET_TEST_C", "-1")

	l := NewLoader()
	l.Int("SIGNET_TEST_A", 1)
	l.Duration("SIGNET_TEST_B", time.Minute)
	rps := l.Float("SIGNET_TEST_C", 5)
	l.Check(rps > 0, "SIGNET_TEST_C", "must be greater than 0")

	err := l.Err()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, key := range []string{"SIGNET_TEST_A", "SIGNET_TEST_B", "SIGNET_TEST_C"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should mention %s, got: %v", key, err)
		}
	}
}

func TestErrIsNilWhenClean(t *testing.T) {
	l := NewLoader()
	l.String("SIGNET_TEST_UNSET", "default")

	if l.Err() != nil {
		t.Errorf("expected nil, got %v", l.Err())
	}
}
