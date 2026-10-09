package repomap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	"github.com/hanchaoqun/codrax/internal/types"
)

func graphLifecycleRepository(t *testing.T, function string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for rel, body := range map[string]string{
		"pkg/widget/widget.py":   "def " + function + "():\n    return 1\n",
		"pkg/sibling/sibling.py": "def unrelated_sibling():\n    return 2\n",
	} {
		file := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func graphLifecycleQuery(t *testing.T, ctx *types.BusContext, function string) {
	t.Helper()
	result, err := (&RepoMapV2{}).Execute(ctx, json.RawMessage(`{"path":"pkg/widget","view":"source_inventory","scope":".","roles":["function"]}`))
	if err != nil || !result.Success {
		t.Fatalf("scoped public query failed: %+v %v", result, err)
	}
	if !strings.Contains(result.Summary, function) || strings.Contains(result.Summary, "unrelated_sibling") {
		t.Fatalf("selected output changed scope: %s", result.Summary)
	}
	if result.SourceInventory == nil || len(result.SourceInventory.QueryPathScopes) != 1 || result.SourceInventory.QueryPathScopes[0] != "pkg/widget" {
		t.Fatalf("request coordinates lost: %+v", result.SourceInventory)
	}
}

func TestSourceInventorySharedGraphPublicResidentRoots(t *testing.T) {
	for _, carrier := range []string{"mutable", "bus", "multigraph", "no_parent"} {
		t.Run(carrier, func(t *testing.T) {
			root := graphLifecycleRepository(t, "selected_widget")
			ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(), Mutable: types.NewMutableState("inspect selected functions")}
			var expected *Graph
			if carrier == "mutable" || carrier == "bus" {
				var err error
				expected, err = BuildOrLoadGraph(root, "")
				if err != nil {
					t.Fatal(err)
				}
				if carrier == "mutable" {
					ctx.Mutable.SetSearchGraph(expected)
				} else {
					ctx.SearchGraph = expected
				}
			} else {
				mg, err := BuildOrLoadMultiGraph(&topology.RepoTopology{ParentRoot: root, Repos: []topology.SubRepo{{Slug: "root", RootAbs: root, RootRel: "."}}}, "", 2, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				ctx.MultiGraph = mg
				if carrier == "multigraph" {
					expected, err = mg.EnsureLoaded("root")
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			graphLifecycleQuery(t, ctx, "selected_widget")
			if expected == nil {
				if ctx.Mutable.SearchGraph() != nil {
					t.Fatalf("partial lens masqueraded as shared graph: %T", ctx.Mutable.SearchGraph())
				}
				if len(MultiGraphFromContext(ctx).AllGraphs()) != 0 {
					t.Fatal("selected scan loaded unrequested parent")
				}
			} else {
				if ctx.Mutable.SearchGraph() != expected {
					t.Fatal("resident full root was not restored")
				}
				if expected.Root != root || expected.FileIndex["pkg/sibling/sibling.py"] == nil {
					t.Fatal("parent index was mutated by scoped lens")
				}
				key := scopedGraphProjectionCacheKey(root, filepath.Join(root, "pkg/widget"))
				selected, _ := ctx.Mutable.ScopedSearchGraph(key).(*Graph)
				if selected == nil || selected.Root != filepath.Join(root, "pkg/widget") || len(selected.FileIndex) != 1 {
					t.Fatal("scoped navigation cache lost its own coordinates")
				}
			}
		})
	}
}

func TestSourceInventorySharedGraphPublicApplyRootSwitch(t *testing.T) {
	main := graphLifecycleRepository(t, "main_old_widget")
	current := graphLifecycleRepository(t, "current_widget")
	mainGraph, err := BuildOrLoadGraph(main, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: current, MainRepoRoot: main, WorktreePath: current, Mode: types.ModeApply, WorkDir: t.TempDir(), Mutable: types.NewMutableState("inspect worktree")}
	ctx.Mutable.SetSearchGraph(mainGraph)
	graphLifecycleQuery(t, ctx, "current_widget")
	if ctx.Mutable.SearchGraph() != mainGraph || mainGraph.Root != main {
		t.Fatal("main navigation baseline discarded or relabeled as current worktree")
	}
	currentGraph, err := BuildOrLoadGraph(current, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx.SearchGraph = currentGraph
	graphLifecycleQuery(t, ctx, "current_widget")
	if ctx.Mutable.SearchGraph() != currentGraph {
		t.Fatal("current complete graph must take precedence over main baseline")
	}
	ctx.SearchGraph = nil
	ctx.Mutable.SetSearchGraph(mainGraph)
	ctx.WorktreePath = ""
	graphLifecycleQuery(t, ctx, "current_widget")
	if ctx.Mutable.SearchGraph() != nil {
		t.Fatal("MainRepoRoot alone inferred a current-worktree relationship")
	}
}
