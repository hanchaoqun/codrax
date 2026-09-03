package orchestrator

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// accepted_closure_reconcile_signal_test.go — §40.43 R2 (fold-in round
// three, finding D). The reconcile auto-complete's signal door
// (`!acceptedClosureEnough && !env.Signals.HasEnoughFacts &&
// !o.busCtx.Signals.HasEnoughFacts`) bypassed the F14 premise: every
// accepted-closure exit sets BusContext.Signals.HasEnoughFacts=true, nothing
// cleared it on the FallbackBackToExplore arm, and buildEnv copies it into
// the criterion env each iteration — so in every production state where the
// backtrack veto was in force the door was open and the requeued reconcile
// node auto-completed from the pre-backtrack state. Root-cause fix: the arm
// clears the signal when it binds the backtrack; the explorer re-earns it on
// a fresh completion.

// templateReconcileNode returns the real reconcile node of a compiler
// template (arch_explain: entry condition has_enough_facts; root_cause: no
// entry condition, no success criteria).
func templateReconcileNode(t *testing.T, scenario types.Scenario) types.TaskNode {
	t.Helper()
	out := compiler.Compile(types.RequestModel{Scenario: scenario, Language: "en"}, budget.BudgetSignals{})
	for _, n := range out.TaskGraph.Nodes {
		if n.Type == types.NodeReconcile {
			return n
		}
	}
	t.Fatalf("template %s has no reconcile node — the pin lost its subject", scenario)
	return types.TaskNode{}
}

// replayExploreContractBacktrackArm mirrors the FallbackBackToExplore arm of
// runReadSchedulerLoop after AdvanceRepairExecutionPlan chose it: populate
// the (explore-owned) retry state, requeue, reset the Mutable for the
// explore target (binds the backtrack epoch), clear the accepted-closure
// signal, then the completion latch reset the next iteration performs.
// The arm's statement order and the signal clear are pinned to production
// by TestRunReadSchedulerLoop_FallbackArmsPopulateBeforeReset and
// TestHardArmMutableCarrierCensus_SignalResetSiteIsTheBacktrackArm.
func replayExploreContractBacktrackArm(t *testing.T, o *Orchestrator) {
	t.Helper()
	mut := o.busCtx.Mutable
	populateRetryState(mut, contractResultOf([]types.Violation{vFacet("diagram_spine")}), retryStateAttempt(mut))
	mut.ResetForFallback(types.FallbackResetTargetExplore)
	o.busCtx.Signals.HasEnoughFacts = false
	mut.ResetInvestigationComplete()
	if !acceptedClosureHasActiveExploreContractBacktrack(mut) {
		t.Fatal("fixture: the replayed arm must leave a bound explore backtrack in force")
	}
}

// PIN (red on 0d9a142e4 through the census reset-site check and the e2e
// below): production backtrack sequence with Signals.HasEnoughFacts=true
// beforehand and the real template reconcile shapes → after the arm the
// signal is false and the node is not auto-completed while the veto holds;
// after the explorer's fresh completion (SetInvestigationComplete + signal
// raised) it completes.
func TestReconcileAutoComplete_ExploreBacktrackClearsSignalUntilFreshCompletion(t *testing.T) {
	for _, scenario := range []types.Scenario{types.ScenarioArchitectureExplain, types.ScenarioRootCause} {
		t.Run(string(scenario), func(t *testing.T) {
			node := templateReconcileNode(t, scenario)
			mut := types.NewMutableState("reconcile signal after backtrack")
			o := &Orchestrator{busCtx: &types.BusContext{
				Mutable:       mut,
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}}
			// Every accepted-closure exit raises the signal alongside the
			// completion mark.
			mut.SetInvestigationComplete("first accepted closure")
			o.busCtx.Signals.HasEnoughFacts = true
			if !o.shouldAutoCompleteReadyReconcileNode(&node, criterion.Env{Signals: o.busCtx.Signals}) {
				t.Fatal("fixture: before the backtrack the accepted closure auto-completes the reconcile node")
			}

			replayExploreContractBacktrackArm(t, o)

			if o.busCtx.Signals.HasEnoughFacts {
				t.Fatal("after the explore backtrack arm the accepted-closure signal must be false")
			}
			env := criterion.Env{Signals: o.busCtx.Signals} // buildEnv copies the bus signals each iteration
			if o.acceptedClosureCanSatisfyReconcileEnoughFacts() {
				t.Fatal("the retained pre-backtrack closure must not satisfy the reconcile node while the veto holds")
			}
			if o.shouldAutoCompleteReadyReconcileNode(&node, env) {
				t.Fatalf("%s: the requeued reconcile node must not auto-complete from the pre-backtrack state while the veto holds", scenario)
			}
			graph := types.TaskGraph{Nodes: []types.TaskNode{node}}
			state := newGraphState(graph)
			remaining := o.autoCompleteReadyReconcileNodes(state, []*types.TaskNode{&state.graph.Nodes[0]}, env)
			if len(remaining) != 1 || state.nodeStatus(node.ID) != nodePending {
				t.Fatalf("the reconcile node must stay pending until fresh facts (remaining=%d status=%v)", len(remaining), state.nodeStatus(node.ID))
			}
			if len(node.EntryConditions) > 0 {
				// arch_explain: the entry condition blocks the node exactly as
				// in an initial run until the explorer raises the signal.
				if ready, _ := state.readyExplorerWindow(env); len(ready) != 0 {
					t.Fatalf("the has_enough_facts entry condition must keep the node out of the ready window, got %d ready", len(ready))
				}
			}

			// The explorer's fresh completion: completion mark + signal.
			mut.SetInvestigationComplete("fresh completion after the backtrack")
			o.busCtx.Signals.HasEnoughFacts = true
			env = criterion.Env{Signals: o.busCtx.Signals}
			if acceptedClosureHasActiveExploreContractBacktrack(mut) {
				t.Fatal("fixture: the fresh completion consumes the veto")
			}
			if !o.shouldAutoCompleteReadyReconcileNode(&node, env) {
				t.Fatal("after the fresh accepted completion the reconcile node must auto-complete")
			}
			remaining = o.autoCompleteReadyReconcileNodes(state, []*types.TaskNode{&state.graph.Nodes[0]}, env)
			if len(remaining) != 0 || state.nodeStatus(node.ID) != nodeDone {
				t.Fatalf("the reconcile node must complete once the fresh closure is in force (remaining=%d status=%v)", len(remaining), state.nodeStatus(node.ID))
			}
		})
	}
}

