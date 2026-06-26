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

func TestExhaustiveEnumerationMemberSetUsableAcceptsVerifiedValueSupportRefClass(t *testing.T) {
	mu := types.NewMutableState("registry member-set")
	mu.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "ev-name",
			Kind:            types.EvidenceDirect,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Name",
			Source:          "internal/agent/sub_explorer.go",
			LineStart:       32,
			LineEnd:         34,
			Snippet:         "func (s *SubExplorer) Name() string {\n\treturn \"explorer\"\n}",
			Summary:         "SubExplorer.Name() returns string \"explorer\"",
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "ev-register",
			Kind:            types.EvidenceRegistration,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "Register",
			Source:          "internal/agent/subagent.go",
			LineStart:       64,
			LineEnd:         64,
			Snippet:         "r.Register(NewSubExplorer(deps))",
			Summary:         "RegisterDefaultSubAgents registers NewSubExplorer(deps)",
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisRegister,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "default SubAgent names",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"explorer"},
		SupportRefs: []string{
			"explorer: internal/agent/subagent.go:64",
		},
	}}
	ok, invalid := exhaustiveEnumerationMemberSetUsable(ctx, facts)
	if !ok {
		t.Fatalf("value-bearing support_refs against grounded evidence should be usable, invalid=%s", invalid)
	}

	facts[0].SupportRefs = []string{"other: internal/agent/subagent.go:64"}
	ok, _ = exhaustiveEnumerationMemberSetUsable(ctx, facts)
	if ok {
		t.Fatal("member-specific support_ref with a different label must not satisfy the member")
	}

	facts[0].Value = "2"
	facts[0].Members = []string{"explorer", "missing"}
	facts[0].SupportRefs = []string{"internal/agent/subagent.go:64"}
	ok, _ = exhaustiveEnumerationMemberSetUsable(ctx, facts)
	if ok {
		t.Fatal("a non-positional location-only support_ref must not satisfy every member in a larger set")
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

func TestReconcileCompletionAggregateFactsWithSourceInventory_DoesNotBroadenFunctionEntrySet(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "internal/analysis/aggregator/aggregator.go",
			Language: "go",
			Symbols: []repotypes.Symbol{
				{Name: "New", Kind: "function", File: "internal/analysis/aggregator/aggregator.go", Line: 112, Exported: true},
				{Name: "Aggregate", Kind: "function", File: "internal/analysis/aggregator/aggregator.go", Line: 132, Exported: true},
			},
		},
		{
			RelPath:  "internal/analysis/priority/score.go",
			Language: "go",
			Symbols: []repotypes.Symbol{
				{Name: "Score", Kind: "function", File: "internal/analysis/priority/score.go", Line: 34, Exported: true},
				{Name: "Raw", Kind: "function", File: "internal/analysis/priority/score.go", Line: 54, Exported: true},
			},
		},
	})
	ctx := sourceInventoryTestContext("", graph, "internal/analysis", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		RequestedFields: []types.SourceInventoryRequestedField{
			types.SourceInventoryFieldName,
			types.SourceInventoryFieldLocation,
			types.SourceInventoryFieldSummary,
		},
		Confidence: 0.95,
	})
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis 子包及入口函数",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"aggregator.New (internal/analysis/aggregator/aggregator.go:112)",
			"priority.Score (internal/analysis/priority/score.go:34)",
		},
		SupportRefs: []string{
			"New: internal/analysis/aggregator/aggregator.go:112",
			"Score: internal/analysis/priority/score.go:34",
		},
	}}

	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	if len(got) != 1 {
		t.Fatalf("facts len = %d, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0].Members, facts[0].Members) {
		t.Fatalf("generic function source inventory must not broaden a model-authored role/member relation set:\n got=%#v\nwant=%#v", got[0].Members, facts[0].Members)
	}
	for _, banned := range []string{"Aggregate", "Raw", "system:source_inventory"} {
		if strings.Contains(strings.Join(got[0].Members, "\n"), banned) ||
			strings.Contains(strings.Join(got[0].SupportRefs, "\n"), banned) ||
			strings.Contains(got[0].Provenance, banned) {
			t.Fatalf("function inventory broadened principal entry set with %q: %+v", banned, got[0])
		}
	}
}

func TestReconcileCompletionAggregateFactsWithSourceInventory_FileScopeDoesNotBroadenToDirectory(t *testing.T) {
	repo := t.TempDir()
	evidenceSource := `package types

// EvidenceKind classifies evidence rows.
type EvidenceKind string
// GroundingStatus classifies grounding results.
type GroundingStatus string

const (
	EvidenceDirect EvidenceKind = "direct"
	GroundingAccepted GroundingStatus = "grounded"
)
`
	otherSource := `package types

// OtherKind belongs to a sibling file and must not leak into a file-scoped inventory.
type OtherKind string

const (
	OtherValue OtherKind = "other"
)
`
	writeAggregateReconcileTestFile(t, repo, "internal/types/evidence.go", evidenceSource)
	writeAggregateReconcileTestFile(t, repo, "internal/types/other.go", otherSource)
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "internal/types/evidence.go",
			Language: "go",
			Symbols: []repotypes.Symbol{
				{Name: "EvidenceKind", Kind: "type", File: "internal/types/evidence.go", Line: 4, EndLine: 4, Exported: true, Doc: "// EvidenceKind classifies evidence rows."},
				{Name: "GroundingStatus", Kind: "type", File: "internal/types/evidence.go", Line: 6, EndLine: 6, Exported: true, Doc: "// GroundingStatus classifies grounding results."},
			},
		},
		{
			RelPath:  "internal/types/other.go",
			Language: "go",
			Symbols: []repotypes.Symbol{
				{Name: "OtherKind", Kind: "type", File: "internal/types/other.go", Line: 4, EndLine: 4, Exported: true, Doc: "// OtherKind belongs to a sibling file and must not leak into a file-scoped inventory."},
			},
		},
	})
	ctx := sourceInventoryTestContext(repo, graph, "internal/types/evidence.go", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
		TypeUnderlying:    types.SourceInventoryTypeUnderlyingString,
		RequiresConstSet:  true,
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
		Confidence:        0.95,
	})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "internal/types/evidence.go public string enum types",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"EvidenceKind"},
	}}

	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	want := []string{"EvidenceKind", "GroundingStatus"}
	if !reflect.DeepEqual(got[0].Members, want) {
		t.Fatalf("file-scoped source inventory members = %#v, want %#v", got[0].Members, want)
	}
	if containsString(got[0].Members, "OtherKind") || strings.Contains(strings.Join(got[0].SupportRefs, " "), "other.go") {
		t.Fatalf("sibling file enum leaked into file-scoped inventory: members=%#v refs=%#v", got[0].Members, got[0].SupportRefs)
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

func TestSourceInventoryScopeForSurface_NormalizesPathForms(t *testing.T) {
	repo := t.TempDir()
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "internal/types/evidence.go"},
		{RelPath: "internal/types/other.go"},
		{RelPath: "internal/tool/source_inventory.go"},
	})
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{raw: "./internal/types", want: "internal/types"},
		{raw: `.\internal\types`, want: "internal/types"},
		{raw: "internal/types/../types", want: "internal/types"},
		{raw: filepath.Join(repo, "internal", "types"), want: "internal/types"},
		{raw: filepath.Join(repo, "internal", "types", "evidence.go"), want: "internal/types/evidence.go"},
		{raw: filepath.Join(repo, "missing", "types"), want: ""},
	} {
		if got := sourceInventoryScopeForSurface(graph, tc.raw); got != tc.want {
			t.Fatalf("scope(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestSourceInventoryRequestedScopes_UsesListFilesToolScope(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/a.go"},
		{RelPath: "src/nested/b.py"},
		{RelPath: "other/c.go"},
	})
	ctx := sourceInventoryTestContext("", graph, "", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		Confidence:        0.95,
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "list_files",
		Success:  true,
		Summary:  "[list_files: path=src recursive=false]\nsrc/a.go\nsrc/nested\n",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindListFiles,
			Path:           "src",
			ResultCount:    2,
			CandidateFiles: []string{"src/a.go", "src/nested"},
		},
	}}})

	got := sourceInventoryRequestedScopes(ctx, graph)
	if !reflect.DeepEqual(got, []string{"src"}) {
		t.Fatalf("requested scopes = %#v, want src from list_files banner", got)
	}
}

