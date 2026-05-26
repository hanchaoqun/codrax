package agent

// P2.1 Session 1 Phase 7 — extractorEvaluator skeleton tests.
//
// These tests pin the SKELETON behavior so Session 2 can extend each
// method without accidentally breaking the contract that Phase 5's
// orchestrator dispatch hook relies on.
//
// Phase 7 deliberately keeps the surface minimal: 4 tests, one per
// Evaluator method. Session 2 will add cardinality / drain / retry
// tests as it implements those branches.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExtractor_ExtractSkill_DeclaresToolContract(t *testing.T) {
	// P2.1 Session 2 cleanup: the tool contract (allowed / forbidden
	// list, output format, completeness honesty contract) lives in
	// the extract-skill declared by internal/skill/defaults.go, NOT
	// in extractor.go's BuildInitialInstruction string builder. This test
	// pins the skill's shape so a future edit that strips the
	// contract from the skill surfaces here immediately.
	r := skill.NewRegistry()
	skill.RegisterDefaults(r)
	sk, err := r.Get("extract-skill")
	if err != nil {
		t.Fatalf("extract-skill not registered by RegisterDefaults: %v", err)
	}

	// ToolSuggestions is the per-skill allowlist that buildToolSchemas
	// reads to scope the LLM tool set. MUST contain exactly the two
	// extractor-unique emit_* tools and nothing else. Evidence is
	// Turn A's exclusive channel after the de-duplication cleanup —
	// emit_evidence MUST NOT appear here.
	want := map[string]bool{
		"emit_answer_symbol":      false,
		"emit_hypothesis_verdict": false,
	}
	for _, ts := range sk.ToolSuggestions {
		if _, ok := want[ts]; !ok {
			t.Errorf("extract-skill ToolSuggestions leaks a non-allowed tool %q", ts)
		}
		want[ts] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("extract-skill ToolSuggestions missing %q", name)
		}
	}

	// Goal must mention the one-line role.
	if !strings.Contains(sk.Goal, "investigation") || !strings.Contains(sk.Goal, "answer-symbol") {
		t.Errorf("extract-skill Goal must describe the role, got: %q", sk.Goal)
	}

	// Workflow must cover the two emit_* code paths this skill owns
	// and must NOT instruct emit_evidence (evidence is Turn A's).
	wf := strings.Join(sk.Workflow, "\n")
	for _, step := range []string{"emit_answer_symbol", "emit_hypothesis_verdict", "completeness"} {
		if !strings.Contains(wf, step) {
			t.Errorf("extract-skill Workflow missing %q:\n%s", step, wf)
		}
	}
	if strings.Contains(wf, "emit_evidence") {
		t.Errorf("extract-skill Workflow must not instruct emit_evidence (duplicates Turn A):\n%s", wf)
	}

	// Prohibitions must convey that Turn B has no file access and must
	// not re-emit evidence.
	prohib := strings.Join(sk.Prohibitions, "\n")
	for _, forbidden := range []string{"no file access", "re-emit evidence"} {
		if !strings.Contains(prohib, forbidden) {
			t.Errorf("extract-skill Prohibitions must convey %q: %s", forbidden, prohib)
		}
	}

	// OutputFormat must spell out the completeness honesty contract
	// (the three enum values + the cross-check downgrade rule).
	for _, mustHave := range []string{"complete", "lower_bound", "unknown", "DOWNGRADED", "honesty contract"} {
		if !strings.Contains(sk.OutputFormat, mustHave) {
			t.Errorf("extract-skill OutputFormat missing %q:\n%s", mustHave, sk.OutputFormat)
		}
	}
}

func TestExtractor_BuildInitialInstructionEchoesQuestion(t *testing.T) {
	// BuildInitialInstruction is the DYNAMIC digest only — Turn A
	// transcript snapshot + cardinality baseline + hypothesis set.
	// The user question is rendered by builder.go as "User Request"
	// section and is NOT repeated here.
	e := &extractorEvaluator{}
	ctx := &types.AgentContext{Objective: "what does Foo return?"}
	prompt := e.BuildInitialInstruction(ctx, nil)
	if prompt == "" {
		t.Fatal("BuildInitialInstruction must produce a non-empty result")
	}
	// User question should NOT be echoed — builder.go handles it.
	if contains(prompt, "## User question") {
		t.Error("BuildInitialInstruction should not duplicate the user question (builder.go renders it)")
	}
	// Post-cleanup invariant: the dynamic prompt MUST NOT repeat the
	// full contract sections (allowed/forbidden/honesty contract).
	// A no-transcript path produces a short "No transcript available"
	// notice; it should not accidentally carry the skill's content.
	forbidden := []string{
		"Allowed tools — call ONLY these",
		"Forbidden tools",
		"honesty contract",
		"MUST NOT add or remove",
	}
	for _, staticContent := range forbidden {
		if contains(prompt, staticContent) {
			t.Errorf("cleanup regression: BuildInitialInstruction re-states %q (should be in skill, not prompt)",
				staticContent)
		}
	}
}

func TestExtractor_BuildInitialInstructionHandlesNilCtx(t *testing.T) {
	// Defensive: Session 2 wiring may call BuildInitialInstruction before
	// AgentContext is fully populated. Must not panic; empty output
	// is acceptable now that all static content lives in the skill.
	e := &extractorEvaluator{}
	_ = e.BuildInitialInstruction(nil, nil) // must not panic
}

func TestExtractor_ShouldStopFiresAfterRetryBudget(t *testing.T) {
	// Turn B happy path is a single parallel-batch LLM call at iter=0.
	// Non-parallel paths need more room: iter=0 partial emit, iter=1
	// soft-stop + correction hint, iter=2 LLM fills the gap,
	// iter>=soft cap idle → stop. Recovery window between soft and
	// hard cap allows one streaming-truncation retry of emit_answer_symbol
	// or emit_hypothesis_verdict.
	s := types.ResolvedAgentSettings(types.AgentSettings{})
	e := &extractorEvaluator{
		defaultSoftIterCap: s.ExtractorSoftIterCap,
		defaultHardIterCap: s.ExtractorHardIterCap,
	}
	for i := 0; i < s.ExtractorSoftIterCap; i++ {
		if e.ShouldStop(llm.Response{}, i) {
			t.Errorf("must NOT stop at iteration %d (below soft cap %d)", i, s.ExtractorSoftIterCap)
		}
	}
	// Soft cap idle → stop.
	if !e.ShouldStop(llm.Response{}, s.ExtractorSoftIterCap) {
		t.Errorf("must stop at soft cap %d when LLM idle", s.ExtractorSoftIterCap)
	}
	// Hard cap stops unconditionally.
	for _, i := range []int{s.ExtractorHardIterCap, s.ExtractorHardIterCap + 5} {
		if !e.ShouldStop(
			llm.Response{ToolCalls: []llm.ToolCall{{Name: emitAnswerSymbolToolName}}},
			i,
		) {
			t.Errorf("must stop at iteration %d (hard cap %d)", i, s.ExtractorHardIterCap)
		}
	}
}

func TestExtractor_ParseOutputDrainsEmittedBuffers(t *testing.T) {
	// ParseOutput must drain MutableState's emit_answer_symbol
	// buffer into StageOutput. Evidence is intentionally NOT drained
	// here — that is Turn A's exclusive channel after the de-
	// duplication cleanup. With Phase 9's Set-semantics API the
	// completeness claim travels with the slate.
	mu := types.NewMutableState("")
	// Pre-populate the evidence buffer with a token item and verify
	// the extractor IGNORES it — a regression that re-drains evidence
	// here would surface immediately.
	mu.AppendEvidence([]types.EvidenceItem{
		{ID: "ev1", Kind: types.EvidenceDirect, Source: "a.go", LineStart: 5},
	})
	mu.SetEmittedAnswerSymbols(
		[]types.AnswerSymbol{{Name: "Foo", File: "a.go", Line: 5, Kind: "function"}},
		types.CompletenessComplete,
	)
	// No baseline data (no TurnAArtifacts.TerminalEvidenceCount, no
	// MustInclude) → the validator passes the claim through untouched.
	// ctx.EvidenceItems carries one Direct item so the R4 fail-loud
	// gate recognises the investigation as non-empty and falls through
	// to the drain logic the test is exercising.
	ctx := &types.AgentContext{
		Objective: "test",
		Mutable:   mu,
		EvidenceItems: []types.EvidenceItem{
			{ID: "ev1", Kind: types.EvidenceDirect, Source: "a.go", LineStart: 5},
		},
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if len(out.EvidenceItems) != 0 {
		t.Errorf("extractor must not drain EvidenceItems (Turn A owns evidence), got %+v", out.EvidenceItems)
	}
	if len(out.AnswerSymbols) != 1 || out.AnswerSymbols[0].Name != "Foo" {
		t.Errorf("AnswerSymbols not drained: %+v", out.AnswerSymbols)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessComplete {
		t.Errorf("completeness not drained: got %q, want complete", out.AnswerSymbolCompleteness)
	}
}

// -----------------------------------------------------------------------------
// P2.1 Phase 9 — cardinality validator tests
// -----------------------------------------------------------------------------
//
// validateCompletenessClaim runs inside ParseOutput and checks the
// LLM's "complete" claim only against precise typed floors such as an
// explicit requested boundary. Turn A terminal counts and analyzer
// MustInclude are soft guidance; they are intentionally not hard
// cardinality gates because they can contain proof-chain helpers.

func extractorCtxWithBaseline(termCount int, mustInclude []string, syms []types.AnswerSymbol, claim types.CompletenessClaim) *types.AgentContext {
	mu := types.NewMutableState("")
	// ReadFiles is a stub: in production a non-zero TerminalEvidenceCount
	// always coincides with Turn A having read at least one file, so
	// tests that assert a baseline must not trip the R4 fail-loud gate.
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion:          "q",
		TerminalEvidenceCount: termCount,
		ReadFiles:             []string{"stub.go"},
	})
	mu.SetEmittedAnswerSymbols(syms, claim)
	return &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				MustInclude: mustInclude,
			},
		},
	}
}

func TestExtractor_Validator_Complete_BaselineMet_PassThrough(t *testing.T) {
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
		{Name: "B", File: "b.go", Line: 2, Kind: "function"},
		{Name: "C", File: "c.go", Line: 3, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(3, nil, syms, types.CompletenessComplete)
	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessComplete {
		t.Errorf("baseline met: claim should stay complete, got %q", out.AnswerSymbolCompleteness)
	}
	if len(out.AnswerSymbols) != 3 {
		t.Errorf("slate: got %d items, want 3", len(out.AnswerSymbols))
	}
}

func TestExtractor_Validator_Complete_EnumerationBoundaryOverridesTerminalBaseline(t *testing.T) {
	syms := []types.AnswerSymbol{
		{Name: "checkCoverage", File: "internal/analysis/gate/gate.go", Line: 127, Kind: "function"},
		{Name: "checkDAGClosure", File: "internal/analysis/gate/gate.go", Line: 128, Kind: "function"},
		{Name: "checkBudgetSanity", File: "internal/analysis/gate/gate.go", Line: 129, Kind: "function"},
		{Name: "checkContractComplete", File: "internal/analysis/gate/gate.go", Line: 135, Kind: "function"},
		{Name: "checkHypothesisCoverage", File: "internal/analysis/gate/gate.go", Line: 136, Kind: "function"},
		{Name: "checkCriterionResolvable", File: "internal/analysis/gate/gate.go", Line: 147, Kind: "function"},
		{Name: "checkPendingFieldsWellformed", File: "internal/analysis/gate/gate.go", Line: 148, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(15, nil, syms, types.CompletenessComplete)
	ctx.AnalysisIR.RequestModel = types.RequestModel{
		RawRequest: "What order do gate.Run's 7 checks execute in?",
		AnalyzerHints: types.AnalyzerHints{
			MentionedEntities: []string{"gate.Run"},
		},
		EnumerationBoundary: &types.RequestedEnumerationBoundary{
			DeclaredCount: 7,
			SourceQuote:   "7 checks",
		},
	}
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessComplete {
		t.Fatalf("enumeration boundary should override larger terminal baseline: got %q", out.AnswerSymbolCompleteness)
	}
}

func TestExtractor_Validator_Complete_TerminalCountShortfall_IsSoftGuidance(t *testing.T) {
	// Turn A found 5 terminal candidates, but without a typed requested
	// boundary those candidates are soft guidance: they may include
	// proof-chain helpers rather than answer members.
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
		{Name: "B", File: "b.go", Line: 2, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(5, nil, syms, types.CompletenessComplete)
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessComplete {
		t.Errorf("terminal candidate shortfall must not hard-downgrade without a typed floor, got %q", out.AnswerSymbolCompleteness)
	}
	if len(out.AnswerSymbols) != 2 {
		t.Errorf("slate should stay model-authored, got %d items", len(out.AnswerSymbols))
	}
}

func TestExtractor_Validator_Complete_MustIncludeShortfall_IsSoftGuidance(t *testing.T) {
	// Analyzer MustInclude may contain helper/tool/mechanism terms.
	// It is surfaced to the model, but cannot force slate padding.
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
		{Name: "B", File: "b.go", Line: 2, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(0, []string{"X", "Y", "Z", "W"}, syms, types.CompletenessComplete)
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessComplete {
		t.Errorf("must-include shortfall must not hard-downgrade without a typed floor, got %q", out.AnswerSymbolCompleteness)
	}
}

func TestExtractor_Validator_Complete_HardBoundaryStillDowngrades(t *testing.T) {
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
		{Name: "B", File: "b.go", Line: 2, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(2, []string{"X", "Y", "Z"}, syms, types.CompletenessComplete)
	ctx.AnalysisIR.RequestModel = types.RequestModel{
		RawRequest:    "What order do gate.Run's 3 checks execute in?",
		Intent:        types.IntentTrace,
		AnalyzerHints: types.AnalyzerHints{MentionedEntities: []string{"gate.Run"}},
		EnumerationBoundary: &types.RequestedEnumerationBoundary{
			DeclaredCount: 3,
			SourceQuote:   "3 checks",
		},
	}
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Errorf("typed boundary shortfall must downgrade, got %q", out.AnswerSymbolCompleteness)
	}
}

func TestExtractor_Validator_LowerBound_PassesThroughUnchanged(t *testing.T) {
	// Only "complete" is falsifiable. lower_bound and unknown are
	// honest by construction and must never be "upgraded" or
	// challenged by the validator.
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(10, []string{"X", "Y"}, syms, types.CompletenessLowerBound)
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Errorf("lower_bound must pass through unchanged, got %q", out.AnswerSymbolCompleteness)
	}
}

func TestExtractor_Validator_Complete_NoBaseline_PassesThrough(t *testing.T) {
	// REPL turn with no Turn A data and no MustInclude — we have
	// nothing to validate against, so the claim is trusted. This is
	// not a silent coercion: the LLM did claim complete explicitly,
	// and we have no structural reason to override it.
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(0, nil, syms, types.CompletenessComplete)
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessComplete {
		t.Errorf("no baseline: claim should be trusted, got %q", out.AnswerSymbolCompleteness)
	}
}

// -----------------------------------------------------------------------------
// R4 — extractor fail-loud gate: 0 files read AND 0 key evidence
// -----------------------------------------------------------------------------
//
// The gate fires exactly when Turn A left the extractor nothing real
// to work with. Passes 0 files read + 0 key evidence through to
// StageOutput.Error so the orchestrator's retry budget sees a loud
// signal instead of letting the extractor synthesize an answer on
// an empty investigation.

func TestExtractor_R4Gate_EmptyInvestigation_FailsLoud(t *testing.T) {
	mu := types.NewMutableState("")
	// Set TurnAArtifacts with zero ReadFiles so the Mutable branch is
	// exercised (not the nil-ta branch). No key evidence on ctx.
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion: "q",
	})
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
	}
	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.Error == "" {
		t.Error("R4 gate must set StageOutput.Error on 0 files + 0 key evidence")
	}
	if len(out.AnswerSymbols) != 0 {
		t.Errorf("fail-loud must skip drain, got AnswerSymbols=%d", len(out.AnswerSymbols))
	}
}

func TestExtractor_R4Gate_NilTurnAArtifacts_FailsLoud(t *testing.T) {
	// nil TurnAArtifacts is the same shape as zero ReadFiles for the
	// gate: both mean "Turn A read no files". Pinning this keeps the
	// gate behaviour identical whether the artefacts struct was
	// initialised or not.
	mu := types.NewMutableState("")
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
	}
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.Error == "" {
		t.Error("R4 gate must fire when TurnAArtifacts is nil and no evidence")
	}
}