// PIN (red on 0d9a142e4): the whole scheduler loop. A finalize contract
// failure routed back_to_explore requeues the evidence and reconcile nodes;
// the re-dispatched explorer exits WITHOUT a fresh completion. With the
// signal cleared the requeued reconcile node (arch_explain shape: entry
// condition has_enough_facts) stays blocked exactly as in an initial run —
// it is not auto-completed from the pre-backtrack state, the finalizer is
// NOT re-dispatched through it (one finalizer call), and the loop
// terminates through the blocked-DAG forced finalize (no deadlock). On the
// untouched code the stale signal auto-completed the reconcile node from
// the pre-backtrack state, the finalizer was dispatched a second time and
// failed again, and the run ended through the same-error-class retry cap
// with the signal still true and no termination profile.
func TestE2E_ExploreBacktrackClearsEnoughFactsSignal_ReconcileWaitsForFreshFacts(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolMustInclude)})
	t.Cleanup(func() { SetFallbackPolicyOverrides(nil) })
	SetFallbackPolicyOverrides(map[string]string{
		string(types.ViolMustInclude): string(FallbackBackToExplore),
	})

	reconcile := templateReconcileNode(t, types.ScenarioArchitectureExplain)
	reconcile.ID = "n1"
	reconcile.Inputs, reconcile.Outputs = nil, nil
	ir := &types.AnalysisIR{
		Version:      types.AnalysisIRVersion,
		RequestModel: types.RequestModel{Language: "en", Intent: types.IntentExplain},
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{
				{ID: "n0", Type: types.NodeEvidence, Objective: "collect"},
				reconcile,
				{ID: "n2", Type: types.NodeFinalize, Objective: "render"},
			},
			Edges: []types.TaskEdge{
				{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
				{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
			},
			ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 3, CriticalPath: []string{"n0", "n1", "n2"}},
		},
		AnswerContract: types.AnswerContract{Language: "en", MustInclude: []string{"FORCE_RETRY"}},
	}

	var explorerCalls, finalizeCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			if explorerCalls == 1 {
				// The first window closes with an accepted completion — the
				// exit that raises Signals.HasEnoughFacts.
				ctx.Mutable.SetInvestigationComplete("model completed investigation")
			}
			// Every later dispatch exits without a fresh completion decision.
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{
					{ID: "ev", Subject: "sym", Source: "test.go", LineStart: explorerCalls, AnchorKind: types.AnchorDefinition},
				},
			}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "answer body without the sentinel"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMaxUpstreamFallbacksPerRun(2)

	done := make(chan struct{})
	var bus *types.BusContext
	go func() {
		bus, _ = o.Run("reconcile waits for fresh facts after a backtrack", "/tmp/repo", "main")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not terminate within 10s — the requeued reconcile node must not deadlock the loop")
	}
	if bus == nil || bus.Mutable == nil {
		t.Fatal("Run returned no bus")
	}
	if explorerCalls != 2 {
		t.Fatalf("explorer calls = %d, want 2 (initial + the one backtrack; the stale signal must not auto-complete the reconcile node and cycle the finalizer)", explorerCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want exactly 1 — a reconcile node auto-completed from the pre-backtrack state re-dispatches the finalizer against the stale closure", finalizeCalls)
	}
	if bus.Signals.HasEnoughFacts {
		t.Fatal("the explorer never re-earned the signal after the backtrack; it must stay false")
	}
	profile := bus.Mutable.TerminationProfile()
	if profile == nil || profile.Kind != types.TerminationBlockedDAG {
		t.Fatalf("the requeued reconcile node blocks on has_enough_facts exactly as in an initial run and the loop exits through the blocked-DAG forced finalize; got termination profile %+v", profile)
	}
}
