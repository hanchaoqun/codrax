package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC258RecoveryNavigationPublicReadCannotReenterEnrichment(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "accepted"}[complete], func(t *testing.T) {
			root := t.TempDir()
			catalog, err := tracecatalog.Discover(context.Background(), root, tracecatalog.DiscoverOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if err := catalog.Save(context.Background(), filepath.Join(root, "settings.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "workers.json"), []byte(`{"workers":8}`), 0600); err != nil {
				t.Fatal(err)
			}
			mut := types.NewMutableState("inspect records")
			ir := &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentConfigQuery,
				RuntimeQuestionProfile:          &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{}}}
			bus := &types.BusContext{RepoRoot: root, WorkDir: t.TempDir(), Mutable: mut, AnalysisIR: ir}
			var results []types.ToolResult
			for _, path := range []string{"settings.json", "workers.json"} {
				raw, _ := json.Marshal(map[string]any{"path": path})
				out, err := (&tool.ReadFile{}).Execute(bus, raw)
				if err != nil || !out.Success {
					t.Fatal(err, out.Summary)
				}
				results = append(results, out)
			}
			if !mut.IsRecoveryNavigationMaterial(root, "settings.json") {
				t.Fatal("public complete read did not qualify navigation")
			}
			// Retained older tool coverage and every source-discovery frontier
			// must obey the same qualification, including concrete-value cache.
			results = append(results, types.ToolResult{ToolName: "read_file", Success: true, ReadCoverage: &types.ToolReadCoverage{Path: "settings.json", LineStart: 1, LineEnd: 999}})
			graph := repomap.BuildGraph(root, []*repotypes.FileInfo{{RelPath: "settings.json"}, {RelPath: "workers.json"}})
			e := &explorerEvaluator{mutable: mut, repoRoot: root, analysisIR: ir, searchResult: &keywordSearchResult{Graph: graph},
				investigationComplete: complete, requiredFiles: []string{"settings.json"}, exactAnchorFiles: []string{"settings.json"}}
			readSet := map[string]bool{"settings.json": true, "workers.json": true}
			selected := e.deterministicEnrichmentFileSet(readSet, "")
			if selected["settings.json"] || !selected["workers.json"] {
				t.Fatalf("bad candidate selection: %v", selected)
			}
			ctx := &types.AgentContext{RepoRoot: root, WorkDir: bus.WorkDir, Mutable: mut, AnalysisIR: ir}
			e.ensureStructuredEvidence(ctx, results)
			seenSource := false
			for _, item := range e.structuredEvidence {
				if item.Source == "settings.json" {
					t.Fatalf("navigation reentered dataflow: %+v", item)
				}
				if item.Source == "workers.json" {
					seenSource = true
				}
			}
			if !seenSource {
				t.Fatal("ordinary config did not traverse deterministic enrichment")
			}
			cv := e.getConcreteValuesCached(context.Background(), root, readSet, nil)
			for _, item := range cv.evidence {
				if item.Source == "settings.json" {
					t.Fatalf("navigation reentered concrete-value cache: %+v", item)
				}
			}
		})
	}
}
