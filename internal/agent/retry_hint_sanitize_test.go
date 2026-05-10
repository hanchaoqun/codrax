package agent

import (
	"os"
	"regexp"
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

// === Fix-γ regression tests (2026-05-10 R6 audit) ===

// TestEnumerationIntentError_NoGoStageConsts pins the analyzer's
// enumeration retry hint against re-leaking Go const names. The
// 2026-05-10 audit found `(e.g. StageAnalyze, StageExplore, ...)`
// hardcoded in analyzer.go:1741; Fix-β replaced it with plain prose.
// This test asserts no Stage* / Phase* PascalCase Go names survive
// (matching the same shape the audit caught).
func TestEnumerationIntentError_NoGoStageConsts(t *testing.T) {
	src, err := os.ReadFile("analyzer.go")
	if err != nil {
		t.Fatalf("read analyzer.go: %v", err)
	}
	body := string(src)
	// Locate the enumeration-intent block by its anchor sentence.
	anchor := "enumeration intent with ≤1 distinct named entity is structurally inconsistent"
	idx := strings.Index(body, anchor)
	if idx < 0 {
		t.Fatalf("anchor sentence missing from analyzer.go — has the enumeration block moved?")
	}
	// Scan the next ~1200 chars (covers the multi-line fmt.Errorf).
	end := idx + 1200
	if end > len(body) {
		end = len(body)
	}
	region := body[idx:end]
	// Match `Stage<UpperWord>` / `Phase<UpperWord>` shapes the audit
	// found leaking. These are Go const names from
	// internal/types/context.go and must never reach the LLM.
	re := regexp.MustCompile(`\b(Stage|Phase)[A-Z][a-zA-Z]+\b`)
	if locs := re.FindAllString(region, -1); len(locs) > 0 {
		t.Errorf("R6 regression: enumeration retry hint leaks Go const(s) %v in:\n%s", locs, region)
	}
}

// TestSanitize_GoStyleStageNamesPassThrough documents an expected
// gap and pins it: the sanitizer's vocab table is hand-curated, not
// a generic PascalCase scrubber, so a chain like
// `StageAnalyze, StageExplore` would PASS THROUGH unchanged. The
// system relies on Fix-β-style source-side discipline to keep such
// names out of LLM-facing hints in the first place — this test
// exists so a future writer who is tempted to broaden the
// sanitizer reads the trade-off here first.
func TestSanitize_GoStyleStageNamesPassThrough(t *testing.T) {
	in := "examples include StageAnalyze, StageExplore, and StageExtract"
	out := sanitizeInternalVocab(in)
	if out != in {
		t.Errorf("sanitize unexpectedly rewrote Go-style stage names:\nin:  %q\nout: %q\n"+
			"NOTE: if you're broadening the sanitizer, review skill prompt + analyzer code"+
			" for Stage*/Phase* literals before relying on the new behaviour.",
			in, out)
	}
}
