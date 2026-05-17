package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestShouldDispatchExploreNodesIndividually_PositiveTwoEvidenceSiblings
// pins the canonical happy path: two NodeEvidence sibling nodes (the
// shape produced by expandEvidenceNodes when len(SubTopics) > 1) trip
// the predicate so the scheduler will give each its own focused
// dispatch.
func TestShouldDispatchExploreNodesIndividually_PositiveTwoEvidenceSiblings(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
	}
	if !shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("two evidence siblings must trip per-node dispatch")
	}
}

// TestShouldDispatchExploreNodesIndividually_PositiveThreeEvidenceSiblings
// confirms the N=3 case (the forensic 01:44/08:14 case from the
// commit-message anchor) also fires.
func TestShouldDispatchExploreNodesIndividually_PositiveThreeEvidenceSiblings(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
		{ID: "n1_evidence_t2", Type: types.NodeEvidence},
	}
	if !shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("three evidence siblings must trip per-node dispatch")
	}
}

// TestShouldDispatchExploreNodesIndividually_SingleNodeSkipped pins
// the byte-equivalent fast-path: single-sub_topic questions (len ==
// 1) go through the original merged-window code path completely
// unchanged.
func TestShouldDispatchExploreNodesIndividually_SingleNodeSkipped(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence", Type: types.NodeEvidence},
	}
	if shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("single evidence node must NOT trip per-node dispatch (byte-equivalent path)")
	}
}

// TestShouldDispatchExploreNodesIndividually_EmptyWindowSkipped pins
// nil-safety.
func TestShouldDispatchExploreNodesIndividually_EmptyWindowSkipped(t *testing.T) {
	if shouldDispatchExploreNodesIndividually(nil) {
		t.Fatal("nil window must NOT trip per-node dispatch")
	}
	if shouldDispatchExploreNodesIndividually([]*types.TaskNode{}) {
		t.Fatal("empty window must NOT trip per-node dispatch")
	}
}

// TestShouldDispatchExploreNodesIndividually_SingleEvidencePlusCompanionsSkipped
// pins that a window with only ONE evidence node — regardless of how
// many non-evidence companions are present — does NOT trip per-node
// dispatch. The companions stay merged with the single evidence
// node as before E'.
func TestShouldDispatchExploreNodesIndividually_SingleEvidencePlusCompanionsSkipped(t *testing.T) {
	cases := []struct {
		name string
		w    []*types.TaskNode
	}{
		{
			"evidence_plus_probe",
			[]*types.TaskNode{
				{ID: "n1_evidence_t0", Type: types.NodeEvidence},
				{ID: "n1_probe", Type: types.NodeProbe},
			},
		},
		{
			"evidence_plus_validate",
			[]*types.TaskNode{
				{ID: "n1_evidence_t0", Type: types.NodeEvidence},
				{ID: "n1_validate", Type: types.NodeValidate},
			},
		},
		{
			"evidence_plus_reconcile",
			[]*types.TaskNode{
				{ID: "n1_evidence_t0", Type: types.NodeEvidence},
				{ID: "n1_reconcile", Type: types.NodeReconcile},
			},
		},
		{
			"all_validate_no_evidence",
			[]*types.TaskNode{
				{ID: "v1", Type: types.NodeValidate},
				{ID: "v2", Type: types.NodeValidate},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if shouldDispatchExploreNodesIndividually(c.w) {
				t.Errorf("case %q must NOT trip per-node dispatch", c.name)
			}
		})
	}
}

// TestShouldDispatchExploreNodesIndividually_ProductionShapeWithCompanions
// pins the LOAD-BEARING case: production-shape windows
// (`probe + N evidence siblings + validate`) MUST trip per-node
// dispatch even though probe/validate are present. The trim helper
// then keeps probe + first evidence + validate while subsequent
// scheduler iterations pick up the remaining siblings.
//
// This is the byte-precise forensic-anchor test for the 08:14 run.
func TestShouldDispatchExploreNodesIndividually_ProductionShapeWithCompanions(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n0_probe", Type: types.NodeProbe},
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		{ID: "n1_evidence_t1", Type: types.NodeEvidence},
		{ID: "n2_validate", Type: types.NodeValidate},
	}
	if !shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("production shape (probe + 2 evidence siblings + validate) MUST trip per-node dispatch")
	}
}

// TestShouldDispatchExploreNodesIndividually_NilEntrySkipped pins the
// defensive nil-entry check (the window slice is built by the
// scheduler and SHOULD have no nil entries, but a future bug in
// readyExplorerWindowContext should not silently fan out).
func TestShouldDispatchExploreNodesIndividually_NilEntrySkipped(t *testing.T) {
	window := []*types.TaskNode{
		{ID: "n1_evidence_t0", Type: types.NodeEvidence},
		nil,
	}
	if shouldDispatchExploreNodesIndividually(window) {
		t.Fatal("nil entry in window must NOT trip per-node dispatch")
	}
}

