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
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no working python interpreter; skip the Python branch test")
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
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no working python interpreter")
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
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no working python interpreter")
	}
	bus := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{Path: "old.py", Kind: "delete"}}
	if rej := dryBuildPython(bus, changes); rej != "" {
		t.Errorf("kind=delete should not run dry-build; got %q", rej)
	}
}

// TestDryBuildNodeJS_ESModuleAccepted: when package.json declares
// `"type": "module"`, the V2 dry-build must NOT reject valid ES
// module syntax. Bug provenance: eval Batch I+J binary-js task
// emitted `export class Binary` and was rejected with
// "SyntaxError: Unexpected token 'export'" because node --check
// defaulted to CommonJS regardless of package.json type.
func TestDryBuildNodeJS_ESModuleAccepted(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"name":"x","type":"module"}`), 0o644); err != nil {
		t.Fatalf("seed package.json: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "module.js",
		Kind:       "create",
		NewContent: "export class Binary {\n  constructor(x) { this.x = x; }\n}\n",
	}}
	if rej := dryBuildNodeJS(bus, changes); rej != "" {
		t.Errorf("ESM-mode (type:module) project must accept `export` syntax; got rejection: %q", rej)
	}
}

// TestDryBuildNodeJS_BabelESMAccepted: when package.json does NOT
// declare type:module but the file ITSELF uses ESM syntax (top-level
// `export`), the per-file content scan auto-detects ESM and renames
// to .mjs in scratch so the check passes. Covers Babel/webpack/jest
// projects whose source is ESM but whose manifest is CJS — the
// dominant real-world JS pattern.
//
// Bug provenance: Batch L binary-js + space-age-js — both used valid
// `export class Foo` syntax, project used Babel/Jest, V2 dry-build
// false-positively rejected. The per-file detection generalizes the
// .mjs rename without needing to enumerate transformer toolchains.
func TestDryBuildNodeJS_BabelESMAccepted(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatalf("seed package.json: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "binary.js",
		Kind:       "create",
		NewContent: "export class Binary {\n  constructor(x) { this.x = x; }\n}\n",
	}}
	rej := dryBuildNodeJS(bus, changes)
	if rej != "" {
		t.Fatalf("file-level ESM should be auto-accepted in CJS project; got rejection:\n%s", rej)
	}
}

// TestDryBuildNodeJS_TrueSyntaxErrorStillRejected confirms the
// auto-ESM detection doesn't mask genuine syntax errors. A file
// using ESM keywords but with a real syntax error (mismatched
// brace) is still rejected — file-level detection only changes
// the parsing MODE, not the validity floor.
func TestDryBuildNodeJS_TrueSyntaxErrorStillRejected(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatalf("seed package.json: %v", err)
	}
	bus := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "broken.js",
		Kind:       "create",
		NewContent: "export class Bad {\n  constructor(x) { this.x = x;\n}\n",
	}}
	rej := dryBuildNodeJS(bus, changes)
	if rej == "" {
		t.Fatal("ESM file with mismatched brace should still be rejected (real syntax error)")
	}
	if !strings.Contains(rej, "Node.js") {
		t.Errorf("rejection should name language; got %q", rej)
	}
}

// TestContainsESMSyntax pins the per-file ESM detection helper. The
// regex is anchored at line-start (multiline) and requires a space
// after the keyword so identifiers like `importance` / `exporting`
// don't false-positive. Comments are not stripped — the detector is
// a parsing-mode hint, not an authoritative syntax check, so a
// best-effort pattern match beats a full parser.
func TestContainsESMSyntax(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"top_level_export", "export class Foo {}", true},
		{"top_level_import", "import { x } from 'y';", true},
		{"export_default", "export default function() {}", true},
		{"indented_import_in_function", "  import { x } from 'y';", true},
		{"plain_cjs", "module.exports = 1;\nconst x = require('y');", false},
		{"identifier_importance", "let importance = 5;", false},
		{"identifier_exporting", "const exporting = true;", false},
		{"comment_only", "// import { x } from 'y'\nmodule.exports = 1;", false},
		{"multi_line_with_export_later", "// header\n\nconst y = 1;\nexport { y };", true},
		{"empty_file", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := containsESMSyntax([]byte(c.src)); got != c.want {
				t.Errorf("containsESMSyntax(%q) = %v, want %v", c.src, got, c.want)
			}
		})
	}
}

// TestNodePackageJSONIsModule covers the helper's decision matrix.
func TestNodePackageJSONIsModule(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		content  string
		want     bool
	}{
		{"explicit module", `{"type":"module"}`, true},
		{"explicit commonjs", `{"type":"commonjs"}`, false},
		{"no type field", `{"name":"foo"}`, false},
		{"empty json object", `{}`, false},
		{"malformed json", `{not valid`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.name+".json")
			if err := os.WriteFile(path, []byte(c.content), 0o644); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if got := nodePackageJSONIsModule(path); got != c.want {
				t.Errorf("got %v, want %v for content %q", got, c.want, c.content)
			}
		})
	}
	t.Run("missing file", func(t *testing.T) {
		if got := nodePackageJSONIsModule(filepath.Join(dir, "absent.json")); got {
			t.Errorf("missing file should return false (CommonJS default)")
		}
	})
}
