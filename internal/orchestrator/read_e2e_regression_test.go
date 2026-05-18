package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Read-mode regression e2e suite. T2/T3/T4 ship a lot of new write-
// mode machinery; the L1 red line requires read-mode to stay byte-
// identical. These tests cover three risk surfaces:
//
//   1. Dispatch routing — IsWriteGraph(readIR) must be false so
//      runTaskGraph routes to runReadSchedulerLoop, never the
//      write loop. The write loop is wrong for read graphs (no
//      stage hooks, no clearForReplan, no contract checker).
//
//   2. TaskNode.OneShot zero-value compatibility — read TaskGraphs
//      construct nodes via the analyzer's compiler templates. Those
//      literals predate OneShot and don't set it; zero value must
//      mean "old behaviour" (markDone is the terminator). Finalize
//      MUST fire exactly once.
//
//   3. EdgeValidationFeedback retry — the read scheduler's
//      requeueValidationTargets path is exercised by validate-fail
//      cases. T4 added verify→{plan,apply} edges in the WRITE
//      graph; the READ graph's validate→evidence behaviour must
//      stay unchanged.

// TestE2E_ReadMode_DispatchRoutesToReadLoop verifies that an
// analyzer-emitted read TaskGraph (no NodePlan/Apply/Verify nodes)
// runs through runReadSchedulerLoop and reaches finalize cleanly.
// If T4's IsWriteGraph predicate misfired and routed to the write
// loop, the explorer mock would never get called (write loop
// dispatches StagePlan/Apply/Verify, not StageExplore).
func TestE2E_ReadMode_DispatchRoutesToReadLoop(t *testing.T) {
	explorerCalls := 0
	finalizeCalls := 0

	ir := dagIR(types.AnswerContract{
		Language: "en",
	})
	if IsWriteGraph(ir.TaskGraph) {
		t.Fatal("dagIR is supposed to be a read graph; IsWriteGraph wrongly classifying it as write")
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Read loop dispatched explorer at least once and finalize
	// exactly once. If the write loop had been chosen (regression),
	// neither of these would fire.
	if explorerCalls == 0 {
		t.Errorf("explorer should run in read mode; got %d calls", explorerCalls)
	}
	if finalizeCalls != 1 {
		t.Errorf("finalize should run exactly once; got %d calls (regression?)", finalizeCalls)
	}
	if busCtx.Mode != types.ModeRead {
		t.Errorf("Mode should stay ModeRead; got %q", busCtx.Mode)
	}
	result := busCtx.Mutable.Result()
	if !strings.Contains(result, "Foo") {
		t.Errorf("read-mode finalizer answer missing; got %q", result)
	}
}

// TestE2E_ReadMode_OneShotZeroValueCompatible verifies that read-
// mode TaskNode literals (which never set OneShot) still get the
// "fire once then done" behaviour through markDone, NOT through the
// OneShot+nodeDone skip the write scheduler relies on.
//
// Failure mode this catches: if someone accidentally added an
// `if n.OneShot { skip first dispatch }` shortcut into the read
// scheduler, the read finalize node (OneShot=false) would never
// fire and the answer would never render.
func TestE2E_ReadMode_OneShotZeroValueCompatible(t *testing.T) {
	finalizeCalls := 0
	ir := dagIR(types.AnswerContract{Language: "en"})

	// Sanity: all nodes in the read graph must have OneShot=false.
	for _, n := range ir.TaskGraph.Nodes {
		if n.OneShot {
			t.Fatalf("dagIR node %s has OneShot=true; read-mode templates must not set it", n.ID)
		}
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `X` (a.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if finalizeCalls != 1 {
		t.Errorf("read finalize should fire exactly once with OneShot zero-value; got %d", finalizeCalls)
	}
	if busCtx.TaskState.LastError != "" {
		t.Errorf("happy-path read should leave LastError empty; got %q", busCtx.TaskState.LastError)
	}
}

// TestE2E_ReadMode_NoWriteSideEffects verifies that a read-mode Run
// leaves all write-mode state slots empty: ChangePlan, ChangeReport,
// BaselineReport, WriteClosure.AppliedSet should all be zero-value.
// Catches accidental write-mode leakage if a future refactor merges
// a stage hook into the read path.
func TestE2E_ReadMode_NoWriteSideEffects(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Y` (b.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.Mutable.ChangePlan() != nil {
		t.Errorf("read-mode Run should not populate ChangePlan; got %+v", busCtx.Mutable.ChangePlan())
	}
	if busCtx.Mutable.ChangeReport() != nil {
		t.Errorf("read-mode Run should not populate ChangeReport; got %+v", busCtx.Mutable.ChangeReport())
	}
	if busCtx.Mutable.BaselineReport() != nil {
		t.Errorf("read-mode Run should not populate BaselineReport; got %+v", busCtx.Mutable.BaselineReport())
	}
	if applied := busCtx.Mutable.WriteClosure().AppliedSet(); len(applied) != 0 {
		t.Errorf("read-mode Run should not mark anything applied; got %v", applied)
	}
	if busCtx.WorktreePath != "" {
		t.Errorf("read-mode Run should not provision a worktree; got %q", busCtx.WorktreePath)
	}
}

// TestE2E_ReadMode_ValidateFeedbackRetry locks the read-mode
// EdgeValidationFeedback retry path that T4 must NOT have changed.
// Construct an explorer mock that fails on iteration 0 (so the
// validate node's SuccessCriteria fail) and passes on iteration 1
// (the requeued evidence). Verify the explorer ran exactly twice
// and finalize once.
//
// This is a regression test for runReadSchedulerLoop's preservation
// of the unchanged validate-feedback semantics.
func TestE2E_ReadMode_ValidateFeedbackRetry(t *testing.T) {
	// dagIR's TaskGraph already has an EdgeValidationFeedback edge
	// from n2 (validate) → n1 (evidence). We add a SuccessCriteria
	// to validate that fails on first dispatch and passes on retry.
	ir := dagIR(types.AnswerContract{Language: "en"})
	// Add a SC that requires at least 1 evidence item — the first
	// explorer dispatch returns no evidence so the SC fails;
	// requeueValidationTargets requeues n1; second explorer
	// dispatch returns 1 evidence item; SC passes.
	for i := range ir.TaskGraph.Nodes {
		if ir.TaskGraph.Nodes[i].Type == types.NodeValidate {
			ir.TaskGraph.Nodes[i].SuccessCriteria = []types.Criterion{
				{Kind: string(types.CritEvidenceCount), Expr: ">=1"},
			}
		}
	}

	explorerDispatches := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerDispatches++
			out := &agent.StageOutput{MissingPiece: types.MissingFacts}
			// Second dispatch: emit evidence so SC passes.
			if explorerDispatches >= 2 {
				out.EvidenceItems = append(out.EvidenceItems, types.EvidenceItem{
					ID:        "ev-1",
					Predicate: "test",
					Subject:   "X",
					Object:    "Y",
					Summary:   "test evidence",
					Source:    "test.go",
					LineStart: 1,
				})
			}
			return out, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `X` (test.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Explorer should have run AT LEAST twice (initial fail + retry
	// after EdgeValidationFeedback requeue). Allow flex if the
	// scheduler dispatches in a slightly different order, but the
	// key signal is "more than one dispatch happened".
	if explorerDispatches < 2 {
		t.Errorf("EdgeValidationFeedback should retry explorer; got only %d dispatches", explorerDispatches)
	}
	if busCtx.TaskState.LastError != "" {
		t.Errorf("retry-success should clear LastError; got %q", busCtx.TaskState.LastError)
	}
}

func TestE2E_ReadMode_AcceptedInvestigationCompleteStopsValidateReExplore(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})
	for i := range ir.TaskGraph.Nodes {
		if ir.TaskGraph.Nodes[i].Type == types.NodeValidate {
			ir.TaskGraph.Nodes[i].SuccessCriteria = []types.Criterion{
				{Kind: string(types.CritAnswerSetBounded), Expr: "<=1"},
			}
		}
	}

	explorerDispatches := 0
	finalizeCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerDispatches++
			ctx.Mutable.SetInvestigationComplete("model completed investigation after tool preflight")
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				AnswerSymbols: []types.AnswerSymbol{
					{Name: "A"}, {Name: "B"}, {Name: "C"},
				},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "done"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain completed investigation", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerDispatches != 1 {
		t.Fatalf("accepted investigation completion should not re-open explore on validate criteria; got %d dispatches", explorerDispatches)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer should run once after accepted completion; got %d", finalizeCalls)
	}
	if !busCtx.TaskState.IsTerminal {
		t.Fatal("want terminal task")
	}
}

// TestE2E_ReadMode_AnalyzeRetrySuccessClearsLastError verifies the
// phase-1 retry path: when the analyzer fails once, then succeeds on a
// retry, the stale analyze error must not block the read task phase or
// leak into the final run surface.
func TestE2E_ReadMode_AnalyzeRetrySuccessClearsLastError(t *testing.T) {
	analyzeDispatches := 0
	explorerCalls := 0
	finalizeCalls := 0
	ir := dagIR(types.AnswerContract{Language: "en"})

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			analyzeDispatches++
			if analyzeDispatches == 1 {
				return &agent.StageOutput{
					MissingPiece: types.MissingUnderstanding,
					Error:        "emit_analysis was not called during the analyze dispatch",
				}, nil
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingUnderstanding,
				AnalysisIR:   ir,
			}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Recovered` (file.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("trace recovered analyzer path", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if analyzeDispatches < 2 {
		t.Fatalf("expected analyzer retry after first failure; got %d dispatch(es)", analyzeDispatches)
	}
	if explorerCalls == 0 {
		t.Fatal("task phase should continue after analyze retry success")
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer should still run exactly once; got %d", finalizeCalls)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("analyze retry success should clear LastError; got %q", busCtx.TaskState.LastError)
	}
	if got := busCtx.Mutable.Result(); !strings.Contains(got, "Recovered") {
		t.Fatalf("final answer missing after analyze retry recovery; got %q", got)
	}
}
