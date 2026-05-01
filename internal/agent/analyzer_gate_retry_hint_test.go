package agent

import (
	"strings"
	"testing"
)

// TestBuildGenericGateRetryHint_PerCheckBulleted pins the
// commit-58 Batch D fix (audit #2): joined gate-failure detail
// gets formatted as a bulleted "## Quality gate rejected" section
// with one bullet per failing check. Pre-fix this never reached
// the LLM (out.Error stayed in logs); post-fix the analyzer's
// next dispatch sees per-check Detail strings via
// Mutable.AnalyzerRetryHint → prependEmitRetryDirective.
func TestBuildGenericGateRetryHint_PerCheckBulleted(t *testing.T) {
	joined := "coverage: score=0.4 threshold=0.7; dag_closure: missing edge A→B; budget_sanity: files=2 min=5"
	hint := buildGenericGateRetryHint(joined)

	if !strings.Contains(hint, "## Quality gate rejected") {
		t.Error("hint missing structural header")
	}
	for _, want := range []string{
		"coverage: score=0.4 threshold=0.7",
		"dag_closure: missing edge A→B",
		"budget_sanity: files=2 min=5",
	} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing per-check detail: %q\nfull hint:\n%s", want, hint)
		}
	}
	// Bulleted: each check should appear after a "- " marker.
	bullets := strings.Count(hint, "\n- ")
	if bullets != 3 {
		t.Errorf("expected 3 bulleted checks, got %d in: %s", bullets, hint)
	}
}

// TestBuildGenericGateRetryHint_EmptyInputIsByteIdentical pins
// the no-op contract: an empty join string produces empty hint
// (no spurious header) so a non-rejection retry path doesn't
// get falsely-decorated.
func TestBuildGenericGateRetryHint_EmptyInputIsByteIdentical(t *testing.T) {
	if got := buildGenericGateRetryHint(""); got != "" {
		t.Errorf("empty input should yield empty hint; got %q", got)
	}
	if got := buildGenericGateRetryHint("   \n\t  "); got != "" {
		t.Errorf("whitespace-only input should yield empty hint; got %q", got)
	}
}

// TestBuildGenericGateRetryHint_GuidanceTellsLLMNotToResubmit
// pins the commit-58 Batch D framing: the hint MUST contain the
// "do NOT resubmit identical emits" guidance so the LLM doesn't
// retry with the same shape.
func TestBuildGenericGateRetryHint_GuidanceTellsLLMNotToResubmit(t *testing.T) {
	hint := buildGenericGateRetryHint("coverage: bad")
	if !strings.Contains(hint, "Do NOT just resubmit") {
		t.Error("hint missing 'do not resubmit' guidance — LLM may waste retries")
	}
}
