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
// `…Prompt` const, every llm.ToolSchema literal (turn policy, chit-chat,
// command-operation, data-task lanes) and every llm.Message literal is
// found by the AST; system prompts are resolved exactly and the
// runtime-assembled user-role instruction text (classifier user blobs,
// no-tool / structured-tool repair prompts, tool replies) is bound by
// text flow into its builder functions. Any other way of binding a
// prompt fails loud. Before §40.52 this package had no jargon gate at
// all — the first red run listed "Low-mind precedence:" in the
// turn-policy system prompt and "the analyzer" in the emit_turn_policy
// schema. EVOLUTION RECORD (§40.52 fold-in, G6-jargon #1): the lane
// originally bound only Role:"system" Content, so the Role:"user"
// repair prompts sat outside every lint; the census below pins that
// they are now bound (a glossary token injected into
// turnPolicyStructuralRepairPrompt is reported — red proof in the
// fold-in record).
func TestNoInternalTermsInReplPrompts(t *testing.T) {
	surfaces := glossarylint.RunPromptSurfaceScan(t, ".")
	owners := map[string]bool{}
	userTexts := 0
	var userText strings.Builder
	for _, s := range surfaces {
		owner := s.Label[strings.LastIndex(s.Label, " ")+1:]
		owners[owner] = true
		if (owner == "UserMessage.Content" || owner == "ToolMessage.Content") && s.Text != "" {
			userTexts++
			userText.WriteString(s.Text)
			userText.WriteString("\n")
		}
	}
	for _, want := range []string{"turnPolicySystemPrompt", "localResponderSystemPrompt", "chitchatSystemPrompt", "commandOperationPlannerSystemPrompt", "dataTaskPlannerSystemPrompt", "ToolSchema.Parameters", "UserMessage.Content", "ToolMessage.Content"} {
		if !owners[want] {
			t.Errorf("prompt surface %s no longer reaches the census (renamed away from the …Prompt / llm.ToolSchema / llm.Message shape?)", want)
		}
	}
	if userTexts < 10 {
		t.Errorf("expected at least 10 user-role instruction surfaces with bound text, got %d", userTexts)
	}
	// The user-role builders reached by flow: the classifier repair
	// prompts (direct call), the data-task no-tool repair (call with the
	// base prompt), the planner prompt reached through the `prompt`
	// parameter of planDataTaskWithTool, and the classifier user blob
	// assembled in a strings.Builder.
	for _, want := range []string{
		"Re-emit one complete emit_turn_policy call",         // turnPolicyPresentationProvenanceRepairPrompt
		"## last_answer_present: true",                       // classifyPolicyLLM builder blob
		"## repair_scope",                                    // operationStructuredToolRepairPrompt / data structured repair
		"## continuation_context",                            // command-operation planner blob
		"## original_planning_context",                       // dataTaskPlannerNoToolRepairPrompt
		"not available — only recall_memory and list_memory", // chit-chat tool reply
	} {
		if !strings.Contains(userText.String(), want) {
			t.Errorf("user-role instruction text %q is no longer bound by the prompt-surface lane", want)
		}
	}
}
