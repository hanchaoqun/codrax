package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// orchestrator_dag_test.go covers the runTaskGraph path: end-to-end
// Run() with an analyzer that produces a real TaskGraph + answer
// contract, and assertions on the merged-schedule dispatch order,
// contract-check backtrack, and budget exhaustion.

// dagIR builds a minimal but realistic AnalysisIR with a 4-node
// chain (probe → evidence → validate → finalize) and a configurable
// answer contract. RetryBudget defaults to 1 so the contract-failure
// path can be tested without hitting an infinite loop.
func dagIR(contract types.AnswerContract) *types.AnalysisIR {
	g := types.TaskGraph{
		Nodes: []types.TaskNode{
			{ID: "n0", Type: types.NodeProbe, Objective: "scan repo",
				SearchHints: types.SearchHints{KeywordIDs: []string{"k1"}}},
			{ID: "n1", Type: types.NodeEvidence, Objective: "collect evidence",
				SearchHints: types.SearchHints{EntityIDs: []string{"e1"}}},
			{ID: "n2", Type: types.NodeValidate, Objective: "check chains"},
			{ID: "n3", Type: types.NodeFinalize, Objective: "render answer"},
		},
		Edges: []types.TaskEdge{
			{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
			{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
			{From: "n2", To: "n3", EdgeType: types.EdgeHardDependency},
			{From: "n2", To: "n1", EdgeType: types.EdgeValidationFeedback},
		},
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1,
			RetryBudget:    1,
			CriticalPath:   []string{"n0", "n1", "n2", "n3"},
		},
	}
	return &types.AnalysisIR{
		Version:      types.AnalysisIRVersion,
		RequestModel: types.RequestModel{Language: "en", Intent: types.IntentExplain},
		TaskGraph:    g,
		EvidencePlan: types.EvidencePlan{
			Budget: types.EvidenceBudget{MaxFiles: 30, MaxBytes: 200000, MaxReactIters: 10, MaxToolCalls: 40},
		},
		AnswerContract: contract,
	}
}

func dagAnalyzerFn(ir *types.AnalysisIR) func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
	return func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
		return &agent.StageOutput{
			MissingPiece: types.MissingFacts,
			AnalysisIR:   ir,
		}, nil
	}
}

func TestRunTaskGraph_HappyPath(t *testing.T) {


	var explorerCalls, finalizeCalls int
	var observedExplorerHints []string

	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		Language:            "en",
	})

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			observedExplorerHints = append(observedExplorerHints, ctx.RetryHint)
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)\n- `Bar` (file.go:2)",
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
	if !busCtx.TaskState.IsTerminal {
		t.Error("want terminal")
	}
	// Conservative schedule: exactly 1 explorer dispatch + 1 finalize dispatch.
	if explorerCalls != 1 {
		t.Errorf("explorer calls: want 1 (merged window), got %d", explorerCalls)
	}
	if finalizeCalls != 1 {
		t.Errorf("finalize calls: want 1, got %d", finalizeCalls)
	}
	// The explorer's RetryHint should mention every node objective.
	if len(observedExplorerHints) == 0 {
		t.Fatal("no hints observed")
	}
	hint := observedExplorerHints[0]
	for _, want := range []string{"scan repo", "collect evidence", "check chains"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q\n%s", want, hint)
		}
	}
	// Recorded answer should land on Mutable.Result.
	if result := busCtx.Mutable.Result(); !strings.Contains(result, "Foo") {
		t.Errorf("task result not recorded: %q", result)
	}
}

