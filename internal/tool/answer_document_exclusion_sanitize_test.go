package tool

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeTypedExcludedAnswerSurface_RedactsSafeGraphVariableNamesFromProse(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"registered": {{
			Name:     "registered",
			Kind:     "var",
			Exported: false,
		}},
		"block": {{
			Name:     "block",
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
			Text: "已排除变量符号 `registered`、`block`、`ErrUnknownKind`、`defaultExternalArtifactFloor`；函数 `SetExternalArtifactFloor` 仍应保留。",
		}},
		Citations: []types.Citation{{
			File:  "internal/analysis/criterion/eval.go",
			Line:  977,
			Quote: "var defaultExternalArtifactFloor float64",
		}},
	}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed == 0 {
		t.Fatal("expected sanitizer to redact code-shaped graph-derived excluded candidates")
	}
	visible := types.AnswerBlockVisibleSurface(doc.Blocks[0]) + "\n" + doc.Citations[0].Quote
	for _, banned := range []string{"defaultExternalArtifactFloor"} {
		if strings.Contains(visible, banned) {
			t.Fatalf("visible answer leaked graph-derived excluded candidate %q:\n%s", banned, visible)
		}
	}
	for _, preserved := range []string{"registered", "block", "ErrUnknownKind"} {
		if !strings.Contains(visible, preserved) {
			t.Fatalf("common lowercase variable name %q should not be redacted from prose:\n%s", preserved, visible)
		}
	}
	if !strings.Contains(visible, "SetExternalArtifactFloor") {
		t.Fatalf("sanitizer must not redact allowed function name:\n%s", visible)
	}
}

func TestNormalizeTypedExcludedAnswerSurface_RedactsExplicitExcludedAggregateCandidates(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:     types.AnswerAggregateExcluded,
			Label:    "excluded variables",
			Value:    "1",
			Excluded: []string{"defaultExternalArtifactFloor"},
		}},
	})
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
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "s1",
		Kind: types.BlockSummary,
		Text: "变量 defaultExternalArtifactFloor 已按排除清单隐藏。",
	}}}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed == 0 {
		t.Fatal("expected explicit excluded[] candidate to be redacted")
	}
	visible := types.AnswerBlockVisibleSurface(doc.Blocks[0])
	if strings.Contains(visible, "defaultExternalArtifactFloor") || !strings.Contains(visible, "[excluded]") {
		t.Fatalf("explicit excluded candidate was not redacted correctly:\n%s", visible)
	}
}

func TestNormalizeTypedExcludedAnswerSurface_PreservesAllowedHomonym(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"Kind": {
			{Name: "Kind", Kind: "field", Exported: true},
			{Name: "Kind", Kind: "type", Exported: true},
		},
		"defaultExternalArtifactFloor": {{
			Name:     "defaultExternalArtifactFloor",
			Kind:     "var",
			Exported: false,
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
					types.AnswerCandidateRoleField,
				},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "类型 Kind 与 Kind 常量必须保留；变量 defaultExternalArtifactFloor 必须排除。",
		}},
		Citations: []types.Citation{{
			File:  "internal/analysis/criterion/grammar.go",
			Line:  29,
			Quote: "KindSymbolPresent Kind = Kind(types.CritSymbolPresent)",
		}},
	}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed == 0 {
		t.Fatalf("expected excluded variable to be redacted")
	}
	visible := types.AnswerBlockVisibleSurface(doc.Blocks[0]) + "\n" + doc.Citations[0].Quote
	if !strings.Contains(visible, "类型 Kind") ||
		!strings.Contains(visible, "Kind 常量") ||
		!strings.Contains(visible, "[excluded]") ||
		strings.Contains(visible, "defaultExternalArtifactFloor") ||
		!strings.Contains(visible, "KindSymbolPresent Kind = Kind(types.CritSymbolPresent)") {
		t.Fatalf("sanitizer over-redacted visible prose:\n%s", visible)
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

func TestNormalizeTypedExcludedAnswerSurface_SourceInventoryTargetRoleOverridesContradictoryExclusion(t *testing.T) {
	ctx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
				TypeUnderlying:    types.SourceInventoryTypeUnderlyingString,
				RequiresConstSet:  true,
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
				},
				Confidence: 0.95,
			},
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleType,
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
			{ID: "type", Label: "EvidenceKind", CandidateRole: types.AnswerCandidateRoleType},
			{ID: "var", Label: "scratch", CandidateRole: types.AnswerCandidateRoleVariable},
		},
	}}}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed != 1 {
		t.Fatalf("changed=%d want only non-requested variable row dropped; items=%+v", changed, doc.Blocks[0].Items)
	}
	if len(doc.Blocks[0].Items) != 1 || doc.Blocks[0].Items[0].Label != "EvidenceKind" {
		t.Fatalf("source inventory target type row should survive contradictory exclusion: %+v", doc.Blocks[0].Items)
	}
}

