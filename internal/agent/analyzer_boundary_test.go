package agent

import (
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReconcileEnumerationBoundaryScope_CollapsesSingleOwnerBoundedSet(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/analysis/gate/gate.go": {
				RelPath: "internal/analysis/gate/gate.go",
				Symbols: []repomap.Symbol{{
					Name:     "Run",
					Receiver: "gate",
					File:     "internal/analysis/gate/gate.go",
				}},
			},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"Run": {{
				Name:     "Run",
				Receiver: "gate",
				File:     "internal/analysis/gate/gate.go",
			}},
		},
	}
	rm := types.RequestModel{
		RawRequest: "What order do gate.Run's 7 checks execute in?",
		Predicates: types.SemanticPredicates{},
		AnalyzerHints: types.AnalyzerHints{
			Kind:              "mechanism",
			Entities:          []string{"gate.Run", "checkCoverage", "checkDAGClosure"},
			PrimaryEntities:   []string{"gate.Run", "checkCoverage", "checkDAGClosure"},
			MentionedEntities: []string{"gate.Run"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "principal order"},
			{Summary: "nearby caveats"},
		},
		EnumerationBoundary: &types.RequestedEnumerationBoundary{
			DeclaredCount: 7,
			SourceQuote:   "7 checks",
		},
	}

	got, reason := reconcileEnumerationBoundaryScope(rm, graph)
	if reason == "" {
		t.Fatal("expected reconcile reason")
	}
	if len(got.SubTopics) != 0 {
		t.Fatalf("sub_topics = %d, want 0 after single-owner boundary collapse", len(got.SubTopics))
	}
	if len(got.AnalyzerHints.Entities) != 1 || got.AnalyzerHints.Entities[0] != "gate.Run" {
		t.Fatalf("entities = %v, want [gate.Run]", got.AnalyzerHints.Entities)
	}
}

func TestReconcileEnumerationBoundaryScope_SkipsCrossComponentBoundary(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/a.go": {RelPath: "internal/a.go"},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"A": {{Name: "A", File: "internal/a.go"}},
		},
	}
	rm := types.RequestModel{
		RawRequest: "What are the first 3 differences between A and B?",
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			Kind:              "mechanism",
			Entities:          []string{"A", "B"},
			PrimaryEntities:   []string{"A", "B"},
			MentionedEntities: []string{"A", "B"},
		},
		EnumerationBoundary: &types.RequestedEnumerationBoundary{
			DeclaredCount: 3,
			SourceQuote:   "first 3 differences",
		},
	}

	got, reason := reconcileEnumerationBoundaryScope(rm, graph)
	if reason != "" {
		t.Fatalf("unexpected reconcile reason: %s", reason)
	}
	if len(got.AnalyzerHints.Entities) != 2 {
		t.Fatalf("entities changed unexpectedly: %v", got.AnalyzerHints.Entities)
	}
}

func TestReconcileEnumerationBoundaryScope_CollapsesWithoutExactGraphHit(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "What order do gate.Run's 7 checks execute in?",
		Predicates: types.SemanticPredicates{},
		AnalyzerHints: types.AnalyzerHints{
			Kind:              "mechanism",
			Entities:          []string{"gate.Run", "checkCoverage", "checkDAGClosure"},
			PrimaryEntities:   []string{"gate.Run", "checkCoverage", "checkDAGClosure"},
			MentionedEntities: []string{"gate.Run"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "principal order"},
			{Summary: "nearby caveats"},
		},
		EnumerationBoundary: &types.RequestedEnumerationBoundary{
			DeclaredCount: 7,
			SourceQuote:   "7 checks",
		},
	}

	got, reason := reconcileEnumerationBoundaryScope(rm, &repomap.Graph{})
	if reason == "" {
		t.Fatal("expected reconcile reason even without an exact graph hit")
	}
	if len(got.SubTopics) != 0 {
		t.Fatalf("sub_topics = %d, want 0 after boundary collapse", len(got.SubTopics))
	}
}

func TestReconcileScalarRoleLocateScope_DropsExploratorySubTopics(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "Which function is the entry point for this behavior?",
		Predicates: types.SemanticPredicates{
			IsRoleLocateLookup: true,
			IsScalarAnswer:     true,
			IsCrossComponent:   true,
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"entry", "behavior"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "entry function"},
			{Summary: "related call chain"},
		},
	}

	got, reason := reconcileScalarRoleLocateScope(rm)
	if reason == "" {
		t.Fatal("expected scalar role-locate scope reconcile")
	}
	if len(got.SubTopics) != 0 {
		t.Fatalf("sub_topics = %d, want 0 after role-locate collapse", len(got.SubTopics))
	}
	if len(got.AnalyzerHints.Entities) != len(rm.AnalyzerHints.Entities) {
		t.Fatalf("entities should remain available as search context: got %v want %v", got.AnalyzerHints.Entities, rm.AnalyzerHints.Entities)
	}
}

func TestReconcileScalarRoleLocateScope_PreservesSetValuedOrRelationalTopics(t *testing.T) {
	base := types.RequestModel{
		Predicates: types.SemanticPredicates{
			IsRoleLocateLookup: true,
			IsScalarAnswer:     true,
		},
		SubTopics: []types.SubTopic{{Summary: "A"}, {Summary: "B"}},
	}
	t.Run("set valued", func(t *testing.T) {
		rm := base
		rm.Predicates.IsScalarAnswer = false
		got, reason := reconcileScalarRoleLocateScope(rm)
		if reason != "" || len(got.SubTopics) != len(base.SubTopics) {
			t.Fatalf("set-valued role lookup should keep topics, reason=%q got=%+v", reason, got.SubTopics)
		}
	})
	t.Run("relational", func(t *testing.T) {
		rm := base
		rm.Predicates.IsRelationalLookup = true
		got, reason := reconcileScalarRoleLocateScope(rm)
		if reason != "" || len(got.SubTopics) != len(base.SubTopics) {
			t.Fatalf("relational role lookup should keep topics, reason=%q got=%+v", reason, got.SubTopics)
		}
	})
}
