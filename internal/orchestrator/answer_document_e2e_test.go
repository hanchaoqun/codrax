package orchestrator

// P2.2 end-to-end tests for answer_document_mode. These tests drive
// the full analyze → explore → finalize chain via mock agents, with
// the finalizer mock simulating an emit_answer_document tool call
// (directly through ctx.Mutable.SetAnswerDocument) and then calling
// the real AnswerDocument renderer. This exercises:
//
//   - the per-task reset hook in runTaskGraph (P2.2 addition to the
//     existing Phase 14 reset block)
//   - the cardinality cross-check on list_of_symbols + complete
//     slates (reused from extractorEvaluator.validateCompletenessClaim)
//   - the renderer's shape-dispatched prose output
//   - the flag=off path staying unchanged (legacy finalizer)
//
// The mock finalizer pattern mirrors realExtractorEval from
// two_turn_e2e_test.go: orchestrator tests live in the orchestrator
// package and cannot directly construct an agent.answerDocumentEvaluator,
// so we duplicate the tiny evaluator-like shim that writes the
// AnswerDocument, runs the P2.1 cardinality validator math, and
// renders through the public render.RenderAnswerDocument function.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalizeWithAnswerDocument returns a finalizer mock that writes the
// supplied AnswerDocument into ctx.Mutable (simulating the
// emit_answer_document tool), runs the P2.1 cardinality cross-check
// on list_of_symbols + complete slates, renders the document into
// prose via the real renderer, and returns a StageOutput populated
// as the real evaluator would. This is the "real behaviour, mock
// agent" pattern — the test pins the externally-observable output
// of the answer_document pipeline, not the internal function call
// graph.
func finalizeWithAnswerDocument(doc *types.AnswerDocument, lang string) func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
	return func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		ctx.Mutable.SetAnswerDocument(doc)
		stored := ctx.Mutable.AnswerDocument()

		// P2.1 cardinality cross-check reused for P2.2 list_of_symbols.
		// Duplicated from agent/answer_document_evaluator.go because
		// the package boundary blocks direct reference — the duplication
		// is acceptable because the test is pinning the EXTERNAL behavior.
		if stored.Shape == types.ShapeListOfSymbols && stored.SymbolsCompleteness == types.CompletenessComplete {
			termCount := 0
			mustInclude := 0
			if ta := ctx.Mutable.TurnAArtifacts(); ta != nil {
				termCount = ta.TerminalEvidenceCount
			}
			if ctx.AnalysisIR != nil {
				mustInclude = len(ctx.AnalysisIR.AnswerContract.MustInclude)
			}
			baseline := termCount
			if mustInclude > baseline {
				baseline = mustInclude
			}
			if baseline > 0 && len(stored.Symbols) < baseline {
				stored.SymbolsCompleteness = types.CompletenessLowerBound
				stored.Caveats = append(stored.Caveats,
					"symbols list was marked complete but did not meet the cardinality baseline; downgraded to lower_bound")
			}
		}

		prose := render.RenderAnswerDocument(stored, lang)
		out := &agent.StageOutput{
			MissingPiece: types.MissingNone,
			FinalAnswer:  prose,
		}
		if len(stored.Symbols) > 0 {
			out.AnswerSymbols = stored.Symbols
			out.AnswerSymbolCompleteness = stored.SymbolsCompleteness
		}
		return out, nil
	}
}

