package topology

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo creates a real git repo at path and writes one tracked
// file so `git ls-files` returns a non-empty result. We use real git
// rather than just `mkdir .git/` because Discover uses
// `tool.NewGitCommand` and many subsystems (probeTier1) only work
// against an initialised repo.
func initGitRepo(t *testing.T, path string, trackedFiles map[string]string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = path
		// Suppress global ~/.gitconfig user.name/email prompts.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	for name, content := range trackedFiles {
		full := filepath.Join(path, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

func resolvedAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs %s: %v", p, err)
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		abs = r
	}
	return abs
}

// TestDiscover_SingleRepoFastPath: parent is itself a git repo →
// degenerate Repos = [{RootRel: ".", GitMode: "dir"}].
func TestDiscover_SingleRepoFastPath(t *testing.T) {
	parent := resolvedAbs(t, t.TempDir())
	initGitRepo(t, parent, map[string]string{"main.go": "package main\n"})

	topo, err := Discover(parent, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !topo.IsSingle() {
		t.Fatalf("expected single-repo topology, got %d sub-repos", len(topo.Repos))
	}
	sr := topo.Repos[0]
	if sr.RootRel != "." {
		t.Errorf("RootRel = %q, want \".\"", sr.RootRel)
	}
	if sr.GitMode != "dir" {
		t.Errorf("GitMode = %q, want \"dir\"", sr.GitMode)
	}
	if sr.RootAbs != parent {
		t.Errorf("RootAbs = %q, want %q", sr.RootAbs, parent)
	}
	if sr.Slug == "" {
		t.Errorf("Slug is empty")
	}
}

// TestDiscover_MultiRepoBFS: parent itself is NOT a git repo, three
// nested independent sub-repos discovered.
func TestDiscover_MultiRepoBFS(t *testing.T) {
	parent := resolvedAbs(t, t.TempDir())
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	initGitRepo(t, filepath.Join(parent, "repo-a"), map[string]string{"main.go": "package main\n"})
	initGitRepo(t, filepath.Join(parent, "repo-b"), map[string]string{"src/lib.rs": "pub fn x() {}"})
	initGitRepo(t, filepath.Join(parent, "repo-c", "nested"), map[string]string{"x.py": "x = 1\n"})

	topo, err := Discover(parent, Options{Depth: 4, MinFiles: 1})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if topo.IsSingle() {
		t.Fatalf("expected multi-repo, got single")
	}
	if got := len(topo.Repos); got != 3 {
		var names []string
		for _, r := range topo.Repos {
			names = append(names, r.RootRel)
		}
		t.Fatalf("expected 3 sub-repos, got %d (%v)", got, names)
	}

	// Sorted by RootRel.
	wantRels := []string{"repo-a", "repo-b", "repo-c/nested"}
	for i, r := range topo.Repos {
		if r.RootRel != wantRels[i] {
			t.Errorf("Repos[%d].RootRel = %q, want %q", i, r.RootRel, wantRels[i])
		}
		if r.GitMode != "dir" {
			t.Errorf("Repos[%d].GitMode = %q, want \"dir\"", i, r.GitMode)
		}
		if r.Slug == "" {
			t.Errorf("Repos[%d].Slug is empty", i)
		}
	}
}

// TestDiscover_PruneNestedSubRepo: a sub-repo containing another git
// repo at deeper level → only the shallower one is reported (BFS
// prunes at boundaries).
func TestDiscover_PruneNestedSubRepo(t *testing.T) {
	parent := resolvedAbs(t, t.TempDir())
	initGitRepo(t, filepath.Join(parent, "outer"), map[string]string{"a.go": "package a\n"})
	initGitRepo(t, filepath.Join(parent, "outer", "inner"), map[string]string{"b.go": "package b\n"})

	topo, err := Discover(parent, Options{Depth: 6})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := len(topo.Repos); got != 1 {
		t.Fatalf("expected 1 sub-repo (prune nested), got %d", got)
	}
	if topo.Repos[0].RootRel != "outer" {
		t.Errorf("expected outer, got %q", topo.Repos[0].RootRel)
	}
}

// TestDiscover_SkipExcludedDirs: BFS skips node_modules, .git, etc.
func TestDiscover_SkipExcludedDirs(t *testing.T) {
	parent := resolvedAbs(t, t.TempDir())
	initGitRepo(t, filepath.Join(parent, "node_modules", "fake-pkg"), map[string]string{"x.go": "package x\n"})
	initGitRepo(t, filepath.Join(parent, "real"), map[string]string{"x.go": "package x\n"})

	topo, err := Discover(parent, Options{Depth: 4})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, r := range topo.Repos {
		if r.RootRel != "real" {
			t.Errorf("unexpected sub-repo %q (BFS should skip node_modules)", r.RootRel)
		}
	}
	if len(topo.Repos) != 1 {
		t.Errorf("expected 1 sub-repo, got %d", len(topo.Repos))
	}
}

// TestDiscover_RuntimeAnchorSkip: codrax-self worktree dirs under
// <runtimeAnchor>/worktrees/ are excluded.
func TestDiscover_RuntimeAnchorSkip(t *testing.T) {
	parent := resolvedAbs(t, t.TempDir())
	runtimeAnchor := filepath.Join(parent, ".codrax")
	initGitRepo(t, filepath.Join(runtimeAnchor, "worktrees", "session-123"), map[string]string{"x.go": "package x\n"})
	initGitRepo(t, filepath.Join(parent, "real"), map[string]string{"x.go": "package x\n"})

	topo, err := Discover(parent, Options{Depth: 6, RuntimeAnchor: runtimeAnchor})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(topo.Repos) != 1 || topo.Repos[0].RootRel != "real" {
		t.Errorf("expected only real sub-repo, got %v", topo.Repos)
	}
}

func TestLookup_LongestPrefix(t *testing.T) {
	topo := &RepoTopology{
		Repos: []SubRepo{
			{Slug: "p", RootRel: "."},
			{Slug: "a", RootRel: "frontend"},
			{Slug: "b", RootRel: "frontend/components"},
		},
	}
	cases := []struct {
		rel  string
		want string // expected slug
	}{
		{"frontend/components/x.tsx", "b"},
		{"frontend/x.tsx", "a"},
		{"frontend", "a"},
		{"misc/x.go", "p"},
		{"", "p"},
	}
	for _, c := range cases {
		got := topo.Lookup(c.rel)
		if got == nil {
			t.Errorf("Lookup(%q) = nil, want slug %q", c.rel, c.want)
			continue
		}
		if got.Slug != c.want {
			t.Errorf("Lookup(%q) = %q, want %q", c.rel, got.Slug, c.want)
		}
	}
}

func TestLookup_NoMatchNoCatchAll(t *testing.T) {
	topo := &RepoTopology{
		Repos: []SubRepo{
			{Slug: "a", RootRel: "frontend"},
		},
	}
	if got := topo.Lookup("backend/x.go"); got != nil {
		t.Errorf("Lookup outside any sub-repo should return nil, got %+v", got)
	}
}

func TestSaveLoad_Roundtrip(t *testing.T) {
	anchor := t.TempDir()
	topo := &RepoTopology{
		ParentRoot: "/tmp/parent",
		ParentSlug: "parent-aabbccdd",
		Repos: []SubRepo{
			{Slug: "a-aabbccdd", RootAbs: "/tmp/parent/a", RootRel: "a", GitMode: "dir", PrimaryLangs: []string{"go"}, PrimaryLangsTier: 1, FileCount: 10, ManifestFingerprint: "deadbeef"},
		},
	}
	if err := topo.Save(anchor); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(anchor, "parent-aabbccdd")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil")
	}
	if got.ParentSlug != topo.ParentSlug {
		t.Errorf("ParentSlug roundtrip mismatch")
	}
	if len(got.Repos) != 1 || got.Repos[0].Slug != "a-aabbccdd" {
		t.Errorf("Repos roundtrip mismatch: %+v", got.Repos)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	anchor := t.TempDir()
	got, err := Load(anchor, "nonexistent-deadbeef")
	if err != nil {
		t.Fatalf("Load (missing file): %v", err)
	}
	if got != nil {
		t.Errorf("Load on missing file should return nil, got %+v", got)
	}
}

func TestLoadOrDiscover_CacheFresh(t *testing.T) {
	parent := resolvedAbs(t, t.TempDir())
	initGitRepo(t, parent, map[string]string{"main.go": "package main\n"})
	anchor := filepath.Join(parent, ".codrax")

	first, err := LoadOrDiscover(parent, anchor, Options{}, "")
	if err != nil {
		t.Fatalf("first LoadOrDiscover: %v", err)
	}
	// LoadOrDiscover passes parentSlug. Compute the canonical one.
	parentSlug := first.ParentSlug
	// Re-call with cache available — should hit cache (DiscoveredAt unchanged).
	second, err := LoadOrDiscover(parent, anchor, Options{}, parentSlug)
	if err != nil {
		t.Fatalf("second LoadOrDiscover: %v", err)
	}
	if !second.DiscoveredAt.Equal(first.DiscoveredAt) {
		t.Errorf("cache fresh: expected DiscoveredAt unchanged, got first=%v second=%v",
			first.DiscoveredAt, second.DiscoveredAt)
	}
}

func TestLoadOrDiscover_StaleCacheRediscover(t *testing.T) {
	parent := resolvedAbs(t, t.TempDir())
	initGitRepo(t, parent, map[string]string{"main.go": "package main\n"})
	anchor := filepath.Join(parent, ".codrax")

	first, err := LoadOrDiscover(parent, anchor, Options{}, "")
	if err != nil {
		t.Fatalf("first LoadOrDiscover: %v", err)
	}
	// Force the cached DiscoveredAt to the past so the parent mtime
	// (now) appears newer than the cache. This simulates a real-world
	// "user touched a directory after the cache was written".
	first.DiscoveredAt = first.DiscoveredAt.Add(-1 * 24 * 60 * 60 * 1e9 /* 1 day */)
	if err := first.Save(anchor); err != nil {
		t.Fatalf("Save back-dated: %v", err)
	}

	second, err := LoadOrDiscover(parent, anchor, Options{}, first.ParentSlug)
	if err != nil {
		t.Fatalf("second LoadOrDiscover: %v", err)
	}
	if !second.DiscoveredAt.After(first.DiscoveredAt) {
		t.Errorf("expected fresh re-discovery (newer DiscoveredAt), first=%v second=%v",
			first.DiscoveredAt, second.DiscoveredAt)
	}
}

func TestSubRepoBySlug(t *testing.T) {
	topo := &RepoTopology{
		Repos: []SubRepo{
			{Slug: "a", RootRel: "alpha"},
			{Slug: "b", RootRel: "beta"},
		},
	}
	if got := topo.SubRepoBySlug("b"); got == nil || got.RootRel != "beta" {
		t.Errorf("SubRepoBySlug(b) = %+v, want beta", got)
	}
	if got := topo.SubRepoBySlug("nope"); got != nil {
		t.Errorf("SubRepoBySlug(nope) should be nil")
	}
}
