package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestCountTier1Evidence covers the tier1/total classification
// independently of the orchestrator wiring.
func TestCountTier1Evidence(t *testing.T) {
	items := []types.EvidenceItem{
		{GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
		{GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierSymbolTable},
		{GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierFQNameSameFile},
		{GroundingStatus: types.GroundingUngrounded},
		{}, // legacy empty-status → counts as Tier-1
	}
	tier1, total := countTier1Evidence(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	// Tier-1 (line_text): 1
	// Tier-2 grounded (symbol_table): 0 (NOT tier 1)
	// Recovered / Ungrounded: 0
	// Legacy empty: 1
	if tier1 != 2 {
		t.Errorf("tier1 = %d, want 2", tier1)
	}
}

// TestCountTier1Evidence_AllRecovered — pure-recovery investigation
// (the log 172408 scenario): zero Tier-1 against total evidence.
func TestCountTier1Evidence_AllRecovered(t *testing.T) {
	items := []types.EvidenceItem{
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
		{GroundingStatus: types.GroundingRecovered},
	}
	tier1, total := countTier1Evidence(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if tier1 != 0 {
		t.Errorf("tier1 = %d, want 0", tier1)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
}

func TestCountTier1Evidence_SkipsAuxiliaryExactResolutionContext(t *testing.T) {
	items := []types.EvidenceItem{
		{
			GroundingStatus: types.GroundingUngrounded,
			ContextRole:     types.EvidenceContextRoleIllustrativeOnly,
			Kind:            types.EvidenceUnresolved,
		},
		{
			GroundingStatus: types.GroundingUngrounded,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			Kind:            types.EvidenceDirect,
		},
		{
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			Kind:            types.EvidenceDirect,
		},
	}
	tier1, total := countTier1Evidence(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if total != 1 {
		t.Fatalf("auxiliary illustrative/absence-support items should be excluded from tier1 denominator, got total=%d", total)
	}
	if tier1 != 1 {
		t.Fatalf("remaining grounded related-context item should count, got tier1=%d", tier1)
	}
}

func TestCountTier1Evidence_ExactAbsenceSkipsNonClosureRelatedContext(t *testing.T) {
	contract := &types.ExactResolutionContract{
		AllowAbsence:         true,
		TargetKind:           types.SubjectConfigKey,
		Targets:              []string{"explore_mid_loop_hint_budget"},
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
	}
	items := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingRecovered,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       296,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	}
	tier1, total := countTier1Evidence(items, contract, types.ScenarioConfigTrace, true, []string{"internal/types/config.go"}, types.RequestModel{})
	if total != 0 || tier1 != 0 {
		t.Fatalf("non-closure related_context items should not count during exact-absence closure, got tier1=%d total=%d", tier1, total)
	}
}

func TestCountGroundingHealth(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText},
		{Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierSymbolTable},
		{Kind: types.EvidenceDirect, GroundingStatus: types.GroundingRecovered},
		{Kind: types.EvidenceDirect, GroundingStatus: types.GroundingUngrounded},
		{Kind: types.EvidenceDirect},
	}
	health := countGroundingHealth(items, nil, types.ScenarioGeneric, false, nil, types.RequestModel{})
	if health.total != 5 {
		t.Fatalf("total = %d, want 5", health.total)
	}
	if health.accepted != 4 {
		t.Fatalf("accepted = %d, want 4", health.accepted)
	}
	if health.tier1 != 2 {
		t.Fatalf("tier1 = %d, want 2", health.tier1)
	}
	if health.recovered != 1 {
		t.Fatalf("recovered = %d, want 1", health.recovered)
	}
}

func TestWarnLowGroundingIfNeededEmitsAdvisoryNotice(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("explain weak evidence")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "a.go",
		LineStart:       1,
		GroundingStatus: types.GroundingUngrounded,
	}})
	var events []render.Event
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:  mu,
			Language: "en",
		},
		emit: func(ev render.Event) {
			events = append(events, ev)
		},
	}
	warned := false
	o.warnLowGroundingIfNeeded(&types.AnalysisIR{
		RequestModel: types.RequestModel{Scenario: types.ScenarioGeneric},
	}, &warned)

	if !warned {
		t.Fatal("warning flag was not set")
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Kind != render.EventOrchestratorNotice || events[0].NoticeKind != render.NoticeLowGrounding {
		t.Fatalf("unexpected event: %+v", events[0])
	}
	if !strings.Contains(events[0].Reasoning, "Evidence grounding is weak") {
		t.Fatalf("warning message missing user-facing grounding cue: %q", events[0].Reasoning)
	}

	o.warnLowGroundingIfNeeded(&types.AnalysisIR{
		RequestModel: types.RequestModel{Scenario: types.ScenarioGeneric},
	}, &warned)
	if len(events) != 1 {
		t.Fatalf("warning should emit once, events=%d", len(events))
	}
}

// TestCheckTier1Floor_RejectMessageR6 — the reject message flows
// through pendingViolation back into the next dispatch's prompt as
// LLM-facing context. R6 forbids internal pipeline stage names from
// LLM-facing strings; the message used to read "explorer must call
// read_file ..." (2026-05-10 audit). Pin the reworded form so a
// future edit cannot regress.
func TestCheckTier1Floor_RejectMessageR6(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.StrictGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	mu := types.NewMutableState("any q")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 1,
		GroundingStatus: types.GroundingRecovered,
	}})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu}}
	ir := &types.AnalysisIR{RequestModel: types.RequestModel{Scenario: types.ScenarioGeneric}}
	state := &graphState{}
	msg, proceed, _ := o.checkTier1Floor(ir, state)
	if proceed {
		t.Skip("strict policy did not reject this evidence shape; nothing to audit")
	}
	if msg == "" {
		t.Fatal("reject message empty when proceed=false")
	}
	for _, banned := range []string{
		"explorer must",
		"extractor must",
		"finalizer must",
		"analyzer must",
		"BusContext",
		"MutableState",
		"AnalysisIR",
	} {
		if strings.Contains(msg, banned) {
			t.Errorf("R6 leak: reject message contains internal term %q: %q", banned, msg)
		}
	}
	if !strings.Contains(msg, "read_file") {
		t.Errorf("reject message must still tell the model to call read_file: %q", msg)
	}
}
