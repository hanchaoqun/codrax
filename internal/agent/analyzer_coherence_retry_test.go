package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestComposeCoherenceRetryHint_FormatsCoherenceFailuresOnly pins
// the contract: composeCoherenceRetryHint renders only coherence
// gate failures (subtopic_coherence / shape_subject_coherence) into
// the retry hint, leaving generic gate failures (coverage,
// dag_closure, etc.) for the normal retry directive path.
func TestComposeCoherenceRetryHint_FormatsCoherenceFailuresOnly(t *testing.T) {
	report := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "coverage", Passed: false, Detail: "weighted coverage 0.30/0.50"},
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.1 domain_divergence: TermGraph spans 3 distinct repomap-verified domains [agent, finalizer, orchestrator] but only 1 sub-topic emitted"},
			{Name: "shape_subject_coherence", Passed: false, Detail: "R2.1 scalar_multi_topic: Predicates.IsScalarAnswer=true but 2 sub-topics emitted"},
			{Name: "dag_closure", Passed: true},
		},
	}
	hint := composeCoherenceRetryHint(report)
	if hint == "" {
		t.Fatal("expected a non-empty hint when coherence checks failed")
	}
	// Internal R-codes (R1.1, R2.1) MUST be stripped before reaching
	// the LLM hint — they are internal documentation references and
	// confuse models with no awareness of the coherence-rule numbering.
	// The plain-language body that followed each code is what the LLM
	// can act on.
	if strings.Contains(hint, "R1.1") || strings.Contains(hint, "R2.1") {
		t.Errorf("hint must NOT surface internal R-codes; got %q", hint)
	}
	if !strings.Contains(hint, "TermGraph spans 3 distinct") {
		t.Errorf("hint must surface the R1.1 plain-language detail; got %q", hint)
	}
	if !strings.Contains(hint, "IsScalarAnswer=true but 2 sub-topics") {
		t.Errorf("hint must surface the R2.1 plain-language detail; got %q", hint)
	}
	if strings.Contains(hint, "coverage") || strings.Contains(hint, "dag_closure") {
		t.Errorf("hint must NOT include non-coherence checks; got %q", hint)
	}
}

func TestComposeCoherenceRetryHint_EmptyOnPass(t *testing.T) {
	report := types.GateReport{
		Rejected: false,
		Checks:   []types.GateCheck{{Name: "subtopic_coherence", Passed: true}},
	}
	if hint := composeCoherenceRetryHint(report); hint != "" {
		t.Errorf("hint must be empty for a passing report; got %q", hint)
	}
}

func TestComposeCoherenceRetryHint_EmptyWhenOnlyGenericFails(t *testing.T) {
	report := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "coverage", Passed: false, Detail: "below threshold"},
			{Name: "dag_closure", Passed: false, Detail: "dangling edge"},
			{Name: "subtopic_coherence", Passed: true},
			{Name: "shape_subject_coherence", Passed: true},
		},
	}
	if hint := composeCoherenceRetryHint(report); hint != "" {
		t.Errorf("hint must be empty when no coherence checks failed; got %q", hint)
	}
}

// TestPrependEmitRetryDirective_RendersAnalyzerRetryHint exercises
// the consume-once contract: when Mutable.AnalyzerRetryHint is set,
// the directive includes the structural-contradiction section AND
// the hint is reset so a re-entry on the same Mutable does not
// double-render it.
func TestPrependEmitRetryDirective_RendersAnalyzerRetryHint(t *testing.T) {
	mu := types.NewMutableState("test")
	mu.SetAnalyzerRetryHint("- subtopic_coherence: R1.1 domain_divergence detail here")
	ctx := &types.AgentContext{
		EmitStageRetryAttempt: 1,
		Mutable:               mu,
	}
	out := prependEmitRetryDirective(ctx, "## existing prompt\n")
	if !strings.Contains(out, "Structural contradiction in your previous emit_analysis") {
		t.Errorf("directive must surface structural-contradiction header; got %q", out)
	}
	if !strings.Contains(out, "R1.1 domain_divergence detail here") {
		t.Errorf("directive must embed the IR-field-level hint; got %q", out)
	}
	if mu.AnalyzerRetryHint() != "" {
		t.Errorf("hint must be reset after consumption; got %q", mu.AnalyzerRetryHint())
	}
	// Re-entrant safety: a second call with the same Mutable must
	// not re-render the (now-cleared) section.
	out2 := prependEmitRetryDirective(ctx, "## existing prompt\n")
	if strings.Contains(out2, "Structural contradiction in your previous emit_analysis") {
		t.Errorf("second call must not re-render the cleared hint; got %q", out2)
	}
}

func TestPrependEmitRetryDirective_HappyPathAttemptZero(t *testing.T) {
	ctx := &types.AgentContext{EmitStageRetryAttempt: 0}
	out := prependEmitRetryDirective(ctx, "## base\n")
	if out != "## base\n" {
		t.Errorf("attempt 0 must return base unchanged; got %q", out)
	}
}
