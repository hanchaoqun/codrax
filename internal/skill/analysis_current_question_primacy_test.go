// Package skill — analysis_current_question_primacy_test.go (2026-05-10).
//
// Issue B: every intent field the analyzer emits MUST be derived from
// the CURRENT request's text only. Prior Conversation, recalled
// memory, and any historical material participate as REFERENCE-ONLY
// context — never as the primary basis for any field's value.
//
// The pre-existing rule was sub_topics-specific; this generalises it
// to ALL intent fields and pins the wording so a future skill-prompt
// edit cannot accidentally narrow the rule back to one field.
//
// Forensic anchor: 2026-05-10 customer-reported scenario where the
// model was asked to focus on the CURRENT question's user intent
// while keeping historical material as reference material only.
package skill

import (
	"strings"
	"testing"
)

func analysisSkillPrompt(t *testing.T) string {
	t.Helper()
	cfg := BuildAnalysisSkill()
	if cfg == nil {
		t.Fatal("BuildAnalysisSkill returned nil")
	}
	return cfg.OutputFormat
}

func currentQuestionPrimacyBlock(t *testing.T) string {
	t.Helper()
	out := analysisSkillPrompt(t)
	startIdx := strings.Index(out, "## Current-question primacy")
	if startIdx < 0 {
		t.Fatal("primacy heading missing")
	}
	endMarker := "Field enums:"
	endIdx := strings.Index(out[startIdx:], endMarker)
	if endIdx < 0 {
		t.Fatal("end-marker 'Field enums:' missing — primacy block placement broken")
	}
	return out[startIdx : startIdx+endIdx]
}

func semanticPredicatesBlock(t *testing.T) string {
	t.Helper()
	out := analysisSkillPrompt(t)
	startIdx := strings.Index(out, "## Semantic predicates")
	if startIdx < 0 {
		t.Fatal("semantic predicates heading missing")
	}
	endMarker := "Entities:"
	endIdx := strings.Index(out[startIdx:], endMarker)
	if endIdx < 0 {
		t.Fatal("end-marker 'Entities:' missing after semantic predicates")
	}
	return out[startIdx : startIdx+endIdx]
}

func diagnosticPredicateParagraph(t *testing.T) string {
	t.Helper()
	region := semanticPredicatesBlock(t)
	startIdx := strings.Index(region, "- `is_diagnostic_question`")
	if startIdx < 0 {
		t.Fatal("is_diagnostic_question predicate instruction missing")
	}
	endIdx := strings.Index(region[startIdx:], "\n\n")
	if endIdx < 0 {
		t.Fatal("is_diagnostic_question predicate paragraph terminator missing")
	}
	return region[startIdx : startIdx+endIdx]
}

func TestAnalysisSkill_CurrentQuestionPrimacyRulePresent(t *testing.T) {
	out := analysisSkillPrompt(t)
	if !strings.Contains(out, "Current-question primacy") {
		t.Errorf("top-level rule missing — analyzer prompt must lead with 'Current-question primacy' before field-specific rules; got:\n%s", out)
	}
	if !strings.Contains(out, "EVERY field you emit") {
		t.Errorf("rule must be unambiguous about scope (EVERY field, not opt-in)")
	}
}

func TestAnalysisSkill_GoalNamesSemanticPredicatesAndConfidence(t *testing.T) {
	cfg := BuildAnalysisSkill()
	if cfg == nil {
		t.Fatal("BuildAnalysisSkill returned nil")
	}
	if strings.Contains(cfg.Goal, "six fields") {
		t.Fatalf("analysis skill goal must not describe stale six-field classification: %q", cfg.Goal)
	}
	for _, want := range []string{"structured emit_analysis fields", "confidence", "semantic predicates"} {
		if !strings.Contains(cfg.Goal, want) {
			t.Errorf("analysis skill goal must mention %q; got: %q", want, cfg.Goal)
		}
	}
}

