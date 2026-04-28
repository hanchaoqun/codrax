package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RecallMemory is the agent-facing surface for querying the REPL's
// prior-conversation memory. The agent (analyzer / explorer) calls
// this when the user references their own history ("之前 / 上次 /
// 记忆里 / 历史 / 我们讨论过") or when the current request involves
// a topic the injected Prior-conversation block clearly does not
// cover.
//
// The implementation is a thin shim over BusContext.Memory.Search;
// scoring and ranking live in internal/memory.scoreIndex (reused
// verbatim — no second algorithm to maintain).
// RecallMemory embeds NonEvidenceTool because its output is the
// REPL conversation index — not a live read of the source files
// under investigation, so the grounder should not cite recalled
// symbols as repo-level evidence. ReadOnly because no filesystem
// mutation. Confidence=0.0 keeps the recall results out of the
// citation gate's per-shape thresholds.
type RecallMemory struct {
	ReadOnly
	NonEvidenceTool
}

func (t *RecallMemory) Name() string { return "recall_memory" }

func (t *RecallMemory) Description() string {
	return "Search the user's prior conversation memory. " +
		"Use when the user references their own history (`之前 / " +
		"上次 / 记忆里 / 历史 / 我们讨论过`, `previously / earlier / " +
		"last time / in memory / we discussed`), OR when the current " +
		"request involves a topic the injected Prior-conversation " +
		"block does not cover. Returns a list of compacted memory " +
		"entries (Topic + LLM-generated Summary + Keywords + Kind + " +
		"timestamp). Pass include_body=true to also fetch the full " +
		"prior response text for entries still in the recent buffer; " +
		"defaults to false to save context tokens."
}

type recallMemoryParams struct {
	Query       string `json:"query"`
	Kind        string `json:"kind,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	IncludeBody bool   `json:"include_body,omitempty"`
}

func (t *RecallMemory) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "query":        {"type": "string",  "description": "Natural-language search term. Entities + keywords work; the helper does its own tokenisation. Empty query returns no matches."},
    "kind":         {"type": "string",  "description": "Filter by turn kind: chitchat | pipeline | plan | shell | empty for any (default any).", "enum": ["", "chitchat", "pipeline", "plan", "shell"]},
    "session_id":   {"type": "string",  "description": "Optional conversation session id. When set, same-session entries get the configured tie-breaker bonus."},
    "limit":        {"type": "integer", "description": "Max entries to return. Default 5, hard cap 20."},
    "include_body": {"type": "boolean", "description": "When true AND an entry is still in the recent buffer, the full Turn.Response is included in the result. Default false to save tokens."}
  },
  "required": ["query"]
}`)
}

func (t *RecallMemory) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p recallMemoryParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}
	if strings.TrimSpace(p.Query) == "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "recall_memory rejected: empty `query`. Pass at least one keyword / entity describing what you want to recall.",
			Timestamp: time.Now(),
		}, nil
	}
	if ctx == nil || ctx.Memory == nil {
		// Single-shot CLI / non-interactive test — no Store wired.
		// Surface a typed message rather than panicking, so the LLM
		// can fall through to the standard repo_map / read_file path.
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "recall_memory unavailable: prior-conversation memory is not available in this run (typically a single-shot non-interactive invocation). Fall back to repo_map / grep for code-side searches.",
			Timestamp: time.Now(),
		}, nil
	}

	entries := ctx.Memory.Search(p.Query, types.MemorySearchOpts{
		Kind:        p.Kind,
		SessionID:   p.SessionID,
		Limit:       p.Limit,
		IncludeBody: p.IncludeBody,
	})

	summary := renderRecallResults(p.Query, entries)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   summary,
		Timestamp: time.Now(),
	}, nil
}

