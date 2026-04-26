package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDryBuildPython_SyntaxErrorRejected pins the new Python branch.
// `python3 -m py_compile` catches the syntax error in the LLM's
// new_content and the rejection text routes through the canonical
// V2 prefix the planner agent already knows how to retry on.
//
// Skipped when python3 is not installed (sandbox / minimal CI).
func TestDryBuildPython_SyntaxErrorRejected(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		if _, err := exec.LookPath("python"); err != nil {
			t.Skip("python3 not on PATH; skip the Python branch test")
		}
	}
	repo := t.TempDir()
	// Seed a tiny Python project — the file content doesn't matter,
	// only that the repo root has a recognised structure.
	if err := os.WriteFile(filepath.Join(repo, "setup.py"), []byte("# stub\n"), 0o644); err != nil {
		t.Fatalf("seed setup.py: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "broken.py",
		Kind:       "create",
		NewContent: "def foo(:\n    return 1\n", // intentional SyntaxError
	}}
	rej := dryBuildPython(bus, changes)
	if rej == "" {
		t.Fatal("Python dry-build must reject syntactically broken new_content")
	}
	if !strings.Contains(rej, "V2 dry-build failed") {
		t.Errorf("rejection text should carry the V2 prefix the planner recognises; got %q", rej)
	}
	if !strings.Contains(rej, "Python") {
		t.Errorf("rejection text should name the language; got %q", rej)
	}
}

// TestDryBuildPython_ValidCodeAccepted: the happy path — well-formed
// Python passes (returns "") so the planner moves on to the next
// validator.
func TestDryBuildPython_ValidCodeAccepted(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		if _, err := exec.LookPath("python"); err != nil {
			t.Skip("python3 not on PATH")
		}
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "setup.py"), []byte("# stub\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "good.py",
		Kind:       "create",
		NewContent: "def foo():\n    return 1\n",
	}}
	if rej := dryBuildPython(bus, changes); rej != "" {
		t.Errorf("valid Python should not be rejected; got %q", rej)
	}
}

// TestDryBuildPython_NoPyChangeNoOp: when changes contains no .py
// files, the helper must return "" without spawning python3 (the
// fallthrough lets the next-language helper run).
func TestDryBuildPython_NoPyChangeNoOp(t *testing.T) {
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{Path: "main.go", Kind: "create", NewContent: "package main\n"}}
	if rej := dryBuildPython(bus, changes); rej != "" {
		t.Errorf("no .py change → no Python dry-build → no rejection; got %q", rej)
	}
}

// TestDryBuildNodeJS_SyntaxErrorRejected pins the Node.js branch.
// `node --check` parses the file and reports SyntaxError; the
// rejection routes through the V2 prefix.
//
// Skipped when node is not installed.
func TestDryBuildNodeJS_SyntaxErrorRejected(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH; skip the Node.js branch test")
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "broken.js",
		Kind:       "create",
		NewContent: "function foo( {\n  return 1;\n}\n",
	}}
	rej := dryBuildNodeJS(bus, changes)
	if rej == "" {
		t.Fatal("Node.js dry-build must reject syntactically broken new_content")
	}
	if !strings.Contains(rej, "V2 dry-build failed") {
		t.Errorf("rejection text should carry the V2 prefix; got %q", rej)
	}
	if !strings.Contains(rej, "Node.js") {
		t.Errorf("rejection text should name the language; got %q", rej)
	}
}

// TestDryBuildPython_DeleteIgnored: kind=delete entries should be
// ignored by the dry-build (nothing to syntax-check). Same shape as
// the existing Go path.
func TestDryBuildPython_DeleteIgnored(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		if _, err := exec.LookPath("python"); err != nil {
			t.Skip("python3 not on PATH")
		}
	}
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{Path: "old.py", Kind: "delete"}}
	if rej := dryBuildPython(bus, changes); rej != "" {
		t.Errorf("kind=delete should not run dry-build; got %q", rej)
	}
}
