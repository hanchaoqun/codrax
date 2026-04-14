package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// P0.2 validator tests. Each shape gets:
//   - happy-path pass
//   - violation fails with a named reason
//   - skip-guard when upstream data is unavailable
// See memory/project_architecture_remediation_roadmap.md (P0.2).

// ---------------------------------------------------------------
// validateStepList
// ---------------------------------------------------------------

func TestValidateStepList_PassesWhenStepsMeetBranchCount(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConditional, Summary: "branch A"},
		{Kind: types.EvidenceConditional, Summary: "branch B"},
		{Kind: types.EvidenceConditional, Summary: "branch C"},
	}
	answer := "1. First branch (file.go:10)\n2. Second branch (file.go:20)\n3. Third branch (file.go:30)\n"
	if got := validateStepList(answer, items); got != "" {
		t.Errorf("expected pass, got violation: %s", got)
	}
}

func TestValidateStepList_FailsWhenStepsBelowBranchCount(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConditional, Summary: "branch A"},
		{Kind: types.EvidenceConditional, Summary: "branch B"},
		{Kind: types.EvidenceConditional, Summary: "branch C"},
	}
	// Prose collapse: 3 branches collapsed into a single narrative step.
	answer := "The mechanism evaluates branches A, B and C in sequence."
	got := validateStepList(answer, items)
	if got == "" {
		t.Fatal("expected violation for prose collapse, got none")
	}
	if !strings.Contains(got, "3") {
		t.Errorf("violation should mention the required branch count (3), got: %s", got)
	}
}

func TestValidateStepList_SkipGuardWhenNoConditionalEvidence(t *testing.T) {
	// 0 [CONDITIONAL] entries → "N ≥ 0 steps" is vacuously true, validator passes.
	items := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, Summary: "x"},
	}
	answer := "The return value is \"explorer\"."
	if got := validateStepList(answer, items); got != "" {
		t.Errorf("no conditional evidence should pass the validator, got: %s", got)
	}
}

func TestValidateStepList_BulletStepsCounted(t *testing.T) {
	// Bullet lists also count as steps (structural regex).
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConditional, Summary: "A"},
		{Kind: types.EvidenceConditional, Summary: "B"},
	}
	answer := "- Step A (file.go:1)\n- Step B (file.go:2)\n"
	if got := validateStepList(answer, items); got != "" {
		t.Errorf("bullet steps should count, got violation: %s", got)
	}
}

// ---------------------------------------------------------------
// validateValue
// ---------------------------------------------------------------

func TestValidateValue_PassesWithLiteralFromEvidence(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Subject: "Name", Object: `"explorer"`},
	}
	answer := `The value is "explorer".`
	if got := validateValue(answer, items); got != "" {
		t.Errorf("expected pass, got: %s", got)
	}
}

func TestValidateValue_FailsWhenLiteralAbsent(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Subject: "Name", Object: `"explorer"`},
	}
	answer := "The name is the explorer agent."
	got := validateValue(answer, items)
	if got == "" {
		t.Fatal("expected violation when answer lacks literal")
	}
	if !strings.Contains(got, `"explorer"`) {
		t.Errorf("violation should name the expected literal, got: %s", got)
	}
}

func TestValidateValue_SkipGuardWhenNoConcreteEvidence(t *testing.T) {
	// Upstream never produced EvidenceConcrete → skip guard, pass.
	items := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, Summary: "x"},
	}
	answer := "Some prose answer."
	if got := validateValue(answer, items); got != "" {
		t.Errorf("missing concrete evidence should skip guard, got: %s", got)
	}
}

// ---------------------------------------------------------------
// validateBoolean
// ---------------------------------------------------------------

func TestValidateBoolean_PassesWithYesPrefix(t *testing.T) {
	if got := validateBoolean("YES — the method is write-gated (file.go:42)."); got != "" {
		t.Errorf("YES prefix should pass, got: %s", got)
	}
}

func TestValidateBoolean_PassesWithNoPrefix(t *testing.T) {
	if got := validateBoolean("NO, it is read-only."); got != "" {
		t.Errorf("NO prefix should pass, got: %s", got)
	}
}

func TestValidateBoolean_PassesWithChinesePrefix(t *testing.T) {
	// Q1 decision: bilingual structural prefix (是/否/YES/NO).
	if got := validateBoolean("是的，该方法写入数据库。"); got != "" {
		t.Errorf("Chinese 是 prefix should pass, got: %s", got)
	}
	if got := validateBoolean("否。该方法只读。"); got != "" {
		t.Errorf("Chinese 否 prefix should pass, got: %s", got)
	}
}

func TestValidateBoolean_FailsWithHedgingPrefix(t *testing.T) {
	got := validateBoolean("It depends on the input.")
	if got == "" {
		t.Fatal("hedging prefix should fail")
	}
}

func TestValidateBoolean_FailsWhenBuriedInProse(t *testing.T) {
	// YES appears but is not the opening token.
	got := validateBoolean("After analysis, YES, the method writes.")
	if got == "" {
		t.Fatal("YES must be the leading token, not buried")
	}
}

// ---------------------------------------------------------------
// validateConfigValue
// ---------------------------------------------------------------

func TestValidateConfigValue_PassesWithKeyValueAndCite(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Kind:    types.EvidenceConcrete,
			Subject: "default_agent",
			Object:  `"explorer"`,
			Source:  "config/codrax.yaml",
		},
	}
	answer := "The config key `default_agent` is set to `\"explorer\"` in config/codrax.yaml:12."
	if got := validateConfigValue(answer, items); got != "" {
		t.Errorf("expected pass, got: %s", got)
	}
}

