package orchestrator

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// explore_backtrack_exhaustion_e2e_test.go — §40.43 F-orch 三轮复核 finding
// Q. After a finalize contract failure is routed back_to_explore, the arm
// clears Signals.HasEnoughFacts (§40.43 R2) so the requeued reconcile /
// validate node waits for the explorer's fresh completion. When the
// re-dispatched explorer does NOT re-earn the signal (it exits without a
// fresh completion decision, or its fact-retry lane is exhausted), the
// untouched code left the node blocked on its has_enough_facts entry
// condition, broke out through the blocked-DAG forced finalize, skipped
// the forced dispatch because the REJECTED draft was still retained in
// lastFinalize, and shipped that exact contract-rejected draft with no
// caveat.
//
// Ruling (typed release instead of a stale door): the backtrack is consumed
// either by a fresh accepted completion OR by the explorer's exhaustion —
// when the re-opened explore window closes without a fresh accepted
// completion the scheduler records a typed ExploreBacktrackExhausted
// decision (advances the completion generation, releasing the veto through
// the same premise the hard arm reads) and restores the accepted-closure
// signal from the retained closure, so reconcile proceeds, the finalizer
// re-runs with the violations as repair context, and termination flows
// through the existing accept-with-caveat lanes. Structural backstop: no
// terminal exit ships a retained contract-rejected draft without the
// contract caveats.

const exploreBacktrackBareDraft = "answer body without the sentinel"

// exploreBacktrackE2E runs the reconcile-template scheduler with a finalizer
// that fails a hard must_include term every round and the explorer
// behaviour `explore` (called with the 1-based dispatch ordinal and the
// dispatch context). It returns the bus plus the call counts.
func exploreBacktrackE2E(t *testing.T, retryBudget, maxSteps int, explore func(call int, ctx *types.AgentContext) *agent.StageOutput) (*types.BusContext, int, int) {
	t.Helper()
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
			ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: retryBudget, CriticalPath: []string{"n0", "n1", "n2"}},
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
			return explore(explorerCalls, ctx), nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: exploreBacktrackBareDraft}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(maxSteps)
	o.SetMaxUpstreamFallbacksPerRun(2)

	done := make(chan struct{})
	var bus *types.BusContext
	go func() {
		bus, _ = o.Run("explore backtrack exhaustion", "/tmp/repo", "main")
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
	return bus, explorerCalls, finalizeCalls
}

func exploreEvidence(call int) *agent.StageOutput {
	return &agent.StageOutput{
		MissingPiece: types.MissingFacts,
		EvidenceItems: []types.EvidenceItem{
			{ID: "ev", Subject: "sym", Source: "test.go", LineStart: call, AnchorKind: types.AnchorDefinition},
		},
	}
}

// requireCaveatedDraft asserts the shipped answer is the rejected draft's
// body WITH the contract caveats — never the bare rejected draft.
func requireCaveatedDraft(t *testing.T, bus *types.BusContext) string {
	t.Helper()
	result := bus.Mutable.Result()
	if strings.TrimSpace(result) == "" {
		t.Fatal("Run produced no result")
	}
	if strings.TrimSpace(result) == exploreBacktrackBareDraft {
		t.Fatalf("the customer received the bare contract-rejected draft with no caveat:\n%s", result)
	}
	if !strings.Contains(result, exploreBacktrackBareDraft) {
		t.Fatalf("the shipped answer must retain the draft body, got:\n%s", result)
	}
	if !strings.Contains(result, "Coverage on some dimensions of the answer may be incomplete") &&
		!strings.Contains(result, "答案在某些维度的覆盖度可能不充分") {
		t.Fatalf("the shipped answer must carry the contract caveat for the unresolved must_include violation, got:\n%s", result)
	}
	return result
}

// PIN (red on 0139bca6b: finalizer once, profile blocked_dag, bare rejected
// draft): the explorer never re-earns the signal after the backtrack. The
// scheduler records the exhaustion decision, reconcile proceeds from the
// retained closure, the finalizer re-runs and the run terminates through
// the existing class-cap accept-with-caveat lane.
func TestE2E_ExploreBacktrackExhausted_ExplorerNeverReearns_ShipsCaveatedDraft(t *testing.T) {
	bus, explorerCalls, finalizeCalls := exploreBacktrackE2E(t, 3, 20, func(call int, ctx *types.AgentContext) *agent.StageOutput {
		if call == 1 {
			ctx.Mutable.SetInvestigationComplete("model completed investigation")
		}
		return exploreEvidence(call)
	})
	if finalizeCalls != 2 {
		t.Fatalf("finalize calls = %d, want 2 (the finalizer re-runs with the violations as repair context after the exhaustion release)", finalizeCalls)
	}
	if explorerCalls != 2 {
		t.Fatalf("explorer calls = %d, want 2 (initial + the one backtrack)", explorerCalls)
	}
	requireCaveatedDraft(t, bus)
	if !bus.Signals.HasEnoughFacts {
		t.Fatal("the accepted-closure signal must be restored from the retained closure by the exhaustion release")
	}
	if profile := bus.Mutable.TerminationProfile(); profile != nil && profile.Kind == types.TerminationBlockedDAG {
		t.Fatalf("the run must not terminate through the blocked-DAG forced finalize, got %+v", profile)
	}
}

