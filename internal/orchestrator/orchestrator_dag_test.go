package orchestrator

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
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
		Language: "en",
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

func TestRunTaskGraph_DegradedFinalizerSkipsStructuredAnswerChecks(t *testing.T) {
	var finalizeCalls int
	ir := dagIR(types.AnswerContract{
		Language: "en",
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "line",
			MinCitations: 99,
		},
	})
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 3

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece:  types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece:     types.MissingNone,
				FinalAnswer:      "degraded prose without structured citations",
				AnswerDegraded:   true,
				SkipAnswerChecks: true,
				DegradeReason:    "answer_document_missing",
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
	if finalizeCalls != 1 {
		t.Fatalf("degraded finalizer answer should not be sent through contract retry loops, finalizeCalls=%d", finalizeCalls)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError = %q, want empty", busCtx.TaskState.LastError)
	}
	if got := busCtx.Mutable.Result(); !strings.Contains(got, "degraded prose") {
		t.Fatalf("degraded final answer was not recorded: %q", got)
	}
}

func TestRun_AnalyzerGateErrorClearedBeforeDegradedSemanticPhase(t *testing.T) {
	partial := dagIR(types.AnswerContract{Language: "zh"})
	partial.RequestModel.Intent = types.IntentExplain
	partial.RequestModel.Scenario = types.ScenarioArchitectureExplain
	partial.RequestModel.Language = "zh"
	partial.RequestModel.AnalyzerHints.Keywords = []string{"architecture", "design"}
	partial.RequestModel.AnalyzerHints.Entities = []string{"ResourceSchedule"}

	var explorerCalls, extractorCalls, finalizerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				Error:      "analyzer quality gate rejected: subtopic_coherence: raw internal detail",
				AnalysisIR: partial,
			}, nil
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizerCalls++
			return &agent.StageOutput{
				MissingPiece:     types.MissingNone,
				FinalAnswer:      "降级但可用的答案",
				AnswerDegraded:   true,
				SkipAnswerChecks: true,
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetMaxSteps(20)
	busCtx, err := o.Run("帮我解析当前目录下的代码，做一个详细的设计文档", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.TaskState.SoftAnalyzerError == "" {
		t.Fatal("SoftAnalyzerError should preserve the analyzer diagnostic")
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("stale analyzer gate error must be cleared before degraded Phase 2; got %q", busCtx.TaskState.LastError)
	}
	if explorerCalls == 0 || finalizerCalls != 1 {
		t.Fatalf("degraded semantic graph should still run explorer/finalizer; explorer=%d extractor=%d finalizer=%d",
			explorerCalls, extractorCalls, finalizerCalls)
	}
	if got := busCtx.Mutable.Result(); !strings.Contains(got, "降级但可用") {
		t.Fatalf("degraded final answer was not recorded: %q", got)
	}
}

func TestRunTaskGraph_AutoCompletesReconcileFromExistingEvidence(t *testing.T) {
	var explorerCalls, extractorCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{Language: "en"})
	reconcile := types.TaskNode{
		ID:              "n_reconcile",
		Type:            types.NodeReconcile,
		Objective:       "reconcile already collected evidence",
		EntryConditions: []types.Criterion{{Kind: types.CritHasEnoughFacts}},
	}
	finalize := ir.TaskGraph.Nodes[len(ir.TaskGraph.Nodes)-1]
	ir.TaskGraph.Nodes = append(append([]types.TaskNode(nil), ir.TaskGraph.Nodes[:3]...), reconcile, finalize)
	ir.TaskGraph.Edges = []types.TaskEdge{
		{From: "n0", To: "n1", EdgeType: types.EdgeHardDependency},
		{From: "n1", To: "n2", EdgeType: types.EdgeHardDependency},
		{From: "n2", To: "n_reconcile", EdgeType: types.EdgeHardDependency},
		{From: "n_reconcile", To: "n3", EdgeType: types.EdgeHardDependency},
		{From: "n2", To: "n1", EdgeType: types.EdgeValidationFeedback},
	}
	ir.TaskGraph.ExecutionPolicy.CriticalPath = []string{"n0", "n1", "n2", "n_reconcile", "n3"}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			if strings.Contains(ctx.RetryHint, "reconcile already collected evidence") {
				t.Fatalf("reconcile-only window should be auto-completed, not dispatched to explorer; hint=%s", ctx.RetryHint)
			}
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "ok"}, nil
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
		t.Fatal("want terminal")
	}
	if explorerCalls != 1 {
		t.Fatalf("explorer calls = %d, want 1; reconcile should not trigger a second explore dispatch", explorerCalls)
	}
	if extractorCalls != 1 {
		t.Fatalf("extractor calls = %d, want 1", extractorCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizeCalls)
	}
}