func TestRunTaskGraph_ContractFailureBacktracks(t *testing.T) {


	var explorerCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		Language:            "en",
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 2,
		},
	})

	// First finalize emits zero citations → contract fails → backtrack.
	// Second finalize emits two citations → contract passes.
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			if finalizeCalls == 1 {
				return &agent.StageOutput{
					MissingPiece: types.MissingNone,
					FinalAnswer:  "- foo\n- bar (no citations)",
				}, nil
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` at file.go:1\n- `Bar` at file.go:2",
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
	if explorerCalls != 2 {
		t.Errorf("explorer calls: want 2 (initial + 1 backtrack), got %d", explorerCalls)
	}
	if finalizeCalls != 2 {
		t.Errorf("finalize calls: want 2, got %d", finalizeCalls)
	}
}

// TestRunTaskGraph_FinalizeSuccessCriterionFailureBacktracks pins the
// 2026-04-17 fix: a failing finalize SuccessCriterion (e.g.
// citation_count_ge 3 — only 1 citation produced) now triggers the
// same requeue + retry path as an AnswerContract violation.
// Pre-fix behaviour only logged the failure and let the answer ship.
//
// Setup: contract has NO failing clauses (shape=explanation, no
// citation requirement) so contract.Check returns Passed=true. The
// finalize TaskNode carries a citation_count_ge=3 SuccessCriterion,
// and the first finalize draft produces only 1 citation in prose —
// the criterion fails. The fix merges that failure into res so the
// retry branch fires; the second draft emits 3 citations and passes.
func TestRunTaskGraph_FinalizeSuccessCriterionFailureBacktracks(t *testing.T) {
	var explorerCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		Language:            "en",
	})
	// Attach the SuccessCriterion to the finalize node. The
	// orchestrator's buildEnv wires DraftCitations from the rendered
	// answer, so citation_count_ge inspects len(extractCitationsFromAnswer).
	for i := range ir.TaskGraph.Nodes {
		if ir.TaskGraph.Nodes[i].Type == types.NodeFinalize {
			ir.TaskGraph.Nodes[i].SuccessCriteria = []types.Criterion{
				{Kind: types.CritCitationCountGE, Expr: "3"},
			}
		}
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			if finalizeCalls == 1 {
				// Only 1 citation — SuccessCriterion citation_count_ge 3 fails.
				return &agent.StageOutput{
					MissingPiece: types.MissingNone,
					FinalAnswer:  "Answer text with one anchor internal/agent/explorer.go:42.",
				}, nil
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "Answer with internal/agent/a.go:1 and internal/agent/b.go:2 and internal/agent/c.go:3.",
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
	if explorerCalls != 2 {
		t.Errorf("explorer calls: want 2 (initial + 1 backtrack after SC failure), got %d", explorerCalls)
	}
	if finalizeCalls != 2 {
		t.Errorf("finalize calls: want 2, got %d", finalizeCalls)
	}
}

func TestRunTaskGraph_BudgetExhaustedFailLoud(t *testing.T) {


	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		Language:            "en",
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 2,
		},
	})

	// Always fail the contract.
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- foo\n- bar",
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
	result := busCtx.Mutable.Result()
	if result == "" {
		t.Fatal("no result recorded")
	}
	if !strings.Contains(result, "answer-contract validation exhausted") {
		t.Errorf("expected fail-loud warning prepended; got %q", result)
	}
	// Original answer body must survive beneath the warning.
	if !strings.Contains(result, "- foo") {
		t.Errorf("expected original answer beneath warning; got %q", result)
	}
}

// TestRunTaskGraph_NilIRFailsFast: when the analyzer does not
// produce a TaskGraph, runTaskGraph marks the task failed fast.
// The legacy fallback was deleted in the 2026-04-14 simplification.
func TestRunTaskGraph_NilIRFailsFast(t *testing.T) {

	var explorerCalls int

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			// Return NO AnalysisIR — runTaskGraph should fail fast.
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("ask something", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if explorerCalls != 0 {
		t.Error("nil-IR path must not dispatch explorer")
	}
	if busCtx.TaskState.LastError == "" {
		t.Error("nil-IR path should record a LastError")
	}
}

func TestRunTaskGraph_EvidencePlanBudgetCapsSteps(t *testing.T) {
	// Build an IR whose EvidencePlan caps stepBudget tighter than
	// the orchestrator's maxSteps. The merged schedule's per-task
	// loop should respect the IR cap.


	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		Language:            "en",
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 5, // contract will keep failing
		},
	})
	// Force tight cap. Only 2 steps allowed (1 explore + 1 finalize).
	ir.EvidencePlan.Budget.MaxReactIters = 2

	var explorerCalls, finalizeCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- foo",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(50) // generous, but the IR cap should win

	_, err := o.Run("ask", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Expect at most 2 dispatches inside runTaskGraph (1 explore + 1
	// finalize). One backtrack would push us over the cap.
	if explorerCalls+finalizeCalls > 2 {
		t.Errorf("EvidencePlan cap should bound dispatches to ≤2; got explore=%d finalize=%d",
			explorerCalls, finalizeCalls)
	}
}