func TestExtractor_R4Gate_FilesRead_BypassesGate(t *testing.T) {
	// A non-empty ReadFiles list is sufficient to pass the gate even
	// when no key evidence is present: the investigation was not
	// structurally empty. Downstream drain/validator logic runs.
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion: "q",
		ReadFiles:    []string{"a.go"},
	})
	mu.SetEmittedAnswerSymbols(
		[]types.AnswerSymbol{{Name: "Foo", File: "a.go", Line: 5, Kind: "function"}},
		types.CompletenessComplete,
	)
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
	}
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.Error != "" {
		t.Errorf("ReadFiles>0 must bypass gate, got Error=%q", out.Error)
	}
	if len(out.AnswerSymbols) != 1 {
		t.Errorf("drain must run, got AnswerSymbols=%d", len(out.AnswerSymbols))
	}
}

func TestExtractor_R4Gate_RuntimeObservationOnlyCompletion_BypassesGate(t *testing.T) {
	// External log / trace explanation can be complete without any
	// current-repo read/search evidence. The explorer sets this typed
	// flag only after a successful artifact-only completion, so the
	// empty-investigation gate must accept it instead of forcing a
	// fixture read.
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion:                     "explain this traceback",
		AcceptedClosureReason:            "external traceback fully explains the failure",
		AcceptedResultKind:               "resolved",
		RuntimeObservationOnlyCompletion: true,
	})
	ctx := &types.AgentContext{
		Objective: "explain this traceback",
		Mutable:   mu,
	}
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.Error != "" {
		t.Errorf("runtime artifact-only completion must bypass R4, got Error=%q", out.Error)
	}
}

func TestExtractor_R4Gate_KeyEvidence_BypassesGate(t *testing.T) {
	// Zero files read is acceptable when key evidence is present —
	// the R7/R8 path populates ctx.EvidenceItems from programmatic
	// concrete_value + resolution chains on files the LLM never
	// opened. That path must not be blocked by the gate.
	mu := types.NewMutableState("")
	mu.SetEmittedAnswerSymbols(
		[]types.AnswerSymbol{{Name: "Foo", File: "a.go", Line: 5, Kind: "function"}},
		types.CompletenessComplete,
	)
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		EvidenceItems: []types.EvidenceItem{
			{ID: "ev1", Kind: types.EvidenceRegistration, Subject: "Register", Object: "NewHandlerA", Source: "a.go", LineStart: 12},
		},
	}
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.Error != "" {
		t.Errorf("Registration evidence must bypass gate, got Error=%q", out.Error)
	}
	if len(out.AnswerSymbols) != 1 {
		t.Errorf("drain must run, got AnswerSymbols=%d", len(out.AnswerSymbols))
	}
}

func TestExtractor_R4Gate_OnlyNonKeyEvidence_FailsLoud(t *testing.T) {
	// Concrete_value / dataflow / conditional / relationship / absent
	// without any of direct/registration/mechanism is NOT enough to
	// bypass the gate: the three "key" kinds are the ones that carry
	// a cross-file join the finalizer can cite.
	mu := types.NewMutableState("")
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		EvidenceItems: []types.EvidenceItem{
			{ID: "ev1", Kind: types.EvidenceConcrete, Source: "a.go", LineStart: 5},
			{ID: "ev2", Kind: types.EvidenceConditional, Source: "a.go", LineStart: 7},
			{ID: "ev3", Kind: types.EvidenceRelationship, Source: "a.go", LineStart: 9},
		},
	}
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.Error == "" {
		t.Error("non-key evidence (concrete/conditional/relationship) must NOT bypass the gate")
	}
}

// -----------------------------------------------------------------------------
// P2.1 Phase 8 — BuildInitialInstruction reads TurnAArtifacts
// -----------------------------------------------------------------------------

func TestExtractor_BuildPrompt_DigestsTurnAArtifacts(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion:       "which handlers register Foo?",
		InvestigationNotes: []string{"iter 1: read reg.go, found Register calls"},
		ReadFiles:          []string{"internal/reg.go", "internal/a.go"},
		EvidenceItems: []types.EvidenceItem{
			{Kind: types.EvidenceRegistration, Subject: "Register", Object: "NewHandlerA", Source: "internal/reg.go", LineStart: 12},
		},
		TerminalEvidenceCount: 3,
	})
	ctx := &types.AgentContext{
		Objective: "which handlers register Foo?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
			},
			AnswerContract: types.AnswerContract{MustInclude: []string{"HandlerA", "HandlerB"}},
		},
	}

	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)

	// User question is NOT echoed here — builder.go renders it as
	// "User Request" section. Verify absence.
	if contains(prompt, "## User question") {
		t.Error("prompt must not duplicate user question (builder.go handles it)")
	}
	// Investigation digest sections (the dynamic content this function is
	// still responsible for post-cleanup)
	for _, section := range []string{
		"Investigation notes",
		"iter 1: read reg.go",
		"Files the investigation read (direct citation gutter)",
		"Grounded deterministic evidence rows below are also accepted anchors via `evidence_id`",
		"internal/reg.go",
		"Deterministic evidence the investigation extracted",
		"evidence_id=ev-",
		"registration",
		"Register",
		"Cardinality guidance",
	} {
		if !contains(prompt, section) {
			t.Errorf("prompt missing investigation digest section/content %q", section)
		}
	}
	// Guidance numbers surfaced — terminal-evidence count and analyzer
	// required-name count.
	if !contains(prompt, "terminal-evidence count") || !contains(prompt, "required-name count") {
		t.Error("prompt must surface the cardinality baselines by name")
	}
	if !contains(prompt, "Hard typed floor") || !contains(prompt, "soft search/completeness reminders") {
		t.Error("prompt must separate hard typed floors from soft guidance counts")
	}
	// Post-cleanup invariant: the dynamic prompt MUST NOT repeat the
	// full tool contract (role, allowed/forbidden list, output format,
	// completeness honesty explanation) — those live in the skill.
	// Individual references to emit_answer_symbol in the baseline
	// hint are legitimate per-dispatch cross-references and are
	// permitted; what we forbid is the CONTRACT sections themselves.
	forbidden := []string{
		"Allowed tools — call ONLY these",
		"Forbidden tools",
		"honesty contract",
		"MUST NOT add or remove",
	}
	for _, staticContent := range forbidden {
		if contains(prompt, staticContent) {
			t.Errorf("cleanup regression: dynamic prompt re-states contract section %q (should be in skill)", staticContent)
		}
	}
}

func TestExtractor_BuildPrompt_CapsAnalyzerSoftGuidanceNames(t *testing.T) {
	var must []string
	for i := 0; i < extractorMaxSoftGuidanceNames+5; i++ {
		must = append(must, fmt.Sprintf("Name%02d", i))
	}
	mu := types.NewMutableState("list names")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{TerminalEvidenceCount: 1})
	ctx := &types.AgentContext{
		Objective: "list names",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{MustInclude: must},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "Analyzer required-name count") || !contains(prompt, "(+5 more)") {
		t.Fatalf("prompt should keep exact count but cap the name preview:\n%s", prompt)
	}
	if contains(prompt, "Name34") || contains(prompt, "Name35") || contains(prompt, "Name36") {
		t.Fatalf("soft guidance names past the display cap should not be expanded:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_MemberSetSuppressesAnalyzerSoftGuidanceNames(t *testing.T) {
	mu := types.NewMutableState("list enum types")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		TerminalEvidenceCount: 1,
		AcceptedClosureReason: "complete set found; variable defaultExternalArtifactFloor was excluded",
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:        types.AnswerAggregateMemberSet,
			Label:       "verified enum members",
			Value:       "2",
			Members:     []string{"Intent", "Scenario"},
			SupportRefs: []string{"Intent: internal/types/analysis_ir.go:847", "Scenario: internal/types/analysis_ir.go:867"},
		}, {
			Kind:     types.AnswerAggregateExcluded,
			Label:    "excluded variables",
			Value:    "1",
			Excluded: []string{"defaultExternalArtifactFloor"},
		}},
	})
	ctx := &types.AgentContext{
		Objective: "list all enum types",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				AnswerExclusionPolicy: &types.AnswerExclusionPolicy{
					IsExclusionRequested: true,
					ExcludedCandidateRoles: []types.AnswerCandidateRole{
						types.AnswerCandidateRoleVariable,
					},
				},
			},
			AnswerContract: types.AnswerContract{MustInclude: []string{
				"Intent", "Scenario", "HelperThatShouldStaySoft",
			}},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "accepted `aggregate_facts.member_set`") {
		t.Fatalf("prompt should tell extractor that member_set supersedes analyzer soft names:\n%s", prompt)
	}
	if contains(prompt, "HelperThatShouldStaySoft") {
		t.Fatalf("analyzer soft guidance names must not be expanded once member_set is accepted:\n%s", prompt)
	}
	if !contains(prompt, "members_rendered_in=authoritative_principal_member_rows") {
		t.Fatalf("structured aggregate metadata should compact duplicate member rows:\n%s", prompt)
	}
	if !contains(prompt, "principal aggregate member_set obligations") ||
		!contains(prompt, "copy every member below into the answer-symbol slate") ||
		!contains(prompt, "`Intent` @ internal/types/analysis_ir.go:847") ||
		!contains(prompt, "`Scenario` @ internal/types/analysis_ir.go:867") {
		t.Fatalf("accepted principal member_set should be rendered as an explicit extractor slate obligation:\n%s", prompt)
	}
	if contains(prompt, "defaultExternalArtifactFloor") {
		t.Fatalf("principal member_set prompt should not project unstructured closure prose candidates:\n%s", prompt)
	}
	if !contains(prompt, "model-authored closure set-level summary") ||
		!contains(prompt, "[excluded candidate omitted]") ||
		!contains(prompt, "typed `aggregate_facts.member_set` rows/counts below remain the authoritative member carrier") {
		t.Fatalf("prompt should preserve sanitized tool-call closure prose as set-level advisory context:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_DeterministicEvidencePrefersTypedSurface(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{{
			Kind:         types.EvidenceConditional,
			Source:       "internal/agent/analyzer.go",
			LineStart:    861,
			AnchorKind:   types.AnchorCondition,
			AnchorSymbol: "buildAnalysisIR",
			Condition:    "ctx == nil || ctx.Mutable == nil",
			Summary:      "nil ctx definitely flowed in from the caller and skipped the guard",
		}},
	})
	ctx := &types.AgentContext{
		Objective: "why does this panic happen?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentRootCause},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "buildAnalysisIR guard condition IF ctx == nil || ctx.Mutable == nil") {
		t.Fatalf("prompt should render deterministic guard surface:\n%s", prompt)
	}
	if contains(prompt, "nil ctx definitely flowed in from the caller") {
		t.Fatalf("prompt should not reuse over-strong free summary in deterministic evidence section:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_DiagnosticSupportScopeFiltersTranscriptDigest(t *testing.T) {
	relevantCall := types.EvidenceItem{
		ID:              "path-call",
		Kind:            types.EvidenceRelationship,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1021,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    "buildAnalysisIR",
		Subject:         "ParseOutput",
		Object:          "buildAnalysisIR",
		Snippet:         "ir, err := buildAnalysisIR(ctx)",
		GroundingStatus: types.GroundingGrounded,
	}
	relevantDef := types.EvidenceItem{
		ID:              "build-def",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1271,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "buildAnalysisIR",
		Subject:         "buildAnalysisIR",
		Snippet:         "func buildAnalysisIR(ctx *types.AgentContext) (*types.AnalysisIR, error) {",
		GroundingStatus: types.GroundingGrounded,
	}
	unrelated := types.EvidenceItem{
		ID:              "unrelated-multigraph",
		Kind:            types.EvidenceDataflowPath,
		Scope:           types.ScopeLine,
		Source:          "internal/tool/repomap/multigraph/build.go",
		LineStart:       42,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    "New",
		Subject:         "MultiGraph.New",
		Object:          "MultiGraph.Topology",
		Snippet:         "return MultiGraph.New(nodes)",
		GroundingStatus: types.GroundingGrounded,
	}
	mu := types.NewMutableState("where did this panic come from?")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:          []string{"internal/agent/analyzer.go"},
		EvidenceItems:      []types.EvidenceItem{relevantCall, unrelated, relevantDef},
		FlowFindings:       []types.FlowFindingDigest{{ID: "flow-relevant", Path: []string{"ParseOutput", "buildAnalysisIR"}, EvidenceIDs: []string{"path-call"}}, {ID: "flow-unrelated", Path: []string{"MultiGraph.New", "Topology"}}},
		AcceptedResultKind: "resolved",
	})
	ctx := &types.AgentContext{
		Mutable:       mu,
		EvidenceItems: []types.EvidenceItem{relevantCall, unrelated, relevantDef},
		FlowFindings:  []types.FlowFindingDigest{{ID: "flow-relevant", Path: []string{"ParseOutput", "buildAnalysisIR"}, EvidenceIDs: []string{"path-call"}}, {ID: "flow-unrelated", Path: []string{"MultiGraph.New", "Topology"}}},
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Type: "runtime error: invalid memory address or nil pointer dereference",
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR"},
					{File: "internal/agent/analyzer.go", Line: 320, Func: "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput"},
				},
			}},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentRootCause,
				Scenario: types.ScenarioRootCause,
			},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "ParseOutput") || !contains(prompt, "buildAnalysisIR") {
		t.Fatalf("diagnostic transcript digest should keep support-linked evidence:\n%s", prompt)
	}
	if contains(prompt, "MultiGraph") {
		t.Fatalf("diagnostic transcript digest should filter unrelated typed evidence and flow rows:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_RendersValueBearingEvidenceLens(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:       types.EvidenceConcrete,
				Scope:      types.ScopeLine,
				Source:     "internal/types/analysis_ir.go",
				LineStart:  278,
				AnchorKind: types.AnchorAssignment,
				Snippet:    "case SubjectFunctionName, SubjectTypeName, SubjectConfigKey:",
				Subject:    "AnswerSubjectKind.IsValid",
				Predicate:  "validates",
				Summary:    "forced-read context value unrelated to the requested answer",
			},
			{
				Kind:       types.EvidenceDirect,
				Scope:      types.ScopeLine,
				Source:     "internal/analysis/compiler/templates.go",
				LineStart:  24,
				AnchorKind: types.AnchorDefinition,
				Snippet:    "TmplRetryBudgetMedium   = 3  // RetryBudget for mid-complexity",
				Subject:    "TmplRetryBudgetMedium",
				Predicate:  "defines",
				Summary:    "TmplRetryBudgetMedium 常量值为 3",
			},
			{
				Kind:         types.EvidenceDirect,
				Scope:        types.ScopeLine,
				Source:       "internal/analysis/compiler/templates.go",
				LineStart:    244,
				AnchorKind:   types.AnchorAssignment,
				AnchorSymbol: "RetryBudget",
				Snippet:      "RetryBudget: subTopicRetryBudgetBoost(rm, TmplRetryBudgetMedium),",
				Subject:      "templateArchitectureExplain",
				Predicate:    "configures",
				Object:       "RetryBudget",
			},
			{
				Kind:       types.EvidenceDirect,
				Scope:      types.ScopeLine,
				Source:     "internal/analysis/compiler/templates.go",
				LineStart:  182,
				AnchorKind: types.AnchorDefinition,
				Snippet:    "func templateArchitectureExplain(rm types.RequestModel) Output {",
				Subject:    "templateArchitectureExplain",
				Summary:    "free-form function definition note",
			},
		},
	})
	ctx := &types.AgentContext{
		Objective: "compare template retry budgets",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentExplain},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Value-bearing evidence facts",
		"TmplRetryBudgetMedium   = 3",
		"RetryBudget: subTopicRetryBudgetBoost(rm, TmplRetryBudgetMedium)",
		"Preserve exact constants",
		"soft visibility guidance",
	} {
		if !contains(prompt, want) {
			t.Fatalf("prompt missing value-bearing lens content %q:\n%s", want, prompt)
		}
	}
	valueSection := prompt[strings.Index(prompt, "### Value-bearing evidence facts"):]
	if strings.Contains(valueSection, "func templateArchitectureExplain") {
		t.Fatalf("plain function definition should not pollute value-bearing lens:\n%s", valueSection)
	}
	if strings.Contains(valueSection, "AnswerSubjectKind.IsValid") {
		t.Fatalf("unrelated forced-read value should not outrank request-relevant facts:\n%s", valueSection)
	}
}

