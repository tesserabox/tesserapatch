package workflow

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinnerNoOpOnNonTTY(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinnerIfTTY(&buf, "Testing...")
	// Give the goroutine (if any) a moment; a correctly-detected non-TTY
	// writer should have no goroutine at all and therefore no output.
	time.Sleep(50 * time.Millisecond)
	sp.Stop()
	sp.Stop() // double-stop must be safe
	if buf.Len() != 0 {
		t.Errorf("expected no output on non-TTY, got %q", buf.String())
	}
}

func TestSpinnerMessageMapping(t *testing.T) {
	cases := map[string]string{
		"analyze":   "Analyzing...",
		"define":    "Defining...",
		"explore":   "Exploring...",
		"implement": "Implementing...",
		"":          "Generating...",
		"reconcile": "Reconcile...",
	}
	for in, want := range cases {
		if got := spinnerMessage(in); got != want {
			t.Errorf("spinnerMessage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSpinnerForcedEnabledWrites(t *testing.T) {
	// NewSpinner (not ...IfTTY) always runs the goroutine, so any writer
	// will receive frames. Used here to cover the run/Stop/clear path.
	var buf bytes.Buffer
	sp := NewSpinner(&buf, "Work")
	time.Sleep(200 * time.Millisecond)
	sp.Stop()
	out := buf.String()
	if !strings.Contains(out, "Work") {
		t.Errorf("expected spinner message in output, got %q", out)
	}
}