func TestRunTaskGraph_ExplorerRetryHintRequeuesBeforeExtract(t *testing.T) {
	var explorerCalls, extractorCalls, finalizeCalls int
	var observedExplorerHints []string

	ir := dagIR(types.AnswerContract{Language: "en"})
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 1

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			observedExplorerHints = append(observedExplorerHints, ctx.RetryHint)
			if explorerCalls == 1 {
				return &agent.StageOutput{
					MissingPiece:  types.MissingFacts,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: false},
					RetryHint:     "emit aggregate_facts.member_set before completing",
				}, nil
			}
			return &agent.StageOutput{
				MissingPiece:  types.MissingNone,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			if explorerCalls < 2 {
				t.Fatalf("extractor must not run before explorer satisfies its own fact retry")
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "ok"}, nil
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
		t.Fatal("want terminal")
	}
	if explorerCalls != 2 {
		t.Fatalf("explorer calls = %d, want 2", explorerCalls)
	}
	if extractorCalls != 1 {
		t.Fatalf("extractor calls = %d, want 1", extractorCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", finalizeCalls)
	}
	if len(observedExplorerHints) < 2 || !strings.Contains(observedExplorerHints[1], "aggregate_facts.member_set") {
		t.Fatalf("second explorer dispatch must receive the stage retry hint, got %+v", observedExplorerHints)
	}
}

