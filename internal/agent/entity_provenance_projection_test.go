package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProjectEntityProvenance_ClassifiesTypedSideLane(t *testing.T) {
	graph := &rmtypes.Graph{
		FileIndex: map[string]*rmtypes.FileInfo{
			"internal/foo.go": {RelPath: "internal/foo.go"},
		},
		SymbolDefs: map[string][]*rmtypes.Symbol{
			"Analyzer": {{Name: "Analyzer", File: "internal/foo.go"}},
		},
	}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	mut.AppendPrescanSummary("grep found RetryBudget in internal/retry.go")
	ctx := &types.AgentContext{Mutable: mut}
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Analyzer", "./internal/foo.go", "RetryBudget", "清单"},
		},
		SubTopics: []types.SubTopic{{
			Summary:  "topic",
			Entities: []string{"Analyzer"},
		}},
	}

	projectEntityProvenance(ctx, &rm)

	if len(rm.AnalyzerHints.EntityProvenance) != 4 {
		t.Fatalf("top-level provenance len = %d, got %+v", len(rm.AnalyzerHints.EntityProvenance), rm.AnalyzerHints.EntityProvenance)
	}
	assertEntityProvenance(t, rm.AnalyzerHints.EntityProvenance, "Analyzer", types.EntityResolutionSymbol, true, true, true)
	assertEntityProvenance(t, rm.AnalyzerHints.EntityProvenance, "./internal/foo.go", types.EntityResolutionFile, true, true, true)
	assertEntityProvenance(t, rm.AnalyzerHints.EntityProvenance, "RetryBudget", types.EntityResolutionPrescanAnchor, false, true, false)
	assertEntityProvenance(t, rm.AnalyzerHints.EntityProvenance, "清单", types.EntityResolutionInferredConcept, false, false, false)
	if len(rm.SubTopics) != 1 || len(rm.SubTopics[0].EntityProvenance) != 1 {
		t.Fatalf("sub-topic provenance missing: %+v", rm.SubTopics)
	}
	assertEntityProvenance(t, rm.SubTopics[0].EntityProvenance, "Analyzer", types.EntityResolutionSymbol, true, true, true)
}

func TestProjectEntityProvenance_MultiRepoScopeIsResolved(t *testing.T) {
	mg := scopeFixtureMG(t, []topology.SubRepo{
		{Slug: "codrax-1234", RootAbs: "/parent/codrax", RootRel: "codrax", FileCount: 1},
	}, []string{"codrax-1234"})
	ctx := ctxWithMG(mg)
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"codrax"}},
	}

	projectEntityProvenance(ctx, &rm)

	assertEntityProvenance(t, rm.AnalyzerHints.EntityProvenance, "codrax", types.EntityResolutionScope, true, true, true)
}

func TestProjectEntityProvenance_EmptyInputKeepsNilLanes(t *testing.T) {
	rm := types.RequestModel{}
	projectEntityProvenance(&types.AgentContext{}, &rm)
	if rm.AnalyzerHints.EntityProvenance != nil {
		t.Fatalf("empty analyzer entities should keep nil provenance, got %+v", rm.AnalyzerHints.EntityProvenance)
	}
	if len(rm.SubTopics) != 0 {
		t.Fatalf("unexpected subtopics: %+v", rm.SubTopics)
	}
}

func assertEntityProvenance(
	t *testing.T,
	provenance []types.EntityProvenance,
	surface string,
	resolution types.EntityResolution,
	resolved bool,
	useForSearch bool,
	useForShape bool,
) {
	t.Helper()
	for _, p := range provenance {
		if p.Surface != surface {
			continue
		}
		if p.Resolution != resolution || p.Resolved != resolved ||
			p.UseForSearch != useForSearch || p.UseForShape != useForShape {
			t.Fatalf("provenance for %q = %+v, want resolution=%s resolved=%v search=%v shape=%v",
				surface, p, resolution, resolved, useForSearch, useForShape)
		}
		return
	}
	t.Fatalf("missing provenance for %q in %+v", surface, provenance)
}
