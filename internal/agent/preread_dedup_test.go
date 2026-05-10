// Package agent — preread_dedup_test.go (2026-05-10).
//
// L2 of the forced-read remediation: preReadRequiredFiles now
// honors an `excludeRead` set (typically the cumulative
// EvidenceClosure.ReadSet across all dispatches in a Run). Files
// the LLM has already read in any prior iteration are silently
// skipped — re-injecting them wastes prompt tokens and contradicts
// the LLM's own judgment when it had chosen not to emit_evidence
// on them.
package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func writePreReadTempFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", full, err)
	}
	return full
}

func TestPreReadRequiredFiles_NilExclude_LegacyBehavior(t *testing.T) {
	repoRoot := t.TempDir()
	writePreReadTempFile(t, repoRoot, "internal/foo.go", "package foo\n\nfunc Foo() {}\n")
	got := preReadRequiredFiles(repoRoot, []string{"internal/foo.go"}, 2, 100, nil)
	if !strings.Contains(got, "internal/foo.go") {
		t.Errorf("nil excludeRead: file should be injected; got %q", got)
	}
}

func TestPreReadRequiredFiles_EmptyExclude_LegacyBehavior(t *testing.T) {
	repoRoot := t.TempDir()
	writePreReadTempFile(t, repoRoot, "internal/foo.go", "package foo\n")
	got := preReadRequiredFiles(repoRoot, []string{"internal/foo.go"}, 2, 100, map[string]bool{})
	if !strings.Contains(got, "internal/foo.go") {
		t.Errorf("empty excludeRead: file should be injected; got %q", got)
	}
}

func TestPreReadRequiredFiles_ExcludesReadFile(t *testing.T) {
	repoRoot := t.TempDir()
	writePreReadTempFile(t, repoRoot, "internal/foo.go", "package foo\n")
	writePreReadTempFile(t, repoRoot, "internal/bar.go", "package bar\n")
	excludeRead := map[string]bool{"internal/foo.go": true}
	got := preReadRequiredFiles(repoRoot, []string{"internal/foo.go", "internal/bar.go"}, 2, 100, excludeRead)
	if strings.Contains(got, "internal/foo.go") {
		t.Errorf("foo.go is in excludeRead; should not be injected. Got: %q", got)
	}
	if !strings.Contains(got, "internal/bar.go") {
		t.Errorf("bar.go should be injected (not in excludeRead). Got: %q", got)
	}
}

func TestPreReadRequiredFiles_ExcludeAllFiles_ReturnsEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	writePreReadTempFile(t, repoRoot, "a.go", "package a\n")
	writePreReadTempFile(t, repoRoot, "b.go", "package b\n")
	excludeRead := map[string]bool{"a.go": true, "b.go": true}
	got := preReadRequiredFiles(repoRoot, []string{"a.go", "b.go"}, 5, 100, excludeRead)
	if got != "" {
		t.Errorf("all files excluded; should return empty. Got: %q", got)
	}
}

func TestPreReadRequiredFiles_CanonicalizesPaths(t *testing.T) {
	// excludeRead lookup uses canonicalExplorerPath (forward-slash,
	// no leading "./"). Ensure paths emitted with backslashes or "./"
	// prefix are still matched.
	repoRoot := t.TempDir()
	writePreReadTempFile(t, repoRoot, "x/y/z.go", "package z\n")
	excludeRead := map[string]bool{"x/y/z.go": true}
	got := preReadRequiredFiles(repoRoot, []string{"./x/y/z.go"}, 2, 100, excludeRead)
	if got != "" {
		t.Errorf("./prefix should canonicalize away; expected exclusion. Got: %q", got)
	}
}

func TestPreReadRequiredFiles_HonorsMaxFiles(t *testing.T) {
	// With excludeRead in play, the maxFiles cap counts SUCCESSFUL
	// injections only — skipped files don't consume the slot.
	repoRoot := t.TempDir()
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		writePreReadTempFile(t, repoRoot, f, "package p\n")
	}
	excludeRead := map[string]bool{"a.go": true} // skip first
	got := preReadRequiredFiles(repoRoot, []string{"a.go", "b.go", "c.go"}, 2, 100, excludeRead)
	if !strings.Contains(got, "b.go") || !strings.Contains(got, "c.go") {
		t.Errorf("after skipping a.go, b.go and c.go should both be injected (maxFiles=2). Got: %q", got)
	}
	if strings.Contains(got, "a.go") {
		t.Errorf("a.go was excluded; should not appear. Got: %q", got)
	}
}

// === excludeReadFromCtx ===

func TestExcludeReadFromCtx_NilCtx_ReturnsNil(t *testing.T) {
	if got := excludeReadFromCtx(nil); got != nil {
		t.Errorf("nil ctx: expected nil; got %v", got)
	}
}

func TestExcludeReadFromCtx_NilMutable_ReturnsNil(t *testing.T) {
	ctx := &types.AgentContext{}
	if got := excludeReadFromCtx(ctx); got != nil {
		t.Errorf("nil Mutable: expected nil; got %v", got)
	}
}

func TestExcludeReadFromCtx_PopulatedReadSet_CanonicalizesEntries(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"./internal/foo.go":  true,
		`internal\bar.go`:    true, // Windows-shaped (canonical fn handles this)
		"internal/baz.go":    true,
	})
	ctx := &types.AgentContext{Mutable: mut}
	got := excludeReadFromCtx(ctx)
	if got == nil {
		t.Fatal("populated ReadSet should produce a non-nil exclude map")
	}
	wantKeys := []string{"internal/foo.go", "internal/bar.go", "internal/baz.go"}
	for _, k := range wantKeys {
		if !got[k] {
			t.Errorf("expected canonical key %q in exclude map; have %v", k, got)
		}
	}
}

func TestExcludeReadFromCtx_EmptyReadSet_ReturnsNil(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{})
	ctx := &types.AgentContext{Mutable: mut}
	if got := excludeReadFromCtx(ctx); got != nil {
		t.Errorf("empty ReadSet should yield nil; got %v", got)
	}
}
