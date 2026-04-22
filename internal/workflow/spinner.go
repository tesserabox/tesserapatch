package workflow

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Spinner is a lightweight braille-frame status indicator for long-running
// LLM calls. When the target writer is not a TTY, the constructor returns
// a no-op instance that never writes anything. Zero external deps.
type Spinner struct {
	w       io.Writer
	msg     string
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	enabled bool
}

// braille frames borrowed from the classic `spin` charset. 150ms cadence
// matches what most CLI tools use; fast enough to feel "alive", slow
// enough to avoid eating terminal bandwidth.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner starts a spinner immediately. Callers that want automatic
// TTY-detection should prefer NewSpinnerIfTTY.
func NewSpinner(w io.Writer, msg string) *Spinner {
	s := &Spinner{
		w:       w,
		msg:     msg,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		enabled: true,
	}
	go s.run()
	return s
}

// NewSpinnerIfTTY returns an active spinner when w is a TTY (or a
// fallback to stderr's TTY-ness if w isn't an *os.File), and a no-op
// instance otherwise. Tests and CI pipes get a silent spinner that
// still satisfies the Start/Stop API without writing output.
func NewSpinnerIfTTY(w io.Writer, msg string) *Spinner {
	if !isTerminal(w) {
		return &Spinner{w: w, msg: msg, enabled: false, stop: make(chan struct{}), done: make(chan struct{})}
	}
	return NewSpinner(w, msg)
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		// Fall back to stderr's TTY-ness so workflow callers passing
		// a bytes.Buffer in tests automatically get the no-op path.
		f = os.Stderr
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func (s *Spinner) run() {
	defer close(s.done)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	frame := 0
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			fmt.Fprintf(s.w, "\r%s %s", spinnerFrames[frame%len(spinnerFrames)], s.msg)
			frame++
		}
	}
}

// Stop halts the spinner and clears the current line. Safe to call
// multiple times; safe to call on a no-op spinner.
func (s *Spinner) Stop() {
	s.once.Do(func() {
		if !s.enabled {
			return
		}
		close(s.stop)
		<-s.done
		// Clear line: \r + spaces covering frame+msg + \r.
		clear := "\r" + pad(len(s.msg)+4) + "\r"
		fmt.Fprint(s.w, clear)
	})
}

func pad(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
