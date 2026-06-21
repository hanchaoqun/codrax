package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/types"
)

// scheduler_test.go locks the graphState abstraction and the
// node-type→stage mapping.

func smallChainGraph() types.TaskGraph {
	probe := types.TaskNode{ID: "n0", Type: types.NodeProbe, Objective: "scan"}
	ev := types.TaskNode{ID: "n1", Type: types.NodeEvidence, Objective: "collect"}
	val := types.TaskNode{ID: "n2", Type: types.NodeValidate, Objective: "check", MaxRetries: 2}
	final := types.TaskNode{ID: "n3", Type: types.NodeFinalize, Objective: "answer"}
	return types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, final},
		Edges: []types.TaskEdge{
			{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
			{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
			{From: "n2", To: "n3", EdgeType: types.EdgeHardDependency},
			{From: "n2", To: "n1", EdgeType: types.EdgeValidationFeedback, Guard: "any_unknown"},
		},
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: 3,
			CriticalPath: []string{"n0", "n1", "n2", "n3"},
		},
	}
}

func chainGraphWithExtract() types.TaskGraph {
	probe := types.TaskNode{ID: "n0", Type: types.NodeProbe, Objective: "scan"}
	ev := types.TaskNode{ID: "n1", Type: types.NodeEvidence, Objective: "collect"}
	extract := types.TaskNode{ID: "n2", Type: types.NodeExtract, Objective: "distill"}
	final := types.TaskNode{ID: "n3", Type: types.NodeFinalize, Objective: "answer"}
	return types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, extract, final},
		Edges: []types.TaskEdge{
			{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
			{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
			{From: "n2", To: "n3", EdgeType: types.EdgeHardDependency},
		},
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: 3,
			CriticalPath: []string{"n0", "n1", "n2", "n3"},
		},
	}
}

func emptyEnv() criterion.Env {
	return criterion.Env{}
}

func TestGraphState_ReadyWindowHonorsHardDependencies(t *testing.T) {
	s := newGraphState(smallChainGraph())
	window, _ := s.readyExplorerWindow(emptyEnv())
	if got := idsOf(window); strings.Join(got, ",") != "n0" {
		t.Fatalf("first window: want only root dependency n0, got %v", got)
	}
	s.markRunning("n0")
	s.markDone("n0")
	window, _ = s.readyExplorerWindow(emptyEnv())
	if got := idsOf(window); strings.Join(got, ",") != "n1" {
		t.Fatalf("after n0 done: want n1, got %v", got)
	}
	s.markRunning("n1")
	s.markDone("n1")
	window, _ = s.readyExplorerWindow(emptyEnv())
	if got := idsOf(window); strings.Join(got, ",") != "n2" {
		t.Fatalf("after n1 done: want n2, got %v", got)
	}
	s.markRunning("n2")
	s.markDone("n2")
	window, _ = s.readyExplorerWindow(emptyEnv())
	if len(window) != 0 {
		t.Errorf("after marking all done: want empty, got %v", idsOf(window))
	}
}

func TestGraphState_NodeExecStatusLoadBearing(t *testing.T) {
	s := newGraphState(smallChainGraph())
	closure := types.NewEvidenceClosure("")
	s.attachEvidenceClosure(closure)

	if got := closure.NodeExecStatus("n1"); got != types.NodeExecPending {
		t.Fatalf("initial closure status = %q, want pending", got)
	}
	if s.status != nil {
		t.Fatalf("bootstrap status map should be retired after closure attach")
	}
	s.markRunning("n1")
	if s.nodeStatus("n1") != nodeRunning || closure.NodeExecStatus("n1") != types.NodeExecRunning {
		t.Fatalf("running status mismatch: graph=%q closure=%q", s.nodeStatus("n1"), closure.NodeExecStatus("n1"))
	}
	s.markDone("n1")
	if s.nodeStatus("n1") != nodeDone || closure.NodeExecStatus("n1") != types.NodeExecDone {
		t.Fatalf("done status mismatch: graph=%q closure=%q", s.nodeStatus("n1"), closure.NodeExecStatus("n1"))
	}
	s.requeue("n1")
	if s.nodeStatus("n1") != nodeRequeued || closure.NodeExecStatus("n1") != types.NodeExecRequeued {
		t.Fatalf("requeued status mismatch: graph=%q closure=%q", s.nodeStatus("n1"), closure.NodeExecStatus("n1"))
	}

	s.status = map[string]nodeStatus{"n1": nodeDone}
	closure.SetNodeExecStatus("n1", types.NodeExecRequeued)
	if got := s.nodeStatus("n1"); got != nodeRequeued {
		t.Fatalf("closure status must be load-bearing over stale bootstrap map; got %q", got)
	}
}

