package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestLintPython_UnusedImportRejected pins ruff's F401 (unused
// import) detection. New file with `import os` but no use of `os`
// must be rejected by V5 lint with the V5 prefix the planner
// recognises.
func TestLintPython_UnusedImportRejected(t *testing.T) {
	if _, err := exec.LookPath("ruff"); err != nil {
		t.Skip("ruff not on PATH; skip Python lint test")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "noisy.py",
		Kind:       "create",
		NewContent: "import os\nimport sys\n\ndef main():\n    return sys.argv[1]\n", // os imported but never used → F401
	}}
	rej := lintPython(bus, changes)
	if rej == "" {
		t.Fatal("ruff F401 unused-import should reject")
	}
	if !strings.Contains(rej, "V5 lint failed") {
		t.Errorf("rejection should carry V5 prefix; got %q", rej)
	}
	if !strings.Contains(rej, "Python") {
		t.Errorf("rejection should name the language; got %q", rej)
	}
}

// TestLintPython_ValidCodeAccepted: well-formed Python with no lint
// issues passes (returns "") so the planner moves on.
func TestLintPython_ValidCodeAccepted(t *testing.T) {
	if _, err := exec.LookPath("ruff"); err != nil {
		t.Skip("ruff not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "clean.py",
		Kind:       "create",
		NewContent: "def add(a, b):\n    return a + b\n",
	}}
	if rej := lintPython(bus, changes); rej != "" {
		t.Errorf("clean Python should not be rejected; got %q", rej)
	}
}

// TestLintPython_ModifyKindSkipped: kind=modify on a Python file is
// skipped — pre-existing files likely have the same pattern, so
// rejecting them creates churn. Only kind=create gets the strict
// lint sweep.
func TestLintPython_ModifyKindSkipped(t *testing.T) {
	if _, err := exec.LookPath("ruff"); err != nil {
		t.Skip("ruff not on PATH")
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "stub.py"), []byte("# stub\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "stub.py",
		Kind:       "modify", // <-- modify, not create
		NewContent: "import os\nimport sys\n\ndef main():\n    return sys.argv[1]\n",
	}}
	if rej := lintPython(bus, changes); rej != "" {
		t.Errorf("kind=modify Python should NOT trigger strict lint; got %q", rej)
	}
}

// TestLintGoFmt_UnformattedRejected: gofmt-dirty source must be
// rejected so the planner re-emits with proper indentation /
// brace placement.
func TestLintGoFmt_UnformattedRejected(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "ugly.go",
		Kind:       "create",
		NewContent: "package x\nfunc Foo()    int {return 1 }\n", // intentional bad spacing
	}}
	rej := lintGoFmt(bus, changes)
	if rej == "" {
		t.Fatal("gofmt -l should reject unformatted Go file")
	}
	if !strings.Contains(rej, "V5 lint failed") {
		t.Errorf("rejection should carry V5 prefix; got %q", rej)
	}
}

// TestLintGoFmt_CleanCodeAccepted: gofmt-clean Go passes.
func TestLintGoFmt_CleanCodeAccepted(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "clean.go",
		Kind:       "create",
		NewContent: "package x\n\nfunc Foo() int { return 1 }\n",
	}}
	if rej := lintGoFmt(bus, changes); rej != "" {
		t.Errorf("clean Go should not be rejected; got %q", rej)
	}
}

// TestValidatePlanLint_DisabledViaSetting: when SetLintEnabled(false)
// is called, validatePlanLint short-circuits regardless of file
// contents.
func TestValidatePlanLint_DisabledViaSetting(t *testing.T) {
	if _, err := exec.LookPath("ruff"); err != nil {
		t.Skip("ruff not on PATH (would short-circuit anyway)")
	}
	prev := LintEnabled()
	defer SetLintEnabled(prev)

	SetLintEnabled(false)
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{
		Path:       "bad.py",
		Kind:       "create",
		NewContent: "import os\n", // would be F401 if lint ran
	}}
	if rej := validatePlanLint(bus, changes); rej != "" {
		t.Errorf("disabled lint should return empty; got %q", rej)
	}
}
