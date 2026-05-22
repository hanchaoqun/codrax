package tool

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
			Value: "5",
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

func TestReconcileCompletionAggregateFactsWithDefinitionEvidence_PreservesExactPrincipalSet(t *testing.T) {
	ctx := aggregateReconcileSubAgentContext()
	evidence := aggregateReconcileSubAgentEvidence()
	facts := []types.AnswerAggregateFact{
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "codrax 默认注册的 SubAgent 名称",
			Value:   "1",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Unit:    "个",
			Members: []string{"explorer"},
		},
		{
			Kind:  types.AnswerAggregateTotalCount,
			Label: "注册调用次数",
			Value: "1",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
		},
	}

	got := reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx, facts, evidence)
	if !reflect.DeepEqual(got, facts) {
		t.Fatalf("self-consistent principal member_set must not absorb support/query methods;\ngot  %#v\nwant %#v", got, facts)
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

func TestReconcileCompletionAggregateFactsWithDefinitionEvidence_DoesNotExpandSiblingSameRoleBuckets(t *testing.T) {
	ctx := aggregateReconcileTestContext()
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{}}
	add := func(name string, line int) {
		graph.SymbolDefs[name] = append(graph.SymbolDefs[name], &repotypes.Symbol{
			Name:     name,
			Kind:     "const",
			File:     "internal/types/grammar.go",
			Line:     line,
			EndLine:  line,
			Exported: true,
		})
	}
	add("KindSymbolPresent", 29)
	add("KindNoCallSites", 30)
	add("KindPlanReady", 54)
	add("KindPatchApplies", 55)
	ctx.Mutable.SetSearchGraph(graph)

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
	evidence := []types.EvidenceItem{
		def("KindSymbolPresent", 29),
		def("KindNoCallSites", 30),
		def("KindPlanReady", 54),
		def("KindPatchApplies", 55),
	}
	facts := []types.AnswerAggregateFact{
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "Kind 常量成员（读模式）",
			Value:   "2",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"KindSymbolPresent", "KindNoCallSites"},
		},
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "Kind 常量成员（写模式）",
			Value:   "2",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"KindPlanReady", "KindPatchApplies"},
		},
	}

	got := reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx, facts, evidence)
	if !reflect.DeepEqual(got, facts) {
		t.Fatalf("same-role sibling buckets must not be expanded by role-only definition evidence;\ngot  %#v\nwant %#v", got, facts)
	}
}

func TestReconcileCompletionAggregateFactsWithDefinitionEvidence_DoesNotExpandAcrossConstDeclarationFamily(t *testing.T) {
	ctx := aggregateReconcileStageAgentContext()
	evidence := aggregateReconcileStageAgentEvidence()
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "read-mode pipeline 所有 stage",
		Value:   "6",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"StageLogTriage", "StagePerfTriage", "StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
	}}

	got := reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx, facts, evidence)
	if !reflect.DeepEqual(got, facts) {
		t.Fatalf("PipelineStage member set must not absorb same-role AgentName constants;\ngot  %#v\nwant %#v", got, facts)
	}
}

func TestReconcileCompletionAggregateFactsWithDefinitionEvidence_ExpandsSameConstDeclarationFamily(t *testing.T) {
	ctx := aggregateReconcileStageAgentContext()
	evidence := aggregateReconcileStageAgentEvidence()
	facts := []types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateTotalCount,
			Label: "read-mode pipeline stage count",
			Value: "6",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
		},
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "read-mode pipeline 所有 stage",
			Value:   "2",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"StageAnalyze", "StageExplore"},
		},
	}

	got := reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx, facts, evidence)
	if len(got) != 2 {
		t.Fatalf("facts len = %d, want 2", len(got))
	}
	for _, unexpected := range []string{"AgentAnalyzer", "AgentExplorer", "AgentExtractor", "AgentFinalizer"} {
		for _, member := range got[1].Members {
			if member == unexpected {
				t.Fatalf("same-family expansion pulled AgentName constant %q into stage set: %#v", unexpected, got[1].Members)
			}
		}
	}
	for _, want := range []string{"StageAnalyze", "StageExplore", "StageLogTriage", "StagePerfTriage", "StageExtract", "StageFinalize"} {
		found := false
		for _, member := range got[1].Members {
			if member == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("same PipelineStage family member %q was not preserved/appended: %#v", want, got[1].Members)
		}
	}
	if got[0].Value != "6" || got[1].Value != "6" {
		t.Fatalf("values = count:%q member_set:%q, want 6/6 after same-family expansion: %+v", got[0].Value, got[1].Value, got)
	}
}

