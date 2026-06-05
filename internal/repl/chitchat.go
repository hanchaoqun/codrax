package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// memoryReaderAlias is the package-private alias the public
// MemoryReader name resolves to. Defined as a private name so the
// chitchat package can import types and re-export the cleaner
// MemoryReader identifier without syntactic noise at the use site.
type memoryReaderAlias = types.MemoryReader

// ChitchatResponder generates a conversational reply to a REPL turn
// that has been routed away from the analysis pipeline. The caller
// (REPL.chitchatDispatch) treats a non-nil error as a visible failure
// — it prints a warning and does NOT write the turn to memory, so a
// failed responder does not pollute future prior-conversation
// context.
type ChitchatResponder interface {
	Respond(ctx context.Context, userLine, priorContext string) (reply string, err error)
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
	RespondStream(ctx context.Context, userLine, priorContext string, onDelta func(string)) (reply string, err error)
}

// toolableChitchatResponder is the optional surface for responders
// that can call tools (today: just recall_memory). The dispatcher
// probes for this AFTER streaming and prefers it when both the
// responder and BusContext.Memory are non-nil — the LLM can then
// look up prior conversation entries on demand instead of relying
// on the keyword-bound priorContext block alone.
//
// The boundary type is types.MemoryReader (defined in internal/types
// to avoid import cycles); the responder implementation in this file
// dispatches recall_memory tool calls onto it directly.
type toolableChitchatResponder interface {
	RespondWithMemory(ctx context.Context, userLine, priorContext string, memory MemoryReader, onDelta func(string)) (reply string, err error)
}

// MemoryReader re-exports types.MemoryReader so chitchat callers do
// not have to import internal/types just to satisfy the optional
// interface above. Pure type alias — same value, no wrapping.
type MemoryReader = memoryReaderAlias

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

// chitchatSystemPromptToolUse is appended to the base system prompt
// when the recall_memory tool is wired in. Tells the LLM the single
// tool is available and when to use it. Same trigger language as the
// explorer skill prose: any meta-reference to "previous / earlier /
// in memory / 之前 / 上次 / 记忆里 / 历史 / 我们讨论过" should call
// recall_memory FIRST so the answer is grounded in the actual
// IndexEntry results, not hallucinated continuity. Without the tool
// (memory unwired) this addendum is omitted and the responder falls
// back to the priorContext-only path.
const chitchatSystemPromptToolUse = `

Available tools:
- recall_memory(query, kind?, limit?, include_body?) — TOPIC search. Use when the user references a SPECIFIC past topic ("我们之前讨论过 OAuth / what did we say about the panic? / did I ask about X?"). recall_memory ranks entries by keyword overlap with your chosen query. Generic listing-intent queries like "memory recall history content" only surface self-referential meta-entries — don't use recall_memory for listings.
- list_memory(limit?, kind?, since?) — LISTING / browse mode. Use when the user asks for an INVENTORY of memory ("都有哪些 / 历史里有什么 / 记忆里都记了些什么 / what's in memory / list all / show me my history / what have I asked you"). Returns the most-recent N entries by TIME, no keyword filter. This is the right tool when the user wants to SEE the conversation history, not search a specific topic.

Tool routing — pick by intent:
  TOPIC (specific keyword)  → recall_memory(query="oauth migration")
  LISTING (broad inventory) → list_memory(limit=10)
  AMBIGUOUS                 → start with list_memory; subsequent
                              user turns can drill into a specific
                              entry via recall_memory next turn.

Multi-turn refinement pattern (preferred for "precise" recall):
  Turn N — user asks "都有哪些 X 相关的" (broad)
           you call list_memory; reply "I see 3 turns about X: A, B, C — which one?"
  Turn N+1 — user picks "A"
              you call recall_memory(query="A details") for the deep look.
This is more reliable than guessing a topic-specific query when the user themselves haven't named one.

Tool-use rules:
- For pure greetings / identity / capability questions (你好 / 谁做的 / 你能干什么), DO NOT call any tool — answer directly from this system prompt.
- Call AT MOST ONE tool per turn (either recall_memory OR list_memory, never both within one turn — multi-turn refinement is the chained path).
- After the tool result returns, write the final reply directly. Do not call a second tool even if the first result looked thin.`

