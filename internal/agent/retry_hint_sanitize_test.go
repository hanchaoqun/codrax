package agent

import (
	"strings"
	"testing"
)

// === sanitizeInternalVocab ===

// TestSanitize_DottedPredicateForms — Python-attr-access shaped tokens
// rewrite to prose. Bare snake_case stays.
func TestSanitize_DottedPredicateForms(t *testing.T) {
	in := "predicates.is_cross_component=true but only 0 sub-topic emitted"
	out := sanitizeInternalVocab(in)
	if strings.Contains(out, "predicates.is_cross_component") {
		t.Errorf("dotted form should be sanitized; got %q", out)
	}
	if !strings.Contains(out, "is_cross_component flag") {
		t.Errorf("expected prose form to mention the flag; got %q", out)
	}
	// Bare `sub-topic` survives (skill vocab)
	if !strings.Contains(out, "sub-topic emitted") {
		t.Errorf("bare sub-topic vocab must survive; got %q", out)
	}
}

// TestSanitize_GoStyleForms — CamelCase Go-style accessors rewrite to
// snake_case skill vocab.
func TestSanitize_GoStyleForms(t *testing.T) {
	in := "R2.1 scalar_multi_topic: Predicates.IsScalarAnswer=true but 2 sub-topics emitted"
	out := sanitizeInternalVocab(in)
	if strings.Contains(out, "Predicates.IsScalarAnswer") {
		t.Errorf("Go-style form must be sanitized; got %q", out)
	}
	if !strings.Contains(out, "is_scalar_answer flag") {
		t.Errorf("expected snake_case vocab; got %q", out)
	}
}

// TestSanitize_AnalyzerHintsFields
func TestSanitize_AnalyzerHintsFields(t *testing.T) {
	in := "AnalyzerHints.PrimaryEntities are inconsistent with AnalyzerHints.PrimaryScopes"
	out := sanitizeInternalVocab(in)
	if strings.Contains(out, "AnalyzerHints.") {
		t.Errorf("AnalyzerHints accessor must be sanitized; got %q", out)
	}
	if !strings.Contains(out, "your primary entities") {
		t.Errorf("expected prose for primary entities; got %q", out)
	}
	if !strings.Contains(out, "your sub-repo scopes") {
		t.Errorf("expected prose for primary scopes; got %q", out)
	}
}

// TestSanitize_RmStructForms
func TestSanitize_RmStructForms(t *testing.T) {
	in := "rm.SubTopics carries 0 entries but rm.Predicates.IsCrossComponent=true"
	out := sanitizeInternalVocab(in)
	if strings.Contains(out, "rm.SubTopics") {
		t.Errorf("rm.SubTopics must be sanitized; got %q", out)
	}
	if strings.Contains(out, "rm.Predicates") {
		t.Errorf("rm.Predicates must be sanitized; got %q", out)
	}
	if !strings.Contains(out, "your sub_topics") {
		t.Errorf("expected your sub_topics prose; got %q", out)
	}
}

// TestSanitize_BareSnakeCaseStays — `is_cross_component` (NO dot) must
// stay as-is. The skill prompt teaches it as the predicate flag.
func TestSanitize_BareSnakeCaseStays(t *testing.T) {
	in := "set is_cross_component=false explicitly"
	out := sanitizeInternalVocab(in)
	if !strings.Contains(out, "is_cross_component=false") {
		t.Errorf("bare snake_case must survive sanitization; got %q", out)
	}
}

// TestSanitize_ToolNamesStay — emit_analysis is a tool the LLM must
// call; sanitizing it would break the structured retry directive.
func TestSanitize_ToolNamesStay(t *testing.T) {
	in := "re-emit emit_analysis with at least 2 sub_topics"
	out := sanitizeInternalVocab(in)
	if !strings.Contains(out, "emit_analysis") {
		t.Errorf("tool name emit_analysis must survive; got %q", out)
	}
}

