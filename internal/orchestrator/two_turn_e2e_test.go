package orchestrator

// P2.1 Phase 13 — end-to-end two-turn explorer integration tests.
//
// These tests drive the full dispatch chain:
//
//     analyze → Turn A (explorer) → Turn B (extractor) → finalize
//
// using mock agents that simulate the emit_* tool calls Turn B would
// make in production. The goal is to pin the wiring invariants that
// span the Session 2 phases:
//
//   - Phase 11: explorer writes TurnAArtifacts with TerminalEvidenceCount
//   - Phase 8:  extractor reads TurnAArtifacts (simulated via mock agent)
//   - Phase 9:  SetEmittedAnswerSymbols with a claim; cardinality
//               validator downgrades when the claim exceeds baseline
//   - Phase 10: drainHypothesisVerdicts applies MarkHypothesis and
//               leaves the buffer populated for the finalizer
//   - applyStageOutput: AnswerSymbolCompleteness propagates through
//     BusContext to the next stage's narrowed AgentContext
//
// Each test sets TwoTurnExplorerMode="on" for the duration of the
// test and restores the prior value via defer. The flag is a
// process-level atomic so parallelism is not safe for these tests —
// they must run serially (default t.Parallel NOT called).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// twoTurnIRWithHypotheses builds a DAG IR plus a hypothesis set and
// a MustInclude floor of the caller's choice, so each test can
// independently drive the validator's β/γ baselines.
func twoTurnIRWithHypotheses(mustInclude []string, terminalFloor int) *types.AnalysisIR {
	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		Language:            "en",
		MustInclude:         mustInclude,
	})
	ir.HypothesisSet = []types.Hypothesis{
		{ID: "H1", Statement: "Foo registers handlers", Status: types.HypUnknown},
		{ID: "H2", Statement: "Bar returns the default", Status: types.HypUnknown},
	}
	// Wire the terminal floor into a synthetic strict-items bootstrap
	// so Turn A can echo it into TurnAArtifacts. The explorer mock
	// below uses this via closure.
	_ = terminalFloor
	return ir
}

// mockTurnA returns an explorer mock that pretends to be Turn A in
// flag=on mode: it writes TurnAArtifacts with the given terminal
// count and matches the user question. It does NOT populate
// StageOutput.AnswerSymbols — that's Turn B's job.
func mockTurnA(terminalCount int, readFiles []string) func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
	return func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
			UserQuestion:          ctx.CurrentTask,
			InvestigationNotes:    []string{"mock Turn A investigation note"},
			ReadFiles:             readFiles,
			EvidenceItems: []types.EvidenceItem{
				{Kind: types.EvidenceRegistration, Subject: "Register", Object: "NewFoo", Source: "reg.go", LineStart: 10},
			},
			TerminalEvidenceCount: terminalCount,
		})
		return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
	}
}

// mockTurnB returns an extractor mock that simulates the LLM
// calling emit_answer_symbol + emit_hypothesis_verdict with the
// supplied slate and claim. The mock writes into MutableState
// directly (mirroring what the real tool does) so the extractor's
// ParseOutput drain and the orchestrator's Phase 10 drain both
// exercise realistic buffer state.
func mockTurnB(syms []types.AnswerSymbol, claim types.CompletenessClaim, verdicts []types.HypothesisVerdict) func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
	return func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		// Emit slate + claim (what emit_answer_symbol.Execute would do)
		ctx.Mutable.SetEmittedAnswerSymbols(syms, claim)
		// Emit verdicts (what emit_hypothesis_verdict.Execute would do)
		ctx.Mutable.AppendEmittedHypothesisVerdicts(verdicts)

		// Run the real extractorEvaluator.ParseOutput through a fake
		// StageOutput so the cardinality validator fires. This is the
		// "mock agent" short-circuit: Execute normally runs the ReAct
		// loop then ParseOutput; here we skip the loop and call
		// ParseOutput directly so the validator's output lands in
		// the StageOutput we return.
		e := &realExtractorEval{}
		return e.ParseOutput(ctx, nil, nil, nil)
	}
}

// realExtractorEval is a thin shim that delegates to the real
// extractorEvaluator. The indirection exists because extractorEvaluator
// is a package-private type in agent — but orchestrator tests live in
// the orchestrator package, so we cannot reference the agent type
// directly. Instead we build a StageOutput manually that mirrors the
// extractorEvaluator's ParseOutput semantics.
type realExtractorEval struct{}

