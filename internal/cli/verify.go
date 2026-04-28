package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tesseracode/tesserapatch/internal/workflow"
)

// verifyCmd implements `tpatch verify <slug>` — Slice A surface of the
// freshness-overlay design (ADR-013 / PRD-verify-freshness §4.1 + §9).
//
// Slice A flags only: <slug>, --json, --quiet, --no-write, --path. The
// `--all` and `--shadow` flags ship in Slice D / are explicitly rejected
// by the design.
//
// Behaviour:
//   - Runs V0 / V1 / V2 as real checks. V2 is `recipe_parses` only;
//     `recipe_op_targets_resolve` (V3) and V4–V9 are stubs that emit
//     `passed: true, skipped: true, reason: "not yet implemented (Slice C)"`
//     so the 10-check report shape is reviewable in this slice.
//   - On `--json`, emits the full report (all 10 checks) on stdout. The
//     persisted `Verify` record carries only the trimmed field set
//     (Reviewer Note 1, M15-W3 APPROVED WITH NOTES at 3c122aa).
//   - `--no-write` runs every check but does NOT persist. Useful for
//     harness dry-runs.
//   - `--quiet` suppresses the human per-check output. Combined with
//     `--json` only the JSON report is emitted.
//
// Exit code contract (PRD-verify-freshness §6 Q7 + §5):
//   - 0  — verdict passed; freshness recorded.
//   - 2  — every verify failure mode: verdict failed, refused pre-apply
//     state, V0 abort (status.json unreadable), missing slug, and
//     running verify outside a tpatch workspace.
//   - 1  — reserved for generic CLI errors (cobra usage, malformed flag).
//
// All exit-2 paths route through *ExitCodeError so cli.Execute() can
// surface the right OS exit code; only truly generic errors fall through
// to the legacy exit-1 collapse.
func verifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <slug>",
		Short: "Run integrity checks against a feature's recipe and dependencies (EXPERIMENTAL)",
		Long: `tpatch verify runs static and apply-simulation checks against a
feature and writes a freshness-overlay record to status.json. Slice A
ships V0/V1/V2 as real checks (status_loaded, intent_files_present,
recipe_parses); V3 (recipe_op_targets_resolve) and V4–V9 are stubs
deferred to Slice C. The lifecycle state is never mutated — verify is
a freshness overlay, not a state transition (ADR-013 D1).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			slug := args[0]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				// Verify could not even open the workspace — covers
				// non-tpatch directory and missing-slug-as-store-error
				// cases. PRD §5 binds both to exit 2.
				return &ExitCodeError{Code: 2, Message: fmt.Sprintf("verify aborted: %v", err)}
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			quiet, _ := cmd.Flags().GetBool("quiet")
			noWrite, _ := cmd.Flags().GetBool("no-write")

			report, runErr := workflow.RunVerify(s, slug, workflow.VerifyOptions{NoWrite: noWrite})
			if report == nil {
				// RunVerify bailed before producing any report (e.g.
				// empty-slug guard). PRD §5 maps every verify failure
				// mode to exit 2.
				return &ExitCodeError{Code: 2, Message: fmt.Sprintf("verify aborted: %v", runErr)}
			}

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			if asJSON {
				if err := report.WriteJSONReport(out); err != nil {
					return err
				}
				if !quiet {
					report.WriteHumanReport(errOut)
				}
			} else if !quiet {
				report.WriteHumanReport(out)
			} else {
				// --quiet without --json: just the verdict line.
				fmt.Fprintf(out, "verify %s — %s\n", report.Slug, report.Verdict)
			}

			// Refusal (pre-apply lifecycle) — surface exit 2 via the
			// typed error. RunVerify did NOT persist a record on this
			// path (PRD §3.4.5 + §5).
			if workflow.IsRefused(runErr) {
				return &ExitCodeError{Code: 2, Message: runErr.Error()}
			}
			if runErr != nil {
				// V0 abort and friends (e.g. missing slug surfacing as
				// LoadFeatureStatus failure). PRD §5 binds these to
				// exit 2 — the report has already been rendered above.
				return &ExitCodeError{Code: 2, Message: runErr.Error()}
			}
			if report.ExitCode != 0 {
				// Verdict-failed — surface exit 2 via the typed error
				// (PRD §6 Q7) without leaking a noisy message; the
				// report is the diagnostic.
				return &ExitCodeError{
					Code:    report.ExitCode,
					Message: fmt.Sprintf("verify failed (%d check(s) did not pass)", countFailedBlockers(report)),
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Emit the full structured report on stdout")
	cmd.Flags().Bool("quiet", false, "Suppress the per-check human output")
	cmd.Flags().Bool("no-write", false, "Run all checks but do not persist the Verify record (harness dry-run)")
	return cmd
}

func countFailedBlockers(report *workflow.VerifyReport) int {
	n := 0
	for _, c := range report.Checks {
		if c.Skipped || c.Passed {
			continue
		}
		if c.Severity == workflow.SeverityBlock || c.Severity == workflow.SeverityBlockAbort {
			n++
		}
	}
	return n
}
