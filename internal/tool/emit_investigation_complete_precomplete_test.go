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

func TestEmitInvestigationComplete_PreCompleteCheck_ExplanationFunctionSubject_NoShapeSwap(t *testing.T) {
	mut := types.NewMutableState("test")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/tool/repomap/tool.go": true})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Source:    "internal/tool/repomap/tool.go",
			LineStart: 133,
			LineEnd:   133,
			Kind:      types.EvidenceDirect,
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
				RequiredAnswerShape: types.ShapeExplanation,
				CitationReq: types.CitationReq{
					Required:     true,
					MinCitations: 1,
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":     "traced the mechanism",
		"confidence": "high",
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
	if got := closure.Stats().ShapeSwapRaised; got != 0 {
		t.Fatalf("ShapeSwapRaised=%d, want 0 for explanation anchored on a function", got)
	}
	for _, repair := range closure.PendingRepairs() {
		if repair.Origin == "pre_complete.subject_shape_mismatch" {
			t.Fatalf("unexpected shape-swap repair: %+v", repair)
		}
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
		"reason":     "traced the answer",
		"confidence": "high",
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
		"reason":     "traced the mechanism",
		"confidence": "high",
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
		"reason":     "traced the mechanism",
		"confidence": "high",
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
		"reason":     "traced the mechanism",
		"confidence": "high",
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
		"reason":     "found the registration",
		"confidence": "high",
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
		"absence_justification": "grep produced zero hits for the claimed symbol",
	})
	res, _ := tool.Execute(bus, params)
	if strings.Contains(res.Summary, "DOWNGRADED") {
		t.Errorf("absence answer must bypass phase1-unread gate: %s", res.Summary)
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
