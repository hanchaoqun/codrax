package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

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
		"reason":     "looks complete",
		"confidence": "high",
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
		"reason":     "looks complete",
		"confidence": "high",
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
		"reason":     "thinks done",
		"confidence": "high",
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
		"reason":     "all evidence collected",
		"confidence": "high",
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
		"reason":               "the system has no such handler",
		"confidence":           "high",
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