// llmChitchatResponder is the default ChitchatResponder. The
// pre-tool-use shape was a single one-shot Chat call; with the
// optional recall_memory tool it becomes a bounded 2-round loop:
// round 1 with the tool catalogue, round 2 (no tools) to answer.
// Memory unwired → loop short-circuits to the original one-shot
// behaviour byte-identically.
type llmChitchatResponder struct {
	adapter  llm.Adapter
	settings types.ChitchatSettings // tool-use limits; zero values fall through to package defaults
}

// SetChitchatSettings overrides the tool-use limits on the default
// llmChitchatResponder. Called by cmd/root.go after construction so
// codrax.yaml's chitchat_recall_default_limit / _max_limit reach
// dispatchChitchatRecall without changing the constructor signature.
// Operators who never set the yaml keys see the type's zero values
// fall through to the package defaults (5 / 10).
func (r *llmChitchatResponder) SetChitchatSettings(s types.ChitchatSettings) {
	if r == nil {
		return
	}
	r.settings = s
}

// recallMemoryToolSchema is the tool catalog entry the tool-use
// loop hands to the LLM. Schema is held inline (not imported from
// internal/tool) so this package does not depend on the tool layer
// — the chitchat path is for non-pipeline conversation and must
// stay independent of the analyzer/explorer wiring. The shape is
// intentionally a SUBSET of the production recall_memory schema:
// just query + limit. kind / session_id / include_body are not
// surfaced because chitchat does not need that selectivity (it's
// always a one-shot search at the topical level).
func recallMemoryToolSchema() llm.ToolSchema {
	return llm.ToolSchema{
		Name: "recall_memory",
		Description: "TOPIC search of the user's prior conversation memory. " +
			"Call when the user references a SPECIFIC past topic " +
			"('did we discuss X / 之前关于 X 怎么说的'). For LISTING " +
			"intent ('what's in memory / 都有哪些'), use list_memory " +
			"instead — recall_memory's keyword scoring will only " +
			"surface self-referential entries on a generic query. " +
			"Returns a list of compacted memory entries (Topic + " +
			"Summary + Keywords + Kind + timestamp).",
		Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string",  "description": "Natural-language search term — your interpretation of what the user is asking about, NOT the literal question text."},
    "limit": {"type": "integer", "description": "Max entries to return. Default 5, hard cap 10."}
  },
  "required": ["query"]
}`),
	}
}

// listMemoryToolSchema is the chitchat-side companion to
// recallMemoryToolSchema. Browse-mode (recency-ordered) listing
// of memory; same boundary discipline (subset schema, no Kind /
// Since exposure since chitchat doesn't need that selectivity).
func listMemoryToolSchema() llm.ToolSchema {
	return llm.ToolSchema{
		Name: "list_memory",
		Description: "LISTING / browse mode of the user's prior " +
			"conversation memory. Call when the user asks for an " +
			"INVENTORY of memory ('都有哪些 / 历史里有什么 / 记忆里都记了 / " +
			"what's in memory / list all / show my history'). Returns " +
			"the most-recent N entries by TIME, no keyword filter. " +
			"For TOPIC search ('we discussed X'), use recall_memory.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "limit": {"type": "integer", "description": "Max entries to return. Default 10, hard cap 30."}
  }
}`),
	}
}

// Tool-use limits live on the responder's ChitchatSettings field
// (settable via SetChitchatSettings, populated by cmd/root.go from
// the codrax.yaml chitchat_recall_default_limit / _max_limit keys).
// The package-level constants the previous shape held are gone —
// the resolver in dispatchChitchatRecall handles zero values.

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
func (r *llmChitchatResponder) Respond(ctx context.Context, userLine, priorContext string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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

	resp, err := r.adapter.Chat(ctx, messages, nil, llm.ChatOptions{})
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
func (r *llmChitchatResponder) RespondStream(ctx context.Context, userLine, priorContext string, onDelta func(string)) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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

	resp, err := r.adapter.Chat(ctx, messages, nil, llm.ChatOptions{OnContentDelta: onDelta})
	if err != nil {
		return "", fmt.Errorf("chitchat llm call: %w", err)
	}
	reply := strings.TrimSpace(resp.Content)
	if reply == "" {
		return "", fmt.Errorf("chitchat llm returned empty content")
	}
	return reply, nil
}

