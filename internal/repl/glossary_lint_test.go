package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInReplPrompts is internal/repl's marker in the
// glossarylint renderer roster (§40.52). The REPL package mixes
// operator-facing text (help, config-knob guidance, status cards) with
// model prompts, so it uses the shape-bound lane: every package-level
// `…Prompt` const and every llm.ToolSchema literal (turn policy,
// chit-chat, command-operation, data-task lanes) is resolved by the AST
// and scanned; any other way of binding a prompt fails loud. Before
// §40.52 this package had no jargon gate at all — the first red run
// listed "Low-mind precedence:" in the turn-policy system prompt and
// "the analyzer" in the emit_turn_policy schema.
func TestNoInternalTermsInReplPrompts(t *testing.T) {
	surfaces := glossarylint.RunPromptSurfaceScan(t, ".")
	owners := map[string]bool{}
	for _, s := range surfaces {
		owners[s.Label[strings.LastIndex(s.Label, " ")+1:]] = true
	}
	for _, want := range []string{"turnPolicySystemPrompt", "localResponderSystemPrompt", "chitchatSystemPrompt", "commandOperationPlannerSystemPrompt", "dataTaskPlannerSystemPrompt", "ToolSchema.Parameters"} {
		if !owners[want] {
			t.Errorf("prompt surface %s no longer reaches the census (renamed away from the …Prompt / llm.ToolSchema shape?)", want)
		}
	}
}
