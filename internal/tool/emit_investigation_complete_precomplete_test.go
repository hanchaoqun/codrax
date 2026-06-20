package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCompletionAggregateFactsAreOptional_SourceOptionalRuntimeDiagnostic(t *testing.T) {
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true},
			LogTriage: &types.LogBundle{
				Errors: []types.LogError{{Type: "jank"}},
			},
		},
	}}
	if !ctx.AnalysisIR.RequestModel.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		t.Fatal("test setup must be a runtime artifact whose current-source lane is not required")
	}
	if !completionAggregateFactsAreOptional(ctx, "resolved") {
		t.Fatal("source-optional runtime diagnostic aggregate facts should be optional handoff context")
	}

	ctx.AnalysisIR.RequestModel.Predicates.IsCountQuestion = true
	if completionAggregateFactsAreOptional(ctx, "resolved") {
		t.Fatal("typed count obligations must remain strict even for runtime artifacts")
	}
}

func TestCurrentSourceForcedReadGatesApply_AttachedTraceObservationOnly(t *testing.T) {
	ctx := &types.BusContext{
		AttachedHitrace: "app-100 (100) [001] .... 2.000000: sched_switch: prev_state=S",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentTrace,
			Scenario: types.ScenarioPerformanceBottleneck,
		}},
	}

	if currentSourceForcedReadGatesApply(ctx) {
		t.Fatal("attached trace observation-only turn must not require current-source forced reads")
	}
}

func TestCurrentSourceForcedReadGatesApply_AttachedTraceCurrentCodeDimension(t *testing.T) {
	ctx := &types.BusContext{
		AttachedHitrace: "app-100 (100) [001] .... 2.000000: sched_switch: prev_state=S",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentTrace,
			Scenario: types.ScenarioPerformanceBottleneck,
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.9,
			},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label:       "current key code",
					Role:        types.RequestedAnswerDimensionCurrentKeyCode,
					SourceQuote: "current key code",
					Required:    true,
				}},
			},
		}},
	}

	if !currentSourceForcedReadGatesApply(ctx) {
		t.Fatal("typed current-code dimension on an attached trace must keep current-source forced reads")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_PendingReadsBlocks
// is the CGEC E1 regression. When the closure has queued a
// PendingRead the tool MUST return a downgrade message AND must NOT
// flip investigationComplete.
func TestEmitInvestigationComplete_PreCompleteCheck_PendingReadsBlocks(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:      "internal/orchestrator/topology.go",
		Rationale: "chain X anchors here but file unread",
		Origin:    "chain_promotion",
	})
	bus := &types.BusContext{Mutable: mut, RepoRoot: t.TempDir()}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "looks complete",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("expected DOWNGRADED message, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/orchestrator/topology.go") {
		t.Errorf("expected pending file in message, got: %s", res.Summary)
	}
	// Critical: the flag MUST stay false so the explorer continues.
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must remain false on downgrade")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_NoPendingReads_AllowsCompletion:
// when the closure has nothing pending and AnalysisIR is nil, the
// tool proceeds to set the flag.
func TestEmitInvestigationComplete_PreCompleteCheck_NoPendingReads_Allows(t *testing.T) {
	mut := types.NewMutableState("test")
	bus := &types.BusContext{Mutable: mut, RepoRoot: t.TempDir()}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "looks complete",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("unexpected downgrade when no pending reads: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when no blockers")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_NoBuiltInRepoSpecificRelationAuthority(t *testing.T) {
	repo := t.TempDir()
	writeTestFile(t, repo, "internal/types/stage_binding.go", `
package types

type StageBinding struct {
    Stage string
    Agent string
}

var builtinStageBindings = []StageBinding{
    {Stage: "analyze", Agent: "analyzer"},
    {Stage: "explore", Agent: "explorer"},
    {Stage: "extract", Agent: "extractor"},
    {Stage: "finalize", Agent: "finalizer"},
}
`)
	mut := types.NewMutableState("read-mode stages and agents")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: repo,
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "structured relation members are complete",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{
			{
				"kind":    "member_set",
				"role":    "supporting_coverage",
				"label":   "pipeline phases",
				"members": []string{`StageAnalyze ("analyze")`, `StageExplore ("explore")`, `StageExtract ("extract")`, `StageFinalize ("finalize")`},
			},
			{
				"kind":    "member_set",
				"role":    "supporting_coverage",
				"label":   "phase actors",
				"members": []string{`AgentAnalyzer ("analyzer")`, `AgentExplorer ("explorer")`, `AgentExtractor ("extractor")`, `AgentFinalizer ("finalizer")`},
			},
		},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("repo-specific stage/agent-looking relation must not trigger a built-in authority downgrade: %s", res.Summary)
	}
	for _, pending := range mut.EvidenceClosure().PendingReads() {
		if strings.Contains(pending.File, "stage_binding") {
			t.Fatalf("repo-specific relation-looking member sets must not enqueue authority reads without an explicit provider: %+v", pending)
		}
	}
	for _, repair := range mut.EvidenceClosure().PendingRepairs() {
		if strings.Contains(repair.Origin, "relation_authority") || strings.Contains(strings.Join(repair.Files, ","), "stage_binding") {
			t.Fatalf("repo-specific relation-looking member sets must not enqueue authority repairs without an explicit provider: %+v", repair)
		}
	}
}