func TestNormalizeAggregateFactsForTypedVisibilityPrunesPrivateMembers(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"Eval":              {{Name: "Eval", Kind: "function", Exported: true}},
		"parseIntThreshold": {{Name: "parseIntThreshold", Kind: "function", Exported: false}},
		"RegisteredKinds":   {{Name: "RegisteredKinds", Kind: "function", Exported: true}},
	}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnswerVisibilityProfile: &types.AnswerVisibilityProfile{
				SymbolVisibility: types.AnswerSymbolVisibilityPublicExported,
				Confidence:       0.95,
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "functions",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Value:   "3",
		Members: []string{"Eval", "parseIntThreshold", "RegisteredKinds"},
		SupportRefs: []string{
			"Eval: internal/pkg/eval.go:15",
			"parseIntThreshold: internal/pkg/eval.go:190",
			"RegisteredKinds: internal/pkg/grammar.go:106",
		},
	}}

	got := normalizeAggregateFactsForTypedExclusion(ctx, facts)
	if len(got) != 1 {
		t.Fatalf("facts len=%d want 1", len(got))
	}
	if got[0].Value != "2" ||
		strings.Join(got[0].Members, ",") != "Eval,RegisteredKinds" ||
		strings.Contains(strings.Join(got[0].SupportRefs, ","), "parseIntThreshold") {
		t.Fatalf("private member was not pruned with support_refs aligned: %+v", got[0])
	}
}

func TestNormalizeAggregateFactsForTypedVisibilityPrunesSelfLocatedPrivateMembers(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"Eval":              {{Name: "Eval", Kind: "function", File: "internal/pkg/eval.go", Line: 15, EndLine: 20, Exported: true}},
		"dispatch":          {{Name: "dispatch", Kind: "function", File: "internal/pkg/eval.go", Line: 51, EndLine: 80, Exported: false}},
		"parseIntThreshold": {{Name: "parseIntThreshold", Kind: "function", File: "internal/pkg/eval.go", Line: 190, EndLine: 220, Exported: false}},
		"RegisteredKinds":   {{Name: "RegisteredKinds", Kind: "function", File: "internal/pkg/grammar.go", Line: 106, EndLine: 112, Exported: true}},
	}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnswerVisibilityProfile: &types.AnswerVisibilityProfile{
				SymbolVisibility: types.AnswerSymbolVisibilityPublicExported,
				Confidence:       0.95,
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "exported_functions",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Value: "4",
		Members: []string{
			"Eval: internal/pkg/eval.go:15",
			"dispatch: internal/pkg/eval.go:51",
			"parseIntThreshold: internal/pkg/eval.go:190",
			"RegisteredKinds: internal/pkg/grammar.go:106",
		},
	}}

	got := normalizeAggregateFactsForTypedExclusion(ctx, facts)
	if len(got) != 1 {
		t.Fatalf("facts len=%d want 1", len(got))
	}
	if got[0].Value != "2" ||
		strings.Join(got[0].Members, ",") != "Eval,RegisteredKinds" ||
		len(got[0].SupportRefs) != 2 ||
		!strings.Contains(got[0].SupportRefs[0], "Eval @ internal/pkg/eval.go:15") ||
		!strings.Contains(got[0].SupportRefs[1], "RegisteredKinds @ internal/pkg/grammar.go:106") {
		t.Fatalf("self-located private helper members were not pruned/aligned: %+v", got[0])
	}
}

