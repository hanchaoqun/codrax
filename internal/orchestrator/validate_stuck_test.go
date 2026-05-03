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
// AND the hypothesis's RequiredEvidence satisfaction advances (i.e.
// the explorer is actually pulling evidence that edges the unknown
// hypothesis toward decidable), the scheduler must run the ORIGINAL
// requeue path — neither the envShape nor the hypProgress
// fingerprint should match its previous failure.
//
// Guards against false positives where either shape-guard dimension
// would pre-emptively give up on a hypothesis that is still making
// progress.
//
// Test setup: the hypothesis carries 5 CritSymbolPresent requirements
// keyed on "item-1".."item-5"; the mock explorer appends one new
// evidence item per round naming the next token — so SatisfiedReqSum
// grows by 1 every requeue cycle, hypProgress advances, and both
// fingerprints remain moving targets.
func TestValidateStuck_ShapeChanged_RequeuesNormally(t *testing.T) {
	ir := buildStuckValidateIR()
	// Session 22: give the hypothesis RequiredEvidence tied to a
	// sequence of distinct Subject tokens so the mock explorer can
	// advance hypProgress.SatisfiedReqSum one step per round.
	ir.HypothesisSet[0].RequiredEvidence = []types.Criterion{
		{Kind: types.CritSymbolPresent, Expr: "item-1"},
		{Kind: types.CritSymbolPresent, Expr: "item-2"},
		{Kind: types.CritSymbolPresent, Expr: "item-3"},
		{Kind: types.CritSymbolPresent, Expr: "item-4"},
		{Kind: types.CritSymbolPresent, Expr: "item-5"},
	}

	var explorerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// Append a DIFFERENT evidence item each call, matching the
			// next RequiredEvidence needle so hypProgress advances.
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

	// When BOTH fingerprints advance each round, the normal requeue
	// path runs multiple times before finally exhausting retry
	// budget. Assert explorer dispatched more than once — if either
	// shape-guard had fired on the first failure despite progress,
	// explorerCalls would be ≤ 1.
	if explorerCalls < 2 {
		t.Errorf("explorer ran %d times — shape-guard fired too aggressively when progress was being made", explorerCalls)
	}
}

// TestValidateStuck_HypProgressPinned_EscalatesToInconclusive is the
// session-22 regression test: envShape advances (new evidence added
// each round) but hypProgress stays pinned because none of the new
// evidence satisfies any unknown hypothesis's RequiredEvidence. This
// is the "traceback paths outside repo → explorer fishes in codrax's
// own infrastructure" pathology. Before A: envShape alone would see
// progress and requeue forever until step budget drained. After A:
// hypProgress fingerprint catches the stall because SatisfiedReqSum
// stays at 0 across rounds — stuck escape fires on iter 2.
func TestValidateStuck_HypProgressPinned_EscalatesToInconclusive(t *testing.T) {
	ir := buildStuckValidateIR()
	// Hypothesis demands a symbol that no emitted evidence will ever
	// mention — satisfies RequiredEvidence.len > 0 so hypProgress is
	// dimensional (distinguishes it from the original envShape-only
	// case), but SatisfiedReqSum is forever 0.
	ir.HypothesisSet[0].RequiredEvidence = []types.Criterion{
		{Kind: types.CritSymbolPresent, Expr: "symbol_that_never_appears_in_this_repo"},
	}

	var explorerCalls, finalizeCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// Every call appends a FRESH evidence item — envShape
			// keeps advancing (EvidenceCount grows) but none of it
			// satisfies the hypothesis's RequiredEvidence, so
			// hypProgress.SatisfiedReqSum stays pinned at 0.
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{
					{
						Subject: "irrelevant-" + string(rune('0'+explorerCalls)),
						Source:  "test.go",
					},
				},
			}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			finalizeCalls++
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "answer despite hypProgress stall",
			}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	done := make(chan *types.BusContext)
	go func() {
		bus, _ := o.Run("test hyp progress stall", "/tmp/repo", "main")
		done <- bus
	}()

	var bus *types.BusContext
	select {
	case bus = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s — hypProgress escape regression")
	}
	if bus == nil {
		t.Fatal("nil BusContext")
	}

	if finalizeCalls == 0 {
		t.Error("finalize was never dispatched after hypProgress escape")
	}
	// Explorer should have run only a few times — at least once to
	// establish baseline, at most a handful before hypProgress catches
	// the stall. Runaway count would mean the new dimension didn't
	// engage.
	if explorerCalls == 0 {
		t.Error("explorer never ran — shape-guard fired too aggressively")
	}
	if explorerCalls > 5 {
		t.Errorf("explorer ran %d times — hypProgress escape did not fire (regression)", explorerCalls)
	}

	// The hypothesis must have landed as HypInconclusive.
	verdicts := bus.Mutable.EmittedHypothesisVerdicts()
	foundInconclusive := false
	for _, v := range verdicts {
		if v.HypothesisID == "h1" && v.Status == types.HypInconclusive {
			foundInconclusive = true
			break
		}
	}
	if !foundInconclusive {
		t.Errorf("h1 not marked HypInconclusive after hypProgress escape; verdicts=%+v", verdicts)
	}
}

