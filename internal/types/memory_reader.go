package types

import "time"

// MemoryReader is the lightweight read/write surface the orchestrator
// hands tools so they can query (and later: pin / forget) the REPL
// memory store. Defined in the types package so internal/tool and
// internal/memory both consume it without an import cycle —
// internal/memory implements it on *Store and threads the value
// through internal/repl into BusContext.Memory.
//
// MVP shape: only Search is implemented today. Pin / Forget are
// declared so future commits can land them with no API churn —
// callers / tests probing capability can interface-assert.
type MemoryReader interface {
	// Search runs the configured per-Kind retrieval policy against the
	// query and returns the matching IndexEntry slice (deleted entries
	// stripped). Results are pre-ordered by relevance score with the
	// expand-refs chain already applied. Limit caps the slice; 0 means
	// "use the policy default".
	Search(query string, opts MemorySearchOpts) []MemoryIndexEntry
}

// MemorySearchOpts parameterises Search. All fields optional; zero
// values fall through to the per-Kind default.
type MemorySearchOpts struct {
	// Kind narrows the scan to one Kind. Empty string = any Kind.
	Kind string

	// SessionID lets the caller boost same-session matches when the
	// configured policy has SessionTieBreakerBonus > 0. Empty disables
	// the boost (the typical tool call from a non-REPL context).
	SessionID string

	// Limit caps the returned slice. 0 → caller-default (typically 5);
	// >0 → exact cap up to the implementation's hard ceiling (20 today).
	Limit int

	// IncludeBody asks the implementation to populate Body on entries
	// whose full turn text is still in the recent buffer (not yet
	// compacted). Default false to save tokens; the LLM asks for true
	// only when it needs verbatim text.
	IncludeBody bool
}

// MemoryLister is the optional capability surface a MemoryReader may
// implement when it can enumerate entries by time rather than score
// them by keyword. Tools that need a LISTING (rather than a topic
// search) type-assert this interface and degrade gracefully when
// the underlying reader does not support it. Pattern mirrors the
// chitchat package's `toolableChitchatResponder` opt-in:
// extending behaviour without breaking existing implementers /
// test fixtures that only conform to MemoryReader.Search.
//
// Why this is separate from Search:
//   - Search ranks by keyword/entity overlap with a query. Generic
//     queries like "memory recall history content" only surface
//     self-referential entries (those whose Topic contains the
//     query tokens), not the full conversation history.
//   - List returns the most recent N entries by timestamp without
//     scoring. Used when the user asks for a LISTING ("what's in
//     memory / 都有哪些 / list all") rather than a TOPIC ("we
//     discussed X").
//
// Production implementer: *internal/memory.Adapter. Test stubs
// that only need Search keep working — type-assertion fails and
// the caller surfaces a typed "list_memory unavailable" reply.
type MemoryLister interface {
	List(opts MemoryListOpts) []MemoryIndexEntry
}

// MemoryListOpts parameterises List. All fields optional; zero
// values fall through to "all entries, most-recent first, no time
// cutoff, default limit".
type MemoryListOpts struct {
	// Kind narrows to one Kind. Empty = any.
	Kind string

	// Limit caps the returned slice. 0 → caller-default (10);
	// >0 → exact cap up to the implementation's hard ceiling
	// (30, larger than Search's 20 because listing is browse-mode
	// and benefits from more context).
	Limit int

	// Since is an optional time floor — only entries newer than
	// Since are returned. Zero time means "no floor". Surfaces in
	// the wire as an RFC3339 string.
	Since time.Time
}

// MemoryIndexEntry mirrors internal/memory.IndexEntry's exported
// shape so tools can render results without importing the memory
// package directly. Field set is intentionally a superset of the
// runtime struct so future additions (Pinned / Deleted) drop in
// without breaking the wire.
type MemoryIndexEntry struct {
	ID        string
	Topic     string
	Summary   string
	Keywords  []string
	Entities  []string
	Refs      []string
	Kind      string
	SessionID string
	FullRef   string

	// Body is populated only when IncludeBody=true AND the turn is
	// still in the recent buffer. Empty otherwise — the caller has
	// the FullRef path to load on demand.
	Body string

	// Timestamp is the turn's emission time. Helpful for the
	// rendered output ("2 days ago" / "in this session") so the
	// LLM can prioritise recency without a separate scoring pass.
	Timestamp time.Time
}