func TestExtractor_BuildPrompt_ValueLensUsesTypedQuestionProfile(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:       types.EvidenceDirect,
				Scope:      types.ScopeLine,
				Source:     "pkg/config.go",
				LineStart:  10,
				AnchorKind: types.AnchorDefinition,
				Snippet:    "const DefaultTimeout = 30",
				Subject:    "DefaultTimeout",
			},
			{
				Kind:       types.EvidenceDirect,
				Scope:      types.ScopeLine,
				Source:     "pkg/config.go",
				LineStart:  20,
				AnchorKind: types.AnchorReturn,
				Snippet:    "return DefaultTimeout",
				Subject:    "DefaultTimeout",
			},
		},
	})
	ctx := &types.AgentContext{
		Objective: "what does DefaultTimeout return",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest:    "What does DefaultTimeout return?",
				Intent:        types.IntentReturnValue,
				PredicateAxis: types.AxisReturn,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectReturnValue},
			},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	sectionStart := strings.Index(prompt, "### Value-bearing evidence facts")
	if sectionStart < 0 {
		t.Fatalf("prompt missing value-bearing section:\n%s", prompt)
	}
	valueSection := prompt[sectionStart:]
	returnAt := strings.Index(valueSection, "return DefaultTimeout")
	defAt := strings.Index(valueSection, "const DefaultTimeout = 30")
	if returnAt < 0 || defAt < 0 {
		t.Fatalf("prompt missing return/definition facts:\n%s", valueSection)
	}
	if returnAt > defAt {
		t.Fatalf("return-value profile should rank return evidence before definition evidence:\n%s", valueSection)
	}
}

func TestExtractor_BuildPrompt_ValueLensDynamicLimitExpandsForMultiBucket(t *testing.T) {
	items := make([]types.EvidenceItem, 0, 24)
	for i := 0; i < 24; i++ {
		items = append(items, types.EvidenceItem{
			Kind:         types.EvidenceDirect,
			Scope:        types.ScopeLine,
			Source:       "pkg/config.go",
			LineStart:    i + 1,
			AnchorKind:   types.AnchorAssignment,
			AnchorSymbol: "CommonBudget",
			Snippet:      fmt.Sprintf("CommonBudget%d = %d", i, i),
			Subject:      "CommonBudget",
		})
	}
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: items})
	ctx := &types.AgentContext{
		Objective: "compare CommonBudget across Alpha and Beta",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "compare CommonBudget across Alpha and Beta",
				Intent:     types.IntentExplain,
				Scenario:   types.ScenarioArchitectureExplain,
				Complexity: types.ComplexityComplex,
				Predicates: types.SemanticPredicates{IsCrossComponent: true},
				SubTopics: []types.SubTopic{
					{Summary: "Alpha CommonBudget", Entities: []string{"CommonBudget"}},
					{Summary: "Beta CommonBudget", Entities: []string{"CommonBudget"}},
					{Summary: "Gamma CommonBudget", Entities: []string{"CommonBudget"}},
				},
				Buckets: []types.QuestionBucket{
					{Label: "Alpha", Anchors: []string{"CommonBudget"}, Index: 1},
					{Label: "Beta", Anchors: []string{"CommonBudget"}, Index: 2},
				},
			},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	sectionStart := strings.Index(prompt, "### Value-bearing evidence facts")
	if sectionStart < 0 {
		t.Fatalf("prompt missing value-bearing section:\n%s", prompt)
	}
	valueSection := prompt[sectionStart:]
	if got := strings.Count(valueSection, "CommonBudget"); got <= extractorDefaultValueFacts {
		t.Fatalf("dynamic multi-bucket limit did not expand beyond default; got %d entries:\n%s", got, valueSection)
	}
}

func TestExtractor_BuildPrompt_RendersAcceptedClosureAsStructuredHandoff(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedClosureReason: "grep + exec_command confirmed 4 production sites: analyzer.go:1903, contract_check.go:63, orchestrator.go:6362, orchestrator.go:6494; two comment hits were excluded",
		AcceptedResultKind:    "resolved",
		AcceptedAggregateFacts: []types.AnswerAggregateFact{
			{
				Kind:    types.AnswerAggregateTotalCount,
				Label:   "production sites",
				Value:   "4",
				Unit:    "locations",
				Members: []string{"internal/orchestrator/orchestrator.go:6362"},
			},
			{
				Kind:     types.AnswerAggregateExcluded,
				Label:    "comment hits",
				Value:    "2",
				Unit:     "lines",
				Excluded: []string{"internal/orchestrator/orchestrator.go:6396"},
			},
		},
		EvidenceItems: []types.EvidenceItem{{
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/orchestrator/orchestrator.go",
			LineStart:       6362,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "CitationReq",
			Snippet:         "CitationReq: types.CitationReq{Required: false}",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
		}},
	})
	ctx := &types.AgentContext{
		Objective: "count production sites",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Accepted exploration closure",
		"result_kind: `resolved`",
		"confirmed 4 production sites",
		"structured aggregate facts",
		"kind=`total_count`, label=production sites, value=`4`",
		"kind=`excluded_count`, label=comment hits, value=`2`",
		"orchestrator.go:6362",
		"Deterministic evidence the investigation extracted",
	} {
		if !contains(prompt, want) {
			t.Fatalf("prompt missing accepted closure handoff %q:\n%s", want, prompt)
		}
	}
	if !contains(prompt, "do not fall back to earlier stale notes") {
		t.Fatalf("prompt must tell extractor to prefer accepted closure over stale early notes:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_EnumerationBoundaryOverridesDisplayedFloor(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{TerminalEvidenceCount: 15})
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "What order do gate.Run's 7 checks execute in?",
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"gate.Run"},
				},
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 7,
					SourceQuote:   "7 checks",
				},
			},
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"checkCoverage", "checkDAGClosure"},
			},
		},
	}

	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "Hard typed floor") || !contains(prompt, "7 item(s), from requested boundary `7 checks`") {
		t.Fatalf("prompt must surface requested boundary override, got %q", prompt)
	}
	if !contains(prompt, "because this floor is typed and precise") {
		t.Fatalf("prompt must explain why this floor is hard, got %q", prompt)
	}
	if !contains(prompt, "MUST have") || !contains(prompt, "7 item(s)") {
		t.Fatalf("prompt must reuse the requested boundary as the completeness floor, got %q", prompt)
	}
}

func TestExtractor_BuildPrompt_AttributeBearingEnumerationSplitsCompletenessAxes(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{TerminalEvidenceCount: 40})
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "list all packages and each package entry point",
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				PredicateAxis: types.AxisDefine,
				AnalyzerHints: types.AnalyzerHints{
					PrimaryEntities: []string{"packages"},
					Entities:        []string{"aggregator", "compiler", "gate"},
				},
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 3,
					SourceQuote:   "all packages",
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all packages",
				},
			},
		},
	}

	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Two-axis enumeration",
		"Apply the `completeness` claim to the principal members only",
		"Attribute gaps do not make the member set incomplete",
		"Put the per-member attribute in `rationale`",
	} {
		if !contains(prompt, want) {
			t.Fatalf("attribute-bearing enumeration prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestExtractor_BuildPrompt_PlainCallChainSkipsAnswerSymbolFloor(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{TerminalEvidenceCount: 12})
	ctx := &types.AgentContext{
		Objective: "trace buildAnalysisIR to gate.Run",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
			},
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"compiler.Compile", "gate.RunWith"},
			},
		},
	}

	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)
	if contains(prompt, "Cardinality baseline") {
		t.Fatalf("plain call-chain prompt must not surface answer-symbol completeness floor: %q", prompt)
	}
	if contains(prompt, "emit_answer_symbol batch MUST have") {
		t.Fatalf("plain call-chain prompt must not push emit_answer_symbol completeness rules: %q", prompt)
	}
	if !contains(prompt, "This dispatch does NOT require `emit_answer_symbol`") {
		t.Fatalf("plain call-chain prompt must explicitly say emit_answer_symbol is inactive: %q", prompt)
	}
}