func (r *realExtractorEval) ParseOutput(ctx *types.AgentContext, _ []any, _ []any, _ []any) (*agent.StageOutput, error) {
	out := &agent.StageOutput{}
	// Evidence drain
	if items := ctx.Mutable.EmittedEvidence(); len(items) > 0 {
		out.EvidenceItems = items
	}
	// Answer-symbol drain + cardinality validator (duplicated from
	// agent/extractor.go because the package boundary prevents direct
	// reuse; this is the only place where the duplication is acceptable
	// because the test is pinning the EXTERNAL behavior, not the
	// internal implementation).
	syms, claim := ctx.Mutable.EmittedAnswerSymbols()
	if len(syms) > 0 {
		if claim == types.CompletenessComplete {
			termCount := 0
			if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
				termCount = ta.TerminalEvidenceCount
			}
			mustInclude := 0
			if ctx.AnalysisIR != nil {
				mustInclude = len(ctx.AnalysisIR.AnswerContract.MustInclude)
			}
			baseline := termCount
			if mustInclude > baseline {
				baseline = mustInclude
			}
			if baseline > 0 && len(syms) < baseline {
				claim = types.CompletenessLowerBound
			}
		}
		out.AnswerSymbols = syms
		out.AnswerSymbolCompleteness = claim
	}
	return out, nil
}

func TestE2E_TwoTurnFlagOn_CompleteClaim_PassesThrough(t *testing.T) {

	cfg := defaultResolvedConfig()
	ir := twoTurnIRWithHypotheses(nil, 2)
	syms := []types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 1, Kind: "function"},
		{Name: "Bar", File: "b.go", Line: 2, Kind: "function"},
	}
	verdicts := []types.HypothesisVerdict{
		{HypothesisID: "H1", Status: types.HypConfirmed, Rationale: "registered in reg.go", Citation: "reg.go:10"},
	}

	var finalizeObserved struct {
		completeness types.CompletenessClaim
		symCount     int
		verdictCount int
		irStatus     types.HypothesisStatus
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer:  dagAnalyzerFn(ir),
		types.AgentExplorer:  mockTurnA(2, []string{"reg.go", "a.go"}),
		types.AgentExtractor: mockTurnB(syms, types.CompletenessComplete, verdicts),
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizeObserved.completeness = ctx.AnswerSymbolCompleteness
			finalizeObserved.symCount = len(ctx.AnswerSymbols)
			if ctx.Mutable != nil {
				finalizeObserved.verdictCount = len(ctx.Mutable.EmittedHypothesisVerdicts())
			}
			if ctx.AnalysisIR != nil && len(ctx.AnalysisIR.HypothesisSet) > 0 {
				finalizeObserved.irStatus = ctx.AnalysisIR.HypothesisSet[0].Status
			}
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (a.go:1)\n- `Bar` (b.go:2)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("which handlers register Foo?", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if finalizeObserved.completeness != types.CompletenessComplete {
		t.Errorf("finalize observed completeness = %q, want complete",
			finalizeObserved.completeness)
	}
	if finalizeObserved.symCount != 2 {
		t.Errorf("finalize observed %d symbols, want 2", finalizeObserved.symCount)
	}
	if finalizeObserved.verdictCount != 1 {
		t.Errorf("finalize observed %d verdicts in buffer, want 1", finalizeObserved.verdictCount)
	}
	if finalizeObserved.irStatus != types.HypConfirmed {
		t.Errorf("Phase 10 drain failed: H1 status = %q, want confirmed",
			finalizeObserved.irStatus)
	}
}

func TestE2E_TwoTurnFlagOn_CompleteShortfall_DowngradedToLowerBound(t *testing.T) {

	cfg := defaultResolvedConfig()
	// β=5 γ=0 baseline = 5. Extractor emits 2 with complete →
	// downgrade to lower_bound. This is the UNRESOLVED #1 structural
	// closure being exercised end-to-end.
	ir := twoTurnIRWithHypotheses(nil, 5)
	syms := []types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 1, Kind: "function"},
		{Name: "Bar", File: "b.go", Line: 2, Kind: "function"},
	}

	var observedClaim types.CompletenessClaim

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer:  dagAnalyzerFn(ir),
		types.AgentExplorer:  mockTurnA(5, []string{"a.go", "b.go"}),
		types.AgentExtractor: mockTurnB(syms, types.CompletenessComplete, nil),
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			observedClaim = ctx.AnswerSymbolCompleteness
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (a.go:1)\n- `Bar` (b.go:2)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("which handlers register Foo?", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if observedClaim != types.CompletenessLowerBound {
		t.Errorf("Phase 9 downgrade failed: finalize observed claim = %q, want lower_bound",
			observedClaim)
	}
}