func TestStructuredRelationAuthorityDemands_IgnoresRelationsWithoutAuthorityProvider(t *testing.T) {
	repo := t.TempDir()
	bus := &types.BusContext{RepoRoot: repo, Mutable: types.NewMutableState("generic relation")}
	facts := []types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateMemberSet,
			Role:  types.AnswerAggregateRoleSupportingCoverage,
			Label: "route handler relations",
			Members: []string{
				`GET /api/users → ListUsersHandler`,
				`POST /api/users → CreateUserHandler`,
			},
		},
		{
			Kind:  types.AnswerAggregateMemberSet,
			Role:  types.AnswerAggregateRoleSupportingCoverage,
			Label: "config readers",
			Members: []string{
				`providers_config → LoadRuntimeConfig`,
				`auth.token → ReadTokenConfig`,
			},
		},
	}
	if got := structuredRelationAuthorityDemands(bus, facts, nil); len(got) != 0 {
		t.Fatalf("generic typed relations without an exact authority provider must not block completion: %+v", got)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationLookupRequiresMemberSetHandoff(t *testing.T) {
	mut := types.NewMutableState("哪些 agent 可以调用 subagent？")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsRelationalLookup:    true,
					IsCategoryEnumeration: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"agent", "subagent"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "SubAgentRuntime validates proposals and runs registered subagents.",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "relation member-set handoff is missing") {
		t.Fatalf("expected relation member-set downgrade, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open until relation member_set is emitted")
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Origin != "pre_complete.relation_member_set" {
		t.Fatalf("expected relation member-set repair directive, got %+v", repairs)
	}

	mut = types.NewMutableState("哪些 agent 可以调用 subagent？")
	bus.Mutable = mut
	params, _ = json.Marshal(map[string]any{
		"reason":      "verified qualifying member set",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "agents that can invoke subagents",
			"value":   "1",
			"members": []string{"explorer agent"},
		}},
	})
	res, err = tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "relation member-set handoff is missing") {
		t.Fatalf("accepted relation member_set should satisfy handoff, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete after relation member_set handoff")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationMemberSetAcceptsRoleLabeledSupportRefWhenLocationContainsMember(t *testing.T) {
	bus := relationMemberSetTestBus(t)
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentExplorer",
		Snippet:         `AgentExplorer AgentName = "explorer"`,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "AgentExplorer is the matching registered sub-agent caller.",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "agent names that can call sub-agents",
			"value":        "1",
			"members":      []string{"explorer"},
			"support_refs": []string{"direct AgentExplorer @ internal/types/enums.go:117"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "relation member-set handoff is missing") {
		t.Fatalf("role-labeled support_ref should satisfy relation member_set when the grounded location contains the member: %s", res.Summary)
	}
	if !bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("investigation should complete after citable relation member_set handoff")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RoleLabeledSupportRefStillRequiresMemberAtLocation(t *testing.T) {
	bus := relationMemberSetTestBus(t)
	bus.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentExplorer",
		Snippet:         `AgentExplorer AgentName = "explorer"`,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "wrong member should remain blocked",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "agent names that can call sub-agents",
			"value":        "1",
			"members":      []string{"coder"},
			"support_refs": []string{"direct AgentExplorer @ internal/types/enums.go:117"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "relation member-set handoff is missing") ||
		!strings.Contains(res.Summary, "coder") {
		t.Fatalf("role-labeled support_ref must not certify a member absent from the grounded location: %s", res.Summary)
	}
	if bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open when the member is absent from the support location")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationMemberSetMustCoverGroundedTypedImplementers(t *testing.T) {
	bus := relationMemberSetTestBus(t)
	bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisImplement
	bus.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Looper"}
	bus.Mutable.SetSearchGraph(relationMemberSetGraph(t,
		relationMemberSetGraphSymbol{name: "alpha", file: "impl_alpha.go", line: 14},
		relationMemberSetGraphSymbol{name: "beta", file: "impl_beta.go", line: 22},
	))
	bus.Mutable.AppendEvidence([]types.EvidenceItem{
		relationMemberSetEvidence("alpha", "impl_alpha.go", 14),
		relationMemberSetEvidence("beta", "impl_beta.go", 22),
	})

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "alpha only",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "Looper implementers",
			"value":        "1",
			"members":      []string{"alpha"},
			"support_refs": []string{"alpha @ impl_alpha.go:14"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "omits grounded typed relation evidence") ||
		!strings.Contains(res.Summary, "implements Looper -> beta @ impl_beta.go:22") {
		t.Fatalf("grounded typed relation omission should downgrade with precise member: %s", res.Summary)
	}
	if bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open when a grounded typed relation member is omitted")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationGraphOnlyImplementerDoesNotForceClosure(t *testing.T) {
	bus := relationMemberSetTestBus(t)
	bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisImplement
	bus.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Looper"}
	bus.Mutable.SetSearchGraph(relationMemberSetGraph(t,
		relationMemberSetGraphSymbol{name: "alpha", file: "impl_alpha.go", line: 14},
		relationMemberSetGraphSymbol{name: "beta", file: "impl_beta.go", line: 22},
	))
	bus.Mutable.AppendEvidence([]types.EvidenceItem{
		relationMemberSetEvidence("alpha", "impl_alpha.go", 14),
	})

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "alpha is the verified implementer in this run",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "Looper implementers",
			"value":        "1",
			"members":      []string{"alpha"},
			"support_refs": []string{"alpha @ impl_alpha.go:14"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "omits grounded typed relation evidence") {
		t.Fatalf("graph-only implementer without grounded evidence must not be hard-forced: %s", res.Summary)
	}
	if !bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("investigation should complete when all grounded typed implementers are represented")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_CallRelationDoesNotForceImplementerSupportRows(t *testing.T) {
	bus := relationMemberSetTestBus(t)
	bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisCall
	bus.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities = []string{"Agent"}
	bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Agent", "SubAgent", "ProposeSubAgents"}
	bus.Mutable.SetSearchGraph(relationMemberSetNamedGraph(t, "Agent",
		relationMemberSetGraphSymbol{name: "ProposeSubAgents", file: "internal/tool/propose_sub_agents.go", line: 18},
	))
	bus.Mutable.AppendEvidence([]types.EvidenceItem{
		relationMemberSetEvidence("ProposeSubAgents", "internal/tool/propose_sub_agents.go", 18),
		relationMemberSetEvidence("ExplorerAgent", "internal/agent/explorer.go", 18522),
	})

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "ExplorerAgent is the verified caller; implementer rows are support context.",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "agents that can call subagents",
			"value":        "1",
			"members":      []string{"ExplorerAgent"},
			"support_refs": []string{"ExplorerAgent @ internal/agent/explorer.go:18522"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "omits grounded typed relation evidence") ||
		strings.Contains(res.Summary, "ProposeSubAgents") {
		t.Fatalf("call relation coverage must not force implementer support rows into principal member_set: %s", res.Summary)
	}
	if !bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("investigation should complete when the call-axis principal member_set is present")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationGroundedAuxiliaryImplementerRespectsProductionScope(t *testing.T) {
	bus := relationMemberSetTestBus(t)
	bus.AnalysisIR.RequestModel.PredicateAxis = types.AxisImplement
	bus.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities = []string{"Looper"}
	bus.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Looper"}
	bus.Mutable.SetSearchGraph(relationMemberSetGraph(t,
		relationMemberSetGraphSymbol{name: "alpha", file: "impl_alpha.go", line: 14},
		relationMemberSetGraphSymbol{name: "testLooper", file: "impl_alpha_test.go", line: 40},
	))
	bus.Mutable.AppendEvidence([]types.EvidenceItem{
		relationMemberSetEvidence("alpha", "impl_alpha.go", 14),
		relationMemberSetEvidence("testLooper", "impl_alpha_test.go", 40),
	})

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "alpha is production; testLooper is test-only",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "Looper production implementers",
			"value":        "1",
			"members":      []string{"alpha"},
			"excluded":     []string{"testLooper (test file)"},
			"support_refs": []string{"alpha @ impl_alpha.go:14"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "testLooper") && strings.Contains(res.Summary, "omits grounded typed relation evidence") {
		t.Fatalf("production scope must not force test-only implementers: %s", res.Summary)
	}
	if !bus.Mutable.IsInvestigationComplete() {
		t.Fatalf("investigation should complete when omitted grounded implementer is auxiliary under production scope")
	}
}

func relationMemberSetTestBus(t *testing.T) *types.BusContext {
	t.Helper()
	return &types.BusContext{
		Mutable:  types.NewMutableState("哪些 agent 可以调用 subagent？"),
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsRelationalLookup:    true,
					IsCategoryEnumeration: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"agent", "subagent"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}
}

type relationMemberSetGraphSymbol struct {
	name string
	file string
	line int
}

func relationMemberSetGraph(t *testing.T, impls ...relationMemberSetGraphSymbol) *repotypes.Graph {
	t.Helper()
	return relationMemberSetNamedGraph(t, "Looper", impls...)
}

func relationMemberSetNamedGraph(t *testing.T, iface string, impls ...relationMemberSetGraphSymbol) *repotypes.Graph {
	t.Helper()
	ifaceFile := &repotypes.FileInfo{RelPath: "iface.go", Language: "go"}
	ifaceSym := repotypes.Symbol{Name: iface, Kind: "interface", File: "iface.go", Line: 7}
	ifaceSym.ID = repotypes.DeriveSymbolID(ifaceFile, &ifaceSym)
	ifaceFile.Symbols = []repotypes.Symbol{ifaceSym}

	files := []*repotypes.FileInfo{ifaceFile}
	fileIndex := map[string]*repotypes.FileInfo{"iface.go": ifaceFile}
	symbolByID := map[repotypes.SymbolID]*repotypes.Symbol{ifaceSym.ID: &ifaceFile.Symbols[0]}
	for _, impl := range impls {
		fi := &repotypes.FileInfo{RelPath: impl.file, Language: "go"}
		sym := repotypes.Symbol{
			Name:       impl.name,
			Kind:       "struct",
			File:       impl.file,
			Line:       impl.line,
			Implements: []repotypes.SymbolID{ifaceSym.ID},
		}
		sym.ID = repotypes.DeriveSymbolID(fi, &sym)
		fi.Symbols = []repotypes.Symbol{sym}
		files = append(files, fi)
		fileIndex[impl.file] = fi
		symbolByID[sym.ID] = &fi.Symbols[0]
	}
	return &repotypes.Graph{
		Files:      files,
		FileIndex:  fileIndex,
		SymbolDefs: map[string][]*repotypes.Symbol{"Looper": {&ifaceFile.Symbols[0]}},
		SymbolByID: symbolByID,
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ExhaustiveHandoffNotDemotedByRequiredFileCap(t *testing.T) {
	mut := types.NewMutableState("列出 internal/analysis 下所有子包入口函数")
	mut.AppendEvidence([]types.EvidenceItem{
		relationMemberSetEvidence("Entry", "internal/analysis/alpha/entry.go", 10),
		relationMemberSetEvidence("Entry", "internal/analysis/beta/entry.go", 10),
		relationMemberSetEvidence("Entry", "internal/analysis/gamma/entry.go", 10),
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"alpha", "beta", "gamma"},
					RequiredFileHints: []types.RequiredFileHint{
						{Path: "internal/analysis/alpha/entry.go", Confidence: 0.95},
						{Path: "internal/analysis/beta/entry.go", Confidence: 0.95},
					},
				},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRolePackage},
					Confidence:        0.95,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "三个子包入口函数已经逐项落地。",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "internal/analysis 子包入口函数全集",
			"value":   "3",
			"role":    "principal_answer",
			"members": []string{"alpha → Entry", "beta → Entry", "gamma → Entry"},
			"support_refs": []string{
				"Entry: internal/analysis/alpha/entry.go:10",
				"Entry: internal/analysis/beta/entry.go:10",
				"Entry: internal/analysis/gamma/entry.go:10",
			},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "role=\"supporting_coverage\" is not principal_answer") ||
		strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("source-inventory required-file cap leaked into exhaustive handoff gate: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete when principal member_set is grounded")
	}
}

func relationMemberSetEvidence(name, file string, line int) types.EvidenceItem {
	return types.EvidenceItem{
		Kind:            types.EvidenceDirect,
		Source:          file,
		LineStart:       line,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    name,
		Snippet:         "type " + name + " struct {}",
		Summary:         name + " implements Looper.",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_FieldValueCountRejectsUnreadAggregateLiteral(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "internal/agent/analyzer.go", `
package agent

func build() {
	out.AnswerContract.CitationReq.Required = false
}
`)
	writeTestFile(t, repoRoot, "internal/orchestrator/orchestrator.go", `
package orchestrator

func fallback() any {
	return AnswerContract{
		CitationReq: types.CitationReq{Required: false},
	}
}
`)
	writeTestFile(t, repoRoot, "internal/orchestrator/orchestrator_test.go", `
package orchestrator

func testOnly() {
	_ = types.CitationReq{Required: false}
}
`)
	writeTestFile(t, repoRoot, "docs/design.md", `
CitationReq.Required = false
`)

	mut := types.NewMutableState("本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？")
	mut.SetRepoRoot(repoRoot)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/analyzer.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/analyzer.go",
		LineStart:       5,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: repoRoot,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"CitationReq.Required"},
					Keywords: []string{"false"},
				},
				FieldValueProfile: &types.FieldValueLookupProfile{
					IsFieldValueLookup: true,
					Target:             "CitationReq.Required",
					Owner:              "CitationReq",
					Field:              "Required",
					Literal:            "false",
					LiteralKind:        types.FieldValueLiteralBool,
					SourceQuote:        "CitationReq.Required 设置为 false",
					Confidence:         0.96,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "direct assignment count is covered",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "field/value count coverage is incomplete") {
		t.Fatalf("expected field/value downgrade, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/orchestrator/orchestrator.go:5") {
		t.Fatalf("downgrade should name unread aggregate literal, got: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "orchestrator_test.go") || strings.Contains(res.Summary, "docs/design.md") {
		t.Fatalf("downgrade must filter tests/docs from production candidates, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open after field/value downgrade")
	}
	repairs := closure.PendingRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Kind != types.RepairExpandSearch {
		t.Fatalf("expected expand-search repair, got %+v", repairs)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ChangeImpactRequiresStructuredTargetHandoff(t *testing.T) {
	mut := types.NewMutableState("Which production files need changes if CitationReq.Required changes?")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[internal/agent/analyzer.go: showing lines 1914-1916 of 2200 total]\n" +
			" 1914│ \tif isMeasurementScalar || isHistoryLookup || rm.HasExternalOnlyRuntimeArtifact() {\n" +
			" 1915│ \t\tout.AnswerContract.CitationReq.Required = false\n" +
			" 1916│ \t}\n",
	})
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "understructured",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1915,
		LineEnd:         1915,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "Required",
		Condition:       "isMeasurementScalar || isHistoryLookup || rm.HasExternalOnlyRuntimeArtifact()",
		Summary:         "production assignment site sets CitationReq.Required to false",
		Producer:        EmitEvidenceProducer,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"CitationReq.Required"},
				},
				ChangeImpactProfile: &types.ChangeImpactProfile{
					IsChangeImpact:  true,
					Target:          "CitationReq.Required",
					RequestedOutput: types.ImpactOutputFiles,
					Scope:           types.ImpactScopeProduction,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all affected files were inspected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "change-impact target handoff is under-structured") {
		t.Fatalf("expected change-impact handoff downgrade, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/agent/analyzer.go:1915") {
		t.Fatalf("downgrade should name the under-structured line, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open until the target is carried in structured evidence fields")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ChangeImpactFilesRequireAggregateMemberSet(t *testing.T) {
	mut := types.NewMutableState("Which production files need changes if CitationReq.Required changes?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "structured-target",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1915,
		LineEnd:         1915,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Summary:         "production assignment site sets CitationReq.Required to false",
		Producer:        EmitEvidenceProducer,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
			AnalyzerHints: types.AnalyzerHints{
				Kind:     string(types.ReqEnumeration),
				Entities: []string{"CitationReq.Required"},
			},
			ChangeImpactProfile: &types.ChangeImpactProfile{
				IsChangeImpact:  true,
				Target:          "CitationReq.Required",
				RequestedOutput: types.ImpactOutputFiles,
				Scope:           types.ImpactScopeProduction,
			},
		}},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all affected files were inspected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "change-impact principal member_set handoff is missing") {
		t.Fatalf("expected change-impact member_set downgrade, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open until affected files are emitted through aggregate_facts.member_set")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_AllowsChangeImpactFilesAggregateMemberSet(t *testing.T) {
	mut := types.NewMutableState("Which production files need changes if CitationReq.Required changes?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "structured-target",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1915,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Producer:        EmitEvidenceProducer,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
			ChangeImpactProfile: &types.ChangeImpactProfile{
				IsChangeImpact:  true,
				Target:          "CitationReq.Required",
				RequestedOutput: types.ImpactOutputFiles,
				Scope:           types.ImpactScopeProduction,
			},
		}},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all affected files were inspected",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "affected production files",
			"members":      []string{"internal/agent/analyzer.go"},
			"support_refs": []string{"internal/agent/analyzer.go:1915"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "change-impact principal member_set handoff is missing") {
		t.Fatalf("valid aggregate member_set should satisfy the handoff: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete after citable member_set handoff")
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || facts[0].Value != "1" {
		t.Fatalf("member_set aggregate should be retained and canonicalized, got %+v", facts)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RejectsUncitedChangeImpactFileMembers(t *testing.T) {
	mut := types.NewMutableState("Which production files need changes if CitationReq.Required changes?")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
			ChangeImpactProfile: &types.ChangeImpactProfile{
				IsChangeImpact:  true,
				Target:          "CitationReq.Required",
				RequestedOutput: types.ImpactOutputFiles,
				Scope:           types.ImpactScopeProduction,
			},
		}},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all affected files were inspected",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "affected production files",
			"members": []string{"internal/agent/analyzer.go"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "not usable as citable source-location principal data") {
		t.Fatalf("file members without support refs should be rejected, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open for uncited file members")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ReusesRetainedAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("list all enum types")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       653,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Intent",
			Subject:         "Intent",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       673,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Scenario",
			Subject:         "Scenario",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	accepted := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "enum type names",
		Value:       "2",
		Members:     []string{"Intent", "Scenario"},
		SupportRefs: []string{"Intent @ internal/types/analysis_ir.go:653", "Scenario @ internal/types/analysis_ir.go:673"},
	}}
	mut.SetInvestigationAggregateFacts(accepted)
	mut.SetInvestigationComplete("accepted member set")
	mut.RetainInvestigationAggregateFacts()
	mut.ResetInvestigationComplete()

	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"Intent", "Scenario"},
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all enum types"},
			},
			AnswerContract: types.AnswerContract{
				MustIncludeTerms: []types.ContractTerm{
					{Text: "Intent", Kind: types.ContractTermSymbol},
					{Text: "Scenario", Kind: types.ContractTermSymbol},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "closure-only reconcile; prior typed member_set remains authoritative",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "required principal members lack typed handoff") ||
		strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("retained member_set should satisfy later closure-only completion, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("closure-only completion should complete by reusing retained aggregate facts")
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || len(got[0].Members) != 2 || got[0].Members[0] != "Intent" {
		t.Fatalf("retained aggregate facts not preserved after closure-only completion: %+v", got)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_MemberSetSupportRefsCanUseDeterministicToolOutput(t *testing.T) {
	mut := types.NewMutableState("list all enum types")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"internal/types/analysis_ir.go:653:type Intent string",
			"internal/types/analysis_ir.go:673:type Scenario string",
		}, "\n"),
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"Intent", "Scenario"},
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all enum types"},
			},
			AnswerContract: types.AnswerContract{
				MustIncludeTerms: []types.ContractTerm{
					{Text: "Intent", Kind: types.ContractTermSymbol},
					{Text: "Scenario", Kind: types.ContractTermSymbol},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "grep output verified both enum type declarations",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "enum type names",
			"value":        "2",
			"members":      []string{"Intent", "Scenario"},
			"support_refs": []string{"Intent @ internal/types/analysis_ir.go:653", "Scenario @ internal/types/analysis_ir.go:673"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("deterministic grep support_refs should satisfy member_set support without per-member emit_evidence: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("deterministic tool-supported member_set should complete")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_AutoSupportRefsFromReadFileGutter(t *testing.T) {
	mut := types.NewMutableState("列出 internal/analysis/ 下所有子包的目录名，以及每个子包的单一入口函数")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: strings.Join([]string{
			"[internal/analysis/findings_validator/validator.go: showing lines 60-75 of 183 total]",
			"    70│ func Validate(text, repoRoot string, graph *repomap.Graph) Result {",
		}, "\n"),
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"findings_validator", "gate"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "the package entry function was read from source",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "internal/analysis package entries",
			"value":   "1",
			"members": []string{"findings_validator → Validate"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("read_file gutter should compile a member-specific support_ref: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("read_file-supported member_set should complete")
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 {
		t.Fatalf("expected one aggregate fact, got %+v", got)
	}
	wantRef := "Member @ internal/analysis/findings_validator/validator.go:70"
	if len(got[0].SupportRefs) != 1 || got[0].SupportRefs[0] != wantRef {
		t.Fatalf("auto support_ref = %+v, want %q", got[0].SupportRefs, wantRef)
	}
}

func TestAggregateMemberReadFileSupportRef_RelationPathMatch(t *testing.T) {
	support := aggregateMemberSupportIndex{
		readFileLines: []aggregateToolLine{{
			File: "internal/analysis/findings_validator/validator.go",
			Line: 70,
			Text: "func Validate(text, repoRoot string, graph *repomap.Graph) Result {",
		}},
	}
	ref, ok := aggregateMemberReadFileSupportRef("findings_validator → Validate", support)
	if !ok {
		t.Fatal("expected read_file gutter line to support relation member")
	}
	if ref != "Member @ internal/analysis/findings_validator/validator.go:70" {
		t.Fatalf("support ref = %q", ref)
	}
}

func TestAggregateReadFilePathMatchesRelationLeft_CrossLanguageSurfaces(t *testing.T) {
	cases := []struct {
		name string
		file string
		left string
	}{
		{name: "go package", file: "internal/analysis/findings_validator/validator.go", left: "findings_validator"},
		{name: "java dotted package", file: "src/main/java/com/example/api/Handler.java", left: "com.example.api"},
		{name: "scoped npm package", file: "packages/scope/pkg/src/index.ts", left: "@scope/pkg"},
		{name: "cpp namespace", file: "src/foo/bar/widget.cc", left: "foo::bar"},
		{name: "monorepo path", file: "packages/core/src/index.ts", left: "packages/core"},
		{name: "hyphen module", file: "node_modules/react-dom/index.js", left: "react-dom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !aggregateReadFilePathMatchesRelationLeft(tc.file, tc.left) {
				t.Fatalf("expected %q to match relation left %q", tc.file, tc.left)
			}
		})
	}
	if aggregateReadFilePathMatchesRelationLeft("internal/analysis/findings_validator/validator.go", "logtriage") {
		t.Fatal("different package segment must not match")
	}
}

func TestAggregateReadFileToolLines_ParsesGutter(t *testing.T) {
	lines := aggregateReadFileToolLines(strings.Join([]string{
		"[internal/analysis/findings_validator/validator.go: showing lines 60-75 of 183 total]",
		"    70│ func Validate(text, repoRoot string, graph *repomap.Graph) Result {",
	}, "\n"))
	if len(lines) != 1 {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[0].File != "internal/analysis/findings_validator/validator.go" || lines[0].Line != 70 ||
		!strings.Contains(lines[0].Text, "Validate") {
		t.Fatalf("parsed line = %+v", lines[0])
	}
}

func TestEnrichCompletionAggregateFactsWithMemberSupport_ReadFileGutter(t *testing.T) {
	mut := types.NewMutableState("x")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: strings.Join([]string{
			"[internal/analysis/findings_validator/validator.go: showing lines 60-75 of 183 total]",
			"    70│ func Validate(text, repoRoot string, graph *repomap.Graph) Result {",
		}, "\n"),
	})
	got := enrichCompletionAggregateFactsWithMemberSupport(&types.BusContext{Mutable: mut}, []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "entries",
		Value:   "1",
		Members: []string{"findings_validator → Validate"},
	}})
	if len(got) != 1 || len(got[0].SupportRefs) != 1 {
		t.Fatalf("enriched facts = %+v", got)
	}
	if got[0].SupportRefs[0] != "Member @ internal/analysis/findings_validator/validator.go:70" {
		t.Fatalf("support refs = %+v", got[0].SupportRefs)
	}
}

func TestEnrichCompletionAggregateFactsWithMemberSupport_DecoratedLineMembers(t *testing.T) {
	mut := types.NewMutableState("x")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: strings.Join([]string{
			"[internal/agent/analyzer.go: showing lines 1377-1378 of 2500 total]",
			"  1377│ graph, graphOrigin := analyzerGraphForNormalize(ctx, rm)",
			"  1378│ rm = reconcileEnumerationBoundaryScope(rm)",
			"[internal/agent/analyzer.go: showing lines 1702-1703 of 2500 total]",
			"  1702│ resolver := analyzerSymbolResolver(ctx, rm)",
			"  1703│ rm.TermGraph = normalizer.Normalize(rm.TermGraph, resolver)",
		}, "\n"),
	})
	got := enrichCompletionAggregateFactsWithMemberSupport(&types.BusContext{Mutable: mut}, []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "call chain",
		Value: "3",
		Members: []string{
			"analyzerGraphForNormalize (L1377)",
			"reconcileEnumerationBoundaryScope (line 1378)",
			"normalizer.Normalize (第1703行)",
			"analyzerSymbolResolver (analyzer.go:1702)",
		},
	}})
	if len(got) != 1 || len(got[0].SupportRefs) != 4 {
		t.Fatalf("enriched facts = %+v", got)
	}
	want := []string{
		"analyzerGraphForNormalize: internal/agent/analyzer.go:1377",
		"reconcileEnumerationBoundaryScope: internal/agent/analyzer.go:1378",
		"normalizer.Normalize: internal/agent/analyzer.go:1703",
		"analyzerSymbolResolver: internal/agent/analyzer.go:1702",
	}
	for i, ref := range want {
		if got[0].SupportRefs[i] != ref {
			t.Fatalf("support ref[%d] = %q, want %q (all=%+v)", i, got[0].SupportRefs[i], ref, got[0].SupportRefs)
		}
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ReadFileGutterRequiresRelationPathMatch(t *testing.T) {
	mut := types.NewMutableState("列出 internal/analysis/ 下所有子包的目录名，以及每个子包的单一入口函数")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: strings.Join([]string{
			"[internal/analysis/findings_validator/validator.go: showing lines 60-75 of 183 total]",
			"    70│ func Validate(text, repoRoot string, graph *repomap.Graph) Result {",
		}, "\n"),
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"logtriage", "findings_validator"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "wrong package/function pair should not be certified by a matching function name in another package",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "internal/analysis package entries",
			"value":   "1",
			"members": []string{"logtriage → Validate"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") ||
		!strings.Contains(res.Summary, "logtriage") {
		t.Fatalf("read_file support must match the relation left-axis path, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open when read_file line belongs to a different relation member")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_TypedPrincipalLaneRequiresMemberSet(t *testing.T) {
	mut := types.NewMutableState("列出 internal/analysis/ 下所有子包的目录名，以及每个子包的单一入口函数")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"aggregator", "compiler", "subject"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "the package names were listed in prior tool output",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("typed principal enumeration lane should require member_set handoff, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open until the typed principal member lane is handed off structurally")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_SourceInventoryChecklistGuidesMissingMemberSet(t *testing.T) {
	mut := types.NewMutableState("列出 internal/analysis/ 下所有子包的目录名，以及每个子包的单一入口函数")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"internal/analysis"},
		Provenance:   []string{"repo_lens:tool_query"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    2,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "Aggregate",
				Key:           "Aggregate",
				SupportRef:    "Aggregate: internal/analysis/aggregator/aggregator.go:132",
				Role:          types.AnswerCandidateRoleFunction,
				File:          "internal/analysis/aggregator/aggregator.go",
				Line:          132,
				Language:      "go",
				CoverageState: types.SourceInventoryCoverageObserved,
			}, {
				Name:          "AnalyzeSubject",
				Key:           "AnalyzeSubject",
				SupportRef:    "AnalyzeSubject: internal/analysis/subject/subject.go:41",
				Role:          types.AnswerCandidateRoleFunction,
				File:          "internal/analysis/subject/subject.go",
				Line:          41,
				Language:      "go",
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: strings.Join([]string{
			"[internal/analysis/aggregator/aggregator.go: showing lines 128-134 of 210 total]",
			"   132│ func Aggregate(closure *types.EvidenceClosure) []FieldHeat {",
		}, "\n"),
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"aggregator", "subject"},
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "所有子包",
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "the package names were listed in prior tool output",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, want := range []string{
		"exhaustive member-set handoff is missing",
		"Active source-inventory verification checklist",
		"not a system-authored member_set",
		"after your own verification",
		"count=2 len(members)=2",
		"verified_source_window `Aggregate`",
		"candidate_needs_verification `AnalyzeSubject`",
		"support_ref=`AnalyzeSubject: internal/analysis/subject/subject.go:41`",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("source-inventory repair summary missing %q:\n%s", want, res.Summary)
		}
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 {
		t.Fatal("expected missing member_set repair directive")
	}
	last := repairs[len(repairs)-1]
	if !containsString(last.Files, "internal/analysis/subject/subject.go") {
		t.Fatalf("repair directive should include source-inventory candidate files, got %+v", last.Files)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open until member_set is emitted")
	}
	if facts := mut.StableInvestigationAggregateFacts(); len(facts) != 0 {
		t.Fatalf("system must not synthesize member_set facts from source-inventory checklist, got %+v", facts)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_SourceInventoryRequiresLensExecution(t *testing.T) {
	mut := types.NewMutableState("列出仓库里的 ArkTS 入口和 Builder 片段")
	mut.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"."},
		Provenance:   []string{"source_inventory_profile", "repomap_graph", "pre_explore_typed_request"},
		Sets: []types.SourceInventoryAdvisorySet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Candidates: []types.SourceInventoryAdvisoryCandidate{{
				Member:     "GlobalCard",
				Key:        "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets::GlobalCard",
				SupportRef: "GlobalCard: internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets:26",
				Role:       types.AnswerCandidateRoleFunction,
				File:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line:       26,
				Language:   "arkts",
			}},
		}},
	})
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"."},
		Provenance:   []string{"tool:list_files:direct"},
		Lens:         []string{"direct_children", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFile,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:       "internal",
				Key:        "internal",
				SupportRef: "internal",
				Provenance: []string{"tool:list_files:direct"},
				Role:       types.AnswerCandidateRolePackage,
				File:       "internal",
			}},
		}},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles: []types.AnswerCandidateRole{
						types.AnswerCandidateRoleFunction,
						types.AnswerCandidateRoleMethod,
					},
					RequestedFields: []types.SourceInventoryRequestedField{
						types.SourceInventoryFieldName,
						types.SourceInventoryFieldLocation,
					},
					Confidence: 0.9,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "the decorator inventory was inspected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, want := range []string{
		"source-inventory lens has not run",
		"`source_inventory_profile`",
		"`repo_map(view=\"source_inventory\")`",
		"roles=[function, method]",
		"list_files direct universe is present",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("source-inventory lens downgrade missing %q:\n%s", want, res.Summary)
		}
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 {
		t.Fatal("expected source-inventory lens repair directive")
	}
	last := repairs[len(repairs)-1]
	if last.Kind != types.RepairStructuredHandoff ||
		last.Origin != "pre_complete.source_inventory_lens_execution" ||
		!strings.Contains(last.Subject, `repo_map`) ||
		!strings.Contains(last.Subject, `source_inventory`) {
		t.Fatalf("unexpected source-inventory repair directive: %+v", last)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("investigation must stay open until source_inventory lens execution lands")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_SourceInventoryMemberSetDoesNotBypassLensExecution(t *testing.T) {
	mut := types.NewMutableState("列出仓库里的 ArkTS 入口和 Builder 片段")
	mut.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"."},
		Provenance:   []string{"source_inventory_profile", "repomap_graph", "pre_explore_typed_request"},
		Sets: []types.SourceInventoryAdvisorySet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Candidates: []types.SourceInventoryAdvisoryCandidate{{
				Member:     "GlobalCard",
				Key:        "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets::GlobalCard",
				SupportRef: "GlobalCard: internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets:26",
				Role:       types.AnswerCandidateRoleFunction,
				File:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets",
				Line:       26,
				Language:   "arkts",
			}},
		}},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles: []types.AnswerCandidateRole{
						types.AnswerCandidateRoleFunction,
						types.AnswerCandidateRoleType,
					},
					RequestedFields: []types.SourceInventoryRequestedField{
						types.SourceInventoryFieldName,
						types.SourceInventoryFieldLocation,
					},
					Confidence: 0.9,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "the decorator inventory was inspected from grep and parser files",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "ArkTS decorator matches",
			"value":   "2",
			"role":    "principal_answer",
			"members": []string{"internal/tool/repomap/index/extract_arkts.go", "internal/tool/repomap/types/lang.go"},
			"support_refs": []string{
				"internal/tool/repomap/index/extract_arkts.go:96",
				"internal/tool/repomap/types/lang.go:46",
			},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, want := range []string{
		"source-inventory lens has not run",
		"`repo_map(view=\"source_inventory\")`",
		"roles=[function, type]",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("source-inventory lens downgrade should override premature member_set; missing %q:\n%s", want, res.Summary)
		}
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("principal member_set must not close a typed source-inventory lane before source_inventory lens execution")
	}
	if facts := mut.StableInvestigationAggregateFacts(); len(facts) != 0 {
		t.Fatalf("downgraded premature member_set must not become stable handoff facts, got %+v", facts)
	}
}

