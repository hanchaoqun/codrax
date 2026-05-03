package orchestrator

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestStopCondFired_TerminatesInsteadOfHotLooping is the regression
// test for the session-19 orchestrator hot-loop bug surfaced by the
// log-triage eval run.
//
// Before the fix: when EvidencePlan.StopConditions triggered (e.g.
// all_hypotheses_decided on an IR with zero hypotheses), the scheduler
// entered an infinite loop — stopcond.ShouldStop fired, runTaskGraph
// called state.forceCloseExploreWindow() + continue, stepsUsed was
// never incremented, and the criterion.Env was unchanged on the next
// iteration so ShouldStop fired again identically. Logs recorded
// thousands of "stop condition fired" lines per second and the process
// only exited via manual kill.
//
// After the fix: forceFinalizeTriggered latches once the stop path
// has been honored; subsequent iterations skip the stop check and
// fall through to the normal window/finalize dispatch path, with
// stepsUsed advancing on each dispatch so the outer budget always
// drains.
//
// The test builds a minimal IR whose StopConditions fire immediately
// (all_hypotheses_decided + empty HypothesisSet → satisfied on the
// first check), runs a mocked pipeline, and asserts that Run returns
// within a bounded wall-clock budget. Without the fix this test would
// spin forever.
func TestStopCondFired_TerminatesInsteadOfHotLooping(t *testing.T) {
	ir := &types.AnalysisIR{
		Version: types.AnalysisIRVersion,
		RequestModel: types.RequestModel{
			Language: "en",
			Intent:   types.IntentExplain,
		},
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{
				{ID: "n0", Type: types.NodeEvidence, Objective: "collect"},
				{ID: "n1", Type: types.NodeFinalize, Objective: "render"},
			},
			Edges: []types.TaskEdge{
				{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
			},
			ExecutionPolicy: types.ExecutionPolicy{
				MaxParallelism: 1,
				RetryBudget:    1,
				CriticalPath:   []string{"n0", "n1"},
			},
		},
		EvidencePlan: types.EvidencePlan{
			Budget: types.EvidenceBudget{MaxReactIters: 10, MaxToolCalls: 20},
			// The stop condition fires on the very first scheduler
			// iteration because HypothesisSet is empty → "all
			// hypotheses decided" is vacuously true.
			StopConditions: []types.StopCondition{
				{Kind: types.CritAllHypothesesDecided},
			},
		},
		AnswerContract: types.AnswerContract{
			Language:            "en",
		},
	}

	var finalizeCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			// Should not be invoked — stop fires before any explore
			// window dispatches — but keep a valid handler for safety.
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "OK",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	done := make(chan struct{})
	var busErr error
	go func() {
		_, busErr = o.Run("test", "/tmp/repo", "main")
		close(done)
	}()

	select {
	case <-done:
		// Fast path — expected post-fix.
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s — stop-condition hot-loop regression")
	}
	if busErr != nil {
		t.Fatalf("Run returned error: %v", busErr)
	}
	if finalizeCalls == 0 {
		t.Error("expected finalize to be dispatched after stop-condition force-close; got 0 calls")
	}
}

// TestStopCondFired_RetryAfterFinalizeFailure_UsesExplorer is the
// second-order regression that the forceFinalizeTriggered latch
// specifically protects: once stop has fired and finalize proceeds,
// a finalize contract violation must be able to requeue explore nodes
// AND have them actually re-dispatched — the latch must NOT re-trigger
// forceCloseExploreWindow on iter 2+ and re-close the retries.
//
// Pre-fix (hypothetical no-latch): iter 1 stop fires + forceClose +
// finalize fails + requeue fin/explore; iter 2 stop fires AGAIN (env
// unchanged) + forceClose RE-CLOSES the requeued explore nodes before
// they can be dispatched; explorer never runs on retry → retries are
// semantically dead.
//
// Post-fix: latch set on iter 1 means iter 2 skips the stop check;
// readyExplorerWindow returns the requeued nodes; explorer dispatches
// normally; pipeline gets a real retry cycle. The assertion here is
// explorerCalls ≥ 1 after a forced stop + failed finalize.
func TestStopCondFired_RetryAfterFinalizeFailure_UsesExplorer(t *testing.T) {
	ir := &types.AnalysisIR{
		Version: types.AnalysisIRVersion,
		RequestModel: types.RequestModel{
			Language: "en",
			Intent:   types.IntentExplain,
		},
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{
				{ID: "n0", Type: types.NodeEvidence, Objective: "collect"},
				{ID: "n1", Type: types.NodeFinalize, Objective: "render"},
			},
			Edges: []types.TaskEdge{
				{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
			},
			ExecutionPolicy: types.ExecutionPolicy{
				MaxParallelism: 1,
				// Budget of 2 gives the retry path room to run explore
				// at least once before exhausting.
				RetryBudget:  2,
				CriticalPath: []string{"n0", "n1"},
			},
		},
		EvidencePlan: types.EvidencePlan{
			Budget: types.EvidenceBudget{MaxReactIters: 10, MaxToolCalls: 20},
			StopConditions: []types.StopCondition{
				{Kind: types.CritAllHypothesesDecided},
			},
		},
		AnswerContract: types.AnswerContract{
			Language:            "en",
			// Answer will never contain this sentinel — forces contract
			// violation on every finalize dispatch so the retry path
			// engages.
			MustInclude: []string{"SENTINEL_NEVER_IN_ANSWER"},
		},
	}

	var explorerCalls, finalizeCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "answer body without the sentinel",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	// Block 3 (architecture overhaul 2026-05-02): the test exercises
	// the latch's ability to keep explore running on retry. The
	// default fallback policy maps ViolMustInclude→FinalizerOnly,
	// which by design does NOT requeue explore (saves wall-clock
	// when shape errors don't need new evidence). Override the
	// policy for THIS test so we can keep verifying the latch's
	// stopcond-non-refire behaviour with explorer dispatching on
	// retry. The override is process-global; we restore defaults
	// at cleanup so other tests run with the production policy.
	t.Cleanup(func() { SetFallbackPolicyOverrides(nil) })
	SetFallbackPolicyOverrides(map[string]string{
		string(types.ViolMustInclude): string(FallbackBackToExplore),
	})

	done := make(chan struct{})
	go func() {
		_, _ = o.Run("test", "/tmp/repo", "main")
		close(done)
	}()

	select {
	case <-done:
		// Expected — retry budget drains in finite time.
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s under stop+retry scenario")
	}

	if finalizeCalls < 2 {
		t.Errorf("finalize calls = %d, want ≥ 2 (initial + retry)", finalizeCalls)
	}
	if explorerCalls < 1 {
		t.Errorf("explorer calls = %d, want ≥ 1 (latch must NOT re-force-close the requeued explore node)", explorerCalls)
	}
}
