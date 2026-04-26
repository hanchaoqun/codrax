package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runLintForName looks up a registry row by its Name and runs the
// generic dispatcher on the supplied changes. Test-only helper so
// we don't have to expose lintRegistry as a public symbol just for
// table-driven tests. Returns "" when the language isn't in the
// registry (caller hard-fails).
func runLintForName(t *testing.T, ctx *types.BusContext, changes []types.FileChange, name string) string {
	t.Helper()
	for _, lang := range lintRegistry {
		if lang.Name == name {
			return runLintForLang(ctx, changes, lang)
		}
	}
	t.Fatalf("lint registry missing entry for %q", name)
	return ""
}

// TestLintRegistry_PythonUnusedImportRejected pins ruff's F401
// (unused import) detection. New file with `import os` but no use
// of `os` must be rejected by V5 lint with the V5 prefix the
// planner recognises.
func TestLintRegistry_PythonUnusedImportRejected(t *testing.T) {
	if _, err := exec.LookPath("ruff"); err != nil {
		t.Skip("ruff not on PATH; skip Python lint test")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "noisy.py",
		Kind:       "create",
		NewContent: "import os\nimport sys\n\ndef main():\n    return sys.argv[1]\n",
	}}
	rej := runLintForName(t, bus, changes, "Python")
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

func TestLintRegistry_PythonValidCodeAccepted(t *testing.T) {
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
	if rej := runLintForName(t, bus, changes, "Python"); rej != "" {
		t.Errorf("clean Python should not be rejected; got %q", rej)
	}
}

func TestLintRegistry_PythonModifyKindSkipped(t *testing.T) {
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
		Kind:       "modify",
		NewContent: "import os\nimport sys\n\ndef main():\n    return sys.argv[1]\n",
	}}
	if rej := runLintForName(t, bus, changes, "Python"); rej != "" {
		t.Errorf("kind=modify should NOT trigger strict lint; got %q", rej)
	}
}

func TestLintRegistry_GoUnformattedRejected(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "ugly.go",
		Kind:       "create",
		NewContent: "package x\nfunc Foo()    int {return 1 }\n",
	}}
	rej := runLintForName(t, bus, changes, "Go")
	if rej == "" {
		t.Fatal("gofmt -l should reject unformatted Go file")
	}
	if !strings.Contains(rej, "V5 lint failed") {
		t.Errorf("rejection should carry V5 prefix; got %q", rej)
	}
}

func TestLintRegistry_GoCleanCodeAccepted(t *testing.T) {
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
	if rej := runLintForName(t, bus, changes, "Go"); rej != "" {
		t.Errorf("clean Go should not be rejected; got %q", rej)
	}
}

// TestLintRegistry_JavaScriptParseErrorRejected: node --check rejects
// JS with a syntax error. The "node" binary is widely available;
// install hint in the registry description.
func TestLintRegistry_JavaScriptParseErrorRejected(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "broken.js",
		Kind:       "create",
		NewContent: "function foo( {\n  return 1;\n}\n", // missing close paren
	}}
	rej := runLintForName(t, bus, changes, "JavaScript")
	if rej == "" {
		t.Fatal("node --check should reject syntactically broken JS")
	}
	if !strings.Contains(rej, "V5 lint failed") {
		t.Errorf("rejection should carry V5 prefix; got %q", rej)
	}
}

// TestLintRegistry_CUnusedVariableRejected: gcc -Wall -Wextra -Werror
// promotes -Wunused-variable to an error so dead variables in new C
// files are caught at plan time.
func TestLintRegistry_CUnusedVariableRejected(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "noisy.c",
		Kind:       "create",
		NewContent: "int main(void){\n    int unused = 42;\n    return 0;\n}\n",
	}}
	rej := runLintForName(t, bus, changes, "C")
	if rej == "" {
		t.Fatal("gcc -Werror=unused-variable should reject")
	}
	if !strings.Contains(rej, "V5 lint failed") {
		t.Errorf("rejection should carry V5 prefix; got %q", rej)
	}
}

// TestLintRegistry_CCleanCodeAccepted: well-formed C with no
// warnings passes.
func TestLintRegistry_CCleanCodeAccepted(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "clean.c",
		Kind:       "create",
		NewContent: "int main(void){\n    return 0;\n}\n",
	}}
	if rej := runLintForName(t, bus, changes, "C"); rej != "" {
		t.Errorf("clean C should not be rejected; got %q", rej)
	}
}