func TestGraphState_ReadyWindowUsesClosureStatus(t *testing.T) {
	s := newGraphState(smallChainGraph())
	closure := types.NewEvidenceClosure("")
	s.attachEvidenceClosure(closure)
	closure.SetNodeExecStatus("n0", types.NodeExecDone)
	s.status = map[string]nodeStatus{"n0": nodePending}

	window, _ := s.readyExplorerWindow(emptyEnv())
	if got := idsOf(window); strings.Join(got, ",") != "n1" {
		t.Fatalf("ready window must use closure status for hard-dependency readiness; got %v", got)
	}
}

func TestGraphState_ReadyWindowContextCancelled(t *testing.T) {
	s := newGraphState(smallChainGraph())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.readyExplorerWindowContext(ctx, emptyEnv()); err == nil {
		t.Fatal("readyExplorerWindowContext must report cancellation")
	}
}

func TestGraphState_WindowExcludesFinalize(t *testing.T) {
	s := newGraphState(smallChainGraph())
	for _, id := range []string{"n0", "n1", "n2"} {
		s.markRunning(id)
		s.markDone(id)
	}
	window, _ := s.readyExplorerWindow(emptyEnv())
	if len(window) != 0 {
		t.Errorf("window after chain done: want empty, got %v", idsOf(window))
	}
	fin := s.firstFinalizeReadyMerged()
	if fin == nil || fin.ID != "n3" {
		t.Errorf("finalize ready: want n3, got %v", fin)
	}
}

func TestGraphState_WindowExcludesExtractStageNode(t *testing.T) {
	s := newGraphState(chainGraphWithExtract())
	window, _ := s.readyExplorerWindow(emptyEnv())
	if got := idsOf(window); strings.Join(got, ",") != "n0" {
		t.Fatalf("explore window should exclude extract/finalize and honor hard deps; got %v", got)
	}
}

func TestGraphState_FinalizeReadinessIgnoresPendingExtractStageNode(t *testing.T) {
	s := newGraphState(chainGraphWithExtract())
	for _, id := range []string{"n0", "n1"} {
		s.markRunning(id)
		s.markDone(id)
	}
	fin := s.firstFinalizeReadyMerged()
	if fin == nil || fin.ID != "n3" {
		t.Fatalf("finalize readiness should preserve legacy pre-finalize extract dispatch; got %v", fin)
	}
	if s.nodeStatus("n2") != nodePending {
		t.Fatalf("extract node should remain pending for the finalize branch to dispatch; got %s", s.nodeStatus("n2"))
	}
}

func TestGraphState_RetryBudgetExhausted(t *testing.T) {
	s := newGraphState(smallChainGraph())
	if s.retryBudgetExhausted() {
		t.Fatal("fresh state should not be exhausted (budget=3)")
	}
	s.recordRetry()
	s.recordRetry()
	s.recordRetry()
	if !s.retryBudgetExhausted() {
		t.Error("after 3 retries with budget=3 should be exhausted")
	}
}

func TestGraphState_TransientNoEmitPlateau(t *testing.T) {
	state := newGraphState(smallChainGraph())
	nodeID := "n1"
	if state.transientNoEmitPlateau(nodeID) {
		t.Fatal("plateau must not fire on a fresh node")
	}
	state.recordTransientNoEmitStall(nodeID)
	if state.transientNoEmitPlateau(nodeID) {
		t.Fatal("one stall is not yet a plateau (threshold is 2)")
	}
	state.recordTransientNoEmitStall(nodeID)
	if !state.transientNoEmitPlateau(nodeID) {
		t.Fatal("two consecutive no-emit stalls should fire advisory plateau")
	}
	state.resetTransientNoEmitStreak(nodeID)
	if state.transientNoEmitPlateau(nodeID) {
		t.Fatal("reset must clear the streak")
	}
}

func TestGraphState_RequeueValidationTargets(t *testing.T) {
	s := newGraphState(smallChainGraph())
	// Walk to validate stage.
	for _, id := range []string{"n0", "n1", "n2"} {
		s.markRunning(id)
		s.markDone(id)
	}
	targets := s.requeueValidationTargets("n2")
	// Validation feedback: n2 → n1 (from smallChainGraph edges).
	if len(targets) != 1 || targets[0] != "n1" {
		t.Fatalf("want [n1], got %v", targets)
	}
	if s.nodeStatus("n1") != nodeRequeued || s.nodeStatus("n2") != nodeRequeued {
		t.Errorf("n1/n2 should be requeued; n1=%v n2=%v", s.nodeStatus("n1"), s.nodeStatus("n2"))
	}
	if s.nodeStatus("n0") != nodeDone {
		t.Error("n0 (not a validation target) must stay done")
	}
}