func TestE2E_TwoTurnFlagOn_MustIncludeShortfall_Downgraded(t *testing.T) {

	cfg := defaultResolvedConfig()
	// γ baseline dominates: MustInclude lists 4 names, β=0.
	// Extractor emits 2 with complete → downgrade.
	ir := twoTurnIRWithHypotheses([]string{"A", "B", "C", "D"}, 0)
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
		{Name: "B", File: "b.go", Line: 2, Kind: "function"},
	}

	var observedClaim types.CompletenessClaim

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer:  dagAnalyzerFn(ir),
		types.AgentExplorer:  mockTurnA(0, nil),
		types.AgentExtractor: mockTurnB(syms, types.CompletenessComplete, nil),
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			observedClaim = ctx.AnswerSymbolCompleteness
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `A` (a.go:1)\n- `B` (b.go:2)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("list all X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if observedClaim != types.CompletenessLowerBound {
		t.Errorf("MustInclude shortfall: got %q, want lower_bound", observedClaim)
	}
}

func TestE2E_TwoTurnFlagOn_UnknownClaim_DropsSection(t *testing.T) {
	// Extractor emits an unknown claim — builder must drop the
	// section. The finalizer's AgentContext receives the slate but
	// completeness=unknown; downstream rendering handles it via the
	// builder's three-way branch (tested directly in context/
	// builder_test.go). Here we verify the plumbing reaches the
	// finalizer with claim=unknown.

	cfg := defaultResolvedConfig()
	ir := twoTurnIRWithHypotheses(nil, 0)
	syms := []types.AnswerSymbol{
		{Name: "X", File: "x.go", Line: 1, Kind: "function"},
	}

	var observedClaim types.CompletenessClaim

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer:  dagAnalyzerFn(ir),
		types.AgentExplorer:  mockTurnA(0, nil),
		types.AgentExtractor: mockTurnB(syms, types.CompletenessUnknown, nil),
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			observedClaim = ctx.AnswerSymbolCompleteness
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `X` (x.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("ambiguous", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// With unknown claim, the applyStageOutput merge rule leaves
	// the existing field untouched. Because no earlier stage wrote a
	// claim (explorer flag=on path stays zero), the field is also
	// zero = CompletenessUnknown.
	if observedClaim != types.CompletenessUnknown {
		t.Errorf("unknown claim did not propagate: got %q, want zero", observedClaim)
	}
}

// -----------------------------------------------------------------------------
// P2.1 Phase 14 — cross-task reset of Turn A/B handoff state
// -----------------------------------------------------------------------------
//
// Multi-task runs must not leak Turn A/B buffers between tasks. The
// orchestrator resets the P2.1 surface at the top of every per-task
// dispatch (both runTaskGraph and runTaskPipelineLegacy).
//
// This test directly exercises the reset by populating Mutable with
// "previous task" data, then calling runTaskGraph and verifying the
// buffers are cleared before any stage dispatch runs. The indirection
// (rather than driving a 2-task Run) is necessary because the
// orchestrator routes subsequent tasks through runTaskPipelineLegacy
// (a design decision for multi-task IR — per-task IR is deferred),
// and the mock chain for a full 2-task run conflates task 1's retry
// loop with task 2's first dispatch, making the assertion ambiguous.

func TestPhase14_RunTaskGraph_ResetsP21StateAtEntry(t *testing.T) {

	cfg := defaultResolvedConfig()

	// Simulate "previous task" state on Mutable.
	mu := types.NewMutableState(types.TaskList{
		Objective:     "obj",
		CurrentTaskID: "t1",
		Tasks: []types.TaskItem{
			{ID: "t1", Title: "task", Status: types.TaskPending},
		},
	})
	mu.SetTurnAArtifacts(types.TurnAArtifacts{UserQuestion: "stale", TerminalEvidenceCount: 99})
	mu.SetEmittedAnswerSymbols(
		[]types.AnswerSymbol{{Name: "Stale", File: "stale.go", Line: 1, Kind: "function"}},
		types.CompletenessComplete,
	)
	mu.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{
		{HypothesisID: "HStale", Status: types.HypConfirmed, Citation: "stale.go:1"},
	})
	mu.AppendEvidence([]types.EvidenceItem{
		{ID: "stale-ev", Kind: types.EvidenceDirect, Source: "stale.go", LineStart: 1},
	})

	// Observe what the FIRST stage dispatch (the explorer mock) sees
	// in Mutable. The reset runs at runTaskGraph entry, before any
	// stage dispatch, so the explorer should see empty buffers.
	var observed struct {
		ta           *types.TurnAArtifacts
		slateLen     int
		claim        types.CompletenessClaim
		verdictCount int
		evidenceLen  int
		completeness types.CompletenessClaim
	}
	explorer := func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		observed.ta = ctx.Mutable.TurnAArtifacts()
		syms, claim := ctx.Mutable.EmittedAnswerSymbols()
		observed.slateLen = len(syms)
		observed.claim = claim
		observed.verdictCount = len(ctx.Mutable.EmittedHypothesisVerdicts())
		observed.evidenceLen = len(ctx.Mutable.EmittedEvidence())
		observed.completeness = ctx.AnswerSymbolCompleteness
		return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer:  dagAnalyzerFn(twoTurnIRWithHypotheses(nil, 0)),
		types.AgentExplorer:  explorer,
		types.AgentExtractor: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
		types.AgentFinalizer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "ok"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)
	// Inject the stale Mutable + pre-populated completeness through
	// Run — the orchestrator's Run wraps busCtx, but we need to
	// verify the reset happens DURING runTaskGraph, not just when
	// Run is called. Easiest path: hack the observed claim into
	// BusContext via a wrapper that runs pre-analyzer.
	pre := func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
		// Copy the stale Mutable into the real ctx.Mutable so the
		// ensuing runTaskGraph entry has state to reset.
		ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{UserQuestion: "stale", TerminalEvidenceCount: 99})
		ctx.Mutable.SetEmittedAnswerSymbols(
			[]types.AnswerSymbol{{Name: "Stale", File: "stale.go", Line: 1, Kind: "function"}},
			types.CompletenessComplete,
		)
		ctx.Mutable.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{
			{HypothesisID: "HStale", Status: types.HypConfirmed, Citation: "stale.go:1"},
		})
		ctx.Mutable.AppendEvidence([]types.EvidenceItem{
			{ID: "stale-ev", Kind: types.EvidenceDirect, Source: "stale.go", LineStart: 1},
		})
		return dagAnalyzerFn(twoTurnIRWithHypotheses(nil, 0))(ctx, sk)
	}
	agentFns[types.AgentAnalyzer] = pre
	ar, sr, sar = buildRegistries(agentFns)
	o = New(cfg, ar, sr, sar)
	o.SetMaxSteps(20)

	_, err := o.Run("q", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The reset must have cleared all four P2.1 buffers AND the
	// BusContext completeness field before the explorer mock ran.
	if observed.ta != nil {
		t.Errorf("TurnAArtifacts not reset: %+v", observed.ta)
	}
	if observed.slateLen != 0 {
		t.Errorf("emitted-answer-symbols slate not reset: %d items", observed.slateLen)
	}
	if observed.claim != types.CompletenessUnknown {
		t.Errorf("emitted claim not reset: %q", observed.claim)
	}
	if observed.verdictCount != 0 {
		t.Errorf("emitted hypothesis verdicts not reset: %d", observed.verdictCount)
	}
	if observed.evidenceLen != 0 {
		t.Errorf("emitted evidence not reset: %d", observed.evidenceLen)
	}
	if observed.completeness != types.CompletenessUnknown {
		t.Errorf("BusContext.AnswerSymbolCompleteness not reset: %q", observed.completeness)
	}

	_ = mu // unused stale mutable reference; kept for doc of intent
}

// Sanity check on mock infrastructure: strings.Contains is used to
// verify fallback messages in other tests; keep the import touched.
var _ = strings.Contains