func TestReconcileCompletionAggregateFactsWithSourceInventory_ListFilesScope(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/a.py", Language: "python", Symbols: []repotypes.Symbol{{Name: "handle", Kind: "function", File: "src/a.py", Line: 7, Exported: true}}},
		{RelPath: "src/B.java", Language: "java", Symbols: []repotypes.Symbol{{Name: "Run", Kind: "function", File: "src/B.java", Line: 20, Exported: true}}},
		{RelPath: "other/c.go", Language: "go", Symbols: []repotypes.Symbol{{Name: "Eval", Kind: "function", File: "other/c.go", Line: 1, Exported: true}}},
	})
	ctx := sourceInventoryTestContext("", graph, "", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		Confidence:        0.95,
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "list_files",
		Success:  true,
		Summary:  "[list_files: path=src recursive=false]\nsrc/a.py\nsrc/B.java\n",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindListFiles,
			Path:           "src",
			ResultCount:    2,
			CandidateFiles: []string{"src/a.py", "src/B.java"},
		},
	}}})
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public entry candidates",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"handle"},
	}}

	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	want := []string{"handle"}
	if !reflect.DeepEqual(got[0].Members, want) {
		t.Fatalf("members = %#v, want model-authored function set preserved %#v", got[0].Members, want)
	}
	if containsString(got[0].Members, "Eval") {
		t.Fatalf("out-of-scope function leaked into source inventory: %#v", got[0].Members)
	}
	if containsString(got[0].Members, "Run") {
		t.Fatalf("system source-inventory must not broaden entry-function requests into every scoped function: %#v", got[0].Members)
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
			name:     "arkts decorator metadata is preserved as structural surface",
			lang:     repotypes.LangArkTS,
			symbol:   "RuntimePanel",
			doc:      "@Component @Entry",
			wantNote: true,
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
			if tc.lang == repotypes.LangArkTS && got != "" && !strings.HasPrefix(got, "surface=@") {
				t.Fatalf("ArkTS decorator metadata should stay structural surface, got %q", got)
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

func TestReconcileCompletionAggregateFactsWithSourceInventory_DoesNotRewriteGraphBackedPublicFunctions(t *testing.T) {
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
	want := []string{"Eval", "EvalAll"}
	if !reflect.DeepEqual(got[0].Members, want) {
		t.Fatalf("members = %#v, want %#v", got[0].Members, want)
	}
	if containsString(got[0].Members, "SetExternalArtifactFloor") {
		t.Fatalf("system source-inventory must not broaden model-authored function sets: %#v", got[0].Members)
	}
	for _, banned := range []string{"parseComparison", "registered"} {
		if containsString(got[0].Members, banned) {
			t.Fatalf("private/helper member %q leaked into public function inventory: %#v", banned, got[0].Members)
		}
	}
}

func TestReconcileCompletionAggregateFactsWithSourceInventory_RelationMemberSetNotRewritten(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "internal/agent/agent.go", Language: "go", Symbols: []repotypes.Symbol{
			{Name: "StageOutput", Kind: "type", File: "internal/agent/agent.go", Line: 24, Exported: true},
			{Name: "LoopController", Kind: "interface", File: "internal/agent/agent.go", Line: 430, Exported: true},
			{Name: "ToolSchemaFilter", Kind: "type", File: "internal/agent/agent.go", Line: 259, Exported: true},
		}},
		{RelPath: "internal/agent/analyzer.go", Language: "go", Symbols: []repotypes.Symbol{{
			Name: "analyzerEvaluator", Kind: "struct", File: "internal/agent/analyzer.go", Line: 46,
		}}},
		{RelPath: "internal/agent/log_triager.go", Language: "go", Symbols: []repotypes.Symbol{{
			Name: "logTriagerEvaluator", Kind: "struct", File: "internal/agent/log_triager.go", Line: 99,
		}}},
	})
	ctx := sourceInventoryTestContext("", graph, "internal/agent/agent.go", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
		Confidence:        0.95,
	})
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisImplement
	ctx.AnalysisIR.RequestModel.Predicates.IsRelationalLookup = true
	facts := []types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "LoopController production implementers",
		Value:      "2",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: "emit_investigation_complete.aggregate_facts, typed_graph",
		Members:    []string{"analyzerEvaluator", "logTriagerEvaluator"},
		SupportRefs: []string{
			"analyzerEvaluator: internal/agent/analyzer.go:46",
			"logTriagerEvaluator: internal/agent/log_triager.go:99",
		},
	}}

	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	if !reflect.DeepEqual(got[0].Members, facts[0].Members) {
		t.Fatalf("relation member set was rewritten by source inventory:\ngot  %#v\nwant %#v", got[0].Members, facts[0].Members)
	}
	for _, banned := range []string{"StageOutput", "LoopController", "ToolSchemaFilter"} {
		if containsString(got[0].Members, banned) {
			t.Fatalf("source inventory member %q leaked into relation member set: %#v", banned, got[0].Members)
		}
	}
}