func TestGraphState_ForceCloseExploreWindow(t *testing.T) {
	s := newGraphState(smallChainGraph())
	s.forceCloseExploreWindow()
	for _, id := range []string{"n0", "n1", "n2"} {
		if s.nodeStatus(id) != nodeDone {
			t.Errorf("%s should be forced to done; got %v", id, s.nodeStatus(id))
		}
	}
	if s.nodeStatus("n3") == nodeDone {
		t.Error("finalize must not be force-closed")
	}
}

func TestGraphState_ForceCloseExploreWindowLeavesExtractPending(t *testing.T) {
	s := newGraphState(chainGraphWithExtract())
	s.forceCloseExploreWindow()
	for _, id := range []string{"n0", "n1"} {
		if s.nodeStatus(id) != nodeDone {
			t.Errorf("%s should be forced to done; got %v", id, s.nodeStatus(id))
		}
	}
	if s.nodeStatus("n2") == nodeDone {
		t.Error("extract must not be force-closed by the explore window")
	}
	if s.nodeStatus("n3") == nodeDone {
		t.Error("finalize must not be force-closed")
	}
}

func TestGraphState_EntryConditionBlocks(t *testing.T) {
	g := smallChainGraph()
	// Gate n1 on has_enough_facts. Env.Signals is zero → blocked.
	g.Nodes[1].EntryConditions = []types.Criterion{
		{Kind: types.CritHasEnoughFacts},
	}
	s := newGraphState(g)
	window, blocked := s.readyExplorerWindow(emptyEnv())
	if got := idsOf(window); strings.Join(got, ",") != "n0" {
		t.Fatalf("hard dependency should dispatch n0 before evaluating downstream entry gates; got %v", got)
	}
	if len(blocked) != 0 {
		t.Fatalf("dependency-waiting downstream nodes must not add blocked noise while n0 is ready: %+v", blocked)
	}
	s.markRunning("n0")
	s.markDone("n0")
	window, blocked = s.readyExplorerWindow(emptyEnv())
	if len(window) != 0 {
		t.Errorf("want no ready nodes while n1 entry gate is false; got %v", idsOf(window))
	}
	foundBlocked := false
	for _, b := range blocked {
		if b.NodeID == "n1" {
			foundBlocked = true
		}
	}
	if !foundBlocked {
		t.Error("n1 should be blocked on entry condition")
	}
	env := criterion.Env{Signals: types.ExecutionSignals{HasEnoughFacts: true}}
	window, _ = s.readyExplorerWindow(env)
	if got := idsOf(window); strings.Join(got, ",") != "n1" {
		t.Errorf("after signal: want n1 only; got %v", got)
	}
}

func TestGraphState_HardDependencyBlockersSurfaceOnlyWhenNoReadyWindow(t *testing.T) {
	s := newGraphState(smallChainGraph())
	s.markFailed("n0")
	window, blocked := s.readyExplorerWindow(emptyEnv())
	if len(window) != 0 {
		t.Fatalf("failed hard predecessor should leave no ready window; got %v", idsOf(window))
	}
	if len(blocked) == 0 {
		t.Fatal("dependency blocker should be surfaced when no node is ready")
	}
	var found bool
	for _, b := range blocked {
		if b.NodeID == "n1" && strings.Join(b.DependencyBlockers, ",") == "n0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("blocked nodes should include n1 waiting for n0; got %+v", blocked)
	}
	hint := renderWindowHint(nil, blocked, nil, nil, "", "", nil)
	if !strings.Contains(hint, "Dependency gate") || !strings.Contains(hint, "n1: waiting for n0") {
		t.Fatalf("dependency blockers should render as typed dependency gate, got:\n%s", hint)
	}
}

