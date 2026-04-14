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
	// in extractor.go's BuildInitialPrompt string builder. This test
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
	if !strings.Contains(sk.Goal, "Turn A") || !strings.Contains(sk.Goal, "answer-symbol") {
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

	// Prohibitions must explicitly forbid the investigation tools and
	// the evidence channel.
	prohib := strings.Join(sk.Prohibitions, "\n")
	for _, forbidden := range []string{"read_file", "grep", "repo_map", "emit_evidence"} {
		if !strings.Contains(prohib, forbidden) {
			t.Errorf("extract-skill Prohibitions must forbid %q: %s", forbidden, prohib)
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

func TestExtractor_BuildInitialPromptEchoesQuestion(t *testing.T) {
	// BuildInitialPrompt is now the DYNAMIC digest only — it echoes
	// the user question and bakes the Turn A transcript snapshot.
	// The static tool contract lives in the skill (see the test
	// above). This test pins what BuildInitialPrompt IS responsible
	// for post-cleanup: the user question + a non-empty result.
	e := &extractorEvaluator{}
	ctx := &types.AgentContext{CurrentTask: "what does Foo return?"}
	prompt := e.BuildInitialPrompt(ctx, nil)
	if prompt == "" {
		t.Fatal("BuildInitialPrompt must produce a non-empty result")
	}
	if !contains(prompt, "what does Foo return?") {
		t.Error("prompt should echo the user question")
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
			t.Errorf("cleanup regression: BuildInitialPrompt re-states %q (should be in skill, not prompt)",
				staticContent)
		}
	}
}

func TestExtractor_BuildInitialPromptHandlesNilCtx(t *testing.T) {
	// Defensive: Session 2 wiring may call BuildInitialPrompt before
	// AgentContext is fully populated. Must not panic; empty output
	// is acceptable now that all static content lives in the skill.
	e := &extractorEvaluator{}
	_ = e.BuildInitialPrompt(nil, nil) // must not panic
}

func TestExtractor_ShouldStopFiresAfterOneIteration(t *testing.T) {
	// Turn B is fundamentally one-shot — there is no soft-stop, no
	// continuation, no ReAct loop. ShouldStop must return true at
	// iteration >= 1 so BaseAgent.Execute exits cleanly after the
	// first LLM response is produced.
	e := &extractorEvaluator{}
	if e.ShouldStop(llm.Response{}, 0) {
		t.Error("must NOT stop at iteration 0 (we need at least one LLM response)")
	}
	if !e.ShouldStop(llm.Response{}, 1) {
		t.Error("must stop at iteration 1 (one-shot policy)")
	}
	if !e.ShouldStop(llm.Response{}, 5) {
		t.Error("must stop at any iteration >= 1")
	}
}

func TestExtractor_ParseOutputDrainsEmittedBuffers(t *testing.T) {
	// ParseOutput must drain MutableState's emit_answer_symbol
	// buffer into StageOutput. Evidence is intentionally NOT drained
	// here — that is Turn A's exclusive channel after the de-
	// duplication cleanup. With Phase 9's Set-semantics API the
	// completeness claim travels with the slate.
	mu := types.NewMutableState(types.TaskList{})
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
	// No baseline data (no TurnAArtifacts, no AnalysisIR) → the
	// validator passes the claim through untouched.
	ctx := &types.AgentContext{
		CurrentTask: "test",
		Mutable:     mu,
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
	mu := types.NewMutableState(types.TaskList{})
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion:          "q",
		TerminalEvidenceCount: termCount,
	})
	mu.SetEmittedAnswerSymbols(syms, claim)
	return &types.AgentContext{
		CurrentTask: "q",
		Mutable:     mu,
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
// P2.1 Phase 8 — BuildInitialPrompt reads TurnAArtifacts
// -----------------------------------------------------------------------------

func TestExtractor_BuildPrompt_DigestsTurnAArtifacts(t *testing.T) {
	mu := types.NewMutableState(types.TaskList{})
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion:          "which handlers register Foo?",
		InvestigationNotes:    []string{"iter 1: read reg.go, found Register calls"},
		ReadFiles:             []string{"internal/reg.go", "internal/a.go"},
		EvidenceItems: []types.EvidenceItem{
			{Kind: types.EvidenceRegistration, Subject: "Register", Object: "NewHandlerA", Source: "internal/reg.go", LineStart: 12},
		},
		TerminalEvidenceCount: 3,
	})
	ctx := &types.AgentContext{
		CurrentTask: "which handlers register Foo?",
		Mutable:     mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{MustInclude: []string{"HandlerA", "HandlerB"}},
		},
	}

	e := &extractorEvaluator{}
	prompt := e.BuildInitialPrompt(ctx, nil)

	// Question echoed
	if !contains(prompt, "which handlers register Foo?") {
		t.Error("prompt must echo user question")
	}
	// Turn A digest sections (the dynamic content this function is
	// still responsible for post-cleanup)
	for _, section := range []string{
		"Investigation notes",
		"iter 1: read reg.go",
		"Files Turn A read",
		"internal/reg.go",
		"Deterministic evidence Turn A extracted",
		"registration",
		"Register",
		"Cardinality baseline",
	} {
		if !contains(prompt, section) {
			t.Errorf("prompt missing Turn A digest section/content %q", section)
		}
	}
	// Baseline numbers surfaced
	if !contains(prompt, "terminal-evidence count") || !contains(prompt, "MustInclude") {
		t.Error("prompt must surface β and γ baselines by name")
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
	mu := types.NewMutableState(types.TaskList{})
	ctx := &types.AgentContext{CurrentTask: "q", Mutable: mu}
	e := &extractorEvaluator{}
	prompt := e.BuildInitialPrompt(ctx, nil)

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
	mu := types.NewMutableState(types.TaskList{})
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		UserQuestion:       "q",
		InvestigationNotes: notes,
	})
	ctx := &types.AgentContext{CurrentTask: "q", Mutable: mu}
	e := &extractorEvaluator{}
	prompt := e.BuildInitialPrompt(ctx, nil)

	if !contains(prompt, "showing the 6 most recent of 10") {
		t.Error("prompt must show clamp notice when notes > 6")
	}
	if !contains(prompt, "…") {
		t.Error("prompt must truncate over-long notes with ellipsis")
	}
}

func TestExtractor_BuildPrompt_IncludesHypothesisSet(t *testing.T) {
	mu := types.NewMutableState(types.TaskList{})
	mu.SetTurnAArtifacts(types.TurnAArtifacts{UserQuestion: "q"})
	ctx := &types.AgentContext{
		CurrentTask: "q",
		Mutable:     mu,
		AnalysisIR: &types.AnalysisIR{
			HypothesisSet: []types.Hypothesis{
				{ID: "H1", Statement: "Foo calls Bar", Status: types.HypUnknown},
				{ID: "H2", Statement: "Bar returns true", Status: types.HypUnknown},
			},
		},
	}
	e := &extractorEvaluator{}
	prompt := e.BuildInitialPrompt(ctx, nil)

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
	mu := types.NewMutableState(types.TaskList{})
	mu.SetEmittedAnswerSymbols(nil, types.CompletenessUnknown)
	ctx := &types.AgentContext{
		CurrentTask: "q",
		Mutable:     mu,
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