func TestReconcileCompletionAggregateFactsWithSourceInventory_GoStringEnumTypes(t *testing.T) {
	repo := t.TempDir()
	source := `package types

// Intent classifies the user's request intent.
type Intent string
// QuestionFamily names the broad answer family.
type QuestionFamily string
type NonString int
type PublicNoConst string
// AnswerSymbolVisibility controls whether exported or internal symbols are included.
type AnswerSymbolVisibility string

const (
	IntentExplain Intent = "explain"
	QuestionFamilyCode QuestionFamily = "code"
	AnswerSymbolVisibilityPublicExported AnswerSymbolVisibility = "public_exported"
	AnswerSymbolVisibilityPrivateOnly AnswerSymbolVisibility = "private_only"
)
`
	writeAggregateReconcileTestFile(t, repo, "internal/types/enums.go", source)
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "internal/types/enums.go",
		Language: "go",
		Symbols: []repotypes.Symbol{
			{Name: "Intent", Kind: "type", File: "internal/types/enums.go", Line: 4, EndLine: 4, Exported: true, Doc: "// Intent classifies the user's request intent."},
			{Name: "QuestionFamily", Kind: "type", File: "internal/types/enums.go", Line: 6, EndLine: 6, Exported: true, Doc: "/* QuestionFamily names the broad answer family. */"},
			{Name: "NonString", Kind: "type", File: "internal/types/enums.go", Line: 7, EndLine: 7, Exported: true, Doc: "// Intentional adjacent prose must not describe Intent."},
			{Name: "PublicNoConst", Kind: "type", File: "internal/types/enums.go", Line: 8, EndLine: 8, Exported: true},
			{Name: "AnswerSymbolVisibility", Kind: "type", File: "internal/types/enums.go", Line: 10, EndLine: 10, Exported: true, Doc: "// AnswerSymbolVisibility controls whether exported or internal symbols are included."},
		},
	}})
	ctx := sourceInventoryTestContext(repo, graph, "internal/types", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
		TypeUnderlying:    types.SourceInventoryTypeUnderlyingString,
		RequiresConstSet:  true,
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
		Confidence:        0.95,
	})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public string enum types",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Intent", "NonString", "private_only"},
	}}

	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	if len(got) != 1 {
		t.Fatalf("facts len = %d, want 1", len(got))
	}
	for _, want := range []string{"Intent", "QuestionFamily", "AnswerSymbolVisibility"} {
		if !containsString(got[0].Members, want) {
			t.Fatalf("members missing %q: %#v", want, got[0].Members)
		}
	}
	for _, banned := range []string{"NonString", "PublicNoConst", "private_only"} {
		if containsString(got[0].Members, banned) || strings.Contains(strings.Join(got[0].SupportRefs, " "), banned) {
			t.Fatalf("invalid enum member/value %q leaked into source inventory: members=%#v refs=%#v", banned, got[0].Members, got[0].SupportRefs)
		}
	}
	joinedNotes := strings.Join(got[0].MemberNotes, "\n")
	if !strings.Contains(joinedNotes, "classifies the user's request intent") ||
		!strings.Contains(joinedNotes, "broad answer family") ||
		!strings.Contains(joinedNotes, "controls whether exported or internal symbols are included") {
		t.Fatalf("source inventory should preserve safe type comments as member notes, got %#v", got[0].MemberNotes)
	}
	if strings.Contains(joinedNotes, "//") || strings.Contains(joinedNotes, "/*") || strings.Contains(joinedNotes, "*/") {
		t.Fatalf("source inventory notes should be clean user-facing prose, got %#v", got[0].MemberNotes)
	}
}

func TestSourceInventoryCommentDescribesSymbolRequiresIdentifierToken(t *testing.T) {
	if !sourceInventoryCommentDescribesSymbol("AnswerSymbolVisibility controls visibility.", "AnswerSymbolVisibility") {
		t.Fatal("expected exact symbol token in doc comment to match")
	}
	if sourceInventoryCommentDescribesSymbol("Intentional adjacent prose mentions a prefix only.", "Intent") {
		t.Fatal("substring prefix should not qualify as the symbol's doc comment")
	}
	if got := sourceInventoryCompactNote("/*\n * Intent classifies requests.\n */"); got != "Intent classifies requests." {
		t.Fatalf("comment marker cleanup = %q", got)
	}
}