func TestAnalysisSkill_CurrentQuestionPrimacy_NamesEveryIntentField(t *testing.T) {
	// Pin that the rule explicitly names every intent field —
	// without explicit enumeration, the LLM can read the rule as
	// "applies to fields the rule mentions" and skip the rest.
	out := currentQuestionPrimacyBlock(t)
	for _, field := range []string{
		"intent",
		"scenario",
		"complexity",
		"question_kind",
		"intent_confidence",
		"complexity_confidence",
		"kind_confidence",
		"keywords",
		"entities",
		"predicates",
		"sub_topics",
		"answer_subject",
		"predicate_axis",
		"exact_targets",
		"exact_context_terms",
		"exact_context_roles",
		"diagram_hint",
		"completeness_obligation",
		"enumeration_boundary",
		"buckets",
		"required_files",
		"irrelevant_files",
	} {
		if !strings.Contains(out, field) {
			t.Errorf("primacy rule must name every intent field; missing %q in:\n%s", field, out)
		}
	}
}

func TestAnalysisSkill_DiagnosticPredicatePromptPinned(t *testing.T) {
	paragraph := diagnosticPredicateParagraph(t)
	for _, want := range []string{
		"is_diagnostic_question",
		"CURRENT request",
		"similar problem still exists",
		"cause / current-risk / remediation analysis",
		"attached log",
		"attached trace",
		"no attachment",
		"how a log/trace feature itself is implemented",
		"intent=root_cause",
		"scenario=performance_bottleneck",
	} {
		if !strings.Contains(paragraph, want) {
			t.Errorf("diagnostic predicate prompt missing %q; got:\n%s", want, paragraph)
		}
	}

	block := semanticPredicatesBlock(t)
	if !strings.Contains(block, "typed flags rather than re-reading the question text") {
		t.Errorf("semantic predicate block must emphasize typed flags over raw text re-interpretation; got:\n%s", block)
	}
}

func TestAnalysisSkill_DiagnosticPredicatePrompt_RedlineAudit(t *testing.T) {
	paragraph := diagnosticPredicateParagraph(t)
	for _, banned := range []string{
		"keyword",
		"regex",
		"regexp",
		"substring",
		"grep",
		"TaskGraph",
		"EvidencePlan",
		"BusContext",
		"MutableState",
		"AnalysisIR",
		"AnalyzerHints",
	} {
		if strings.Contains(paragraph, banned) {
			t.Errorf("diagnostic predicate prompt leaked banned term %q: %q", banned, paragraph)
		}
	}
}

func TestAnalysisSkill_CurrentQuestionPrimacy_PronounExceptionPreserved(t *testing.T) {
	// The single legitimate use of Prior Conversation is pronoun
	// disambiguation. The exception must stay narrow: only the
	// resolved CONCRETE identifier crosses over, never the topic /
	// framing / structural shape.
	out := analysisSkillPrompt(t)
	if !strings.Contains(out, "pronoun") || !strings.Contains(out, "demonstrative") {
		t.Errorf("pronoun-disambiguation exception missing; got:\n%s", out)
	}
	for _, marker := range []string{"\"它\"", "\"那个\"", "\"this\"", "\"that\"", "\"them\""} {
		if !strings.Contains(out, marker) {
			t.Errorf("pronoun exception must list common pronouns/demonstratives across CN+EN; missing %q", marker)
		}
	}
	if !strings.Contains(out, "disambiguation, not lifting") {
		t.Errorf("rule must explicitly distinguish disambiguation from lifting; got:\n%s", out)
	}
}

func TestAnalysisSkill_CurrentQuestionPrimacy_EnumeratesLiftingErrors(t *testing.T) {
	// The rule must enumerate the structural errors so the LLM
	// has concrete instances of "what NOT to do". Without
	// instances, the abstract rule is easy to violate.
	out := analysisSkillPrompt(t)
	for _, banned := range []string{
		"topic / framing / sub-topic from a prior turn",
		"predicate flag value",
		"entity name that appeared in a prior turn",
		"predicate axis",
		"sub-repo scope",
	} {
		if !strings.Contains(out, banned) {
			t.Errorf("lifting-error enumeration missing %q; got:\n%s", banned, out)
		}
	}
}

