package repomap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestResolveRepoMapRootScopedRejectsParentEscape(t *testing.T) {
	repo := t.TempDir()

	if _, err := resolveRepoMapRootScoped("..", repo, repo); err == nil {
		t.Fatal("expected parent escape to be rejected")
	}
	if _, err := resolveRepoMapRootScoped(filepath.Join("src", "..", ".."), repo, repo); err == nil {
		t.Fatal("expected normalized parent escape to be rejected")
	}
}

func TestResolveRepoMapRootScopedRejectsAbsoluteOutside(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveRepoMapRootScoped(outside, repo, repo); err == nil {
		t.Fatal("expected absolute path outside the repo to be rejected")
	}
}

func TestResolveRepoMapRootScopedRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows builders")
	}
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "escape")); err != nil {
		t.Skipf("cannot create symlink on this filesystem: %v", err)
	}

	if _, err := resolveRepoMapRootScoped("escape", repo, repo); err == nil {
		t.Fatal("expected symlink escaping the repo to be rejected")
	}
}

func TestResolveRepoMapRootScopedAllowsChildren(t *testing.T) {
	repo := t.TempDir()

	resolved, err := resolveRepoMapRootScoped(filepath.Join("src", "pkg"), repo, repo)
	if err != nil {
		t.Fatalf("expected child path to be allowed: %v", err)
	}
	want := filepath.Join(repo, "src", "pkg")
	if resolved != want {
		t.Fatalf("resolved path mismatch: got %q want %q", resolved, want)
	}
}

func TestResolveRepoMapRootScopedHonorsActiveSubRepoScope(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "repo-a")
	if err := os.Mkdir(active, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "repo-b"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveRepoMapRootScoped(filepath.Join("repo-a", "..", "repo-b"), root, active); err == nil {
		t.Fatal("expected escape from active sub-repo to be rejected")
	}
	if _, err := resolveRepoMapRootScoped(filepath.Join("repo-a", "src"), root, active); err != nil {
		t.Fatalf("expected path under active sub-repo to be allowed: %v", err)
	}
}

func TestRepoMapExecuteRejectsParentEscapeBeforeScan(t *testing.T) {
	repo := t.TempDir()
	ctx := &types.BusContext{RepoRoot: repo}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":".."}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected repo_map to refuse parent escape, got success: %+v", res)
	}
	if !strings.Contains(res.Summary, "outside the current repository scope") {
		t.Fatalf("unexpected refusal summary: %q", res.Summary)
	}
}

func TestBuildOrLoadGraphWithinRejectsBeforeScanner(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := BuildOrLoadGraphWithin(outside, repo, ""); err == nil {
		t.Fatal("expected scoped graph loader to reject before scanning outside root")
	}
}
