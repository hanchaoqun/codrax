// Package agent — required_file_hint_retry_test.go (2026-05-10).
//
// L3-T4 of the forced-read remediation: composeRequiredFileHintsRetryAdvice
// emits an advisory paragraph when the analyzer's prior emit_analysis
// included borderline-confidence (0.5–0.79) RequiredFileHints. On the
// next dispatch the LLM sees this advice in the retry hint and can
// either push the confidence higher (with a stronger rationale) or
// drop the entry entirely.
package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func makeIRWithHints(hints ...types.RequiredFileHint) *types.AnalysisIR {
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.RequiredFileHints = hints
	return ir
}

func TestComposeRequiredFileHintsRetryAdvice_NilIR_Empty(t *testing.T) {
	if got := composeRequiredFileHintsRetryAdvice(nil); got != "" {
		t.Errorf("nil ir: expected empty; got %q", got)
	}
}

func TestComposeRequiredFileHintsRetryAdvice_NoHints_Empty(t *testing.T) {
	if got := composeRequiredFileHintsRetryAdvice(makeIRWithHints()); got != "" {
		t.Errorf("no hints: expected empty; got %q", got)
	}
}

func TestComposeRequiredFileHintsRetryAdvice_AllHigh_Empty(t *testing.T) {
	ir := makeIRWithHints(
		types.RequiredFileHint{Path: "a.go", Confidence: 0.9},
		types.RequiredFileHint{Path: "b.go", Confidence: 0.85},
	)
	if got := composeRequiredFileHintsRetryAdvice(ir); got != "" {
		t.Errorf("all high-confidence: expected empty; got %q", got)
	}
}

func TestComposeRequiredFileHintsRetryAdvice_AllLow_Empty(t *testing.T) {
	// Below 0.5 entries are already dropped by the consumer; no
	// advice needed.
	ir := makeIRWithHints(
		types.RequiredFileHint{Path: "a.go", Confidence: 0.3},
		types.RequiredFileHint{Path: "b.go", Confidence: 0.4},
	)
	if got := composeRequiredFileHintsRetryAdvice(ir); got != "" {
		t.Errorf("all low-confidence: expected empty (dropped already); got %q", got)
	}
}

func TestComposeRequiredFileHintsRetryAdvice_BorderlineEntries_AdviceEmitted(t *testing.T) {
	ir := makeIRWithHints(
		types.RequiredFileHint{Path: "a.go", Confidence: 0.6},
		types.RequiredFileHint{Path: "b.go", Confidence: 0.79},
		types.RequiredFileHint{Path: "c.go", Confidence: 0.95}, // high — not in advice
	)
	got := composeRequiredFileHintsRetryAdvice(ir)
	if !strings.Contains(got, "borderline") {
		t.Errorf("expected 'borderline' in advice; got %q", got)
	}
	if !strings.Contains(got, "`a.go`") {
		t.Errorf("expected a.go in advice; got %q", got)
	}
	if !strings.Contains(got, "`b.go`") {
		t.Errorf("expected b.go in advice; got %q", got)
	}
	if strings.Contains(got, "`c.go`") {
		t.Errorf("c.go is high-confidence; should NOT appear in advice; got %q", got)
	}
	if !strings.Contains(got, "0.60") || !strings.Contains(got, "0.79") {
		t.Errorf("expected confidences in advice; got %q", got)
	}
}

func TestComposeRequiredFileHintsRetryAdvice_NoInternalTermsInOutput(t *testing.T) {
	// R6 red line: no internal Go field names AND no internal
	// pipeline stage names should appear in LLM-facing output.
	// User-flagged 2026-05-10: "the explorer" is invisible to the
	// LLM — phrase as "next stage" / "downstream investigation".
	ir := makeIRWithHints(
		types.RequiredFileHint{Path: "x.go", Confidence: 0.7},
	)
	got := composeRequiredFileHintsRetryAdvice(ir)
	for _, internal := range []string{
		// Go-side field / function names
		"RequiredFileHints", "AnalyzerHints", "ir.RequestModel",
		"composeRequiredFileHintsRetryAdvice",
		// Internal stage names
		"explorer", "extractor", "finalizer", "analyzer",
		// Internal type / package terms
		"AnalyzerHints", "EvidencePlan", "AnalysisIR",
	} {
		if strings.Contains(got, internal) {
			t.Errorf("internal term %q leaked into LLM-facing advice: %q", internal, got)
		}
	}
}

func TestComposeRequiredFileHintsRetryAdvice_LanguageNeutral(t *testing.T) {
	// Cross-language phrasing: should not say "Go file" / "Python
	// module" / etc. Just "file" so the advice applies regardless
	// of language.
	ir := makeIRWithHints(
		types.RequiredFileHint{Path: "x.go", Confidence: 0.7},
	)
	got := composeRequiredFileHintsRetryAdvice(ir)
	for _, langSpecific := range []string{
		"Go file", "Python module", "Java class file", "Rust crate",
	} {
		if strings.Contains(got, langSpecific) {
			t.Errorf("language-specific phrasing %q in cross-language advice: %q", langSpecific, got)
		}
	}
}
