package multigraph

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// gateTestMG builds a multi-repo MultiGraph against an in-memory
// topology. PendingSubRepos = []string passed via BusContext drives
// the active/inactive split.
func gateTestMG(t *testing.T, repos []topology.SubRepo) *MultiGraph {
	t.Helper()
	topo := &topology.RepoTopology{Repos: repos}
	mg, err := New(Config{
		Topology: topo,
		Build: func(_, _ string) (*rmtypes.Graph, error) { // unused in gate tests
			return &rmtypes.Graph{}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return mg
}

func TestResolveActiveSetPath_SingleRepoBypass(t *testing.T) {
	// Single-repo (one entry, RootRel="."): the gate must let any
	// path through untouched so single-repo callers stay byte-
	// identical.
	mg := gateTestMG(t, []topology.SubRepo{{Slug: "self", RootRel: "."}})
	ctx := &types.BusContext{MultiGraph: mg}
	res := mg.ResolveActiveSetPath(ctx, "read_file", "internal/foo.go", nil)
	if !res.Allowed {
		t.Errorf("single-repo must bypass gate, got refusal: %s", res.RefusalProse)
	}
	if res.ResolvedPath != "internal/foo.go" {
		t.Errorf("single-repo bypass should not modify path, got %q", res.ResolvedPath)
	}
}

func TestResolveActiveSetPath_RefusesParentWide(t *testing.T) {
	mg := gateTestMG(t, []topology.SubRepo{
		{Slug: "a", RootRel: "repo-a"},
		{Slug: "b", RootRel: "repo-b"},
	})
	ctx := &types.BusContext{MultiGraph: mg}
	for _, llmPath := range []string{"", ".", "./"} {
		t.Run(llmPath, func(t *testing.T) {
			res := mg.ResolveActiveSetPath(ctx, "repo_map", llmPath, nil)
			if res.Allowed {
				t.Errorf("parent-wide path %q must be refused", llmPath)
			}
			if !strings.Contains(res.RefusalProse, "scan the workspace's parent") {
				t.Errorf("refusal prose missing parent-wide explanation: %q", res.RefusalProse)
			}
			if !strings.Contains(res.RefusalProse, "repo-a") || !strings.Contains(res.RefusalProse, "repo-b") {
				t.Errorf("refusal must list active sub-repos: %q", res.RefusalProse)
			}
		})
	}
}

func TestResolveActiveSetPath_AllowsActivePrefix(t *testing.T) {
	mg := gateTestMG(t, []topology.SubRepo{
		{Slug: "a", RootRel: "repo-a"},
		{Slug: "b", RootRel: "repo-b"},
	})
	ctx := &types.BusContext{MultiGraph: mg}
	res := mg.ResolveActiveSetPath(ctx, "read_file", "repo-a/svc/main.go", nil)
	if !res.Allowed {
		t.Errorf("active-prefixed path must be allowed: %s", res.RefusalProse)
	}
	if res.SubRepoRootRel != "repo-a" {
		t.Errorf("expected SubRepoRootRel=repo-a, got %q", res.SubRepoRootRel)
	}
	if res.AutoPrefixed {
		t.Errorf("path was already prefixed; AutoPrefixed must be false")
	}
}

func TestResolveActiveSetPath_RefusesInactivePrefix(t *testing.T) {
	mg := gateTestMG(t, []topology.SubRepo{
		{Slug: "a", RootRel: "repo-a"},
		{Slug: "b", RootRel: "repo-b"},
	})
	ctx := &types.BusContext{
		MultiGraph:      mg,
		PendingSubRepos: []string{"repo-b"}, // repo-b inactive
	}
	res := mg.ResolveActiveSetPath(ctx, "grep", "repo-b/svc/main.py", nil)
	if res.Allowed {
		t.Fatalf("path inside inactive sub-repo must be refused")
	}
	if !strings.Contains(res.RefusalProse, "currently outside the active") {
		t.Errorf("refusal prose missing inactive marker: %q", res.RefusalProse)
	}
	if !strings.Contains(res.RefusalProse, "repo-b") {
		t.Errorf("refusal should name the inactive sub-repo: %q", res.RefusalProse)
	}
	if !strings.Contains(res.RefusalProse, "repo-a") {
		t.Errorf("refusal should list the active sub-repo (repo-a): %q", res.RefusalProse)
	}
}

func TestResolveActiveSetPath_AutoPrefixUniqueMatch(t *testing.T) {
	// Bare path "svc/main.go" with fileExists letting only repo-a
	// pass — gate must auto-prefix to repo-a.
	mg := gateTestMG(t, []topology.SubRepo{
		{Slug: "a", RootRel: "repo-a", RootAbs: "/tmp/test/repo-a"},
		{Slug: "b", RootRel: "repo-b", RootAbs: "/tmp/test/repo-b"},
	})
	ctx := &types.BusContext{MultiGraph: mg}
	probe := func(abs string) bool { return abs == "/tmp/test/repo-a/svc/main.go" }
	res := mg.ResolveActiveSetPath(ctx, "read_file", "svc/main.go", probe)
	if !res.Allowed {
		t.Fatalf("unique auto-prefix match must be allowed: %s", res.RefusalProse)
	}
	if res.ResolvedPath != "repo-a/svc/main.go" {
		t.Errorf("expected auto-prefixed repo-a/svc/main.go, got %q", res.ResolvedPath)
	}
	if !res.AutoPrefixed {
		t.Errorf("AutoPrefixed should be true on bare-path match")
	}
}

func TestResolveActiveSetPath_AutoPrefixAmbiguousRefused(t *testing.T) {
	mg := gateTestMG(t, []topology.SubRepo{
		{Slug: "a", RootRel: "repo-a", RootAbs: "/tmp/test/repo-a"},
		{Slug: "b", RootRel: "repo-b", RootAbs: "/tmp/test/repo-b"},
	})
	ctx := &types.BusContext{MultiGraph: mg}
	probeAll := func(_ string) bool { return true }
	res := mg.ResolveActiveSetPath(ctx, "read_file", "svc/main.go", probeAll)
	if res.Allowed {
		t.Fatalf("ambiguous bare path must be refused")
	}
	if !strings.Contains(res.RefusalProse, "could resolve into multiple") {
		t.Errorf("refusal prose missing ambiguity hint: %q", res.RefusalProse)
	}
	if !strings.Contains(res.RefusalProse, "repo-a/svc/main.go") || !strings.Contains(res.RefusalProse, "repo-b/svc/main.go") {
		t.Errorf("refusal should list both candidates: %q", res.RefusalProse)
	}
}

func TestResolveActiveSetPath_BareNoMatchRefused(t *testing.T) {
	mg := gateTestMG(t, []topology.SubRepo{
		{Slug: "a", RootRel: "repo-a", RootAbs: "/tmp/test/repo-a"},
		{Slug: "b", RootRel: "repo-b", RootAbs: "/tmp/test/repo-b"},
	})
	ctx := &types.BusContext{MultiGraph: mg}
	probeNone := func(_ string) bool { return false }
	res := mg.ResolveActiveSetPath(ctx, "read_file", "missing/file.go", probeNone)
	if res.Allowed {
		t.Fatalf("bare path matching no active sub-repo must be refused")
	}
	if !strings.Contains(res.RefusalProse, "was not found inside any active") {
		t.Errorf("refusal prose missing no-match marker: %q", res.RefusalProse)
	}
}

func TestResolveActiveSetPath_DirToolNilProbeRefusesAmbiguous(t *testing.T) {
	// repo_map / grep / list_files pass fileExists=nil. Bare path
	// against multiple active sub-repos resolves as ambiguous —
	// the gate forces the LLM to specify a prefix.
	mg := gateTestMG(t, []topology.SubRepo{
		{Slug: "a", RootRel: "repo-a"},
		{Slug: "b", RootRel: "repo-b"},
	})
	ctx := &types.BusContext{MultiGraph: mg}
	res := mg.ResolveActiveSetPath(ctx, "grep", "src/", nil)
	if res.Allowed {
		t.Fatalf("nil-probe bare path against 2 active sub-repos must be refused as ambiguous")
	}
	if !strings.Contains(res.RefusalProse, "could resolve into multiple") {
		t.Errorf("refusal must mention ambiguity: %q", res.RefusalProse)
	}
}

func TestResolveActiveSetPath_BackslashAcceptedAsWindowsPath(t *testing.T) {
	mg := gateTestMG(t, []topology.SubRepo{
		{Slug: "a", RootRel: "repo-a"},
		{Slug: "b", RootRel: "repo-b"},
	})
	ctx := &types.BusContext{MultiGraph: mg}
	res := mg.ResolveActiveSetPath(ctx, "read_file", `repo-a\svc\main.go`, nil)
	if !res.Allowed {
		t.Fatalf("backslash-form active-prefixed path must be allowed: %s", res.RefusalProse)
	}
	if res.SubRepoRootRel != "repo-a" {
		t.Errorf("expected SubRepoRootRel=repo-a, got %q", res.SubRepoRootRel)
	}
}