func TestGraphState_OptionalEntryConditionDoesNotBlockFinalize(t *testing.T) {
	g := chainGraphWithExtract()
	optional := types.TaskNode{
		ID:       "n_opt",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceClassUniverseIncomplete,
		}},
	}
	g.Nodes = append([]types.TaskNode{optional}, g.Nodes...)
	s := newGraphState(g)
	window, blocked := s.readyExplorerWindow(emptyEnv())
	if got := idsOf(window); strings.Contains(strings.Join(got, ","), "n_opt") {
		t.Fatalf("optional node with false entry condition should not be ready: %v", got)
	}
	for _, b := range blocked {
		if b.NodeID == "n_opt" {
			t.Fatalf("optional node with false entry condition must not add blocked noise: %+v", blocked)
		}
	}
	for _, id := range []string{"n0", "n1"} {
		s.markRunning(id)
		s.markDone(id)
	}
	if fin := s.firstFinalizeReadyMerged(); fin == nil || fin.ID != "n3" {
		t.Fatalf("optional pending node must not prevent finalize readiness; got %v", fin)
	}
}

func TestGraphState_OptionalEntryConditionCanDispatchWhenTypedSignalTrue(t *testing.T) {
	g := chainGraphWithExtract()
	optional := types.TaskNode{
		ID:       "n_opt",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceClassUniverseIncomplete,
		}},
	}
	g.Nodes = append([]types.TaskNode{optional}, g.Nodes...)
	s := newGraphState(g)
	env := criterion.Env{SourceInventoryActive: true, SourceClassUniverseComplete: false}
	window, blocked := s.readyExplorerWindow(env)
	if len(blocked) != 0 {
		t.Fatalf("optional ready node should not be blocked: %+v", blocked)
	}
	if got := idsOf(window); !strings.Contains(strings.Join(got, ","), "n_opt") {
		t.Fatalf("optional node should be ready when typed criterion is true: %v", got)
	}
}

func TestGraphState_SourceInventoryLensMissingDispatchesWhenTypedSignalTrue(t *testing.T) {
	g := chainGraphWithExtract()
	optional := types.TaskNode{
		ID:       "n_source_inventory_lens",
		Type:     types.NodeProbe,
		Optional: true,
		OneShot:  true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceInventoryLensMissing,
		}},
	}
	g.Nodes = append([]types.TaskNode{optional}, g.Nodes...)
	s := newGraphState(g)
	window, blocked := s.readyExplorerWindow(criterion.Env{
		SourceInventoryProfileActive: true,
		SourceInventoryLensExecuted:  false,
	})
	if len(blocked) != 0 {
		t.Fatalf("optional source-inventory lens node should not be blocked: %+v", blocked)
	}
	if got := idsOf(window); !strings.Contains(strings.Join(got, ","), "n_source_inventory_lens") {
		t.Fatalf("source-inventory lens node should be ready from typed execution state: %v", got)
	}
}

func TestAnalyzeRefineUsesPreAuthoredOptionalNodeOnly(t *testing.T) {
	g := chainGraphWithExtract()
	refine := types.TaskNode{
		ID:        "n_refine",
		Type:      types.NodeProbe,
		Objective: "Refine analysis scope using typed progress delta.",
		Optional:  true,
		OneShot:   true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritProgressReplanRequired,
		}},
	}
	g.Nodes = append([]types.TaskNode{refine}, g.Nodes...)
	s := newGraphState(g)
	window, blocked := s.readyExplorerWindow(emptyEnv())
	if got := idsOf(window); strings.Contains(strings.Join(got, ","), "n_refine") {
		t.Fatalf("preauthored refine node must stay quiet without typed progress decision: %v", got)
	}
	for _, b := range blocked {
		if b.NodeID == "n_refine" {
			t.Fatalf("preauthored optional refine must not add blocked noise: %+v", blocked)
		}
	}
	env := criterion.Env{ProgressDecision: types.ShouldReplan(types.ProgressDelta{
		Kind:          types.ProgressDeltaDowngradeBlocker,
		Consecutive:   1,
		HardThreshold: 3,
	})}
	window, blocked = s.readyExplorerWindow(env)
	if len(blocked) != 0 {
		t.Fatalf("typed refine node should not be blocked: %+v", blocked)
	}
	if got := idsOf(window); !strings.Contains(strings.Join(got, ","), "n_refine") {
		t.Fatalf("preauthored refine node should be ready when typed progress requires replan: %v", got)
	}
}

// ── envShape fingerprint tests ──────────────────────────────────

// TestEnvShape_EmptyInput_IsZero verifies the baseline: zero Env,
// nil BusContext → zero shape. This is the sentinel value the
// scheduler uses when it has never evaluated a predicate before.
func TestEnvShape_EmptyInput_IsZero(t *testing.T) {
	shape := computeEnvShape(nil, criterion.Env{})
	if shape != (envShape{}) {
		t.Errorf("zero input → %+v, want zero shape", shape)
	}
}

