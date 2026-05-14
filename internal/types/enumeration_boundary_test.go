package types

import "testing"

// enumeration_boundary_test.go — v3.1 (2026-05-05) cleanup. The
// regex-driven RecoverRequestedEnumerationBoundary path was deleted
// per feedback_no_custom_keyword_matching.md (the 4-pattern keyword
// table covered only EN/ZH counter words from the original eval
// set; would miss any future Chinese counter / Japanese / Korean /
// novel English shape). Tests for that function were removed.
//
// Surviving typed-only surface tests:
//   - NormalizeRequestedEnumerationBoundary (typed validation,
//     verifies LLM-emitted SourceQuote is verbatim in RawRequest)
//   - RequestedEnumerationBoundaryOwner (entity-based, no regex)
//   - EnumerationBoundaryCountString (typed integer → string)

func TestNormalizeRequestedEnumerationBoundary_AcceptsLLMEmitWithVerbatimQuote(t *testing.T) {
	raw := "list all 9 checks performed by gate.Run"
	in := &RequestedEnumerationBoundary{DeclaredCount: 9, SourceQuote: "9 checks"}
	got := NormalizeRequestedEnumerationBoundary(raw, in)
	if got == nil {
		t.Fatal("expected normalized boundary; got nil")
	}
	if got.DeclaredCount != 9 || got.SourceQuote != "9 checks" {
		t.Errorf("unexpected normalized boundary: %+v", got)
	}
}

func TestNormalizeRequestedEnumerationBoundary_RejectsQuoteWithoutDeclaredCount(t *testing.T) {
	raw := "列出 internal/analysis 下所有子包及各自入口点"
	in := &RequestedEnumerationBoundary{DeclaredCount: 26, SourceQuote: "所有子包"}
	if got := NormalizeRequestedEnumerationBoundary(raw, in); got != nil {
		t.Errorf("quote without explicit numeric count must be rejected; got %+v", got)
	}
}

func TestNormalizeRequestedEnumerationBoundary_RejectsEmbeddedDifferentCount(t *testing.T) {
	raw := "list all 13 checks performed by gate.Run"
	in := &RequestedEnumerationBoundary{DeclaredCount: 3, SourceQuote: "13 checks"}
	if got := NormalizeRequestedEnumerationBoundary(raw, in); got != nil {
		t.Errorf("embedded different count must be rejected; got %+v", got)
	}
}

func TestNormalizeRequestedEnumerationBoundary_AcceptsLeadingZeroCountToken(t *testing.T) {
	raw := "list all 07 checks performed by gate.Run"
	in := &RequestedEnumerationBoundary{DeclaredCount: 7, SourceQuote: "07 checks"}
	got := NormalizeRequestedEnumerationBoundary(raw, in)
	if got == nil {
		t.Fatal("expected normalized boundary for leading-zero count token; got nil")
	}
	if got.DeclaredCount != 7 || got.SourceQuote != "07 checks" {
		t.Errorf("unexpected normalized boundary: %+v", got)
	}
}

func TestNormalizeRequestedEnumerationBoundary_RejectsFabricatedQuote(t *testing.T) {
	raw := "describe the architecture of the system"
	in := &RequestedEnumerationBoundary{DeclaredCount: 9, SourceQuote: "9 layers"}
	if got := NormalizeRequestedEnumerationBoundary(raw, in); got != nil {
		t.Errorf("fabricated quote (not in raw) must be rejected; got %+v", got)
	}
}

func TestNormalizeRequestedEnumerationBoundary_NilInput(t *testing.T) {
	if got := NormalizeRequestedEnumerationBoundary("any raw", nil); got != nil {
		t.Errorf("nil input must return nil; got %+v", got)
	}
}

func TestNormalizeRequestedEnumerationBoundary_ZeroOrNegativeCount(t *testing.T) {
	raw := "test"
	for _, n := range []int{0, -1, -99} {
		got := NormalizeRequestedEnumerationBoundary(raw, &RequestedEnumerationBoundary{
			DeclaredCount: n,
			SourceQuote:   "test",
		})
		if got != nil {
			t.Errorf("count=%d must return nil; got %+v", n, got)
		}
	}
}

func TestEnumerationBoundaryCountString(t *testing.T) {
	cases := []struct {
		b    *RequestedEnumerationBoundary
		want string
	}{
		{nil, ""},
		{&RequestedEnumerationBoundary{DeclaredCount: 0}, ""},
		{&RequestedEnumerationBoundary{DeclaredCount: -1}, ""},
		{&RequestedEnumerationBoundary{DeclaredCount: 9}, "9"},
		{&RequestedEnumerationBoundary{DeclaredCount: 100}, "100"},
	}
	for _, tc := range cases {
		if got := EnumerationBoundaryCountString(tc.b); got != tc.want {
			t.Errorf("EnumerationBoundaryCountString(%+v) = %q, want %q", tc.b, got, tc.want)
		}
	}
}

func TestRequestedEnumerationBoundaryOwner_SingleEntityReturnsIt(t *testing.T) {
	rm := RequestModel{
		RawRequest: "describe gate.Run behavior",
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"gate.Run"},
		},
	}
	if got := RequestedEnumerationBoundaryOwner(rm); got != "gate.Run" {
		t.Errorf("got %q, want %q", got, "gate.Run")
	}
}

func TestRequestedEnumerationBoundaryOwner_MultipleEntitiesReturnsEmpty(t *testing.T) {
	rm := RequestModel{
		RawRequest: "compare gate.Run with checker.Verify",
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"gate.Run", "checker.Verify"},
		},
	}
	if got := RequestedEnumerationBoundaryOwner(rm); got != "" {
		t.Errorf("multi-entity request must return empty; got %q", got)
	}
}
