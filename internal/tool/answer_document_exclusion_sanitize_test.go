package tool

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeTypedExcludedAnswerSurface_RedactsGraphVariableNames(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"registered": {{
			Name:     "registered",
			Kind:     "var",
			Exported: false,
		}},
		"ErrUnknownKind": {{
			Name:     "ErrUnknownKind",
			Kind:     "var",
			Exported: true,
		}},
		"defaultExternalArtifactFloor": {{
			Name:     "defaultExternalArtifactFloor",
			Kind:     "var",
			Exported: false,
		}},
		"SetExternalArtifactFloor": {{
			Name:     "SetExternalArtifactFloor",
			Kind:     "function",
			Exported: true,
		}},
	}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleVariable,
				},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "已排除变量符号 `registered`、`ErrUnknownKind`、`defaultExternalArtifactFloor`；函数 `SetExternalArtifactFloor` 仍应保留。",
		}},
		Citations: []types.Citation{{
			File:  "internal/analysis/criterion/eval.go",
			Line:  977,
			Quote: "var defaultExternalArtifactFloor float64",
		}},
	}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed == 0 {
		t.Fatal("expected sanitizer to redact excluded variable symbols")
	}
	visible := types.AnswerBlockVisibleSurface(doc.Blocks[0]) + "\n" + doc.Citations[0].Quote
	for _, banned := range []string{"registered", "ErrUnknownKind", "defaultExternalArtifactFloor"} {
		if strings.Contains(visible, banned) {
			t.Fatalf("visible answer leaked excluded candidate %q:\n%s", banned, visible)
		}
	}
	if !strings.Contains(visible, "SetExternalArtifactFloor") {
		t.Fatalf("sanitizer must not redact allowed function name:\n%s", visible)
	}
}

func TestNormalizeTypedExcludedAnswerSurface_DropsPrincipalExcludedRoleRows(t *testing.T) {
	ctx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleVariable,
				},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "list",
		Kind:        types.BlockOrderedList,
		SurfaceRole: types.SurfacePrincipal,
		Items: []types.AnswerBlockItem{
			{ID: "fn", Label: "Eval", CandidateRole: types.AnswerCandidateRoleFunction},
			{ID: "var", Label: "registered", CandidateRole: types.AnswerCandidateRoleVariable},
		},
	}}}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed != 1 {
		t.Fatalf("changed=%d want 1 dropped row", changed)
	}
	if len(doc.Blocks[0].Items) != 1 || doc.Blocks[0].Items[0].Label != "Eval" {
		t.Fatalf("unexpected items after pruning: %+v", doc.Blocks[0].Items)
	}
}

func TestNormalizeAggregateFactsForTypedExclusion_PrunesExactMemberSets(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"Eval":            {{Name: "Eval", Kind: "function", Exported: true}},
		"parseComparison": {{Name: "parseComparison", Kind: "function", Exported: false}},
		"ErrUnknownKind":  {{Name: "ErrUnknownKind", Kind: "var", Exported: true}},
	}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRolePrivate,
					types.AnswerCandidateRoleVariable,
				},
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "public functions",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Value:       "3",
		Members:     []string{"Eval (eval.go:15)", "parseComparison (eval.go:918)", "ErrUnknownKind (grammar.go:118)"},
		SupportRefs: []string{"Eval: internal/analysis/criterion/eval.go:15", "parseComparison: internal/analysis/criterion/eval.go:918", "ErrUnknownKind: internal/analysis/criterion/grammar.go:118"},
	}}

	got := normalizeAggregateFactsForTypedExclusion(ctx, facts)
	if len(got) != 1 {
		t.Fatalf("facts len=%d want 1", len(got))
	}
	if got[0].Value != "1" || len(got[0].Members) != 1 || got[0].Members[0] != "Eval (eval.go:15)" {
		t.Fatalf("unexpected sanitized member set: %+v", got[0])
	}
	if len(got[0].SupportRefs) != 1 || !strings.Contains(got[0].SupportRefs[0], "Eval") {
		t.Fatalf("support refs not kept in member order: %+v", got[0].SupportRefs)
	}
}