func TestSourceInventoryLensExecutionRepoMapCallShapeNormalizesFileScopes(t *testing.T) {
	path, scopes := sourceInventoryLensExecutionRepoMapCallShape([]string{"internal/tool/repomap/index/extract_arkts.go"})
	if path != "internal/tool/repomap/index" || len(scopes) != 1 || scopes[0] != "extract_arkts.go" {
		t.Fatalf("file scope should become containing repo_map path plus relative scope, got path=%q scopes=%+v", path, scopes)
	}
	path, scopes = sourceInventoryLensExecutionRepoMapCallShape([]string{"internal/tool", "internal/types"})
	if path != "." || len(scopes) != 2 || scopes[0] != "internal/tool" || scopes[1] != "internal/types" {
		t.Fatalf("multi-scope directory calls should keep root path plus scopes, got path=%q scopes=%+v", path, scopes)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_SourceInventoryExactUniverseRejectsPartialMemberSet(t *testing.T) {
	mut := types.NewMutableState("list all source scopes")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"tool:list_files:direct"},
		Lens:         []string{"direct_children", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRolePackage,
			Complete: true,
			Count:    3,
			Members: []types.SourceInventoryObservationMember{{
				Name:       "alpha",
				Key:        "src/alpha",
				SupportRef: "src/alpha",
				Provenance: []string{"tool:list_files:direct"},
				Role:       types.AnswerCandidateRolePackage,
				File:       "src/alpha",
			}, {
				Name:       "beta",
				Key:        "src/beta",
				SupportRef: "src/beta",
				Provenance: []string{"tool:list_files:direct"},
				Role:       types.AnswerCandidateRolePackage,
				File:       "src/beta",
			}, {
				Name:       "gamma",
				Key:        "src/gamma",
				SupportRef: "src/gamma",
				Provenance: []string{"tool:list_files:direct"},
				Role:       types.AnswerCandidateRolePackage,
				File:       "src/gamma",
			}},
		}},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqEnumeration),
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all direct scopes",
				},
			},
			AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}},
		},
	}
	params, _ := json.Marshal(map[string]any{
		"reason":      "alpha and beta are verified principal scopes",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "source scopes",
			"value":        "2",
			"members":      []string{"alpha", "beta"},
			"support_refs": []string{"src/alpha", "src/beta"},
		}},
	})
	res, err := (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "exact candidate universe") || !strings.Contains(res.Summary, "gamma") {
		t.Fatalf("partial exact universe should reject with missing member guidance, got:\n%s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("investigation should remain open until the exact universe is covered or excluded")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationMemberWithoutTypedSupportBlocks(t *testing.T) {
	mut := types.NewMutableState("列出 internal/analysis/ 下所有子包的目录名，以及每个子包的单一入口函数")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/analysis/aggregator/aggregator.go",
		LineStart:       132,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Aggregate",
		Subject:         "Aggregate",
		Snippet:         "func (a *Aggregator) Aggregate(closure *types.EvidenceClosure) []FieldHeat {",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "所有子包",
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "one unsupported relation member should not close a principal set",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "internal/analysis 子包及其入口函数",
			"value":        "2",
			"members":      []string{"aggregator: Aggregate", "subject: AnalyzeIR"},
			"support_refs": []string{"aggregator: Aggregate @ internal/analysis/aggregator/aggregator.go:132"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") ||
		!strings.Contains(res.Summary, "subject: AnalyzeIR") {
		t.Fatalf("unsupported relation member should block exhaustive completion, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open when a relation member lacks typed support")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_GenericMemberSupportRefUsesGroundedSnippet(t *testing.T) {
	mut := types.NewMutableState("默认注册的 SubAgent 名称是什么？")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       31,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Name",
		Snippet:         "func (s *SubExplorer) Name() string {\n\treturn \"explorer\"\n}",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "默认注册的 SubAgent 名称"},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "SubExplorer.Name returns the registered SubAgent name.",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "默认注册的 SubAgent 名称",
			"value":        "1",
			"members":      []string{"explorer"},
			"support_refs": []string{"Member @ internal/agent/sub_explorer.go:31"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("generic Member support_ref should be validated against grounded snippet text: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("generic Member support_ref should allow completion once the member appears in grounded snippet")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_GenericMemberSupportRefStillRequiresMemberAtLocation(t *testing.T) {
	mut := types.NewMutableState("默认注册的 SubAgent 名称是什么？")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       31,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Name",
		Snippet:         "func (s *SubExplorer) Name() string {\n\treturn \"explorer\"\n}",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "默认注册的 SubAgent 名称"},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "wrong member should remain blocked",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "默认注册的 SubAgent 名称",
			"value":        "1",
			"members":      []string{"worker"},
			"support_refs": []string{"Member @ internal/agent/sub_explorer.go:31"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") ||
		!strings.Contains(res.Summary, "worker") {
		t.Fatalf("generic Member support_ref must not certify a different member: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open when the member is absent from the support location")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ValueLiteralMemberMayUseOwnerSupportRef(t *testing.T) {
	mut := types.NewMutableState("列出默认注册的组件名称")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       31,
		AnchorKind:      types.AnchorReturn,
		AnchorSymbol:    "Name",
		Snippet:         "func (s *SubExplorer) Name() string {",
		Summary:         "SubExplorer.Name() returns the registered name \"explorer\".",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "默认注册的组件名称"},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "value literal member is backed by a value-bearing owner location",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "registered component names",
			"value":        "1",
			"members":      []string{"explorer"},
			"support_refs": []string{"SubExplorer (internal/agent/sub_explorer.go:31)"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("owner support_ref should certify exact value literal from accepted return evidence: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("owner support_ref should allow completion once the exact member appears in accepted value evidence")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ValueLiteralOwnerSupportRefRequiresExactMember(t *testing.T) {
	mut := types.NewMutableState("列出默认注册的组件名称")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       31,
		AnchorKind:      types.AnchorReturn,
		AnchorSymbol:    "Name",
		Snippet:         "func (s *SubExplorer) Name() string {",
		Summary:         "SubExplorer.Name() returns the registered name \"explorer\".",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "默认注册的组件名称"},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "different value member must remain blocked",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "registered component names",
			"value":        "1",
			"members":      []string{"worker"},
			"support_refs": []string{"SubExplorer (internal/agent/sub_explorer.go:31)"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") ||
		!strings.Contains(res.Summary, "worker") {
		t.Fatalf("owner support_ref must not certify a different value literal: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open when the requested member is absent from accepted value evidence")
	}
}

func TestFieldValueCountCandidates_CoversCrossLanguageInitializerSurfaces(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "src/config.ets", `
export const cfg: CitationReq = {
  required: false,
}
`)
	writeTestFile(t, repoRoot, "native/options.cpp", `
struct CitationReq cfg = {
  .required = false,
};
`)
	writeTestFile(t, repoRoot, "cj/config.cj", `
let cfg = CitationReq(
  required: false
)
`)
	writeTestFile(t, repoRoot, "cj/compiled.cjo", `
let cfg = CitationReq(
  required: false
)
`)

	got := scanRepoForFieldValueCountCandidates(repoRoot, fieldValueCountTarget{
		Full:    "CitationReq.required",
		Owner:   "CitationReq",
		Field:   "required",
		Literal: "false",
	})
	var surfaces []string
	for _, c := range got {
		surfaces = append(surfaces, fmt.Sprintf("%s:%d", c.File, c.Line))
	}
	joined := strings.Join(surfaces, "\n")
	for _, want := range []string{
		"src/config.ets:2",
		"native/options.cpp:2",
		"cj/config.cj:2",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing cross-language candidate %s in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, ".cjo") {
		t.Fatalf("compiled Cangjie artifact must not be scanned as source: %s", joined)
	}
}

func TestFieldValueCountTargetFromContext_DoesNotTreatUnrelatedCountAsValueLiteral(t *testing.T) {
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: "列出 Top 3 个 Foo.Bar 的调用点",
		Predicates: types.SemanticPredicates{
			IsScalarAnswer:  true,
			IsCountQuestion: true,
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Foo.Bar"},
			Keywords: []string{"3"},
		},
	}}}
	if target, ok := fieldValueCountTargetFromContext(ctx); ok {
		t.Fatalf("unrelated count should not become a field/value target: %+v", target)
	}

	ctx.AnalysisIR.RequestModel.FieldValueProfile = &types.FieldValueLookupProfile{
		IsFieldValueLookup: true,
		Target:             "Foo.timeout",
		Owner:              "Foo",
		Field:              "timeout",
		Literal:            "30",
		LiteralKind:        types.FieldValueLiteralNumber,
		SourceQuote:        "Foo.timeout = 30",
		Confidence:         0.95,
	}
	target, ok := fieldValueCountTargetFromContext(ctx)
	if !ok {
		t.Fatal("code-adjacent numeric literal should become a field/value target")
	}
	if target.Full != "Foo.timeout" || target.Field != "timeout" || target.Literal != "30" {
		t.Fatalf("target = %+v, want Foo.timeout/timeout/30", target)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_CountRequiresDeterministicCommandOutput(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "internal/agent/analyzer.go", `
package agent

func build() {
	out.AnswerContract.CitationReq.Required = false
}
`)

	mut := types.NewMutableState("本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？")
	mut.SetRepoRoot(repoRoot)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/analyzer.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/analyzer.go",
		LineStart:       5,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: repoRoot,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "本仓库里，把 CitationReq.Required 设置为 false 的生产代码位点一共有几处？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"CitationReq.Required"},
					Keywords: []string{"false"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all candidate lines were read",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("expected deterministic-count downgrade, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open without command-backed count")
	}

	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "internal/agent/analyzer.go:5:\tout.AnswerContract.CitationReq.Required = false\n",
		Success:  true,
	})
	res, err = tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error after match listing: %v", err)
	}
	if !strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("grep match listing must not clear deterministic-count downgrade: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open after match listing output")
	}

	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "count=1\n",
		Success:  true,
	})
	res, err = tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error after count proof: %v", err)
	}
	if strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("deterministic count proof should clear downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should close after command-backed count")
	}
	facts := mut.StableInvestigationAggregateFacts()
	var foundScalar bool
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateScalar {
			continue
		}
		if fact.Value != "1" {
			continue
		}
		var hasCountAxis, hasExecProof bool
		for _, dim := range fact.Dimensions {
			switch {
			case strings.EqualFold(dim.Name, "answer_axis") && strings.EqualFold(dim.Value, "count"):
				hasCountAxis = true
			case strings.EqualFold(dim.Name, "proof_source") && strings.EqualFold(dim.Value, "exec_command"):
				hasExecProof = true
			}
		}
		if !hasCountAxis || !hasExecProof {
			t.Fatalf("deterministic exec count scalar should carry typed dimensions, got %+v", fact)
		}
		foundScalar = true
		break
	}
	if !foundScalar {
		t.Fatalf("deterministic exec count should be carried as scalar_value aggregate, got %+v", facts)
	}
}

