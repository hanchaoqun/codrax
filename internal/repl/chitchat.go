package repl

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/memory"
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

// streamingChitchatResponder is an optional extension interface. When
// the REPL detects that the configured responder also satisfies this,
// it wires the callback into a live pterm.Area so the user sees the
// reply token-by-token while the call is still in flight. The final
// accumulated text is still returned, and the caller re-renders it
// through the glamour-backed renderBordered path after the area
// closes — so the streaming phase never competes with markdown
// formatting. Callers that do not implement this fall back to the
// synchronous Respond with a spinner.
type streamingChitchatResponder interface {
	RespondStream(userLine, priorContext string, onDelta func(string)) (reply string, err error)
}

// chitchatSystemPrompt is the responder's single source of persona.
// It deliberately avoids naming pipeline internals (stages, agents,
// tools) so the user-visible surface stays small and the prompt does
// not couple to the analyzer/explorer/extractor/finalizer vocabulary.
//
// Identity is fixed: the chit-chat persona is CODRAX itself. When the
// user asks "who are you" / "what are you" / "who made you", the
// model must answer as CODRAX and surface the author contact
// (hanssccv@gmail.com) verbatim for the author question — this is
// the single public-facing surface where that address is advertised,
// so we anchor it in the prompt rather than leaving it up to the
// model's priors.
const chitchatSystemPrompt = `You are CODRAX, a read-only code-analysis tool. The user is chatting casually with you — this turn is NOT a request to read or analyze a repository.

Identity:
- Your name is CODRAX. When asked who you are or what you are, answer as CODRAX: a read-only, grounded code-analysis assistant that reads source files and produces answers backed by real file:line citations.
- When asked who built you / who your author is / who made you / 作者是谁 / 谁做的, answer with the author's email exactly: hanssccv@gmail.com. Include the address verbatim in your reply.

Style:
- Reply briefly and helpfully.
- Match the user's language (reply in Chinese when the user writes Chinese, English when they write English, and so on).
- Do NOT invent tool capabilities, file paths, APIs, or repository details.

Scope:
- If the user asks what you can do, explain in plain terms: you answer questions about code repositories — functions, files, behaviours, or crash logs — by reading the relevant source and returning an answer grounded in the code. This /chat path is reserved for conversation that does not require repository access.
- If the user's casual question drifts into asking for code analysis, suggest they ask it as a regular question instead of prefixing /chat.`

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

// RespondStream is the streaming sibling of Respond. onDelta fires
// on every content chunk the adapter surfaces; it is a no-op when
// the underlying adapter is non-streaming (the delta callback is
// simply never invoked and the final reply still arrives via the
// return value). Implements streamingChitchatResponder so the REPL
// can light up the pterm.Area live-reply path.
func (r *llmChitchatResponder) RespondStream(userLine, priorContext string, onDelta func(string)) (string, error) {
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

	resp, err := r.adapter.Chat(messages, nil, llm.ChatOptions{OnContentDelta: onDelta})
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

	prior := r.store.BuildContext(line, memory.BuildOpts{
		Kind:      memory.KindChitchat,
		SessionID: r.sessionID,
	})

	logging.Info("[repl/chitchat] dispatching: %s", oneLine(line))

	// Streaming path is used when (a) the responder implements the
	// optional streaming interface AND (b) we have a live renderer —
	// i.e. interactive TTY mode. In tests (renderer nil) and in any
	// non-TTY context we skip the pterm.Area and fall back to the
	// spinner+sync path so scripted output stays byte-stable.
	streamer, canStream := r.chitchatResponder.(streamingChitchatResponder)
	streaming := canStream && r.renderer != nil

	var response string
	var err error
	if streaming {
		response, err = r.runStreamingChitchat(streamer, line, prior)
	} else {
		if r.renderer != nil {
			r.renderer.StartSpinner()
		}
		response, err = r.chitchatResponder.Respond(line, prior)
		if r.renderer != nil {
			r.renderer.StopSpinner()
		}
	}

	if err != nil {
		logging.Warning("[repl/chitchat] responder failed: %v", err)
		r.errorf("chat failed: %s\n", friendlyRunError(r.language, err))
		return
	}

	logging.Info("[repl/chitchat] reply:\n%s", response)

	if response == "" {
		fmt.Fprintln(r.out, emptyResponseHint(r.language))
		return
	}
	// One-line marker so the user can tell at a glance the answer
	// came from chitchat (no repo analysis, no plan) vs the main
	// pipeline. Pre-fix: only difference was an INFO log line —
	// invisible at the terminal — so a misrouted code question got
	// a generic answer with no recovery hint.
	fmt.Fprintln(r.out, chitchatReplyHeader(r.language))
	r.renderBordered(response)
	r.recordTurn(display, line, response, memory.KindChitchat)
}