// TestSanitize_EmptyAndNoMatch
func TestSanitize_EmptyAndNoMatch(t *testing.T) {
	if got := sanitizeInternalVocab(""); got != "" {
		t.Errorf("empty: want empty, got %q", got)
	}
	plain := "no internal tokens at all in this message"
	if got := sanitizeInternalVocab(plain); got != plain {
		t.Errorf("no-match must be unchanged; got %q", got)
	}
}

// TestSanitize_LongerAlternationWins — `predicates.is_cross_component`
// (long) must be matched before any shorter overlapping prefix that
// could theoretically partially match.
func TestSanitize_LongerAlternationWins(t *testing.T) {
	in := "X predicates.is_cross_component Y"
	out := sanitizeInternalVocab(in)
	// We expect ONE replacement, not partial replacement.
	if !strings.Contains(out, "the is_cross_component flag") {
		t.Errorf("long alternation should fully replace; got %q", out)
	}
	if strings.Contains(out, "predicates.is_cross_component") {
		t.Errorf("long form should be entirely consumed; got %q", out)
	}
}

// TestSanitize_AppliesViaPlainCoherenceDetail — end-to-end through the
// LLM-prompt boundary.
func TestSanitize_AppliesViaPlainCoherenceDetail(t *testing.T) {
	leaky := "R1.2 predicate_contradiction: predicates.is_cross_component=true but only 0 sub-topic emitted"
	out := plainCoherenceDetail(leaky)
	if strings.Contains(out, "R1.2 predicate_contradiction") {
		t.Errorf("rule-code prefix should be stripped; got %q", out)
	}
	if strings.Contains(out, "predicates.is_cross_component") {
		t.Errorf("dotted form should be sanitized at LLM boundary; got %q", out)
	}
	if !strings.Contains(out, "is_cross_component flag") {
		t.Errorf("expected prose form; got %q", out)
	}
}

// TestSanitize_CJKContextPreserved — non-ASCII surrounding text stays.
// Cross-language safety: sanitizer must not mangle CJK / Cyrillic /
// Arabic prose around the sanitized tokens.
func TestSanitize_CJKContextPreserved(t *testing.T) {
	in := "请检查 predicates.is_cross_component 标志(应当是 true)"
	out := sanitizeInternalVocab(in)
	if strings.Contains(out, "predicates.is_cross_component") {
		t.Errorf("dotted form should be sanitized in CJK prose; got %q", out)
	}
	if !strings.Contains(out, "请检查") || !strings.Contains(out, "标志") {
		t.Errorf("CJK surrounding text must be preserved; got %q", out)
	}
}

// TestSanitize_R1_8ScopeAdvisoryDoesNotLeak — the new R1.8 detail
// string should round-trip cleanly through plainCoherenceDetail.
func TestSanitize_R1_8ScopeAdvisoryDoesNotLeak(t *testing.T) {
	in := "R1.8 scope_anchor_distribution (advisory): cross-component question with 2 sub-repo scopes [a, b] but 1 sub-repo(s) [a] have no sub-topic anchor — if the answer should compare each sub-repo, consider adding a sub-topic per scope; if a cross-cutting decomposition is intentional, ignore this advisory"
	out := plainCoherenceDetail(in)
	if strings.Contains(out, "R1.8") {
		t.Errorf("R1.8 prefix should be stripped; got %q", out)
	}
	if !strings.Contains(out, "sub-repo") {
		t.Errorf("user-visible sub-repo vocab must survive; got %q", out)
	}
}

// TestSanitize_AllVocabLookups — sanity check every entry in the
// abstraction table actually round-trips through the regex engine.
// Catches future additions that QuoteMeta would mishandle.
func TestSanitize_AllVocabLookups(t *testing.T) {
	internalVocabOnce.Do(buildInternalVocab)
	for k, want := range internalVocabMap {
		got := sanitizeInternalVocab("X " + k + " Y")
		if !strings.Contains(got, want) {
			t.Errorf("entry %q expected to produce %q; got %q", k, want, got)
		}
	}
}