func TestEmitInvestigationComplete_DemotesScalarCountCoverageMemberSetBeforeStablePool(t *testing.T) {
	mut := types.NewMutableState("internal/tool 下非测试 Go 文件总行数是多少？")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "[exec_command: $ find internal/tool -name '*.go' | xargs wc -l | tail -1]\n42 total\n",
		Success:  true,
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "internal/tool 下非测试 Go 文件总行数是多少？",
				Intent:     types.IntentReturnValue,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "line count command and included files were verified",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":       "member_set",
			"label":      "non-test .go files in internal/tool",
			"value":      "2",
			"role":       "principal_answer",
			"provenance": "command:find internal/tool -name '*.go' | xargs wc -l",
			"unit":       "files",
			"members":    []string{"internal/tool/a.go", "internal/tool/b.go"},
		}, {
			"kind":       "scalar_value",
			"label":      "total lines",
			"value":      "42",
			"role":       "principal_answer",
			"provenance": "command:find internal/tool -name '*.go' | xargs wc -l | tail -1",
			"unit":       "lines",
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success || !mut.IsInvestigationComplete() {
		t.Fatalf("expected completion, got success=%v complete=%v summary=%s", res.Success, mut.IsInvestigationComplete(), res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) < 2 {
		t.Fatalf("stable aggregate facts = %+v, want member_set + scalar", facts)
	}
	var memberSet, scalar *types.AnswerAggregateFact
	for i := range facts {
		switch facts[i].Kind {
		case types.AnswerAggregateMemberSet:
			memberSet = &facts[i]
		case types.AnswerAggregateScalar:
			scalar = &facts[i]
		}
	}
	if memberSet == nil || scalar == nil {
		t.Fatalf("stable aggregate facts missing expected lanes: %+v", facts)
	}
	if memberSet.Role != types.AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("coverage member_set role = %q, want supporting_coverage: %+v", memberSet.Role, *memberSet)
	}
	if scalar.Role != types.AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("scalar role = %q, want principal_answer: %+v", scalar.Role, *scalar)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ConflictingDeterministicCountsNeedStructuredHandoff(t *testing.T) {
	mut := types.NewMutableState("internal/tool 下非测试 Go 文件总行数是多少？")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "count=120\n",
		Success:  true,
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "count=121\n",
		Success:  true,
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "internal/tool 下非测试 Go 文件总行数是多少？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "two counting commands were run",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("conflicting deterministic count outputs should require structured handoff, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open when deterministic count outputs conflict")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ScalarAggregateClearsCountDowngrade(t *testing.T) {
	mut := types.NewMutableState("internal/analysis/criterion/grammar.go 里 Kind 常量有多少个？")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "count=51\n",
		Success:  true,
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "count=25\n",
		Success:  true,
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "internal/analysis/criterion/grammar.go 里 Kind 常量有多少个？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "scoped deterministic count was verified after rejecting the broader candidate count",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":       "scalar_value",
			"label":      "Kind 常量数量",
			"value":      "25",
			"provenance": "scoped const-block count",
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success || strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("scalar_value exact count handoff should clear deterministic-count downgrade: success=%v summary=%s", res.Success, res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should close after model-authored scalar_value count handoff")
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateScalar || facts[0].Value != "25" {
		t.Fatalf("scalar aggregate handoff not retained: %+v", facts)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ConflictingScalarAggregatesDoNotClearCountDowngrade(t *testing.T) {
	mut := types.NewMutableState("internal/analysis/criterion/grammar.go 里 Kind 常量有多少个？")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "internal/analysis/criterion/grammar.go 里 Kind 常量有多少个？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "two scalar candidates were copied from different command scopes",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":  "scalar_value",
			"label": "broad candidate count",
			"value": "51",
		}, {
			"kind":  "scalar_value",
			"label": "scoped answer count",
			"value": "25",
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("conflicting scalar_value counts should still require a clearer handoff, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open when scalar aggregate counts conflict")
	}
}

func TestDeterministicHistoryCountToolResultValue_AcceptsOnlyLabeledGitProof(t *testing.T) {
	mut := types.NewMutableState("history count")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "[exec_command: $ git log --format=%H -20 -- internal/orchestrator | awk 'END { print \"answer_count=0\" }']\nanswer_count=0\n",
		Success:  true,
	})
	got, ok := deterministicHistoryCountToolResultValue(&types.BusContext{Mutable: mut})
	if !ok || got != 0 {
		t.Fatalf("deterministicHistoryCountToolResultValue = (%d,%v), want (0,true)", got, ok)
	}

	mut = types.NewMutableState("history count")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "[exec_command: $ git log --oneline -20 -- internal/orchestrator | wc -l]\n20\n",
		Success:  true,
	})
	if got, ok := deterministicHistoryCountToolResultValue(&types.BusContext{Mutable: mut}); ok {
		t.Fatalf("bare git count should not be accepted as exact history proof: (%d,true)", got)
	}
}

func TestDeterministicHistoryCountToolResultValue_AcceptsGitHistorySearchProof(t *testing.T) {
	mut := types.NewMutableState("history count")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "git_history_search",
		Summary:  "[git_history_search: window_path=internal/orchestrator window_count=20 diff_path=internal/orchestrator/orchestrator.go contains=runTaskGraph]\nwindow_size=20\nanswer_count=1\nmatched_commits:\n- 5305ef76 Stabilize write-mode analysis fallback\nunmatched=19\n",
		Success:  true,
	})
	got, ok := deterministicHistoryCountToolResultValue(&types.BusContext{Mutable: mut})
	if !ok || got != 1 {
		t.Fatalf("deterministicHistoryCountToolResultValue = (%d,%v), want (1,true)", got, ok)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_HistoryCountAcceptsGitHistorySearchProof(t *testing.T) {
	mut := types.NewMutableState("最近 20 个提交里有多少次改到了 runTaskGraph？")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "git_history_search",
		Summary:  "[git_history_search: window_path=internal/orchestrator window_count=20 diff_path=internal/orchestrator/orchestrator.go contains=runTaskGraph]\nwindow_size=20\nanswer_count=1\nmatched_commits:\n- 5305ef76 Stabilize write-mode analysis fallback\nunmatched=19\n",
		Success:  true,
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "最近 20 个提交里有多少次改到了 runTaskGraph？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
					IsHistoryLookup: true,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "git_history_search produced answer_count",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "history count aggregate handoff is missing") {
		t.Fatalf("git_history_search count proof should clear history handoff downgrade: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateScalar || fact.Value != "1" {
			continue
		}
		var hasGitProof bool
		for _, dim := range fact.Dimensions {
			if strings.EqualFold(dim.Name, "proof_source") && strings.EqualFold(dim.Value, "git_history_search") {
				hasGitProof = true
			}
		}
		if !hasGitProof {
			t.Fatalf("git_history_search scalar should preserve proof source, got %+v", fact)
		}
		return
	}
	t.Fatalf("git_history_search proof should be carried as scalar aggregate, got %+v", facts)
}

func TestEmitInvestigationComplete_PreCompleteCheck_HistoryCountAcceptsLabeledVCSProof(t *testing.T) {
	mut := types.NewMutableState("过去 20 个提交里有多少次同时改过 internal/orchestrator 和 runTaskGraph？")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "[exec_command: $ git log --format=%H -20 -- internal/orchestrator | awk 'END { print \"answer_count=0\" }']\nanswer_count=0\n",
		Success:  true,
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "过去 20 个提交里有多少次同时改过 internal/orchestrator 和 runTaskGraph？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
					IsHistoryLookup: true,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "exact VCS set operation produced answer_count",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "history count aggregate handoff is missing") {
		t.Fatalf("labeled VCS count proof should clear history handoff downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should close after labeled VCS count proof: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	var found bool
	for _, fact := range facts {
		if fact.Kind != types.AnswerAggregateScalar || fact.Value != "0" ||
			fact.Provenance != "system_deterministic_history_count_enrichment" {
			continue
		}
		var hasAxis, hasKind bool
		for _, dim := range fact.Dimensions {
			switch {
			case strings.EqualFold(dim.Name, "answer_axis") && strings.EqualFold(dim.Value, "count"):
				hasAxis = true
			case strings.EqualFold(dim.Name, "measurement_kind") && strings.EqualFold(dim.Value, "vcs_history_count"):
				hasKind = true
			}
		}
		if !hasAxis || !hasKind {
			t.Fatalf("history deterministic scalar should carry typed dimensions, got %+v", fact)
		}
		found = true
	}
	if !found {
		t.Fatalf("labeled VCS count proof should be carried as scalar aggregate, got %+v", facts)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_HistoryCountRejectsBareBroadGitCount(t *testing.T) {
	mut := types.NewMutableState("过去 20 个提交里有多少次同时改过 internal/orchestrator 和 runTaskGraph？")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "[exec_command: $ git log --oneline -20 -- internal/orchestrator | wc -l]\n20\n",
		Success:  true,
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "过去 20 个提交里有多少次同时改过 internal/orchestrator 和 runTaskGraph？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
					IsHistoryLookup: true,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "broad candidate count only",
		"confidence":  "medium",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "history count aggregate handoff is missing") {
		t.Fatalf("bare broad git count must not clear history handoff downgrade: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open after a broad unlabeled history count")
	}
}

func TestPrincipalSupportMaterializationRequired_SkipsHistoryNarrative(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("最近一次合入的是什么特性？请说明这个特性做了什么，不要只给 commit id。"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
			},
		},
	}
	view := &types.AnswerSemanticView{Family: types.QFArchitecture}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "feature commit",
		Value:   "1",
		Members: []string{"af683718 Tighten explorer guidance and evidence grounding"},
		Role:    types.AnswerAggregateRolePrincipalAnswer,
	}}

	if principalSupportMaterializationRequired(bus, view, facts) {
		t.Fatalf("history narrative should close from VCS/raw command evidence without forcing repo file:line principal support")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationalCountAlsoNeedsStructuredProof(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "internal/orchestrator/orchestrator.go", `
package orchestrator

func build() {
	Contract{CitationReq: CitationReq{Required: false}}
}
`)

	mut := types.NewMutableState("CitationReq.Required=false 的位置有几处？")
	mut.SetRepoRoot(repoRoot)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/orchestrator/orchestrator.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       5,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "CitationReq.Required",
		Subject:         "CitationReq.Required",
		Object:          "false",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: repoRoot,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "CitationReq.Required=false 的位置有几处？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:     true,
					IsCountQuestion:    true,
					IsRelationalLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"CitationReq.Required"},
					Keywords: []string{"false"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "one candidate was classified",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("relational count should still require proof handoff, got: %s", res.Summary)
	}

	params, _ = json.Marshal(map[string]any{
		"reason":      "classified exact member set",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "total_count",
			"label":   "production assignment locations",
			"value":   "1",
			"unit":    "locations",
			"members": []string{"internal/orchestrator/orchestrator.go:5"},
		}},
	})
	res, err = tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error after aggregate handoff: %v", err)
	}
	if strings.Contains(res.Summary, "deterministic count proof is missing") {
		t.Fatalf("structured aggregate handoff should clear deterministic-count downgrade: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_HistoryCountRequiresAggregateHandoff(t *testing.T) {
	mut := types.NewMutableState("recent commits touching runTaskGraph")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Summary:  "count=14\n",
		Success:  true,
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "最近 20 次修改 internal/orchestrator/ 的 commit 中，有多少个直接涉及 runTaskGraph？",
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"internal/orchestrator", "runTaskGraph"},
					Keywords: []string{"recent commits"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "broad git output is available",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "history count aggregate handoff is missing") {
		t.Fatalf("history count should require model-authored aggregate handoff, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("investigation must remain open until the verified history count is structured")
	}

	params, _ = json.Marshal(map[string]any{
		"reason":      "verified each recent commit against the function body and call sites",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":  "total_count",
			"label": "recent commits directly involving runTaskGraph",
			"value": "0",
			"unit":  "commits",
			"dimensions": []map[string]any{
				{"name": "history_window", "value": "20 commits touching internal/orchestrator"},
				{"name": "filter_basis", "value": "verified function body or call-site involvement"},
			},
		}},
	})
	res, err = tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error after aggregate handoff: %v", err)
	}
	if strings.Contains(res.Summary, "history count aggregate handoff is missing") {
		t.Fatalf("structured aggregate handoff should clear history-count downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should close after model-authored history count aggregate")
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorBlocks:
// when AnalysisIR requires ≥1 citation but the evidence buffer has
// no cite-eligible items inside ReadSet, the tool downgrades.
func TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorBlocks(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	// ReadSet has one file, but evidence has nothing pointing there.
	closure.SetReadSet(map[string]bool{"internal/skill/defaults.go": true})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "thinks done",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("expected DOWNGRADED for citation floor miss, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must remain false on citation floor failure")
	}
	vs := closure.ViolationsByKind(types.ViolPreCompleteDowngrade)
	if len(vs) != 1 {
		t.Fatalf("expected one pre-complete downgrade violation, got %d", len(vs))
	}
	if got, want := vs[0].ClusterKey, "root:CitationReq|stage:pre_complete"; got != want {
		t.Fatalf("ClusterKey=%q, want %q", got, want)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorPrefersStructuredSupportRefs(t *testing.T) {
	repoRoot := t.TempDir()
	writeTestFile(t, repoRoot, "internal/analysis/aggregator/aggregator.go", "package aggregator\nfunc New() {}\n")
	writeTestFile(t, repoRoot, "internal/analysis/gate/gate.go", "package gate\nfunc Run() {}\n")
	mut := types.NewMutableState("test")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: repoRoot,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					ExactTargets: []string{"internal/analysis"},
					Keywords:     []string{"entry function"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 2},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "repo lens found the candidate rows",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"label":        "subpackage entry functions",
			"value":        "2",
			"role":         "principal_answer",
			"members":      []string{"New", "Run"},
			"support_refs": []string{"aggregator/aggregator.go:2", "gate/gate.go:2"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected citation floor downgrade, got: %s", res.Summary)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 {
		t.Fatalf("expected structured read repair")
	}
	var readRepair types.RepairDirective
	for _, repair := range repairs {
		if repair.Origin == "pre_complete.citation_floor_support_refs" {
			readRepair = repair
		}
		if repair.Origin == "pre_complete.citation_floor_low" {
			t.Fatalf("structured support_refs must suppress generic expand search, got %+v", repair)
		}
	}
	if readRepair.Kind != types.RepairReadFile {
		t.Fatalf("expected RepairReadFile from support refs, got %+v", repairs)
	}
	want := []string{"internal/analysis/aggregator/aggregator.go", "internal/analysis/gate/gate.go"}
	if strings.Join(readRepair.Files, ",") != strings.Join(want, ",") {
		t.Fatalf("read repair files = %+v, want %+v", readRepair.Files, want)
	}
	pending := mut.EvidenceClosure().PendingReads()
	if len(pending) != 2 {
		t.Fatalf("RepairReadFile should bridge to pending reads, got %+v", pending)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorFallsBackToExpandSearchWithoutStructuredTargets(t *testing.T) {
	mut := types.NewMutableState("test")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Keywords: []string{"orchestrator", "dispatch"}},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 2},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "thinks done",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected citation floor downgrade, got: %s", res.Summary)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Kind != types.RepairExpandSearch {
		t.Fatalf("expected fallback expand-search repair, got %+v", repairs)
	}
	if repairs[len(repairs)-1].Origin != "pre_complete.citation_floor_low" {
		t.Fatalf("fallback origin = %q, want pre_complete.citation_floor_low", repairs[len(repairs)-1].Origin)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorCapsToSingletonMemberSet(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/sub_explorer.go": true})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 3},
			},
		},
	}
	evidence := []types.EvidenceItem{{
		Kind:         types.EvidenceDirect,
		Scope:        types.ScopeLine,
		Subject:      "explorer",
		Source:       "internal/agent/sub_explorer.go",
		LineStart:    31,
		AnchorKind:   types.AnchorStringLiteral,
		AnchorSymbol: "explorer",
		Summary:      "SubExplorer.Name returns explorer.",
	}}
	mut.AppendEvidence(evidence)
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "default subagent names",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"explorer"},
		SupportRefs: []string{"internal/agent/sub_explorer.go:31"},
	}}

	if got := preCompleteContractCheckWithEvidence(bus, "", evidence, facts); got != "" {
		t.Fatalf("singleton principal member set should satisfy capped citation floor, got: %s", got)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ExternalSourceLogWaivesCitationFloor(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "NoMethodError"}},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"cmd/root.go": true})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "external runtime log already explains the failure chain",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("external-source log should waive citation preflight, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set for external-source log closure")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ExternalTraceOptionalSourceWaivesCitationFloor(t *testing.T) {
	perfBundle := &types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace", Signals: []string{"jank"}},
		Observations: []types.PerfObservation{{
			Kind:       "trace_query",
			Subject:    "Choreographer#doFrame 1254842",
			Summary:    "frame took 123ms with scheduler latency and binder wait",
			LineStart:  1102717,
			DurationMs: 123,
		}},
	}
	mut := types.NewMutableState("trace-only runtime artifact")
	mut.SetPerfTrace(perfBundle)
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				PerfTrace: perfBundle,
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 2,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "trace_query returned line-backed runtime observations that answer the frame jank question",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":  "scalar_value",
			"label": "frame 1254842 duration",
			"value": "123 ms",
			"unit":  "duration",
			"dimensions": []map[string]any{{
				"name":  "origin",
				"value": string(types.AnswerEvidenceOriginRuntimeArtifact),
			}},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("trace-only runtime closure should not be blocked by current-source citation floor, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set for trace-only runtime closure")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_TraceQueryObservationsWaiveCitationFloorWithoutModelEscapeHatch(t *testing.T) {
	mut := types.NewMutableState("trace-query observations")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:frame#root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace", ArtifactKind: "trace"},
			Subject:         "CookieMonsterCl-59843",
			Predicate:       "root_cause_primary",
			Object:          "runnable",
			Value:           "8.307",
			Unit:            "ms",
			Summary:         "trace_query ranked on-chain runnable delay as the frame cause",
		}},
	})
	mut.ResetDispatchToolResults()
	bus := &types.BusContext{
		Mutable:         mut,
		RepoRoot:        t.TempDir(),
		AttachedHitrace: "com.baidu.tieba-59566 (59566) [004] .... 34579.587805: sched_wakeup: comm=com.baidu.tieba pid=59566",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 2,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "trace_query returned typed runtime observations for the selected frame window",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("trace_query typed observations should not require model-authored aggregate_facts or evidence_floor_waiver, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set from trace_query typed observations")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_TraceQueryPathObservationsWaiveCitationFloorWithoutAttachment(t *testing.T) {
	mut := types.NewMutableState("trace-query path observations")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:path#root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "eval/fixtures/path_runtime_trace_capture", ArtifactKind: "trace"},
			Subject:         "app-100",
			Predicate:       "root_cause_primary",
			Object:          "worker-200 sleep on wakeup chain",
			Value:           "5.000",
			Unit:            "ms",
			Summary:         "trace_query ranked the explicit-path trace window without an attachment pre-stage",
		}},
	})
	mut.ResetDispatchToolResults()
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentRootCause,
				RawRequest: "只分析 eval/fixtures/path_runtime_trace_capture 这个文件，不分析代码。",
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					SourceQuotes:      []string{"不分析代码"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 2,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "trace_query returned typed runtime observations for the explicit path trace",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("explicit-path trace_query observations should not require source file citations, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set from explicit-path trace_query observations")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_MCPOriginWaivesCitationFloorWhenSourceOptional(t *testing.T) {
	mut := types.NewMutableState("MCP line facts")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 2},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "MCP typed rows line 7 and line 12 answer the requested external observation",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "MCP sleep/wakeup rows",
			"value":   "2",
			"role":    "principal_answer",
			"members": []string{"line 7: sleep", "line 12: wakeup"},
			"dimensions": []map[string]any{{
				"name":  "origin",
				"value": string(types.AnswerEvidenceOriginMCPResource),
			}, {
				"name":  "resource_uri",
				"value": "mcp://fixture/trace/sleep-wakeup",
			}},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("MCP-origin closure should not be blocked by current-source citation floor, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set for MCP-origin closure")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_MCPOriginDoesNotWaiveRequiredCurrentSource(t *testing.T) {
	mut := types.NewMutableState("MCP plus source")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes: []types.CurrentSourceExplanationMode{
						types.CurrentSourceExplanationExplainCurrentMechanism,
					},
					Confidence:   0.9,
					SourceQuotes: []string{"结合源码"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 1},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "MCP rows collected but current-source explanation is still required",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":    "member_set",
			"label":   "MCP sleep/wakeup rows",
			"value":   "1",
			"role":    "principal_answer",
			"members": []string{"line 12: wakeup"},
			"dimensions": []map[string]any{{
				"name":  "origin",
				"value": string(types.AnswerEvidenceOriginMCPResource),
			}},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("typed current-source profile must still enforce source citation floor, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete must remain false until current-source evidence is covered")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ExternalSourceCurrentVerificationRequiresHintRead(t *testing.T) {
	logBundle := &types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:      types.LogObservationRetryCycle,
			Severity:  types.LogObservationWarning,
			Summary:   "first_byte_timeout exceeded after 40s",
			LineStart: 3,
		}},
	}
	mut := types.NewMutableState("mixed runtime + current source")
	mut.SetLogTriage(logBundle)
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/llm/stream_errors.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic:        true,
					CurrentVersionCheck: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqMechanism),
					RequiredFileHints: []types.RequiredFileHint{
						{Path: "internal/llm/stream_errors.go", Confidence: 0.9},
						{Path: "internal/llm/openai.go", Confidence: 0.85},
						{Path: "internal/llm/retryable_error.go", Confidence: 0.7},
					},
				},
				LogTriage: logBundle,
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "runtime timeout plus current source explanation",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("mixed runtime/current verification should block unread high-confidence current-source hints, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/llm/openai.go") {
		t.Fatalf("unread high-confidence hint should be listed, got: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "internal/llm/retryable_error.go") {
		t.Fatalf("soft-confidence hint must remain advisory, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete must remain false until high-confidence current-source hints are covered")
	}
}

