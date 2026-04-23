package tool

import (
	"encoding/json"
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
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
		"reason":     "traced the mechanism",
		"confidence": "high",
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
		"reason":     "first attempt",
		"confidence": "high",
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

// TestEmitInvestigationComplete_PreCompleteCheck_MultiPathCoverageParity_Blocks
// pins the session-28 hard-gate: when ≥ 2 primary anchors exist and
// one has < 30% of the max-covered anchor's line coverage, the tool
// must downgrade and queue the under-covered file in PendingReads so
// the LLM has to balance before the next completion attempt.
func TestEmitInvestigationComplete_PreCompleteCheck_MultiPathCoverageParity_Blocks(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/agent/extractor.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	// Both files are in ReadSet so the primary_anchor gate doesn't
	// pre-empt this test. The coverage imbalance is what matters.
	closure.SetReadSet(map[string]bool{
		"internal/agent/explorer.go":  true,
		"internal/agent/extractor.go": true,
	})
	closure.SetReadRanges(map[string][]types.LineRange{
		// explorer.go covered 200 lines; extractor.go only 20 — 10%
		// ratio, well under the 30% parity floor.
		"internal/agent/explorer.go":  {{Start: 1, End: 200}},
		"internal/agent/extractor.go": {{Start: 1, End: 20}},
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "mechanism",
					PrimaryEntities: []string{"explorer.go", "extractor.go"},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":     "compared both paths",
		"confidence": "high",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(res.Summary, "DOWNGRADED") {
		t.Fatalf("expected DOWNGRADED on multi-path coverage imbalance, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "internal/agent/extractor.go") {
		t.Fatalf("expected the under-covered file in downgrade message, got: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Errorf("InvestigationComplete must stay false on coverage imbalance")
	}

	// The under-covered file must be queued for re-read with
	// multi_path_coverage origin so retry hints can cite it.
	var foundOrigin bool
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_coverage" && strings.Contains(p.File, "extractor.go") {
			foundOrigin = true
			break
		}
	}
	if !foundOrigin {
		t.Errorf("expected PendingRead with origin=pre_complete.multi_path_coverage for extractor.go; got %+v", closure.PendingReads())
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_MultiPathCoverageParity_BalancedPasses
// locks the complementary contract: when coverage is within the 30%
// floor the gate does NOT fire, so balanced investigations proceed.
func TestEmitInvestigationComplete_PreCompleteCheck_MultiPathCoverageParity_BalancedPasses(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/agent/extractor.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/agent/explorer.go":  true,
		"internal/agent/extractor.go": true,
	})
	closure.SetReadRanges(map[string][]types.LineRange{
		// 200 vs 100 lines → 50% ratio, well above the 30% floor.
		"internal/agent/explorer.go":  {{Start: 1, End: 200}},
		"internal/agent/extractor.go": {{Start: 1, End: 100}},
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "mechanism",
					PrimaryEntities: []string{"explorer.go", "extractor.go"},
				},
			},
		},
	}

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":     "compared both paths",
		"confidence": "high",
	})
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	// Match ONLY the multi-path-coverage-specific downgrade text. A
	// different gate downstream (e.g. citation floor) might still
	// DOWNGRADE for its own reason, but the balance gate itself must
	// not fire.
	if strings.Contains(res.Summary, "multi-path balance") {
		t.Fatalf("balanced coverage should not trigger multi-path gate, got: %s", res.Summary)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_coverage" {
			t.Fatalf("balanced coverage should not queue multi_path_coverage PendingRead, got %+v", p)
		}
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_MultiPathCoverageParity_NonBreadthIntent_NoOp
// proves the breadth-intent filter: enumeration (and registration /
// return_value / config_mapping) are single-lookup intents where
// unequal coverage across anchors is expected and the gate must not
// fire, otherwise "list all agents"-style questions would be blocked
// by incidentally-named sibling files.
func TestEmitInvestigationComplete_PreCompleteCheck_MultiPathCoverageParity_NonBreadthIntent_NoOp(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetPhase1Ranking([]types.Phase1RankedFile{
		{Path: "internal/agent/explorer.go", Score: 60, ExactEntityRank: 2},
		{Path: "internal/agent/extractor.go", Score: 58, ExactEntityRank: 2},
	})
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{
		"internal/agent/explorer.go":  true,
		"internal/agent/extractor.go": true,
	})
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/agent/explorer.go":  {{Start: 1, End: 200}},
		"internal/agent/extractor.go": {{Start: 1, End: 5}}, // 2.5%
	})

	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Kind: "enumeration",
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{"reason": "listed all", "confidence": "high"})
	_, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_coverage" {
			t.Fatalf("enumeration intent must skip multi-path gate, got %+v", p)
		}
	}
}

// TestEmitInvestigationComplete_PreCompleteCheck_MultiPathCoverageParity_SingleAnchor_NoOp
// guards against false-fire on single-subject questions — one primary
// anchor has nothing to balance against.
func TestEmitInvestigationComplete_PreCompleteCheck_MultiPathCoverageParity_SingleAnchor_NoOp(t *testing.T) {
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

	tool := &EmitInvestigationComplete{}
	params, _ := json.Marshal(map[string]any{
		"reason":     "single-subject mechanism",
		"confidence": "high",
	})
	_, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, p := range closure.PendingReads() {
		if p.Origin == "pre_complete.multi_path_coverage" {
			t.Fatalf("single anchor should not trigger multi-path gate, got %+v", p)
		}
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