func TestAnalysisSkill_CurrentQuestionPrimacy_SelfContainedFastPath(t *testing.T) {
	// When the current request is fully self-contained (no
	// pronoun), Prior Conversation has zero bearing. The rule
	// must explicitly say so — otherwise the LLM may default to
	// "consult Prior Conversation to be safe".
	out := analysisSkillPrompt(t)
	if !strings.Contains(out, "self-contained") {
		t.Errorf("self-contained fast path missing; got:\n%s", out)
	}
	if !strings.Contains(out, "no pronoun") {
		t.Errorf("rule must explicitly tie self-contained to no-pronoun-resolution; got:\n%s", out)
	}
	if !strings.Contains(out, "no bearing") && !strings.Contains(out, "NO bearing") {
		t.Errorf("rule must state that Prior Conversation has NO bearing on a self-contained request; got:\n%s", out)
	}
}

func TestAnalysisSkill_CurrentQuestionPrimacy_AppliesToMemoryToo(t *testing.T) {
	// recall_memory / agent memory is the OTHER source of historical
	// material. The rule must cover it the same way — otherwise
	// the LLM can route around the rule via memory recall.
	out := analysisSkillPrompt(t)
	if !strings.Contains(out, "recall_memory") {
		t.Errorf("primacy rule must cover memory recall, not just Prior Conversation; got:\n%s", out)
	}
	if !strings.Contains(out, "memory-recalled material") {
		t.Errorf("rule must explicitly classify memory-recalled material as REFERENCE-only; got:\n%s", out)
	}
}

func TestAnalysisSkill_CurrentQuestionPrimacy_PrecedesFieldRules(t *testing.T) {
	// The top-level rule must appear BEFORE per-field instructions,
	// so the LLM anchors on "current question first" before reading
	// any field-specific guidance. If the rule moves below field
	// rules, it loses its anchoring effect.
	out := analysisSkillPrompt(t)
	primacyIdx := strings.Index(out, "Current-question primacy")
	if primacyIdx < 0 {
		t.Fatal("primacy rule heading not found")
	}
	// Pick a few representative field-specific instructions that
	// existed before this rule was added.
	for _, field := range []string{
		"## Sub-topic detection",
		"## Confidence",
		"## Semantic predicates",
		"## Question structural obligations",
	} {
		idx := strings.Index(out, field)
		if idx < 0 {
			t.Errorf("field-specific section %q missing — test fixture stale?", field)
			continue
		}
		if idx < primacyIdx {
			t.Errorf("primacy rule must appear BEFORE field-specific section %q (idx %d) — got primacy at idx %d", field, idx, primacyIdx)
		}
	}
}

func TestAnalysisSkill_CurrentQuestionPrimacy_R6Audit(t *testing.T) {
	// The skill prompt is LLM-facing — internal pipeline jargon
	// must stay out of the new rule. Window the audit to just the
	// primacy block (heading → "Field enums:" sentinel that marks
	// the end of my insertion). The next `##` heading lives well
	// past field-specific prose that legitimately mentions
	// "explorer agent" as an entity-axes example, so a naive
	// "next ##" window false-positives there.
	region := currentQuestionPrimacyBlock(t)
	for _, banned := range []string{
		"explorer agent", "extractor agent", "finalizer agent", "analyzer agent",
		"BusContext", "MutableState", "AnalysisIR",
		"L1 gate", "L2 gate", "L3 gate", "L4 gate",
		"AnalyzerHints",
	} {
		if strings.Contains(region, banned) {
			t.Errorf("internal term %q leaked into LLM-facing primacy rule: %q", banned, region)
		}
	}
}
