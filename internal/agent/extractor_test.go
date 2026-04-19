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
	// iter>=3 hard stop.
	e := &extractorEvaluator{}
	for _, i := range []int{0, 1, 2} {
		if e.ShouldStop(llm.Response{}, i) {
			t.Errorf("must NOT stop at iteration %d", i)
		}
	}
	for _, i := range []int{3, 5, 10} {
		if !e.ShouldStop(llm.Response{}, i) {
			t.Errorf("must stop at iteration %d (≥3 cap)", i)
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
// LLM's "complete" claim against max(TerminalEvidenceCount,
// len(MustInclude)). A mismatch downgrades complete → lower_bound.
// Other claims pass through unchanged.

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

func TestExtractor_Validator_Complete_TerminalCountShortfall_Downgrade(t *testing.T) {
	// Turn A found 5 terminal items, LLM only emitted 2 and claimed
	// complete. β baseline catches this; downgrade to lower_bound.
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
		{Name: "B", File: "b.go", Line: 2, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(5, nil, syms, types.CompletenessComplete)
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Errorf("β shortfall must downgrade to lower_bound, got %q", out.AnswerSymbolCompleteness)
	}
	// Slate MUST still be rendered — it becomes the floor for the
	// softened prompt, not dropped entirely.
	if len(out.AnswerSymbols) != 2 {
		t.Errorf("downgrade must preserve slate as floor, got %d items", len(out.AnswerSymbols))
	}
}

func TestExtractor_Validator_Complete_MustIncludeShortfall_Downgrade(t *testing.T) {
	// Analyzer's MustInclude lists 4 names, LLM only emitted 2 and
	// claimed complete. γ baseline catches this independently of β.
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
		{Name: "B", File: "b.go", Line: 2, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(0, []string{"X", "Y", "Z", "W"}, syms, types.CompletenessComplete)
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Errorf("γ shortfall must downgrade to lower_bound, got %q", out.AnswerSymbolCompleteness)
	}
}

func TestExtractor_Validator_Complete_MaxOfTwoBaselines(t *testing.T) {
	// β=2 γ=3 — max = 3. LLM emits 2 with complete → downgrade.
	// This test pins the max() semantics: either baseline alone
	// would have different verdicts (β=2 would pass, γ=3 would fail),
	// and max catches the stricter one.
	syms := []types.AnswerSymbol{
		{Name: "A", File: "a.go", Line: 1, Kind: "function"},
		{Name: "B", File: "b.go", Line: 2, Kind: "function"},
	}
	ctx := extractorCtxWithBaseline(2, []string{"X", "Y", "Z"}, syms, types.CompletenessComplete)
	e := &extractorEvaluator{}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Errorf("max baseline must catch γ=3, got %q", out.AnswerSymbolCompleteness)
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
		"Files the investigation read",
		"internal/reg.go",
		"Deterministic evidence the investigation extracted",
		"registration",
		"Register",
		"Cardinality baseline",
	} {
		if !contains(prompt, section) {
			t.Errorf("prompt missing investigation digest section/content %q", section)
		}
	}
	// Baseline numbers surfaced — terminal-evidence count AND must-include count.
	if !contains(prompt, "terminal-evidence count") || !contains(prompt, "must-include count") {
		t.Error("prompt must surface the cardinality baselines by name")
	}
	// Effective floor = max(3, 2) = 3
	if !contains(prompt, "Effective floor") || !contains(prompt, "3") {
		t.Error("prompt must surface effective floor")
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

	schemas := agent.buildToolSchemas(sk)

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

	schemas := agent.buildToolSchemas(sk)
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
// shape is list_of_symbols, emit_hypothesis_verdict iff at least one
// hypothesis lacks a verdict in the buffer.

// observeMidLoopFixture assembles a minimal ctx + obs so each test
// can toggle one axis (shape, pending hypothesis, tool results) and
// assert on the signal.
func observeMidLoopFixture(shape types.AnswerShape, hypotheses []types.Hypothesis, existingVerdicts []types.HypothesisVerdict, tools []types.ToolResult) (*types.AgentContext, LoopObservation) {
	mu := types.NewMutableState("")
	if len(existingVerdicts) > 0 {
		mu.AppendEmittedHypothesisVerdicts(existingVerdicts)
	}
	ctx := &types.AgentContext{
		Objective: "q",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{RequiredAnswerShape: shape},
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
		types.ShapeListOfSymbols,
		[]types.Hypothesis{{ID: "h1"}},
		nil, // no pre-injected verdict → h1 is pending
		[]types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true},
			{ToolName: "emit_hypothesis_verdict", Success: true},
		},
	)
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
		types.ShapeListOfSymbols,
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

func TestExtractor_Observe_MidLoop_MissingSymbols_Continues(t *testing.T) {
	// Symmetric case: LLM emitted verdict but forgot symbols.
	ctx, obs := observeMidLoopFixture(
		types.ShapeListOfSymbols,
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

func TestExtractor_Observe_MidLoop_AutoVerdictAlreadyInBuffer_StopsOnSymbolsOnly(t *testing.T) {
	// Dominant production case: orchestrator pre-injected an auto-verdict
	// before extractor ran, so hasPendingHypotheses=false. The LLM only
	// needs to emit symbols; mid-loop stops after that single tool.
	ctx, obs := observeMidLoopFixture(
		types.ShapeListOfSymbols,
		[]types.Hypothesis{{ID: "h1"}},
		[]types.HypothesisVerdict{{HypothesisID: "h1", Status: types.HypInconclusive}},
		[]types.ToolResult{
			{ToolName: "emit_answer_symbol", Success: true},
		},
	)
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
		types.ShapeExplanation,
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
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeExplanation},
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
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
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
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
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
		AnswerContract: types.AnswerContract{
			RequiredAnswerShape: types.ShapeExplanation,
		},
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

// TestExtractor_SingleTopicExplanationNoSkeleton — shape=explanation
// without sub_topics is a single-topic question; the old "summary is
// the answer" path applies and no skeleton is emitted.
func TestExtractor_SingleTopicExplanationNoSkeleton(t *testing.T) {
	ir := &types.AnalysisIR{
		AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeExplanation},
	}
	ctx := &types.AgentContext{AnalysisIR: ir, Mutable: types.NewMutableState("q")}
	if isMultiTopicExplanation(ctx) {
		t.Errorf("isMultiTopicExplanation must be false without sub_topics")
	}
}

func TestExtractor_ParseOutput_EmptySlateFallsBackToDeclarativeLiterals(t *testing.T) {
	mu := types.NewMutableState("Which skills are registered by default?")
	mu.SetRequestModel(types.RequestModel{
		PredicateAxis: types.AxisRegister,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
	})
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		Objective: "Which skills are registered by default?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
			},
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
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

func TestExtractor_ParseOutput_PrunesDeclarativeHelperSymbols(t *testing.T) {
	mu := types.NewMutableState("Which skills are registered by default?")
	mu.SetRequestModel(types.RequestModel{
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
				AnalyzerHints: types.AnalyzerHints{Kind: "enumeration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectStringLiteral},
			},
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
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
		PredicateAxis: types.AxisRegister,
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
	})
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		Objective: "Which skills are registered by default?",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: "registration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
			},
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
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

func TestExtractor_ParseOutput_AugmentsDeclarativeSlateFromReadFileLiterals(t *testing.T) {
	mu := types.NewMutableState("Which skills are registered by default?")
	mu.SetRequestModel(types.RequestModel{
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
				AnalyzerHints: types.AnalyzerHints{Kind: "registration"},
				PredicateAxis: types.AxisRegister,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectGeneric},
			},
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
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
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Fatalf("augmented declarative slate should be lower_bound, got %q", out.AnswerSymbolCompleteness)
	}
	if len(out.AnswerSymbols) != 4 {
		t.Fatalf("read_file-backed fallback should augment the slate to four items, got %+v", out.AnswerSymbols)
	}
	got := make(map[string]bool, len(out.AnswerSymbols))
	for _, sym := range out.AnswerSymbols {
		got[sym.Name] = true
		if sym.Kind != types.KindLiteral {
			t.Fatalf("augmented symbol %q should be literal, got %q", sym.Name, sym.Kind)
		}
	}
	for _, want := range []string{"analysis-skill", "explore-skill", "extract-skill", "answer-document-skill"} {
		if !got[want] {
			t.Fatalf("augmented slate missing %q: %+v", want, out.AnswerSymbols)
		}
	}
}
