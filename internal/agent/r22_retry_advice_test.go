// Package agent — r22_retry_advice_test.go (2026-05-10).
//
// Fix-B of the analyzer-failure remediation: when the gate rejects
// the IR with R2.2 longform_scalar_subject, the analyzer's retry
// hint composer appends targeted advisory text guiding the LLM
// toward UNSETTING answer_subject or picking a non-scalar kind.
package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === reportContainsR22 ===

func TestReportContainsR22_NotRejected_False(t *testing.T) {
	r := types.GateReport{Rejected: false}
	if reportContainsR22(r) {
		t.Error("not-rejected report: expected false")
	}
}

func TestReportContainsR22_R22Failed_True(t *testing.T) {
	r := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=root_cause_trace expects non-scalar"},
		},
	}
	if !reportContainsR22(r) {
		t.Error("R2.2 failed: expected true")
	}
}

func TestReportContainsR22_R22Passed_False(t *testing.T) {
	r := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "shape_subject_coherence", Passed: true, Detail: "family=generic ..."},
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse"},
		},
	}
	if reportContainsR22(r) {
		t.Error("R2.2 passed (different rule failed): expected false")
	}
}

func TestReportContainsR22_OtherCheckWithR22Detail_False(t *testing.T) {
	// Different check name, even if Detail mentions R2.2 — should
	// not match (precise key on check name).
	r := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R2.2 ... mentioned but wrong check"},
		},
	}
	if reportContainsR22(r) {
		t.Error("Detail prefix on wrong check: expected false (precise key)")
	}
}

// === composeCoherenceRetryHint includes R22 advice ===

func TestComposeCoherenceRetryHint_R22_AppendsAdvice(t *testing.T) {
	r := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "shape_subject_coherence", Passed: false, Detail: "R2.2 longform_scalar_subject: family=root_cause_trace expects non-scalar payload but AnswerSubject.Kind=string_literal at confidence 0.90"},
		},
	}
	got := composeCoherenceRetryHint(r)
	if !strings.Contains(got, "scalar value") || !strings.Contains(got, "answer_subject") {
		t.Errorf("R2.2 advice missing key terms; got %q", got)
	}
	if !strings.Contains(got, "UNSETTING") {
		t.Error("expected UNSETTING guidance")
	}
}

func TestComposeCoherenceRetryHint_NoR22_NoAdvice(t *testing.T) {
	// Other check failed; advice should NOT appear.
	r := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.4 axis_collapse: ..."},
		},
	}
	got := composeCoherenceRetryHint(r)
	if strings.Contains(got, "scalar value") {
		t.Errorf("non-R2.2 should not get R2.2-specific advice; got %q", got)
	}
}

// === r22RetryAdvice red-line audit (R6 + cross-language + cross-domain) ===

func TestR22RetryAdvice_NoInternalStageNames(t *testing.T) {
	got := r22RetryAdvice()
	for _, internal := range []string{
		"explorer", "extractor", "finalizer", "analyzer",
		"downstream stage", "downstream stages", "the explorer", "the extractor",
		"AnswerSubject.Kind", "AnalyzerHints", "AnalysisIR", "RequestModel",
		"contract gate", "shape contract gate", "narrative-family",
	} {
		if strings.Contains(got, internal) {
			t.Errorf("internal term %q leaked into LLM-facing advice: %q", internal, got)
		}
	}
}

func TestR22RetryAdvice_NoLanguageSpecificFamilyExamples(t *testing.T) {
	// User-flagged 2026-05-10: don't curve-fit to Python tracebacks
	// or Go panics. Advice must work cross-language so all 12+
	// supported languages benefit.
	got := r22RetryAdvice()
	for _, langSpecific := range []string{
		"Python traceback", "Go panic", "Java exception",
		"RuntimeError", "KeyError", "ValueError",
		"NullPointerException", "Segmentation fault", "stack overflow",
		"goroutine dump", "concurrent map writes",
	} {
		if strings.Contains(got, langSpecific) {
			t.Errorf("language-specific phrasing %q in cross-language advice: %q", langSpecific, got)
		}
	}
}

func TestR22RetryAdvice_NoCaseSpecificCurveFit(t *testing.T) {
	// User-flagged 2026-05-10: don't curve-fit to one failure case
	// (logtri_python). Advice must generalise across question
	// shapes that trigger R2.2.
	got := r22RetryAdvice()
	for _, caseSpecific := range []string{
		"logtri_python", "logtri_goroutine_dump",
		"explain what happened", // overly-specific phrase
	} {
		if strings.Contains(got, caseSpecific) {
			t.Errorf("case-specific phrasing %q leaked: %q", caseSpecific, got)
		}
	}
}

func TestR22RetryAdvice_GeneralisesAcrossQuestionShapes(t *testing.T) {
	// Verify the advice text references the GENERALISED question
	// shapes that all map to non-scalar answer (causation /
	// mechanism / sequence / walk-through / comparison) rather
	// than one specific shape.
	got := r22RetryAdvice()
	shapes := []string{"causation", "mechanism", "sequence", "walk-through", "comparison"}
	hits := 0
	for _, s := range shapes {
		if strings.Contains(got, s) {
			hits++
		}
	}
	if hits < 3 {
		t.Errorf("expected ≥ 3 generalised shape terms; got %d in %q", hits, got)
	}
}

func TestR22RetryAdvice_TeachesSubjectVsAnswerDistinction(t *testing.T) {
	// The core insight is "subject the user named ≠ answer they
	// want" — verify the advice text carries this distinction.
	got := r22RetryAdvice()
	if !strings.Contains(got, "SUBJECT") || !strings.Contains(got, "ANSWER") {
		t.Errorf("expected SUBJECT / ANSWER distinction in advice; got %q", got)
	}
}