// TestLintRegistry_CppUnusedVariableRejected: same as C but with g++
// + std=c++17. Pins the C++-specific extension matching (.cc / .cpp /
// .cxx / .hpp / .hh).
func TestLintRegistry_CppUnusedVariableRejected(t *testing.T) {
	if _, err := exec.LookPath("g++"); err != nil {
		t.Skip("g++ not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "noisy.cpp",
		Kind:       "create",
		NewContent: "int main(){\n    int unused = 42;\n    return 0;\n}\n",
	}}
	rej := runLintForName(t, bus, changes, "C++")
	if rej == "" {
		t.Fatal("g++ -Werror=unused-variable should reject")
	}
	if !strings.Contains(rej, "V5 lint failed") {
		t.Errorf("rejection should carry V5 prefix; got %q", rej)
	}
}

// TestLintRegistry_CppCleanCodeAccepted: clean C++ passes.
func TestLintRegistry_CppCleanCodeAccepted(t *testing.T) {
	if _, err := exec.LookPath("g++"); err != nil {
		t.Skip("g++ not on PATH")
	}
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "clean.cpp",
		Kind:       "create",
		NewContent: "int main(){\n    return 0;\n}\n",
	}}
	if rej := runLintForName(t, bus, changes, "C++"); rej != "" {
		t.Errorf("clean C++ should not be rejected; got %q", rej)
	}
}

// TestLintRegistry_TypeScriptToolchainSkipsWhenAbsent: tsc isn't on
// our test environment's PATH; the registry's silent-skip behaviour
// must hold (return "" without spawning anything). Documents the
// "binary missing → noop" contract.
func TestLintRegistry_TypeScriptToolchainSkipsWhenAbsent(t *testing.T) {
	if _, err := exec.LookPath("tsc"); err == nil {
		t.Skip("tsc IS on PATH; this test pins the missing-binary path")
	}
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{
		Path:       "broken.ts",
		Kind:       "create",
		NewContent: "let x: number = \"not a number\";\n", // would reject if tsc ran
	}}
	if rej := runLintForName(t, bus, changes, "TypeScript"); rej != "" {
		t.Errorf("missing tsc must short-circuit silently; got %q", rej)
	}
}

// TestValidatePlanLint_DisabledViaSetting: when SetLintEnabled(false)
// is called, validatePlanLint short-circuits regardless of contents.
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
		NewContent: "import os\n",
	}}
	if rej := validatePlanLint(bus, changes); rej != "" {
		t.Errorf("disabled lint should return empty; got %q", rej)
	}
}

// TestLintRegistry_OrderAndCoverage locks the registry against
// silent drift. The 10-language coverage was the contract this
// version delivers (added C, C++, TypeScript on top of the prior
// 7); if a future commit drops a language without notice, this
// test catches it.
func TestLintRegistry_OrderAndCoverage(t *testing.T) {
	want := []string{"C", "C++", "Go", "Java", "JavaScript", "Python", "Ruby", "Rust", "Swift", "TypeScript"}
	if len(lintRegistry) != len(want) {
		t.Fatalf("lintRegistry size = %d, want %d", len(lintRegistry), len(want))
	}
	for i, exp := range want {
		if lintRegistry[i].Name != exp {
			t.Errorf("lintRegistry[%d].Name = %q, want %q", i, lintRegistry[i].Name, exp)
		}
		if len(lintRegistry[i].Extensions) == 0 {
			t.Errorf("lintRegistry[%d] %q has no extensions", i, exp)
		}
		if lintRegistry[i].Binary == "" {
			t.Errorf("lintRegistry[%d] %q has empty binary", i, exp)
		}
		if lintRegistry[i].BuildArgs == nil {
			t.Errorf("lintRegistry[%d] %q has nil BuildArgs", i, exp)
		}
		if lintRegistry[i].FailedFn == nil {
			t.Errorf("lintRegistry[%d] %q has nil FailedFn", i, exp)
		}
	}
}

// TestLintLanguagesEnabled_ReturnsRegistry exposes the operator-
// facing capability snapshot used by cmd/root.go's startup log.
func TestLintLanguagesEnabled_ReturnsRegistry(t *testing.T) {
	got := LintLanguagesEnabled()
	if len(got) != len(lintRegistry) {
		t.Fatalf("LintLanguagesEnabled len = %d, want %d (registry size)", len(got), len(lintRegistry))
	}
	for i, st := range got {
		if st.Name != lintRegistry[i].Name {
			t.Errorf("entry %d Name = %q, want %q", i, st.Name, lintRegistry[i].Name)
		}
		if st.Binary != lintRegistry[i].Binary {
			t.Errorf("entry %d Binary = %q, want %q", i, st.Binary, lintRegistry[i].Binary)
		}
	}
}