// renderRecallResults shapes the entries into a compact text block
// the LLM consumes inline. Each entry is one paragraph: Topic line +
// Summary + Keywords / Kind / SessionID / FullRef + (optional) Body.
// Empty result set returns an explicit "no matches" so the LLM
// knows to widen the query or move on.
//
// Self-referential hint: when ≤2 entries match AND every match's
// Topic looks like a meta-reference to memory itself ("记忆 / 历史 /
// memory / recall / list / 都有哪些"), the renderer appends a
// "consider list_memory" suggestion. The pattern surfaced in a
// 2026-04-28 user trace where the LLM picked a generic listing-
// style query against recall_memory and only matched self-
// referential entries — list_memory would have been the right tool.
// Educating via tool-result text avoids schema churn while training
// the LLM's next-turn decision.
func renderRecallResults(query string, entries []types.MemoryIndexEntry) string {
	var b strings.Builder
	if len(entries) == 0 {
		fmt.Fprintf(&b, "[recall_memory] query=%q matched 0 entries.\n", query)
		fmt.Fprintln(&b, "Nothing in prior conversation memory mentions this topic. Either widen the query (synonyms / parent concept), call list_memory(limit=20) for an inventory listing, or fall back to repo_map / read_file for code-side answers.")
		return b.String()
	}
	fmt.Fprintf(&b, "[recall_memory] query=%q matched %d entries (ranked by relevance):\n\n",
		query, len(entries))
	for i, e := range entries {
		fmt.Fprintf(&b, "#%d %s", i+1, e.ID)
		if e.Kind != "" {
			fmt.Fprintf(&b, " (kind=%s)", e.Kind)
		}
		if e.SessionID != "" {
			fmt.Fprintf(&b, " session=%s", trunc(e.SessionID, 12))
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
		if len(e.Entities) > 0 {
			fmt.Fprintf(&b, "   entities: %s\n", strings.Join(e.Entities, ", "))
		}
		if len(e.Refs) > 0 {
			fmt.Fprintf(&b, "   refs: %s\n", strings.Join(e.Refs, ", "))
		}
		if e.FullRef != "" {
			fmt.Fprintf(&b, "   full: %s\n", e.FullRef)
		}
		if e.Body != "" {
			// Cap inlined body so a recent shell-output turn doesn't
			// blow the LLM's tool-result budget. 600 char cap matches
			// the order of magnitude used elsewhere (citation quote
			// preview ceiling).
			body := e.Body
			if len(body) > 600 {
				body = body[:600] + " …[truncated]"
			}
			fmt.Fprintf(&b, "   body: %s\n", body)
		}
		b.WriteByte('\n')
	}
	// Self-referential / low-recall hint. Fires when ≤5 entries
	// returned AND every match's Topic resembles a meta-reference
	// to memory itself — the canonical sign of a generic listing-
	// intent query that should have used list_memory. Suggesting
	// list_memory in the tool-result trains the LLM's next-turn
	// choice without changing the schema.
	//
	// Threshold ≤5 (was ≤2 originally) was raised after the
	// 2026-04-28 user trace where 3 of 25 entries matched a
	// generic query ("memory recall history content") and all 3
	// were self-referential, but the prior ≤2 guard didn't fire.
	// 5 self-referential matches is a strong signal in any
	// realistic memory store — natural conversation rarely
	// generates that many meta-questions about memory.
	if len(entries) <= 5 && allTopicsLookSelfReferential(entries) {
		fmt.Fprintln(&b, "Note: only a few entries matched and they look self-referential (their Topics are about memory itself, not actual conversation content). If the user is asking for an INVENTORY of memory ('都有哪些 / what's in memory / list all'), call list_memory(limit=20) instead — that returns the most-recent N entries by time, regardless of topic.")
	}
	return b.String()
}

func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// allTopicsLookSelfReferential reports whether every entry's Topic
// reads like a meta-reference to memory itself (rather than actual
// conversation content). Used by renderRecallResults to detect the
// "LLM picked a generic listing-intent query against recall_memory"
// failure mode: the only matches will be the user's prior questions
// about memory itself, which makes recall look broken.
//
// Detection is bilingual (zh + en) and case-insensitive; minimum 3
// runes per Topic so very short Topics that happen to match a
// keyword don't false-fire. Requires ALL entries to look
// self-referential — a single non-meta entry means the LLM did
// find real content and the hint would be misleading.
func allTopicsLookSelfReferential(entries []types.MemoryIndexEntry) bool {
	if len(entries) == 0 {
		return false
	}
	keywords := []string{
		"记忆", "历史", "对话", "聊过", "讨论过",
		"都有哪些", "看一下", "找一下",
		"memory", "history", "recall", "list all",
		"prior conversation", "what did we", "what's in",
		"chat history",
	}
	for _, e := range entries {
		topic := strings.ToLower(strings.TrimSpace(e.Topic))
		if len([]rune(topic)) < 3 {
			return false
		}
		matched := false
		for _, kw := range keywords {
			if strings.Contains(topic, kw) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
