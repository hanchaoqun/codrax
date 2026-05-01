package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// ContinuationClassifier decides whether the current REPL turn is a
// continuation of the prior conversation (and so the downstream
// pipeline stages should INHERIT the prior-conversation context) or a
// fresh self-contained question (and so prior context should be
// hidden to keep the analyzer / explorer focused on the current
// request alone).
//
// Pre-commit-48 the decision lived in types.IsContinuation, which
// matched user input against a hardcoded continuationPrefixes list
// ("再", "继续", "more on", ...). That is exactly the keyword-table
// pattern feedback_no_custom_keyword_matching.md flags as an
// anti-pattern — REPL-control or otherwise, hardcoded user-input
// keywords are a brittle classifier the LLM can model better.
//
// Mirror of internal/repl/chitchat.go::ChitchatClassifier (by design,
// reuses the same pattern). One direct adapter.Chat call with a
// structured emit tool, tool_choice=required. Failure paths return
// (false, err) so the caller falls back to the existing keyword
// heuristic — graceful degradation, not hard dependency.
type ContinuationClassifier interface {
	Classify(ctx context.Context, current, prior string) (isContinuation bool, err error)
}

// continuationClassifierTool is the structured-output schema. Local
// to this file (bypasses the agent framework, mirroring chitchat).
var continuationClassifierTool = llm.ToolSchema{
	Name:        "emit_continuation_classification",
	Description: "Decide whether the current request is a continuation of the prior conversation (downstream stages should inherit prior context) or a fresh self-contained question. Exactly one call per turn.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "decision": {
      "type": "string",
      "enum": ["continuation", "fresh"],
      "description": "continuation = current message references the prior turn (pronoun-only head, follow-up phrasing like '再详细讲一下', 'more on that', or topic-drilling on something just answered). fresh = current message names its own subject and would be answerable without prior context. When uncertain, emit fresh — over-isolating the current request is safer than poisoning it with stale prior-conversation entities."
    },
    "reason": {
      "type": "string",
      "description": "One short sentence naming the structural signal that justified the decision. Not shown to the user; used for debugging + telemetry."
    }
  },
  "required": ["decision", "reason"]
}`),
}

const continuationClassifierSystemPrompt = `You decide whether the current user message is a CONTINUATION of the prior conversation, or a FRESH self-contained request.

Emit exactly one call to emit_continuation_classification with decision = "continuation" or "fresh". Never emit any other tool, never emit free-form text.

Definition: a message is "continuation" when answering it correctly REQUIRES context from the prior turn — typically because the message refers to something just discussed without re-naming it. Examples:
- "那它是什么意思" / "what does that mean" — pronoun without sibling identifier
- "再详细一点" / "more on that" — follow-up phrasing
- "expand 5" after a numbered list was just shown — drill-in reference
- "继续上面的分析" / "continue with the analysis above" — explicit back-reference

A message is "fresh" when it names its own subject and would be answerable without prior context. Examples:
- "How does the orchestrator dispatch stages?" — names entity
- "Read internal/agent/explorer.go and explain the keyword search" — concrete file
- "Why does TestFoo fail?" — names test

Use structural judgement, not keyword matching. A message containing "再" might be fresh ("再实现一个 X", "implement another X"); a message without obvious continuation cues might be continuation ("它的返回值呢" → pronoun-headed, no identifier).

When the prior section is empty or absent, the only legal answer is "fresh" — there is no prior turn to continue from.

When uncertain, emit "fresh". The downstream cost of falsely treating a continuation as fresh (re-doing some analysis the prior turn already did) is much smaller than the cost of falsely treating a fresh question as continuation (poisoning the analyzer's entity disambiguation with stale prior-turn entities).`

// llmContinuationClassifier is the default implementation. One Chat
// call per turn with tool_choice=required.
type llmContinuationClassifier struct {
	adapter llm.Adapter
}

// NewContinuationClassifier wires the adapter. Nil adapter yields a
// classifier whose Classify always errors so the caller falls back to
// the keyword heuristic in types.IsContinuation. Wired from cmd/root.go
// using providers.yaml :: agents.continuation_classifier (recommended:
// cheap-model routing).
func NewContinuationClassifier(adapter llm.Adapter) ContinuationClassifier {
	return &llmContinuationClassifier{adapter: adapter}
}

// Classify dispatches one Chat. Failure paths return (false, err)
// without raising — caller's responsibility to fall back.
func (c *llmContinuationClassifier) Classify(ctx context.Context, current, prior string) (bool, error) {
	if c == nil || c.adapter == nil {
		return false, fmt.Errorf("continuation classifier: no adapter")
	}
	current = strings.TrimSpace(current)
	prior = strings.TrimSpace(prior)
	if current == "" {
		return false, fmt.Errorf("continuation classifier: empty current")
	}
	if prior == "" {
		// No prior — by definition fresh. Skip the LLM round-trip.
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userBlob := "## prior\n" + prior + "\n\n## current\n" + current
	messages := []llm.Message{
		{Role: "system", Content: continuationClassifierSystemPrompt},
		{Role: "user", Content: userBlob},
	}
	tools := []llm.ToolSchema{continuationClassifierTool}
	resp, err := c.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return false, fmt.Errorf("continuation classifier: chat: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return false, fmt.Errorf("continuation classifier: no tool call")
	}
	call := resp.ToolCalls[0]
	if call.Name != continuationClassifierTool.Name {
		return false, fmt.Errorf("continuation classifier: unexpected tool %q", call.Name)
	}
	var parsed struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		return false, fmt.Errorf("continuation classifier: unmarshal: %w", err)
	}
	switch parsed.Decision {
	case "continuation":
		logging.Debug("[continuation_classifier] continuation reason=%q", parsed.Reason)
		return true, nil
	case "fresh":
		logging.Debug("[continuation_classifier] fresh reason=%q", parsed.Reason)
		return false, nil
	default:
		return false, fmt.Errorf("continuation classifier: unknown decision %q", parsed.Decision)
	}
}