func TestReconcileCompletionAggregateFactsWithSourceInventory_MixedLanguageGraphBackedPublicFunctions(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/a.go", Language: "go", Symbols: []repotypes.Symbol{{Name: "Eval", Kind: "function", File: "src/a.go", Line: 1, Exported: true}}},
		{RelPath: "src/B.java", Language: "java", Symbols: []repotypes.Symbol{{Name: "Run", Kind: "function", File: "src/B.java", Line: 1, Exported: true}}},
		{RelPath: "src/c.ts", Language: "typescript", Symbols: []repotypes.Symbol{{Name: "render", Kind: "function", File: "src/c.ts", Line: 12, Exported: false}}},
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
	want := []string{"Eval"}
	if !reflect.DeepEqual(got[0].Members, want) {
		t.Fatalf("mixed-language function candidates should stay support-only and not rewrite the model set;\ngot  %#v\nwant %#v", got[0].Members, want)
	}
	if containsString(got[0].Members, "render") {
		t.Fatalf("non-exported TypeScript function leaked into public inventory: %#v", got[0].Members)
	}
}

func TestReconcileCompletionAggregateFactsWithSourceInventory_PreservesExplorerFunctionNotesWithoutReplacing(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/a.go", Language: "go", Symbols: []repotypes.Symbol{{
			Name:     "Eval",
			Kind:     "function",
			File:     "src/a.go",
			Line:     10,
			Exported: true,
			Doc:      "// Eval evaluates one Criterion against the current environment.",
		}}},
		{RelPath: "src/B.java", Language: "java", Symbols: []repotypes.Symbol{{
			Name:     "Run",
			Kind:     "function",
			File:     "src/B.java",
			Line:     20,
			Exported: true,
			Doc:      "/** Run executes the Java entrypoint. */",
		}}},
		{RelPath: "src/c.ts", Language: "typescript", Symbols: []repotypes.Symbol{{
			Name:     "render",
			Kind:     "function",
			File:     "src/c.ts",
			Line:     30,
			Exported: false,
			Doc:      "/** render updates the private view. */",
		}}},
	})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		RequestedFields: []types.SourceInventoryRequestedField{
			types.SourceInventoryFieldName,
			types.SourceInventoryFieldLocation,
			types.SourceInventoryFieldSummary,
		},
		Confidence: 0.95,
	})
	facts := []types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "public functions",
		Value:      "1",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: "emit_investigation_complete.aggregate_facts",
		Members:    []string{"Eval", "render"},
		MemberNotes: []string{
			"Eval 对单个 Criterion 执行评估，未知 Kind 返回 UnknownKind。",
			"render 是探索早期误收集的私有函数。",
		},
		SupportRefs: []string{
			"Eval @ src/a.go:10",
			"render @ src/c.ts:30",
		},
	}}

	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	wantMembers := []string{"Eval", "render"}
	if !reflect.DeepEqual(got[0].Members, wantMembers) {
		t.Fatalf("members = %#v, want %#v", got[0].Members, wantMembers)
	}
	if len(got[0].MemberNotes) != 2 {
		t.Fatalf("member notes should remain model-authored, got %#v", got[0].MemberNotes)
	}
	if !strings.Contains(got[0].MemberNotes[0], "单个 Criterion") ||
		!strings.Contains(got[0].MemberNotes[1], "误收集") {
		t.Fatalf("system must not replace function notes with graph docs: %#v", got[0].MemberNotes)
	}
	if !reflect.DeepEqual(got[0].SupportRefs, facts[0].SupportRefs) {
		t.Fatalf("support refs should remain model-authored for function sets, got %#v", got[0].SupportRefs)
	}
	if got[0].Provenance != facts[0].Provenance {
		t.Fatalf("function source-inventory must not add rewrite provenance, got %q", got[0].Provenance)
	}
}

func TestBuildSourceInventoryAdvisory_ActiveProfileUsesGraphCandidates(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/a.py", Language: "python", Symbols: []repotypes.Symbol{{
			Name:     "Eval",
			Kind:     "function",
			File:     "src/a.py",
			Line:     10,
			Exported: true,
			Doc:      "Evaluate one item.",
		}}},
		{RelPath: "src/B.java", Language: "java", Symbols: []repotypes.Symbol{{
			Name:     "Run",
			Kind:     "function",
			File:     "src/B.java",
			Line:     20,
			Exported: true,
			Doc:      "/** Run executes the Java entrypoint. */",
		}}},
		{RelPath: "src/private.ts", Language: "typescript", Symbols: []repotypes.Symbol{{
			Name:     "render",
			Kind:     "function",
			File:     "src/private.ts",
			Line:     30,
			Exported: false,
		}}},
	})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
		Confidence:        0.95,
	})

	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
	if !advisory.IsActive() || advisory.AdvisoryOnly {
		t.Fatalf("advisory = %+v, want active authoritative-compatible", advisory)
	}
	if len(advisory.Sets) != 1 || advisory.Sets[0].Role != types.AnswerCandidateRoleFunction {
		t.Fatalf("sets = %+v, want one function set", advisory.Sets)
	}
	got := advisoryMemberNames(advisory.Sets[0].Candidates)
	want := []string{"Run", "Eval"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("members = %#v, want %#v", got, want)
	}
	if containsString(got, "render") {
		t.Fatalf("private TypeScript helper leaked into public advisory: %#v", got)
	}
}

func TestBuildSourceInventoryAdvisory_DedupesBasenameScopeAliases(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "internal/analysis/gate/gate.go", Language: "go", Package: "gate", Symbols: []repotypes.Symbol{{
			Name:     "Run",
			Kind:     "function",
			File:     "internal/analysis/gate/gate.go",
			Line:     127,
			Exported: true,
		}}},
		{RelPath: "internal/analysis/axis/matrix.go", Language: "go", Package: "axis", Symbols: []repotypes.Symbol{{
			Name:     "Affinity",
			Kind:     "function",
			File:     "internal/analysis/axis/matrix.go",
			Line:     30,
			Exported: true,
		}}},
	})
	ctx := sourceInventoryTestContext("", graph, "", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
		Confidence:        0.95,
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{
		"internal/analysis/gate",
		"internal/analysis/axis",
		"gate",
		"axis",
	}

	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
	if !advisory.IsActive() {
		t.Fatalf("advisory inactive: %+v", advisory)
	}
	if got, want := advisory.Scopes, []string{"internal/analysis/axis", "internal/analysis/gate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("scopes = %#v, want canonical full scopes %#v", got, want)
	}
	if len(advisory.Sets) != 1 {
		t.Fatalf("sets = %+v, want one package set", advisory.Sets)
	}
	gotMembers := advisoryMemberNames(advisory.Sets[0].Candidates)
	wantMembers := []string{"axis", "gate"}
	if !reflect.DeepEqual(gotMembers, wantMembers) {
		t.Fatalf("members = %#v, want %#v", gotMembers, wantMembers)
	}
	if got := len(advisory.Sets[0].Candidates); got != 2 {
		t.Fatalf("candidate count = %d, want no basename duplicates", got)
	}
}