func TestRepoGroundingBypassLabel_CurrentVersionCheckDisablesExternalSourceBypass(t *testing.T) {
	logBundle := &types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:      types.LogObservationPerformance,
			Severity:  types.LogObservationWarning,
			Summary:   "first_byte_timeout exceeded",
			LineStart: 3,
		}},
	}
	mut := types.NewMutableState("mixed runtime + current source")
	mut.SetLogTriage(logBundle)
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic:        true,
					CurrentVersionCheck: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					RequiredFileHints: []types.RequiredFileHint{{Path: "internal/llm/openai.go", Confidence: 0.9}},
				},
				LogTriage: logBundle,
			},
		},
	}
	if label, ok := repoGroundingBypassLabel(bus); ok {
		t.Fatalf("current-version verification must keep current-source grounding active, got bypass %q", label)
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorPasses_WithEligibleEvidence:
// when ReadSet covers the evidence Source, the floor is satisfied.
func TestEmitInvestigationComplete_PreCompleteCheck_CitationFloorPasses(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/skill/defaults.go": true})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:    "internal/skill/defaults.go",
			LineStart: 14,
			LineEnd:   14,
			Kind:      types.EvidenceConcrete,
		},
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 1},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "all evidence collected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("unexpected downgrade when eligible evidence present: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when contract preflight passes")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ExplanationFunctionSubject_NoShapeSwap(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/tool/repomap/tool.go": true})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "build-graph-def",
			Source:          "internal/tool/repomap/tool.go",
			LineStart:       133,
			LineEnd:         133,
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "buildOrLoadGraph",
			GroundingStatus: types.GroundingGrounded,
		},
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("unexpected downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set when explanation preflight passes")
	}
	if got := closure.Stats().ViewSwapRaised; got != 0 {
		t.Fatalf("ViewSwapRaised=%d, want 0 for explanation anchored on a function", got)
	}
	for _, repair := range closure.PendingRepairs() {
		if repair.Origin == "pre_complete.subject_shape_mismatch" {
			t.Fatalf("unexpected shape-swap repair: %+v", repair)
		}
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ConfigFamilyFunctionSubject_RaisesViewSwap(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/config/runtime.go": true})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "runtime-def",
			Source:          "internal/config/runtime.go",
			LineStart:       32,
			LineEnd:         32,
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			GroundingStatus: types.GroundingGrounded,
		},
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentConfigQuery,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.90},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "resolved the config lookup",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("unexpected downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set when preflight passes")
	}
	if got := closure.Stats().ViewSwapRaised; got != 1 {
		t.Fatalf("ViewSwapRaised=%d, want 1", got)
	}
	vs := closure.ViolationsByKind(types.ViolViewSwap)
	if len(vs) != 1 {
		t.Fatalf("expected one view-swap violation, got %d", len(vs))
	}
	if got, want := vs[0].ClusterKey, "subject:function_name|from:config_precedence|to:role_lookup|root:answer_subject"; got != want {
		t.Fatalf("ClusterKey=%q, want %q", got, want)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_MultiTopicExplanationAnchorsBlock(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/analysis_ir.go": true,
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:          "internal/types/analysis_ir.go",
			LineStart:       574,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Criterion",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Source:          "internal/types/analysis_ir.go",
			LineStart:       896,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Hypothesis",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				SubTopics: []types.SubTopic{
					{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
					{Summary: "Hypothesis 的角色", Entities: []string{"Hypothesis"}},
					{Summary: "AnalysisIR 如何持有 HypothesisSet", Entities: []string{"AnalysisIR.HypothesisSet", "HypothesisSet"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "enough evidence collected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected downgrade for incomplete multi-topic anchors, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "HypothesisSet") {
		t.Fatalf("expected missing sub-topic in message, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete must remain false on downgrade")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_ArchitectureSkipsAnalyzerExtraTopicAnchors(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/enums.go": true,
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:          "internal/types/enums.go",
			LineStart:       26,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "StageAnalyze",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Source:          "internal/types/enums.go",
			LineStart:       27,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "StageExplore",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:      types.IntentExplain,
				Scenario:    types.ScenarioArchitectureExplain,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow},
				SubTopics: []types.SubTopic{
					{Summary: "Analyze 阶段", Entities: []string{"StageAnalyze"}},
					{Summary: "Explore 阶段", Entities: []string{"StageExplore"}},
					{Summary: "Review 阶段", Entities: []string{"Review"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "the architecture explanation is bounded by grounded stage evidence",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "multi-topic explanation still lacks") {
		t.Fatalf("architecture diagrams must not hard-block on optional analyzer sub-topic anchors: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("unexpected downgrade for architecture narrative: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set for architecture narrative despite optional missing topic anchor")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_WriteExplorationSkipsAnswerAnchorSkeleton(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID: "batch-1",
		Goal:    "locate the implementation before planning a write batch",
	})
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/enums.go": true,
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:          "internal/types/enums.go",
			LineStart:       26,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Criterion",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				SubTopics: []types.SubTopic{
					{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
					{Summary: "Hypothesis 的角色", Entities: []string{"Hypothesis"}},
					{Summary: "AnalysisIR 如何持有 HypothesisSet", Entities: []string{"AnalysisIR.HypothesisSet", "HypothesisSet"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "write exploration has enough implementation context for a planner handoff",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "multi-topic explanation still lacks") {
		t.Fatalf("write exploration must not hard-block on final-answer anchor skeletons: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("unexpected downgrade for write exploration handoff: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set for write exploration handoff")
	}
}

func TestNarrativePrincipalMemberSetCompletesBoundary(t *testing.T) {
	mut := types.NewMutableState("test")
	evidence := []types.EvidenceItem{
		{
			Source:          "internal/types/enums.go",
			LineStart:       26,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "StageAnalyze",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Source:          "internal/types/enums.go",
			LineStart:       27,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "StageExplore",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
		},
	}
	mut.AppendEvidence(evidence)
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:      types.IntentExplain,
				Scenario:    types.ScenarioArchitectureExplain,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow},
				SubTopics: []types.SubTopic{
					{Summary: "Analyze 阶段", Entities: []string{"StageAnalyze"}},
					{Summary: "Explore 阶段", Entities: []string{"StageExplore"}},
					{Summary: "Review 阶段", Entities: []string{"Review"}},
				},
			},
		},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "主 stage",
		Value:       "2",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"StageAnalyze", "StageExplore"},
		SupportRefs: []string{"StageAnalyze @ internal/types/enums.go:26", "StageExplore @ internal/types/enums.go:27"},
	}}
	if !narrativePrincipalMemberSetCompletesBoundary(bus, facts, evidence) {
		t.Fatal("grounded principal architecture member_set should satisfy the narrative boundary")
	}
	facts[0].Role = types.AnswerAggregateRoleSupportingCoverage
	if narrativePrincipalMemberSetCompletesBoundary(bus, facts, evidence) {
		t.Fatal("supporting member_set must not bypass narrative forced-read gates")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_WaiverBypassesMultiTopicAnchors(t *testing.T) {
	mut := types.NewMutableState("test")
	logBundle := &types.LogBundle{Observations: []types.LogObservation{{
		Kind:       types.LogObservationRuntimeEvent,
		Summary:    "attached runtime artifact names an external service",
		Confidence: 0.95,
	}}}
	mut.SetLogTriage(logBundle)
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"attached-log": true,
	})
	mut.AppendEvidence([]types.EvidenceItem{{
		Source:          "attached-log",
		LineStart:       1,
		AnchorKind:      types.AnchorTextReference,
		AnchorSymbol:    "RuntimeError",
		Kind:            types.EvidenceDirect,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
		Origin:          types.ClaimOriginLog,
	}})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				LogTriage: logBundle,
				SubTopics: []types.SubTopic{
					{Summary: "top-level exception", Entities: []string{"RuntimeError"}},
					{Summary: "missing external service", Entities: []string{"ExternalService"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "attached runtime artifact is sufficient and external to this repo",
		"confidence":  "high",
		"result_kind": "resolved",
		"evidence_floor_waiver": map[string]any{
			"reason":    string(types.EvidenceFloorWaiverExternalLog),
			"rationale": "runtime artifact names services that are not defined in the current repository",
		},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "multi-topic explanation still lacks") {
		t.Fatalf("runtime waiver must bypass repo-anchor skeleton gate, got: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("unexpected downgrade after waiver: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set when runtime waiver bypasses repo anchors")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_SingleTopicExplanationSkipsAnchorSkeleton(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/analysis_ir.go": true,
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:          "internal/types/analysis_ir.go",
			LineStart:       574,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Criterion",
			Kind:            types.EvidenceDirect,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				SubTopics: []types.SubTopic{
					{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "single-topic explanation is already grounded",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("single-topic explanation must not be blocked by anchor skeleton downgrade: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatal("InvestigationComplete should be set for single-topic explanation")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_MultiTopicExplanationAnchorsUseSharedSurfacePlan(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/analysis_ir.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		EvidenceItems: []types.EvidenceItem{
			{
				Source:          "internal/types/analysis_ir.go",
				LineStart:       574,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "Criterion",
				Kind:            types.EvidenceDirect,
				Summary:         "Criterion 定义 hypothesis criterion 的结构。",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
			{
				Source:          "internal/types/analysis_ir.go",
				LineStart:       896,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "Hypothesis",
				Kind:            types.EvidenceDirect,
				Summary:         "Hypothesis 表示单条假设。",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
			{
				Source:          "internal/types/analysis_ir.go",
				LineStart:       933,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "AnalysisIR.HypothesisSet",
				Kind:            types.EvidenceDirect,
				Summary:         "AnalysisIR.HypothesisSet 持有 analyzer 产出的假设集合。",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				SubTopics: []types.SubTopic{
					{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
					{Summary: "Hypothesis 的角色", Entities: []string{"Hypothesis"}},
					{Summary: "AnalysisIR 如何持有 HypothesisSet", Entities: []string{"AnalysisIR.HypothesisSet", "HypothesisSet"}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "enough evidence collected",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("shared answer surface plan should accept grounded bus evidence, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete should be set when the shared plan already covers every explanation sub-topic")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadBlocks
// reproduces the 2026-04-18 "explorer calls subagent how" bug at the
// tool level. When the explorer's keyword-search top-K ranked files
// remain unread AND the declared RequirementKind is a breadth-intent,
// the gate queues PendingReads so the LLM's complete call is
// downgraded with the standard "Forced Read List" message.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadBlocks(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 50},
		{Path: "internal/agent/subagent.go", Score: 40},
		{Path: "internal/tool/propose_sub_agents.go", Score: 29},
		{Path: "internal/orchestrator/orchestrator.go", Score: 24},
		{Path: "internal/agent/sub_explorer.go", Score: 23},
	})
	closure := mut.EvidenceClosure()
	// LLM only read 2 of the top-5 — the rest are unread.
	closure.SetReadSet(map[string]bool{
		"internal/agent/explorer.go": true,
		"internal/agent/subagent.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the answer",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected DOWNGRADED message when top-K are unread, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "propose_sub_agents.go") {
		t.Errorf("expected unread top-K file in forced-read list, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must remain false on downgrade")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RuntimeArtifactSourceOptionalDoesNotForceRead(t *testing.T) {
	mut := types.NewMutableState("runtime trace")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "record_trace_20260526174055.systrace", Score: 90, ExactEntityRank: 3},
		{Path: "sys.systrace", Score: 80, ExactEntityRank: 2},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "runtime trace"}}},
				AnalyzerHints: types.AnalyzerHints{
					Kind:         "mechanism",
					ExactTargets: []string{"record_trace_20260526174055.systrace", "Choreographer#doFrame"},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "trace_query established the runtime sleep/wakeup chain and no current-source anchor was resolved",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]string{{
			"kind":       "scalar_value",
			"label":      "sleep duration",
			"value":      "135",
			"unit":       "ms",
			"provenance": "trace_query.wakeup_chain",
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("runtime-only phase1 ranking must not force current-source reads: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("runtime closure should complete when source lane is optional")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RequiredSourceSkipsRuntimeArtifactSeed(t *testing.T) {
	mut := types.NewMutableState("runtime trace plus current code")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "record_trace_20260526174055.systrace", Score: 95, ExactEntityRank: 3},
		{Path: "internal/agent/explorer.go", Score: 80, ExactEntityRank: 2},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "runtime trace"}}},
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
					SourceQuotes:                        []string{"结合当前代码解释"},
					Confidence:                          0.9,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "source explanation still needs implementation evidence",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("required source lane should still downgrade until source read: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "record_trace_20260526174055.systrace") {
		t.Fatalf("runtime artifact path must not appear as forced source read: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/agent/explorer.go") {
		t.Fatalf("current-source seed should still be forced: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_HistoryCurrentCodeSkipsGenericForcedReads(t *testing.T) {
	mut := types.NewMutableState("history-backed current-code explanation")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/tool/emit_evidence.go", Score: 58, ExactEntityRank: 2},
		{Path: "internal/tool/emit_investigation_complete.go", Score: 56, ExactEntityRank: 2},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "commit diff establishes the change; current source evidence explains the implementation",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "Forced Read List") ||
		strings.Contains(res.Summary, "pending forced reads") {
		t.Fatalf("history+current explanation should not be forced into generic cross-file read gates, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("history+current explanation should complete once the model declares the mixed evidence boundary")
	}
	if pending := mut.EvidenceClosure().PendingReads(); len(pending) != 0 {
		t.Fatalf("generic forced-read gates should not queue pending reads for this shape: %+v", pending)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_HistoryCurrentExplicitCallChainStillBlocks(t *testing.T) {
	mut := types.NewMutableState("history-backed explicit current call-chain")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/tool/emit_evidence.go", Score: 58, ExactEntityRank: 2},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentTrace,
				PredicateAxis: types.AxisCall,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:         string(types.ReqCallChain),
					ExactTargets: []string{"Start", "Finish"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: false},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "not enough current endpoint evidence yet",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("explicit source-to-sink history+current trace must preserve current-code forced-read gates, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("explicit endpoint call-chain should remain open until current-code evidence is gathered")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadSkipsKeywordOnlyAfterReadFocus(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60},
		{Path: "internal/agent/sub_explorer.go", Score: 50},
		{Path: "internal/agent/explorer_erm.go", Score: 49},
		{Path: "internal/agent/agent.go", Score: 48},
		{Path: "internal/agent/answer_document_evaluator.go", Score: 47},
	})
	mut.SetSearchGraph(&repotypes.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/agent/explorer.go": {
				RelPath: "internal/agent/explorer.go",
			},
			"internal/agent/explorer_erm.go": {
				RelPath: "internal/agent/explorer_erm.go",
			},
			"internal/agent/sub_explorer.go": {
				RelPath: "internal/agent/sub_explorer.go",
			},
			"internal/agent/agent.go": {
				RelPath: "internal/agent/agent.go",
			},
			"internal/agent/answer_document_evaluator.go": {
				RelPath: "internal/agent/answer_document_evaluator.go",
			},
		},
		ImportGraph: map[string][]string{
			"internal/agent/explorer.go": {"internal/agent/explorer_erm.go"},
		},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/agent/explorer.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind:     "mechanism",
					Entities: []string{"ContinuationPrompt"},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("keyword-only siblings should not trip phase1_unread after graph focus, got: %s", res.Summary)
	}
	for _, pending := range closure.PendingReads() {
		if pending.Origin == "phase1_unread" {
			t.Fatalf("phase1_unread should not queue keyword-only siblings after focus: %+v", pending)
		}
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when only non-mandatory ranked files remain unread")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadHonorsCanonicalReadSet(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/tool/repomap/tool.go", Score: 42, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{".\\internal\\tool\\repomap\\tool.go": true})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("canonical read-set match should prevent phase1_unread downgrade, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when the ranked file was already read")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_ConfigMappingMultiAnchorBlocks(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 2
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "cmd/root.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/types/config.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"cmd/root.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "config_mapping"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the config flow",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("multi-anchor config_mapping should trigger phase1_unread, got: %s", res.Summary)
	}
	var found bool
	for _, pending := range closure.PendingReads() {
		if pending.Origin == "phase1_unread" && pending.File == "internal/types/config.go" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected phase1_unread PendingRead for config anchor, got %+v", closure.PendingReads())
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("InvestigationComplete must remain false when config anchor is unread")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_PrimaryAnchorUnreadBlocks(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/tool/repomap/tool.go", Score: 42, ExactEntityRank: 2},
		{Path: "internal/context/builder.go", Score: 41},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected DOWNGRADED when primary anchor is unread, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/tool/repomap/tool.go") {
		t.Fatalf("expected primary anchor file in downgrade message, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must remain false when primary anchor is unread")
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_PrincipalMechanismBoundaryBypassesGenericForcedReads(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 4
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	mut := types.NewMutableState("io_uring send recv mechanism")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "io_uring/opdef.c", Score: 80, ExactEntityRank: 1},
		{Path: "io_uring/net.c", Score: 70, ExactEntityRank: 2},
		{Path: "io_uring/io_uring.c", Score: 60, ExactEntityRank: 3},
		{Path: "io_uring/cmd_net.c", Score: 50, ExactEntityRank: 4},
	})
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "io_uring/opdef.c", LineStart: 277,
			AnchorKind: types.AnchorInitializer, AnchorSymbol: "IORING_OP_SEND",
			Subject: "IORING_OP_SEND", Object: "io_send", GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceDirect, Source: "io_uring/net.c", LineStart: 646,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "io_send",
			Subject: "io_send", Object: "sock_sendmsg", GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceDirect, Source: "net/socket.c", LineStart: 810,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "sock_sendmsg",
			Subject: "sock_sendmsg", Object: "sock_sendmsg_nosec", GroundingStatus: types.GroundingGrounded,
		},
	}
	mut.AppendEvidence(evidence)
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"io_uring/opdef.c": true,
		"io_uring/net.c":   true,
		"net/socket.c":     true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Scenario:   types.ScenarioArchitectureExplain,
				Complexity: types.ComplexityComplex,
				Predicates: types.SemanticPredicates{IsCrossComponent: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqMechanism),
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "SEND/RECV opcode registration, io_uring execution, and socket calls are all covered by grounded principal evidence",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{
			{
				"kind":    "member_set",
				"label":   "SEND/RECV principal path",
				"role":    "principal_answer",
				"value":   "3",
				"members": []string{"IORING_OP_SEND", "io_send", "sock_sendmsg"},
				"support_refs": []string{
					"IORING_OP_SEND: io_uring/opdef.c:277",
					"io_send: io_uring/net.c:646",
					"sock_sendmsg: net/socket.c:810",
				},
			},
		},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") || strings.Contains(res.Summary, "Forced Read List") {
		t.Fatalf("grounded model-owned principal boundary must not be overridden by generic forced-read debt, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete once grounded principal mechanism boundary is declared")
	}
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Fatalf("accepted completion should clear advisory generic pending reads, got %+v", pending)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_GroundedNarrativeEvidenceBypassesGenericForcedReadsWithoutAggregateFacts(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 4
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	mut := types.NewMutableState("io_uring send recv mechanism")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "tools/include/io_uring/mini_liburing.h", Score: 95, ExactEntityRank: 5},
		{Path: "io_uring/opdef.c", Score: 80, ExactEntityRank: 1},
		{Path: "io_uring/net.c", Score: 70, ExactEntityRank: 2},
		{Path: "net/socket.c", Score: 60, ExactEntityRank: 3},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "io_uring/opdef.c", LineStart: 277,
			AnchorKind: types.AnchorInitializer, AnchorSymbol: "IORING_OP_SEND",
			Subject: "IORING_OP_SEND", Object: "io_send", GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceDirect, Source: "io_uring/net.c", LineStart: 646,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "io_send",
			Subject: "io_send", Object: "sock_sendmsg", GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceDirect, Source: "net/socket.c", LineStart: 810,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "sock_sendmsg",
			Subject: "sock_sendmsg", Object: "sock_sendmsg_nosec", GroundingStatus: types.GroundingGrounded,
		},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"io_uring/opdef.c": true,
		"io_uring/net.c":   true,
		"net/socket.c":     true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Scenario:   types.ScenarioArchitectureExplain,
				Complexity: types.ComplexityComplex,
				Predicates: types.SemanticPredicates{IsCrossComponent: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqMechanism),
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 2},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "已覆盖 opcode 注册、io_uring 执行函数和 socket 层调用，关键内核路径证据已完整落地。",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "mini_liburing") ||
		strings.Contains(res.Summary, "DOWNGRADED") ||
		strings.Contains(res.Summary, "Forced Read List") {
		t.Fatalf("grounded narrative evidence boundary must not be overruled by generic pre-scan anchors, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete after grounded narrative evidence establishes the answer boundary")
	}
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Fatalf("generic pending reads should not be queued after grounded narrative boundary, got %+v", pending)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_CallChainGroundedEvidenceBypassesGenericForcedReads(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 5
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	mut := types.NewMutableState("bpf map update call chain")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "kernel/bpf/syscall.c", Score: 90, ExactEntityRank: 3},
		{Path: "kernel/bpf/hashtab.c", Score: 80, ExactEntityRank: 2},
		{Path: "kernel/bpf/arraymap.c", Score: 78, ExactEntityRank: 2},
		{Path: "drivers/platform/surface/surface_aggregator_registry.c", Score: 62, ExactEntityRank: 1},
		{Path: "arch/arm64/include/asm/topology.h", Score: 60, ExactEntityRank: 1},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceRelationship, Source: "kernel/bpf/syscall.c", LineStart: 6359,
			AnchorKind: types.AnchorCall, AnchorSymbol: "map_update_elem",
			Subject: "bpf syscall entry", Predicate: "dispatches", Object: "map_update_elem",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, Source: "kernel/bpf/syscall.c", LineStart: 200,
			AnchorKind: types.AnchorInitializer, AnchorSymbol: "bpf_map_ops",
			Subject: "bpf_map_ops", Predicate: "selects", Object: "update_elem",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, Source: "kernel/bpf/hashtab.c", LineStart: 2364,
			AnchorKind: types.AnchorInitializer, AnchorSymbol: "map_update_elem",
			Subject: "hash map ops", Predicate: "calls", Object: "htab_map_update_elem",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, Source: "kernel/bpf/arraymap.c", LineStart: 811,
			AnchorKind: types.AnchorInitializer, AnchorSymbol: "map_update_elem",
			Subject: "array map ops", Predicate: "calls", Object: "array_map_update_elem",
			GroundingStatus: types.GroundingGrounded,
		},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"kernel/bpf/syscall.c":  true,
		"kernel/bpf/hashtab.c":  true,
		"kernel/bpf/arraymap.c": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentTrace,
				Scenario:      types.ScenarioArchitectureExplain,
				Complexity:    types.ComplexityComplex,
				PredicateAxis: types.AxisCall,
				Predicates:    types.SemanticPredicates{IsCrossComponent: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqCallChain),
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 3},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "已读并落地 syscall 分发、ops 表和 hash/array map update_elem 实现的主调用链证据；topology 候选只是导航噪音。",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "surface_aggregator_registry") ||
		strings.Contains(res.Summary, "topology.h") ||
		strings.Contains(res.Summary, "DOWNGRADED") ||
		strings.Contains(res.Summary, "Forced Read List") {
		t.Fatalf("grounded call-chain boundary must not be overruled by generic ranker collateral, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete after grounded call-chain evidence establishes the answer boundary")
	}
	if pending := closure.PendingReads(); len(pending) != 0 {
		t.Fatalf("generic pending reads should not be queued after grounded call-chain boundary, got %+v", pending)
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_RelationMemberSetBypassesGenericTopologyReads(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 5
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	mut := types.NewMutableState("哪个 agent 可以调用 subagent")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/sub_explorer.go", Score: 90, ExactEntityRank: 3},
		{Path: "internal/agent/agent.go", Score: 88, ExactEntityRank: 3},
		{Path: "internal/tool/propose_sub_agents.go", Score: 86, ExactEntityRank: 2},
		{Path: "internal/orchestrator/topology.go", Score: 70, ExactEntityRank: 2},
		{Path: "internal/tool/repomap/topology/topology.go", Score: 68, ExactEntityRank: 2},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/sub_explorer.go", LineStart: 32,
			AnchorKind: types.AnchorReturn, AnchorSymbol: "Name",
			Subject: "SubExplorer.Name", Predicate: "returns", Object: "explorer",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/agent.go", LineStart: 3412,
			AnchorKind: types.AnchorCall, AnchorSymbol: "SubAgents.Get",
			Subject: "BaseAgent.buildToolSchemas", Predicate: "gates tool exposure by", Object: "registered subagent name",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/tool/propose_sub_agents.go", LineStart: 18,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "ProposeSubAgents",
			Subject: "propose_sub_agents", Predicate: "carries", Object: "subagent requests",
			GroundingStatus: types.GroundingGrounded,
		},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/agent/sub_explorer.go":      true,
		"internal/agent/agent.go":             true,
		"internal/tool/propose_sub_agents.go": true,
		"internal/agent/subagent_runtime.go":  true,
		"internal/types/subagent.go":          true,
		"internal/agent/subagent.go":          true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentTrace,
				Scenario:      types.ScenarioArchitectureExplain,
				Complexity:    types.ComplexityModerate,
				PredicateAxis: types.AxisCall,
				Predicates: types.SemanticPredicates{
					IsRelationalLookup:    true,
					IsCategoryEnumeration: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:            string(types.ReqCallChain),
					PrimaryEntities: []string{"SubAgentRequest", "sub_explorer.go"},
					Entities:        []string{"SubAgentRequest", "sub_explorer.go"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 2},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "已落地 explorer 是可调用 subagent 的 principal relation member，topology 文件只是预扫描候选。",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind":         "member_set",
			"role":         "principal_answer",
			"label":        "agents that can call subagent",
			"value":        "1",
			"members":      []string{"explorer"},
			"support_refs": []string{"explorer: internal/agent/sub_explorer.go:32"},
		}},
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "topology.go") ||
		strings.Contains(res.Summary, "DOWNGRADED") ||
		strings.Contains(res.Summary, "Forced Read List") ||
		strings.Contains(res.Summary, "Suspicious Anchors") {
		t.Fatalf("typed relation member_set boundary must not be overruled by generic topology candidates, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete after typed relation member_set establishes the answer boundary")
	}
	for _, pending := range closure.PendingReads() {
		if pending.Origin == "phase1_unread" {
			t.Fatalf("generic phase1 topology reads should not remain pending after relation member_set boundary: %+v", pending)
		}
	}
}

func TestPartitionPendingReadsForAcceptedClosure_KeepsRequiredCurrentSourceBlocking(t *testing.T) {
	mut := types.NewMutableState("mixed current source")
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/explorer.go", LineStart: 10,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Explorer",
			Subject: "Explorer", GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceDirect, Source: "internal/tool/repomap/tool.go", LineStart: 20,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "RepoMap",
			Subject: "RepoMap", GroundingStatus: types.GroundingGrounded,
		},
	}
	mut.AppendEvidence(evidence)
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Scenario:   types.ScenarioArchitectureExplain,
				Complexity: types.ComplexityComplex,
				AnalyzerHints: types.AnalyzerHints{
					Kind: string(types.ReqMechanism),
				},
			},
		},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "principal mechanism",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Explorer", "RepoMap"},
		SupportRefs: []string{
			"Explorer: internal/agent/explorer.go:10",
			"RepoMap: internal/tool/repomap/tool.go:20",
		},
	}}
	pending := []types.PendingRead{
		{File: "support.go", Origin: "pre_complete.primary_anchor"},
		{File: "bridged-support.go", Origin: "auto_bridge.pre_complete.primary_anchor"},
		{File: "required.go", Origin: "required_file_hint_unread"},
	}
	blocking, advisory := partitionPendingReadsForAcceptedClosure(bus, pending, facts, evidence)
	if len(blocking) != 1 || blocking[0].File != "required.go" {
		t.Fatalf("required current-source file must stay blocking, got blocking=%+v advisory=%+v", blocking, advisory)
	}
	if len(advisory) != 2 || advisory[0].File != "support.go" || advisory[1].File != "bridged-support.go" {
		t.Fatalf("generic primary-anchor debt should become advisory, got blocking=%+v advisory=%+v", blocking, advisory)
	}
}