func TestValidateConfigValue_FailsWhenKeyMissing(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Subject: "default_agent", Object: `"explorer"`},
	}
	answer := `The value is "explorer" and lives at config/codrax.yaml:12.`
	if got := validateConfigValue(answer, items); got == "" {
		t.Fatal("missing key mention should fail")
	}
}

func TestValidateConfigValue_FailsWhenValueMissing(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Subject: "default_agent", Object: `"explorer"`},
	}
	answer := "The key `default_agent` is declared in config/codrax.yaml:12."
	if got := validateConfigValue(answer, items); got == "" {
		t.Fatal("missing value literal should fail")
	}
}

func TestValidateConfigValue_FailsWhenCiteMissing(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceConcrete, Subject: "default_agent", Object: `"explorer"`},
	}
	answer := "The key `default_agent` is set to `\"explorer\"`."
	if got := validateConfigValue(answer, items); got == "" {
		t.Fatal("missing file:line cite should fail")
	}
}

func TestValidateConfigValue_SkipGuardWhenNoConcreteEvidence(t *testing.T) {
	items := []types.EvidenceItem{
		{Kind: types.EvidenceDirect, Summary: "x"},
	}
	if got := validateConfigValue("anything", items); got != "" {
		t.Errorf("no concrete evidence should skip guard, got: %s", got)
	}
}

// ---------------------------------------------------------------
// ShouldStop + ContinuationPrompt wiring for P0.2 shapes
// ---------------------------------------------------------------

func newStepListCtx(items []types.EvidenceItem) *types.AgentContext {
	return &types.AgentContext{
		Stage:         types.StageFinalize,
		EvidenceItems: items,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Shape: "step_list"},
			},
		},
	}
}

func TestFinalizerShouldStop_StepListRoutesThroughContinuation(t *testing.T) {
	// No AnswerSymbols, but shape=step_list → ShouldStop must return
	// false so ContinuationPrompt runs the P0.2 validator.
	e := &finalizerEvaluator{}
	ctx := newStepListCtx(nil)
	e.BuildInitialPrompt(ctx, nil)
	if e.ShouldStop(llm.Response{}, 0) {
		t.Error("step_list shape must route through ContinuationPrompt, not one-shot")
	}
}

func TestFinalizerContinuationPrompt_StepListViolationRetries(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := newStepListCtx([]types.EvidenceItem{
		{Kind: types.EvidenceConditional, Summary: "A"},
		{Kind: types.EvidenceConditional, Summary: "B"},
		{Kind: types.EvidenceConditional, Summary: "C"},
	})
	e.BuildInitialPrompt(ctx, nil)
	resp := llm.Response{Content: "The mechanism evaluates three branches in a single pass."}
	prompt, cont := e.ContinuationPrompt(resp, 0, 0, nil)
	if !cont {
		t.Fatal("step_list collapse should trigger retry")
	}
	if !strings.Contains(prompt, "step") {
		t.Errorf("correction prompt should explain the step-count rule, got: %s", prompt)
	}
	if e.retriesUsed != 1 {
		t.Errorf("retriesUsed = %d, want 1", e.retriesUsed)
	}
}

func TestFinalizerContinuationPrompt_StepListPassStops(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := newStepListCtx([]types.EvidenceItem{
		{Kind: types.EvidenceConditional, Summary: "A"},
		{Kind: types.EvidenceConditional, Summary: "B"},
	})
	e.BuildInitialPrompt(ctx, nil)
	resp := llm.Response{Content: "1. First branch (x.go:1)\n2. Second branch (x.go:2)\n"}
	_, cont := e.ContinuationPrompt(resp, 0, 0, nil)
	if cont {
		t.Error("valid step_list should stop, not retry")
	}
}

// Fail-loud contract: after retries exhausted, ParseOutput must append
// an honesty declaration to the answer. Per P0.2 red line, P0.2 shapes
// do NOT silent-accept (unlike list_of_symbols L0-2 S3).
func TestFinalizerFailLoud_StepListValidationExhausted(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := newStepListCtx([]types.EvidenceItem{
		{Kind: types.EvidenceConditional, Summary: "A"},
		{Kind: types.EvidenceConditional, Summary: "B"},
	})
	e.BuildInitialPrompt(ctx, nil)

	badResp := llm.Response{Content: "A single-paragraph narrative, no enumeration."}
	// Burn retries.
	for i := 0; i < maxFinalizerCorrectionRetries; i++ {
		_, cont := e.ContinuationPrompt(badResp, i, i, nil)
		if !cont {
			t.Fatalf("retry %d should request continuation", i)
		}
	}
	// One more call — cap hit, must return shouldContinue=false AND
	// record the failure so ParseOutput can append the honesty note.
	_, cont := e.ContinuationPrompt(badResp, maxFinalizerCorrectionRetries, maxFinalizerCorrectionRetries, nil)
	if cont {
		t.Error("retry cap must stop continuation")
	}

	msgs := []llm.Message{{Role: "assistant", Content: badResp.Content}}
	out, err := e.ParseOutput(ctx, msgs, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "validation exhausted") {
		t.Errorf("FinalAnswer should carry fail-loud note, got: %s", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, badResp.Content) {
		t.Error("FinalAnswer should still preserve the LLM's last answer below the note")
	}
}

// Non-P0.2 shapes (explanation/none) stay in legacy one-shot path.
func TestFinalizerShouldStop_ExplanationStaysOneShot(t *testing.T) {
	e := &finalizerEvaluator{}
	ctx := &types.AgentContext{
		Stage: types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Shape: "explanation"},
			},
		},
	}
	e.BuildInitialPrompt(ctx, nil)
	if !e.ShouldStop(llm.Response{}, 0) {
		t.Error("explanation shape should stay one-shot (legacy)")
	}
}