// chitchatStreamRedrawInterval throttles pterm.Area redraws during a
// streaming chit-chat reply. Every incoming delta appends to the
// accumulator; a ticker drains the latest snapshot to the Area at
// this cadence. 80ms (~12.5 fps) reads as smooth typewriter motion
// without thrashing terminals that choke on rapid cursor-up redraws.
const chitchatStreamRedrawInterval = 80 * time.Millisecond

// runStreamingChitchat renders a live pterm.Area for the assistant's
// reply while the LLM is still generating, then tears the area down
// so the caller's renderBordered re-prints the accumulated text with
// full glamour formatting. The area is WithRemoveWhenDone so Stop
// wipes the raw stream preview — no double-printed text.
//
// Concurrency model: streamer.RespondStream runs on this goroutine
// and invokes onDelta synchronously from the SSE read loop. A
// separate ticker goroutine owns all pterm.Area writes; onDelta only
// mutates the accumulator under a mutex. This keeps pterm off the
// network goroutine (where a slow terminal could block SSE reads)
// and ensures redraws happen at a bounded rate even when the model
// emits chunks faster than the terminal can repaint.
func (r *REPL) runStreamingChitchat(streamer streamingChitchatResponder, userLine, prior string) (string, error) {
	area, err := pterm.DefaultArea.WithRemoveWhenDone(true).Start()
	if err != nil {
		// Falling back to the sync path keeps chit-chat usable when the
		// terminal refuses to host a live area (uncommon; pterm starts
		// succeed on every TTY we've seen).
		logging.Warning("[repl/chitchat] pterm area start failed, falling back to sync: %v", err)
		return r.chitchatResponder.Respond(userLine, prior)
	}

	var mu sync.Mutex
	var buf strings.Builder
	dirty := false
	stopTicker := make(chan struct{})
	tickerDone := make(chan struct{})

	snapshot := func() string {
		mu.Lock()
		defer mu.Unlock()
		if !dirty {
			return ""
		}
		dirty = false
		return buf.String()
	}

	go func() {
		defer close(tickerDone)
		ticker := time.NewTicker(chitchatStreamRedrawInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopTicker:
				return
			case <-ticker.C:
				if snap := snapshot(); snap != "" {
					area.Update(snap)
				}
			}
		}
	}()

	onDelta := func(delta string) {
		if delta == "" {
			return
		}
		mu.Lock()
		buf.WriteString(delta)
		dirty = true
		mu.Unlock()
	}

	response, err := streamer.RespondStream(userLine, prior, onDelta)

	close(stopTicker)
	<-tickerDone
	// Drain any final bytes the ticker missed. We still stop the area
	// immediately after — renderBordered will redraw the full reply
	// with glamour formatting, so this last Update is optional but
	// avoids a visible blank frame on very short replies.
	if snap := snapshot(); snap != "" {
		area.Update(snap)
	}
	if stopErr := area.Stop(); stopErr != nil {
		logging.Debug("[repl/chitchat] pterm area stop: %v", stopErr)
	}
	return response, err
}

// ChitchatClassifier decides whether a user turn should be routed to
// the chit-chat responder (bypassing the analysis pipeline) or to the
// normal pipeline path. A non-nil error means "could not decide" and
// the REPL's gate treats it as "fall through to pipeline" — the safe
// default, because over-analyzing a casual greeting wastes cycles but
// under-analyzing a real code question gives the wrong answer.
type ChitchatClassifier interface {
	Classify(userLine string) (isChitchat bool, err error)
}

// chitchatClassifierTool is the structured-output schema the LLM
// classifier is required to emit. Kept LOCAL to this file — it is
// NOT registered in tool.Registry because the classifier bypasses the
// agent framework entirely and makes one direct adapter.Chat call
// with this schema passed inline.
//
// Enum is 2-valued and exhaustive: every user turn is either
// chitchat or repo_question. There is no "unknown" — when genuinely
// uncertain, the schema description instructs the model to emit
// repo_question (safer default). A structural enum with clear
// non-keyword examples is the right mechanism for classification
// disambiguation; the repo's red line forbids adding keyword tables
// or reconcile-style overrides in Go code.
var chitchatClassifierTool = llm.ToolSchema{
	Name:        "emit_chitchat_classification",
	Description: "Classify the user's turn as casual chit-chat or a repository question. Exactly one call per turn.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "decision": {
      "type": "string",
      "enum": ["chitchat", "repo_question"],
      "description": "chitchat = greetings, thanks, casual conversation, meta-questions about this tool's capabilities, off-topic chat. repo_question = any request that needs reading repository files, tracing logic, searching symbols, answering factual questions about code, investigating a crash, or analyzing behaviour. When uncertain, emit repo_question — over-analyzing is safe, under-analyzing is not."
    },
    "reason": {
      "type": "string",
      "description": "One short sentence naming the structural signal that justified the decision. Not shown to the user; used for debugging and telemetry only."
    }
  },
  "required": ["decision", "reason"]
}`),
}

const chitchatClassifierSystemPrompt = `You route each user turn to one of two handlers.

