package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDetectLogOperationalSemanticsSeparatesStageProgressFromDispatchAttempt(t *testing.T) {
	raw := strings.Join([]string{
		"2026-05-23T09:00:20.000 WARN [llm] first_byte_timeout exceeded after 40s",
		"2026-05-23T09:00:20.001 WARN [orchestrator] finalizer attempt 1/3 failed: LLM stream timeout",
		"2026-05-23T09:00:20.002 INFO [render] ⟳ 4/4 模型响应出错,正在重新撰写答案",
	}, "\n")

	got := DetectLogOperationalSemantics(raw)
	if len(got) != 2 {
		t.Fatalf("semantics=%d, want 2: %+v", len(got), got)
	}
	attempt, progress := got[0], got[1]
	if attempt.EventKind != types.LogOperationalEventDispatchAttemptFailed ||
		attempt.CounterDomain != types.LogOperationalCounterDispatchAttempt ||
		attempt.Subject != "finalizer" || attempt.Numerator != 1 || attempt.Denominator != 3 {
		t.Fatalf("dispatch-attempt semantic wrong: %+v", attempt)
	}
	if progress.EventKind != types.LogOperationalEventPipelineStageLifecycle ||
		progress.CounterDomain != types.LogOperationalCounterPipelineStageProgress ||
		progress.StageKey != "finalize" || progress.Lifecycle != "retry" ||
		progress.Numerator != 4 || progress.Denominator != 4 {
		t.Fatalf("renderer progress semantic wrong: %+v", progress)
	}
	if strings.Contains(strings.Join(progress.ExcludedMeanings, ","), "pipeline_stage_progress") ||
		!strings.Contains(strings.Join(progress.ExcludedMeanings, ","), "model_count") {
		t.Fatalf("renderer exclusion boundary wrong: %+v", progress.ExcludedMeanings)
	}
}

func TestDetectLogOperationalSemanticsRejectsVisualLookalikes(t *testing.T) {
	raw := strings.Join([]string{
		"INFO [customer] 4/4 模型响应出错,正在重新撰写答案",
		"INFO [render] 3/4 模型响应出错,正在重新撰写答案",
		"WARN [orchestrator] finalizer attempts 1/3 failed: timeout",
		`DEBUG [collector] copied="INFO [render] ⟳ 4/4 模型响应出错,正在重新撰写答案"`,
	}, "\n")
	if got := DetectLogOperationalSemantics(raw); len(got) != 0 {
		t.Fatalf("lookalikes must not mint protocol semantics: %+v", got)
	}
}

func TestRenderLogOperationalSemanticsForPromptPinsCounterNamespaces(t *testing.T) {
	rows := DetectLogOperationalSemantics(
		"INFO [render] ⟳ 4/4 Model response error, re-composing the final answer\n" +
			"WARN [orchestrator] finalizer attempt 1/3 failed: timeout",
	)
	got := RenderLogOperationalSemanticsForPrompt(rows)
	for _, want := range []string{
		"counter_domain=pipeline_stage_progress value=4/4",
		"does_not_mean=model_count,llm_attempt_count,fallback_count,repair_round_count",
		"counter_domain=agent_dispatch_attempt value=1/3",
		"separate namespaces",
		"does not prove that gate was traversed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}
