package orchestrator

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// explore_fork_retained_closure_e2e_test.go — §40.43 F-orch 四轮复核 finding
// W at Orchestrator.Run level: a multi-topic arch_explain graph whose two
// evidence siblings (`_t{i}` per SubTopic, so each window owns its own
// scoped evidence lane and the collective handoff tracker cannot converge on
// one sibling alone) are dispatched as parallel explore forks; mechanism
// sub-topics wait for every sibling, so both forks merge in declaration
// order. Round 1: sibling A completes "closure round 1", sibling B decides
// nothing. The finalizer fails a hard must_include term → back_to_explore
// (ResetInvestigationComplete; the retained lane keeps closure round 1).
// Round 2: A completes "closure round 2", B decides nothing again — B was
// forked with the parent's retained copy "closure round 1", and MergeExploreFork
// used to write that copy back AFTER A's merge, reverting the retained lane.
// Failure 2 → back_to_explore again (class cap 0). Round 3: neither sibling
// completes, the window closes with reconcile blocked, and the exhaustion
// release proceeds from the RETAINED closure: on 64ceb5b06 that was "closure
// round 1" (the stale probe copy); it must be "closure round 2".
//
// PIN (red on 64ceb5b06): the release's typed decision records the
// later-accepted closure; the run terminates through the P6 hard cap on the
// third failure with the caveated draft.
func TestE2E_ParallelExploreFork_NonCompletingSiblingKeepsAcceptedRetainedClosure(t *testing.T) {
	restoreClass := sameErrorClassRetryCap()
	t.Cleanup(func() {
		SetSameErrorClassRetryCap(restoreClass)
		SetSoftViolationKinds(nil, nil)
		SetFallbackPolicyOverrides(nil)
	})
	SetSameErrorClassRetryCap(0)
	SetSoftViolationKinds(nil, []string{string(types.ViolMustInclude)})
	SetFallbackPolicyOverrides(map[string]string{string(types.ViolMustInclude): string(FallbackBackToExplore)})

	reconcile := templateReconcileNode(t, types.ScenarioArchitectureExplain)
	reconcile.ID = "n1"
	reconcile.Inputs, reconcile.Outputs = nil, nil
	ir := &types.AnalysisIR{
		Version: types.AnalysisIRVersion,
		RequestModel: types.RequestModel{
			Language:      "en",
			Intent:        types.IntentExplain,
			Scenario:      types.ScenarioArchitectureExplain,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			SubTopics: []types.SubTopic{
				{Summary: "runtime invocation", Entities: []string{"SubAgentRuntime"}},
				{Summary: "registered agent gate", Entities: []string{"SubAgentRegistry"}},
			},
		},
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{
				{ID: "n0_t0", Type: types.NodeEvidence, Objective: "runtime invocation", SearchHints: types.SearchHints{EntityIDs: []string{"e0"}}},
				{ID: "n0_t1", Type: types.NodeEvidence, Objective: "registered agent gate", SearchHints: types.SearchHints{EntityIDs: []string{"e1"}}},
				reconcile,
				{ID: "n2", Type: types.NodeFinalize, Objective: "render"},
			},
			Edges: []types.TaskEdge{
				{From: "n0_t0", To: "n1", EdgeType: types.EdgeHardDependency},
				{From: "n0_t1", To: "n1", EdgeType: types.EdgeHardDependency},
				{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
			},
			ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 2, RetryBudget: 5, CriticalPath: []string{"n0_t0", "n1", "n2"}},
		},
		AnswerContract: types.AnswerContract{Language: "en", MustInclude: []string{"FORCE_RETRY"}},
	}

	var mu sync.Mutex
	roundsByKey := map[string]int{}
	finalizeCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			mu.Lock()
			roundsByKey[ctx.ExploreDispatchKey]++
			round := roundsByKey[ctx.ExploreDispatchKey]
			mu.Unlock()
			// Sibling A decides completion on rounds 1 and 2 only; sibling B
			// is a window-scoped probe that never decides.
			if ctx.ExploreDispatchKey == "n0_t0" && round <= 2 {
				ctx.Mutable.SetInvestigationComplete("closure round " + string(rune('0'+round)))
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{
					{ID: "ev-" + ctx.ExploreDispatchKey, Subject: "sym", Source: "test.go", LineStart: round, AnchorKind: types.AnchorDefinition},
				},
			}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			mu.Lock()
			finalizeCalls++
			mu.Unlock()
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: exploreBacktrackBareDraft}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.SetMaxSteps(40)
	o.SetMaxUpstreamFallbacksPerRun(3)
	o.SetFinalizeRepairHardCap(2)

	done := make(chan struct{})
	var bus *types.BusContext
	go func() {
		bus, _ = o.Run("explain the runtime invocation and the registered agent gate", "/tmp/repo", "main")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not terminate within 10s")
	}
	if bus == nil || bus.Mutable == nil {
		t.Fatal("Run returned no bus")
	}
	mu.Lock()
	defer mu.Unlock()
	if roundsByKey["n0_t0"] < 3 || roundsByKey["n0_t1"] < 3 {
		t.Fatalf("fixture: both siblings must be dispatched on every round (forks merge in declaration order), got %v", roundsByKey)
	}
	if finalizeCalls != 3 {
		t.Fatalf("finalize calls = %d, want 3 (two backtracks, then the P6 hard cap ships on failure 3)", finalizeCalls)
	}
	d := bus.Mutable.LastExploreBacktrackExhausted()
	if d == nil {
		t.Fatalf("fixture: round 3 closes without a fresh completion — the exhaustion release must have recorded its decision (profile=%+v)", bus.Mutable.TerminationProfile())
	}
	if d.RetainedClosureReason != "closure round 2" {
		t.Fatalf("the release proceeded from %q; the retained lane must be the most recently ACCEPTED closure (\"closure round 2\") — a non-completing sibling merged after the completing fork must never write its fork-time copy back", d.RetainedClosureReason)
	}
	if got := bus.Mutable.StableInvestigationCompleteReason(); got != "closure round 2" {
		t.Fatalf("StableInvestigationCompleteReason = %q, want closure round 2", got)
	}
	if result := bus.Mutable.Result(); !strings.Contains(result, exploreBacktrackBareDraft) || strings.TrimSpace(result) == exploreBacktrackBareDraft {
		t.Fatalf("the run ships the draft WITH caveats, got:\n%s", result)
	}
}