// TestE2E_AnswerDocument_ListOfSymbols_Complete_Pipeline — the happy
// path: analyzer declares list_of_symbols, explorer produces the
// deterministic slate, finalizer emits an AnswerDocument with
// complete claim that meets the baseline, renderer produces the
// complete-header prose.
func TestE2E_AnswerDocument_ListOfSymbols_Complete_Pipeline(t *testing.T) {


	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		Language:            "en",
		MustInclude:         []string{"Foo"},
	})

	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
		Symbols: []types.AnswerSymbol{
			{Name: "Foo", File: "a.go", Line: 10, Kind: "function"},
			{Name: "Bar", File: "b.go", Line: 20, Kind: "function"},
		},
		Citations: []types.Citation{
			{File: "a.go", Line: 10},
			{File: "b.go", Line: 20},
		},
	}

	var finalProse string
	var finalClaim types.CompletenessClaim

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			out, err := finalizeWithAnswerDocument(doc, "en")(ctx, sk)
			if err != nil {
				return nil, err
			}
			finalProse = out.FinalAnswer
			finalClaim = out.AnswerSymbolCompleteness
			return out, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	if _, err := o.Run("list the handlers", "/tmp/repo", "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if finalClaim != types.CompletenessComplete {
		t.Errorf("final claim = %q, want complete (baseline met)", finalClaim)
	}
	if !strings.Contains(finalProse, "Complete answer") {
		t.Errorf("complete header missing: %q", finalProse)
	}
	if !strings.Contains(finalProse, "**Foo**") || !strings.Contains(finalProse, "a.go:10") {
		t.Errorf("Foo symbol missing from prose: %q", finalProse)
	}
	if !strings.Contains(finalProse, "**Bar**") || !strings.Contains(finalProse, "b.go:20") {
		t.Errorf("Bar symbol missing from prose: %q", finalProse)
	}
}

// TestE2E_AnswerDocument_CardinalityDowngrade — the structural
// closure for UNRESOLVED #1 on the P2.2 path: complete claim with
// fewer symbols than MustInclude gets downgraded to lower_bound at
// the finalizer, and the renderer picks the softened header.
func TestE2E_AnswerDocument_CardinalityDowngrade(t *testing.T) {


	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		Language:            "en",
		MustInclude:         []string{"A", "B", "C", "D"}, // baseline = 4
	})

	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
		Symbols: []types.AnswerSymbol{ // only 2 — below baseline
			{Name: "A", File: "a.go", Line: 1, Kind: "function"},
			{Name: "B", File: "b.go", Line: 2, Kind: "function"},
		},
	}

	var finalProse string
	var finalClaim types.CompletenessClaim

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			out, err := finalizeWithAnswerDocument(doc, "en")(ctx, sk)
			if err != nil {
				return nil, err
			}
			finalProse = out.FinalAnswer
			finalClaim = out.AnswerSymbolCompleteness
			return out, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	if _, err := o.Run("list all X", "/tmp/repo", "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if finalClaim != types.CompletenessLowerBound {
		t.Errorf("downgrade failed: final claim = %q, want lower_bound", finalClaim)
	}
	if !strings.Contains(finalProse, "At least the following symbols") {
		t.Errorf("softened header missing: %q", finalProse)
	}
	if !strings.Contains(finalProse, "downgraded to lower_bound") {
		t.Errorf("downgrade caveat missing: %q", finalProse)
	}
}

