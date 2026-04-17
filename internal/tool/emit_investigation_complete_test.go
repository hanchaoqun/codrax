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

	params := json.RawMessage(`{"reason":"done","confidence":"high","absence_justification":"answer is zero"}`)
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

	params := json.RawMessage(`{"reason":"none found","confidence":"high","absence_justification":"no .py files exist"}`)
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

// TestEmitInvestigationComplete_CompletionWithoutAbsenceOnEvidenceAccepted
// — the normal happy path: grounded evidence exists, LLM signals
// completion WITHOUT absence_justification. Must succeed.
func TestEmitInvestigationComplete_CompletionWithoutAbsenceOnEvidenceAccepted(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"evidence collected","confidence":"high"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("normal completion must succeed: %s", res.Summary)
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

	params := json.RawMessage(`{"reason":"done","confidence":"high"}`)
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

	params := json.RawMessage(`{"reason":"done","confidence":"high"}`)
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

	params := json.RawMessage(`{"reason":"done","confidence":"high"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Errorf("Tier1Floor=0 must disable the gate; got rejection: %s", res.Summary)
	}
}