func TestExtractor_BuildPrompt_MultiTopicScalarLiteralSkipsAnswerSymbols(t *testing.T) {
	mu := types.NewMutableState("which emit tools")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{TerminalEvidenceCount: 2})
	ctx := &types.AgentContext{
		Objective: "analyzer agent and finalizer agent emit tool names",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				Scenario:      types.ScenarioArchitectureExplain,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
				SubTopics: []types.SubTopic{
					{Summary: "analyzer agent emit tool", Entities: []string{"AnalyzerAgent", "emit_analysis"}},
					{Summary: "finalizer agent emit tool", Entities: []string{"FinalizerAgent", "emit_answer_document"}},
				},
			},
		},
	}

	if types.IRAllowsAnchorSkeleton(ctx.AnalysisIR) {
		t.Fatal("multi-topic exact literal lookups must not activate anchor skeleton")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("multi-topic scalar literal answers must not force emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if contains(prompt, "## Anchor skeleton") || contains(prompt, "emit_answer_symbol batch MUST have") {
		t.Fatalf("prompt should not push symbol anchors for exact literal sub-topics:\n%s", prompt)
	}
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should explicitly disable answer-symbol slate for exact literal sub-topics:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_ImportEnumerationUsesTypedEvidenceLane(t *testing.T) {
	mu := types.NewMutableState("list imports")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:             []string{"internal/agent/explorer.go"},
		TerminalEvidenceCount: 1,
	})
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "import-types",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/explorer.go",
		LineStart:       26,
		AnchorKind:      types.AnchorImport,
		AnchorSymbol:    "types",
		Producer:        "explorer.emit_evidence",
		SurfaceTerms:    []string{"github.com/hanchaoqun/codrax/internal/types"},
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.AgentContext{
		Objective: "internal/agent/explorer.go import 了哪些 internal/ package？",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	if !viewNeedsEnumerationSlate(ctx) {
		t.Fatal("fixture must still be an enumeration-shaped answer")
	}
	if !enumerationPrincipalEvidenceRendersWithoutAnswerSymbols(ctx) {
		t.Fatal("typed import-edge principal evidence should render through the support lane")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("import-edge enumeration must not force a symbol-definition answer slate")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if contains(prompt, "Cardinality baseline") || contains(prompt, "emit_answer_symbol batch MUST have") {
		t.Fatalf("typed import enumeration must not surface symbol-slate cardinality rules:\n%s", prompt)
	}
	if !contains(prompt, "non-symbol evidence enumeration") {
		t.Fatalf("prompt must explain why emit_answer_symbol is inactive for non-symbol principal evidence:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_AggregateImportEnumerationUsesSupportLane(t *testing.T) {
	mu := types.NewMutableState("list aggregate imports")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:             []string{"src/main.ts"},
		TerminalEvidenceCount: 2,
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "internal import packages",
		Value:   "2",
		Members: []string{"@acme/internal/foo", "@acme/internal/bar"},
	}})
	mu.SetInvestigationComplete("aggregate import member set emitted")
	mu.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "import-foo",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "src/main.ts",
			LineStart:       3,
			AnchorKind:      types.AnchorImport,
			AnchorSymbol:    "@acme/internal/foo",
			Producer:        "explorer.emit_evidence",
			SurfaceTerms:    []string{"@acme/internal/foo"},
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "import-bar",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "src/main.ts",
			LineStart:       4,
			AnchorKind:      types.AnchorImport,
			AnchorSymbol:    "@acme/internal/bar",
			Producer:        "explorer.emit_evidence",
			SurfaceTerms:    []string{"@acme/internal/bar"},
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.AgentContext{
		Objective: "src/main.ts import 了哪些 @acme/internal package？",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all @acme/internal imports"},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	if !viewNeedsEnumerationSlate(ctx) {
		t.Fatal("fixture must still be an enumeration-shaped answer")
	}
	if !enumerationPrincipalEvidenceRendersWithoutAnswerSymbols(ctx) {
		t.Fatal("aggregate-backed import literals should render through the support lane")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("aggregate-backed import enumeration must not force a symbol-definition answer slate")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should disable answer-symbol slate when aggregate member_set carries import literals:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_CallRelationEnumerationUsesSupportLane(t *testing.T) {
	mu := types.NewMutableState("which production functions call helper")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:             []string{"internal/types/typed_relation_hint.go"},
		TerminalEvidenceCount: 2,
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "call-a",
				Kind:            types.EvidenceDirect,
				Scope:           types.ScopeLine,
				Source:          "internal/types/typed_relation_hint.go",
				LineStart:       182,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "appendTypedRelationKinds",
				Subject:         "BuildTypedRelationQueryWithResolvedSources",
				Object:          "appendTypedRelationKinds",
				Predicate:       "calls",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				ID:              "call-b",
				Kind:            types.EvidenceDirect,
				Scope:           types.ScopeLine,
				Source:          "internal/types/typed_relation_hint.go",
				LineStart:       209,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "appendTypedRelationKinds",
				Subject:         "TypedRelationKindsForRequest",
				Object:          "appendTypedRelationKinds",
				Predicate:       "calls",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	})
	mu.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "call-a",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/typed_relation_hint.go",
			LineStart:       182,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "appendTypedRelationKinds",
			Subject:         "BuildTypedRelationQueryWithResolvedSources",
			Object:          "appendTypedRelationKinds",
			Predicate:       "calls",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "call-b",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/typed_relation_hint.go",
			LineStart:       209,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "appendTypedRelationKinds",
			Subject:         "TypedRelationKindsForRequest",
			Object:          "appendTypedRelationKinds",
			Predicate:       "calls",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
		},
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "调用 appendTypedRelationKinds 的生产代码函数",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"BuildTypedRelationQueryWithResolvedSources", "TypedRelationKindsForRequest"},
	}})
	mu.SetInvestigationComplete("call relation member_set accepted")
	ctx := &types.AgentContext{
		Objective: "哪些生产代码函数调用了 appendTypedRelationKinds？",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				PredicateAxis: types.AxisCall,
				Predicates: types.SemanticPredicates{
					IsRelationalLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "call-a",
				Kind:            types.EvidenceDirect,
				Scope:           types.ScopeLine,
				Source:          "internal/types/typed_relation_hint.go",
				LineStart:       182,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "appendTypedRelationKinds",
				Subject:         "BuildTypedRelationQueryWithResolvedSources",
				Object:          "appendTypedRelationKinds",
				Predicate:       "calls",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				ID:              "call-b",
				Kind:            types.EvidenceDirect,
				Scope:           types.ScopeLine,
				Source:          "internal/types/typed_relation_hint.go",
				LineStart:       209,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "appendTypedRelationKinds",
				Subject:         "TypedRelationKindsForRequest",
				Object:          "appendTypedRelationKinds",
				Predicate:       "calls",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	if !viewNeedsEnumerationSlate(ctx) {
		t.Fatal("fixture must still be an enumeration-shaped answer")
	}
	if !relationAxisPrincipalEvidenceRendersWithoutAnswerSymbols(ctx) {
		t.Fatal("call-edge member_set should render through the relation/evidence support lane")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("call-edge enumeration must not force a symbol-definition answer slate")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "relation-edge evidence enumeration") ||
		contains(prompt, "emit_answer_symbol batch MUST have") {
		t.Fatalf("prompt should disable answer-symbol slate for call-edge member_set:\n%s", prompt)
	}
	if contains(prompt, "copy every member below into the answer-symbol slate") ||
		!contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("relation member_set prompt must not contradict the inactive answer-symbol contract:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_ChangeImpactFileEnumerationUsesSupportLane(t *testing.T) {
	mu := types.NewMutableState("list affected files")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:             []string{"internal/types/analysis_ir.go", "internal/analysis/gate/gate.go"},
		TerminalEvidenceCount: 2,
	})
	mu.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "def-citation-req",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       1243,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "CitationReq",
			Subject:         "CitationReq",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "guard-required",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/analysis/gate/gate.go",
			LineStart:       301,
			AnchorKind:      types.AnchorCondition,
			AnchorSymbol:    "CitationReq.Required",
			Subject:         "CitationReq.Required",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.AgentContext{
		Objective: "Which production files need changes if CitationReq.Required changes?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				ChangeImpactProfile: &types.ChangeImpactProfile{
					IsChangeImpact:  true,
					Target:          "CitationReq.Required",
					RequestedOutput: types.ImpactOutputFiles,
					Scope:           types.ImpactScopeProduction,
					Confidence:      0.9,
				},
			},
		},
	}

	if !viewNeedsEnumerationSlate(ctx) {
		t.Fatal("fixture must remain enumeration-shaped")
	}
	if !enumerationPrincipalEvidenceRendersWithoutAnswerSymbols(ctx) {
		t.Fatal("typed requested_output=files should render through support lanes even when evidence includes definitions")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("change-impact file enumeration must not force emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should explicitly disable answer-symbol slate for file-output impact answers:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_ChangeImpactAggregateMemberSetSkipsAnswerSymbol(t *testing.T) {
	mu := types.NewMutableState("list affected files from aggregate")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:             []string{"internal/agent/analyzer.go"},
		TerminalEvidenceCount: 0,
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "affected source members",
		Value: "1",
		Members: []string{
			"internal/agent/analyzer.go:1935-1949",
		},
	}})
	mu.SetInvestigationComplete("aggregate member set emitted")
	ctx := &types.AgentContext{
		Objective: "If ShapeValue is renamed, list affected files.",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				ChangeImpactProfile: &types.ChangeImpactProfile{
					IsChangeImpact:  true,
					Target:          "ShapeValue",
					RequestedOutput: types.ImpactOutputFiles,
					Scope:           types.ImpactScopeProduction,
					Confidence:      0.9,
				},
			},
		},
	}

	if !viewNeedsEnumerationSlate(ctx) {
		t.Fatal("fixture must remain enumeration-shaped")
	}
	if !enumerationPrincipalEvidenceRendersWithoutAnswerSymbols(ctx) {
		t.Fatal("model-emitted aggregate member_set should render through typed support lanes")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("aggregate-backed file enumeration must not force emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should disable answer-symbol slate when aggregate member_set carries non-symbol members:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_HistoryCurrentCodeMechanismSkipsAnswerSymbol(t *testing.T) {
	mu := types.NewMutableState("history plus current code mechanism")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{TerminalEvidenceCount: 4})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "相关 commit",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"40f36f93 Accept scalar aggregate count handoffs", "e4f15aa1 Treat scalar count boundaries as scope"},
	}, {
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "当前实现函数",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"NormalizeAggregateFactRolesForRequest", "AggregateMemberSetIsScalarCountSupport"},
	}})
	mu.SetInvestigationComplete("history commits explain why the current scalar chain works this way")
	ctx := &types.AgentContext{
		Objective: "从历史提交里找一次和 scalar 相关的改动，再结合当前代码解释现在链路怎么工作",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				Scenario:      types.ScenarioArchitectureExplain,
				Complexity:    types.ComplexityComplex,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.8},
				Predicates: types.SemanticPredicates{
					IsHistoryLookup:  true,
					IsCrossComponent: true,
				},
				AnalyzerHints:       types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				EnumerationBoundary: &types.RequestedEnumerationBoundary{DeclaredCount: 1, SourceQuote: "一次"},
			},
		},
	}

	if types.ResolveQuestionFamily(ctx.AnalysisIR.RequestModel) != types.QFArchitecture {
		t.Fatalf("fixture must route as architecture narrative, got %s", types.ResolveQuestionFamily(ctx.AnalysisIR.RequestModel))
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("history + current-code mechanism explanation must not force emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should explicitly disable answer-symbol slate for mixed history/current-code explanations:\n%s", prompt)
	}
	if contains(prompt, "list_of_symbols question") {
		t.Fatalf("mixed history/current-code explanation must not be framed as list_of_symbols:\n%s", prompt)
	}

	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	if types.ResolveQuestionFamily(ctx.AnalysisIR.RequestModel) != types.QFArchitecture {
		t.Fatalf("trace-labeled mixed history/current-code mechanism should still route as architecture narrative, got %s", types.ResolveQuestionFamily(ctx.AnalysisIR.RequestModel))
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("trace-labeled history + current-code mechanism explanation must not force emit_answer_symbol")
	}
}

