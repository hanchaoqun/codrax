package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ListMemory is the agent-facing surface for ENUMERATING REPL
// prior-conversation memory by recency rather than searching it by
// topic. Use cases:
//
//   - User asks for a LISTING of memory ("都有哪些 / what's in
//     memory / list all the history / 历史里有什么"). recall_memory
//     would only surface entries whose Topic / Keywords overlap with
//     the LLM's chosen query — typically self-referential meta-
//     questions about memory itself, not the actual content.
//   - User wants browse-mode access to the most recent N turns
//     regardless of topic.
//
// Capability gate: requires the underlying MemoryReader to also
// implement types.MemoryLister. The production *memory.Adapter does;
// test stubs and single-shot CLI runs (no Store wired) do not. The
// type-assertion failure surfaces as a typed "unavailable" reply
// matching the recall_memory unavailable shape so the LLM has a
// consistent fail-mode to react to.
//
// Confidence=0 (NonEvidenceTool): output is the conversation index,
// not a live read of the source files under investigation. ReadOnly:
// no filesystem mutation.
type ListMemory struct {
	ReadOnly
	NonEvidenceTool
}

func (t *ListMemory) Name() string { return "list_memory" }

func (t *ListMemory) Description() string {
	return "List recent prior-conversation memory entries by time, " +
		"regardless of topic. Use when the user asks for a LISTING " +
		"of memory content (`都有哪些 / 历史里有什么 / what's in " +
		"memory / list all`) — NOT for topic-specific recall (which " +
		"is recall_memory). Returns the latest N entries (default 10, " +
		"hard cap 30), most-recent first, with optional `kind` filter " +
		"and `since` time floor."
}

type listMemoryParams struct {
	Limit int    `json:"limit,omitempty"`
	Kind  string `json:"kind,omitempty"`
	// Since is an optional RFC3339 lower bound — entries older than
	// Since are dropped. Empty string means "no floor".
	Since string `json:"since,omitempty"`
}

func (t *ListMemory) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "limit": {"type": "integer", "description": "Max entries to return. Default 10, hard cap 30."},
    "kind":  {"type": "string",  "description": "Filter by turn kind: chitchat | pipeline | plan | shell | empty for any (default any).", "enum": ["", "chitchat", "pipeline", "plan", "shell"]},
    "since": {"type": "string",  "description": "Optional RFC3339 lower bound — entries older than this drop. Empty means no floor."}
  }
}`)
}

func (t *ListMemory) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	var p listMemoryParams
	if err := json.Unmarshal(params, &p); err != nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   fmt.Sprintf("invalid params: %v", err),
			Timestamp: time.Now(),
		}, err
	}
	if ctx == nil || ctx.Memory == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "list_memory unavailable: prior-conversation memory is not available in this run (typically a single-shot non-interactive invocation). Fall back to repo_map / grep for code-side searches.",
			Timestamp: time.Now(),
		}, nil
	}
	lister, ok := ctx.Memory.(types.MemoryLister)
	if !ok {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "list_memory unavailable: the wired MemoryReader does not implement MemoryLister (browse-mode capability). The recall_memory tool is still available for topic searches.",
			Timestamp: time.Now(),
		}, nil
	}
	opts := types.MemoryListOpts{
		Kind:  p.Kind,
		Limit: p.Limit,
	}
	if strings.TrimSpace(p.Since) != "" {
		ts, err := time.Parse(time.RFC3339, strings.TrimSpace(p.Since))
		if err != nil {
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("list_memory rejected: invalid `since` %q (must be RFC3339, e.g. 2026-04-28T00:00:00Z): %v", p.Since, err),
				Timestamp: time.Now(),
			}, nil
		}
		opts.Since = ts
	}
	entries := lister.List(opts)
	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderListResults(opts, entries),
		Timestamp: time.Now(),
	}, nil
}

// renderListResults shapes the entries into a compact text block —
// same per-entry shape as renderRecallResults so the two tools'
// outputs are interchangeable in the LLM's parsing patterns. The
// header banner says "[list_memory] N entries (recency-ordered)"
// to make the listing semantics unmissable.
func renderListResults(opts types.MemoryListOpts, entries []types.MemoryIndexEntry) string {
	var b strings.Builder
	if len(entries) == 0 {
		filter := ""
		if opts.Kind != "" {
			filter = fmt.Sprintf(" kind=%s", opts.Kind)
		}
		if !opts.Since.IsZero() {
			filter += fmt.Sprintf(" since=%s", opts.Since.Format(time.RFC3339))
		}
		fmt.Fprintf(&b, "[list_memory]%s 0 entries.\n", filter)
		fmt.Fprintln(&b, "No prior conversation memory in this REPL session yet. Tell the user the memory is genuinely empty.")
		return b.String()
	}
	header := fmt.Sprintf("[list_memory] %d entries (most-recent first)", len(entries))
	if opts.Kind != "" {
		header += fmt.Sprintf(", kind=%s", opts.Kind)
	}
	if !opts.Since.IsZero() {
		header += fmt.Sprintf(", since=%s", opts.Since.Format(time.RFC3339))
	}
	header += ":"
	fmt.Fprintln(&b, header)
	b.WriteByte('\n')
	for i, e := range entries {
		fmt.Fprintf(&b, "#%d %s", i+1, e.ID)
		if e.Kind != "" {
			fmt.Fprintf(&b, " (kind=%s)", e.Kind)
		}
		if e.SessionID != "" {
			fmt.Fprintf(&b, " session=%s", trunc(e.SessionID, 12))
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
		if len(e.Keywords) > 0 {
			fmt.Fprintf(&b, "   keywords: %s\n", strings.Join(e.Keywords, ", "))
		}
		if e.FullRef != "" {
			fmt.Fprintf(&b, "   full: %s\n", e.FullRef)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
