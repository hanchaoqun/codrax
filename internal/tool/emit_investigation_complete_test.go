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
}

// TestEmitInvestigationComplete_CompletionWithoutAbsenceOnEvidenceAccepted
// — the normal happy path: grounded evidence exists, LLM signals
// completion WITHOUT absence_justification. Must succeed.
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
	if !strings.Contains(res.Summary, "Tier-1 proven ratio") {
		t.Errorf("rejection must name the Tier-1 gate: %q", res.Summary)
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