// TestEnvShape_EvidenceCountCaptured verifies the EvidenceCount
// cursor changes when Env.Evidence grows.
func TestEnvShape_EvidenceCountCaptured(t *testing.T) {
	a := computeEnvShape(nil, criterion.Env{
		Evidence: []types.EvidenceItem{{Subject: "a"}},
	})
	b := computeEnvShape(nil, criterion.Env{
		Evidence: []types.EvidenceItem{{Subject: "a"}, {Subject: "b"}},
	})
	if a == b {
		t.Errorf("shape should differ: a=%+v b=%+v", a, b)
	}
	if a.EvidenceCount != 1 || b.EvidenceCount != 2 {
		t.Errorf("EvidenceCount cursor wrong: a=%d b=%d", a.EvidenceCount, b.EvidenceCount)
	}
}

// TestEnvShape_DecidedHypothesesCursor verifies the HypothesisSet
// decision cursor changes when a hypothesis is decided.
func TestEnvShape_DecidedHypothesesCursor(t *testing.T) {
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{ID: "h1", Status: types.HypUnknown},
			{ID: "h2", Status: ""},
			{ID: "h3", Status: types.HypConfirmed},
		},
	}
	a := computeEnvShape(nil, criterion.Env{IR: ir})
	if a.DecidedHypotheses != 1 {
		t.Errorf("DecidedHypotheses = %d, want 1 (only h3 decided)", a.DecidedHypotheses)
	}

	ir.HypothesisSet[0].Status = types.HypRejected
	b := computeEnvShape(nil, criterion.Env{IR: ir})
	if b.DecidedHypotheses != 2 {
		t.Errorf("DecidedHypotheses after h1 decided = %d, want 2", b.DecidedHypotheses)
	}
	if a.equals(b) {
		t.Error("shapes should differ after hypothesis decision")
	}
}

// TestEnvShape_FieldsAreIndependent pins that each field detects its
// own source independently — a change to any one field produces a
// distinct shape, so a false negative on one cursor does not mask
// progress captured by another.
func TestEnvShape_FieldsAreIndependent(t *testing.T) {
	base := envShape{}
	mods := []envShape{
		{EvidenceCount: 1},
		{AnswerSymbolCount: 1},
		{AnswerChainCount: 1},
		{AggregateFactCount: 1},
		{ToolResultCount: 1},
		{ReadSetSize: 1},
		{PendingReadsSize: 1},
		{DecidedHypotheses: 1},
		{PrescanBytes: 1},
	}
	for i, m := range mods {
		if base.equals(m) {
			t.Errorf("mod %d: base equals modified shape %+v (field not load-bearing)", i, m)
		}
	}
}

// TestEnvShape_EqualsIsReflexive sanity-checks Go struct comparison.
func TestEnvShape_EqualsIsReflexive(t *testing.T) {
	s := envShape{EvidenceCount: 3, ToolResultCount: 5, DecidedHypotheses: 1}
	if !s.equals(s) {
		t.Error("shape should equal itself")
	}
}

// ── hypProgress fingerprint tests ───────────────────────────────

// TestHypProgress_EmptyEnv_IsZero pins the baseline: nil IR → zero
// fingerprint, the sentinel "never evaluated" value.
func TestHypProgress_EmptyEnv_IsZero(t *testing.T) {
	if p := computeHypProgress(criterion.Env{}); p != (hypProgress{}) {
		t.Errorf("nil IR → %+v, want zero", p)
	}
}

// TestHypProgress_UnknownCountCaptured verifies that unknown /
// empty-status hypotheses are counted, decided ones are not.
func TestHypProgress_UnknownCountCaptured(t *testing.T) {
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{ID: "h1", Status: types.HypUnknown},
			{ID: "h2", Status: ""}, // also unknown per convention
			{ID: "h3", Status: types.HypConfirmed},
			{ID: "h4", Status: types.HypRejected},
			{ID: "h5", Status: types.HypInconclusive},
		},
	}
	p := computeHypProgress(criterion.Env{IR: ir})
	if p.UnknownCount != 2 {
		t.Errorf("UnknownCount = %d, want 2", p.UnknownCount)
	}
}