func TestBuildSourceInventoryAdvisory_TypedRoleFallbackIsAdvisoryOnly(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/a.py", Language: "python", Symbols: []repotypes.Symbol{{
			Name:     "Eval",
			Kind:     "function",
			File:     "src/a.py",
			Line:     10,
			Exported: true,
		}}},
		{RelPath: "src/B.java", Language: "java", Symbols: []repotypes.Symbol{{
			Name:     "Run",
			Kind:     "function",
			File:     "src/B.java",
			Line:     20,
			Exported: true,
		}}},
	})
	ctx := sourceInventoryTestContext("", graph, "src", nil)
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisDefine
	ctx.AnalysisIR.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all functions",
	}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"src", "src/a.py"}
	ctx.AnalysisIR.RequestModel.AnswerRoleProfile = &types.AnswerRoleProfile{
		IsRoleBindingRequested: true,
		RequiredCandidateRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
	}

	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
	if !advisory.IsActive() || !advisory.AdvisoryOnly {
		t.Fatalf("advisory = %+v, want active advisory-only", advisory)
	}
	if !containsString(advisory.Provenance, "answer_role_profile") {
		t.Fatalf("provenance = %#v, want answer_role_profile", advisory.Provenance)
	}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "functions",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval"},
	}}
	got := reconcileCompletionAggregateFactsWithSourceInventory(ctx, facts, nil)
	if !reflect.DeepEqual(got, facts) {
		t.Fatalf("advisory-only fallback must not rewrite aggregate facts;\ngot  %#v\nwant %#v", got, facts)
	}
}

func TestBuildSourceInventoryAdvisory_NoTypedRoleStaysSilent(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/a.py",
		Language: "python",
		Symbols: []repotypes.Symbol{{
			Name:     "Eval",
			Kind:     "function",
			File:     "src/a.py",
			Line:     10,
			Exported: true,
		}},
	}})
	ctx := sourceInventoryTestContext("", graph, "src", nil)
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisDefine
	ctx.AnalysisIR.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all functions",
	}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"src", "src/a.py"}

	if advisory := buildSourceInventoryAdvisory(ctx, nil, nil); advisory.IsActive() {
		t.Fatalf("advisory without typed role/profile should stay silent: %+v", advisory)
	}
}

func TestBuildSourceInventoryAdvisory_BoundedScopeFallbackIsAdvisoryOnly(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/alpha/a.py", Language: "python", Package: "alpha", Symbols: []repotypes.Symbol{{
			Name: "run_alpha", Kind: "function", File: "src/alpha/a.py", Line: 7, Exported: true,
		}}},
		{RelPath: "src/alpha/b.java", Language: "java", Package: "alpha", Symbols: []repotypes.Symbol{{
			Name: "execute", Kind: "method", File: "src/alpha/b.java", Line: 11, Parent: "Beta", Exported: true,
		}}},
		{RelPath: "src/alpha/c.ts", Language: "typescript"},
		{RelPath: "src/alpha/d.kt", Language: "kotlin"},
		{RelPath: "src/alpha/e.proto", Language: "proto"},
		{RelPath: "src/alpha/f.cj", Language: "cangjie"},
	})
	ctx := sourceInventoryTestContext("", graph, "", nil)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = nil
	ctx.AnalysisIR.EvidencePlan.RequiredFiles = []string{
		"src/alpha/a.py",
		"src/alpha/b.java",
		"src/alpha/c.ts",
		"src/alpha/d.kt",
		"src/alpha/e.proto",
		"src/alpha/f.cj",
	}

	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
	if !advisory.IsActive() || !advisory.AdvisoryOnly {
		t.Fatalf("bounded scope fallback advisory = %+v, want active advisory-only", advisory)
	}
	if !containsString(advisory.Provenance, "request_traits:bounded_source_enumeration_scope") {
		t.Fatalf("provenance = %#v, want bounded_source_enumeration_scope", advisory.Provenance)
	}
	if len(advisory.Sets) != 1 || advisory.Sets[0].Role != types.AnswerCandidateRolePackage {
		t.Fatalf("sets = %+v, want advisory package candidates only", advisory.Sets)
	}
	if got := advisoryMemberNames(advisory.Sets[0].Candidates); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("package candidates = %#v, want [alpha]", got)
	}
	attrs := advisory.Sets[0].Candidates[0].Attributes
	if len(attrs) == 0 {
		t.Fatalf("bounded fallback package candidate should carry graph-backed callable attributes: %+v", advisory)
	}
	if attrs[0].Member != "run_alpha" {
		t.Fatalf("first callable attribute = %q, want run_alpha (attrs=%+v)", attrs[0].Member, attrs)
	}
}

func TestBuildSourceInventoryAdvisory_BoundedScopeFallbackRespectsTypedRoleProfile(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/alpha/a.py", Language: "python", Package: "alpha", Symbols: []repotypes.Symbol{{
			Name: "run_alpha", Kind: "function", File: "src/alpha/a.py", Line: 7, Exported: true,
		}}},
		{RelPath: "src/alpha/b.java", Language: "java"},
		{RelPath: "src/alpha/c.ts", Language: "typescript"},
		{RelPath: "src/alpha/d.kt", Language: "kotlin"},
		{RelPath: "src/alpha/e.proto", Language: "proto"},
		{RelPath: "src/alpha/f.cj", Language: "cangjie"},
	})
	ctx := sourceInventoryTestContext("", graph, "", nil)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = nil
	ctx.AnalysisIR.RequestModel.AnswerRoleProfile = &types.AnswerRoleProfile{
		IsRoleBindingRequested: true,
		RequiredCandidateRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
	}
	ctx.AnalysisIR.EvidencePlan.RequiredFiles = []string{
		"src/alpha/a.py",
		"src/alpha/b.java",
		"src/alpha/c.ts",
		"src/alpha/d.kt",
		"src/alpha/e.proto",
		"src/alpha/f.cj",
	}

	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
	if !advisory.IsActive() || !advisory.AdvisoryOnly {
		t.Fatalf("bounded scope fallback advisory = %+v, want active advisory-only", advisory)
	}
	if len(advisory.Sets) != 1 || advisory.Sets[0].Role != types.AnswerCandidateRoleFunction {
		t.Fatalf("sets = %+v, want model-suggested function role", advisory.Sets)
	}
	if got := advisoryMemberNames(advisory.Sets[0].Candidates); !reflect.DeepEqual(got, []string{"run_alpha"}) {
		t.Fatalf("function candidates = %#v, want [run_alpha]", got)
	}
}