// TestTrimExploreWindowToFirstEvidence_DropsExtraSiblingsKeepsCompanions
// pins the trim helper's load-bearing contract on production-shape
// windows: companions (probe / validate) survive; only sibling
// evidence beyond the first is dropped.
func TestTrimExploreWindowToFirstEvidence_DropsExtraSiblingsKeepsCompanions(t *testing.T) {
	probe := &types.TaskNode{ID: "n0_probe", Type: types.NodeProbe}
	t0 := &types.TaskNode{ID: "n1_evidence_t0", Type: types.NodeEvidence}
	t1 := &types.TaskNode{ID: "n1_evidence_t1", Type: types.NodeEvidence}
	t2 := &types.TaskNode{ID: "n1_evidence_t2", Type: types.NodeEvidence}
	validate := &types.TaskNode{ID: "n2_validate", Type: types.NodeValidate}
	window := []*types.TaskNode{probe, t0, t1, t2, validate}

	trimmed := trimExploreWindowToFirstEvidence(window)
	if len(trimmed) != 3 {
		t.Fatalf("trimmed window length = %d, want 3 (probe + t0 + validate); got: %+v", len(trimmed), trimmed)
	}
	if trimmed[0] != probe {
		t.Errorf("trimmed[0] must be probe (preserved companion); got %s", trimmed[0].ID)
	}
	if trimmed[1] != t0 {
		t.Errorf("trimmed[1] must be the FIRST evidence (t0); got %s", trimmed[1].ID)
	}
	if trimmed[2] != validate {
		t.Errorf("trimmed[2] must be validate (preserved companion); got %s", trimmed[2].ID)
	}
	for _, n := range trimmed {
		if n != nil && n.ID == "n1_evidence_t1" {
			t.Errorf("trimmed window must NOT contain dropped sibling t1")
		}
		if n != nil && n.ID == "n1_evidence_t2" {
			t.Errorf("trimmed window must NOT contain dropped sibling t2")
		}
	}
}

// TestTrimExploreWindowToFirstEvidence_NoEvidenceLeavesWindowUnchanged
// pins that windows with no evidence siblings pass through verbatim.
func TestTrimExploreWindowToFirstEvidence_NoEvidenceLeavesWindowUnchanged(t *testing.T) {
	probe := &types.TaskNode{ID: "n0_probe", Type: types.NodeProbe}
	validate := &types.TaskNode{ID: "n2_validate", Type: types.NodeValidate}
	window := []*types.TaskNode{probe, validate}

	trimmed := trimExploreWindowToFirstEvidence(window)
	if len(trimmed) != 2 {
		t.Errorf("no-evidence window must pass through; got len=%d", len(trimmed))
	}
}

// TestTrimExploreWindowToFirstEvidence_SingleEvidenceLeavesWindowUnchanged
// pins that single-evidence windows are unchanged (byte-equivalent
// to the pre-E' merged-dispatch path).
func TestTrimExploreWindowToFirstEvidence_SingleEvidenceLeavesWindowUnchanged(t *testing.T) {
	probe := &types.TaskNode{ID: "n0_probe", Type: types.NodeProbe}
	t0 := &types.TaskNode{ID: "n1_evidence_t0", Type: types.NodeEvidence}
	validate := &types.TaskNode{ID: "n2_validate", Type: types.NodeValidate}
	window := []*types.TaskNode{probe, t0, validate}

	trimmed := trimExploreWindowToFirstEvidence(window)
	if len(trimmed) != 3 {
		t.Errorf("single-evidence window must pass through; got len=%d", len(trimmed))
	}
}

// TestTrimExploreWindowToFirstEvidence_EmptyReturnsEmpty pins nil-safety.
func TestTrimExploreWindowToFirstEvidence_EmptyReturnsEmpty(t *testing.T) {
	if got := trimExploreWindowToFirstEvidence(nil); len(got) != 0 {
		t.Errorf("nil window must trim to empty; got len=%d", len(got))
	}
	if got := trimExploreWindowToFirstEvidence([]*types.TaskNode{}); len(got) != 0 {
		t.Errorf("empty window must trim to empty; got len=%d", len(got))
	}
}

// ============================================================
// E2E: TaskGraph with N evidence siblings → N explorer dispatches
// ============================================================