// RespondWithMemory is the toolableChitchatResponder entry point.
// Runs a bounded 2-round ReAct loop with the recall_memory tool
// available; falls through to the no-tool single-call path when
// memory is nil. onDelta is the optional streaming callback — fires
// only on the FINAL Chat call (the one that produces the user-
// visible reply); intermediate tool-call rounds are non-streaming
// because we need the complete tool_use payload before we can
// decide whether the round is finished.
//
// Bounded by design: at most one recall_memory call per turn, then
// one final synthesis Chat. Either the LLM has enough info from one
// recall to answer, or the topic is genuinely absent and it should
// say so honestly. A second tool call would give the LLM rope to
// hang itself with an iterative search that costs tokens but adds
// nothing — chitchat is conversation, not investigation.
func (r *llmChitchatResponder) RespondWithMemory(ctx context.Context, userLine, priorContext string, mem types.MemoryReader, onDelta func(string)) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.adapter == nil {
		return "", fmt.Errorf("chitchat responder not configured: no LLM adapter")
	}
	userLine = strings.TrimSpace(userLine)
	if userLine == "" {
		return "", fmt.Errorf("chitchat responder: empty user line")
	}
	// No memory wired → fall through to the historical one-shot
	// path. Streaming preference preserved: when onDelta is non-nil
	// we still light up the live area on RespondStream, otherwise
	// the synchronous Respond.
	if mem == nil {
		if onDelta != nil {
			return r.RespondStream(ctx, userLine, priorContext, onDelta)
		}
		return r.Respond(ctx, userLine, priorContext)
	}

	systemPrompt := chitchatSystemPrompt + chitchatSystemPromptToolUse
	userContent := userLine
	if prior := strings.TrimSpace(priorContext); prior != "" {
		userContent = "## Prior conversation (for continuity)\n" + prior +
			"\n\n## Current message\n" + userLine
	}
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	// Round 1 — tool catalogue available, no streaming so we can
	// fully observe ToolCalls before committing. Both recall_memory
	// (topic search) and list_memory (recency listing) are exposed
	// so the LLM picks by intent — the system prompt teaches the
	// TOPIC-vs-LISTING routing rule.
	tools := []llm.ToolSchema{recallMemoryToolSchema(), listMemoryToolSchema()}
	resp, err := r.adapter.Chat(ctx, messages, tools, llm.ChatOptions{})
	if err != nil {
		return "", fmt.Errorf("chitchat llm round 1: %w", err)
	}

	// No tool call → the LLM had enough to answer in one shot.
	// Surface the content directly — but we lost the streaming
	// opportunity here because we had to disable it for round 1.
	// Acceptable trade-off: chitchat replies are short, and a
	// brief non-streaming pause is fine when no tool is needed.
	// The (much rarer) tool-use path also needs the final call to
	// stream, so we run it explicitly below.
	if len(resp.ToolCalls) == 0 {
		reply := strings.TrimSpace(resp.Content)
		if reply == "" {
			return "", fmt.Errorf("chitchat llm returned empty content")
		}
		return reply, nil
	}

	// Tool dispatch. We honour exactly one recall_memory call —
	// any extra calls in the same response are ignored (the
	// system prompt already says so explicitly; this enforces it).
	// Mismatched tool names also drop through; the model will
	// notice the missing tool result and answer from priorContext.
	messages = append(messages, llm.Message{
		Role:             "assistant",
		Content:          resp.Content,
		ReasoningContent: resp.ReasoningContent,
		ToolCalls:        resp.ToolCalls,
	})
	for _, call := range resp.ToolCalls {
		var toolReply string
		switch call.Name {
		case "recall_memory":
			toolReply = dispatchChitchatRecall(call, mem, r.settings)
		case "list_memory":
			toolReply = dispatchChitchatList(call, mem, r.settings)
		default:
			toolReply = fmt.Sprintf("(tool %q not available — only recall_memory and list_memory are wired)", call.Name)
			logging.Debug("[repl/chitchat] tool_use unknown tool=%q", call.Name)
		}
		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    toolReply,
			ToolCallID: call.ID,
		})
		// Bound: one tool dispatch per turn.
		break
	}

	// Round 2 — synthesise the user-visible reply. No tools this
	// round (system prompt already capped to one call). Streaming
	// re-enabled here because it's the user-facing answer.
	finalResp, err := r.adapter.Chat(ctx, messages, nil, llm.ChatOptions{OnContentDelta: onDelta})
	if err != nil {
		return "", fmt.Errorf("chitchat llm round 2: %w", err)
	}
	reply := strings.TrimSpace(finalResp.Content)
	if reply == "" {
		return "", fmt.Errorf("chitchat llm round 2 returned empty content")
	}
	return reply, nil
}

