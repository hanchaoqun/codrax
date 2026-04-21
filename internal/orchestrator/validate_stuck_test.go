package orchestrator

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestValidateStuck_SameShape_EscalatesToInconclusive is the
// regression test for the hypothesis-validate infinite-loop bug.
//
// Before the shape-guard refactor: when a validate node's SuccessCriteria
// (e.g. all_hypotheses_decided) failed on a hypothesis that the explorer
// provably could not resolve (classic case: Java trace attached to a Go
// repo — no UserService.java exists in-repo for the explorer to read),
// the scheduler requeued the upstream evidence nodes, explorer re-ran,
// found the same nothing, validate failed again on identical env,
// requeue... loop ran until step budget drained (~15-30 minutes of
// wall-clock with default limits; observed 28+ min in eval).
//
// After the shape-guard refactor: the scheduler captures the envShape
// at the first SC failure. On the second failure, if shape is identical
// (re-investigation produced no new Evidence / ToolResults / ReadSet),
// it calls injectInconclusiveForStuckHypotheses to mark still-unknown
// hypotheses as HypInconclusive with a stuck-signal rationale, marks
// the validate node done, and lets finalize ship with a caveat.
//
// Test construction: explorer mock returns without any side-effects so
// every validate-SC evaluation sees an identical env. Expect Run to
// terminate within 5s AND the still-unknown hypothesis to land as
// HypInconclusive in the emitted verdict set.
func TestValidateStuck_SameShape_EscalatesToInconclusive(t *testing.T) {
	ir := buildStuckValidateIR()

	var explorerCalls, finalizeCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			// No side-effects: no evidence appended, no tool results,
			// nothing changes. Env cursor stays frozen across dispatches.
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "answer shipped despite stuck hypothesis",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	done := make(chan *types.BusContext)
	go func() {
		bus, _ := o.Run("test stuck hypothesis", "/tmp/repo", "main")
		done <- bus
	}()

	var bus *types.BusContext
	select {
	case bus = <-done:
		// Expected: shape-guard escape fires on second validate-SC
		// failure, scheduler marks validate done and proceeds to
		// finalize.
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s — shape-guard escape regression")
	}
	if bus == nil {
		t.Fatal("nil BusContext returned")
	}

	// The validate node must have failed SC at least once (forcing the
	// shape-guard check) and finalize must have run.
	if finalizeCalls == 0 {
		t.Error("finalize was never dispatched after shape-guard escape")
	}

	// Explorer should have run a bounded number of times — at least
	// once (to establish the initial env shape) and at most a few
	// (before shape-guard triggers). A runaway count indicates the
	// escape hatch didn't engage.
	if explorerCalls == 0 {
		t.Error("explorer never ran — shape-guard fired too aggressively")
	}
	if explorerCalls > 5 {
		t.Errorf("explorer ran %d times — shape-guard escape did not fire (regression)", explorerCalls)
	}

	// The stuck hypothesis must have landed as HypInconclusive with
	// the stuck-signal rationale.
	verdicts := bus.Mutable.EmittedHypothesisVerdicts()
	foundInconclusive := false
	for _, v := range verdicts {
		if v.HypothesisID == "h1" && v.Status == types.HypInconclusive {
			foundInconclusive = true
			if !strings.Contains(v.Rationale, "stuck") {
				t.Errorf("rationale missing stuck signal: %q", v.Rationale)
			}
			break
		}
	}
	if !foundInconclusive {
		t.Errorf("h1 not marked HypInconclusive after stuck escape; verdicts=%+v", verdicts)
	}
}

// TestValidateStuck_ShapeChanged_RequeuesNormally pins the NON-escape
// path: when the env shape DOES change between validate-SC failures
// (explorer actually pulled new evidence / read new files), the
// scheduler must run the ORIGINAL requeue path — not the stuck
// escape. Guards against false positives where the shape-guard would
// pre-emptively give up on hypotheses that just needed one more
// explore round.
//
// The mock explorer appends a fresh evidence item each time it runs
// so the envShape advances. Expect multiple explorer dispatches (one
// per requeue cycle) and the hypothesis to eventually reach
// HypInconclusive ONLY via retry-budget exhaustion (or normal
// termination via step budget), NOT via the shape-guard escape.
func TestValidateStuck_ShapeChanged_RequeuesNormally(t *testing.T) {
	ir := buildStuckValidateIR()

	var explorerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// Append a DIFFERENT evidence item each call so the envShape
			// cursor advances between validate-SC failures — this is
			// the "still making progress, keep retrying" case.
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{
					{
						Subject: "item-" + string(rune('0'+explorerCalls)),
						Source:  "test.go",
					},
				},
			}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "test answer",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(15)

	done := make(chan struct{})
	go func() {
		_, _ = o.Run("test progress", "/tmp/repo", "main")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s")
	}

	// When shape advances, the normal requeue path runs multiple
	// times before finally exhausting retry budget. Assert explorer
	// dispatched more than once — if shape-guard had fired on the
	// first failure despite shape change, explorerCalls would be ≤ 1.
	if explorerCalls < 2 {
		t.Errorf("explorer ran %d times — shape-guard fired too aggressively when shape was changing", explorerCalls)
	}
}

// buildStuckValidateIR assembles the canonical stuck-validate IR used
// by both tests: a 4-node DAG (probe → evidence → validate → finalize)
// where the validate node gates on all_hypotheses_decided and the
// HypothesisSet contains one unresolvable HypUnknown entry.
func buildStuckValidateIR() *types.AnalysisIR {
	return &types.AnalysisIR{
		Version: types.AnalysisIRVersion,
		RequestModel: types.RequestModel{
			Language: "en",
			Intent:   types.IntentRootCause,
		},
		HypothesisSet: []types.Hypothesis{
			{
				ID:        "h1",
				Statement: "the bug is in foreign code",
				Status:    types.HypUnknown,
			},
		},
		TaskGraph: types.TaskGraph{
			Nodes: []types.TaskNode{
				{ID: "n0", Type: types.NodeEvidence, Objective: "collect"},
				{
					ID:        "n1",
					Type:      types.NodeValidate,
					Objective: "validate hypothesis decision",
					SuccessCriteria: []types.Criterion{
						{Kind: types.CritAllHypothesesDecided},
					},
					MaxRetries: 5,
				},
				{ID: "n2", Type: types.NodeFinalize, Objective: "render"},
			},
			Edges: []types.TaskEdge{
				{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
				{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
				{From: "n1", To: "n0", EdgeType: types.EdgeValidationFeedback},
			},
			ExecutionPolicy: types.ExecutionPolicy{
				MaxParallelism: 1,
				RetryBudget:    5,
				CriticalPath:   []string{"n0", "n1", "n2"},
			},
		},
		EvidencePlan: types.EvidencePlan{
			Budget: types.EvidenceBudget{MaxReactIters: 10, MaxToolCalls: 20},
		},
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
			Language:            "en",
		},
	}
}
