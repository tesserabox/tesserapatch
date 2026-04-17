# M8 — LLM Output Validation & Retry

**Status**: ⬜ Not started  
**Depends on**: M7

## Overview

Currently, if the LLM returns malformed JSON or unexpected output, the system silently falls back to heuristic mode. This milestone adds structured validation, retry with feedback, and guardrails.

## Tasks

- [ ] M8.1 — Add `ValidateLLMResponse()` in `internal/workflow/` — check JSON structure against expected schema per command
- [ ] M8.2 — Implement retry loop: on parse failure, re-prompt with "Your previous response was invalid: {error}. Please output ONLY valid JSON."
- [ ] M8.3 — Add configurable `max_retries` (default 2) to config.yaml
- [ ] M8.4 — Add `--no-retry` flag to analyze/define/explore/implement for debugging
- [ ] M8.5 — Log raw LLM responses to `artifacts/raw-response-{n}.txt` for debugging
- [ ] M8.6 — Add response quality metrics: track tokens used, latency, retry count in status.json
- [ ] M8.7 — Write tests with mock provider returning malformed responses

## Acceptance Criteria

- Malformed LLM output triggers retry with corrective feedback (up to max_retries)
- Each retry logs the raw response for debugging
- After max retries, falls back to heuristic with a clear warning
- Token/latency metrics visible in `tpatch status --verbose`