func TestNormalizeTypedExcludedAnswerSurface_HonorsExcludedRoleOverPrincipalAggregate(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"KindSymbolPresent": {{Name: "KindSymbolPresent", Kind: "const", File: "internal/analysis/criterion/grammar.go", Line: 29, EndLine: 29, Exported: true}},
		"KindCallEdge":      {{Name: "KindCallEdge", Kind: "const", File: "internal/analysis/criterion/grammar.go", Line: 30, EndLine: 30, Exported: true}},
	}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "Kind const block members",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Value:       "2",
		Members:     []string{"KindSymbolPresent", "KindCallEdge"},
		SupportRefs: []string{"KindSymbolPresent: internal/analysis/criterion/grammar.go:29", "KindCallEdge: internal/analysis/criterion/grammar.go:30"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleVariable,
					types.AnswerCandidateRoleConstant,
				},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "list",
		Kind:        types.BlockOrderedList,
		SurfaceRole: types.SurfacePrincipal,
		Items: []types.AnswerBlockItem{
			{ID: "const", Label: "KindSymbolPresent", CandidateRole: types.AnswerCandidateRoleConstant},
			{ID: "var", Label: "registered", CandidateRole: types.AnswerCandidateRoleVariable},
		},
	}}}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed != 2 {
		t.Fatalf("changed=%d want both explicitly excluded roles pruned; items=%+v", changed, doc.Blocks[0].Items)
	}
	if len(doc.Blocks[0].Items) != 0 {
		t.Fatalf("explicit excluded roles must not be protected by principal aggregate rows: %+v", doc.Blocks[0].Items)
	}
}

func TestNormalizeTypedExcludedAnswerSurface_RedactsGraphOnlyExcludedVariables(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"defaultExternalArtifactFloor": {{
			Name:     "defaultExternalArtifactFloor",
			Kind:     "var",
			Exported: false,
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
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "变量 defaultExternalArtifactFloor 按要求不列入。",
	}}}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed == 0 {
		t.Fatalf("expected graph-only excluded variable to be redacted")
	}
	if strings.Contains(doc.Blocks[0].Text, "defaultExternalArtifactFloor") ||
		!strings.Contains(doc.Blocks[0].Text, "[excluded]") {
		t.Fatalf("excluded variable was not redacted: %q", doc.Blocks[0].Text)
	}
}

func TestNormalizeTypedExcludedAnswerSurface_PublicVisibilityKeepsPrivateExamplesInScopeProse(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"allNegativeScopes": {{
			Name:     "allNegativeScopes",
			Kind:     "var",
			Exported: false,
		}},
		"allEvidenceKinds": {{
			Name:     "allEvidenceKinds",
			Kind:     "var",
			Exported: false,
		}},
	}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnswerVisibilityProfile: &types.AnswerVisibilityProfile{
				SymbolVisibility: types.AnswerSymbolVisibilityPublicExported,
				Confidence:       0.95,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "排除的非目标类型包括非导出类型，如 allNegativeScopes 变量、allEvidenceKinds 变量。",
	}}}

	changed := normalizeTypedExcludedAnswerSurface(doc, ctx)
	if changed != 0 {
		t.Fatalf("public visibility should not redact private examples from scope prose; changed=%d text=%q", changed, doc.Blocks[0].Text)
	}
	if strings.Contains(doc.Blocks[0].Text, "[excluded]") ||
		!strings.Contains(doc.Blocks[0].Text, "allNegativeScopes") ||
		!strings.Contains(doc.Blocks[0].Text, "allEvidenceKinds") {
		t.Fatalf("scope examples were unexpectedly redacted: %q", doc.Blocks[0].Text)
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
	if got[0].Value != "1" || len(got[0].Members) != 1 || got[0].Members[0] != "Eval" {
		t.Fatalf("unexpected sanitized member set: %+v", got[0])
	}
	if len(got[0].SupportRefs) != 1 || !strings.Contains(got[0].SupportRefs[0], "Eval") {
		t.Fatalf("support refs not kept in member order: %+v", got[0].SupportRefs)
	}
}