func TestSourceInventoryCandidateNoteFromGraphCrossLanguageSafety(t *testing.T) {
	cases := []struct {
		name     string
		lang     string
		symbol   string
		doc      string
		wantNote bool
	}{
		{
			name:     "go doc convention accepted",
			lang:     repotypes.LangGo,
			symbol:   "Intent",
			doc:      "// Intent classifies requests.",
			wantNote: true,
		},
		{
			name:     "adjacent comment without symbol is not promoted",
			lang:     repotypes.LangJava,
			symbol:   "RequestHandler",
			doc:      "// Handles incoming requests.",
			wantNote: false,
		},
		{
			name:     "python docstring is structurally bound",
			lang:     repotypes.LangPython,
			symbol:   "handle_request",
			doc:      "Handles incoming requests.",
			wantNote: true,
		},
		{
			name:     "arkts decorator metadata is not descriptive doc",
			lang:     repotypes.LangArkTS,
			symbol:   "RuntimePanel",
			doc:      "@Component @Entry",
			wantNote: false,
		},
		{
			name:     "cangjie modifier metadata is not descriptive doc",
			lang:     repotypes.LangCangjie,
			symbol:   "Runtime",
			doc:      "public open",
			wantNote: false,
		},
		{
			name:     "substring prefix is rejected across languages",
			lang:     repotypes.LangTypeScript,
			symbol:   "Intent",
			doc:      "/** Intentional state bucket. */",
			wantNote: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sourceInventoryCandidateNoteFromGraph(&repotypes.Symbol{Name: tc.symbol, Doc: tc.doc}, tc.lang)
			if (got != "") != tc.wantNote {
				t.Fatalf("note accepted=%v want %v; got %q", got != "", tc.wantNote, got)
			}
			if strings.Contains(got, "//") || strings.Contains(got, "/*") || strings.Contains(got, "*/") {
				t.Fatalf("note should be cleaned, got %q", got)
			}
		})
	}
}

func TestEffectiveCompletionAggregateFactsForValidation_ReconcilesSourceInventoryBeforePreComplete(t *testing.T) {
	repo := t.TempDir()
	source := `package types

type Intent string
type QuestionFamily string
type NonString int
type PublicNoConst string
type AnswerSymbolVisibility string

const (
	IntentExplain Intent = "explain"
	QuestionFamilyCode QuestionFamily = "code"
	AnswerSymbolVisibilityPublicExported AnswerSymbolVisibility = "public_exported"
	AnswerSymbolVisibilityPrivateOnly AnswerSymbolVisibility = "private_only"
)
`
	writeAggregateReconcileTestFile(t, repo, "internal/types/enums.go", source)
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "internal/types/enums.go",
		Language: "go",
		Symbols: []repotypes.Symbol{
			{Name: "Intent", Kind: "type", File: "internal/types/enums.go", Line: 3, EndLine: 3, Exported: true},
			{Name: "QuestionFamily", Kind: "type", File: "internal/types/enums.go", Line: 4, EndLine: 4, Exported: true},
			{Name: "NonString", Kind: "type", File: "internal/types/enums.go", Line: 5, EndLine: 5, Exported: true},
			{Name: "PublicNoConst", Kind: "type", File: "internal/types/enums.go", Line: 6, EndLine: 6, Exported: true},
			{Name: "AnswerSymbolVisibility", Kind: "type", File: "internal/types/enums.go", Line: 7, EndLine: 7, Exported: true},
		},
	}})
	ctx := sourceInventoryTestContext(repo, graph, "internal/types", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles: []types.AnswerCandidateRole{
			types.AnswerCandidateRoleType,
			types.AnswerCandidateRoleConstant,
		},
		TypeUnderlying:   types.SourceInventoryTypeUnderlyingString,
		RequiresConstSet: true,
		RequestedFields: []types.SourceInventoryRequestedField{
			types.SourceInventoryFieldName,
			types.SourceInventoryFieldLocation,
		},
		Confidence: 0.95,
	})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "internal/types public string enum types",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Intent"},
	}}

	got := effectiveCompletionAggregateFactsForValidation(ctx, facts, nil)
	if len(got) != 1 {
		t.Fatalf("facts len = %d, want 1", len(got))
	}
	wantMembers := []string{"Intent", "QuestionFamily", "AnswerSymbolVisibility"}
	if !reflect.DeepEqual(got[0].Members, wantMembers) {
		t.Fatalf("members = %#v, want %#v", got[0].Members, wantMembers)
	}
	if got[0].Value != "3" {
		t.Fatalf("value = %q, want 3", got[0].Value)
	}
	if len(got[0].SupportRefs) != len(wantMembers) {
		t.Fatalf("support refs = %#v, want one per member", got[0].SupportRefs)
	}
	ok, invalid := exhaustiveEnumerationMemberSetUsable(ctx, got)
	if !ok {
		t.Fatalf("effective source-inventory fact should be pre-complete usable, invalid=%s facts=%#v", invalid, got)
	}
}