func TestBuildSourceInventoryAdvisory_SourceScopeEntityFallbackIsAdvisoryOnly(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/alpha/a.py", Language: "python", Package: "alpha", Symbols: []repotypes.Symbol{{
			Name: "run_alpha", Kind: "function", File: "src/alpha/a.py", Line: 7, Exported: true,
		}}},
		{RelPath: "src/beta/B.java", Language: "java", Package: "com.example.beta", Symbols: []repotypes.Symbol{{
			Name: "execute", Kind: "method", File: "src/beta/B.java", Line: 11, Parent: "Beta", Exported: true,
		}}},
	})
	ctx := sourceInventoryTestContext("", graph, "src", nil)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = nil
	ctx.AnalysisIR.EvidencePlan.RequiredFiles = nil

	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
	if !advisory.IsActive() || !advisory.AdvisoryOnly {
		t.Fatalf("source-scope entity fallback advisory = %+v, want active advisory-only", advisory)
	}
	if !containsString(advisory.Provenance, "request_traits:source_scope_enumeration") {
		t.Fatalf("provenance = %#v, want source_scope_enumeration", advisory.Provenance)
	}
	if len(advisory.Sets) != 1 || advisory.Sets[0].Role != types.AnswerCandidateRolePackage {
		t.Fatalf("sets = %+v, want package/directory/module scope candidates", advisory.Sets)
	}
	if got := advisoryMemberNames(advisory.Sets[0].Candidates); !reflect.DeepEqual(got, []string{"alpha", "com.example.beta"}) {
		t.Fatalf("scope candidates = %#v", got)
	}
}

func TestBuildSourceInventoryAdvisory_FileRoleUsesGraphFiles(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{RelPath: "src/a.py", Language: "python", Symbols: []repotypes.Symbol{{
			Name:     "Eval",
			Kind:     "function",
			File:     "src/a.py",
			Line:     10,
			Exported: true,
		}}},
		{RelPath: "src/B.java", Language: "java", Symbols: []repotypes.Symbol{{
			Name:     "Run",
			Kind:     "function",
			File:     "src/B.java",
			Line:     20,
			Exported: true,
		}}},
		{RelPath: "README.md", Language: "", IsSpecial: true},
	})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFile},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
		Confidence:        0.95,
	})

	advisory := buildSourceInventoryAdvisory(ctx, nil, nil)
	if !advisory.IsActive() || len(advisory.Sets) != 1 {
		t.Fatalf("advisory = %+v, want one file-role set", advisory)
	}
	got := advisoryMemberNames(advisory.Sets[0].Candidates)
	want := []string{"src/B.java", "src/a.py"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("file candidates = %#v, want %#v", got, want)
	}
}

func TestPublishSourceInventoryAdvisoryFromTypedRequest_AdvisoryOnlyPackageAndFunctions(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "src/alpha/a.py",
			Language: "python",
			Package:  "alpha",
			Symbols: []repotypes.Symbol{{
				Name:     "run_alpha",
				Kind:     "function",
				File:     "src/alpha/a.py",
				Line:     7,
				Exported: true,
			}},
		},
		{
			RelPath:  "src/beta/B.java",
			Language: "java",
			Package:  "com.example.beta",
			Symbols: []repotypes.Symbol{{
				Name:     "RunBeta",
				Kind:     "function",
				File:     "src/beta/B.java",
				Line:     11,
				Exported: true,
			}},
		},
	})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles: []types.AnswerCandidateRole{
			types.AnswerCandidateRolePackage,
			types.AnswerCandidateRoleFunction,
		},
		RequestedFields: []types.SourceInventoryRequestedField{
			types.SourceInventoryFieldName,
			types.SourceInventoryFieldLocation,
		},
		Confidence: 0.95,
	})

	if !PublishSourceInventoryAdvisoryFromTypedRequest(ctx) {
		t.Fatal("expected typed request to publish pre-completion advisory")
	}
	advisory := ctx.Mutable.SourceInventoryAdvisory()
	if !advisory.IsActive() || !advisory.AdvisoryOnly {
		t.Fatalf("advisory = %+v, want active advisory-only", advisory)
	}
	if !containsString(advisory.Provenance, "pre_explore_typed_request") {
		t.Fatalf("provenance = %#v, want pre_explore_typed_request", advisory.Provenance)
	}
	var packages, functions []string
	for _, set := range advisory.Sets {
		switch set.Role {
		case types.AnswerCandidateRolePackage:
			packages = advisoryMemberNames(set.Candidates)
		case types.AnswerCandidateRoleFunction:
			functions = advisoryMemberNames(set.Candidates)
		}
	}
	if !reflect.DeepEqual(packages, []string{"alpha", "com.example.beta"}) {
		t.Fatalf("package candidates = %#v", packages)
	}
	if !reflect.DeepEqual(functions, []string{"run_alpha", "RunBeta"}) {
		t.Fatalf("function candidates = %#v", functions)
	}
}

func TestPublishSourceInventoryAdvisoryFromTypedRequest_QuerySynthesizesProfileAndFiltersLanguage(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			Language: "arkts",
			Symbols: []repotypes.Symbol{{
				Name: "Index", Kind: "component", File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", Line: 7, Exported: true, Doc: "@Entry @Component",
			}},
		},
		{
			RelPath:  "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
			Language: "arkts",
			Symbols: []repotypes.Symbol{{
				Name: "GlobalCard", Kind: "builder", File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", Line: 26, Exported: true, Doc: "@Builder",
			}},
		},
		{
			RelPath:  "internal/tool/repomap/index/extract_arkts.go",
			Language: "go",
			Symbols: []repotypes.Symbol{{
				Name: "builderFunctionRegex", Kind: "function", File: "internal/tool/repomap/index/extract_arkts.go", Line: 130, Exported: false, Doc: "@Builder parser helper",
			}},
		},
	})
	mut := types.NewMutableState("typed source enumeration query")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			AnalyzerHints: types.AnalyzerHints{
				Keywords: []string{"@Entry", "@Builder", "ArkTS"},
				Entities: []string{"@Entry", "@Builder", "ArkTS"},
			},
		}},
	}

	if !PublishSourceInventoryAdvisoryFromTypedRequest(ctx) {
		t.Fatal("expected typed source enumeration query to publish advisory")
	}
	advisory := ctx.Mutable.SourceInventoryAdvisory()
	if !advisory.IsActive() || !advisory.AdvisoryOnly {
		t.Fatalf("advisory = %+v, want active advisory-only", advisory)
	}
	for _, want := range []string{
		"request_traits:typed_source_enumeration_query",
		"request_traits:query_root_scope",
		"pre_explore_typed_request",
	} {
		if !containsString(advisory.Provenance, want) {
			t.Fatalf("provenance = %#v, want %s", advisory.Provenance, want)
		}
	}
	gotByRole := map[types.AnswerCandidateRole][]string{}
	for _, set := range advisory.Sets {
		gotByRole[set.Role] = advisoryMemberNames(set.Candidates)
	}
	if !reflect.DeepEqual(gotByRole[types.AnswerCandidateRoleType], []string{"Index"}) {
		t.Fatalf("type candidates = %#v, want [Index] (advisory=%+v)", gotByRole[types.AnswerCandidateRoleType], advisory)
	}
	if !reflect.DeepEqual(gotByRole[types.AnswerCandidateRoleFunction], []string{"GlobalCard"}) {
		t.Fatalf("function candidates = %#v, want [GlobalCard] (advisory=%+v)", gotByRole[types.AnswerCandidateRoleFunction], advisory)
	}
	for _, names := range gotByRole {
		for _, name := range names {
			if name == "builderFunctionRegex" {
				t.Fatalf("language-filtered source inventory leaked Go parser helper: %+v", advisory)
			}
		}
	}
	obs := ctx.Mutable.SourceInventoryObservation()
	if !obs.IsActive() {
		t.Fatalf("source inventory advisory should maintain observation companion: %+v", obs)
	}
	if !SourceInventoryLensExecutionGapForContext(ctx).Blocking {
		t.Fatal("advisory-only typed query lane should still require executable lens before auto-observation")
	}
	if !PublishSourceInventoryObservationFromTypedRequest(ctx) {
		t.Fatal("expected typed query lane to publish deterministic lens observation")
	}
	obs = ctx.Mutable.SourceInventoryObservation()
	if !sourceInventoryObservationHasRepoLensToolQuery(obs) {
		t.Fatalf("auto-observed source inventory missing repo_lens provenance: %+v", obs)
	}
	if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking {
		t.Fatalf("auto-observed source inventory should satisfy lens gate, got %+v", gap)
	}
}

