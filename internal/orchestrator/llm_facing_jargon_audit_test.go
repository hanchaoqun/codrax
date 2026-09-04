package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestReviewerPrompts_LLMFacingNoInternalJargon is the orchestrator's
// prompt-surface marker in the glossarylint renderer roster (§40.52).
// It replaces the hand-curated map of six reviewer system prompts plus
// one tool description (which had silently omitted the semantic-quality
// reviewer prompt and six tool schemas) with the shape-bound census:
// every package-level `…Prompt` const and every llm.ToolSchema literal
// in this package is found by the AST, its text resolved, and scanned
// against the full glossary. A reviewer added with a new construction
// shape fails loud instead of slipping past the roster.
//
// The census also pins the roster size so a reviewer file deleted or
// renamed away from the `…Prompt` / llm.ToolSchema shapes is noticed.
func TestReviewerPrompts_LLMFacingNoInternalJargon(t *testing.T) {
	surfaces := glossarylint.RunPromptSurfaceScan(t, ".")
	prompts := map[string]bool{}
	schemas := 0
	for _, s := range surfaces {
		owner := s.Label[strings.LastIndex(s.Label, " ")+1:]
		if strings.HasPrefix(owner, "ToolSchema.") {
			if owner == "ToolSchema.Name" {
				schemas++
			}
			continue
		}
		prompts[owner] = true
	}
	for _, want := range []string{
		"selfConsistencyReviewerSystemPrompt",
		"semanticQualityReviewerSystemPrompt",
		"answerReviewerSystemPrompt",
		"continuationClassifierSystemPrompt",
		"acceptanceSystemPrompt",
		"planCriticSystemPrompt",
		"reflectorSystemPrompt",
	} {
		if !prompts[want] {
			t.Errorf("reviewer prompt %s no longer reaches the prompt-surface census (renamed away from the …Prompt shape?)", want)
		}
	}
	if schemas < 8 {
		t.Errorf("expected at least 8 llm.ToolSchema literals in the orchestrator (acceptance, answer-pattern, continuation, plan-critic, reflector, failure-pattern, semantic-quality, self-consistency), census found %d", schemas)
	}
}