func TestReconcileCompletionAggregateFactsWithSourceInventory_GraphBackedPublicFunctions(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "internal/analysis/criterion/eval.go",
		Language: "go",
		Symbols: []repotypes.Symbol{
			{Name: "Eval", Kind: "function", File: "internal/analysis/criterion/eval.go", Line: 15, EndLine: 20, Exported: true},
			{Name: "EvalAll", Kind: "function", File: "internal/analysis/criterion/eval.go", Line: 36, EndLine: 45, Exported: true},
			{Name: "SetExternalArtifactFloor", Kind: "function", File: "internal/analysis/criterion/eval.go", Line: 1029, EndLine: 1032, Exported: true},
			{Name: "parseComparison", Kind: "function", File: "internal/analysis/criterion/eval.go", Line: 1100, EndLine: 1110, Exported: false},
			{Name: "registered", Kind: "var", File: "internal/analysis/criterion/eval.go", Line: 1120, EndLine: 1120, Exported: false},
		},
	}})
	ctx := sourceInventoryTestContext("", graph, "internal/analysis/criterion", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation, types.SourceInventoryFieldSummary},
		Confidence:        0.95,
	})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public functions",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval", "EvalAll"},
	}}

	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	want := []string{"Eval", "EvalAll", "SetExternalArtifactFloor"}
	if !reflect.DeepEqual(got[0].Members, want) {
		t.Fatalf("members = %#v, want %#v", got[0].Members, want)
	}
	for _, banned := range []string{"parseComparison", "registered"} {
		if containsString(got[0].Members, banned) {
			t.Fatalf("private/helper member %q leaked into public function inventory: %#v", banned, got[0].Members)
		}
	}
}

func TestReconcileCompletionAggregateFactsWithSourceInventory_MixedLanguageNoOp(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/a.go", Language: "go", Symbols: []repotypes.Symbol{{Name: "Eval", Kind: "function", File: "src/a.go", Line: 1, Exported: true}}},
		{RelPath: "src/B.java", Language: "java", Symbols: []repotypes.Symbol{{Name: "Run", Kind: "function", File: "src/B.java", Line: 1, Exported: true}}},
	})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		Confidence:        0.95,
	})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public functions",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval"},
	}}

	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	if !reflect.DeepEqual(got, facts) {
		t.Fatalf("mixed-language scope should no-op until the adapter can prove completeness;\ngot  %#v\nwant %#v", got, facts)
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

func writeAggregateReconcileTestFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func sourceInventoryTestContext(repo string, graph *repotypes.Graph, scope string, profile *types.SourceInventoryProfile) *types.BusContext {
	mut := types.NewMutableState("source inventory")
	mut.SetSearchGraph(graph)
	return &types.BusContext{
		RepoRoot: repo,
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
			AnswerVisibilityProfile: &types.AnswerVisibilityProfile{
				SymbolVisibility: types.AnswerSymbolVisibilityPublicExported,
				Confidence:       0.95,
			},
			SourceInventoryProfile: profile,
			AnalyzerHints: types.AnalyzerHints{
				Entities: []string{scope},
			},
		}},
	}
}

func testGraphWithFiles(files []*repotypes.FileInfo) *repotypes.Graph {
	g := &repotypes.Graph{
		Files:      files,
		FileIndex:  map[string]*repotypes.FileInfo{},
		SymbolDefs: map[string][]*repotypes.Symbol{},
	}
	for _, fi := range files {
		if fi == nil {
			continue
		}
		g.FileIndex[fi.RelPath] = fi
		for i := range fi.Symbols {
			sym := &fi.Symbols[i]
			if sym.File == "" {
				sym.File = fi.RelPath
			}
			g.SymbolDefs[sym.Name] = append(g.SymbolDefs[sym.Name], sym)
		}
	}
	return g
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func aggregateReconcileStageAgentContext() *types.BusContext {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{}}
	add := func(name string, line int) {
		graph.SymbolDefs[name] = append(graph.SymbolDefs[name], &repotypes.Symbol{
			Name:     name,
			Kind:     "const",
			File:     "internal/types/enums.go",
			Line:     line,
			EndLine:  line,
			Exported: true,
		})
	}
	for name, line := range map[string]int{
		"StageLogTriage":  15,
		"StagePerfTriage": 24,
		"StageAnalyze":    26,
		"StageExplore":    27,
		"StageExtract":    28,
		"StageFinalize":   29,
		"AgentAnalyzer":   116,
		"AgentExplorer":   117,
		"AgentExtractor":  118,
		"AgentFinalizer":  119,
	} {
		add(name, line)
	}
	mut := types.NewMutableState("pipeline stages")
	mut.SetSearchGraph(graph)
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[internal/types/enums.go: showing lines 15-119 of 130 total]\n" +
			"   15│ \tStageLogTriage PipelineStage = \"log_triage\"\n" +
			"   24│ \tStagePerfTriage PipelineStage = \"perf_triage\"\n" +
			"   26│ \tStageAnalyze  PipelineStage = \"analyze\"\n" +
			"   27│ \tStageExplore  PipelineStage = \"explore\"\n" +
			"   28│ \tStageExtract  PipelineStage = \"extract\"\n" +
			"   29│ \tStageFinalize PipelineStage = \"finalize\"\n" +
			"  116│ \tAgentAnalyzer   AgentName = \"analyzer\"\n" +
			"  117│ \tAgentExplorer   AgentName = \"explorer\"\n" +
			"  118│ \tAgentExtractor  AgentName = \"extractor\"\n" +
			"  119│ \tAgentFinalizer  AgentName = \"finalizer\"",
	})
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
					Path:       "internal/types/enums.go",
					Confidence: 0.95,
				}},
			},
		}},
	}
}

