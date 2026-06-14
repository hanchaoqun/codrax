package orchestrator

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestConfigureWriteScopedMultiRepoSwitchesToSingleActiveSubRepo(t *testing.T) {
	parent := t.TempDir()
	bindingsRoot := filepath.Join(parent, "bindings-py")
	clientRoot := filepath.Join(parent, "client-ts")
	mg := testMultiGraph(t, parent, []topology.SubRepo{
		{Slug: "bindings-py", RootRel: "bindings-py", RootAbs: bindingsRoot},
		{Slug: "client-ts", RootRel: "client-ts", RootAbs: clientRoot},
	})
	mu := types.NewMutableState("patch python bindings")
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:         mu,
			Mode:            types.ModeApply,
			RepoRoot:        parent,
			MainRepoRoot:    parent,
			MultiGraph:      mg,
			SubRepos:        testSubRepoSnapshots(bindingsRoot, clientRoot),
			PendingSubRepos: []string{"client-ts"},
		},
	}

	if err := o.configureWriteScopedMultiRepo(parent); err != nil {
		t.Fatalf("configureWriteScopedMultiRepo: %v", err)
	}
	if o.busCtx.RepoRoot != bindingsRoot || o.busCtx.MainRepoRoot != bindingsRoot {
		t.Fatalf("roots not scoped: repo=%q main=%q want %q", o.busCtx.RepoRoot, o.busCtx.MainRepoRoot, bindingsRoot)
	}
	if o.busCtx.ActiveSubRepo == nil || o.busCtx.ActiveSubRepo.RootRel != "bindings-py" {
		t.Fatalf("ActiveSubRepo not set: %+v", o.busCtx.ActiveSubRepo)
	}
	if o.busCtx.MultiGraph != nil {
		t.Fatalf("repo-scoped write must disable parent MultiGraph for file tools")
	}
	if len(o.busCtx.PendingSubRepos) != 0 {
		t.Fatalf("PendingSubRepos should be cleared after scoping, got %+v", o.busCtx.PendingSubRepos)
	}
	if got := mu.RepoRoot(); got != bindingsRoot {
		t.Fatalf("Mutable repo root = %q, want %q", got, bindingsRoot)
	}
}

func TestConfigureWriteScopedMultiRepoRequiresExactlyOneActiveSubRepo(t *testing.T) {
	parent := t.TempDir()
	bindingsRoot := filepath.Join(parent, "bindings-py")
	clientRoot := filepath.Join(parent, "client-ts")
	mg := testMultiGraph(t, parent, []topology.SubRepo{
		{Slug: "bindings-py", RootRel: "bindings-py", RootAbs: bindingsRoot},
		{Slug: "client-ts", RootRel: "client-ts", RootAbs: clientRoot},
	})
	mu := types.NewMutableState("patch workspace")
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:      mu,
			Mode:         types.ModeApply,
			RepoRoot:     parent,
			MainRepoRoot: parent,
			MultiGraph:   mg,
			SubRepos:     testSubRepoSnapshots(bindingsRoot, clientRoot),
		},
	}

	err := o.configureWriteScopedMultiRepo(parent)
	if err == nil {
		t.Fatal("expected multi-repo write without single active sub-repo to fail")
	}
	if !strings.Contains(err.Error(), "requires exactly one active sub-repo") {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.busCtx.RepoRoot != parent || o.busCtx.MainRepoRoot != parent {
		t.Fatalf("roots changed after failed scope: repo=%q main=%q", o.busCtx.RepoRoot, o.busCtx.MainRepoRoot)
	}
}

func TestValidateChangePlanScopeRejectsWorkspacePrefixedTargetsWhenScoped(t *testing.T) {
	bus := &types.BusContext{
		SubRepos: testSubRepoSnapshots("/workspace/bindings-py", "/workspace/client-ts"),
		ActiveSubRepo: &types.SubRepoSnapshot{
			Slug:    "bindings-py",
			RootRel: "bindings-py",
			RootAbs: "/workspace/bindings-py",
		},
	}
	okPlan := &types.ChangePlan{TargetPaths: []string{"fastlex/tokenizer.py"}}
	if v := ValidateChangePlanScope(bus, okPlan); v != nil {
		t.Fatalf("sub-repo-relative target should pass, got %+v", v)
	}

	for _, target := range []string{"bindings-py/fastlex/tokenizer.py", "client-ts/src/index.ts"} {
		plan := &types.ChangePlan{TargetPaths: []string{target}}
		v := ValidateChangePlanScope(bus, plan)
		if v == nil {
			t.Fatalf("workspace-prefixed target %q should fail", target)
		}
		if v.ClusterKey != "write_scoped_sub_repo_prefixed_target" {
			t.Fatalf("cluster key = %q, want scoped prefix violation", v.ClusterKey)
		}
	}
}

func testMultiGraph(t *testing.T, parent string, subs []topology.SubRepo) *multigraph.MultiGraph {
	t.Helper()
	mg, err := multigraph.New(multigraph.Config{
		Topology: &topology.RepoTopology{
			ParentRoot:   parent,
			ParentSlug:   "parent-test",
			Repos:        subs,
			DiscoveredAt: time.Now(),
		},
		Build: func(repoRoot, query string) (*rmtypes.Graph, error) {
			return &rmtypes.Graph{Root: repoRoot}, nil
		},
		Cap: 1,
	})
	if err != nil {
		t.Fatalf("multigraph.New: %v", err)
	}
	return mg
}

func testSubRepoSnapshots(bindingsRoot, clientRoot string) []types.SubRepoSnapshot {
	return []types.SubRepoSnapshot{
		{Slug: "bindings-py", RootRel: "bindings-py", RootAbs: bindingsRoot, PrimaryLangs: []string{"python"}},
		{Slug: "client-ts", RootRel: "client-ts", RootAbs: clientRoot, PrimaryLangs: []string{"typescript"}},
	}
}
