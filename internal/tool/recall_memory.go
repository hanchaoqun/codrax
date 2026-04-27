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
func renderRecallResults(query string, entries []types.MemoryIndexEntry) string {
	var b strings.Builder
	if len(entries) == 0 {
		fmt.Fprintf(&b, "[recall_memory] query=%q matched 0 entries.\n", query)
		fmt.Fprintln(&b, "Nothing in prior conversation memory mentions this topic. Either widen the query (synonyms / parent concept) or fall back to repo_map / read_file for code-side answers.")
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
	return b.String()
}

func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