func TestPrimaryAnchorPendingRead_CapabilitySurfaceSkipsToolImplementationAnchor(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/analysis/dataflow/engine.go", Score: 60, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
					CapabilitySurface: &types.CapabilitySurfaceHint{
						Binding: types.StageBinding{
							Stage: types.StageAnalyze,
							Agent: types.AgentAnalyzer,
							Skill: "analysis-skill",
						},
						Tool: "read_file",
						AuthorityFiles: []string{
							"internal/orchestrator/topology.go",
							"internal/skill/analysis_contract.go",
							"internal/agent/agent.go",
							"internal/agent/analyzer.go",
						},
					},
				},
			},
		},
	}
	raisePrimaryAnchorPendingRead(bus, closure)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.primary_anchor" {
			t.Fatalf("capability surface should skip tool implementation anchor forcing, got %+v", p)
		}
	}
}

func TestPhase1UnreadPendingReads_CapabilitySurfaceQueuesAuthorityFilesOnly(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 4
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/analysis/dataflow/engine.go", Score: 70, ExactEntityRank: 2},
		{Path: "internal/orchestrator/topology.go", Score: 68, ExactEntityRank: 2},
		{Path: "internal/skill/analysis_contract.go", Score: 66, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/orchestrator/topology.go": true,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
					CapabilitySurface: &types.CapabilitySurfaceHint{
						Binding: types.StageBinding{
							Stage: types.StageAnalyze,
							Agent: types.AgentAnalyzer,
							Skill: "analysis-skill",
						},
						Tool: "read_file",
						AuthorityFiles: []string{
							"internal/orchestrator/topology.go",
							"internal/skill/analysis_contract.go",
							"internal/agent/agent.go",
							"internal/agent/analyzer.go",
						},
					},
				},
			},
		},
	}
	raisePhase1UnreadPendingReads(bus, closure)
	got := closure.PendingReads()
	if len(got) == 0 {
		t.Fatal("expected unread capability authority files to be queued")
	}
	want := map[string]bool{
		"internal/skill/analysis_contract.go": true,
		"internal/agent/agent.go":             true,
		"internal/agent/analyzer.go":          true,
	}
	for _, p := range got {
		if p.File == "internal/analysis/dataflow/engine.go" {
			t.Fatalf("tool implementation file should not be queued for capability closure: %+v", got)
		}
		if !want[p.File] {
			t.Fatalf("unexpected pending capability file %q in %+v", p.File, got)
		}
		delete(want, p.File)
	}
	if len(want) != 0 {
		t.Fatalf("missing capability authority files from pending queue: %v", want)
	}
}