func TestExtractor_BuildPrompt_PureHistoryEnumerationSkipsAnswerSymbol(t *testing.T) {
	mu := types.NewMutableState("最近 10 次提交都做了什么")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "git_log",
			Success:  true,
			Summary:  "[git_log: count=10 format=medium evidence_origin=vcs_metadata]\ncommit 3ae8465",
		}},
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "最近10次提交",
		Value:      "10",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: "git_log",
		Members: []string{
			"3ae8465 orchestrator: route retries through claim bindings",
			"125687ab orchestrator: suppress generic caveats after semantic pass",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	mu.SetInvestigationComplete("git_log provides the requested VCS metadata")
	ctx := &types.AgentContext{
		Objective: "最近 10 次提交都做了哪些事情？请逐个说明每次提交的作用和影响。",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
			},
		},
	}

	contract := types.CompileAnswerIntentContract(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract)
	if !contract.HasOrigin(types.AnswerEvidenceOriginVCSMetadata) ||
		contract.HasOrigin(types.AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("fixture must remain pure VCS metadata, got %+v", contract)
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("pure VCS history enumeration must not force emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should explicitly disable answer-symbol slate for pure VCS history:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_MixedVCSDiffCurrentSourceEnumerationUsesOriginLedger(t *testing.T) {
	mu := types.NewMutableState("compare diffs and current code")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "git_diff",
			Success:  true,
			Summary:  "[git_diff: evidence_origin=vcs_diff ref=HEAD~2..HEAD]\ndiff --git a/internal/agent/extractor.go b/internal/agent/extractor.go",
		}},
		ReadFiles: []string{"internal/agent/extractor.go"},
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "两个 diff 涉及的当前源码线索",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"git_diff: e645a3f1 touches internal/agent/extractor.go",
			"git_diff: c3b2a9d0 touches internal/tool/emit_hypothesis_verdict.go",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: string(types.AnswerEvidenceOriginVCSDiff)}},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		Objective: "对比最近两次 diff，并结合当前源码分析作用和影响",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentEnumerate,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup:       true,
					IsCategoryEnumeration: true,
				},
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					SourceQuotes:                        []string{"结合当前源码"},
					Confidence:                          0.9,
				},
			},
		},
	}

	if !viewNeedsEnumerationSlate(ctx) {
		t.Fatal("fixture must remain enumeration-shaped")
	}
	if !originSpecificAnswerRendersWithoutAnswerSymbols(ctx) {
		t.Fatal("typed VCS diff ledger should be enough for extractor handoff")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("mixed VCS/current-source explanation must not force repo answer_symbol rows for diff refs")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if contains(prompt, "list_of_symbols question") || contains(prompt, "Cardinality guidance") {
		t.Fatalf("mixed VCS/current-source prompt must not pressure symbol slate:\n%s", prompt)
	}
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should explicitly disable symbol slate for origin-specific principal rows:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_ExternalLedgerNarrativeSkipsAnchorSkeleton(t *testing.T) {
	mu := types.NewMutableState("external resource narrative")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateScalar,
			Label: "MCP contract conclusion",
			Value: "resource documents the contract",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			MemberNotes: []string{
				"MCP resource already carries the answer-grade narrative.",
			},
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginMCPResource)},
				{Name: "server", Value: "docs"},
				{Name: "resource_uri", Value: "mcp://docs/spec"},
				{Name: "json_pointer", Value: "/items/0/title"},
			},
		}},
	})
	ctx := &types.AgentContext{
		Objective: "根据 MCP 资源解释两个模块的关系",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				SubTopics: []types.SubTopic{
					{Summary: "模块 A"},
					{Summary: "模块 B"},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	if !requiresMultiTopicAnchorSkeleton(ctx) {
		t.Fatal("fixture must exercise the generic multi-topic anchor-skeleton path")
	}
	if !originSpecificAnswerRendersWithoutAnswerSymbols(ctx) {
		t.Fatal("accepted origin-specific ledger support should render without a repo answer-symbol slate")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("external MCP ledger narrative must not force current-source answer symbols")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should explicitly disable answer-symbol slate for external ledger narrative:\n%s", prompt)
	}
	if contains(prompt, "For each, call emit_answer_symbol") {
		t.Fatalf("prompt must not ask for repo anchors when external ledger already carries the answer:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_SourceInventoryAggregateSkipsAnswerSymbol(t *testing.T) {
	mu := types.NewMutableState("list public string enum types")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:             []string{"internal/types/analysis_ir.go"},
		TerminalEvidenceCount: 0,
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "internal/types public string enum types",
		Value:      "2",
		Role:       types.AnswerAggregateRolePrincipalAnswer,
		Provenance: "system:source_inventory",
		Members:    []string{"Intent", "Scenario"},
		MemberNotes: []string{
			"Intent describes the top-level request purpose.",
			"Scenario describes the answer workflow family.",
		},
		SupportRefs: []string{
			"Intent: internal/types/analysis_ir.go:847",
			"Scenario: internal/types/analysis_ir.go:867",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		Objective: "列出 internal/types 里的公开字符串枚举类型名",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "所有公开字符串枚举类型名",
				},
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
					TypeUnderlying:    types.SourceInventoryTypeUnderlyingString,
					RequiresConstSet:  true,
					RequestedFields: []types.SourceInventoryRequestedField{
						types.SourceInventoryFieldName,
						types.SourceInventoryFieldLocation,
						types.SourceInventoryFieldSummary,
					},
					Confidence: 0.95,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	if !viewNeedsEnumerationSlate(ctx) {
		t.Fatal("fixture must remain enumeration-shaped")
	}
	if !sourceInventoryPrincipalAggregateRendersWithoutAnswerSymbols(ctx) {
		t.Fatal("complete source-inventory aggregate rows should render without an answer-symbol slate")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("source-inventory aggregate rows must not force the model to re-emit 100+ answer symbols")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should disable answer-symbol slate for grounded source inventory aggregates:\n%s", prompt)
	}
	if !contains(prompt, "Intent: 2 member(s)") && !contains(prompt, "internal/types public string enum types: 2 member(s)") {
		t.Fatalf("prompt should still carry the structured aggregate handoff:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_DefinitionEnumerationStillRequiresAnswerSymbols(t *testing.T) {
	mu := types.NewMutableState("list functions")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:             []string{"internal/agent/explorer.go"},
		TerminalEvidenceCount: 1,
	})
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "def-explorer",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/explorer.go",
		LineStart:       42,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "newExplorerAgent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.AgentContext{
		Objective: "internal/agent/explorer.go 里有哪些函数？",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	if !needsAnswerSymbols(ctx) {
		t.Fatal("symbol-backed enumerations still need emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "Cardinality guidance") {
		t.Fatalf("definition enumeration should keep symbol-slate cardinality rules:\n%s", prompt)
	}
}

func TestExtractor_BuildPrompt_NoArtifacts_GracefulDegrade(t *testing.T) {
	// When Mutable exists but TurnAArtifacts is nil (unit test
	// bootstrap, or wiring bug), the prompt must still be usable and
	// must warn the LLM to set completeness=unknown.
	mu := types.NewMutableState("")
	ctx := &types.AgentContext{Objective: "q", Mutable: mu}
	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)

	if !contains(prompt, "No transcript available") {
		t.Error("prompt must announce missing transcript")
	}
	if !contains(prompt, "set `completeness` to `unknown`") {
		t.Error("prompt must instruct unknown fallback when no transcript")
	}
}

func TestExtractor_BuildPrompt_ClampsLongInvestigationNotes(t *testing.T) {
	// Prompt budget: more than 6 iterations must be clamped to the
	// 6 most recent. A single note longer than 1200 chars must be
	// truncated with an ellipsis. Both branches exercised here.
	longNote := strings.Repeat("x", 2000)
	notes := make([]string, 10)
	for i := range notes {
		notes[i] = fmt.Sprintf("iter %d: %s", i, longNote)
	}
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion:       "q",
		InvestigationNotes: notes,
	})
	ctx := &types.AgentContext{Objective: "q", Mutable: mu}
	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)

	if !contains(prompt, "showing the 6 most recent of 10") {
		t.Error("prompt must show clamp notice when notes > 6")
	}
	if !contains(prompt, "…") {
		t.Error("prompt must truncate over-long notes with ellipsis")
	}
}

func TestExtractor_BuildPrompt_IncludesHypothesisSet(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{UserQuestion: "q"})
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			HypothesisSet: []types.Hypothesis{
				{ID: "H1", Statement: "Foo calls Bar", Status: types.HypUnknown},
				{ID: "H2", Statement: "Bar returns true", Status: types.HypUnknown},
			},
		},
	}
	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)

	for _, want := range []string{"Hypotheses", "H1", "H2", "Foo calls Bar", "Bar returns true"} {
		if !contains(prompt, want) {
			t.Errorf("prompt missing hypothesis content %q", want)
		}
	}
}

func TestExtractor_Validator_EmptySlate_LeavesCompletenessZero(t *testing.T) {
	// len(syms)==0 short-circuits the drain: StageOutput.AnswerSymbols
	// stays nil and completeness stays at zero (CompletenessUnknown).
	// The builder then drops the section entirely.
	mu := types.NewMutableState("")
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
	}
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if len(out.AnswerSymbols) != 0 {
		t.Errorf("empty slate should not populate AnswerSymbols, got %d", len(out.AnswerSymbols))
	}
	if out.AnswerSymbolCompleteness != types.CompletenessUnknown {
		t.Errorf("empty slate completeness should be zero, got %q", out.AnswerSymbolCompleteness)
	}
}

func TestExtractor_ParseOutput_SkipsAutoVerdictsForObservationOnlyRuntimeArtifact(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Func: "Cart.itemAt",
				File: "src/cart/Cart.cj",
				Line: 78,
			}},
		}},
	}
	mu := types.NewMutableState("runtime panic")
	mu.SetLogTriage(logBundle)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{RuntimeObservationOnlyCompletion: true})
	ctx := &types.AgentContext{
		Objective: "runtime panic",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: logBundle,
			},
			HypothesisSet: []types.Hypothesis{{
				ID:                     "h1",
				Status:                 types.HypUnknown,
				FalsificationCondition: types.Criterion{Kind: types.CritNoCallSites, Expr: "Cart.itemAt"},
			}},
		},
	}
	e := &extractorEvaluator{}

	if _, err := e.ParseOutput(ctx, nil, nil, nil); err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if got := mu.EmittedHypothesisVerdicts(); len(got) != 0 {
		t.Fatalf("observation-only artifact should not get repo-evidence auto-verdicts, got %+v", got)
	}
}

func TestExtractor_ParseOutputHandlesNilMutable(t *testing.T) {
	// Defensive: sub-agent dispatch passes a context with Mutable nil.
	// The extractor is not a sub-agent target today, but the symmetry
	// with emit_evidence's "no Mutable → fail-safe" is worth pinning.
	e := &extractorEvaluator{}
	out, err := e.ParseOutput(&types.AgentContext{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput should not error on nil Mutable, got: %v", err)
	}
	if out == nil {
		t.Fatal("ParseOutput must return non-nil StageOutput even on nil Mutable")
	}
	if len(out.EvidenceItems) != 0 || len(out.AnswerSymbols) != 0 {
		t.Errorf("nil Mutable must produce empty slices, got: %+v", out)
	}
}

func TestExtractor_DetermineMissingPieceAlwaysNone(t *testing.T) {
	// Turn B never triggers a backtrack — the contract checker
	// downstream of finalize owns retry decisions. Returning
	// MissingNone keeps the extractor out of the orchestrator's
	// stage-routing branch.
	e := &extractorEvaluator{}
	if got := e.DetermineMissingPiece(nil, &StageOutput{}); got != types.MissingNone {
		t.Errorf("got %v, want MissingNone", got)
	}
}

// -----------------------------------------------------------------------------
// P2.1 Phase 12 — stage-local tool whitelist invariant
// -----------------------------------------------------------------------------
//
// The extractor's LLM call must see ONLY emit_evidence /
// emit_answer_symbol / emit_hypothesis_verdict. Phase 12 is not a
// new code path — it is a pin on the skill.ToolSuggestions mechanism
// already used by buildToolSchemas. This test constructs a BaseAgent
// with a tool registry containing both the allowed emit_* tools AND
// forbidden investigation tools (grep, read_file), then verifies
// buildToolSchemas returns exactly the allowed three.
//
// A regression here would mean either (a) ToolSuggestions filtering
// drifted, or (b) a new auto-injection source was added to
// buildToolSchemas without being gated by agent name. Either would
// silently give Turn B file access — the exact thing the two-turn
// split exists to prevent.

func TestExtractor_ToolSchemas_ExactlyThreeEmitTools(t *testing.T) {
	registry := tool.NewRegistry()
	// Register both allowed and forbidden tools to prove filtering.
	registry.Register(&tool.EmitEvidence{})
	registry.Register(&tool.EmitAnswerSymbol{})
	registry.Register(&tool.EmitHypothesisVerdict{})
	registry.Register(&tool.GrepTool{})
	registry.Register(&tool.ReadFile{})

	deps := &Dependencies{
		LLM:           nil,
		Tools:         registry,
		MaxIterations: 1,
	}
	agent := NewExtractorAgent(deps).(*BaseAgent)

	// Simulate what the orchestrator does: fetch the extract-skill
	// from the config layer. We hand-build the equivalent of what
	// cmd/root.go's P2.1 bootstrap appends: the three emit_* names.
	sk := &skill.Config{
		Name: "extract-skill",
		ToolSuggestions: []string{
			"emit_evidence",
			"emit_answer_symbol",
			"emit_hypothesis_verdict",
		},
	}

	schemas := agent.buildToolSchemas(sk, nil)

	got := make(map[string]bool, len(schemas))
	for _, s := range schemas {
		got[s.Name] = true
	}

	for _, want := range []string{"emit_evidence", "emit_answer_symbol", "emit_hypothesis_verdict"} {
		if !got[want] {
			t.Errorf("missing allowed tool %q from schemas", want)
		}
	}
	for _, forbidden := range []string{"grep", "read_file", "repo_map", "exec_command"} {
		if got[forbidden] {
			t.Errorf("forbidden tool %q leaked into schemas — Phase 12 scoping broken", forbidden)
		}
	}
	// Cardinality: no other tool should be in the schema (propose_sub_agents
	// is auto-injected only when a sub-agent of the same name exists; the
	// extractor has none, so the schema length must equal the allowlist).
	if len(schemas) != 3 {
		names := make([]string, 0, len(schemas))
		for _, s := range schemas {
			names = append(names, s.Name)
		}
		t.Errorf("extractor schema must contain exactly 3 tools, got %d: %v", len(schemas), names)
	}
}

func TestExtractor_ToolSchemas_EmptyToolSuggestions_YieldsEmpty(t *testing.T) {
	// Defensive: if the extract-skill's ToolSuggestions is empty
	// (config drift, or test path that skips cmd/root.go bootstrap),
	// buildToolSchemas must NOT inject any fallback investigation
	// tools. Empty in → empty out.
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerSymbol{})
	registry.Register(&tool.GrepTool{})

	deps := &Dependencies{Tools: registry, MaxIterations: 1}
	agent := NewExtractorAgent(deps).(*BaseAgent)
	sk := &skill.Config{Name: "extract-skill", ToolSuggestions: nil}

	schemas := agent.buildToolSchemas(sk, nil)
	if len(schemas) != 0 {
		t.Errorf("empty ToolSuggestions must produce empty schemas, got %d", len(schemas))
	}
}

func TestExtractor_RegisteredAsAgent(t *testing.T) {
	// The factory must produce something that satisfies the Agent
	// interface so registry.go can store it. This catches a typo in
	// NewExtractorAgent's return shape immediately, before any
	// orchestrator-level integration test runs.
	deps := &Dependencies{LLM: nil, Tools: nil, MaxIterations: 1}
	a := NewExtractorAgent(deps)
	if a == nil {
		t.Fatal("NewExtractorAgent returned nil")
	}
	if a.Name() != types.AgentExtractor {
		t.Errorf("name = %q, want %q", a.Name(), types.AgentExtractor)
	}
}

// (uses the package-level `contains` helper from explorer_test.go)

// -----------------------------------------------------------------------------
// Observe — mid-loop stop decision (option B)
// -----------------------------------------------------------------------------
//
// The mid-loop controller must stop ONLY when every EXPECTED emit has
// succeeded; missing expectations must return an empty signal so the
// loop runs another iteration. "Expected" = emit_answer_symbol iff
// intent is enumerate (the V2 carrier's enumeration slate),
// emit_hypothesis_verdict iff at least one hypothesis lacks a verdict
// in the buffer.

// observeMidLoopFixture assembles a minimal ctx + obs so each test
// can toggle one axis (intent, pending hypothesis, tool results) and
// assert on the signal. (Pre-PR5 the parameter was AnswerShape; the
// V2 carrier keys off intent/QuestionFamily, so the fixture switched
// to taking the resolved intent directly.)
func observeMidLoopFixture(intent types.Intent, hypotheses []types.Hypothesis, existingVerdicts []types.HypothesisVerdict, tools []types.ToolResult) (*types.AgentContext, LoopObservation) {
	mu := types.NewMutableState("")
	if len(existingVerdicts) > 0 {
		mu.AppendEmittedHypothesisVerdicts(existingVerdicts)
	}
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: intent},
			AnswerContract: types.AnswerContract{},
			HypothesisSet:  hypotheses,
		},
	}
	obs := LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      0,
		AllToolResults: tools,
	}
	return ctx, obs
}

