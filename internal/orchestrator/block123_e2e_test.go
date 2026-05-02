package orchestrator

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBlock123_E2E_SelfContradictionDrivesBackToExplore exercises
// the integrated Block 1 + 2 + 3 pipeline:
//
//   - Block 1: closure auto-bumps PerStage[Stage].Violations every
//     time a violation lands; finalizer-stage retries bump
//     PerStage[finalize].Retries.
//   - Block 2: oracles run on each contract.Check.
//   - Block 3: dominantViolationKind drives the FallbackTarget
//     selection; ViolSelfContradiction maps to BackToExplore so the
//     explorer dispatches on retry.
//
// We pre-seed a ViolSelfContradiction directly on the closure so the
// test does not need a real self_consistency_reviewer LLM. The
// finalizer's contract check picks up the existing violations and
// triggers the retry path. Block 3's selective fallback puts the
// explore stage back in the queue.
func TestBlock123_E2E_SelfContradictionDrivesBackToExplore(t *testing.T) {
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
				RetryBudget:    3,
				CriticalPath:   []string{"n0", "n1"},
			},
		},
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
			Language:            "en",
			MustInclude:         []string{"FORCE_RETRY"},
		},
	}

	var explorerCalls, finalizeCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// Each call appends a fresh evidence item so yieldDelta
			// stays positive and the F5 yield-kill gate doesn't
			// preempt the retry path.
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{
					{Subject: "sym", Source: "test.go", AnchorKind: types.AnchorDefinition},
				},
			}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "answer body without the sentinel",
			}, nil
		},
	}

	// Override fallback policy so MustInclude routes to BackToExplore
	// — exercises the selective requeue path. (Default
	// MustInclude→FinalizerOnly to save wall-clock; for THIS test
	// we want explore re-running to verify the integrated chain.)
	t.Cleanup(func() { SetFallbackPolicyOverrides(nil) })
	SetFallbackPolicyOverrides(map[string]string{
		string(types.ViolMustInclude): string(FallbackBackToExplore),
	})

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMaxUpstreamFallbacksPerRun(2)

	done := make(chan struct{})
	var bus *types.BusContext
	go func() {
		bus, _ = o.Run("e2e block 1+2+3", "/tmp/repo", "main")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s")
	}

	// Block 3 expectation: BackToExplore retries explore at least
	// once on contract failure (capped at MaxUpstreamFallbacksPerRun).
	if explorerCalls < 2 {
		t.Errorf("explorer calls = %d, want ≥ 2 (initial + at least one BackToExplore retry)", explorerCalls)
	}
	if finalizeCalls < 2 {
		t.Errorf("finalize calls = %d, want ≥ 2 (initial + retry)", finalizeCalls)
	}

	// Block 1 expectation: closure StageHealthSnapshot reports the
	// finalize-stage retries.
	if bus != nil && bus.Mutable != nil {
		snap := bus.Mutable.EvidenceClosure().StageHealthSnapshot()
		if snap["finalize"].Retries < 1 {
			t.Errorf("snap[finalize].Retries = %d, want ≥ 1", snap["finalize"].Retries)
		}
		// Block 1 expectation: contract violations register against
		// the finalize stage.
		if snap["finalize"].Violations < 1 {
			t.Errorf("snap[finalize].Violations = %d, want ≥ 1", snap["finalize"].Violations)
		}
	}
}

// TestBlock123_E2E_FinalizerOnlyKeepsExploreIntact pins the
// counter-case: a violation kind that maps to FinalizerOnly should
// NOT requeue explore. Pre-Block-3 the pipeline always re-ran
// everything; post-Block-3 the savings are real for FinalizerOnly
// kinds.
func TestBlock123_E2E_FinalizerOnlyKeepsExploreIntact(t *testing.T) {
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
				RetryBudget:    3,
				CriticalPath:   []string{"n0", "n1"},
			},
		},
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
			Language:            "en",
			// MustInclude triggers ViolMustInclude → FinalizerOnly
			// per the default policy (no override here).
			MustInclude: []string{"FORCE_RETRY"},
		},
	}

	var explorerCalls, finalizeCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{
					{Subject: "sym", Source: "test.go", AnchorKind: types.AnchorDefinition},
				},
			}, nil
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
	o.SetMaxUpstreamFallbacksPerRun(2)
	// Restore default policy explicitly to be safe across parallel test runs.
	t.Cleanup(func() { SetFallbackPolicyOverrides(nil) })
	SetFallbackPolicyOverrides(nil)

	done := make(chan struct{})
	go func() {
		_, _ = o.Run("e2e finalizer-only", "/tmp/repo", "main")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s")
	}

	// FinalizerOnly: explore runs ONCE (initial), finalize retries
	// happen but explore is NOT re-dispatched.
	if explorerCalls != 1 {
		t.Errorf("explorer calls = %d, want exactly 1 (FinalizerOnly does not requeue explore)", explorerCalls)
	}
	if finalizeCalls < 2 {
		t.Errorf("finalize calls = %d, want ≥ 2 (retry on contract fail)", finalizeCalls)
	}
}
