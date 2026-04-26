package workflow

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestRecipeRoundtrip_WithoutCreatedBy_ByteIdentical asserts that a
// v0.5.3-shaped recipe (no `created_by` field anywhere) round-trips
// through json.Marshal byte-identical. The `omitempty` annotation on
// CreatedBy is load-bearing for backward compatibility (M14.2 / ADR-011).
func TestRecipeRoundtrip_WithoutCreatedBy_ByteIdentical(t *testing.T) {
	original := []byte(`{
  "feature": "demo",
  "operations": [
    {
      "type": "ensure-directory",
      "path": "src/",
      "content": "",
      "search": "",
      "replace": ""
    },
    {
      "type": "write-file",
      "path": "src/x.go",
      "content": "package x\n",
      "search": "",
      "replace": ""
    }
  ]
}`)
	var r ApplyRecipe
	if err := json.Unmarshal(original, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(out), bytes.TrimSpace(original)) {
		t.Fatalf("byte-identity violated.\nwant:\n%s\n\ngot:\n%s", original, out)
	}
	if strings.Contains(string(out), "created_by") {
		t.Fatalf("created_by must be omitted via omitempty; got:\n%s", out)
	}
}

// TestRecipeRoundtrip_WithCreatedBy_Preserved asserts that when a
// recipe declares `created_by`, the field round-trips through
// marshal/unmarshal preserving its value.
func TestRecipeRoundtrip_WithCreatedBy_Preserved(t *testing.T) {
	r := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{
				Type:      "write-file",
				Path:      "src/auth.ts",
				Content:   "export {}\n",
				CreatedBy: "feat-jwt-auth",
			},
		},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"created_by":"feat-jwt-auth"`) {
		t.Fatalf("created_by not emitted: %s", data)
	}
	var back ApplyRecipe
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Operations[0].CreatedBy != "feat-jwt-auth" {
		t.Fatalf("created_by lost on round-trip: got %q", back.Operations[0].CreatedBy)
	}
}

// TestRecipeUnmarshal_DisallowsUnknownFields confirms the parity guard's
// schema closure remains intact: an unknown field on an op (e.g. a
// hypothetical `tag` field invented by a confused agent) must be
// rejected by DisallowUnknownFields, exactly as the parity guard does.
func TestRecipeUnmarshal_DisallowsUnknownFields(t *testing.T) {
	withUnknown := `{
  "version": 1,
  "operations": [
    { "type": "write-file", "path": "x", "content": "", "tag": "oops" }
  ]
}`
	var recipe struct {
		Version    int               `json:"version"`
		Operations []RecipeOperation `json:"operations"`
	}
	dec := json.NewDecoder(strings.NewReader(withUnknown))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&recipe); err == nil {
		t.Fatalf("expected DisallowUnknownFields to reject unknown op field, got nil")
	}
}