// TestHypProgress_SatisfiedReqSumAdvancesOnRelevantEvidence is the
// load-bearing property: SatisfiedReqSum moves when new evidence
// actually satisfies an unknown hypothesis's RequiredEvidence.
func TestHypProgress_SatisfiedReqSumAdvancesOnRelevantEvidence(t *testing.T) {
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{
				ID:     "h1",
				Status: types.HypUnknown,
				RequiredEvidence: []types.Criterion{
					{Kind: types.CritSymbolPresent, Expr: "load_config"},
				},
			},
		},
	}
	// No evidence → SatisfiedReqSum stays 0.
	p0 := computeHypProgress(criterion.Env{IR: ir})
	if p0.SatisfiedReqSum != 0 {
		t.Errorf("baseline SatisfiedReqSum = %d, want 0", p0.SatisfiedReqSum)
	}
	// Evidence mentioning the target symbol → SatisfiedReqSum = 1.
	p1 := computeHypProgress(criterion.Env{
		IR:       ir,
		Evidence: []types.EvidenceItem{{Subject: "load_config", Summary: "reads yaml"}},
	})
	if p1.SatisfiedReqSum != 1 {
		t.Errorf("after relevant evidence SatisfiedReqSum = %d, want 1", p1.SatisfiedReqSum)
	}
	if p0.equals(p1) {
		t.Error("fingerprints should differ after relevant evidence")
	}
}

// TestHypProgress_IrrelevantEvidenceDoesNotAdvance is the bug-fix
// property: evidence unrelated to any unknown hypothesis's
// RequiredEvidence must NOT move SatisfiedReqSum, so the
// fingerprint-match trigger fires on the "explorer fishes in unrelated
// code" stall pattern even when envShape counters grow.
func TestHypProgress_IrrelevantEvidenceDoesNotAdvance(t *testing.T) {
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{
				ID:     "h1",
				Status: types.HypUnknown,
				RequiredEvidence: []types.Criterion{
					{Kind: types.CritSymbolPresent, Expr: "load_config"},
				},
			},
		},
	}
	p0 := computeHypProgress(criterion.Env{IR: ir, Evidence: []types.EvidenceItem{
		{Subject: "alpha"},
	}})
	// Add more evidence, none of it mentioning load_config.
	p1 := computeHypProgress(criterion.Env{IR: ir, Evidence: []types.EvidenceItem{
		{Subject: "alpha"},
		{Subject: "beta"},
		{Subject: "gamma"},
	}})
	if !p0.equals(p1) {
		t.Errorf("SatisfiedReqSum should stay pinned when no evidence satisfies RequiredEvidence (p0=%+v p1=%+v)", p0, p1)
	}
}

// TestHypProgress_DecidedHypothesesSkipped verifies that hypotheses
// already confirmed / rejected / inconclusive don't contribute to
// either field — only unknowns do.
func TestHypProgress_DecidedHypothesesSkipped(t *testing.T) {
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{
				ID:     "h1",
				Status: types.HypConfirmed,
				RequiredEvidence: []types.Criterion{
					{Kind: types.CritContainsSymbol, Expr: "alpha"},
				},
			},
		},
	}
	p := computeHypProgress(criterion.Env{
		IR:       ir,
		Evidence: []types.EvidenceItem{{Subject: "alpha"}},
	})
	if p != (hypProgress{}) {
		t.Errorf("decided-only hypotheses should produce zero fingerprint, got %+v", p)
	}
}

// TestHypProgress_HypothesisWithoutRequiredEvidenceContributesZeroSum
// pins that a hypothesis with no RequiredEvidence (e.g. the
// session-18 explainSetHypothesis) counts toward UnknownCount but
// contributes 0 to SatisfiedReqSum. envShape's DecidedHypotheses
// dimension is the primary stuck signal for that family.
func TestHypProgress_HypothesisWithoutRequiredEvidenceContributesZeroSum(t *testing.T) {
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{ID: "h1", Status: types.HypUnknown, RequiredEvidence: nil},
		},
	}
	p := computeHypProgress(criterion.Env{IR: ir, Evidence: []types.EvidenceItem{
		{Subject: "anything"},
	}})
	if p.UnknownCount != 1 {
		t.Errorf("UnknownCount = %d, want 1", p.UnknownCount)
	}
	if p.SatisfiedReqSum != 0 {
		t.Errorf("SatisfiedReqSum = %d, want 0 for RequiredEvidence=nil", p.SatisfiedReqSum)
	}
}

// TestEnvShape_ReadSetSourcedFromClosure verifies that ReadSetSize is
// read through the BusContext.Mutable.EvidenceClosure() accessor and
// updates when the closure's ReadSet grows.
func TestEnvShape_ReadSetSourcedFromClosure(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("t")}
	a := computeEnvShape(bus, criterion.Env{})
	// ReadSetSize starts at 0.
	if a.ReadSetSize != 0 {
		t.Fatalf("baseline ReadSetSize = %d, want 0", a.ReadSetSize)
	}

	closure := bus.Mutable.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"file1.go": true, "file2.go": true})

	b := computeEnvShape(bus, criterion.Env{})
	if b.ReadSetSize != 2 {
		t.Errorf("after SetReadSet: ReadSetSize = %d, want 2", b.ReadSetSize)
	}
	if a.equals(b) {
		t.Error("shapes should differ after ReadSet growth")
	}
}