func TestMultiPathAnchorChecks_CapabilitySurfaceSkipsGate(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/agent.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/agent/analyzer.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/agent/agent.go":    true,
		"internal/agent/analyzer.go": true,
	})
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/agent/agent.go":    {{Start: 1, End: 200}},
		"internal/agent/analyzer.go": {{Start: 1, End: 5}},
	})
	closure.SetFileTotalLines(map[string]int{
		"internal/agent/agent.go":    400,
		"internal/agent/analyzer.go": 1200,
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
					CapabilitySurface: &types.CapabilitySurfaceHint{
						Binding: types.StageBinding{
							Stage: types.StageAnalyze,
							Agent: types.AgentAnalyzer,
							Skill: "analysis-skill",
						},
						Tool: "read_file",
						AuthorityFiles: []string{
							"internal/orchestrator/topology.go",
							"internal/skill/analysis_contract.go",
							"internal/agent/agent.go",
							"internal/agent/analyzer.go",
						},
					},
				},
			},
		},
	}
	applyMultiPathAnchorChecks(bus, closure)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" {
			t.Fatalf("capability surface should not demand cross-file coverage parity, got %+v", p)
		}
	}
}

func TestEmitInvestigationComplete_PreCompleteCheck_PrimaryAnchorHonorsDispatchReadHistory(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/tool/repomap/tool.go", Score: 42, ExactEntityRank: 2},
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/tool/repomap/tool.go: showing lines 141-160 of 323 total]\n",
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "traced the mechanism",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "internal/tool/repomap/tool.go") {
		t.Fatalf("dispatch read history should satisfy the primary anchor gate, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete should be set when the dispatch already read the anchor")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_Registration
// is the negative control: a non-breadth intent (registration) must
// NOT be blocked by the phase1-unread gate even when ranked files are
// unread. Single-lookup intents commonly need only 1-2 files.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_Registration(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "a.go", Score: 50},
		{Path: "b.go", Score: 40},
		{Path: "c.go", Score: 30},
		{Path: "d.go", Score: 20},
		{Path: "e.go", Score: 10},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"a.go": true})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "registration"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "found the registration",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, _ := tool.Execute(bus, params)
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("registration intent should not be blocked by phase1-unread gate: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("non-breadth intent should proceed when no other blockers")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_AbsenceBypass
// verifies absence_justification bypasses the phase1-unread gate —
// when the LLM honestly declares "no such thing in the repo" there is
// nothing to cite so forcing more reads is noise.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1Unread_AbsenceBypass(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "a.go", Score: 50},
		{Path: "b.go", Score: 40},
		{Path: "c.go", Score: 30},
		{Path: "d.go", Score: 20},
		{Path: "e.go", Score: 10},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":                "no such mechanism exists",
		"confidence":            "high",
		"result_kind":           "absence",
		"absence_justification": "grep produced zero hits for the claimed symbol",
	})
	res, _ := tool.Execute(bus, params)
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("absence answer must bypass phase1-unread gate: %s", res.Summary)
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadLatchFiresOnce:
// T2.1 — the phase1_unread gate must not keep re-firing on subsequent
// emit_investigation_complete calls within the same pipeline. Once
// the gate has surfaced the unread top-K files + raised the
// RepairExpandSearch directive, a second firing adds no information
// and only amplifies redispatches. The latch lives on EvidenceClosure,
// reset on task entry.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadLatchFiresOnce(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "a.go", Score: 50},
		{Path: "b.go", Score: 40},
		{Path: "c.go", Score: 30},
		{Path: "d.go", Score: 20},
		{Path: "e.go", Score: 10},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"a.go": true}) // only 1 of 5 read
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "first attempt",
		"confidence":  "high",
		"result_kind": "resolved",
	})

	// First call: gate fires, files queued, latch flips.
	res1, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if !strings.Contains(res1.Summary, "DOWNGRADED") {
		t.Fatalf("first call must DOWNGRADE with top-K unread, got: %s", res1.Summary)
	}
	if !strings.Contains(res1.Summary, "b.go") || !strings.Contains(res1.Summary, "c.go") {
		t.Errorf("first call summary must list unread top-K files, got: %s", res1.Summary)
	}
	if !closure.Phase1UnreadFired() {
		t.Errorf("latch must be set after first firing")
	}

	// Clear any PendingReads that were queued so the SECOND pre-complete
	// check isn't blocked by leftover state from the first — we want to
	// isolate whether the gate itself fires a second time.
	for _, p := range closure.PendingReads() {
		if p.Origin == "phase1_unread" || p.Origin == "pre_complete.primary_anchor" {
			closure.ClearPendingReadFor(p.File)
		}
	}

	// Second call with same unread top-K setup: gate must NOT re-fire.
	// PendingReads from phase1_unread origin must remain empty.
	res2, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	phase1Queued := false
	for _, p := range closure.PendingReads() {
		if p.Origin == "phase1_unread" {
			phase1Queued = true
			break
		}
	}
	if phase1Queued {
		t.Errorf("latch must suppress second firing; found phase1_unread PendingRead: %s", res2.Summary)
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadLatchResetOnTaskEntry:
// T2.1 — the latch lives on EvidenceClosure and must be cleared when
// the closure is reset for a new task. Otherwise a second pipeline on
// the same REPL process would skip phase1_unread entirely.
func TestEmitInvestigationComplete_PreCompleteCheck_Phase1UnreadLatchResetOnTaskEntry(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	closure.MarkPhase1UnreadFired()
	if !closure.Phase1UnreadFired() {
		t.Fatalf("precondition: latch should be set")
	}
	closure.Reset()
	if closure.Phase1UnreadFired() {
		t.Errorf("Reset must clear phase1UnreadFired latch")
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_AbsenceWaivesCitationFloor:
// absence_justification skips check (b) by contract.
func TestEmitInvestigationComplete_PreCompleteCheck_AbsenceWaivesFloor(t *testing.T) {
	mut := types.NewMutableState("test")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 1},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":                "the system has no such handler",
		"confidence":            "high",
		"result_kind":           "absence",
		"absence_justification": "no handler with that name exists in the repo",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("absence path should not downgrade on citation floor: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Errorf("absence path should still mark complete")
	}
}

// TestPrimaryAnchorPendingRead_ProjectOrientationSkipsGate proves
// that the "primary anchor unread" gate also short-circuits on
// project-orientation questions. Cross-gate consistency: all three
// pre-complete gates (primary_anchor / phase1_unread /
// multi_path_coverage) must agree on what "orientation question"
// means so an orientation answer cannot be partially blocked.
func TestPrimaryAnchorPendingRead_ProjectOrientationSkipsGate(t *testing.T) {
	mut := types.NewMutableState("test")
	// Phase1 ranking with an unread exact-anchor file. Pre-fix the
	// primary_anchor gate would queue this as a PendingRead. The
	// orientation skip must prevent that.
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "explorer.go", Score: 60, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Complexity: types.ComplexitySimple,
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism", // would otherwise trigger
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason": "project overview", "confidence": "high", "result_kind": "resolved",
	})
	if _, err := tool.Execute(bus, params); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.primary_anchor" {
			t.Errorf("orientation must skip primary_anchor gate; got %+v", p)
		}
	}
}

// TestPhase1UnreadPendingReads_ProjectOrientationSkipsGate proves
// the phase1_unread gate also honours the orientation short-circuit.
func TestPhase1UnreadPendingReads_ProjectOrientationSkipsGate(t *testing.T) {
	mut := types.NewMutableState("test")
	// Populate Phase1Ranking with > Phase1UnreadTopK entries, all
	// unread — pre-fix this would trigger the gate.
	ranks := make([]types.Phase1RankedFile, 0, 6)
	for i := 0; i < 6; i++ {
		ranks = append(ranks, types.Phase1RankedFile{
			Path:            fmt.Sprintf("file%d.go", i),
			Score:           float64(60 - i),
			ExactEntityRank: 1,
		})
	}
	mut.SetPhase1Ranking(ranks)
	closure := mut.EvidenceClosure()

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Complexity: types.ComplexitySimple,
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason": "project overview", "confidence": "high", "result_kind": "resolved",
	})
	if _, err := tool.Execute(bus, params); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "phase1_unread" {
			t.Errorf("orientation must skip phase1_unread gate; got %+v", p)
		}
	}
}

// TestMultiPathAnchorChecks_ProjectOrientationSkipsGate locks the
// structural-signal skip: when the analyzer classifies the question
// as intent=explain + complexity=simple + no PrimaryEntities + no
// cross-component, the gate must not fire even when coverage looks
// unbalanced. A project-orientation question ("what does this repo
// do?") answers from README + manifest + entry-point and never
// needs cross-component depth parity.
//
// The detection is structured-signal-only — see
// IsProjectOrientationQuestion. No keyword matching on RawRequest;
// the analyzer LLM's own complexity/intent/predicates classification
// IS the signal.
//
// Cross-gate consistency: this test guards the orientation skip on
// the multi-path symbol-anchored gate (origin
// "pre_complete.multi_path_anchor"); a separate test in this file
// guards the same skip on primary_anchor + phase1_unread.
func TestMultiPathAnchorChecks_ProjectOrientationSkipsGate(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "README.md", Score: 60, ExactEntityRank: 2},
		{Path: "main.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"README.md": true, "main.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{
		"README.md": {{Start: 1, End: 50}},
		"main.go":   {{Start: 1, End: 5}},
	})
	closure.SetFileTotalLines(map[string]int{"README.md": 50, "main.go": 1000})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Complexity: types.ComplexitySimple,
				AnalyzerHints: types.AnalyzerHints{
					Kind: "mechanism",
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason": "project overview", "confidence": "high", "result_kind": "resolved",
	})
	if _, err := tool.Execute(bus, params); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" {
			t.Errorf("project-orientation question must skip multi-path symbol-anchored gate; got %+v", p)
		}
	}
}

// TestApplyMultiPathAnchorChecks_SingleAnchorSkips guards against
// false-fire when only one primary anchor exists — single-subject
// questions have no parity target.
func TestApplyMultiPathAnchorChecks_SingleAnchorSkips(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/explorer.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/agent/explorer.go": {{Start: 1, End: 5}},
	})
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}
	applyMultiPathAnchorChecks(bus, closure)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" {
			t.Fatalf("single anchor must not trigger multi-path symbol-anchored gate, got %+v", p)
		}
	}
}