// PIN (red on 0139bca6b): the re-dispatched explorer asks for a fact retry
// with the retry budget already spent ("explore requested fact retry but
// retry budget is exhausted; continuing toward finalize") — the second
// exhaustion lane. Same release, same caveated termination (here through
// the template retry-budget lane).
func TestE2E_ExploreBacktrackExhausted_FactRetryBudgetSpent_ShipsCaveatedDraft(t *testing.T) {
	bus, explorerCalls, finalizeCalls := exploreBacktrackE2E(t, 1, 20, func(call int, ctx *types.AgentContext) *agent.StageOutput {
		if call == 1 {
			ctx.Mutable.SetInvestigationComplete("model completed investigation")
			return exploreEvidence(call)
		}
		out := exploreEvidence(call)
		out.RetryHint = "need one more file before deciding"
		return out
	})
	if finalizeCalls != 2 {
		t.Fatalf("finalize calls = %d, want 2", finalizeCalls)
	}
	if explorerCalls != 2 {
		t.Fatalf("explorer calls = %d, want 2 (the fact retry is refused because the budget is spent)", explorerCalls)
	}
	requireCaveatedDraft(t, bus)
	if !bus.Signals.HasEnoughFacts {
		t.Fatal("the accepted-closure signal must be restored from the retained closure by the exhaustion release")
	}
}

// Existing lane (green before and after): the re-dispatched explorer
// re-earns the closure with a fresh completion decision — the veto is
// released by the fresh generation, not by an exhaustion decision.
func TestE2E_ExploreBacktrackFreshCompletion_ReleasesByGeneration(t *testing.T) {
	bus, explorerCalls, finalizeCalls := exploreBacktrackE2E(t, 3, 20, func(call int, ctx *types.AgentContext) *agent.StageOutput {
		ctx.Mutable.SetInvestigationComplete("model completed investigation")
		return exploreEvidence(call)
	})
	if finalizeCalls != 2 || explorerCalls != 2 {
		t.Fatalf("finalize=%d explorer=%d, want 2/2", finalizeCalls, explorerCalls)
	}
	requireCaveatedDraft(t, bus)
	assertNoExploreBacktrackExhaustionDecision(t, bus)
}

// PIN (red on 0139bca6b): the structural backstop. The first completion
// carries no reason, so after the backtrack there is no retained closure to
// proceed from; the DAG stays blocked and the run exits through the
// blocked-DAG break with the rejected draft still retained. The draft must
// ship WITH the contract caveats.
func TestE2E_BlockedDAGExitWithRetainedRejectedDraft_CarriesContractCaveats(t *testing.T) {
	bus, _, finalizeCalls := exploreBacktrackE2E(t, 3, 20, func(call int, ctx *types.AgentContext) *agent.StageOutput {
		if call == 1 {
			ctx.Mutable.SetInvestigationComplete("")
		}
		return exploreEvidence(call)
	})
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1 (no retained closure: nothing to proceed from)", finalizeCalls)
	}
	profile := bus.Mutable.TerminationProfile()
	if profile == nil || profile.Kind != types.TerminationBlockedDAG {
		t.Fatalf("fixture: this lane must exit through the blocked-DAG break, got %+v", profile)
	}
	requireCaveatedDraft(t, bus)
}

// PIN (red on 0139bca6b): the step-drain exit. The budget runs out right
// after the backtrack re-dispatched the explorer, so the loop leaves with
// the rejected draft retained — same backstop.
func TestE2E_StepDrainExitWithRetainedRejectedDraft_CarriesContractCaveats(t *testing.T) {
	bus, _, finalizeCalls := exploreBacktrackE2E(t, 3, 3, func(call int, ctx *types.AgentContext) *agent.StageOutput {
		if call == 1 {
			ctx.Mutable.SetInvestigationComplete("model completed investigation")
		}
		return exploreEvidence(call)
	})
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1 (the step budget drains before the re-run)", finalizeCalls)
	}
	requireCaveatedDraft(t, bus)
}