// TestValidateStuck_NonHypothesisSC_HypProgressDoesNotFalseTrigger
// pins the session-22 defensive guard on hypProgress-stuck detection.
//
// Some validate templates carry SC that are NOT about hypotheses
// (e.g. ScenarioArchitectureExplain uses CritAnswerSetBounded). For
// those, the hypothesis-scope fingerprint is trivially pinned at
// {UnknownCount:0, SatisfiedReqSum:0} across every iteration — if
// hypStuck fired on that match, it would falsely mark the validate
// node done and skip to finalize on a genuine non-hypothesis SC
// failure.
//
// The guard: hypStuck also requires currentHyp.UnknownCount > 0, so
// "no hypothesis to inject inconclusive for" short-circuits to the
// normal envShape-only behavior. The envShape check still catches
// the "nothing at all advanced" case for non-hypothesis SC.
//
// Test construction: IR has ZERO hypotheses but the validate node
// has CritAnswerSetBounded SC that will never be satisfied (mock
// explorer emits too many answer symbols each round so envShape
// advances). Expect the normal requeue path to run until retry
// budget exhausts — NOT the hypProgress shortcut.
func TestValidateStuck_NonHypothesisSC_HypProgressDoesNotFalseTrigger(t *testing.T) {
	// Pre-shape-retirement, this test relied on V1 checkShape firing
	// ViolFamilyMismatch on a stub finalizer (FinalAnswer="ok" without a real
	// AnswerDocument) to drive repeated explorer dispatches. P9-C
	// retires checkShape — the contract.Check side never produces
	// ViolFamilyMismatch now. The hypStuck guard semantic is still validated
	// indirectly by other tests in this file via SC-driven requeue.
	t.Skip("V1 ViolFamilyMismatch-driven re-explore retired with P9-C; coverage moves to V2 block-oracle tests in contract_check_block_test.go")
	ir := buildStuckValidateIR()
	// Wipe hypotheses — this test exercises the non-hypothesis SC
	// branch of validate nodes.
	ir.HypothesisSet = nil
	// Override validate SC to non-hypothesis criterion.
	for i := range ir.TaskGraph.Nodes {
		if ir.TaskGraph.Nodes[i].ID == "n1" {
			ir.TaskGraph.Nodes[i].SuccessCriteria = []types.Criterion{
				{Kind: types.CritAnswerSetBounded, Expr: "<=1"},
			}
		}
	}

	var explorerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts, AnalysisIR: ir}, nil
		},
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			// Each call appends a fresh evidence item — envShape
			// advances every round. hypProgress stays pinned at
			// {0,0} because there are zero hypotheses in the set.
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				EvidenceItems: []types.EvidenceItem{
					{Subject: "sym-" + string(rune('0'+explorerCalls)), Source: "test.go"},
				},
			}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "ok"}, nil
		},
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(15)
	// Block 3 (architecture overhaul 2026-05-02): the test stub
	// finalizer returns FinalAnswer="ok" without constructing an
	// AnswerDocument with shape=Explanation, which triggers
	// ViolFamilyMismatch on every contract.Check. The default fallback
	// policy maps ViolFamilyMismatch→FinalizerOnly (a shape mismatch is
	// re-rendered, not re-investigated). For THIS test — which
	// validates the hypStuck guard's interaction with normal
	// requeue — we override the policy to BackToExplore so the
	// pre-Block-3 "explorer re-runs on contract failure"
	// assumption holds. Restored at cleanup.
	t.Cleanup(func() { SetFallbackPolicyOverrides(nil) })
	SetFallbackPolicyOverrides(map[string]string{
		string(types.ViolFamilyMismatch): string(FallbackBackToExplore),
	})

	done := make(chan struct{})
	go func() {
		_, _ = o.Run("non-hyp sc progress", "/tmp/repo", "main")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate within 5s")
	}

	// With the guard, hypStuck is short-circuited by UnknownCount=0
	// so the normal requeue path runs. Expect explorer to dispatch
	// more than twice — if hypStuck had false-triggered, it would
	// mark validate done after the 2nd SC failure (explorerCalls=2).
	if explorerCalls < 3 {
		t.Errorf("explorer ran %d times — hypStuck guard regression: empty hypothesis set false-triggered stuck escape", explorerCalls)
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
			Language:            "en",
		},
	}
}