func TestPublishSourceInventoryAdvisoryFromTypedRequest_LowConfidenceProfileUsesQueryRootScope(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			Language: "arkts",
			Symbols: []repotypes.Symbol{{
				Name: "Index", Kind: "component", File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", Line: 7, Exported: true, Doc: "@Entry @Component",
			}},
		},
		{
			RelPath:  "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
			Language: "arkts",
			Symbols: []repotypes.Symbol{{
				Name: "GlobalCard", Kind: "builder", File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", Line: 26, Exported: true, Doc: "@Builder",
			}},
		},
	})
	mut := types.NewMutableState("low-confidence source inventory profile")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			AnalyzerHints: types.AnalyzerHints{
				Keywords: []string{"@Entry", "@Builder", "ArkTS"},
				Entities: []string{"@Entry", "@Builder", "ArkTS"},
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleFunction,
					types.AnswerCandidateRoleMethod,
					types.AnswerCandidateRoleType,
				},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
				},
				Confidence: 0.45,
			},
		}},
	}

	if !PublishSourceInventoryAdvisoryFromTypedRequest(ctx) {
		t.Fatal("expected low-confidence inventory profile to use typed query root scope")
	}
	advisory := ctx.Mutable.SourceInventoryAdvisory()
	for _, want := range []string{
		"source_inventory_profile:low_confidence",
		"request_traits:query_root_scope",
		"pre_explore_typed_request",
	} {
		if !containsString(advisory.Provenance, want) {
			t.Fatalf("provenance = %#v, want %s", advisory.Provenance, want)
		}
	}
	gotByRole := map[types.AnswerCandidateRole][]string{}
	for _, set := range advisory.Sets {
		gotByRole[set.Role] = advisoryMemberNames(set.Candidates)
	}
	if !reflect.DeepEqual(gotByRole[types.AnswerCandidateRoleType], []string{"Index"}) ||
		!reflect.DeepEqual(gotByRole[types.AnswerCandidateRoleFunction], []string{"GlobalCard"}) {
		t.Fatalf("low-confidence query-root advisory did not preserve ArkTS candidates: roles=%+v advisory=%+v", gotByRole, advisory)
	}
}

func TestPublishSourceInventoryAdvisory_PackageCandidatesCarryCrossLanguageCallableAttributes(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "src/alpha/a.py",
			Language: "python",
			Package:  "alpha",
			Symbols: []repotypes.Symbol{
				{Name: "Alpha", Kind: "class", File: "src/alpha/a.py", Line: 3, Exported: true},
				{Name: "run_alpha", Kind: "function", File: "src/alpha/a.py", Line: 9, Exported: true},
			},
		},
		{
			RelPath:  "src/beta/B.java",
			Language: "java",
			Package:  "com.example.beta",
			Symbols: []repotypes.Symbol{
				{Name: "Beta", Kind: "class", File: "src/beta/B.java", Line: 4, Exported: true},
				{Name: "execute", Kind: "method", File: "src/beta/B.java", Line: 18, Parent: "Beta", Exported: true},
			},
		},
		{
			RelPath:  "src/ui/Index.ets",
			Language: "arkts",
			Package:  "ui",
			Symbols: []repotypes.Symbol{
				{Name: "Index", Kind: "component", File: "src/ui/Index.ets", Line: 6, Exported: true},
				{Name: "build", Kind: "ui-entry", File: "src/ui/Index.ets", Line: 24, Parent: "Index", Exported: true},
			},
		},
		{
			RelPath:  "src/proto/user.proto",
			Language: "proto",
			Package:  "user.v1",
			Symbols: []repotypes.Symbol{
				{Name: "UserService", Kind: "service", File: "src/proto/user.proto", Line: 8, Exported: true},
				{Name: "GetUser", Kind: "rpc", File: "src/proto/user.proto", Line: 11, Parent: "UserService", Exported: true},
			},
		},
		{
			RelPath:  "src/cj/main.cj",
			Language: "cangjie",
			Package:  "cj",
			Symbols: []repotypes.Symbol{{
				Name: "main", Kind: "function", File: "src/cj/main.cj", Line: 5, Exported: true,
			}},
		},
	})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName},
		Confidence:        0.95,
	})

	if !PublishSourceInventoryAdvisoryFromTypedRequest(ctx) {
		t.Fatal("expected typed request to publish package advisory")
	}
	advisory := ctx.Mutable.SourceInventoryAdvisory()
	var packageSet *types.SourceInventoryAdvisorySet
	for i := range advisory.Sets {
		if advisory.Sets[i].Role == types.AnswerCandidateRolePackage {
			packageSet = &advisory.Sets[i]
			break
		}
	}
	if packageSet == nil {
		t.Fatalf("package set missing from advisory: %+v", advisory)
	}
	attrsByMember := map[string][]types.SourceInventoryAdvisoryAttribute{}
	for _, candidate := range packageSet.Candidates {
		attrsByMember[candidate.Member] = candidate.Attributes
	}
	for member, want := range map[string]string{
		"alpha":            "run_alpha",
		"com.example.beta": "execute",
		"ui":               "build",
		"user.v1":          "GetUser",
		"cj":               "main",
	} {
		attrs := attrsByMember[member]
		if len(attrs) == 0 {
			t.Fatalf("package candidate %q has no callable attributes; advisory=%+v", member, advisory)
		}
		if attrs[0].Member != want {
			t.Fatalf("package %q first attribute = %q, want %q (attrs=%+v)", member, attrs[0].Member, want, attrs)
		}
		if attrs[0].File == "" || attrs[0].Line == 0 || attrs[0].Language == "" {
			t.Fatalf("package %q attribute missing location/language: %+v", member, attrs[0])
		}
	}
}