Emit exactly one call to emit_chitchat_classification with a decision
of "chitchat" or "repo_question". Never emit any other tool and never
emit free-form text.

Examples of chitchat (not exhaustive — use structural judgement):
- "hello", "你好", "谢谢" without a follow-on code question
- "what can you do?", "你是谁?"
- pleasantries, acknowledgements, meta-conversation about the tool

Examples of repo_question (not exhaustive):
- "how does X work?", "X 在哪里被调用?"
- "why did this panic happen?", "这个函数返回什么?"
- any question that names a file, function, symbol, config key, or behaviour
- any request to read, find, trace, compare, or explain code

When a turn looks ambiguous (e.g. a continuation like "那它呢" without
prior context), emit repo_question — the pipeline handles it safely
and the cost of being wrong is higher for chitchat than for pipeline.`

// llmChitchatClassifier is the default ChitchatClassifier. One
// adapter.Chat call per turn, with tool_choice=required and the local
// schema above.
type llmChitchatClassifier struct {
	adapter llm.Adapter
}

// NewChitchatClassifier builds the default classifier. Nil adapter
// yields a classifier that errors on every Classify so the gate falls
// through to the pipeline (fail-safe).
func NewChitchatClassifier(adapter llm.Adapter) ChitchatClassifier {
	return &llmChitchatClassifier{adapter: adapter}
}

// Classify makes one LLM call and parses the tool_call arguments.
// ANY error path (nil adapter, chat error, no tool call emitted,
// unparseable JSON, unknown decision value) returns (false, err).
// The caller treats that as "fall through to pipeline", so the
// classifier cannot silently reroute a real code question to the
// chit-chat responder when it is genuinely confused.
func (c *llmChitchatClassifier) Classify(userLine string) (bool, error) {
	if c.adapter == nil {
		return false, fmt.Errorf("chitchat classifier not configured: no LLM adapter")
	}
	userLine = strings.TrimSpace(userLine)
	if userLine == "" {
		return false, fmt.Errorf("chitchat classifier: empty user line")
	}
	messages := []llm.Message{
		{Role: "system", Content: chitchatClassifierSystemPrompt},
		{Role: "user", Content: userLine},
	}
	tools := []llm.ToolSchema{chitchatClassifierTool}
	resp, err := c.adapter.Chat(messages, tools, llm.ChatOptions{ToolChoice: "required"})
	if err != nil {
		return false, fmt.Errorf("chitchat classifier llm call: %w", err)
	}
	if len(resp.ToolCalls) == 0 {
		return false, fmt.Errorf("chitchat classifier: LLM returned no tool_call")
	}
	// Use the FIRST tool call — schema forbids more, but be liberal in
	// what we accept. Downstream only cares about decision.
	call := resp.ToolCalls[0]
	if call.Name != chitchatClassifierTool.Name {
		return false, fmt.Errorf("chitchat classifier: unexpected tool %q", call.Name)
	}
	var parsed struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(call.Params, &parsed); err != nil {
		return false, fmt.Errorf("chitchat classifier: unmarshal tool params: %w", err)
	}
	switch parsed.Decision {
	case "chitchat":
		logging.Debug("[repl/chitchat] classifier → chitchat: %s", oneLineClamp(parsed.Reason, 120))
		return true, nil
	case "repo_question":
		logging.Debug("[repl/chitchat] classifier → repo_question: %s", oneLineClamp(parsed.Reason, 120))
		return false, nil
	default:
		return false, fmt.Errorf("chitchat classifier: unknown decision %q", parsed.Decision)
	}
}

// oneLineClamp collapses a string to a single line and clips to n
// runes. Used for classifier debug logging where a multi-line reason
// would mangle log readability.
func oneLineClamp(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if n > 0 {
		runes := []rune(s)
		if len(runes) > n {
			return string(runes[:n]) + "…"
		}
	}
	return s
}