func TestExtractor_Observe_MidLoop_BothExpected_BothSucceeded_Stops(t *testing.T) {
	ctx, obs := observeMidLoopFixture(
		types.IntentEnumerate,
		[]types.Hypothesis{{ID: "h1"}},
		nil, // no pre-injected verdict → h1 is pending
		[]types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true},
			{ToolName: "emit_hypothesis_verdict", Success: true},
		},
	)
	ctx.Mutable.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 5, Kind: "function"},
	}, types.CompletenessLowerBound)
	e := &extractorEvaluator{}
	sig := e.Observe(ctx, obs)
	if !sig.StopRequested {
		t.Fatalf("both expected emits succeeded → must StopRequested; got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_MissingVerdict_Continues(t *testing.T) {
	// Emblematic of the original bug: LLM only emitted symbols, didn't
	// batch verdict in parallel. Current code must NOT stop — the
	// loop runs another iteration so iter=1 can emit the verdict.
	ctx, obs := observeMidLoopFixture(
		types.IntentEnumerate,
		[]types.Hypothesis{{ID: "h1"}},
		nil,
		[]types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true},
		},
	)
	e := &extractorEvaluator{}
	sig := e.Observe(ctx, obs)
	if sig.StopRequested {
		t.Fatalf("missing emit_hypothesis_verdict → must NOT StopRequested; got %+v", sig)
	}
}

func TestExtractor_Observe_RuntimeArtifactOnly_DoesNotRequireHypothesisVerdict(t *testing.T) {
	ctx, obs := observeMidLoopFixture(
		types.IntentReturnValue,
		[]types.Hypothesis{{ID: "h1"}},
		nil,
		nil,
	)
	ctx.AnalysisIR.RequestModel.LogTriage = &types.LogBundle{
		Observations: []types.LogObservation{{
			Kind:       types.LogObservationRuntimeEvent,
			Summary:    "WARN appears on attached log line 3",
			Confidence: 1,
		}},
	}
	e := &extractorEvaluator{}
	sig := e.Observe(ctx, obs)
	if !sig.StopRequested {
		t.Fatalf("observation-only runtime artifacts should not force emit_hypothesis_verdict; got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_MissingSymbols_Continues(t *testing.T) {
	// Symmetric case: LLM emitted verdict but forgot symbols.
	ctx, obs := observeMidLoopFixture(
		types.IntentEnumerate,
		[]types.Hypothesis{{ID: "h1"}},
		nil,
		[]types.ToolResult{
			{ToolName: "emit_hypothesis_verdict", Success: true},
		},
	)
	e := &extractorEvaluator{}
	sig := e.Observe(ctx, obs)
	if sig.StopRequested {
		t.Fatalf("missing emit_answer_symbol → must NOT StopRequested; got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_RelationMemberSetNoOpSymbolsStops(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "appendTypedRelationKinds callers",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"BuildTypedRelationQueryWithResolvedSources", "TypedRelationKindsForRequest"},
		SupportRefs: []string{
			"BuildTypedRelationQueryWithResolvedSources: internal/types/typed_relation_hint.go:182",
			"TypedRelationKindsForRequest: internal/types/typed_relation_hint.go:209",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("relation member_set handoff should render without answer-symbol slate")
	}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		AllToolResults: []types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true, Summary: "ignored"},
		},
	}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if !sig.StopRequested {
		t.Fatalf("successful no-op answer_symbol with no pending verdicts should stop, got %+v", sig)
	}
	if sig.HintRequested {
		t.Fatalf("relation member_set no-op must not request symbol materialization, got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_RelationEvidenceNoOpWhenAnalyzerMissesRelation(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "appendTypedRelationKinds callers",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"TypedRelationKindsForRequest"},
		SupportRefs: []string{"TypedRelationKindsForRequest: internal/types/typed_relation_hint.go:209"},
	}})
	mu.RetainInvestigationAggregateFacts()
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "call-site",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/typed_relation_hint.go",
		LineStart:       209,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    "appendTypedRelationKinds",
		Subject:         "TypedRelationKindsForRequest",
		Object:          "appendTypedRelationKinds",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("relation evidence should disable answer-symbol slate even when analyzer missed relation flags")
	}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		AllToolResults: []types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true, Summary: "ignored"},
		},
	}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if !sig.StopRequested || sig.HintRequested {
		t.Fatalf("relation evidence no-op should stop without materialization retry, got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_RelationMemberSetNoOpWaitsForVerdictOnly(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "appendTypedRelationKinds callers",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"TypedRelationKindsForRequest"},
		SupportRefs: []string{"TypedRelationKindsForRequest: internal/types/typed_relation_hint.go:209"},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsRelationalLookup: true,
				},
			},
			AnswerContract: types.AnswerContract{},
			HypothesisSet:  []types.Hypothesis{{ID: "h1"}},
		},
	}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		AllToolResults: []types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true, Summary: "ignored"},
			{ToolName: "emit_hypothesis_verdict", Success: false, Summary: "citation required"},
		},
	}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if sig.StopRequested {
		t.Fatalf("pending verdict should still keep extractor open, got %+v", sig)
	}
	if sig.HintRequested {
		t.Fatalf("pending verdict path must not ask for answer_symbol materialization, got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_RejectedVerdictOverrideIsRepairedDespiteAutoVerdict(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{{
		HypothesisID: "h1",
		Status:       types.HypInconclusive,
		Rationale:    "deterministic fallback before extractor",
	}})
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			HypothesisSet: []types.Hypothesis{{ID: "h1"}},
		},
	}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		AllToolResults: []types.ToolResult{
			{
				ToolName: "emit_hypothesis_verdict",
				Success:  false,
				Summary:  "items[0]: status \"confirmed\" requires a citation",
			},
		},
	}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if sig.StopRequested {
		t.Fatalf("rejected model-authored verdict must not be masked by older auto-verdict, got %+v", sig)
	}
	if !sig.HintRequested {
		t.Fatalf("expected bounded repair hint for rejected verdict override, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "citation") || !strings.Contains(sig.Hint, "inconclusive") {
		t.Fatalf("repair hint should explain anchored or inconclusive options, got %q", sig.Hint)
	}
}

func TestExtractor_Observe_MidLoop_BoundedStepListMissingSymbols_Continues(t *testing.T) {
	mu := types.NewMutableState("q")
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "What order do gate.Run's 7 checks execute in?",
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"gate.Run"},
				},
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 7,
					SourceQuote:   "7 checks",
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		AllToolResults: []types.ToolResult{
			{ToolName: "emit_hypothesis_verdict", Success: true},
		},
	}
	e := &extractorEvaluator{}
	sig := e.Observe(ctx, obs)
	if sig.StopRequested {
		t.Fatalf("bounded principal step_list still expects emit_answer_symbol; got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_BoundedStepListPartialSymbols_Hints(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "checkCoverage", File: "internal/analysis/gate/gate.go", Line: 127, Kind: "function"},
		{Name: "checkDAGClosure", File: "internal/analysis/gate/gate.go", Line: 128, Kind: "function"},
	}, types.CompletenessLowerBound)
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "What order do gate.Run's 7 checks execute in?",
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"gate.Run"},
				},
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 7,
					SourceQuote:   "7 checks",
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		AllToolResults: []types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true},
		},
	}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if !sig.HintRequested {
		t.Fatalf("partial bounded slate should trigger a materialization hint; got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "full principal member slate") {
		t.Fatalf("hint must ask for the full principal member slate, got %q", sig.Hint)
	}
}

func TestExtractor_Observe_MidLoop_AutoVerdictAlreadyInBuffer_StopsOnSymbolsOnly(t *testing.T) {
	// Dominant production case: orchestrator pre-injected an auto-verdict
	// before extractor ran, so hasPendingHypotheses=false. The LLM only
	// needs to emit symbols; mid-loop stops after that single tool.
	ctx, obs := observeMidLoopFixture(
		types.IntentEnumerate,
		[]types.Hypothesis{{ID: "h1"}},
		[]types.HypothesisVerdict{{HypothesisID: "h1", Status: types.HypInconclusive}},
		[]types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true},
		},
	)
	ctx.Mutable.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 5, Kind: "function"},
	}, types.CompletenessLowerBound)
	e := &extractorEvaluator{}
	sig := e.Observe(ctx, obs)
	if !sig.StopRequested {
		t.Fatalf("auto-verdict filled → symbols-only emit must stop; got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_NonListShapeNoHypothesis_NothingExpected_AnySuccessStops(t *testing.T) {
	// No expected emits at all (shape=explanation, empty hypothesis
	// set). Any success — or even none — satisfies "everything
	// expected is done", so stop.
	ctx, obs := observeMidLoopFixture(
		types.IntentExplain,
		nil,
		nil,
		nil,
	)
	e := &extractorEvaluator{}
	sig := e.Observe(ctx, obs)
	if !sig.StopRequested {
		t.Fatalf("no expected emits → must stop; got %+v", sig)
	}
}

func TestExtractor_Observe_MidLoop_UnsupportedToolDoesNotCompleteStage(t *testing.T) {
	ctx, obs := observeMidLoopFixture(
		types.IntentExplain,
		nil,
		nil,
		[]types.ToolResult{
			{ToolName: "read_file", Success: false, Summary: "tool is not available in stage extract"},
		},
	)
	obs.Response = llm.Response{ToolCalls: []llm.ToolCall{{Name: "read_file"}}}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if sig.StopRequested {
		t.Fatalf("unsupported extractor tool call must not complete the stage: %+v", sig)
	}
	if !sig.HintRequested {
		t.Fatalf("unsupported extractor tool call should receive a stage-boundary hint: %+v", sig)
	}
	for _, want := range []string{"read_file", "emit_answer_symbol", "do not read/search"} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("unsupported-tool hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestExtractor_Observe_MidLoop_UnsupportedToolAfterValidEmitDoesNotBlockStop(t *testing.T) {
	ctx, obs := observeMidLoopFixture(
		types.IntentEnumerate,
		nil,
		nil,
		[]types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true},
			{ToolName: "read_file", Success: false, Summary: "tool is not available in stage extract"},
		},
	)
	ctx.Mutable.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "Foo", File: "a.go", Line: 5, Kind: "function"},
	}, types.CompletenessLowerBound)
	obs.Response = llm.Response{ToolCalls: []llm.ToolCall{{Name: "emit_answer_symbol"}, {Name: "read_file"}}}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if !sig.StopRequested {
		t.Fatalf("valid structured emit should not be blocked by an extra failed unavailable call: %+v", sig)
	}
}

// -----------------------------------------------------------------------------
// Observe — soft-stop correction (option B)
// -----------------------------------------------------------------------------

func TestExtractor_Observe_SoftStop_MissingVerdict_InjectsHint(t *testing.T) {
	// LLM stopped without tool calls while h1 is still pending → hint
	// must fire, retriesUsed++.
	mu := types.NewMutableState("")
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			HypothesisSet:  []types.Hypothesis{{ID: "h1"}},
		},
	}
	obs := LoopObservation{Phase: PhaseSoftStop, Iteration: 1}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if !sig.HintRequested {
		t.Fatalf("pending hypothesis + soft-stop → hint expected; got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "emit_hypothesis_verdict") {
		t.Errorf("hint must name emit_hypothesis_verdict; got %q", sig.Hint)
	}
	if e.retriesUsed != 1 {
		t.Errorf("retriesUsed must increment to 1, got %d", e.retriesUsed)
	}
}

func TestExtractor_Observe_SoftStop_RetryBudgetExhausted_NoHint(t *testing.T) {
	// retriesUsed already at cap → no new hint, accept soft-stop.
	mu := types.NewMutableState("")
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			HypothesisSet:  []types.Hypothesis{{ID: "h1"}},
		},
	}
	obs := LoopObservation{Phase: PhaseSoftStop, Iteration: 2}
	e := &extractorEvaluator{maxRetries: 1, retriesUsed: 1}
	sig := e.Observe(ctx, obs)
	if sig.HintRequested {
		t.Errorf("retry budget exhausted → must NOT inject hint; got %+v", sig)
	}
}

func TestExtractor_Observe_SoftStop_NothingMissing_NoHint(t *testing.T) {
	// Symbols already in buffer AND all hypotheses verdicted → no
	// correction needed, accept soft-stop.
	mu := types.NewMutableState("")
	mu.SetEmittedAnswerSymbols(
		[]types.AnswerSymbol{{Name: "Foo", File: "a.go", Line: 5, Kind: "function"}},
		types.CompletenessComplete,
	)
	mu.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{
		{HypothesisID: "h1", Status: types.HypInconclusive},
	})
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			HypothesisSet:  []types.Hypothesis{{ID: "h1"}},
		},
	}
	obs := LoopObservation{Phase: PhaseSoftStop, Iteration: 1}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if sig.HintRequested {
		t.Errorf("nothing missing → must NOT inject hint; got %+v", sig)
	}
}

func TestExtractor_Observe_SoftStop_BoundedStepListMissingSymbols_Hint(t *testing.T) {
	mu := types.NewMutableState("q")
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "What order do gate.Run's 7 checks execute in?",
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"gate.Run"},
				},
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 7,
					SourceQuote:   "7 checks",
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	obs := LoopObservation{Phase: PhaseSoftStop, Iteration: 1}
	e := &extractorEvaluator{maxRetries: 1}
	sig := e.Observe(ctx, obs)
	if !sig.HintRequested {
		t.Fatalf("bounded step_list without symbols should trigger a correction hint; got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "principal member slate") {
		t.Fatalf("hint must name the principal member slate path, got %q", sig.Hint)
	}
}

// -----------------------------------------------------------------------------
// hasPendingHypotheses — B-lenient invariant
// -----------------------------------------------------------------------------

func TestHasPendingHypotheses_EmptyHypothesisSet_False(t *testing.T) {
	mu := types.NewMutableState("")
	ctx := &types.AgentContext{Mutable: mu, AnalysisIR: &types.AnalysisIR{}}
	if hasPendingHypotheses(ctx) {
		t.Error("empty HypothesisSet → must return false")
	}
}

