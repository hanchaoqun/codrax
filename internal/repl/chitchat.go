package repl

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// ChitchatResponder generates a conversational reply to a REPL turn
// that has been routed away from the analysis pipeline. The caller
// (REPL.chitchatDispatch) treats a non-nil error as a visible failure
// — it prints a warning and does NOT write the turn to memory, so a
// failed responder does not pollute future prior-conversation
// context.
type ChitchatResponder interface {
	Respond(userLine, priorContext string) (reply string, err error)
}

// chitchatSystemPrompt is the responder's single source of persona.
// It deliberately avoids naming pipeline internals (stages, agents,
// tools) so the user-visible surface stays small and the prompt does
// not couple to the analyzer/explorer/extractor/finalizer vocabulary.
const chitchatSystemPrompt = `You are the conversational companion built into a code-analysis tool.
The user is chatting casually — this turn is NOT a request to read or analyze a repository.
Reply briefly and helpfully. Match the user's language (reply in Chinese when the user writes Chinese, English when they write English, etc.).

If the user asks what you can do, explain in plain terms: the tool answers questions about code repositories — you can ask it about functions, files, behaviours, or crash logs, and it will read the relevant source files and produce an answer grounded in the code. This /chat path is reserved for conversation that does not require repository access.

Keep replies short. Do NOT invent tool capabilities, file paths, APIs, or repository details. If the user's casual question drifts into asking for code analysis, suggest they ask it as a regular question instead of prefixing /chat.`

// llmChitchatResponder is the default ChitchatResponder backed by a
// single llm.Adapter.Chat call. No tools, no ReAct loop; the
// responder is a one-shot exchange by design.
type llmChitchatResponder struct {
	adapter llm.Adapter
}

// NewChitchatResponder builds the default responder. Callers pass the
// adapter they want used; a nil adapter yields a responder that errors
// on every Respond so the REPL can print a clean "not configured"
// warning instead of panicking.
func NewChitchatResponder(adapter llm.Adapter) ChitchatResponder {
	return &llmChitchatResponder{adapter: adapter}
}

// Respond makes one LLM call with the current user line plus any
// prior-conversation context the REPL wants to pass for continuity.
// Prior context is folded into the same user message as the current
// line — mirrors the dispatch() idiom so the "match user's language"
// instruction applies to the current turn, not the prior.
func (r *llmChitchatResponder) Respond(userLine, priorContext string) (string, error) {
	if r.adapter == nil {
		return "", fmt.Errorf("chitchat responder not configured: no LLM adapter")
	}
	userLine = strings.TrimSpace(userLine)
	if userLine == "" {
		return "", fmt.Errorf("chitchat responder: empty user line")
	}

	content := userLine
	if prior := strings.TrimSpace(priorContext); prior != "" {
		content = "## Prior conversation (for continuity)\n" + prior +
			"\n\n## Current message\n" + userLine
	}

	messages := []llm.Message{
		{Role: "system", Content: chitchatSystemPrompt},
		{Role: "user", Content: content},
	}

	resp, err := r.adapter.Chat(messages, nil, llm.ChatOptions{})
	if err != nil {
		return "", fmt.Errorf("chitchat llm call: %w", err)
	}
	reply := strings.TrimSpace(resp.Content)
	if reply == "" {
		return "", fmt.Errorf("chitchat llm returned empty content")
	}
	return reply, nil
}

// chitchatDispatch runs one /chat turn. line is the substantive user
// text (command prefix already stripped); display is the form shown
// on the terminal and persisted to memory. This path deliberately
// bypasses the analysis pipeline (no runner.Run call) and does not
// touch the sticky attached-log state — if the user later asks a real
// code question, the existing dispatch() will propagate the log
// exactly as before.
//
// Fail-safe: any responder error prints a warning, logs the detail,
// and returns WITHOUT writing to memory. Polluting prior-conversation
// context with placeholder failure text (the way the pipeline error
// path does) would encourage future responders to hallucinate around
// phantom prior turns; the cleaner contract is "chat this turn failed
// silently from memory's point of view — try again".
func (r *REPL) chitchatDispatch(line, display string) {
	if r.chitchatResponder == nil {
		r.warn("chit-chat is disabled (no responder wired). Ask your operator to enable chitchat_enabled in codrax.yaml.\n")
		return
	}

	prior := r.store.BuildContext(line)

	logging.Info("[repl/chitchat] dispatching: %s", oneLine(line))
	if r.renderer != nil {
		r.renderer.StartSpinner()
	}

	response, err := r.chitchatResponder.Respond(line, prior)

	if r.renderer != nil {
		r.renderer.StopSpinner()
	}

	if err != nil {
		logging.Warning("[repl/chitchat] responder failed: %v", err)
		r.errorf("chat failed: %v\n", err)
		return
	}

	logging.Info("[repl/chitchat] reply:\n%s", response)

	if response == "" {
		fmt.Fprintln(r.out, "  ??")
		return
	}
	r.renderBordered(response)
	r.recordTurn(display, line, response)
}
