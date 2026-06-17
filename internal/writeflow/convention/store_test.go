package convention

import (
	"os"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestStoreMergeAndSavePersistsConventionGraph(t *testing.T) {
	store := NewStore(t.TempDir())
	graph := types.ConventionGraph{
		BatchID: "batch-1",
		Goal:    "repair callback",
		Nodes: []types.ConventionNode{{
			Category:     types.ConventionCategoryMechanism,
			Summary:      "callbacks stay inside the emit guard",
			Source:       "pkg/a.py",
			LineStart:    12,
			AnchorSymbol: "set_lim",
		}, {
			Category:     types.ConventionCategoryMechanism,
			Summary:      "callbacks stay inside the emit guard",
			Source:       "pkg/a.py",
			LineStart:    12,
			AnchorSymbol: "set_lim",
		}},
	}

	merged, path, err := store.MergeAndSave("wf-1", graph)
	if err != nil {
		t.Fatalf("MergeAndSave: %v", err)
	}
	if len(merged.Nodes) != 1 {
		t.Fatalf("expected deduped graph, got %+v", merged.Nodes)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("convention graph was not persisted: %v", err)
	}
	loaded, err := store.Load("wf-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Nodes) != 1 || loaded.Nodes[0].Summary != "callbacks stay inside the emit guard" {
		t.Fatalf("loaded graph mismatch: %+v", loaded)
	}
}

func TestSelectTopNPrefersActivePathAndSymbol(t *testing.T) {
	graph := types.NormalizeConventionGraph(types.ConventionGraph{
		BatchID: "batch-1",
		Nodes: []types.ConventionNode{{
			Category: types.ConventionCategoryLocalPattern,
			Summary:  "generic local pattern",
			Source:   "pkg/z.py",
		}, {
			Category:     types.ConventionCategoryMechanism,
			Summary:      "active symbol convention",
			Source:       "pkg/b.py",
			AnchorSymbol: "target",
		}, {
			Category: types.ConventionCategoryMechanism,
			Summary:  "active path convention",
			Source:   "pkg/a.py",
		}},
	})

	selected := SelectTopN(graph, []string{"pkg/a.py"}, []string{"target"}, 2)
	if len(selected.Nodes) != 2 {
		t.Fatalf("selected nodes = %d, want 2", len(selected.Nodes))
	}
	if selected.Nodes[0].Summary != "active path convention" ||
		selected.Nodes[1].Summary != "active symbol convention" {
		t.Fatalf("top-n did not prefer active scope: %+v", selected.Nodes)
	}
}