func TestHasPendingHypotheses_AllVerdicted_False(t *testing.T) {
	mu := types.NewMutableState("")
	mu.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{
		{HypothesisID: "h1", Status: types.HypInconclusive},
		{HypothesisID: "h2", Status: types.HypConfirmed},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			HypothesisSet: []types.Hypothesis{{ID: "h1"}, {ID: "h2"}},
		},
	}
	if hasPendingHypotheses(ctx) {
		t.Error("all hypotheses verdicted → must return false")
	}
}

func TestHasPendingHypotheses_SomePending_True(t *testing.T) {
	mu := types.NewMutableState("")
	mu.AppendEmittedHypothesisVerdicts([]types.HypothesisVerdict{
		{HypothesisID: "h1", Status: types.HypInconclusive},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			HypothesisSet: []types.Hypothesis{{ID: "h1"}, {ID: "h2"}},
		},
	}
	if !hasPendingHypotheses(ctx) {
		t.Error("h2 has no verdict → must return true")
	}
}

// TestExtractor_MultiTopicExplanationTriggersAnchorSkeleton pins the
// 2026-04-17 change: shape=explanation with sub_topics ≥ 1 renders
// an "Anchor skeleton" section in the extractor prompt naming each
// sub-topic, and expects emit_answer_symbol as a required emit
// (observable via the missing-symbols correction hint).
func TestExtractor_MultiTopicExplanationTriggersAnchorSkeleton(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			SubTopics: []types.SubTopic{
				{Summary: "How explorer invokes sub-agents", Entities: []string{"explorer"}},
				{Summary: "SubExplorer execution path", Entities: []string{"SubExplorer"}},
			},
		},
		AnswerContract: types.AnswerContract{},
	}
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{"a.go"}})
	ctx := &types.AgentContext{
		AnalysisIR: ir,
		Mutable:    mut,
	}

	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "Anchor skeleton") {
		t.Errorf("multi-topic explanation prompt must include Anchor skeleton section: %q", prompt)
	}
	if !contains(prompt, "How explorer invokes sub-agents") {
		t.Errorf("prompt must list first sub-topic summary: %q", prompt)
	}
	if !contains(prompt, "SubExplorer execution path") {
		t.Errorf("prompt must list second sub-topic summary: %q", prompt)
	}

	// isMultiTopicExplanation must also classify this as needing emit_answer_symbol.
	if !isMultiTopicExplanation(ctx) {
		t.Errorf("isMultiTopicExplanation must return true for shape=explanation + 2 sub_topics")
	}
}

func TestExtractor_HistorySubTopicsStayOptionalStructure(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:        types.IntentExplain,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			SubTopics: []types.SubTopic{
				{Summary: "latest merge", Entities: []string{"mergeCommit"}},
				{Summary: "commit impact", Entities: []string{"ApplyPatch"}},
			},
		},
		AnswerContract: types.AnswerContract{},
	}
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{"internal/agent/explorer.go"}})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/agent/explorer.go",
		LineStart:       3572,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "mergeCommit",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.AgentContext{
		AnalysisIR: ir,
		Mutable:    mut,
	}

	if !isMultiTopicExplanation(ctx) {
		t.Fatal("history sub-topics should still be rendered as optional structure guidance")
	}
	if requiresMultiTopicAnchorSkeleton(ctx) {
		t.Fatal("history sub-topics must not force emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "## Optional sub-topic structure") {
		t.Fatalf("history prompt should keep optional sub-topic structure:\n%s", prompt)
	}
	if contains(prompt, "## Anchor skeleton") ||
		contains(prompt, "For each, call emit_answer_symbol with ONE anchor symbol") ||
		contains(prompt, "Grounded anchor candidates already compiled") {
		t.Fatalf("history prompt must not promote sub-topics into code-anchor skeletons:\n%s", prompt)
	}
}

func TestExtractor_ArchitectureMultiTopicSubtopicsAreOptionalStructure(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			SubTopics: []types.SubTopic{
				{Summary: "read-mode retry loop", Entities: []string{"runAnalyzePhase"}},
				{Summary: "write-mode fallback context", Entities: []string{"fallbackWriteAnalysisIR"}},
			},
		},
		AnswerContract: types.AnswerContract{},
	}
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{"internal/orchestrator/orchestrator.go"}})
	ctx := &types.AgentContext{
		AnalysisIR: ir,
		Mutable:    mut,
	}

	if !isMultiTopicExplanation(ctx) {
		t.Fatal("architecture sub-topics may still render optional structure context")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("architecture/mechanism sub-topics must not force emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "Optional sub-topic structure") {
		t.Fatalf("prompt should keep sub-topic guidance visible without a hard symbol slate:\n%s", prompt)
	}
	if contains(prompt, "Hard typed floor") && contains(prompt, "from analyzer-resolved sub-topic count") {
		t.Fatalf("architecture sub-topics must not become a hard answer-symbol floor:\n%s", prompt)
	}
	if contains(prompt, "For each, call emit_answer_symbol with ONE anchor symbol") {
		t.Fatalf("architecture sub-topic prompt must not tell the model to emit one anchor per sub-topic:\n%s", prompt)
	}
}

func TestExtractor_MechanismCoverageObligationDoesNotForceSymbolSlate(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest:    "说明分析阶段重试机制，并覆盖 MaxRetriesPerStage、dynamicAnalyzeRetries、StageOutput/Error 的关系",
			Intent:        types.IntentExplain,
			Scenario:      types.ScenarioArchitectureExplain,
			PredicateAxis: types.AxisCondition,
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
			SubTopics: []types.SubTopic{
				{Summary: "retry budget", Entities: []string{"MaxRetriesPerStage", "dynamicAnalyzeRetries"}},
				{Summary: "stage output", Entities: []string{"StageOutput", "Error"}},
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "覆盖 MaxRetriesPerStage、dynamicAnalyzeRetries、StageOutput/Error 的关系",
			},
		},
		AnswerContract: types.AnswerContract{},
	}
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{"internal/orchestrator/orchestrator.go"}})
	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: mut}

	if got := types.ResolveQuestionFamily(ir.RequestModel); got != types.QFGeneric {
		t.Fatalf("coverage-only mechanism obligation routed to %s, want generic", got)
	}
	if types.IRAllowsAnchorSkeleton(ir) {
		t.Fatal("coverage-only mechanism sub-topics must not activate anchor skeleton")
	}
	if needsAnswerSymbols(ctx) {
		t.Fatal("coverage-only mechanism obligation must not force emit_answer_symbol")
	}
	prompt := (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "does NOT require `emit_answer_symbol`") {
		t.Fatalf("prompt should explicitly disable answer-symbol slate:\n%s", prompt)
	}
	if contains(prompt, "## Anchor skeleton") ||
		contains(prompt, "For each, call emit_answer_symbol with ONE anchor symbol") ||
		contains(prompt, "from analyzer-resolved sub-topic count") {
		t.Fatalf("coverage-only mechanism prompt must not surface anchor skeleton pressure:\n%s", prompt)
	}
}

func TestExtractor_MultiTopicExplanationPromptReusesCompiledAnchorBackbone(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			SubTopics: []types.SubTopic{
				{Summary: "Criterion 的角色", Entities: []string{"Criterion"}},
				{Summary: "Hypothesis 的角色", Entities: []string{"Hypothesis"}},
			},
		},
		AnswerContract: types.AnswerContract{},
	}
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{"internal/types/analysis_ir.go"}})
	ctx := &types.AgentContext{
		AnalysisIR: ir,
		Mutable:    mut,
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/types/analysis_ir.go",
				LineStart:       574,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "Criterion",
				Summary:         "Criterion 定义 hypothesis criterion 的结构。",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/types/analysis_ir.go",
				LineStart:       896,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "Hypothesis",
				Summary:         "Hypothesis 表示单条假设。",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "Grounded anchor candidates already compiled") {
		t.Fatalf("prompt must surface compiled anchor backbone: %q", prompt)
	}
	if !contains(prompt, "Criterion") || !contains(prompt, "internal/types/analysis_ir.go:574") {
		t.Fatalf("prompt must expose the first compiled anchor candidate: %q", prompt)
	}
	if !contains(prompt, "Hypothesis") || !contains(prompt, "internal/types/analysis_ir.go:896") {
		t.Fatalf("prompt must expose the second compiled anchor candidate: %q", prompt)
	}
}

func TestExtractor_BoundedPrincipalStepListPromptReusesCompiledCandidates(t *testing.T) {
	ir := &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "What order do gate.Run's 7 checks execute in?",
			AnalyzerHints: types.AnalyzerHints{
				MentionedEntities: []string{"gate.Run"},
			},
			EnumerationBoundary: &types.RequestedEnumerationBoundary{
				DeclaredCount: 7,
				SourceQuote:   "7 checks",
			},
		},
		AnswerContract: types.AnswerContract{},
	}
	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{"internal/analysis/gate/gate.go"}})
	ctx := &types.AgentContext{
		AnalysisIR: ir,
		Mutable:    mut,
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       127,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkCoverage",
				Summary:         "checkCoverage is appended as check 1",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       128,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkDAGClosure",
				Summary:         "checkDAGClosure is appended as check 2",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       129,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkBudgetSanity",
				Summary:         "checkBudgetSanity is appended as check 3",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	e := &extractorEvaluator{}
	prompt := e.BuildInitialInstruction(ctx, nil)
	if !contains(prompt, "## Principal member slate") {
		t.Fatalf("bounded step_list prompt must include principal member slate section: %q", prompt)
	}
	if !contains(prompt, "`7 checks` (7 item(s))") {
		t.Fatalf("prompt must surface the requested boundary: %q", prompt)
	}
	if !contains(prompt, "`checkCoverage` @ internal/analysis/gate/gate.go:127") {
		t.Fatalf("prompt must expose compiled candidate pool: %q", prompt)
	}
	if !viewNeedsBoundedPrincipalList(ctx) {
		t.Fatal("viewNeedsBoundedPrincipalList must return true for step_list + explicit boundary + single owner")
	}
}

// TestExtractor_SingleTopicExplanationNoSkeleton — shape=explanation
// without sub_topics is a single-topic question; the old "summary is
// the answer" path applies and no skeleton is emitted.
func TestExtractor_SingleTopicExplanationNoSkeleton(t *testing.T) {
	ir := &types.AnalysisIR{
		AnswerContract: types.AnswerContract{},
	}
	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: types.NewMutableState("q")}
	if isMultiTopicExplanation(ctx) {
		t.Errorf("isMultiTopicExplanation must be false without sub_topics")
	}
}

func TestExtractor_ParseOutput_EmptySlateFallsBackToDeclarativeLiterals(t *testing.T) {
	mu := types.NewMutableState("Which skills are registered by default?")
	mu.SetRequestModel(types.RequestModel{
		Intent:        types.IntentEnumerate,
		PredicateAxis: types.AxisRegister,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
	})
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		Objective: "Which skills are registered by default?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:      types.EvidenceRegistration,
				Object:    "analysis-skill",
				Summary:   `RegisterDefaults registers "analysis-skill"`,
				Source:    "internal/skill/defaults.go",
				LineStart: 12,
			},
			{
				Kind:      types.EvidenceRegistration,
				Object:    "explore-skill",
				Summary:   `RegisterDefaults registers "explore-skill"`,
				Source:    "internal/skill/defaults.go",
				LineStart: 13,
			},
		},
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Fatalf("fallback slate should be lower_bound, got %q", out.AnswerSymbolCompleteness)
	}
	if len(out.AnswerSymbols) != 2 {
		t.Fatalf("fallback should synthesize two literal symbols, got %+v", out.AnswerSymbols)
	}
	got := make(map[string]bool, len(out.AnswerSymbols))
	for _, sym := range out.AnswerSymbols {
		got[sym.Name] = true
		if sym.Kind != types.KindLiteral {
			t.Fatalf("fallback symbol %q should be literal, got %q", sym.Name, sym.Kind)
		}
	}
	for _, want := range []string{"analysis-skill", "explore-skill"} {
		if !got[want] {
			t.Fatalf("fallback slate missing %q: %+v", want, out.AnswerSymbols)
		}
	}
}

func TestExtractor_ParseOutput_EmptySlateFallsBackToPrincipalDefinitionSymbols(t *testing.T) {
	mu := types.NewMutableState("List all public string enum type names.")
	rm := types.RequestModel{
		Intent:        types.IntentEnumerate,
		AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectTypeName},
	}
	mu.SetRequestModel(rm)
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		Objective: "List all public string enum type names.",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   rm,
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "enum-intent",
				Kind:            types.EvidenceDirect,
				Scope:           types.ScopeLine,
				Source:          "internal/types/analysis_ir.go",
				LineStart:       642,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "Intent",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				ID:              "enum-complexity",
				Kind:            types.EvidenceDirect,
				Scope:           types.ScopeLine,
				Source:          "internal/types/analysis_ir.go",
				LineStart:       678,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "Complexity",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Fatalf("definition fallback should preserve an honest lower_bound claim, got %q", out.AnswerSymbolCompleteness)
	}
	if len(out.AnswerSymbols) != 2 {
		t.Fatalf("definition fallback should synthesize both principal symbols, got %+v", out.AnswerSymbols)
	}
	got := make(map[string]types.AnswerSymbol, len(out.AnswerSymbols))
	for _, sym := range out.AnswerSymbols {
		got[sym.Name] = sym
		if sym.Kind != types.KindType {
			t.Fatalf("type-name definition fallback should emit kind=type, got %+v", sym)
		}
	}
	for _, want := range []string{"Intent", "Complexity"} {
		if got[want].Line == 0 {
			t.Fatalf("definition fallback missing %q: %+v", want, out.AnswerSymbols)
		}
	}
}

