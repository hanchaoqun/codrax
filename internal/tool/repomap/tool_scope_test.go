package repomap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
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

func TestRepoMapExecuteReusesMutableSearchGraph(t *testing.T) {
	repo := t.TempDir()
	mut := types.NewMutableState("repo map reuse")
	mut.SetSearchGraph(BuildGraph(repo, []*FileInfo{{
		RelPath:  "virtual.go",
		Language: "go",
		Size:     12,
	}}))
	ctx := &types.BusContext{RepoRoot: repo, Mutable: mut}

	res, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":".","view":"overview","query":"virtual"}`))
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected repo_map to reuse the in-memory graph instead of scanning the empty repo: %+v", res)
	}
}

func TestRepoMapExecuteMultiRepoHonorsEachExplicitSubRepoPath(t *testing.T) {
	parent := t.TempDir()
	baseRoot := filepath.Join(parent, "frameworks", "base")
	resschedRoot := filepath.Join(parent, "hm_z", "foundation", "resourceschedule", "ressched")
	if err := os.MkdirAll(baseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resschedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mg := newTestMultiGraph(t, parent, baseRoot, resschedRoot)
	ctx := &types.BusContext{RepoRoot: parent, MultiGraph: mg}

	base, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":"frameworks/base","view":"overview"}`))
	if err != nil {
		t.Fatalf("base repo_map returned error: %v", err)
	}
	if !base.Success {
		t.Fatalf("base repo_map failed: %+v", base)
	}
	ressched, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":"hm_z/foundation/resourceschedule/ressched","view":"overview"}`))
	if err != nil {
		t.Fatalf("ressched repo_map returned error: %v", err)
	}
	if !ressched.Success {
		t.Fatalf("ressched repo_map failed: %+v", ressched)
	}

	if !strings.Contains(base.Summary, "- java: 1 files") {
		t.Fatalf("base overview should describe the base graph, got:\n%s", base.Summary)
	}
	if strings.Contains(base.Summary, "- cpp: 1 files") {
		t.Fatalf("base overview leaked the ressched graph:\n%s", base.Summary)
	}
	if !strings.Contains(ressched.Summary, "- cpp: 1 files") {
		t.Fatalf("ressched overview should describe the ressched graph, got:\n%s", ressched.Summary)
	}
	if strings.Contains(ressched.Summary, "- java: 1 files") {
		t.Fatalf("ressched overview leaked the base graph:\n%s", ressched.Summary)
	}
}

func TestGraphFromAgentContextOrLoadMultiRepoHonorsExplicitSubRepoRoot(t *testing.T) {
	parent := t.TempDir()
	baseRoot := filepath.Join(parent, "frameworks", "base")
	resschedRoot := filepath.Join(parent, "hm_z", "foundation", "resourceschedule", "ressched")
	if err := os.MkdirAll(baseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resschedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mg := newTestMultiGraph(t, parent, baseRoot, resschedRoot)
	ctx := &types.AgentContext{RepoRoot: parent, MultiGraph: mg}

	graph, err := GraphFromAgentContextOrLoad(ctx, resschedRoot, "ressched")
	if err != nil {
		t.Fatalf("GraphFromAgentContextOrLoad returned error: %v", err)
	}
	if graph == nil {
		t.Fatal("GraphFromAgentContextOrLoad returned nil graph")
	}
	if !sameRepoMapRoot(graph.Root, resschedRoot) {
		t.Fatalf("explicit ressched root was routed to %q", graph.Root)
	}
	if graph.Metadata.Languages["cpp"] != 1 || graph.Metadata.Languages["java"] != 0 {
		t.Fatalf("explicit ressched root got wrong graph metadata: %+v", graph.Metadata.Languages)
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

func newTestMultiGraph(t *testing.T, parent, baseRoot, resschedRoot string) *multigraph.MultiGraph {
	t.Helper()
	topo := &topology.RepoTopology{
		ParentRoot: parent,
		Repos: []topology.SubRepo{
			{
				Slug:      "base-10eb2a5e",
				RootAbs:   baseRoot,
				RootRel:   "frameworks/base",
				FileCount: 41530,
			},
			{
				Slug:      "ressched-c088d3ed",
				RootAbs:   resschedRoot,
				RootRel:   "hm_z/foundation/resourceschedule/ressched",
				FileCount: 259,
			},
		},
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(root, query string) (*Graph, error) {
			switch {
			case sameRepoMapRoot(root, baseRoot):
				return BuildGraph(root, []*FileInfo{{
					RelPath:  "core/Foo.java",
					Language: "java",
					Size:     1,
				}}), nil
			case sameRepoMapRoot(root, resschedRoot):
				return BuildGraph(root, []*FileInfo{{
					RelPath:  "services/Scheduler.cpp",
					Language: "cpp",
					Size:     1,
				}}), nil
			default:
				t.Fatalf("unexpected graph root %q", root)
				return nil, nil
			}
		},
		Cap: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return mg
}
