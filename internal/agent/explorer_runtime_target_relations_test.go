package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeTargetRelationEvaluator(graph *repomap.Graph) *explorerEvaluator {
	return &explorerEvaluator{
		searchResult: &keywordSearchResult{Graph: graph},
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisCall,
			CallChainEndpointProfile: &types.CallChainEndpointProfile{
				Source:   "run_pipeline",
				SinkMode: types.CallChainSinkResolutionDiscover,
			},
		}},
	}
}

func TestBuildConcreteValuesSection_StandaloneStructuralRelationsReachHandoff(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath:  "pipeline/plugins.py",
		Language: repotypes.LangPython,
		Relations: []repotypes.Relation{
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "TimestampMixin"}, File: "pipeline/plugins.py", Line: 8, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "ValidationMixin"}, File: "pipeline/plugins.py", Line: 8, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "JsonPlugin"}, ToEP: repotypes.RelationEndpoint{Name: "BasePlugin"}, File: "pipeline/plugins.py", Line: 8, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "python_base_class"},
			// Same apparent shape, but regex salvage is not eligible for a
			// typed/citable structural relation.
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "Noise"}, ToEP: repotypes.RelationEndpoint{Name: "Guess"}, File: "pipeline/plugins.py", Line: 9, Confidence: repotypes.ConfidenceRegexSalvage, Provenance: repotypes.ProvenanceRegexFallback, ResolvedBy: "regex_guess"},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)

	got := eval.buildConcreteValuesSection(context.Background(), t.TempDir(), map[string]bool{"pipeline/plugins.py": true}, nil)
	if len(got.evidence) != 3 {
		t.Fatalf("standalone AST relations must survive the no-concrete-value path: got %d items: %+v", len(got.evidence), got.evidence)
	}
	for _, item := range got.evidence {
		if item.Producer != "repomap_structural_relation" || item.Kind != types.EvidenceRelationship ||
			item.Predicate != "inheritance" || item.Subject != "JsonPlugin" || !item.IsCitable() {
			t.Fatalf("unexpected structural relation evidence: %+v", item)
		}
		if item.Object == "Guess" {
			t.Fatalf("regex salvage must not become typed structural evidence: %+v", item)
		}
	}
	for _, want := range []string{
		"## Typed Declared-Type Relations",
		"declaration relationships, not invocation edges",
		"`JsonPlugin`",
		"`TimestampMixin`",
		"`ValidationMixin`",
		"`BasePlugin`",
	} {
		if !strings.Contains(got.markdown, want) {
			t.Fatalf("structural relation markdown missing %q:\n%s", want, got.markdown)
		}
	}
	if strings.Contains(got.markdown, "Noise") || strings.Contains(got.markdown, "Guess") {
		t.Fatalf("regex relation leaked into structural markdown:\n%s", got.markdown)
	}
}

func TestBuildRuntimeTargetStructuralRelations_RequiresExactReadLine(t *testing.T) {
	file := &repotypes.FileInfo{
		RelPath:  "src/types.cj",
		Language: repotypes.LangCangjie,
		Relations: []repotypes.Relation{
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "FastHandler"}, ToEP: repotypes.RelationEndpoint{Name: "Handler"}, File: "src/types.cj", Line: 5, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceCangjieParser, ResolvedBy: "cangjie_inheritance_clause"},
			{Kind: "inheritance", FromEP: repotypes.RelationEndpoint{Name: "HiddenHandler"}, ToEP: repotypes.RelationEndpoint{Name: "Handler"}, File: "src/types.cj", Line: 45, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceCangjieParser, ResolvedBy: "cangjie_inheritance_clause"},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{file})
	eval := runtimeTargetRelationEvaluator(graph)
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(map[string]bool{"src/types.cj": true})
	closure.AddReadRanges(map[string][]types.LineRange{"src/types.cj": {{Start: 1, End: 10}}})

	got := eval.buildRuntimeTargetStructuralRelations(graph, map[string]bool{"src/types.cj": true}, map[string]bool{"src/types.cj": true}, closure)
	if len(got.evidence) != 1 || got.evidence[0].Subject != "FastHandler" {
		t.Fatalf("only the precisely read relation line may be promoted: %+v", got.evidence)
	}
	if strings.Contains(got.markdown, "HiddenHandler") {
		t.Fatalf("unread structural relation line leaked into prompt:\n%s", got.markdown)
	}
}

func TestBuildRuntimeTargetStructuralRelations_InactiveForUnrelatedQuestion(t *testing.T) {
	graph := repomap.BuildGraph(t.TempDir(), []*repotypes.FileInfo{{
		RelPath:   "x.go",
		Relations: []repotypes.Relation{{Kind: "embedding", FromEP: repotypes.RelationEndpoint{Name: "X"}, ToEP: repotypes.RelationEndpoint{Name: "Y"}, File: "x.go", Line: 1, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_struct_embedding"}},
	}})
	eval := &explorerEvaluator{analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{PredicateAxis: types.AxisDefine}}}
	got := eval.buildRuntimeTargetStructuralRelations(graph, map[string]bool{"x.go": true}, map[string]bool{"x.go": true}, nil)
	if len(got.evidence) != 0 || got.markdown != "" {
		t.Fatalf("unrelated requests must not inherit runtime-target prompt load: %+v", got)
	}
}
