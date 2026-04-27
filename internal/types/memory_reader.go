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