func TestPublishSourceInventoryAdvisoryFromToolObservation_FirstActivationReturnsHint(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/alpha/a.py",
		Language: "python",
		Package:  "alpha",
		Symbols: []repotypes.Symbol{{
			Name:     "run_alpha",
			Kind:     "function",
			File:     "src/alpha/a.py",
			Line:     7,
			Exported: true,
		}},
	}})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName},
		Confidence:        0.95,
	})

	hint := PublishSourceInventoryAdvisoryFromToolObservation(ctx, types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
	})
	if !strings.Contains(hint, "Structured source-inventory candidate checklist") ||
		!strings.Contains(hint, "run_alpha@src/alpha/a.py:7") {
		t.Fatalf("hint did not expose compact advisory: %q", hint)
	}
	for _, want := range []string{
		"## Cascaded Repo Lens Guide (advisory)",
		"scope_groups=1 candidate_files=1 candidate_items=1 ambiguous_groups=0",
		"`src/alpha` — files=1 candidates=1 roles=function:1 languages=python:1",
		"repo_map {\"path\": \"<same repo path>\", \"view\": \"source_inventory\", \"scope\": \"src/alpha\"",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint missing cascaded repo lens guide %q:\n%s", want, hint)
		}
	}
	if guide := strings.Index(hint, "## Cascaded Repo Lens Guide (advisory)"); guide < 0 {
		t.Fatalf("hint missing cascaded repo lens guide:\n%s", hint)
	} else if rows := strings.Index(hint, "- function candidates:"); rows < 0 || rows < guide {
		t.Fatalf("hint should surface cascade guide before compact candidate rows:\n%s", hint)
	}
	if second := PublishSourceInventoryAdvisoryFromToolObservation(ctx, types.ToolResult{
		ToolName: "list_files",
		Success:  true,
	}); second != "" {
		t.Fatalf("second observation should not spam another hint, got %q", second)
	}
}

func TestPublishSourceInventoryAdvisoryFromToolObservation_RendersPackageAsScopeCarrier(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/alpha/a.py",
		Language: "python",
		Package:  "alpha",
	}})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName},
		Confidence:        0.95,
	})

	hint := PublishSourceInventoryAdvisoryFromToolObservation(ctx, types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
	})
	if !strings.Contains(hint, "package/directory/module scope candidates") {
		t.Fatalf("package advisory should render as scope carrier, got %q", hint)
	}
}

func TestPublishSourceInventoryAdvisoryFromToolObservation_PreActiveStillHintsOnce(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{{
		RelPath:  "src/alpha/a.py",
		Language: "python",
		Package:  "alpha",
		Symbols: []repotypes.Symbol{{
			Name:     "run_alpha",
			Kind:     "function",
			File:     "src/alpha/a.py",
			Line:     7,
			Exported: true,
		}},
	}})
	ctx := sourceInventoryTestContext("", graph, "src", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
		RequestedFields:   []types.SourceInventoryRequestedField{types.SourceInventoryFieldName},
		Confidence:        0.95,
	})
	if !PublishSourceInventoryAdvisoryFromTypedRequest(ctx) {
		t.Fatal("expected pre-explore advisory to publish")
	}

	first := PublishSourceInventoryAdvisoryFromToolObservation(ctx, types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
	})
	if !strings.Contains(first, "Structured source-inventory candidate checklist") ||
		!strings.Contains(first, "run_alpha") {
		t.Fatalf("pre-active advisory should still attach one compact tool hint, got %q", first)
	}
	second := PublishSourceInventoryAdvisoryFromToolObservation(ctx, types.ToolResult{
		ToolName: "list_files",
		Success:  true,
	})
	if second != "" {
		t.Fatalf("pre-active advisory hint should emit only once, got %q", second)
	}
}

func TestPublishSourceInventoryObservationFromLens_ModelDrivenRolesAndScopes(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "src/alpha/a.py",
			Language: "python",
			Package:  "alpha",
			Symbols: []repotypes.Symbol{{
				Name:     "run_alpha",
				Kind:     "function",
				File:     "src/alpha/a.py",
				Line:     7,
				Exported: true,
			}},
		},
		{
			RelPath:  "src/beta/B.java",
			Language: "java",
			Package:  "com.example.beta",
			Symbols: []repotypes.Symbol{{
				Name:     "RunBeta",
				Kind:     "function",
				File:     "src/beta/B.java",
				Line:     11,
				Exported: true,
			}},
		},
	})
	ctx := sourceInventoryTestContext("", graph, "", nil)

	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Scopes:            []string{"src"},
		Roles:             []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRolePackage},
		IncludeAttributes: true,
		IncludeCounts:     true,
	})
	if !obs.IsActive() || len(obs.Sets) != 2 {
		t.Fatalf("observation = %+v, want two active sets", obs)
	}
	if obs.Sets[0].Role != types.AnswerCandidateRoleFunction ||
		obs.Sets[1].Role != types.AnswerCandidateRolePackage {
		t.Fatalf("role order should preserve model query order, got %+v", obs.Sets)
	}
	if obs.Sets[0].Count != 2 || len(obs.Sets[0].Members) != 2 {
		t.Fatalf("function count/list invariant broken: %+v", obs.Sets[0])
	}
	if stored := ctx.Mutable.SourceInventoryObservation(); !stored.IsActive() || stored.Sets[0].Count != 2 {
		t.Fatalf("observation not stored on mutable: %+v", stored)
	}
	rendered := RenderSourceInventoryObservationView(obs, types.SourceInventoryLensQuery{
		IncludeAttributes: true,
		IncludeCounts:     true,
	})
	for _, want := range []string{
		"Repo Lens: Source Inventory",
		"count=2",
		"`run_alpha` @ src/alpha/a.py:7",
		"support_ref=`run_alpha: src/alpha/a.py:7`",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered lens missing %q:\n%s", want, rendered)
		}
	}
	paged := RenderSourceInventoryObservationView(obs, types.SourceInventoryLensQuery{
		IncludeCounts: true,
		TopN:          1,
		Cursor:        "1",
	})
	if !strings.Contains(paged, "page_offset: 1") ||
		!strings.Contains(paged, "`RunBeta`") ||
		!strings.Contains(paged, "@ src/beta/B.java:11") ||
		!strings.Contains(paged, "next_cursor=2") ||
		strings.Contains(paged, "`run_alpha` @ src/alpha/a.py:7") {
		t.Fatalf("paged lens should preserve cursor semantics without dropping structured state:\n%s", paged)
	}
}

