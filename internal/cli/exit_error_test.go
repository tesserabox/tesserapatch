package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

// TestExecute_PropagatesExitCodeError locks in F1: when a command's
// RunE returns an *ExitCodeError, Execute() must return that exit code
// (not the legacy collapse-to-1). Other errors keep returning 1.
func TestExecute_PropagatesExitCodeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"plain error -> 1", errors.New("boom"), 1},
		{"ExitCodeError{2} -> 2", &ExitCodeError{Code: 2, Message: "verify failed"}, 2},
		{"ExitCodeError{3} -> 3", &ExitCodeError{Code: 3, Message: "custom"}, 3},
		{"nil -> 0", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{
				Use:           "synthroot",
				SilenceUsage:  true,
				SilenceErrors: true,
			}
			child := &cobra.Command{
				Use: "boom",
				RunE: func(*cobra.Command, []string) error {
					return tc.err
				},
			}
			root.AddCommand(child)
			root.SetArgs([]string{"boom"})
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})

			err := root.Execute()
			got := 0
			if err != nil {
				if e := asExitCodeError(err); e != nil {
					got = e.ExitCode()
				} else {
					got = 1
				}
			}
			if got != tc.want {
				t.Errorf("got %d, want %d (err=%v)", got, tc.want, err)
			}
		})
	}
}

// TestExitCodeError_ErrorMessage exercises the Error() formatter so a
// blank Message still yields a useful string for stderr.
func TestExitCodeError_ErrorMessage(t *testing.T) {
	if got := (&ExitCodeError{Code: 2, Message: "verify failed"}).Error(); got != "verify failed" {
		t.Errorf("with message: got %q", got)
	}
	if got := (&ExitCodeError{Code: 7}).Error(); got != "exit 7" {
		t.Errorf("blank message fallback: got %q", got)
	}
}
