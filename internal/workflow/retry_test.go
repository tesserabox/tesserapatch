package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tesserabox/tesserapatch/internal/provider"
)

// fakeProvider is a test double that returns scripted responses.
type fakeProvider struct {
	responses []string
	calls     int
	prompts   []string
}

func (f *fakeProvider) Check(ctx context.Context, cfg provider.Config) (*provider.Health, error) {
	return &provider.Health{}, nil
}

func (f *fakeProvider) Generate(ctx context.Context, cfg provider.Config, req provider.GenerateRequest) (string, error) {
	f.prompts = append(f.prompts, req.UserPrompt)
	if f.calls >= len(f.responses) {
		return "", errors.New("fake: out of scripted responses")
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func TestRetrySucceedsAfterRetry(t *testing.T) {
	fp := &fakeProvider{responses: []string{"not json", `{"ok": true}`}}
	cfg := provider.Config{BaseURL: "x", Model: "y"}

	var target map[string]any
	out, err := GenerateWithRetry(
		context.Background(), fp, cfg,
		provider.GenerateRequest{UserPrompt: "hello"},
		RetryOptions{MaxRetries: 2, Validate: JSONObjectValidator(&target)},
	)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if fp.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", fp.calls)
	}
	if !strings.Contains(fp.prompts[1], "previous response was invalid") {
		t.Fatalf("retry prompt missing corrective text: %q", fp.prompts[1])
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRetryExhaustsAndReturnsError(t *testing.T) {
	fp := &fakeProvider{responses: []string{"nope1", "nope2", "nope3"}}
	cfg := provider.Config{BaseURL: "x", Model: "y"}

	var target map[string]any
	_, err := GenerateWithRetry(
		context.Background(), fp, cfg,
		provider.GenerateRequest{UserPrompt: "hello"},
		RetryOptions{MaxRetries: 2, Validate: JSONObjectValidator(&target)},
	)
	if err == nil {
		t.Fatal("expected error after exhaustion")
	}
	if fp.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", fp.calls)
	}
}

func TestRetryDisabledByContext(t *testing.T) {
	fp := &fakeProvider{responses: []string{"nope", `{"ok":true}`}}
	cfg := provider.Config{BaseURL: "x", Model: "y"}

	var target map[string]any
	ctx := WithDisableRetry(context.Background(), true)
	_, err := GenerateWithRetry(
		ctx, fp, cfg,
		provider.GenerateRequest{UserPrompt: "hello"},
		RetryOptions{MaxRetries: 5, Validate: JSONObjectValidator(&target)},
	)
	if err == nil {
		t.Fatal("expected error because retry is disabled")
	}
	if fp.calls != 1 {
		t.Fatalf("expected 1 call, got %d", fp.calls)
	}
}

func TestJSONObjectValidatorStripsFences(t *testing.T) {
	var target map[string]any
	v := JSONObjectValidator(&target)
	if err := v("```json\n{\"x\": 1}\n```"); err != nil {
		t.Fatalf("fenced JSON should validate: %v", err)
	}
}

func TestNonEmptyValidator(t *testing.T) {
	v := NonEmptyValidator()
	if err := v("  \n"); err == nil {
		t.Fatal("expected error for empty response")
	}
	if err := v("hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