// TestApplyMultiPathAnchorChecks_AdvisoryHintNonBlocking pins the
// load-bearing UX rule: when both Signal 1 and Signal 2 fail (no
// question-related symbol identifiable AND no grounded evidence
// emitted), the gate emits a NON-BLOCKING advisory directive only —
// it MUST NOT enqueue a PendingRead, otherwise emit_investigation_complete
// would still be blocked and we are back to the pre-2026-05-01
// pathology of stalling completion on signals we cannot defend.
func TestApplyMultiPathAnchorChecks_AdvisoryHintNonBlocking(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "anchor_a.go", Score: 60, ExactEntityRank: 2},
		{Path: "anchor_b.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	// No reads, no evidence — both signals fail; no symbol oracle
	// (Mutable.SearchGraph returns nil) so Signal 1 cannot identify
	// any symbol either. Engine should pick SignalOpaqueAdvisory.
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "mechanism",
					PrimaryEntities: []string{"someEntity"},
				},
			},
		},
	}
	applyMultiPathAnchorChecks(bus, closure)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" {
			t.Fatalf("opaque-advisory case MUST NOT enqueue PendingRead (would block completion), got %+v", p)
		}
	}
	// At least one RepairExpandSearch advisory directive must have
	// been emitted so the LLM is informed without being blocked.
	advisoryCount := 0
	for _, r := range closure.ActiveRepairs() {
		if r.Origin == "pre_complete.multi_path_anchor" && r.Kind == types.RepairExpandSearch {
			advisoryCount++
		}
	}
	if advisoryCount == 0 {
		t.Fatalf("opaque-advisory case must emit at least one RepairExpandSearch advisory; got %+v", closure.ActiveRepairs())
	}
}

func TestApplyMultiPathAnchorChecks_CallChainUnrelatedSymbolDemandIsAdvisory(t *testing.T) {
	mut := types.NewMutableState("trace buildAnalysisIR to gate.Run")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/analyzer.go", Score: 70, ExactEntityRank: 2},
		{Path: "internal/analysis/dataflow/engine.go", Score: 60, ExactEntityRank: 2},
	})
	mut.SetSearchGraph(&repotypes.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Symbols: []repotypes.Symbol{
					{Name: "buildAnalysisIR", File: "internal/agent/analyzer.go", Line: 100, EndLine: 300},
					{Name: "RunWith", File: "internal/agent/analyzer.go", Line: 300, EndLine: 300},
				},
			},
			"internal/analysis/dataflow/engine.go": {
				RelPath: "internal/analysis/dataflow/engine.go",
				Symbols: []repotypes.Symbol{
					{Name: "Analyze", File: "internal/analysis/dataflow/engine.go", Line: 22, EndLine: 113},
				},
			},
		},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/analyzer.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/agent/analyzer.go": {{Start: 85, End: 315}},
	})
	closure.SetFileTotalLines(map[string]int{
		"internal/agent/analyzer.go":           500,
		"internal/analysis/dataflow/engine.go": 780,
	})
	evidence := []types.EvidenceItem{
		{ID: "start", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR", Subject: "buildAnalysisIR", Source: "internal/agent/analyzer.go", LineStart: 100},
		{ID: "sink", Kind: types.EvidenceDirect, AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith", Subject: "buildAnalysisIR", Object: "gate.RunWith", Source: "internal/agent/analyzer.go", LineStart: 300},
	}
	mut.AppendEvidence(evidence)
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              string(types.ReqCallChain),
					MentionedEntities: []string{"buildAnalysisIR", "gate.Run"},
					PrimaryEntities:   []string{"Analyze"},
				},
			},
		},
	}

	applyMultiPathAnchorChecksWithEvidence(bus, closure, evidence)
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_anchor" && p.File == "internal/analysis/dataflow/engine.go" {
			t.Fatalf("call-chain should not hard-block on unrelated pre-scan symbol demand, got %+v", p)
		}
	}
	var advisory bool
	for _, r := range closure.ActiveRepairs() {
		if r.Origin == "pre_complete.multi_path_anchor" && r.Kind == types.RepairExpandSearch && len(r.Files) == 1 && r.Files[0] == "internal/analysis/dataflow/engine.go" {
			advisory = true
			break
		}
	}
	if !advisory {
		t.Fatalf("unrelated call-chain symbol demand should remain visible as advisory, got repairs=%+v", closure.ActiveRepairs())
	}
}

// === Multi-repo forced-read phantom-path fix ===
//
// docs/design/post_phase2a_forensic_followups.md §2.1.B — the 09:09
// production run had a 2-sub_repo workspace (codrax + opencode, with
// opencode containing a nested packages/opencode/...) where the
// pre-scanner queued forced-read entries with bare paths like
// "packages/opencode/src/tool/apply_patch.ts". The queue's
// canonicalisation key differed from the LLM's actual read of the
// FS-real "opencode/packages/opencode/src/tool/apply_patch.ts" —
// queue never drained → 10× DOWNGRADED + 5 min wasted before the
// LLM gave up. The LLM noted "the queue's block seems to be a bug"
// in its <think> block. The fix qualifies the path against the
// active-set gate at seed time so the queued canonical key matches
// what the LLM's read will produce.

// phantomPathTestGater is a focused stand-in for the real multigraph
// gater. It only models the auto-prefix bare-path branch of
// ResolveActiveSetPath that the seed actually exercises — single-repo
// bypass / inactive-prefix / absolute-path branches are out of scope
// for these tests and exercised separately in the multigraph package's
// own active_set_gate_test.go.
type phantomPathTestGater struct {
	uniqueMatchPrefix map[string]string // bare-path → sub_repo prefix; "" means no match
	ambiguousPaths    map[string]bool   // bare-path → multi-match (refused)
}

func (g *phantomPathTestGater) ResolveActiveSetPath(_ *types.BusContext, _, llmPath string, _ func(string) bool) types.ActiveSetGateResult {
	if g.ambiguousPaths[llmPath] {
		return types.ActiveSetGateResult{Allowed: false, RefusalProse: "ambiguous"}
	}
	for _, prefix := range g.uniqueMatchPrefix {
		if prefix != "" && strings.HasPrefix(llmPath, prefix+"/") {
			return types.ActiveSetGateResult{
				Allowed:        true,
				ResolvedPath:   llmPath,
				SubRepoRootRel: prefix,
			}
		}
	}
	prefix, ok := g.uniqueMatchPrefix[llmPath]
	if !ok || prefix == "" {
		return types.ActiveSetGateResult{Allowed: false, RefusalProse: "no match"}
	}
	return types.ActiveSetGateResult{
		Allowed:        true,
		ResolvedPath:   prefix + "/" + llmPath,
		SubRepoRootRel: prefix,
		AutoPrefixed:   true,
	}
}

func (g *phantomPathTestGater) ResolveActiveSetCommand(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{Allowed: true}
}

func TestQualifyForcedReadPathForMultiRepo_SingleRepoBypass(t *testing.T) {
	// nil MultiGraph → single-repo posture — helper passes the path
	// through byte-identical so the existing single-repo behaviour
	// is preserved.
	ctx := &types.BusContext{RepoRoot: "/tmp/single"}
	got, ok := qualifyForcedReadPathForMultiRepo(ctx, "internal/tool/foo.go")
	if !ok {
		t.Fatal("single-repo bypass must return ok=true")
	}
	if got != "internal/tool/foo.go" {
		t.Fatalf("single-repo bypass must not modify path, got %q", got)
	}
}

func TestQualifyForcedReadPathForMultiRepo_QualifiesUniqueMatch(t *testing.T) {
	// Verbatim 09:09 case shape: bare path resolves to a unique
	// active sub_repo on disk, helper returns the prefixed form.
	gater := &phantomPathTestGater{
		uniqueMatchPrefix: map[string]string{
			"packages/opencode/src/tool/apply_patch.ts": "opencode",
		},
	}
	ctx := &types.BusContext{MultiGraph: gater}
	got, ok := qualifyForcedReadPathForMultiRepo(ctx, "packages/opencode/src/tool/apply_patch.ts")
	if !ok {
		t.Fatal("unique-match bare path must qualify, ok was false")
	}
	want := "opencode/packages/opencode/src/tool/apply_patch.ts"
	if got != want {
		t.Fatalf("qualified path mismatch: got %q want %q", got, want)
	}
}

func TestQualifyForcedReadPathForMultiRepo_DropsPhantomPath(t *testing.T) {
	// Bare path that does not exist in any active sub_repo on disk:
	// helper returns ok=false so the caller drops the seed (queueing
	// a phantom entry would cause repeated DOWNGRADED rejects with
	// no recourse — the entire 09:09 forensic symptom).
	gater := &phantomPathTestGater{
		uniqueMatchPrefix: map[string]string{},
	}
	ctx := &types.BusContext{MultiGraph: gater}
	_, ok := qualifyForcedReadPathForMultiRepo(ctx, "packages/ghost/file.go")
	if ok {
		t.Fatal("phantom path must be dropped (ok=false), got ok=true")
	}
}

func TestRaisePhase1UnreadPendingReads_MultiRepoQualifiesBarePathAndDrains(t *testing.T) {
	// End-to-end: 09:09 forensic case replay. 2 active sub_repos
	// (codrax + opencode); explorer pre-scanner produced bare paths
	// from a graph keyed under the opencode sub_repo. Without the
	// fix the queued PendingRead would carry the bare path and the
	// subsequent LLM read of the FS-real opencode-prefixed path
	// would NOT drain it. With the fix the path is qualified at
	// seed time and the LLM's read drains the queue.

	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 2
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	gater := &phantomPathTestGater{
		uniqueMatchPrefix: map[string]string{
			"packages/opencode/src/tool/apply_patch.ts": "opencode",
			"packages/opencode/src/tool/shell.ts":       "opencode",
		},
	}

	mut := types.NewMutableState("multi-repo forced-read seed replay")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		// Bare paths as the ranker emits them in multi-repo posture —
		// the opencode/ prefix is missing because the graph itself is
		// keyed under the sub_repo.
		{Path: "packages/opencode/src/tool/apply_patch.ts", Score: 60, ExactEntityRank: 2},
		{Path: "packages/opencode/src/tool/shell.ts", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	bus := &types.BusContext{
		Mutable:    mut,
		RepoRoot:   t.TempDir(),
		MultiGraph: gater,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	raisePhase1UnreadPendingReads(bus, closure)

	pending := closure.PendingReads()
	if len(pending) != 2 {
		t.Fatalf("expected 2 PendingReads after multi-repo qualify, got %d: %+v", len(pending), pending)
	}
	for _, p := range pending {
		if p.Origin != "phase1_unread" {
			t.Errorf("PendingRead origin = %q, want phase1_unread", p.Origin)
		}
		// Assertion #1 from the prompt: forced-read seed produces the
		// FS-real prefixed path, NOT a phantom codrax/packages/... or
		// a bare packages/... that would dead-end the queue.
		if !strings.HasPrefix(p.File, "opencode/") {
			t.Errorf("PendingRead.File %q must carry opencode/ sub_repo prefix after seed qualification; bare path leak would replay the 09:09 phantom-path bug", p.File)
		}
		if strings.HasPrefix(p.File, "codrax/") {
			t.Errorf("PendingRead.File %q phantom-prefixed with codrax/ — the bug is reproduced", p.File)
		}
	}

	// Assertion #2 from the prompt: the LLM reading the FS-real
	// opencode-prefixed path drains the queue. With the bug, the
	// queue would linger forever because the read path's canonical
	// key did not match the bare-path entry's canonical key.
	closure.SetReadSet(map[string]bool{
		"opencode/packages/opencode/src/tool/apply_patch.ts": true,
		"opencode/packages/opencode/src/tool/shell.ts":       true,
	})
	drained := closure.DrainSatisfiedPendingReads()
	if drained != 2 {
		t.Fatalf("expected both PendingReads to drain after the LLM read the FS-real opencode-prefixed paths, drained=%d; remaining=%+v",
			drained, closure.PendingReads())
	}
	if remaining := closure.PendingReads(); len(remaining) != 0 {
		t.Fatalf("queue must be empty after drain, got %+v", remaining)
	}
}

func TestRaisePhase1UnreadPendingReads_MultiRepoPhantomPathSkipped(t *testing.T) {
	// When the bare path does not exist in any active sub_repo on
	// disk, the seed must drop it rather than queue an entry the
	// LLM cannot satisfy. (Without the fix the queued entry would
	// linger and the LLM would face repeated DOWNGRADED rejects on
	// emit_investigation_complete.) MinUnread=1 + only-one-phantom-
	// candidate means the gate produces ZERO queued reads.

	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.Phase1UnreadTopK = 1
	limits.Phase1UnreadMinUnread = 1
	SetAnalysisLimits(limits)

	gater := &phantomPathTestGater{
		uniqueMatchPrefix: map[string]string{}, // no matches — phantom
	}

	mut := types.NewMutableState("multi-repo phantom seed drop")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "packages/ghost/never_existed.go", Score: 50, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	bus := &types.BusContext{
		Mutable:    mut,
		RepoRoot:   t.TempDir(),
		MultiGraph: gater,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	raisePhase1UnreadPendingReads(bus, closure)

	if got := closure.PendingReads(); len(got) != 0 {
		t.Fatalf("phantom-path seed must be dropped, got %d PendingRead(s): %+v", len(got), got)
	}
}

func TestPrimaryAnchorPendingRead_MultiRepoBarePathHonorsPrefixedReadHistory(t *testing.T) {
	gater := &phantomPathTestGater{
		uniqueMatchPrefix: map[string]string{
			"packages/opencode/src/tool/read.ts": "opencode",
		},
	}
	mut := types.NewMutableState("multi-repo primary anchor read history")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "packages/opencode/src/tool/read.ts", Score: 60, ExactEntityRank: 2},
	})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[opencode/packages/opencode/src/tool/read.ts: showing lines 1-338 of 338 total]\n",
	})
	bus := &types.BusContext{
		Mutable:    mut,
		RepoRoot:   t.TempDir(),
		MultiGraph: gater,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "mechanism"},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":      "prefixed read history covers the bare primary anchor",
		"confidence":  "high",
		"result_kind": "resolved",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(res.Summary, "DOWNGRADED") || strings.Contains(res.Summary, "packages/opencode/src/tool/read.ts") {
		t.Fatalf("bare multi-repo primary anchor should be satisfied by prefixed read history, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should complete once prefixed read history covers the primary anchor")
	}
}

func TestApplyMultiPathAnchorChecks_MultiRepoQualifiesBareAnchors(t *testing.T) {
	prev := CurrentAnalysisLimits()
	t.Cleanup(func() { SetAnalysisLimits(prev) })
	limits := prev
	limits.MultiPathSymbolContextLines = 15
	SetAnalysisLimits(limits)

	gater := &phantomPathTestGater{
		uniqueMatchPrefix: map[string]string{
			"packages/opencode/src/tool/read.ts":    "opencode",
			"packages/opencode/src/plugin/codex.ts": "opencode",
		},
	}
	mut := types.NewMutableState("multi-repo multipath anchor qualification")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "packages/opencode/src/tool/read.ts", Score: 60, ExactEntityRank: 2},
		{Path: "packages/opencode/src/plugin/codex.ts", Score: 58, ExactEntityRank: 2},
	})
	mut.SetSearchGraph(&repotypes.Graph{
		FileIndex: map[string]*repotypes.FileInfo{
			"opencode/packages/opencode/src/tool/read.ts": {
				RelPath: "opencode/packages/opencode/src/tool/read.ts",
				Symbols: []repotypes.Symbol{{
					Name:    "ReadTool",
					File:    "opencode/packages/opencode/src/tool/read.ts",
					Line:    37,
					EndLine: 337,
				}},
			},
			"opencode/packages/opencode/src/plugin/codex.ts": {
				RelPath: "opencode/packages/opencode/src/plugin/codex.ts",
				Symbols: []repotypes.Symbol{{
					Name:    "verifier",
					File:    "opencode/packages/opencode/src/plugin/codex.ts",
					Line:    27,
					EndLine: 27,
				}},
			},
		},
	})
	closure := mut.EvidenceClosure()
	bus := &types.BusContext{
		Mutable:    mut,
		RepoRoot:   t.TempDir(),
		MultiGraph: gater,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "mechanism",
					PrimaryEntities: []string{"ReadTool", "verifier"},
				},
			},
		},
	}

	applyMultiPathAnchorChecks(bus, closure)

	var found int
	for _, p := range closure.PendingReads() {
		if p.Origin != "pre_complete.multi_path_anchor" {
			continue
		}
		found++
		if !strings.HasPrefix(p.File, "opencode/") {
			t.Fatalf("multi_path_anchor PendingRead must use active-set qualified path, got %+v", p)
		}
		if strings.HasPrefix(p.File, "packages/") {
			t.Fatalf("multi_path_anchor leaked bare path that cannot drain against read_file history: %+v", p)
		}
	}
	if found != 2 {
		t.Fatalf("expected two qualified multi_path_anchor pending reads, got %d: %+v", found, closure.PendingReads())
	}
}