// dagIRMultiTopic builds an AnalysisIR with N independent evidence
// sibling nodes (`{prefix}_t{i}` shape) that mirror the production
// expandEvidenceNodes output for multi-sub_topic questions.
func dagIRMultiTopic(siblingCount int) *types.AnalysisIR {
	if siblingCount < 2 {
		siblingCount = 2
	}
	nodes := []types.TaskNode{
		{ID: "n0", Type: types.NodeProbe, Objective: "scan repo",
			SearchHints: types.SearchHints{KeywordIDs: []string{"k1"}}},
	}
	edges := []types.TaskEdge{}
	// N evidence siblings, each `n1_evidence_tI`, no sibling-sibling
	// edge (so shouldDispatchExploreNodesIndividually fires).
	for i := 0; i < siblingCount; i++ {
		nodeID := "n1_evidence_t" + string(rune('0'+i))
		nodes = append(nodes, types.TaskNode{
			ID:          nodeID,
			Type:        types.NodeEvidence,
			Objective:   "evidence for sub-topic " + string(rune('0'+i)),
			SearchHints: types.SearchHints{EntityIDs: []string{"e" + string(rune('0'+i))}},
		})
		edges = append(edges,
			types.TaskEdge{From: "n0", To: nodeID, EdgeType: types.EdgeHardDependency},
			types.TaskEdge{From: nodeID, To: "n2_validate", EdgeType: types.EdgeHardDependency},
		)
	}
	nodes = append(nodes,
		types.TaskNode{ID: "n2_validate", Type: types.NodeValidate, Objective: "check chains"},
		types.TaskNode{ID: "n3", Type: types.NodeFinalize, Objective: "render answer"},
	)
	edges = append(edges,
		types.TaskEdge{From: "n2_validate", To: "n3", EdgeType: types.EdgeHardDependency},
	)
	critPath := []string{"n0", "n1_evidence_t0", "n2_validate", "n3"}
	return &types.AnalysisIR{
		Version: types.AnalysisIRVersion,
		RequestModel: types.RequestModel{
			Language: "en",
			Intent:   types.IntentExplain,
			SubTopics: []types.SubTopic{
				{Summary: "sub-topic 0", Entities: []string{"e0"}},
				{Summary: "sub-topic 1", Entities: []string{"e1"}},
			},
		},
		TaskGraph: types.TaskGraph{
			Nodes:           nodes,
			Edges:           edges,
			ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 1, CriticalPath: critPath},
		},
		EvidencePlan: types.EvidencePlan{
			Budget: types.EvidenceBudget{MaxFiles: 30, MaxBytes: 200000, MaxReactIters: 10, MaxToolCalls: 40},
		},
		AnswerContract: types.AnswerContract{Language: "en"},
	}
}

// TestRunTaskGraph_PerNodeDispatch_TwoEvidenceSiblings is the E'
// load-bearing e2e test: when the analyzer produces 2 independent
// evidence sibling nodes, the explorer MUST be dispatched twice
// (once per sub-topic) — not once with both objectives merged into
// the same hint.
//
// Forensic anchor: this is the byte-precise regression test for the
// 08:14 "compare codrax and opencode" run where one explorer instance
// chewed through both sub-topics serially for 7+ minutes. After E',
// the scheduler dispatches each sibling independently so each
// dispatch's hint is focused on a single sub-topic.
func TestRunTaskGraph_PerNodeDispatch_TwoEvidenceSiblings(t *testing.T) {
	var explorerCalls int
	var observedExplorerHints []string

	ir := dagIRMultiTopic(2)

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			observedExplorerHints = append(observedExplorerHints, ctx.RetryHint)
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("compare A and B", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// E': 2 sibling evidence nodes → 2 explorer dispatches (not 1
	// merged).
	if explorerCalls != 2 {
		t.Errorf("E': expected 2 explorer dispatches (1 per evidence sibling), got %d", explorerCalls)
	}
	if len(observedExplorerHints) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(observedExplorerHints))
	}

	// E': each hint should focus on ONE sub-topic. Concretely:
	//   - dispatch 0 mentions sub-topic 0 (entity e0); does NOT
	//     mention sub-topic 1's entity e1
	//   - dispatch 1 mentions sub-topic 1's objective; does NOT
	//     re-list sub-topic 0
	hint0 := observedExplorerHints[0]
	hint1 := observedExplorerHints[1]
	if !strings.Contains(hint0, "sub-topic 0") {
		t.Errorf("dispatch 0 hint must mention 'sub-topic 0'; got %q", hint0)
	}
	if strings.Contains(hint0, "sub-topic 1") {
		t.Errorf("dispatch 0 hint must NOT mention 'sub-topic 1' (would defeat per-node focus); got %q", hint0)
	}
	if !strings.Contains(hint1, "sub-topic 1") {
		t.Errorf("dispatch 1 hint must mention 'sub-topic 1'; got %q", hint1)
	}
	if strings.Contains(hint1, "sub-topic 0") {
		t.Errorf("dispatch 1 hint must NOT mention 'sub-topic 0' (sibling already done); got %q", hint1)
	}
}

// TestRunTaskGraph_SingleEvidenceNode_ByteEquivalent pins the
// zero-regression guarantee: a single-sub_topic question (the
// dagIR happy path) MUST behave exactly as before E' — one explorer
// dispatch covering one evidence node. trimExploreWindowToFirstEvidence
// must NOT fire here.
func TestRunTaskGraph_SingleEvidenceNode_ByteEquivalent(t *testing.T) {
	var explorerCalls int

	ir := dagIR(types.AnswerContract{Language: "en"})

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Single-sub_topic → exactly 1 explorer dispatch (unchanged).
	if explorerCalls != 1 {
		t.Errorf("single-sub_topic: expected 1 explorer dispatch (byte-equivalent path), got %d", explorerCalls)
	}
}