func TestRunTaskGraph_ContractFailureBacktracks(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolCitation)})

	var explorerCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{
		Language: "en",
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
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolSuccessCriterion)})

	var explorerCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{
		Language: "en",
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
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolCitation)})

	ir := dagIR(types.AnswerContract{
		Language: "en",
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
	// B4-F1: explicitly opt into user-visible caveat for this test
	// (default is false post-B4 — digest goes to logger only).
	// The test's purpose is to verify the digest content rendering
	// when the user-visible path IS active.
	settings := types.PipelineSettings{
		ViolationBudget: types.ViolationBudgetSettings{
			FailLoudEnabled:            true,
			UserVisibleViolationCaveat: true,
		},
	}
	o := New(settings, ar, sr, sar)
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

// TestRunTaskGraph_BudgetExhaustedHidesCaveatByDefault pins the
// post-B4-F1 default: when the operator does not flip
// UserVisibleViolationCaveat the per-violation digest is
// suppressed from the user panel (goes to logger only). The
// answer body still survives — just without the digest banner.
func TestRunTaskGraph_BudgetExhaustedHidesCaveatByDefault(t *testing.T) {
	ir := dagIR(types.AnswerContract{
		Language: "en",
		CitationReq: types.CitationReq{
			Required:     true,
			Granularity:  "file_line",
			MinCitations: 2,
		},
	})
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
	// Default zero-value settings — UserVisibleViolationCaveat=false
	// (post-B4 default). FailLoudEnabled defaults to false here too
	// because zero-value PipelineSettings does not call
	// DefaultViolationBudgetSettings; that's fine — the test pins
	// "digest does NOT leak when caveat flag is false".
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
	if strings.Contains(result, "answer-contract validation exhausted") {
		t.Errorf("digest must NOT leak to user panel under default settings; got %q", result)
	}
	if !strings.Contains(result, "- foo") {
		t.Errorf("answer body must survive; got %q", result)
	}
}

func TestRunTaskGraph_RetryableExploreErrorRequeuesWindow(t *testing.T) {
	var explorerCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{
		Language: "en",
	})
	// Disable graph-level content retry so this test isolates the
	// transient retry path (budget decoupling means transient retry
	// no longer drains content budget). With RetryBudget=0, any
	// finalize-stage contract violation goes terminal immediately.
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 0

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			if explorerCalls == 1 {
				return nil, context.DeadlineExceeded
			}
			// On successful retry, return at least one EvidenceItem so
			// busCtx.EvidenceItems is non-empty and the structurally-
			// empty content retry path stays out (decoupled budgets:
			// the transient retry no longer pays for the content path).
			return &agent.StageOutput{
				MissingPiece:  types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{ID: "ev-recover", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "Recovered answer.",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	// Read-mode transient retry now consumes the dedicated transient
	// budget (decoupled from RetryBudget); enable it here.
	o.SetTransientRetryBudget(1)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError = %q, want empty", busCtx.TaskState.LastError)
	}
	if explorerCalls != 2 {
		t.Fatalf("explorer calls = %d, want 2", explorerCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", finalizeCalls)
	}
}

func TestRunTaskGraph_RetryableFinalizeErrorRequeuesFinalize(t *testing.T) {
	var explorerCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{
		Language: "en",
	})
	// Same reason as RetryableExploreErrorRequeuesWindow: keep the test
	// isolated to transient retry by disabling graph-level content retry.
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 0

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// Populate evidence so handleStructurallyEmptyInvestigation
			// stays out of the loop (see RetryableExploreErrorRequeuesWindow
			// for the rationale; same architecture-decoupling concern).
			return &agent.StageOutput{
				MissingPiece:  types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{ID: "ev-explore", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			if finalizeCalls == 1 {
				return nil, io.ErrUnexpectedEOF
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "Recovered answer.",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	// Analyze, explore, and the successful finalizer consume the
	// available productive steps. The EOF retry itself must be free;
	// if the transient lane charged the step budget, the second
	// finalizer dispatch would never run.
	o.SetMaxSteps(4)
	o.SetTransientRetryBudget(1) // enable read-mode transient retry

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError = %q, want empty", busCtx.TaskState.LastError)
	}
	if explorerCalls != 1 {
		t.Fatalf("explorer calls = %d, want 1", explorerCalls)
	}
	if finalizeCalls != 2 {
		t.Fatalf("finalize calls = %d, want 2", finalizeCalls)
	}
}

func TestRunTaskGraph_ReadFinalizeNoToolStallUsesBudgetNotPlateau(t *testing.T) {
	var explorerCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{Language: "en"})
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 0

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{
				MissingPiece:  types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{ID: "ev-explore", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			if finalizeCalls <= 2 {
				return nil, &llm.StreamStalledError{IdleFor: 61 * time.Second, Cause: context.Canceled}
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "Recovered answer after no-tool stalls.",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(4)
	o.SetTransientRetryBudget(2)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError = %q, want empty", busCtx.TaskState.LastError)
	}
	if explorerCalls != 1 {
		t.Fatalf("explorer calls = %d, want 1", explorerCalls)
	}
	if finalizeCalls != 3 {
		t.Fatalf("finalize calls = %d, want 3", finalizeCalls)
	}
}

func TestRunTaskGraph_RetryableExtractErrorRetriesBeforeFinalize(t *testing.T) {
	var explorerCalls, extractorCalls, finalizeCalls int

	ir := dagIR(types.AnswerContract{Language: "en"})
	ir.TaskGraph.ExecutionPolicy.RetryBudget = 0

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{
				MissingPiece:  types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{{ID: "ev-explore", Source: "src.go", LineStart: 1}},
			}, nil
		},
		types.AgentExtractor: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			extractorCalls++
			if extractorCalls == 1 {
				return nil, io.ErrUnexpectedEOF
			}
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "Recovered answer after extract retry.",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetTransientRetryBudget(1)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.TaskState.LastError != "" {
		t.Fatalf("LastError = %q, want empty", busCtx.TaskState.LastError)
	}
	if explorerCalls != 1 {
		t.Fatalf("explorer calls = %d, want 1", explorerCalls)
	}
	if extractorCalls != 2 {
		t.Fatalf("extractor calls = %d, want 2", extractorCalls)
	}
	if finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", finalizeCalls)
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
		t.Error("nil-IR path must not dispatch explorer (legacy single-finalize-node graph)")
	}
	// 2026-05-10 P-audit follow-up: when the analyzer fails to
	// produce an IR, the orchestrator now falls back to
	// buildDegradedSemanticIR (which delegates to the legacy
	// single-finalize builder for nil partial). The diagnostic
	// is recorded on TaskState.SoftAnalyzerError (not
	// LastError) so the runTaskPhase guard at line 1921 still
	// fires and the finalizer can produce a degraded answer
	// instead of "(no result)". LastError is reserved for
	// hard failures where Phase 2 truly cannot run (unknown
	// pipeline mode etc.).
	if busCtx.TaskState.SoftAnalyzerError == "" {
		t.Error("nil-IR path should record analyzer diagnostic on SoftAnalyzerError so the user-panel caveat carries it")
	}
	if busCtx.TaskState.LastError != "" {
		t.Errorf("nil-IR path must NOT set LastError (would skip Phase 2 + degraded answer); got %q", busCtx.TaskState.LastError)
	}
}

func TestRunTaskGraph_EvidencePlanBudgetCapsSteps(t *testing.T) {
	// Build an IR whose EvidencePlan caps stepBudget tighter than
	// the orchestrator's maxSteps. The merged schedule's per-task
	// loop should respect the IR cap.

	ir := dagIR(types.AnswerContract{
		Language: "en",
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
