package skill

import (
	"strings"
	"testing"
)

// TestNoKeywordExamplesInEnums is the batch-4A hard gate against the
// red line documented in feedback_no_custom_keyword_matching:
//
//   "LLM classification errors must be fixed by sharpening schema
//    descriptions, not by teaching the LLM keyword examples — each
//    example seeds a future drift and the enum descriptions must
//    stay structural."
//
// The test scans every classification enum description
// (AnalysisIntentChoices / AnalysisComplexityChoices /
// AnalysisQuestionKindChoices / AnalysisAnswerShapeChoices /
// AnalysisAnswerSubjectChoices / AnalysisPredicateAxisChoices) and
// the AnalysisHardRules slice for two disallowed patterns:
//
//   1. English imperative user-wording fragments wrapped in double
//      quotes — "how many X", "list all X", "what is X", etc. The
//      enum description should describe the STRUCTURE of the
//      subject, not the surface wording the user might use.
//   2. Chinese imperative cue characters — the common question-form
//      particles (统计 / 多少 / 几个 / 怎么 / 什么时候 / 是什么 /
//      对比 etc.). Any of these inside an enum description is a
//      keyword-match-by-prompt-example in disguise.
//
// The exact phrase list lives in skill.KeywordExamplePhrases so it
// can be updated in one place; this test just iterates it.
//
// Counterpart to TestNoInternalTermsInPrompts / InToolSchemas /
// InHints from batches 2A/2B: those three catch implementation
// jargon; this one catches classification by example. Both classes
// of leak erode the LLM's schema-driven classification.
func TestNoKeywordExamplesInEnums(t *testing.T) {
	corpora := map[string][]AnalysisEnumChoice{
		"analysisIntents":         AnalysisIntentChoices(),
		"analysisComplexities":    AnalysisComplexityChoices(),
		"analysisQuestionKinds":   AnalysisQuestionKindChoices(),
		"analysisAnswerShapes":    AnalysisAnswerShapeChoices(),
		"analysisAnswerSubjects":  AnalysisAnswerSubjectChoices(),
		"analysisPredicateAxes":   AnalysisPredicateAxisChoices(),
	}

	var hits []string
	for name, choices := range corpora {
		for _, c := range choices {
			for _, phrase := range KeywordExamplePhrases {
				if phrase == "" {
					continue
				}
				if strings.Contains(c.Desc, phrase) {
					hits = append(hits, name+"["+c.Value+"].desc: contains keyword example "+phraseQuote(phrase))
				}
			}
		}
	}

	// AnalysisHardRules — the rendered Prohibitions for the analyzer.
	// Keyword examples leaked here are functionally the same as enum
	// desc leaks: both teach the LLM that certain surface words map
	// to certain classifications.
	for i, rule := range AnalysisHardRules {
		for _, phrase := range KeywordExamplePhrases {
			if phrase == "" {
				continue
			}
			if strings.Contains(rule, phrase) {
				hits = append(hits, "AnalysisHardRules["+itoa(i)+"]: contains keyword example "+phraseQuote(phrase))
			}
		}
	}

	if len(hits) == 0 {
		return
	}

	// Per-violation lines via t.Errorf first, summary via t.Fatalf
	// last so the full list is visible in a single test run.
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
	t.Fatalf("TestNoKeywordExamplesInEnums found %d keyword-example leak(s); replace with structural descriptions per docs/prompt_glossary.md §7", len(hits))
}

// phraseQuote wraps a phrase in quotes, doubled if the phrase itself
// contains quotes (rare — only to keep log lines unambiguous).
func phraseQuote(p string) string {
	if strings.ContainsAny(p, `"'`) {
		return "«" + p + "»"
	}
	return `"` + p + `"`
}