// dispatchChitchatRecall parses the LLM's tool_use params, calls
// MemoryReader.Search, and returns the text payload the tool
// message will carry back to the next Chat round. Errors are
// transparently surfaced as the tool reply — the LLM sees the
// failure and can mention "I tried to search but couldn't" in its
// reply rather than hallucinating around the gap.
//
// Debug coverage: every dispatch logs (call.Params raw, parsed
// query, mem-state, entry count) so an operator inspecting a
// finalised chitchat turn can reconstruct what happened in the
// tool round between the two LLM calls. Pre-this-fix the round
// was a black box — the user only saw the round-1 finish_reason
// and the round-2 final reply.
func dispatchChitchatRecall(call llm.ToolCall, mem types.MemoryReader, settings types.ChitchatSettings) string {
	logging.Debug("[repl/chitchat] tool_use recall_memory params=%s mem_nil=%t",
		string(call.Params), mem == nil)
	if call.Name != "recall_memory" {
		msg := fmt.Sprintf("(tool %q not available in this context — only recall_memory is wired)", call.Name)
		logging.Debug("[repl/chitchat] tool_result recall_memory rejected: unknown tool %q", call.Name)
		return msg
	}
	var p struct {
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(call.Params, &p); err != nil {
		msg := fmt.Sprintf("(recall_memory tool params invalid: %v)", err)
		logging.Debug("[repl/chitchat] tool_result recall_memory rejected: bad params: %v", err)
		return msg
	}
	q := strings.TrimSpace(p.Query)
	if q == "" {
		logging.Debug("[repl/chitchat] tool_result recall_memory rejected: empty query")
		return "(recall_memory rejected: empty query — pass at least one keyword / topic)"
	}
	if mem == nil {
		// Defensive: r.memory should be non-nil by REPL construction
		// (cmd/root.go wires memory.NewAdapter(store)) but a future
		// refactor that hands a nil adapter into RespondWithMemory
		// would otherwise panic on mem.Search. Surface a typed reply
		// matching the production recall_memory tool's "unavailable"
		// message so the LLM has a consistent fail-mode to react to.
		logging.Debug("[repl/chitchat] tool_result recall_memory unavailable: mem is nil (REPL not wired)")
		return "(recall_memory unavailable: prior-conversation memory is not wired in this REPL session)"
	}
	settings = types.ResolvedChitchatSettings(settings)
	limit := p.Limit
	if limit <= 0 {
		limit = settings.RecallDefaultLimit
	}
	if limit > settings.RecallMaxLimit {
		limit = settings.RecallMaxLimit
	}
	entries := mem.Search(q, types.MemorySearchOpts{Limit: limit})
	logging.Debug("[repl/chitchat] tool_result recall_memory query=%q limit=%d entries=%d",
		q, limit, len(entries))
	return renderChitchatRecallResults(q, entries)
}

// dispatchChitchatList is the chitchat companion to dispatchChitchat-
// Recall: instead of scoring by query, it enumerates the most-recent
// N entries via MemoryLister.List. Capability-gated — when the wired
// MemoryReader does not implement MemoryLister (test stubs, future
// alternative impls), surfaces a typed unavailable reply matching
// the recall_memory unavailable shape so the LLM has a consistent
// fail-mode.
func dispatchChitchatList(call llm.ToolCall, mem types.MemoryReader, settings types.ChitchatSettings) string {
	logging.Debug("[repl/chitchat] tool_use list_memory params=%s mem_nil=%t",
		string(call.Params), mem == nil)
	var p struct {
		Limit int `json:"limit,omitempty"`
	}
	if len(call.Params) > 0 {
		if err := json.Unmarshal(call.Params, &p); err != nil {
			msg := fmt.Sprintf("(list_memory tool params invalid: %v)", err)
			logging.Debug("[repl/chitchat] tool_result list_memory rejected: bad params: %v", err)
			return msg
		}
	}
	if mem == nil {
		logging.Debug("[repl/chitchat] tool_result list_memory unavailable: mem is nil (REPL not wired)")
		return "(list_memory unavailable: prior-conversation memory is not wired in this REPL session)"
	}
	lister, ok := mem.(types.MemoryLister)
	if !ok {
		logging.Debug("[repl/chitchat] tool_result list_memory unavailable: MemoryReader does not implement MemoryLister")
		return "(list_memory unavailable: the wired MemoryReader does not support browse-mode listing in this build)"
	}
	settings = types.ResolvedChitchatSettings(settings)
	limit := p.Limit
	if limit <= 0 {
		limit = settings.ListDefaultLimit
	}
	if limit > settings.ListMaxLimit {
		limit = settings.ListMaxLimit
	}
	entries := lister.List(types.MemoryListOpts{Limit: limit})
	logging.Debug("[repl/chitchat] tool_result list_memory limit=%d entries=%d",
		limit, len(entries))
	return renderChitchatListResults(limit, entries)
}

// renderChitchatListResults formats listing entries for tool-message
// inclusion. Same per-entry shape as renderChitchatRecallResults so
// the LLM parses both consistently; banner says "[list_memory]" so
// the LLM doesn't confuse listing with topic search.
//
// Closing hint (multi-turn refinement): list_memory output ends
// with a one-line guidance reminding the LLM that recall_memory is
// the next step when the user names a specific entry. Trains the
// chained-tool pattern documented in chitchatSystemPromptToolUse.
func renderChitchatListResults(limit int, entries []types.MemoryIndexEntry) string {
	var b strings.Builder
	if len(entries) == 0 {
		fmt.Fprintf(&b, "[list_memory] requested limit=%d returned 0 entries.\n", limit)
		fmt.Fprintln(&b, "Memory is genuinely empty in this REPL session. Tell the user honestly there is no prior conversation yet.")
		return b.String()
	}
	fmt.Fprintf(&b, "[list_memory] %d entries (most-recent first):\n\n", len(entries))
	for i, e := range entries {
		fmt.Fprintf(&b, "#%d %s", i+1, e.ID)
		if e.Kind != "" {
			fmt.Fprintf(&b, " (kind=%s)", e.Kind)
		}
		if !e.Timestamp.IsZero() {
			fmt.Fprintf(&b, " ts=%s", e.Timestamp.Format("2006-01-02 15:04:05"))
		}
		b.WriteByte('\n')
		if e.Topic != "" {
			fmt.Fprintf(&b, "   topic: %s\n", e.Topic)
		}
		if e.Summary != "" {
			fmt.Fprintf(&b, "   summary: %s\n", e.Summary)
		}
		b.WriteByte('\n')
	}
	fmt.Fprintln(&b, "Hint: present these entries to the user as a menu of available conversation topics. If the user names a SPECIFIC entry on their next turn, call recall_memory(query=\"<topic words>\") to surface the full content of that entry.")
	return b.String()
}

// renderChitchatRecallResults formats the IndexEntry slice for
// inclusion in a tool message. Same shape as the production
// recall_memory tool's output (so the LLM's parsing patterns are
// consistent across paths) but without the include_body branch
// since chitchat doesn't surface that knob.
func renderChitchatRecallResults(query string, entries []types.MemoryIndexEntry) string {
	var b strings.Builder
	if len(entries) == 0 {
		fmt.Fprintf(&b, "[recall_memory] query=%q matched 0 entries.\n", query)
		fmt.Fprintln(&b, "Nothing in prior conversation memory mentions this topic. Tell the user honestly that you do not recall such discussion.")
		return b.String()
	}
	fmt.Fprintf(&b, "[recall_memory] query=%q matched %d entries (ranked by relevance):\n\n", query, len(entries))
	for i, e := range entries {
		fmt.Fprintf(&b, "#%d %s", i+1, e.ID)
		if e.Kind != "" {
			fmt.Fprintf(&b, " (kind=%s)", e.Kind)
		}
		b.WriteByte('\n')
		if e.Topic != "" {
			fmt.Fprintf(&b, "   topic: %s\n", e.Topic)
		}
		if e.Summary != "" {
			fmt.Fprintf(&b, "   summary: %s\n", e.Summary)
		}
		if len(e.Keywords) > 0 {
			fmt.Fprintf(&b, "   keywords: %s\n", strings.Join(e.Keywords, ", "))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// isStreamingChitchat / isToolableChitchat are the tiny type-assert
// predicates the dispatcher uses to pick the path. Pulled out so the
// switch in chitchatDispatch reads top-down without nested
// `_, ok := X.(...)` clutter.
func isStreamingChitchat(c ChitchatResponder) bool {
	_, ok := c.(streamingChitchatResponder)
	return ok
}

func isToolableChitchat(c ChitchatResponder) bool {
	_, ok := c.(toolableChitchatResponder)
	return ok
}

// runToolableChitchat drives the tool-use loop variant of the
// dispatch. Streaming is wired into the FINAL synthesis call inside
// the responder (round-1 must be non-streaming so we observe the
// tool_use payload before deciding next step); the renderer-based
// live area lights up only for the answer-producing chunks.
func (r *REPL) runToolableChitchat(ctx context.Context, line, prior string) (string, error) {
	tool := r.chitchatResponder.(toolableChitchatResponder)
	// Streaming hookup mirrors runStreamingChitchat: when the
	// renderer is alive AND the responder will eventually surface
	// content deltas, we light up an Area that draws the partial
	// reply. When renderer is nil (tests / non-TTY), pass nil to
	// onDelta and let the synchronous Chat path serve the reply.
	if r.renderer == nil {
		return tool.RespondWithMemory(ctx, line, prior, r.memory, nil)
	}
	// Use a fresh pterm.Area so the streaming render path is
	// contained — matches the runStreamingChitchat structure but
	// without the streaming-only setup tail. Spinner during round 1
	// (no tokens yet to stream); area opens at first delta.
	//
	// WithRemoveWhenDone(true) is load-bearing: the dispatcher
	// renders the final reply via r.renderBordered after this fn
	// returns, so the live-stream area must be CLEARED on Stop.
	// Without it the user sees the reply twice — once as the
	// streamed snapshot (no border) and once again inside the
	// bordered block. runStreamingChitchat already uses the same
	// option for the same reason; both paths must stay in sync.
	r.renderer.StartSpinner()
	var buf strings.Builder
	var areaMu sync.Mutex
	var area *pterm.AreaPrinter
	startedArea := false
	onDelta := func(delta string) {
		if delta == "" {
			return
		}
		areaMu.Lock()
		defer areaMu.Unlock()
		if !startedArea {
			r.renderer.StopSpinner()
			a, err := pterm.DefaultArea.WithRemoveWhenDone(true).Start()
			if err != nil {
				return
			}
			area = a
			startedArea = true
		}
		buf.WriteString(delta)
		if area != nil {
			area.Update(buf.String())
		}
	}
	reply, err := tool.RespondWithMemory(ctx, line, prior, r.memory, onDelta)
	areaMu.Lock()
	if area != nil {
		_ = area.Stop()
	}
	if !startedArea {
		// Stream never opened — the responder either never streamed
		// or the round-1 path returned content directly. Stop the
		// spinner manually so the user sees a clean state.
		armDockTerminalState(r.renderer, err)
		r.renderer.StopSpinner()
	}
	areaMu.Unlock()
	return reply, err
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
		if r.renderer != nil && r.renderer.SpinnerActive() {
			r.renderer.MarkRunFatal()
			r.renderer.StopSpinner()
		}
		r.warn("chit-chat is disabled (no responder wired). Ask your operator to enable chitchat_enabled in codrax.yaml.\n")
		return
	}

	// Per-turn ctx: SIGINT handler can cancel us via cancelTurn()
	// so an in-flight chitchat HTTP request unwinds immediately
	// instead of waiting for the LLM call's natural completion.
	ctx := r.startTurn()
	defer r.endTurn()

	prior := r.store.BuildContext(line, memory.BuildOpts{
		Kind:      memory.KindChitchat,
		SessionID: r.sessionID,
	})

	logging.Info("[repl/chitchat] dispatching: %s", oneLine(line))

	// Arm the dock shutdown override BEFORE any branch starts a
	// spinner — runToolableChitchat / runStreamingChitchat both
	// call StopSpinner internally before returning, so the override
	// must be cached on the renderer at that moment. Same shape as
	// the local route: "◇ 闲聊回复 · 未读仓库 · 未生成 plan · 总耗时 Xs"
	// replaces the default "◆ 已结束 · …" line. Armed unconditionally
	// — the badge describes WHICH route ran; the error surface
	// below is a separate concern.
	chatLabel, chatSegs := chitchatRouteSummary(r.language)
	if r.renderer != nil {
		// Same lightweight-route discipline as the local route: no
		// pipeline stages, so totalStages=0 disables the K/N counter.
		// SetRouteSummary carries the label/segments into both the
		// shutdown summary and (via composeCurrentDockRows) row 2's
		// stage-label slot during dispatch.
		r.renderer.SetTotalStages(0)
		r.renderer.SetRouteSummary(chatLabel, chatSegs)
	}

	// Dispatch path selection, in priority order:
	//   1. toolableChitchatResponder + memory wired → tool-use loop.
	//      Streaming is wired only into the FINAL Chat call inside
	//      the loop (round-1 tool detection cannot stream).
	//   2. streamingChitchatResponder + live renderer → streaming
	//      single-call path with pterm.Area live area.
	//   3. plain Respond → single-call sync with spinner.
	var response string
	var err error
	switch {
	case r.chitchatResponder != nil && r.memory != nil && isToolableChitchat(r.chitchatResponder):
		response, err = r.runToolableChitchat(ctx, line, prior)
	case isStreamingChitchat(r.chitchatResponder) && r.renderer != nil:
		streamer := r.chitchatResponder.(streamingChitchatResponder)
		response, err = r.runStreamingChitchat(ctx, streamer, line, prior)
	default:
		if r.renderer != nil {
			r.renderer.StartSpinner()
		}
		response, err = r.chitchatResponder.Respond(ctx, line, prior)
		if r.renderer != nil {
			armDockTerminalState(r.renderer, err)
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
	// Renderer-nil fallback: tests / scripted mode without a TTY
	// renderer never run StartSpinner / StopSpinner, so the dock
	// shutdown override never fires. Emit the same line directly
	// using the shared formatter so the route badge still shows.
	if r.renderer == nil {
		fmt.Fprintln(r.out, render.FormatLightRouteSummary(chatLabel, chatSegs, "", r.language))
	}
	// Same rendering pipeline as the local route: mermaid →
	// ASCII grids + glamour markdown styling. Pre-2026-04-30 the
	// chitchat path called renderBordered on raw markdown source,
	// so a chitchat reply that surfaced a fenced code block /
	// mermaid diagram / markdown table printed as literal source
	// text. recordTurn persists the raw `response` (NOT the
	// styled output) so memory transcripts stay plain.
	r.renderBordered(r.renderRichResponse(response))
	if result := r.writeLocalMarkdownTranscript(line, response); result.MarkdownPath != "" {
		r.emitMarkdownTranscriptHints(result)
	}
	r.lastAnswerOrigin = replAnswerOriginLocal
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
func (r *REPL) runStreamingChitchat(ctx context.Context, streamer streamingChitchatResponder, userLine, prior string) (string, error) {
	area, err := pterm.DefaultArea.WithRemoveWhenDone(true).Start()
	if err != nil {
		// Falling back to the sync path keeps chit-chat usable when the
		// terminal refuses to host a live area (uncommon; pterm starts
		// succeed on every TTY we've seen).
		logging.Warning("[repl/chitchat] pterm area start failed, falling back to sync: %v", err)
		return r.chitchatResponder.Respond(ctx, userLine, prior)
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

	response, err := streamer.RespondStream(ctx, userLine, prior, onDelta)

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
//
// priorTurnHint is a compact 1-line summary of the previous turn the
// REPL provides for multi-turn disambiguation. Empty string means "no
// prior turn / first turn / hint unavailable" — classifier MUST
// behave byte-identically to the no-hint case in that scenario.
// Format (REPL constructs, classifier consumes as opaque text):
//
//	kind=<chitchat|pipeline|plan|shell> topic=<oneLine, ≤100 chars>
//
// The hint exists to disambiguate continuation references (e.g. user
// types "expand 10" after a list_memory listing) that the classifier
// cannot route correctly from the current line alone.
type ChitchatClassifier interface {
	Classify(ctx context.Context, userLine, priorTurnHint string) (isChitchat bool, err error)
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
- meta-references to the conversation itself rather than the codebase:
  "记忆里有什么 / 之前我们聊过什么 / 历史里有 / 我之前问过什么"
  "what's in memory / what did we discuss before / show me my history /
  what have I asked you previously" — the chitchat handler has direct
  access to recall_memory and is the right route for these.

Examples of repo_question (not exhaustive):
- "how does X work?", "X 在哪里被调用?"
- "why did this panic happen?", "这个函数返回什么?"
- any question that names a file, function, symbol, config key, or behaviour
- any request to read, find, trace, compare, or explain CODE in the
  repository (not the conversation history)

When a turn looks ambiguous (e.g. a continuation like "那它呢" without
prior context), emit repo_question — the pipeline handles it safely
and the cost of being wrong is higher for chitchat than for pipeline.

priorTurn (when shown ABOVE the current message in the user content)
hints what the previous turn produced. The shape is one line:
  kind=<chitchat|pipeline|plan|shell> topic=<one-line summary>
Use priorTurn ONLY to disambiguate continuation references in the
current message. Concrete rules:

- If priorTurn.kind=chitchat AND priorTurn.topic suggests a numbered
  listing, inventory, or menu the user was asked to pick from, AND
  the current message is a SHORT continuation reference (drill-in
  on a numbered item, pick-N style, pronoun-style "那个" / "that one"
  / "show me details"), route to chitchat — the chitchat handler
  can call recall_memory to surface the picked entry's details.
  This is the multi-turn refinement chain: list → user picks → recall.

- Do NOT route to chitchat just because priorTurn.kind=chitchat. A
  real code question after a greeting ("foo() is defined where?"
  after "你好") is still a code question.

- If priorTurn.kind=pipeline / plan / shell, the previous turn was
  not chitchat — apply normal classification by current message
  content. Pipeline-to-pipeline follow-ups stay in the pipeline.

- When priorTurn is absent (no '## priorTurn:' section), ignore this
  block entirely and classify by current message content.

- The hint may carry an attachment=true token when the user has
  a sticky runtime log (set via /log <path>) or perf trace (set
  via /htrace) attached to the session. Routing rules under
  attachment:
    * If the current message references the attachment content
      (panic, exception, stack frame, perf metric, error message,
      or any code-side analysis of the attached payload), route
      to repo_question — the pipeline path consumes the attachment
      via log_triage / perf_triage; chitchat does not.
    * If the current message is unrelated to the attachment
      (memory-meta, greeting, capability question, casual chat),
      route to chitchat normally — the chitchat handler ignores
      the attachment field, so the sticky payload survives for
      the next pipeline turn.
    * Treat attachment=true as a SOFT signal: weighted toward
      repo_question only when the message also mentions the
      attached content. Never as a hard route — a clear chitchat
      signal in the message itself (e.g. "记忆里都有什么")
      always wins over the attachment hint.`

// llmChitchatClassifier is the default ChitchatClassifier. One
// adapter.Chat call per turn, with tool_choice=required and the local
// schema above.
type llmChitchatClassifier struct {
	adapter   llm.Adapter
	lastTrace replLLMCallTrace
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
//
// priorTurnHint, when non-empty, is rendered as a `## priorTurn:`
// section above the current message so the LLM can disambiguate
// continuation references. Empty hint preserves byte-identical
// behaviour with the no-hint shape (locked by test).
func (c *llmChitchatClassifier) Classify(ctx context.Context, userLine, priorTurnHint string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.adapter == nil {
		return false, fmt.Errorf("chitchat classifier not configured: no LLM adapter")
	}
	userLine = strings.TrimSpace(userLine)
	if userLine == "" {
		return false, fmt.Errorf("chitchat classifier: empty user line")
	}
	userContent := userLine
	if hint := strings.TrimSpace(priorTurnHint); hint != "" {
		userContent = "## priorTurn: " + hint + "\n\n## current: " + userLine
	}
	messages := []llm.Message{
		{Role: "system", Content: chitchatClassifierSystemPrompt},
		{Role: "user", Content: userContent},
	}
	tools := []llm.ToolSchema{chitchatClassifierTool}
	resp, err := c.adapter.Chat(ctx, messages, tools, llm.ChatOptions{ToolChoice: "required"})
	c.lastTrace = traceFromLLMResponse("chitchat_classifier", resp)
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

func (c *llmChitchatClassifier) LastReplLLMTrace() replLLMCallTrace {
	if c == nil {
		return replLLMCallTrace{}
	}
	return c.lastTrace
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
