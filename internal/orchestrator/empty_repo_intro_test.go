package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDirIsEffectivelyEmpty_TrueOnFreshTempDir locks the canonical
// "no source files anywhere" case. t.TempDir() returns a brand new
// directory the OS just created — nothing in it.
func TestDirIsEffectivelyEmpty_TrueOnFreshTempDir(t *testing.T) {
	if !dirIsEffectivelyEmpty(t.TempDir()) {
		t.Error("fresh tmp dir must be effectively empty")
	}
}

// TestDirIsEffectivelyEmpty_FalseOnSingleFile pins the negative
// case: a single source file at the root counts.
func TestDirIsEffectivelyEmpty_FalseOnSingleFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if dirIsEffectivelyEmpty(root) {
		t.Error("dir with main.go must NOT be effectively empty")
	}
}

// TestDirIsEffectivelyEmpty_FalseOnNestedFile pins recursive walk:
// a file three levels deep still flips the predicate to false.
func TestDirIsEffectivelyEmpty_FalseOnNestedFile(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deep, "x.py"), []byte("# x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if dirIsEffectivelyEmpty(root) {
		t.Error("dir with nested x.py must NOT be effectively empty")
	}
}

// TestDirIsEffectivelyEmpty_TrueWhenOnlyGitDir locks that a freshly
// `git init`'d repo (just .git/ inside) still counts as empty for
// pipeline purposes — there's nothing to analyse / plan against.
func TestDirIsEffectivelyEmpty_TrueWhenOnlyGitDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "refs"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("seed .git/HEAD: %v", err)
	}
	if !dirIsEffectivelyEmpty(root) {
		t.Error("dir whose only content is .git/ must be effectively empty for pipeline purposes")
	}
}

// TestDirIsEffectivelyEmpty_TrueWhenOnlyCodraxDir mirrors the .git
// exclusion for the .codrax runtime dir (logs, blobs, plans). A
// dir whose only content is the codrax-managed runtime data is
// "effectively empty" because the analyzer / planner cannot do
// anything with it.
func TestDirIsEffectivelyEmpty_TrueWhenOnlyCodraxDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codrax", "logs"), 0o755); err != nil {
		t.Fatalf("mkdir .codrax: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codrax", "logs", "x.log"), []byte("noise\n"), 0o644); err != nil {
		t.Fatalf("seed .codrax: %v", err)
	}
	if !dirIsEffectivelyEmpty(root) {
		t.Error("dir whose only content is .codrax/ must be effectively empty for pipeline purposes")
	}
}

// TestEmptyRepoReadIntro_PlainLanguage locks the user-facing message
// shape: contains the directory path, the three concrete next
// actions, and no internal jargon (no internal stage names, no
// project brand, no implementation terms).
func TestEmptyRepoReadIntro_PlainLanguage(t *testing.T) {
	got := emptyRepoReadIntro("zh", "/some/dir")
	if !strings.Contains(got, "/some/dir") {
		t.Errorf("must name the directory path; got %q", got)
	}
	if !strings.Contains(got, "源代码") {
		t.Errorf("must explain the no-source observation; got %q", got)
	}
	if !strings.Contains(got, "--auto-init-repo") {
		t.Errorf("must surface the from-scratch action with the CLI flag; got %q", got)
	}
	if !strings.Contains(got, "/chat") {
		t.Errorf("must surface the chat alternative; got %q", got)
	}
	// Defense against re-leaking jargon: assert internal stage
	// names + project brand are absent. These tokens appear in code
	// comments and skill prompts, but should not bleed into user
	// surface text per the 2026-04-29 directive.
	for _, banned := range []string{"orchestrator", "BusContext", "ChangePlan",
		"PipelineStage", "codrax 会"} {
		if strings.Contains(got, banned) {
			t.Errorf("plain-language directive: %q must NOT appear in user message; got %q", banned, got)
		}
	}
}

// TestEmptyRepoReadIntro_EnglishFallback covers non-zh languages.
func TestEmptyRepoReadIntro_EnglishFallback(t *testing.T) {
	got := emptyRepoReadIntro("en", "/x/y")
	if !strings.Contains(got, "/x/y") {
		t.Errorf("must name the directory path; got %q", got)
	}
	if !strings.Contains(got, "no source files") {
		t.Errorf("must explain the no-source observation; got %q", got)
	}
	if !strings.Contains(got, "--auto-init-repo") {
		t.Errorf("must surface the from-scratch action; got %q", got)
	}
}