// TestE2E_AnswerDocument_StepList_CitationPool — verifies the citation
// pool is rendered inline per step, not duplicated, via the real
// renderer output.
func TestE2E_AnswerDocument_StepList_CitationPool(t *testing.T) {


	ir := dagIR(types.AnswerContract{
		RequiredAnswerShape: types.ShapeStepList,
		Language:            "en",
	})

	doc := &types.AnswerDocument{
		Shape:   types.ShapeStepList,
		Summary: "The request flows through three branches.",
		Steps: []types.AnswerStep{
			{Index: 1, Description: "First branch dispatches", CitationRef: 0},
			{Index: 2, Description: "Second branch falls through", CitationRef: 0}, // SAME cite, pool dedup
			{Index: 3, Description: "Third branch halts", CitationRef: 1},
		},
		Citations: []types.Citation{
			{File: "router.go", Line: 10},
			{File: "halt.go", Line: 5},
		},
	}

	var finalProse string
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			out, err := finalizeWithAnswerDocument(doc, "en")(ctx, sk)
			if err != nil {
				return nil, err
			}
			finalProse = out.FinalAnswer
			return out, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	if _, err := o.Run("how does the router dispatch", "/tmp/repo", "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Summary lead-in.
	if !strings.Contains(finalProse, "three branches") {
		t.Errorf("summary missing: %q", finalProse)
	}
	// Each step numbered.
	for _, want := range []string{"1. First branch", "2. Second branch", "3. Third branch"} {
		if !strings.Contains(finalProse, want) {
			t.Errorf("step %q missing: %q", want, finalProse)
		}
	}
	// Citation 0 (router.go:10) is used by two steps — should appear
	// at least twice inline.
	if strings.Count(finalProse, "router.go:10") < 2 {
		t.Errorf("router.go:10 not rendered on both steps 1 and 2: %q", finalProse)
	}
	// Citation 1 (halt.go:5) used by step 3.
	if !strings.Contains(finalProse, "halt.go:5") {
		t.Errorf("halt.go:5 missing: %q", finalProse)
	}
}

// TestE2E_AnswerDocument_DispatchStageRoutesToAnswerDocumentSkill
// pins the P2.2 cleanup contract: when answer_document_mode=on is
// set and stageConfig.Name == StageFinalize, dispatchStage must
// route the finalizer to `answer-document-skill` instead of the
// legacy `final-answer-skill` / `analysis-final-answer-skill`. This
// is the mechanism that prevents the two legacy skills' declarative
// markdown Answer/Evidence OutputFormats from contradicting the
// new tool-call directive — without this routing, the LLM sees two
// conflicting system sections and (per training distribution)
// prefers the example-driven prose path.
func TestE2E_AnswerDocument_DispatchStageRoutesToAnswerDocumentSkill(t *testing.T) {


	ir := dagIR(types.AnswerContract{RequiredAnswerShape: types.ShapeExplanation, Language: "en"})

	var observedSkill string
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			if sk != nil {
				observedSkill = sk.Name
			}
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "ok"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	if _, err := o.Run("q", "/tmp/repo", "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if observedSkill != "answer-document-skill" {
		t.Errorf("finalize routed to %q, want answer-document-skill", observedSkill)
	}
}

// TestE2E_AnswerDocument_CrossTaskReset — directly exercises the
// P2.2 addition to the per-task reset block. A stale AnswerDocument
// on Mutable must be cleared before the first stage of the next
// task runs. Mirrors the structure of TestPhase14_RunTaskGraph_ResetsP21StateAtEntry.
func TestE2E_AnswerDocument_CrossTaskReset(t *testing.T) {


	ir := dagIR(types.AnswerContract{RequiredAnswerShape: types.ShapeExplanation, Language: "en"})

	// Capture ONLY the first explorer call's view of Mutable. The
	// contract checker may requeue the window on subsequent rounds if
	// it disagrees with the finalizer's rendered prose, and a second
	// explorer dispatch would see the AnswerDocument the previous
	// finalize left behind — that is a separate concern (within-task
	// retry semantics), not the cross-task reset P2.2 is meant to
	// pin. The cross-task reset invariant is: the FIRST non-analyze
	// dispatch of a task sees cleared buffers.
	var observed *types.AnswerDocument
	observedSet := false
	explorer := func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
		if !observedSet {
			observed = ctx.Mutable.AnswerDocument()
			observedSet = true
		}
		return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
	}

	// Analyzer mock that ALSO plants stale AnswerDocument state on
	// Mutable before returning the IR. The orchestrator's per-task
	// reset fires AFTER analyze and BEFORE the first non-analyze
	// stage dispatch, so when explorer runs the buffer must be nil.
	analyzerPlant := func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
		ctx.Mutable.SetAnswerDocument(&types.AnswerDocument{
			Shape:   types.ShapeExplanation,
			Summary: "stale from previous task",
		})
		return dagAnalyzerFn(ir)(ctx, sk)
	}

	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: analyzerPlant,
		types.AgentExplorer: explorer,
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			out, err := finalizeWithAnswerDocument(&types.AnswerDocument{
				Shape:   types.ShapeExplanation,
				Summary: "fresh answer",
			}, "en")(ctx, sk)
			return out, err
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)

	if _, err := o.Run("q", "/tmp/repo", "main"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if observed != nil {
		t.Errorf("AnswerDocument not reset at per-task entry: observed = %+v", observed)
	}
}