func TestExtractor_ParseOutput_PrunesDeclarativeHelperSymbols(t *testing.T) {
	mu := types.NewMutableState("Which skills are registered by default?")
	mu.SetRequestModel(types.RequestModel{
		Intent:        types.IntentEnumerate,
		PredicateAxis: types.AxisRegister,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
	})
	mu.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "RegisterDefaults", File: "internal/skill/defaults.go", Line: 8, Kind: types.KindFunction},
		{Name: "analysis-skill", File: "internal/skill/defaults.go", Line: 12, Kind: types.KindLiteral},
		{Name: "Registry", File: "internal/skill/defaults.go", Line: 2, Kind: types.KindStruct},
		{Name: "explore-skill", File: "internal/skill/defaults.go", Line: 13, Kind: types.KindLiteral},
	}, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		Objective: "Which skills are registered by default?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:      types.EvidenceRegistration,
				Object:    "analysis-skill",
				Summary:   `RegisterDefaults registers "analysis-skill"`,
				Source:    "internal/skill/defaults.go",
				LineStart: 12,
			},
			{
				Kind:      types.EvidenceRegistration,
				Object:    "explore-skill",
				Summary:   `RegisterDefaults registers "explore-skill"`,
				Source:    "internal/skill/defaults.go",
				LineStart: 13,
			},
		},
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Fatalf("pruned declarative slate should be lower_bound, got %q", out.AnswerSymbolCompleteness)
	}
	if len(out.AnswerSymbols) != 2 {
		t.Fatalf("helper pruning should leave only the literal terminals, got %+v", out.AnswerSymbols)
	}
	for _, sym := range out.AnswerSymbols {
		if sym.Kind != types.KindLiteral {
			t.Fatalf("pruned symbol %q should stay literal, got %q", sym.Name, sym.Kind)
		}
		if sym.Name != "analysis-skill" && sym.Name != "explore-skill" {
			t.Fatalf("helper symbol %q should have been pruned", sym.Name)
		}
	}
}

func TestExtractor_ParseOutput_EmptySlateFallsBackForGenericRegistrationLists(t *testing.T) {
	mu := types.NewMutableState("Which skills are registered by default?")
	mu.SetRequestModel(types.RequestModel{
		Intent:        types.IntentEnumerate,
		PredicateAxis: types.AxisRegister,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
	})
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		Objective: "Which skills are registered by default?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "registration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:      types.EvidenceRegistration,
				Object:    "analysis-skill",
				Summary:   `RegisterDefaults registers "analysis-skill"`,
				Source:    "internal/skill/defaults.go",
				LineStart: 12,
			},
			{
				Kind:      types.EvidenceRegistration,
				Object:    "explore-skill",
				Summary:   `RegisterDefaults registers "explore-skill"`,
				Source:    "internal/skill/defaults.go",
				LineStart: 13,
			},
		},
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Fatalf("generic registration fallback should be lower_bound, got %q", out.AnswerSymbolCompleteness)
	}
	if len(out.AnswerSymbols) != 2 {
		t.Fatalf("generic registration fallback should synthesize literal terminals, got %+v", out.AnswerSymbols)
	}
	for _, sym := range out.AnswerSymbols {
		if sym.Kind != types.KindLiteral {
			t.Fatalf("generic registration fallback should keep literal terminals, got %q", sym.Kind)
		}
	}
}

func TestExtractor_ParseOutput_DoesNotSynthesizeFallbackSlateForSingleTopicExplanation(t *testing.T) {
	mu := types.NewMutableState("`explore_mid_loop_hint_budget` 的覆盖优先级是什么？")
	mu.SetRequestModel(types.RequestModel{
		PredicateAxis: types.AxisConfigure,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
	})
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		Objective: "Explain the precedence for explore_mid_loop_hint_budget",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{Kind: "config_mapping"},
				PredicateAxis: types.AxisConfigure,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:        types.EvidenceDirect,
				Subject:     "DefaultExploreHeuristics",
				Predicate:   "defines",
				Object:      "ExploreHeuristics defaults",
				Summary:     "DefaultExploreHeuristics defines the code-default explorer thresholds.",
				Source:      "internal/types/config.go",
				LineStart:   707,
				ContextRole: types.EvidenceContextRoleRelatedContext,
				DiagramRole: types.EvidenceDiagramRoleDefault,
			},
			{
				Kind:        types.EvidenceDirect,
				Subject:     "RuntimeSettings",
				Predicate:   "binds",
				Object:      "explore_midloop_min_iteration",
				Summary:     "RuntimeSettings exposes the YAML/runtime binding layer.",
				Source:      "internal/config/runtime.go",
				LineStart:   231,
				ContextRole: types.EvidenceContextRoleRelatedContext,
				DiagramRole: types.EvidenceDiagramRoleRuntime,
			},
		},
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if len(out.AnswerSymbols) != 0 {
		t.Fatalf("single-topic explanation must not synthesize a declarative fallback symbol floor, got %+v", out.AnswerSymbols)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessUnknown {
		t.Fatalf("single-topic explanation should keep answer-symbol completeness unknown, got %q", out.AnswerSymbolCompleteness)
	}
}

func TestExtractor_ParseOutput_DoesNotSynthesizeFallbackFromRawReadFileOnly(t *testing.T) {
	mu := types.NewMutableState("Which skills are registered by default?")
	mu.SetRequestModel(types.RequestModel{
		Intent:        types.IntentEnumerate,
		PredicateAxis: types.AxisRegister,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
	})
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/skill/defaults.go"},
		ToolResults: []types.ToolResult{
			buildGutterReadResult("internal/skill/defaults.go", 10, []string{
				`\t\tName: "analysis-skill",`,
				`\t\tName: "explore-skill",`,
			}, 20),
		},
	})
	ctx := &types.AgentContext{
		Objective: "Which skills are registered by default?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "registration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{{
			Kind:      types.EvidenceRegistration,
			Scope:     types.ScopeLine,
			Source:    "internal/skill/defaults.go",
			LineStart: 10,
			Summary:   "RegisterDefaults has default registrations",
		}},
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if len(out.AnswerSymbols) != 0 {
		t.Fatalf("raw read_file literals must not system-fill the answer slate: %+v", out.AnswerSymbols)
	}
	if out.AnswerSymbolCompleteness != "" {
		t.Fatalf("empty fallback should not claim completeness, got %q", out.AnswerSymbolCompleteness)
	}
}

func TestExtractor_ParseOutput_DoesNotAugmentModelSlateFromReadFileLiterals(t *testing.T) {
	mu := types.NewMutableState("Which skills are registered by default?")
	mu.SetRequestModel(types.RequestModel{
		Intent:        types.IntentEnumerate,
		PredicateAxis: types.AxisRegister,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
	})
	mu.SetEmittedAnswerSymbols([]types.AnswerSymbol{
		{Name: "analysis-skill", File: "internal/skill/defaults.go", Line: 11, Kind: types.KindLiteral},
		{Name: "explore-skill", File: "internal/skill/defaults.go", Line: 13, Kind: types.KindLiteral},
	}, types.CompletenessUnknown)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/skill/defaults.go", "internal/skill/analysis_contract.go"},
		ToolResults: []types.ToolResult{
			buildGutterReadResult("internal/skill/defaults.go", 11, []string{
				`\tr.Register(BuildAnalysisSkill())`,
				``,
				`\tr.Register(&Config{`,
				`\t\tName: "explore-skill",`,
			}, 200),
			buildGutterReadResult("internal/skill/defaults.go", 69, []string{
				`\tr.Register(&Config{`,
				`\t\tName: "answer-document-skill",`,
			}, 200),
			buildGutterReadResult("internal/skill/defaults.go", 155, []string{
				`\tr.Register(&Config{`,
				`\t\tName: "extract-skill",`,
			}, 200),
			buildGutterReadResult("internal/skill/analysis_contract.go", 339, []string{
				`\t\tName: "analysis-skill",`,
			}, 354),
		},
	})
	ctx := &types.AgentContext{
		Objective: "Which skills are registered by default?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: "registration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:      types.EvidenceRegistration,
				Source:    "internal/skill/defaults.go",
				LineStart: 11,
				Summary:   "RegisterDefaults registers analysis skill via BuildAnalysisSkill",
			},
			{
				Kind:      types.EvidenceRegistration,
				Source:    "internal/skill/defaults.go",
				LineStart: 13,
				Summary:   "RegisterDefaults registers explore skill",
			},
			{
				Kind:      types.EvidenceRegistration,
				Source:    "internal/skill/defaults.go",
				LineStart: 69,
				Summary:   "RegisterDefaults registers answer document skill",
			},
			{
				Kind:      types.EvidenceRegistration,
				Source:    "internal/skill/defaults.go",
				LineStart: 155,
				Summary:   "RegisterDefaults registers extract skill",
			},
			{
				Kind:      types.EvidenceDirect,
				Source:    "internal/skill/analysis_contract.go",
				LineStart: 339,
				Summary:   "BuildAnalysisSkill sets Name field",
			},
		},
	}

	e := &extractorEvaluator{}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessUnknown {
		t.Fatalf("model-authored unknown claim should not be system-upgraded, got %q", out.AnswerSymbolCompleteness)
	}
	if len(out.AnswerSymbols) != 2 {
		t.Fatalf("read_file fallback must not augment a model-authored slate, got %+v", out.AnswerSymbols)
	}
	got := make(map[string]bool, len(out.AnswerSymbols))
	for _, sym := range out.AnswerSymbols {
		got[sym.Name] = true
		if sym.Kind != types.KindLiteral {
			t.Fatalf("augmented symbol %q should be literal, got %q", sym.Name, sym.Kind)
		}
	}
	for _, want := range []string{"analysis-skill", "explore-skill"} {
		if !got[want] {
			t.Fatalf("model slate missing %q: %+v", want, out.AnswerSymbols)
		}
	}
	for _, unwanted := range []string{"extract-skill", "answer-document-skill"} {
		if got[unwanted] {
			t.Fatalf("system fallback must not append %q to a model-authored slate: %+v", unwanted, out.AnswerSymbols)
		}
	}
}

// TestMergeDeclarativeFallback_CrossFileSameName pins the s1a-20260422
// audit regression: dedup by sym.Name alone silently dropped fallback
// symbols when the answer is a cross-file collection of same-name
// methods (e.g. "list all Run methods in the agent package"). The key
// must incorporate File + Line so each definition site counts as a
// distinct answer.
func TestMergeDeclarativeFallback_CrossFileSameName(t *testing.T) {
	current := []types.AnswerSymbol{
		{Name: "Run", File: "internal/agent/analyzer.go", Line: 450, Kind: types.KindMethod},
	}
	fallback := []types.AnswerSymbol{
		{Name: "Run", File: "internal/agent/explorer.go", Line: 1200, Kind: types.KindMethod},
		{Name: "Run", File: "internal/agent/extractor.go", Line: 310, Kind: types.KindMethod},
		{Name: "Run", File: "internal/agent/finalizer.go", Line: 85, Kind: types.KindMethod},
		{Name: "Run", File: "internal/agent/analyzer.go", Line: 450, Kind: types.KindMethod}, // exact dup, must drop
	}
	got := mergeDeclarativeFallbackSymbols(current, fallback)
	if len(got) != 4 {
		t.Fatalf("cross-file same-name merge: got %d symbols, want 4 (1 current + 3 distinct fallback files, exact-dup drops)", len(got))
	}
	seen := make(map[string]bool, len(got))
	for _, sym := range got {
		seen[sym.File+":"+sym.Name] = true
	}
	wantFiles := []string{
		"internal/agent/analyzer.go:Run",
		"internal/agent/explorer.go:Run",
		"internal/agent/extractor.go:Run",
		"internal/agent/finalizer.go:Run",
	}
	for _, want := range wantFiles {
		if !seen[want] {
			t.Errorf("missing %q in merged slate: %+v", want, got)
		}
	}
}

// TestMergeDeclarativeFallback_SameSiteDedupStillHolds pins that the
// (Name, File, Line) key still collapses truly identical entries —
// the coarse-key loosening must not become a no-op.
func TestMergeDeclarativeFallback_SameSiteDedupStillHolds(t *testing.T) {
	current := []types.AnswerSymbol{
		{Name: "Run", File: "a.go", Line: 10, Kind: types.KindMethod},
	}
	fallback := []types.AnswerSymbol{
		{Name: "Run", File: "a.go", Line: 10, Kind: types.KindMethod},
		{Name: "Run", File: "a.go", Line: 10, Kind: types.KindMethod},
	}
	got := mergeDeclarativeFallbackSymbols(current, fallback)
	if len(got) != 1 {
		t.Fatalf("same-site dedup: got %d, want 1", len(got))
	}
}

// TestTrimDeclarativeSlate_CrossFileSameNameSurvives pins the inverse:
// when the fallback has N distinct (Name,File,Line) entries sharing a
// name, a current slate entry for any one of those sites must survive
// the terminal-filter. Name-only keying would have accepted any entry
// named Run regardless of file — technically permissive, but it kept
// unrelated Runs alive. With the tri-field key, a current entry
// survives iff its exact site appears in the fallback terminal set.
func TestTrimDeclarativeSlate_CrossFileSameNameSurvives(t *testing.T) {
	current := []types.AnswerSymbol{
		{Name: "Run", File: "a.go", Line: 10, Kind: types.KindMethod},
		{Name: "Run", File: "b.go", Line: 20, Kind: types.KindMethod},
		{Name: "Helper", File: "c.go", Line: 30, Kind: types.KindMethod},       // not in fallback — drop
		{Name: "InternalStr", File: "c.go", Line: 40, Kind: types.KindLiteral}, // literals always kept
	}
	fallback := []types.AnswerSymbol{
		{Name: "Run", File: "a.go", Line: 10, Kind: types.KindMethod},
		{Name: "Run", File: "b.go", Line: 20, Kind: types.KindMethod},
	}
	got := trimDeclarativeSlateToTerminals(current, fallback)
	if len(got) != 3 {
		t.Fatalf("trim: got %d, want 3 (2 matching Runs + 1 literal)", len(got))
	}
	var kept []string
	for _, sym := range got {
		kept = append(kept, sym.Name+"@"+sym.File)
	}
	want := map[string]bool{
		"Run@a.go":         true,
		"Run@b.go":         true,
		"InternalStr@c.go": true,
	}
	for _, k := range kept {
		if !want[k] {
			t.Errorf("unexpected survivor %q in %v", k, kept)
		}
		delete(want, k)
	}
	for k := range want {
		t.Errorf("missing expected survivor %q", k)
	}
}