func TestNormalizeAggregateFactsForTypedExclusion_HonorsExcludedRoleOverSameRolePrincipalMemberSet(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"KindSymbolPresent": {{Name: "KindSymbolPresent", Kind: "const", File: "internal/analysis/criterion/grammar.go", Line: 29, EndLine: 29, Exported: true}},
		"KindCallEdge":      {{Name: "KindCallEdge", Kind: "const", File: "internal/analysis/criterion/grammar.go", Line: 30, EndLine: 30, Exported: true}},
		"registered":        {{Name: "registered", Kind: "var", File: "internal/analysis/criterion/registry.go", Line: 12, EndLine: 12, Exported: false}},
	}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleVariable,
					types.AnswerCandidateRoleConstant,
				},
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "Kind const block members",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Value:       "2",
		Members:     []string{"KindSymbolPresent", "KindCallEdge"},
		SupportRefs: []string{"KindSymbolPresent: internal/analysis/criterion/grammar.go:29", "KindCallEdge: internal/analysis/criterion/grammar.go:30"},
	}, {
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "mixed internal candidates",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Value:       "2",
		Members:     []string{"KindSymbolPresent", "registered"},
		SupportRefs: []string{"KindSymbolPresent: internal/analysis/criterion/grammar.go:29", "registered: internal/analysis/criterion/registry.go:12"},
	}}

	got := normalizeAggregateFactsForTypedExclusion(ctx, facts)
	if len(got) != 2 {
		t.Fatalf("facts len=%d want 2", len(got))
	}
	if got[0].Value != "0" || len(got[0].Members) != 0 {
		t.Fatalf("same-role principal constants should not override explicit exclusion: %+v", got[0])
	}
	if got[1].Value != "0" || len(got[1].Members) != 0 {
		t.Fatalf("mixed set should prune all explicitly excluded roles: %+v", got[1])
	}
}

func TestNormalizeAggregateFactsForTypedExclusion_SourceInventoryTargetRoleOverridesContradictoryExclusion(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"EvidenceKind":    {{Name: "EvidenceKind", Kind: "type", File: "internal/types/evidence.go", Line: 11, EndLine: 11, Exported: true}},
		"GroundingStatus": {{Name: "GroundingStatus", Kind: "type", File: "internal/types/evidence.go", Line: 21, EndLine: 21, Exported: true}},
		"scratch":         {{Name: "scratch", Kind: "var", File: "internal/types/evidence.go", Line: 99, EndLine: 99, Exported: false}},
	}}
	mut := types.NewMutableState("test")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
				TypeUnderlying:    types.SourceInventoryTypeUnderlyingString,
				RequiresConstSet:  true,
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
				},
				Confidence: 0.95,
			},
			AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
				IsExclusionRequested: true,
				ExcludedCandidateRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleType,
					types.AnswerCandidateRoleVariable,
				},
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "internal/types/evidence.go public string enum types",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Value:   "3",
		Members: []string{"EvidenceKind", "GroundingStatus", "scratch"},
		SupportRefs: []string{
			"EvidenceKind: internal/types/evidence.go:11",
			"GroundingStatus: internal/types/evidence.go:21",
			"scratch: internal/types/evidence.go:99",
		},
	}}

	got := normalizeAggregateFactsForTypedExclusion(ctx, facts)
	if len(got) != 1 {
		t.Fatalf("facts len=%d want 1", len(got))
	}
	if got[0].Value != "2" ||
		strings.Join(got[0].Members, ",") != "EvidenceKind,GroundingStatus" ||
		len(got[0].SupportRefs) != 2 ||
		!strings.Contains(got[0].SupportRefs[0], "EvidenceKind") ||
		!strings.Contains(got[0].SupportRefs[1], "GroundingStatus") {
		t.Fatalf("source inventory target types should survive while variables prune: %+v", got[0])
	}
}

func TestNormalizeAggregateFactsForTypedExclusion_PreservesAllowedHomonym(t *testing.T) {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{
		"Kind": {
			{Name: "Kind", Kind: "field", Exported: true},
			{Name: "Kind", Kind: "type", Exported: true},
		},
		"ErrUnknownKind": {{
			Name:     "ErrUnknownKind",
			Kind:     "var",
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
					types.AnswerCandidateRoleField,
				},
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "公开类型",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Value:       "2",
		Members:     []string{"Kind (grammar.go:26)", "ErrUnknownKind (grammar.go:118)"},
		SupportRefs: []string{"Kind: internal/analysis/criterion/grammar.go:26", "ErrUnknownKind: internal/analysis/criterion/grammar.go:118"},
	}}

	got := normalizeAggregateFactsForTypedExclusion(ctx, facts)
	if len(got) != 1 {
		t.Fatalf("facts len=%d want 1", len(got))
	}
	if got[0].Value != "1" || len(got[0].Members) != 1 || got[0].Members[0] != "Kind" {
		t.Fatalf("unexpected sanitized member set: %+v", got[0])
	}
	if len(got[0].SupportRefs) != 1 || !strings.Contains(got[0].SupportRefs[0], "Kind:") {
		t.Fatalf("support refs not kept for allowed homonym: %+v", got[0].SupportRefs)
	}
}
