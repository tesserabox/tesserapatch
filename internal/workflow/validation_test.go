package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTmp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateRejectsConflictMarkers(t *testing.T) {
	dir := t.TempDir()
	content := "func Foo() {\n<<<<<<< ours\n\treturn 1\n=======\n\treturn 2\n>>>>>>> theirs\n}\n"
	path := writeTmp(t, dir, "foo.go", content)

	res := ValidateResolvedFile(path, []byte(content), nil, nil, ValidationConfig{})
	if res.Passed {
		t.Fatal("expected Passed = false for conflict-marker file")
	}
	if f := res.FirstFailure(); f == nil || f.Name != "conflict-markers" {
		t.Errorf("expected conflict-markers failure first, got %+v", f)
	}
}

func TestValidateGoSyntax(t *testing.T) {
	dir := t.TempDir()

	good := "package main\n\nfunc Foo() int { return 1 }\n"
	pathGood := writeTmp(t, dir, "good.go", good)
	res := ValidateResolvedFile(pathGood, []byte(good), nil, nil, ValidationConfig{})
	if !res.Passed {
		t.Errorf("good Go should pass, got %+v", res)
	}

	bad := "package main\n\nfunc Foo( { return }\n"
	pathBad := writeTmp(t, dir, "bad.go", bad)
	res2 := ValidateResolvedFile(pathBad, []byte(bad), nil, nil, ValidationConfig{})
	if res2.Passed {
		t.Error("malformed Go should fail")
	}
	if f := res2.FirstFailure(); f == nil || f.Name != "native-parse" {
		t.Errorf("expected native-parse failure, got %+v", f)
	}
}

func TestValidateNonGoWithoutSyntaxCheckSkips(t *testing.T) {
	dir := t.TempDir()
	// Random bytes — would fail any real parser, but without
	// SyntaxCheckCmd the gate is skipped.
	path := writeTmp(t, dir, "data.bin", "not real content {{{{")
	res := ValidateResolvedFile(path, []byte("garbage"), nil, nil, ValidationConfig{})
	if !res.Passed {
		t.Errorf("no SyntaxCheckCmd → native-parse should skip, got %+v", res)
	}
	var found bool
	for _, g := range res.Gates {
		if g.Name == "native-parse" && g.Skipped {
			found = true
		}
	}
	if !found {
		t.Errorf("expected native-parse skipped, got gates %+v", res.Gates)
	}
}

func TestValidateNonGoWithSyntaxCheckCmd(t *testing.T) {
	dir := t.TempDir()
	path := writeTmp(t, dir, "doc.txt", "hi")

	// Pass: `true` exits 0 regardless of {file}.
	res := ValidateResolvedFile(path, []byte("hi"), nil, nil, ValidationConfig{
		SyntaxCheckCmd: "true {file}",
	})
	if !res.Passed {
		t.Errorf("true should pass, got %+v", res)
	}

	// Fail: `false` exits 1.
	res2 := ValidateResolvedFile(path, []byte("hi"), nil, nil, ValidationConfig{
		SyntaxCheckCmd: "false {file}",
	})
	if res2.Passed {
		t.Error("false should fail the native-parse gate")
	}
}

func TestValidateIdentifierPreservation(t *testing.T) {
	dir := t.TempDir()
	ours := []byte("func ProcessInput() {}\nfunc HelperOne() {}\n")
	theirs := []byte("func ProcessInput() {}\nfunc HelperTwo() {}\n")

	// Resolved preserves both — passes.
	resolved := []byte("func ProcessInput() {}\nfunc HelperOne() {}\nfunc HelperTwo() {}\n")
	path := writeTmp(t, dir, "any.txt", string(resolved))
	res := ValidateResolvedFile(path, resolved, ours, theirs, ValidationConfig{IdentifierCheck: true})
	if !res.Passed {
		t.Errorf("preserving-all should pass, got %+v", res)
	}

	// Resolved drops HelperTwo — fails.
	dropped := []byte("func ProcessInput() {}\nfunc HelperOne() {}\n")
	path2 := writeTmp(t, dir, "any2.txt", string(dropped))
	res2 := ValidateResolvedFile(path2, dropped, ours, theirs, ValidationConfig{IdentifierCheck: true})
	if res2.Passed {
		t.Error("dropping HelperTwo should fail identifier-preservation")
	}
	if f := res2.FirstFailure(); f == nil || f.Name != "identifier-preservation" {
		t.Fatalf("expected identifier-preservation failure, got %+v", f)
	} else if !strings.Contains(f.Detail, "HelperTwo") {
		t.Errorf("detail should name HelperTwo, got %q", f.Detail)
	}
}

func TestValidateIdentifierCheckOffSkips(t *testing.T) {
	dir := t.TempDir()
	ours := []byte("func MustExist() {}\n")
	resolved := []byte("// nothing\n")
	path := writeTmp(t, dir, "any.txt", string(resolved))

	res := ValidateResolvedFile(path, resolved, ours, nil, ValidationConfig{})
	// IdentifierCheck off — gate is skipped, others pass → overall pass.
	if !res.Passed {
		t.Errorf("IdentifierCheck off should skip that gate, got %+v", res)
	}
}

func TestRunTestCommandSuccess(t *testing.T) {
	dir := t.TempDir()
	res, err := RunTestCommandInShadow(dir, ValidationConfig{TestCommand: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Ran || !res.Passed {
		t.Errorf("true should Ran=true Passed=true, got %+v", res)
	}
}

func TestRunTestCommandFailure(t *testing.T) {
	dir := t.TempDir()
	res, err := RunTestCommandInShadow(dir, ValidationConfig{TestCommand: "exit 7"})
	if err != nil {
		t.Fatalf("non-zero exit is not an error: %v", err)
	}
	if !res.Ran {
		t.Error("expected Ran=true")
	}
	if res.Passed {
		t.Error("expected Passed=false")
	}
	if res.ExitCode != 7 {
		t.Errorf("exit code = %d, want 7", res.ExitCode)
	}
}

func TestRunTestCommandEmptyIsNoop(t *testing.T) {
	dir := t.TempDir()
	res, err := RunTestCommandInShadow(dir, ValidationConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ran {
		t.Error("empty TestCommand should not Run")
	}
}

func TestRunTestCommandTimeout(t *testing.T) {
	dir := t.TempDir()
	res, err := RunTestCommandInShadow(dir, ValidationConfig{
		TestCommand: "sleep 5",
		TestTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("timeout is not an error: %v", err)
	}
	if !res.TimedOut {
		t.Errorf("expected TimedOut=true, got %+v", res)
	}
}
