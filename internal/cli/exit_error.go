package cli

import "fmt"

// ExitCodeError is an error variant that carries a specific process
// exit code so `Execute()` can distinguish e.g. `tpatch verify` failure
// (exit 2) from generic command errors (exit 1).
//
// Only commands with a binding non-1 exit-code contract should use this
// (currently `tpatch verify` per PRD-verify-freshness §6 Q7). Returning
// a plain `error` from any other RunE keeps the legacy exit-1 behaviour.
type ExitCodeError struct {
	Code    int
	Message string
}

// Error satisfies the error interface.
func (e *ExitCodeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Message
}

// ExitCode returns the desired process exit code.
func (e *ExitCodeError) ExitCode() int {
	if e == nil {
		return 0
	}
	return e.Code
}

// asExitCodeError unwraps a chain looking for an *ExitCodeError. Returns
// the first one found (or nil). Intentionally narrow: errors.As-ish but
// inlined to avoid an extra import in cobra.go's already-busy import set.
func asExitCodeError(err error) *ExitCodeError {
	for err != nil {
		if e, ok := err.(*ExitCodeError); ok {
			return e
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return nil
		}
		err = u.Unwrap()
	}
	return nil
}
