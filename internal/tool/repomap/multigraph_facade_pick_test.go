package repomap

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
)

func pickFixtureMultiGraph(t *testing.T) (*multigraph.MultiGraph, *topology.RepoTopology) {
	t.Helper()
	topo := &topology.RepoTopology{
		ParentRoot: t.TempDir(),
		Repos: []topology.SubRepo{
			{Slug: "greet-go", RootRel: "repo-greet-go", FileCount: 3},
			{Slug: "stub-rust", RootRel: "repo-stub-rust", FileCount: 2},
			{Slug: "tools-py", RootRel: "repo-tools-py", FileCount: 2},
		},
	}
	mg, err := multigraph.New(multigraph.Config{
		Topology: topo,
		Build: func(root, query string) (*Graph, error) {
			return BuildGraph(root, nil), nil
		},
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	return mg, topo
}

// Batch-6 C1: the parent-graph compatibility fallback must honour the
// question's routed active set — when routing left repo-greet-go and
// repo-tools-py inactive, the fallback picks the active sub-repo, not
// the largest-by-filecount out-of-set one (the live mr_focus_single
// shape: a Go overview injected into a Rust question's prompt).
func TestPickPrimarySubRepo_RoutedActiveSetWins(t *testing.T) {
	mg, topo := pickFixtureMultiGraph(t)
	got := pickPrimarySubRepo(mg, topo, []string{"repo-greet-go", "repo-tools-py"})
	if got.RootRel != "repo-stub-rust" {
		t.Fatalf("picked %s, want the routed-active repo-stub-rust", got.RootRel)
	}
}

// No routing decision (empty pending) keeps the historical
// largest-by-filecount behaviour.
func TestPickPrimarySubRepo_NoRoutingKeepsLargest(t *testing.T) {
	mg, topo := pickFixtureMultiGraph(t)
	got := pickPrimarySubRepo(mg, topo, nil)
	if got.RootRel != "repo-greet-go" {
		t.Fatalf("picked %s, want largest repo-greet-go", got.RootRel)
	}
}

// Defensive: every sub-repo pending falls back to largest instead of nil.
func TestPickPrimarySubRepo_AllPendingFallsBack(t *testing.T) {
	mg, topo := pickFixtureMultiGraph(t)
	got := pickPrimarySubRepo(mg, topo, []string{"repo-greet-go", "repo-stub-rust", "repo-tools-py"})
	if got == nil || got.RootRel != "repo-greet-go" {
		t.Fatalf("all-pending must fall back to largest, got %+v", got)
	}
}
