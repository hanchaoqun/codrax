package tool

import (
	"reflect"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReconcileCompletionAggregateFactsWithDefinitionEvidence_AppendsMissingSameRoleDefinitions(t *testing.T) {
	ctx := aggregateReconcileTestContext()
	evidence := aggregateReconcileTestEvidence()
	facts := []types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateScalar,
			Label: "function count",
			Value: "3",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
		},
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "functions",
			Value:   "3",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval", "EvalAll", "SetExternalArtifactFloor"},
			SupportRefs: []string{
				"Eval: internal/types/grammar.go:93",
				"EvalAll: internal/types/grammar.go:94",
				"SetExternalArtifactFloor: internal/types/grammar.go:95",
			},
		},
	}

	got := reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx, facts, evidence)
	if len(got) != 2 {
		t.Fatalf("facts len = %d, want 2", len(got))
	}
	wantMembers := []string{"Eval", "EvalAll", "SetExternalArtifactFloor", "IsRegistered", "RegisteredKinds"}
	if !reflect.DeepEqual(got[1].Members, wantMembers) {
		t.Fatalf("members = %#v, want %#v", got[1].Members, wantMembers)
	}
	if got[0].Value != "5" || got[1].Value != "5" {
		t.Fatalf("values = scalar:%q member_set:%q, want 5/5", got[0].Value, got[1].Value)
	}
	for _, unexpected := range []string{"parseComparison", "registered"} {
		for _, member := range got[1].Members {
			if member == unexpected {
				t.Fatalf("unexpected candidate %q appended: %#v", unexpected, got[1].Members)
			}
		}
	}
}

func TestReconcileCompletionAggregateFactsWithDefinitionEvidence_DisabledWithoutTypedScopedInventory(t *testing.T) {
	ctx := aggregateReconcileTestContext()
	ctx.AnalysisIR.RequestModel.SourceScopeProfile = nil
	ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = nil
	evidence := aggregateReconcileTestEvidence()
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "functions",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval", "EvalAll", "SetExternalArtifactFloor"},
		SupportRefs: []string{
			"Eval: internal/types/grammar.go:93",
			"EvalAll: internal/types/grammar.go:94",
			"SetExternalArtifactFloor: internal/types/grammar.go:95",
		},
	}}

	got := reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx, facts, evidence)
	if !reflect.DeepEqual(got, facts) {
		t.Fatalf("reconciliation should be disabled without typed scoped inventory;\ngot  %#v\nwant %#v", got, facts)
	}
}

func TestReconcileCompletionAggregateFactsWithDefinitionEvidence_DoesNotExpandMixedExportStatus(t *testing.T) {
	ctx := aggregateReconcileTestContext()
	evidence := aggregateReconcileTestEvidence()
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public functions",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval", "parseComparison"},
		SupportRefs: []string{
			"Eval: internal/types/grammar.go:93",
			"parseComparison: internal/types/grammar.go:120",
		},
	}}

	got := reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx, facts, evidence)
	if !reflect.DeepEqual(got, facts) {
		t.Fatalf("mixed exported/private seed must not be auto-expanded;\ngot  %#v\nwant %#v", got, facts)
	}
}

func aggregateReconcileTestContext() *types.BusContext {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{}}
	add := func(name, kind string, line int, exported bool) {
		graph.SymbolDefs[name] = append(graph.SymbolDefs[name], &repotypes.Symbol{
			Name:     name,
			Kind:     kind,
			File:     "internal/types/grammar.go",
			Line:     line,
			EndLine:  line,
			Exported: exported,
		})
	}
	add("Eval", "function", 93, true)
	add("EvalAll", "function", 94, true)
	add("SetExternalArtifactFloor", "function", 95, true)
	add("IsRegistered", "function", 100, true)
	add("RegisteredKinds", "function", 106, true)
	add("parseComparison", "function", 120, false)
	add("registered", "var", 130, false)

	mut := types.NewMutableState("inventory functions")
	mut.SetSearchGraph(graph)
	return &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
			},
			AnalyzerHints: types.AnalyzerHints{
				RequiredFileHints: []types.RequiredFileHint{{
					Path:       "internal/types/grammar.go",
					Confidence: 0.95,
				}},
			},
		}},
	}
}

func aggregateReconcileTestEvidence() []types.EvidenceItem {
	def := func(symbol string, line int) types.EvidenceItem {
		return types.EvidenceItem{
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    symbol,
			Subject:         symbol,
			Source:          "internal/types/grammar.go",
			LineStart:       line,
			GroundingStatus: types.GroundingGrounded,
		}
	}
	return []types.EvidenceItem{
		def("Eval", 93),
		def("EvalAll", 94),
		def("SetExternalArtifactFloor", 95),
		def("IsRegistered", 100),
		def("RegisteredKinds", 106),
		def("parseComparison", 120),
		def("registered", 130),
	}
}
