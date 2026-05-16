package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitInvestigationComplete_AbsenceWithGroundedEvidenceRejected
// pins the 2026-04-17 contradiction gate: when the explorer already
// buffered grounded/recovered evidence, an emit_investigation_complete
// call that carries absence_justification is rejected. This prevents
// the LLM from shortcutting the finalize citation-floor gate by
// tacking "this is an absence answer" onto every completion call.
func TestEmitInvestigationComplete_AbsenceWithGroundedEvidenceRejected(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"absence","absence_justification":"answer is zero"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected rejection when grounded evidence exists and absence claimed; got success")
	}
	if !strings.Contains(res.Summary, "absence_justification") {
		t.Errorf("rejection summary must name the offending field: %q", res.Summary)
	}
	if mut.AbsenceJustification() != "" {
		t.Errorf("absence must NOT be stored on rejection, got %q", mut.AbsenceJustification())
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Errorf("completion flag must NOT fire on rejection")
	}
}

// TestEmitInvestigationComplete_AbsenceWithoutEvidenceAccepted locks
// the legit honest-zero path: when no evidence was emitted, the LLM
// can still declare absence (e.g. "how many .py files?" → 0). The
// hasAnyInvestigationSuccess audit in contract_check still applies
// downstream.
func TestEmitInvestigationComplete_AbsenceWithoutEvidenceAccepted(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"none found","confidence":"high","result_kind":"absence","absence_justification":"no .py files exist"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("honest-zero absence must be accepted: %s", res.Summary)
	}
	if mut.AbsenceJustification() == "" {
		t.Errorf("absence must be stored on acceptance")
	}
}