func TestStageMapping_AllNodeTypes(t *testing.T) {
	g := types.TaskGraph{}
	cases := []struct {
		nt   types.TaskNodeType
		want types.PipelineStage
	}{
		{types.NodeProbe, types.StageExplore},
		{types.NodeEvidence, types.StageExplore},
		{types.NodeValidate, types.StageExplore},
		{types.NodeReconcile, types.StageExplore},
		{types.NodeExtract, types.StageExtract},
		{types.NodeFinalize, types.StageFinalize},
	}
	for _, c := range cases {
		got, err := stageMapping(g, &types.TaskNode{Type: c.nt}, false)
		if err != nil {
			t.Errorf("nt=%s: %v", c.nt, err)
			continue
		}
		if got != c.want {
			t.Errorf("nt=%s: want %s, got %s", c.nt, c.want, got)
		}
	}
}

func TestGraphState_RequeueToStageExtractOnly(t *testing.T) {
	s := newGraphState(chainGraphWithExtract())
	for _, id := range []string{"n0", "n1", "n2", "n3"} {
		s.markRunning(id)
		s.markDone(id)
	}
	requeued := s.requeueToStage(types.StageExtract, false)
	if got := strings.Join(requeued, ","); got != "n2,n3" {
		t.Fatalf("extract fallback should requeue extract plus finalize only; got %v", requeued)
	}
	if s.nodeStatus("n0") != nodeDone || s.nodeStatus("n1") != nodeDone {
		t.Fatalf("explore nodes should remain done; n0=%s n1=%s", s.nodeStatus("n0"), s.nodeStatus("n1"))
	}
	if s.nodeStatus("n2") != nodeRequeued || s.nodeStatus("n3") != nodeRequeued {
		t.Fatalf("extract/finalize should be requeued; n2=%s n3=%s", s.nodeStatus("n2"), s.nodeStatus("n3"))
	}
}

func TestStageMapping_UnknownTypeFailsLoud(t *testing.T) {
	g := types.TaskGraph{}
	if _, err := stageMapping(g, &types.TaskNode{Type: "bogus"}, false); err == nil {
		t.Error("want error for unknown node type")
	}
}

func TestRenderWindowHint_StructuralOutput(t *testing.T) {
	window := []*types.TaskNode{
		{
			ID: "n0", Type: types.NodeProbe, Objective: "Locate the explorer agent",
			SearchHints: types.SearchHints{KeywordIDs: []string{"k1"}, EntityIDs: []string{"e1"}},
		},
		{
			ID: "n1", Type: types.NodeEvidence, Objective: "Read its tools",
			SearchHints: types.SearchHints{KeywordIDs: []string{"k2"}},
		},
	}
	surfaces := map[string]string{"k1": "explorer", "k2": "tool", "e1": "Explorer"}
	hint := renderWindowHint(window, nil, nil, func(id string) string { return surfaces[id] }, "", "", nil)
	for _, want := range []string{"Locate the explorer agent", "Read its tools", "explorer", "Explorer", "tool"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q\n%s", want, hint)
		}
	}
}

func TestRenderWindowHint_PrependsViolationPreamble(t *testing.T) {
	hint := renderWindowHint(nil, nil, nil, func(string) string { return "" },
		"citation count too low; collect more file:line anchors", "", nil)
	if !strings.Contains(hint, "previous final answer") {
		t.Errorf("missing preamble: %s", hint)
	}
	if !strings.Contains(hint, "citation count too low") {
		t.Errorf("missing violation text: %s", hint)
	}
}

func TestRenderWindowHint_ValidationTargets(t *testing.T) {
	hint := renderWindowHint(nil, nil, []string{"n1_evidence"}, func(string) string { return "" }, "", "", nil)
	if !strings.Contains(hint, "n1_evidence") {
		t.Errorf("expected validation target name in hint: %s", hint)
	}
	if !strings.Contains(hint, "Validation rejected") {
		t.Errorf("expected validation-reject preamble in hint: %s", hint)
	}
}

func TestRenderWindowHint_EmptyAllReturnsEmpty(t *testing.T) {
	if got := renderWindowHint(nil, nil, nil, func(string) string { return "" }, "", "", nil); got != "" {
		t.Errorf("empty inputs should yield empty hint; got %q", got)
	}
}

func TestRenderWindowHintContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	window := []*types.TaskNode{{ID: "n0", Type: types.NodeProbe, Objective: "scan"}}
	if _, err := renderWindowHintContext(ctx, window, nil, nil, func(string) string { return "" }, "", "", nil); err == nil {
		t.Fatal("renderWindowHintContext must report cancellation")
	}
}

// CGEC D2: when the closure has queued RepairReadFile directives,
// the rendered hint must contain the structured "Forced Read List"
// section so the next explore round sees the missing files.
func TestRenderWindowHint_RendersRepairReadFile(t *testing.T) {
	repairs := []types.RepairDirective{
		{
			Kind:      types.RepairReadFile,
			Files:     []string{"internal/orchestrator/topology.go"},
			Rationale: "previous citation pointed at this file but it was unread",
			Origin:    "emit_answer_document.grounder",
		},
	}
	hint := renderWindowHint(nil, nil, nil, func(string) string { return "" }, "", "", repairs)
	if !strings.Contains(hint, "Forced Read List") {
		t.Errorf("expected Forced Read List header, got: %s", hint)
	}
	if !strings.Contains(hint, "internal/orchestrator/topology.go") {
		t.Errorf("expected forced-read file in hint, got: %s", hint)
	}
}

func TestRenderWindowHint_RendersRepairEmitEvidence(t *testing.T) {
	repairs := []types.RepairDirective{
		{
			Kind:      types.RepairEmitEvidence,
			Files:     []string{"internal/types/config.go"},
			Rationale: "already-read same-scope anchor still needs grounded evidence materialized",
			Origin:    "emit_investigation_complete.exact_absence_precedence",
		},
	}
	hint := renderWindowHint(nil, nil, nil, func(string) string { return "" }, "", "", repairs)
	if !strings.Contains(hint, "Evidence Materialization") {
		t.Errorf("expected Evidence Materialization header, got: %s", hint)
	}
	if !strings.Contains(hint, "internal/types/config.go") {
		t.Errorf("expected materialization file in hint, got: %s", hint)
	}
}

// CGEC D2: multiple directive kinds compose into one hint.
func TestRenderWindowHint_RendersMultipleDirectives(t *testing.T) {
	repairs := []types.RepairDirective{
		{Kind: types.RepairReadFile, Files: []string{"a.go"}, Rationale: "missing"},
		{Kind: types.RepairRebindSubject, Subject: "skill_name", Rationale: "answer-shape mismatch"},
	}
	hint := renderWindowHint(nil, nil, nil, func(string) string { return "" }, "", "", repairs)
	if !strings.Contains(hint, "Forced Read List") {
		t.Errorf("missing Forced Read List section: %s", hint)
	}
	if !strings.Contains(hint, "Subject Constraint") {
		t.Errorf("missing Subject Constraint section: %s", hint)
	}
	if !strings.Contains(hint, "skill_name") {
		t.Errorf("missing subject value: %s", hint)
	}
}

func TestTermSurfaceLookup_NilSafe(t *testing.T) {
	fn := termSurfaceLookup(nil)
	if got := fn("any"); got != "" {
		t.Errorf("nil IR lookup should return empty; got %q", got)
	}
}

func TestTermSurfaceLookup_ResolvesKnownID(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			TermGraph: types.TermGraph{
				Canonical: []types.CanonicalTerm{
					{ID: "k1", Surface: "explorer"},
					{ID: "k2", Surface: "Finalizer"},
				},
			},
		},
	}
	fn := termSurfaceLookup(ir)
	if got := fn("k1"); got != "explorer" {
		t.Errorf("k1: want explorer, got %q", got)
	}
	if got := fn("k2"); got != "Finalizer" {
		t.Errorf("k2: want Finalizer, got %q", got)
	}
}

func TestTermSurfaceLookup_ReturnsRawSoftHintCandidate(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			TermGraph: types.TermGraph{
				Canonical: []types.CanonicalTerm{
					{ID: "code:foo", Surface: "Foo"},
				},
			},
		},
	}
	fn := termSurfaceLookup(ir)
	if got := fn("Finalizer"); got != "Finalizer" {
		t.Errorf("raw soft hint candidate should round-trip, got %q", got)
	}
	if got := fn("code:missing"); got != "" {
		t.Errorf("unknown canonical-looking ID should stay hidden, got %q", got)
	}
}

// ── helpers ──────────────────────────────────────────────────────

func idsOf(ns []*types.TaskNode) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}