func aggregateReconcileStageAgentEvidence() []types.EvidenceItem {
	def := func(symbol string, line int) types.EvidenceItem {
		return types.EvidenceItem{
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    symbol,
			Subject:         symbol,
			Source:          "internal/types/enums.go",
			LineStart:       line,
			GroundingStatus: types.GroundingGrounded,
		}
	}
	return []types.EvidenceItem{
		def("StageLogTriage", 15),
		def("StagePerfTriage", 24),
		def("StageAnalyze", 26),
		def("StageExplore", 27),
		def("StageExtract", 28),
		def("StageFinalize", 29),
		def("AgentAnalyzer", 116),
		def("AgentExplorer", 117),
		def("AgentExtractor", 118),
		def("AgentFinalizer", 119),
	}
}

func aggregateReconcileSubAgentContext() *types.BusContext {
	graph := &repotypes.Graph{SymbolDefs: map[string][]*repotypes.Symbol{}}
	add := func(name, kind, file string, line int, endLine int) {
		graph.SymbolDefs[name] = append(graph.SymbolDefs[name], &repotypes.Symbol{
			Name:     name,
			Kind:     kind,
			File:     file,
			Line:     line,
			EndLine:  endLine,
			Exported: true,
		})
	}
	add("Name", "method", "internal/agent/sub_explorer.go", 31, 33)
	add("Register", "method", "internal/agent/subagent.go", 34, 38)
	add("Names", "method", "internal/agent/subagent.go", 52, 60)
	add("RegisterDefaultSubAgents", "function", "internal/agent/subagent.go", 63, 65)

	mut := types.NewMutableState("subagent registry names")
	mut.SetSearchGraph(graph)
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[internal/agent/sub_explorer.go: showing lines 31-33 of 33 total]\n" +
			"   31│ func (s *SubExplorer) Name() string {\n" +
			"   32│ \treturn \"explorer\"\n" +
			"   33│ }\n" +
			"[internal/agent/subagent.go: showing lines 52-64 of 65 total]\n" +
			"   52│ func (r *SubAgentRegistry) Names() []string {\n" +
			"   63│ func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {\n" +
			"   64│ \tr.Register(NewSubExplorer(deps))",
	})
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
				RequiredFileHints: []types.RequiredFileHint{
					{Path: "internal/agent/subagent.go", Confidence: 0.95},
					{Path: "internal/agent/sub_explorer.go", Confidence: 0.95},
				},
			},
		}},
	}
}

func aggregateReconcileSubAgentEvidence() []types.EvidenceItem {
	def := func(symbol, subject, object, file string, line int) types.EvidenceItem {
		return types.EvidenceItem{
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    symbol,
			Subject:         subject,
			Object:          object,
			Source:          file,
			LineStart:       line,
			GroundingStatus: types.GroundingGrounded,
		}
	}
	return []types.EvidenceItem{
		def("Name", "SubExplorer.Name", "explorer", "internal/agent/sub_explorer.go", 31),
		def("Register", "SubAgentRegistry.Register", "sa.Name()", "internal/agent/subagent.go", 34),
		def("Names", "SubAgentRegistry.Names", "", "internal/agent/subagent.go", 52),
		def("RegisterDefaultSubAgents", "RegisterDefaultSubAgents", "NewSubExplorer", "internal/agent/subagent.go", 63),
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
