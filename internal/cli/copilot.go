package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/tesseracode/tesserapatch/internal/provider"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// copilotInstallHint returns the multi-line install pointer for the
// upstream copilot-api proxy. Per ADR-004 D2 we point at ericc-ch/copilot-api.
//
// TODO(adr-004): re-evaluate whether to recommend the tesseracode/copilot-api
// fork if its divergent fixes (1M-context model translations) start to
// matter more than the ease-of-install of upstream. Check periodically.
func copilotInstallHint() string {
	return "The tpatch Copilot preset expects the `copilot-api` proxy running on localhost:4141.\n" +
		"Install and start it once in a dedicated terminal:\n\n" +
		"    npm install -g copilot-api\n" +
		"    copilot-api start\n\n" +
		"Or run it without installing:\n\n" +
		"    npx copilot-api@latest start\n\n" +
		"Upstream: https://github.com/ericc-ch/copilot-api\n" +
		"Note: copilot-api is a reverse-engineered proxy, not supported by GitHub.\n"
}

// copilotAUPWarning returns the Acceptable Use Policy warning text shown
// once per user (persisted via store.AcknowledgeCopilotAUP). Keep the
// wording direct — the user should consciously consent before we start
// driving traffic to GitHub's Copilot surface.
func copilotAUPWarning() string {
	return "⚠  Heads up — the GitHub Copilot integration uses copilot-api\n" +
		"   (https://github.com/ericc-ch/copilot-api), a reverse-engineered\n" +
		"   proxy that is NOT supported by GitHub. Excessive automated use\n" +
		"   may trigger GitHub's abuse-detection systems and could result in\n" +
		"   temporary suspension of your Copilot access.\n" +
		"   See: https://docs.github.com/en/site-policy/acceptable-use-policies\n"
}

// maybeShowAUPWarning prints the Copilot AUP warning the first time the
// user selects a Copilot-flavoured configuration, then records the
// acknowledgement in the global config so subsequent runs are quiet.
// Idempotent; safe to call unconditionally.
func maybeShowAUPWarning(w io.Writer, cfg provider.Config) {
	if !provider.IsCopilotProxyEndpoint(cfg) {
		return
	}
	if store.CopilotAUPAcknowledged() {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprint(w, copilotAUPWarning())
	fmt.Fprintln(w)
	_ = store.AcknowledgeCopilotAUP()
}

// ensureProviderReachable probes the configured provider. For local
// endpoints (copilot-api, Ollama) it returns a descriptive error if
// unreachable so workflow commands can hard-fail with an actionable
// install hint. For remote endpoints it trusts the user's network and
// returns (nil, nil) without probing.
//
// Per ADR-004 D4: only scoped to local endpoints to avoid penalising
// custom remote configurations where a 2s probe might be unreliable.
//
// Returns the resolved *provider.Health when the probe succeeded, so
// callers can feed PickProvider the model metadata they need without
// re-issuing /v1/models.
func ensureProviderReachable(ctx context.Context, cfg provider.Config) (*provider.Health, error) {
	if !provider.IsLocalEndpoint(cfg) {
		return nil, nil
	}
	health, err := provider.Probe(ctx, cfg)
	if err != nil {
		msg := err.Error()
		if provider.IsCopilotProxyEndpoint(cfg) {
			msg = strings.TrimSpace(msg) + "\n\n" + copilotInstallHint()
		}
		return nil, fmt.Errorf("provider at %s is unreachable: %s", cfg.BaseURL, msg)
	}
	return health, nil
}

// warnIfUnreachable runs a reachability probe and writes a user-facing
// warning to w when the probe fails. Never returns an error — `init`
// uses this to flag problems without blocking the workspace bootstrap.
func warnIfUnreachable(ctx context.Context, w io.Writer, cfg provider.Config) {
	if !cfg.Configured() || !provider.IsLocalEndpoint(cfg) {
		return
	}
	if err := provider.Reachable(ctx, cfg); err != nil {
		fmt.Fprintf(w, "\n⚠  provider at %s not reachable yet: %v\n", cfg.BaseURL, err)
		if provider.IsCopilotProxyEndpoint(cfg) {
			fmt.Fprintln(w)
			fmt.Fprint(w, copilotInstallHint())
		}
	}
}

// providerConfigFromStore converts the merged store config into a
// provider.Config. Returns a zero value (with Configured()==false) if
// the store has not been configured yet.
func providerConfigFromStore(s *store.Store) provider.Config {
	cfg, err := s.LoadMergedConfig()
	if err != nil {
		return provider.Config{}
	}
	return provider.Config{
		Type:      cfg.Provider.Type,
		BaseURL:   cfg.Provider.BaseURL,
		Model:     cfg.Provider.Model,
		AuthEnv:   cfg.Provider.AuthEnv,
		Initiator: cfg.Provider.Initiator,
	}
}
