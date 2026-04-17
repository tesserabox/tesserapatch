package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tesserabox/tesserapatch/internal/provider"
	"github.com/tesserabox/tesserapatch/internal/store"
)

// copilotNativeAUPNotice is shown whenever the user opts in to the
// copilot-native provider. It mirrors the AUP language from M10 so the
// user acknowledges the policy regardless of whether they pick the
// managed proxy or the native path. See ADR-005.
func copilotNativeAUPNotice() string {
	return `Native GitHub Copilot provider (experimental, ADR-005)
───────────────────────────────────────────────────────────────
This activates a first-party OAuth flow that authenticates
tpatch as an editor against api.githubcopilot.com, mirroring
how VS Code's Copilot extension talks to the upstream service.

By opting in you agree to:
  • GitHub Copilot's Acceptable Use Policies
    (https://docs.github.com/site-policy/acceptable-use-policies)
  • That your Copilot entitlement gates model access;
    some models may reject requests for non-entitled seats.
  • That tpatch is not affiliated with or endorsed by GitHub.

A long-lived OAuth token is stored at:
  $XDG_DATA_HOME/tpatch/copilot-auth.json (Linux)
  ~/Library/Application Support/tpatch/copilot-auth.json (macOS)
File is created with 0600 permissions. Run
  tpatch provider copilot-logout
to delete it.
`
}

func copilotNativeOptInPrompt() string {
	return copilotNativeAUPNotice() + `
To proceed:
  tpatch config set provider.copilot_native_optin true

`
}

func providerCopilotLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "copilot-login",
		Short: "Log in to GitHub Copilot (native provider, experimental)",
		Long: `Run the GitHub OAuth device-code flow and persist a session token
that tpatch will use to authenticate against api.githubcopilot.com.

Requires prior opt-in:
  tpatch config set provider.copilot_native_optin true`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !store.CopilotNativeOptedIn() {
				fmt.Fprint(cmd.OutOrStdout(), copilotNativeOptInPrompt())
				return fmt.Errorf("copilot-native opt-in required")
			}

			// Enterprise prompt; default to github.com.
			enterprise, _ := cmd.Flags().GetString("enterprise")
			if enterprise == "" && isInteractive() {
				fmt.Fprint(cmd.OutOrStdout(), "GitHub Enterprise host (press enter for github.com): ")
				r := bufio.NewReader(os.Stdin)
				line, _ := r.ReadString('\n')
				enterprise = strings.TrimSpace(line)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
			defer cancel()

			opts := provider.CopilotLoginOptions{
				EnterpriseDomain: enterprise,
				Prompt:           cmd.OutOrStdout(),
			}
			dev, err := provider.RequestDeviceCode(ctx, opts)
			if err != nil {
				return fmt.Errorf("device-code request failed: %w", err)
			}
			provider.PrintDevicePrompt(cmd.OutOrStdout(), dev)

			token, err := provider.PollAccessToken(ctx, opts, dev)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Received access token — exchanging for Copilot session…")

			auth := &provider.CopilotAuth{
				Version: 1,
				OAuth: provider.CopilotOAuthBlock{
					AccessToken:   token,
					ObtainedAt:    time.Now().UTC().Format(time.RFC3339),
					EnterpriseURL: enterprise,
				},
			}
			if err := provider.ExchangeSessionToken(ctx, opts, auth); err != nil {
				return fmt.Errorf("session exchange failed: %w", err)
			}
			if err := provider.SaveCopilotAuth(auth); err != nil {
				return err
			}
			path, _ := provider.CopilotAuthFilePath()
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in. Token stored at %s\n", path)
			fmt.Fprintf(cmd.OutOrStdout(), "Session endpoint: %s (expires %s)\n",
				auth.Session.Endpoints["api"], auth.Session.ExpiresAt)
			return nil
		},
	}
	cmd.Flags().String("enterprise", "", "GitHub Enterprise host (default: github.com)")
	return cmd
}

func providerCopilotLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "copilot-logout",
		Short: "Delete the stored GitHub Copilot OAuth token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := provider.DeleteCopilotAuth(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Copilot auth removed.")
			return nil
		},
	}
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