func TestEmitInvestigationComplete_AcceptsStructuredAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"deterministic count and candidate classification complete",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"total_count",
				"label":"production assignment locations",
				"value":"4",
				"unit":"locations",
				"members":["internal/a.go:10","internal/a.go:20","src/native/foo.cpp:22","src/native/foo.cpp:23"]
			},
			{
				"kind":"unique_count",
				"label":"unique files",
				"value":"2",
				"unit":"files",
				"members":["internal/a.go","src/native/foo.cpp"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("aggregate facts completion should be accepted: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "aggregate_facts: 2") {
		t.Fatalf("summary should report aggregate fact handoff: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 2 || got[0].Kind != types.AnswerAggregateTotalCount || got[1].Value != "2" {
		t.Fatalf("stable aggregate facts not stored: %+v", got)
	}
}

func TestEmitInvestigationComplete_AcceptsStringEncodedAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	encodedFacts, err := json.Marshal([]map[string]any{{
		"kind":    "member_set",
		"label":   "enum type names",
		"value":   "2",
		"members": []string{"Intent", "Scenario"},
	}})
	if err != nil {
		t.Fatalf("marshal facts: %v", err)
	}
	paramsBytes, err := json.Marshal(map[string]any{
		"reason":          "model emitted valid aggregate_facts JSON inside a string field",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": string(encodedFacts),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("string-encoded aggregate facts should be accepted when the string is a complete JSON array: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateMemberSet || got[0].Members[1] != "Scenario" {
		t.Fatalf("string-encoded aggregate facts not retained: %+v", got)
	}
}

func TestEmitInvestigationComplete_RepairsStringEncodedAggregateFactsWithMisplacedResolvedTail(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"reason":      "model emitted aggregate_facts as a JSON string with a misplaced sibling field tail",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": `[
			{"kind":"member_set","label":"read mode stages","value":"2","members":["StageAnalyze","StageExplore"]}
		], "absence_justification": "No write-mode stages execute in read mode."`,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("misplaced aggregate_facts tail should not reject a resolved completion: %s", res.Summary)
	}
	if got := mut.AbsenceJustification(); got != "" {
		t.Fatalf("resolved completion must not promote misplaced absence_justification, got %q", got)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateMemberSet || got[0].Value != "2" {
		t.Fatalf("aggregate facts not recovered from leading array: %+v", got)
	}
}

func TestEmitInvestigationComplete_PromotesMisplacedAbsenceTailForAbsenceResult(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"reason":          "searched and found none",
		"confidence":      "high",
		"result_kind":     "absence",
		"aggregate_facts": `[], "absence_justification": "no .py files exist"`,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("absence completion should promote misplaced absence_justification: %s", res.Summary)
	}
	if got := mut.AbsenceJustification(); got != "no .py files exist" {
		t.Fatalf("absence justification = %q, want promoted misplaced string", got)
	}
}

func TestEmitInvestigationComplete_RepairsStringEncodedAggregateFactsWithDanglingTailCloser(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"reason":      "model emitted aggregate_facts as a JSON string copied from an object member",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": `[
			{"kind":"member_set","label":"core types","value":"2","members":["SubAgentValidator","SubAgentReducer"],}
		], "absence_justification": "not an absence answer"}`,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("dangling object-tail closer and trailing commas should be repaired: %s", res.Summary)
	}
	if got := mut.AbsenceJustification(); got != "" {
		t.Fatalf("resolved completion must not promote misplaced absence_justification, got %q", got)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateMemberSet || len(got[0].Members) != 2 {
		t.Fatalf("aggregate facts not recovered: %+v", got)
	}
}

func TestEmitInvestigationComplete_RejectsInvalidAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{"kind":"guess","label":"x","value":"1"}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("invalid aggregate kind should reject")
	}
	if !strings.Contains(res.Summary, "aggregate_facts[0]") {
		t.Fatalf("rejection should name aggregate fact path: %s", res.Summary)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("rejected aggregate facts must not be retained: %+v", got)
	}
}

func TestEmitInvestigationComplete_RejectsInconsistentAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"total_count",
			"label":"production locations",
			"value":"2",
			"unit":"locations",
			"members":["internal/a.go:10","src/native/foo.cpp:22"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("missing unique_count companion should reject")
	}
	if !strings.Contains(res.Summary, "unique_count") {
		t.Fatalf("rejection should explain missing unique_count companion: %s", res.Summary)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("rejected aggregate facts must not be retained: %+v", got)
	}
}

func TestEmitInvestigationComplete_RejectsDecoratorMemberSetMismatchedToEvidence(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"@Builder"}},
		}},
	}
	seedReadFileHistory(bus, "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", 1,
		"// Source: openharmony @Styles and @Extend decorators",
		"",
		"@Styles function commonCardStyle() {",
		"  .width('100%')",
		"}",
	)
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceRegistration,
		Source:          "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
		LineStart:       3,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "commonCardStyle",
		Object:          "Styles",
		SurfaceTerms:    []string{"@Styles", "commonCardStyle"},
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"member set carried through aggregate_facts",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"@Builder reusable fragments",
			"value":"1",
			"members":["commonCardStyle"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected decorator aggregate mismatch rejection, got success")
	}
	if !strings.Contains(res.Summary, "requested decorator \"@Builder\"") || !strings.Contains(res.Summary, "@Styles") {
		t.Fatalf("rejection should explain aggregate member mismatch, got %q", res.Summary)
	}
}

func TestEmitInvestigationComplete_NormalizesLogSourceDriftReasonToBoundedSurface(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Frames: []types.LogFrame{
				{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				{File: "internal/agent/analyzer.go", Line: 320, Func: "(*analyzerEvaluator).ParseOutput"},
			},
		}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       651,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       861,
			AnchorKind:      types.AnchorCondition,
			AnchorSymbol:    "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"the nil receiver path proves ParseOutput panicked before entering the method body","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("bounded drift completion should still succeed: %s", res.Summary)
	}
	got := mut.InvestigationCompleteReason()
	if strings.Contains(strings.ToLower(got), "nil receiver") {
		t.Fatalf("completion reason should be normalized to bounded drift surface, got %q", got)
	}
	for _, want := range []string{"ParseOutput", "buildAnalysisIR"} {
		if !strings.Contains(got, want) {
			t.Fatalf("completion reason missing %q: %q", want, got)
		}
	}
}

func TestEmitInvestigationComplete_AbsenceRequiresHonestZeroPhrasing(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"all necessary evidence has been collected","confidence":"high","result_kind":"resolved","absence_justification":"already found enough evidence to explain it"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("positive completion text must not be accepted as absence: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "result_kind=absence") {
		t.Errorf("rejection should explain structured absence contract: %s", res.Summary)
	}
	if mut.AbsenceJustification() != "" {
		t.Errorf("absence must NOT be stored on rejection")
	}
}

func TestEmitInvestigationComplete_ConfigTraceAbsenceRejectsGroundedSameScopeContextBeforeValidatedPrecedenceRole(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"absence","absence_justification":"no config key named explore_mid_loop_hint_budget exists"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("config-trace absence closure should keep requiring a precedence-capable anchor before closing")
	}
	if !strings.Contains(res.Summary, "precedence-capable lineage anchor") || !strings.Contains(res.Summary, "diagram_role_hint") {
		t.Fatalf("rejection should explain the missing precedence-role requirement, got %q", res.Summary)
	}
	if res.Repair == nil {
		t.Fatal("missing structured repair on precedence-anchor rejection")
	}
	if got := res.Repair.Code; got != "exact_absence_precedence_anchor" {
		t.Fatalf("repair code = %q, want exact_absence_precedence_anchor", got)
	}
	if len(res.Repair.Targets) != 1 || res.Repair.Targets[0].File != "internal/types/config.go" || res.Repair.Targets[0].Action != string(types.RepairReadFile) {
		t.Fatalf("repair targets = %+v, want read_file on internal/types/config.go", res.Repair.Targets)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[0].Kind != types.RepairReadFile {
		t.Fatalf("closure repairs = %+v, want queued read repair", repairs)
	}
}

func TestEmitInvestigationComplete_ConfigTraceAbsenceAlreadyReadScopeQueuesEmitEvidenceRepair(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.EvidenceClosure().SetReadSet(map[string]bool{"internal/types/config.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"absence","absence_justification":"no config key named explore_mid_loop_hint_budget exists"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("config-trace absence closure should still reject until precedence evidence is materialized")
	}
	if !strings.Contains(res.Summary, "already in read coverage") || !strings.Contains(res.Summary, "Do NOT widen scope") {
		t.Fatalf("rejection should explain that already-read scope needs evidence materialization, got %q", res.Summary)
	}
	if res.Repair == nil {
		t.Fatal("missing structured repair on already-read precedence-anchor rejection")
	}
	if got := res.Repair.Code; got != "exact_absence_precedence_evidence" {
		t.Fatalf("repair code = %q, want exact_absence_precedence_evidence", got)
	}
	if len(res.Repair.Targets) != 1 || res.Repair.Targets[0].File != "internal/types/config.go" || res.Repair.Targets[0].Action != string(types.RepairEmitEvidence) {
		t.Fatalf("repair targets = %+v, want emit_evidence on internal/types/config.go", res.Repair.Targets)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[0].Kind != types.RepairEmitEvidence {
		t.Fatalf("closure repairs = %+v, want queued emit_evidence repair", repairs)
	}
}

// TestEmitInvestigationComplete_CompletionWithoutAbsenceOnEvidenceAccepted
// — the normal happy path: grounded evidence exists, LLM signals
// completion WITHOUT absence_justification. Must succeed.
func TestEmitInvestigationComplete_ConfigTraceAbsencePrefersMaterializeOverUnreadSibling(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go", "internal/types/context.go"})
	mut.EvidenceClosure().SetReadSet(map[string]bool{"internal/types/config.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"absence","absence_justification":"no config key named explore_mid_loop_hint_budget exists"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("config-trace absence closure should still reject until precedence evidence is materialized")
	}
	if !strings.Contains(res.Summary, "already in read coverage") || !strings.Contains(res.Summary, "Do NOT widen scope") {
		t.Fatalf("rejection should keep the repair on already-read scope before unread siblings, got %q", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "exact_absence_precedence_evidence" {
		t.Fatalf("repair = %+v, want exact_absence_precedence_evidence", res.Repair)
	}
	if len(res.Repair.Targets) != 1 || res.Repair.Targets[0].File != "internal/types/config.go" || res.Repair.Targets[0].Action != string(types.RepairEmitEvidence) {
		t.Fatalf("repair targets = %+v, want emit_evidence on internal/types/config.go", res.Repair.Targets)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[0].Kind != types.RepairEmitEvidence || len(repairs[0].Files) != 1 || repairs[0].Files[0] != "internal/types/config.go" {
		t.Fatalf("closure repairs = %+v, want queued emit_evidence on internal/types/config.go", repairs)
	}
}

func TestEmitInvestigationComplete_ConfigTraceContextOnlyEvidenceRequiresValidatedPrecedenceRole(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       32,
			Subject:         "RuntimeSettings",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("config-trace context-only evidence should not close absence until one same-scope anchor carries a validated precedence role")
	}
	if !strings.Contains(res.Summary, "precedence-capable lineage anchor") || !strings.Contains(res.Summary, "diagram_role_hint") {
		t.Fatalf("rejection should explain the validated-precedence requirement, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "exact_absence_precedence_anchor" {
		t.Fatalf("repair = %+v, want exact_absence_precedence_anchor", res.Repair)
	}
	if len(res.Repair.Targets) != 1 || res.Repair.Targets[0].File != "internal/types/config.go" {
		t.Fatalf("repair targets = %+v, want internal/types/config.go", res.Repair.Targets)
	}
}

func TestEmitInvestigationComplete_CompletionWithoutAbsenceOnEvidenceAccepted(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"evidence collected","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("normal completion must succeed: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_CallChainRejectsLatePrincipalSpanGap(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/analyzer.go", LineStart: 100,
			Subject: "buildAnalysisIR", AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 160,
			Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 300,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run", "analyzer.go"},
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"source and sink are covered","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("pre-complete downgrades should be tool-success keepalives, got failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "call-chain principal span") || !strings.Contains(res.Summary, "250-299") {
		t.Fatalf("downgrade should name the missing late span, got %q", res.Summary)
	}
	if got := strings.TrimSpace(mut.InvestigationCompleteReason()); got != "" {
		t.Fatalf("completion flag must not be set on span downgrade, got %q", got)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairReadFile {
		t.Fatalf("repair = %+v, want one read_file repair", repairs)
	}
	if len(repairs[0].LineRanges) != 1 || repairs[0].LineRanges[0] != (types.LineRange{Start: 250, End: 299}) {
		t.Fatalf("repair range = %+v, want 250-299", repairs[0].LineRanges)
	}
}

func TestEmitInvestigationComplete_CallChainAlreadyReadGapAsksForEvidenceMaterialization(t *testing.T) {
	mut := types.NewMutableState("q")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/analyzer.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/agent/analyzer.go": {{Start: 1, End: 320}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/analyzer.go", LineStart: 100,
			Subject: "buildAnalysisIR", AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 160,
			Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 300,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run"},
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"source and sink are covered","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Summary, "already in read coverage") {
		t.Fatalf("downgrade should keep the explorer on already-read evidence materialization, got %q", res.Summary)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairEmitEvidence {
		t.Fatalf("repair = %+v, want one emit_evidence repair", repairs)
	}
}

func TestEmitInvestigationComplete_CallChainAcceptsLatePrincipalEvidence(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/analyzer.go", LineStart: 100,
			Subject: "buildAnalysisIR", AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 160,
			Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 260,
			Subject: "buildAnalysisIR", Object: "compiler.Compile", AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.Compile",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 300,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run", "analyzer.go"},
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"source, interior, and sink are covered","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("completion with late principal evidence should succeed: %s", res.Summary)
	}
	if got := strings.TrimSpace(mut.InvestigationCompleteReason()); got == "" {
		t.Fatalf("completion flag should be set")
	}
}

func TestEmitInvestigationComplete_CallChainRejectsLargeTailGapAfterLateEvidence(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/analyzer.go", LineStart: 100,
			Subject: "buildAnalysisIR", AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 180,
			Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 530,
			Subject: "buildAnalysisIR", Object: "compiler.Compile", AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.Compile",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 660,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run"},
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"source, one late interior, and sink are covered","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("pre-complete downgrades should be tool-success keepalives, got failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "531-659") {
		t.Fatalf("downgrade should name the large late tail gap, got %q", res.Summary)
	}
	if got := strings.TrimSpace(mut.InvestigationCompleteReason()); got != "" {
		t.Fatalf("completion flag must not be set on tail-gap downgrade, got %q", got)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairReadFile {
		t.Fatalf("repair = %+v, want one read_file repair", repairs)
	}
	if len(repairs[0].LineRanges) != 1 || repairs[0].LineRanges[0] != (types.LineRange{Start: 531, End: 659}) {
		t.Fatalf("repair range = %+v, want 531-659", repairs[0].LineRanges)
	}
}

func TestEmitInvestigationComplete_NormalizesResolvedToAbsenceForExactConfigTraceClosure(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       32,
			Subject:         "RuntimeSettings",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       815,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Code defaults for the surviving explore heuristics fields.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key explore_mid_loop_hint_budget in any layer","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected structured exact-absence closure to auto-normalize, got: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
	if got := mut.StableAbsenceJustification(); got == "" {
		t.Fatal("StableAbsenceJustification = empty, want synthesized justification")
	}
	if !strings.Contains(res.Summary, "result_kind=absence") {
		t.Fatalf("summary should reflect normalized absence result, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_SynthesizesAbsenceForRelatedContextOnlyExactClosure(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       815,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Code defaults for the surviving explore heuristics fields.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
					ExactTargets:      []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key explore_mid_loop_hint_budget in any layer","confidence":"high","result_kind":"absence"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected exact-absence closure with validated related context to auto-synthesize justification, got: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
	if got := mut.StableAbsenceJustification(); got == "" {
		t.Fatal("StableAbsenceJustification = empty, want synthesized justification")
	}
	if !strings.Contains(res.Summary, "result_kind=absence") {
		t.Fatalf("summary should reflect synthesized absence result, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_AbsenceAllowsProductionNegativeSummaryMentions(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       839,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "DefaultExploreHeuristics defines the surviving defaults, and there is no explore_mid_loop_hint_budget field here.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
					ExactTargets:      []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key explore_mid_loop_hint_budget in any layer","confidence":"high","result_kind":"absence","absence_justification":"the exact config key explore_mid_loop_hint_budget is absent from the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("negative summary mentions should not be upgraded into defining exact proof, got: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
}

func TestEmitInvestigationComplete_PreservesPriorAbsenceOverResolvedRetryWithoutNewProof(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetAbsenceJustification("the exact config key explore_mid_loop_hint_budget is absent from the repo")
	mut.SetInvestigationResultKind("absence")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       839,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "DefaultExploreHeuristics defines the surviving defaults, and there is no explore_mid_loop_hint_budget field here.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
					ExactTargets:      []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"the three-layer precedence chain is understood and the key is still absent","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("resolved retry without new exact proof should preserve prior absence closure, got: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
	if !strings.Contains(res.Summary, "result_kind=absence") {
		t.Fatalf("summary should reflect preserved absence result, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_RejectsResolvedExactClosureWithoutDefiningProof(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go", "cmd/root.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			Subject:         "DefaultExploreHeuristics",
			Object:          "16 hard-coded defaults; " + missingKey + " is not one of them",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       768,
			Subject:         "ExploreHeuristics",
			Object:          missingKey,
			Predicate:       "absent_from",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
					ExactTargets:      []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"the general precedence mechanism is understood and the key still does not exist","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("resolved completion without defining proof or stable absence closure should be rejected, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "still has no grounded defining proof") || !strings.Contains(res.Summary, "result_kind=\"absence\"") {
		t.Fatalf("reject should steer either toward real defining proof or exact absence closure, got: %s", res.Summary)
	}
}

// TestEmitInvestigationComplete_Tier1FloorRejectsPureRecovery pins the
// session-8 upstream-intercept: when every item is Recovered (the LLM
// never read_file'd any of the cited sources), the Tier-1 floor fires
// and rejects the completion claim. Rejection message names the
// recovered-only items and tells the LLM to call read_file.
// Matches the trace 1776444788929246456 failure mode where the
// finalizer dropped all 4 citations because none were read-file proven.
func TestEmitInvestigationComplete_Tier1FloorRejectsPureRecovery(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	closure := mut.EvidenceClosure()
	// 3 recovered items — grounded+recovered ratio = 100% (passes
	// GroundingFloor) but Tier-1 ratio = 0% (fails Tier1Floor).
	for i := 0; i < 3; i++ {
		mut.AppendEvidence([]types.EvidenceItem{{
			Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10 + i,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
			GroundingStatus: types.GroundingRecovered,
			GroundingTier:   types.TierFQNameSameFile,
		}})
	}
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatalf("pure-recovery investigation must be rejected; got success=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "line-text-grounded ratio") {
		t.Errorf("rejection must name the line-text grounding gate: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "read_file") {
		t.Errorf("rejection must suggest read_file repair: %q", res.Summary)
	}
	if repairs := closure.PendingRepairs(); len(repairs) != 1 {
		t.Fatalf("expected one deduped RepairReadFile directive, got %d: %+v", len(repairs), repairs)
	} else if repairs[0].Kind != types.RepairReadFile || len(repairs[0].Files) != 1 || repairs[0].Files[0] != "a.go" {
		t.Fatalf("unexpected repair payload: %+v", repairs[0])
	}
	if pending := closure.PendingReads(); len(pending) != 1 {
		t.Fatalf("expected one mirrored PendingRead, got %d: %+v", len(pending), pending)
	} else if pending[0].File != "a.go" {
		t.Fatalf("pending read file = %q, want a.go", pending[0].File)
	}
	if !strings.Contains(res.Summary, "10, 11, 12") {
		t.Errorf("summary should collapse same-file line hints, got: %q", res.Summary)
	}
}

// TestEmitInvestigationComplete_Tier1FloorAcceptsMixed — 30% Tier-1
// threshold met by 1 Tier-1 + 2 Recovered items (1/3 = 33%). Gate
// passes, completion accepted.
func TestEmitInvestigationComplete_Tier1FloorAcceptsMixed(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceDirect, Source: "b.go", LineStart: 20,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Bar",
			GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierFQNameSameFile,
		},
		{
			Kind: types.EvidenceDirect, Source: "c.go", LineStart: 30,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Baz",
			GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierPackageSymbol,
		},
	})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("33%% Tier-1 ratio must pass 30%% floor, got rejection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DowngradesWhenPrincipalSupportLaneMissing(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("Which agents can call subagents?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "context-guard",
		Kind:            types.EvidenceConditional,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/subagent_runtime.go",
		LineStart:       42,
		AnchorKind:      types.AnchorCondition,
		AnchorSymbol:    "Validate",
		Subject:         "SubAgentRuntime",
		Summary:         "SubAgentRuntime validates proposals before execution",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: enumerationPrincipalGateIR(),
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"found enough context","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("principal support downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "typed principal evidence handoff is missing") {
		t.Fatalf("summary should explain missing principal handoff, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Kind != types.RepairEmitEvidence {
		t.Fatalf("expected RepairEmitEvidence queued, got %+v", repairs)
	}
}

func TestEmitInvestigationComplete_AllowsFacetBoundPrincipalSupport(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("Which agents can call subagents?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "agent-explorer",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentExplorer",
		Subject:         "AgentExplorer",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: enumerationPrincipalGateIR(),
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"principal member is grounded","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("facet-bound principal support should pass: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after principal support passes")
	}
}

func TestEmitInvestigationComplete_DowngradesExhaustiveEnumerationWithoutMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"the command output listed all members","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("exhaustive member-set downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("summary should ask for structured member_set handoff, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_DowngradesDeclaredEnumerationWithoutMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List the three public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints = types.AnalyzerHints{
		Kind:     string(types.ReqEnumeration),
		Entities: []string{"Intent", "Scenario", "Complexity"},
	}
	ir.RequestModel.EnumerationBoundary = &types.RequestedEnumerationBoundary{
		DeclaredCount: 3,
		SourceQuote:   "three public enum types",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"the command output listed all members","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("exhaustive member-set downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("summary should ask for structured member_set handoff, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_AllowsExhaustiveEnumerationMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set carried through aggregate_facts",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types",
			"value":"1",
			"unit":"types",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("member_set aggregate should satisfy exhaustive handoff: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after member_set aggregate passes")
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || len(facts[0].Members) != 1 {
		t.Fatalf("member_set aggregate should be retained, got %+v", facts)
	}
}

func TestMergeCompletionAggregateFacts_DedupesEquivalentMemberSetRetries(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "all subpackages and entry functions",
		Value:   "2",
		Members: []string{"aggregator → Aggregate", "compiler → Compile"},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "subpackage directory and entry function",
		Value:   "2",
		Members: []string{"aggregator/Aggregate", "compiler/Compile"},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 1 {
		t.Fatalf("equivalent retry member_set facts should merge to one authoritative set, got %+v", got)
	}
	if got[0].Label != "all subpackages and entry functions" {
		t.Fatalf("current retry aggregate should win over retained display metadata, got %+v", got[0])
	}
}

func TestMergeCompletionAggregateFacts_RetainsStableSupersetMemberSet(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "2",
		Unit:  "rows",
		Members: []string{
			"aggregator: Aggregate (aggregator.go:132)",
			"compiler: Compile (compile.go:37)",
		},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "3",
		Unit:  "rows",
		Members: []string{
			"aggregator -> Aggregate (line 132)",
			"compiler -> Compile (line 37)",
			"subject -> Score (line 40)",
		},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 1 {
		t.Fatalf("same labeled member_set facts should collapse to the fullest typed set, got %+v", got)
	}
	if got[0].Value != "3" || len(got[0].Members) != 3 {
		t.Fatalf("stable superset should survive a later narrowed retry, got %+v", got[0])
	}
	if !strings.Contains(strings.Join(got[0].Members, "\n"), "subject") {
		t.Fatalf("stable-only member should be retained, got %+v", got[0].Members)
	}
}

func TestMergeCompletionAggregateFacts_CurrentSupersetWinsStableSubset(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "3",
		Unit:  "rows",
		Members: []string{
			"aggregator: Aggregate (aggregator.go:132)",
			"compiler: Compile (compile.go:37)",
			"subject: Score (taxonomy.go:40)",
		},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "2",
		Unit:  "rows",
		Members: []string{
			"aggregator -> Aggregate (line 132)",
			"compiler -> Compile (line 37)",
		},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 1 {
		t.Fatalf("same labeled member_set facts should collapse when current is the typed superset, got %+v", got)
	}
	if got[0].Value != "3" || len(got[0].Members) != 3 {
		t.Fatalf("current superset should win over retained narrower set, got %+v", got[0])
	}
}

func TestMergeCompletionAggregateFacts_KeepsDisjointSameLabelMemberSetsSeparate(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public API",
		Value:   "1",
		Members: []string{"functions: Eval"},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public API",
		Value:   "1",
		Members: []string{"types: Kind"},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 2 {
		t.Fatalf("disjoint same-label member sets should not be collapsed as a false superset, got %+v", got)
	}
}

func TestEmitInvestigationComplete_RejectsUnsupportedExhaustiveEnumerationMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set contains one grounded member and one nearby helper",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types",
			"value":"2",
			"unit":"types",
			"members":["Intent","NearbyHelper"],
			"support_refs":["Intent @ internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unsupported member-set downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "member_set") ||
		!strings.Contains(res.Summary, "NearbyHelper") ||
		!strings.Contains(res.Summary, "typed evidence") {
		t.Fatalf("summary should point at unsupported principal members, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_MemberSetNarrowsAnalyzerEntityCandidates(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types with const sets")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints.Entities = []string{"Intent", "CandidateWithoutConstSet"}
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types with const sets",
	}
	ir.AnswerContract.MustIncludeTerms = []types.ContractTerm{
		{Text: "Intent", Kind: types.ContractTermSymbol, Source: types.ContractTermSourceAnalyzerEntity},
		{Text: "CandidateWithoutConstSet", Kind: types.ContractTermSymbol, Source: types.ContractTermSourceAnalyzerEntity},
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set contains the verified filtered members",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types with const sets",
			"value":"1",
			"unit":"types",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("verified member_set should supersede analyzer-entity candidates: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "required principal members lack typed handoff") {
		t.Fatalf("analyzer-entity candidates absent from member_set must not block completion: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after analyzer candidates are narrowed by member_set")
	}
}

func TestEmitInvestigationComplete_MemberSetDoesNotWaiveExplicitRequiredTerms(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all agents")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "agent-explorer",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentExplorer",
		Subject:         "AgentExplorer",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints.Entities = []string{"AgentExplorer", "AgentFinalizer"}
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all agents",
	}
	ir.AnswerContract.MustIncludeTerms = []types.ContractTerm{
		{Text: "AgentExplorer", Kind: types.ContractTermSymbol},
		{Text: "AgentFinalizer", Kind: types.ContractTermSymbol},
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set carried one member only",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"agents",
			"value":"1",
			"unit":"agents",
			"members":["AgentExplorer"],
			"support_refs":["internal/types/enums.go:117"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("required-term handoff downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "required principal members lack typed handoff") ||
		!strings.Contains(res.Summary, "AgentFinalizer") {
		t.Fatalf("explicit required terms must still block when absent from member_set, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_CanonicalizesMemberSetValue(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set carried through aggregate_facts",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types",
			"unit":"types",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("member_set value should be canonicalized from members: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Value != "1" {
		t.Fatalf("member_set value not canonicalized: %+v", facts)
	}
}

func TestEmitInvestigationComplete_DowngradesWhenRequiredEnumerationTermsLackTypedHandoff(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all agents")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "agent-explorer",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentExplorer",
		Subject:         "AgentExplorer",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints.Entities = []string{"AgentExplorer", "AgentFinalizer"}
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all agents",
	}
	ir.AnswerContract.MustIncludeTerms = []types.ContractTerm{
		{Text: "AgentExplorer", Kind: types.ContractTermSymbol},
		{Text: "AgentFinalizer", Kind: types.ContractTermSymbol},
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"only one member grounded","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("required-term handoff downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "required principal members lack typed handoff") ||
		!strings.Contains(res.Summary, "AgentFinalizer") {
		t.Fatalf("summary should name missing typed handoff member, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Origin != "pre_complete.principal_required_term_handoff" {
		t.Fatalf("expected principal required-term handoff repair, got %+v", repairs)
	}
}

func enumerationPrincipalGateIR() *types.AnalysisIR {
	return &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:     types.IntentEnumerate,
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		},
		AnswerContract: types.AnswerContract{
			CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
		},
	}
}

// TestEmitInvestigationComplete_Tier1FloorDisabledWhenZero — floor=0
// preserves session-7 backward-compat behaviour (no Tier-1 gate).
func TestEmitInvestigationComplete_Tier1FloorDisabledWhenZero(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
		GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierFQNameSameFile,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Errorf("Tier1Floor=0 must disable the gate; got rejection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_Tier1FloorQueuesTier2GroundedReads(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	closure := mut.EvidenceClosure()
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceMechanism, Source: "internal/tool/repomap/tool.go", LineStart: 133,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildOrLoadGraph",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierFQNameSameFile,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if res.Success {
		t.Fatalf("tier2-only investigation must be rejected; got success=%q", res.Summary)
	}
	if repairs := closure.PendingRepairs(); len(repairs) != 1 {
		t.Fatalf("expected one RepairReadFile for tier2 grounded item, got %d: %+v", len(repairs), repairs)
	} else if repairs[0].Files[0] != "internal/tool/repomap/tool.go" {
		t.Fatalf("repair file = %q, want internal/tool/repomap/tool.go", repairs[0].Files[0])
	}
}

func TestEmitInvestigationComplete_Tier1FloorIgnoresAuxiliaryContextOnlyItems(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceUnresolved,
			Source:          "docs/example.go",
			LineStart:       10,
			ContextRole:     types.EvidenceContextRoleIllustrativeOnly,
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       214,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "explore_mid_loop_hint_budget",
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("auxiliary illustrative/absence-support items should not force a tier1-floor retry, got rejection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_Tier1FloorIgnoresRelatedContextForScalarRoleLocate(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.6})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       883,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "buildAnalysisIR",
			ContextRole:     types.EvidenceContextRoleDefining,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       25,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "AnalysisIR",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingRecovered,
			GroundingTier:   types.TierFQNameSameFile,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				Complexity:    types.ComplexitySimple,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:     true,
					IsRoleLocateLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "mechanism",
					PrimaryEntities: []string{"AnalysisIR"},
					Entities:        []string{"AnalysisIR"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"the defining function line is grounded and the nearby type name is only explanatory context","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("scalar role-locate related_context should not dilute the tier1 floor, got rejection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsContextualEvidence(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "cmd/root.go", LineStart: 889,
		Subject: "explore_midloop_min_iteration", AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + `; related budget keys are only context","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("config-key absence with contextual related evidence should be accepted: %s", res.Summary)
	}
	if mut.AbsenceJustification() == "" {
		t.Errorf("absence must be stored on acceptance")
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsTestOnlyExactMentions(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "cmd/root.go", LineStart: 889,
			Subject: "explore_midloop_min_iteration", AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceDirect, Source: "internal/tool/emit_answer_document_test.go", LineStart: 813,
			Subject: missingKey, AnchorKind: types.AnchorDefinition, AnchorSymbol: "hint_budget",
			Snippet:         "no config key named `" + missingKey + "` exists in the repo",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 鍦ㄥ摢閲屽畾涔夛紵",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + `; related explore defaults are only context","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("test-only exact mentions should not block absence closure: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should be marked complete")
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsAbsenceSupportProductionMentions(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    missingKey,
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			LineEnd:         724,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Related context only: nearby explore defaults do not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终值怎么计算？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("absence-support production mentions should still allow exact absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceIgnoresNegativeProbeDefiningHints(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			Subject:         "RuntimeSettings",
			Predicate:       "does not bind",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not bind the missing exact config key.",
			ContextRole:     types.EvidenceContextRoleDefining,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "does not provide",
			Object:          "mid_loop_hint_budget",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Same-family defaults code does not provide a mid_loop_hint_budget field.",
			ContextRole:     types.EvidenceContextRoleDefining,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值怎么计算？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("negative exact-target probes should not block honest absence closure even if their context_role is stale/defining: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsRelatedContextThatKeepsTargetInSubjectOnly(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/explore_budget.go",
			LineStart:       40,
			LineEnd:         48,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreBudget",
			Object:          "internal/types/explore_budget.go",
			Summary:         "Nearby runtime budget struct does not define the missing exact config key.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 鐨勬渶缁堝€兼€庝箞璁＄畻锛?",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("related-context evidence that keeps the exact target only in subject text must not block exact absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsClosureReadyMixedContextRoles(t *testing.T) {
	missingKey := "zz_absent_config_mixed_roles"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{
		"internal/types/config.go",
		"codrax.yaml.example",
		"cmd/root.go",
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			LineEnd:         866,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "does not provide",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "DefaultExploreHeuristics does not provide the missing exact config key.",
			ContextRole:     types.EvidenceContextRoleDefining,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "codrax.yaml.example",
			LineStart:       410,
			LineEnd:         410,
			Subject:         "explore_midloop_min_iteration",
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "explore_midloop_min_iteration",
			Summary:         "Config file layer lists supported explore_* keys, but not the missing exact key.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleConfig,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "cmd/root.go",
			LineStart:       1618,
			LineEnd:         1620,
			Subject:         "ExploreMidLoopMinIteration",
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			Summary:         "Operator override layer only merges supported Explore_* fields, not the missing exact key.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleOverride,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值怎么计算？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in any supported precedence layer","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("closure-ready mixed context roles should still allow exact absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsUngroundedRequiredContext(t *testing.T) {
	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("absence closure should be rejected until a grounded required related-context anchor exists")
	}
	if !strings.Contains(res.Summary, "precedence-capable lineage anchor") || !strings.Contains(res.Summary, "diagram_role_hint") || !strings.Contains(res.Summary, "internal/types/config.go") {
		t.Fatalf("rejection should name the missing related-context requirement, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsGroundedRequiredContext(t *testing.T) {
	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			LineEnd:         724,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("grounded required related-context anchor should allow absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsNearbyGroundedContextThatMentionsMissingKeyInObject(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go", "codrax.yaml.example", "cmd/root.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       296,
			Subject:         "RuntimeSettings",
			Object:          missingKey,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			Subject:         "DefaultExploreHeuristics",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "DefaultExploreHeuristics defines nearby explore defaults but not " + missingKey,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       296,
			Subject:         "ExploreMidLoopMinIteration",
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			Summary:         "Runtime binding for nearby explore defaults.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleRuntime,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("nearby grounded context that names the missing key only in explanatory object text should still allow absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsGroundedRequiredContextWithoutDiagramRole(t *testing.T) {
	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			LineEnd:         724,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("config-trace absence closure should keep requiring a precedence-capable lineage anchor when the nearby context lacks a validated diagram role")
	}
	if !strings.Contains(res.Summary, "precedence-capable lineage anchor") || !strings.Contains(res.Summary, "diagram_role_hint") {
		t.Fatalf("rejection should explain the missing precedence anchor requirement, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsRecoveredRequiredContext(t *testing.T) {
	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			LineEnd:         724,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingRecovered,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatal("recovered related-context evidence should not satisfy config-trace absence closure")
	}
	if !strings.Contains(res.Summary, "grounded precedence-capable lineage anchor") && !strings.Contains(res.Summary, "precedence-capable lineage anchor") {
		t.Fatalf("rejection should preserve the missing-grounded-anchor guidance, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceIgnoresAuxiliaryRecoveredContextInTier1Floor(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       296,
			LineEnd:         310,
			Subject:         missingKey,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			LineEnd:         865,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       768,
			LineEnd:         784,
			Subject:         "ExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreHeuristics",
			Summary:         "Nearby same-family struct context.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingRecovered,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("auxiliary recovered nearby-context items should not force a tier1-floor retry once exact-absence closure already has a grounded same-scope anchor: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceBypassesGroundingFloorsForContextOnlyEvidence(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "cmd/root.go", LineStart: 889,
			Subject: "explore_midloop_min_iteration", AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "internal/config/runtime.go",
			Subject: "AgentLoopMaxMidLoopInjects", AnchorKind: types.AnchorDefinition, AnchorSymbol: "AgentLoopMaxMidLoopInjects",
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "cmd/root.go",
			Subject: "zz_absent_context_knob", AnchorKind: types.AnchorAssignment, AnchorSymbol: "zz_absent_context_knob",
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "internal/config/runtime.go",
			Subject: "AgentLoopMaxDocBytes", AnchorKind: types.AnchorDefinition, AnchorSymbol: "AgentLoopMaxDocBytes",
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "codrax.yaml",
			Subject: "zz_absent_config_knob_context", AnchorKind: types.AnchorAssignment, AnchorSymbol: "zz_absent_config_knob_context",
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "internal/agent/explorer.go",
			Subject: "context budget knob", AnchorKind: types.AnchorDefinition, AnchorSymbol: "contextBudgetKnob",
			GroundingStatus: types.GroundingUngrounded,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
		StageReports: []types.StageReport{{
			Stage:    types.StageAnalyze,
			Agent:    types.AgentAnalyzer,
			Findings: "The key ~~`" + missingKey + "`~~ [unverified: symbol not in repo graph] was not found exactly.",
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"found no exact config key ` + missingKey + `; related budget keys are context only","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("honest config-key absence should bypass generic grounding floors: %s", res.Summary)
	}
	if mut.AbsenceJustification() == "" {
		t.Fatalf("absence justification must be stored")
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should be marked complete")
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsExactEvidence(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "codrax.yaml", LineStart: 12,
		Subject: missingKey, AnchorKind: types.AnchorAssignment, AnchorSymbol: missingKey,
		Snippet:         missingKey + ": 2",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"no exact config key ` + missingKey + ` exists","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("absence must still be rejected when exact config-key evidence exists")
	}
	if mut.AbsenceJustification() != "" {
		t.Errorf("absence must NOT be stored on rejection")
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsPositiveSubstituteFromPriorReport(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "internal/config/runtime.go", LineStart: 184,
		Subject: "AgentLoopMaxMidLoopInjects", AnchorKind: types.AnchorDefinition, AnchorSymbol: "AgentLoopMaxMidLoopInjects",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
		StageReports: []types.StageReport{{
			Stage:    types.StageAnalyze,
			Agent:    types.AgentAnalyzer,
			Findings: "The key ~~`" + missingKey + "`~~ [unverified: symbol not in repo graph] was not found exactly.",
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"positive chain is fully traced through AgentLoopMaxMidLoopInjects","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("positive substitute completion must be rejected")
	}
	if !strings.Contains(res.Summary, "primary exact config key") {
		t.Fatalf("rejection should explain exact-key guard: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("completion flag must not fire on positive substitute rejection")
	}
}

// TestEmitInvestigationComplete_RejectsDecoratedMemberSetWithoutSupportRefs
// is the B regression for the 2026-05-16 architectural-comparison
// loop. A member_set whose members are shaped
// "<code identifier> (<CJK qualifier>)" cannot auto-resolve against
// evidence anchors (the decorator changes the surface), so the
// finalizer's row-item alignment oracle rejects every row that
// quotes such a member with candidate_citations=[]. The schema marks
// support_refs `omitempty` but the downstream answer-document
// rendering REQUIRES per-member grounding; close the asymmetry at
// emit time so the model fixes it in the next emit, not after
// burning four finalize iterations.
func TestEmitInvestigationComplete_RejectsDecoratedMemberSetWithoutSupportRefs(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"comparison investigation complete",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"codrax 防幻觉组件",
				"value":"3",
				"members":[
					"Gate.Run (8个独立检查)",
					"Ground (3层验证)",
					"Orchestrator (4阶段管道整合)"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("decorated member_set without support_refs must be rejected; got success: %s", res.Summary)
	}
	for _, want := range []string{
		"member_set",
		"support_refs is empty",
		"Gate.Run",
		"<file>:<line>",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("reject summary missing %q in:\n%s", want, res.Summary)
		}
	}
}

// TestEmitInvestigationComplete_AcceptsBareCodeMemberSetWithoutSupportRefs
// is the negative-control: bare code-identity members (no decorator)
// can still be auto-grounded by the enrichment pass against verbatim
// evidence anchors. The narrowed B check fires only on decorated
// members so callers and tests with bare-symbol member_sets keep
// working without source-level changes.
func TestEmitInvestigationComplete_AcceptsBareCodeMemberSetWithoutSupportRefs(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"plain symbol enum collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"read mode stages",
				"value":"2",
				"members":["StageAnalyze","StageExplore"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("bare-symbol member_set without support_refs should pass through B: %s", res.Summary)
	}
}

// TestEmitInvestigationComplete_AcceptsDecoratedMemberSetWithSupportRefs
// is the happy path: when the model attaches support_refs alongside
// decorated members, the B gate is satisfied and the emit proceeds.
func TestEmitInvestigationComplete_AcceptsDecoratedMemberSetWithSupportRefs(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"comparison with explicit per-member grounding",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"codrax 防幻觉组件",
				"value":"2",
				"members":[
					"Gate.Run (8个独立检查)",
					"Orchestrator (4阶段管道整合)"
				],
				"support_refs":[
					"Gate.Run: internal/analysis/gate/gate.go:128",
					"Orchestrator: internal/orchestrator/orchestrator.go:42"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("decorated member_set WITH support_refs should pass: %s", res.Summary)
	}
}