func TestPublishSourceInventoryObservationFromLens_UsesCrossLanguageRepoMapKinds(t *testing.T) {
	graph := testGraphWithFiles([]*repotypes.FileInfo{
		{
			RelPath:  "src/ui/Index.ets",
			Language: "arkts",
			Package:  "ui",
			Symbols: []repotypes.Symbol{
				{Name: "Index", Kind: "component", File: "src/ui/Index.ets", Line: 6, Exported: true},
				{Name: "build", Kind: "ui-entry", File: "src/ui/Index.ets", Line: 24, Parent: "Index", Exported: true},
				{Name: "message", Kind: "state-field", File: "src/ui/Index.ets", Line: 9, Parent: "Index", Exported: true},
			},
		},
		{
			RelPath:  "src/proto/user.proto",
			Language: "proto",
			Package:  "user.v1",
			Symbols: []repotypes.Symbol{
				{Name: "UserService", Kind: "service", File: "src/proto/user.proto", Line: 8, Exported: true},
				{Name: "GetUser", Kind: "rpc", File: "src/proto/user.proto", Line: 11, Parent: "UserService", Exported: true},
			},
		},
		{
			RelPath:  "src/cj/math.cj",
			Language: "cangjie",
			Package:  "cj",
			Symbols: []repotypes.Symbol{
				{Name: "operator +", Kind: "operator", File: "src/cj/math.cj", Line: 17, Exported: true},
			},
		},
		{
			RelPath:  "src/swift/Loader.swift",
			Language: "swift",
			Package:  "swiftpkg",
			Symbols: []repotypes.Symbol{
				{Name: "Loader", Kind: "actor", File: "src/swift/Loader.swift", Line: 4, Exported: true},
				{Name: "init", Kind: "ctor", File: "src/swift/Loader.swift", Line: 7, Parent: "Loader", Exported: true},
			},
		},
		{
			RelPath:  "src/kotlin/Singleton.kt",
			Language: "kotlin",
			Package:  "kt",
			Symbols: []repotypes.Symbol{
				{Name: "Singleton", Kind: "object", File: "src/kotlin/Singleton.kt", Line: 5, Exported: true},
				{Name: "java.util.List", Kind: "import", File: "src/kotlin/Singleton.kt", Line: 2, Exported: true},
			},
		},
		{
			RelPath:     "src/config/app.yaml",
			IsSpecial:   true,
			SpecialType: "build_config",
			Symbols: []repotypes.Symbol{
				{Name: "feature.enabled", Kind: "config_key", File: "src/config/app.yaml", Line: 3, Exported: true},
			},
		},
	})
	ctx := sourceInventoryTestContext("", graph, "", nil)

	obs := PublishSourceInventoryObservationFromLens(ctx, types.SourceInventoryLensQuery{
		Scopes: []string{"src"},
		Roles: []types.AnswerCandidateRole{
			types.AnswerCandidateRoleFunction,
			types.AnswerCandidateRoleMethod,
			types.AnswerCandidateRoleType,
			types.AnswerCandidateRoleField,
			types.AnswerCandidateRoleConfigFile,
			types.AnswerCandidateRoleConfigKey,
			types.AnswerCandidateRoleImportPath,
		},
		IncludeCounts: true,
	})
	if !obs.IsActive() || len(obs.Sets) != 7 {
		t.Fatalf("observation = %+v, want seven role sets", obs)
	}
	gotByRole := map[types.AnswerCandidateRole][]string{}
	for _, set := range obs.Sets {
		if set.Count != len(set.Members) {
			t.Fatalf("count/list invariant broken for role %s: %+v", set.Role, set)
		}
		for _, member := range set.Members {
			gotByRole[set.Role] = append(gotByRole[set.Role], member.Name)
		}
	}
	for role, wantMembers := range map[types.AnswerCandidateRole][]string{
		types.AnswerCandidateRoleFunction:   {"GetUser", "build", "operator +"},
		types.AnswerCandidateRoleMethod:     {"init"},
		types.AnswerCandidateRoleType:       {"Index", "Loader", "Singleton", "UserService"},
		types.AnswerCandidateRoleField:      {"message"},
		types.AnswerCandidateRoleConfigFile: {"src/config/app.yaml"},
		types.AnswerCandidateRoleConfigKey:  {"feature.enabled"},
		types.AnswerCandidateRoleImportPath: {"java.util.List"},
	} {
		for _, want := range wantMembers {
			if !containsString(gotByRole[role], want) {
				t.Fatalf("role %s missing %q; got=%#v observation=%+v", role, want, gotByRole[role], obs)
			}
		}
	}
}

func TestAggregateMemberSetMemberUsableWithSourceInventoryObservationSupportRef(t *testing.T) {
	mut := types.NewMutableState("source inventory")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRolePackage,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:       "alpha",
				Key:        "src/alpha",
				SupportRef: "src/alpha",
				Role:       types.AnswerCandidateRolePackage,
				File:       "src/alpha",
			}},
		}, {
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:       "run_alpha",
				Key:        "run_alpha",
				SupportRef: "run_alpha: src/alpha/a.py:7",
				Role:       types.AnswerCandidateRoleFunction,
				File:       "src/alpha/a.py",
				Line:       7,
			}},
		}},
	})
	ctx := &types.BusContext{Mutable: mut}
	support := buildAggregateMemberSupportIndex(ctx)

	for _, tc := range []struct {
		name string
		fact types.AnswerAggregateFact
	}{
		{
			name: "path-backed package row",
			fact: types.AnswerAggregateFact{
				Kind:        types.AnswerAggregateMemberSet,
				Members:     []string{"alpha"},
				SupportRefs: []string{"src/alpha"},
			},
		},
		{
			name: "line-backed symbol row",
			fact: types.AnswerAggregateFact{
				Kind:        types.AnswerAggregateMemberSet,
				Members:     []string{"run_alpha"},
				SupportRefs: []string{"run_alpha: src/alpha/a.py:7"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !aggregateMemberSetMemberUsable(tc.fact, tc.fact.Members[0], support) {
				t.Fatalf("member should be usable via source-inventory observation support ref: %+v", tc.fact)
			}
		})
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

func advisoryMemberNames(candidates []types.SourceInventoryAdvisoryCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.Member)
	}
	return out
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
