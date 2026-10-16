package types

// RuntimeThreadLookup locates records through a user-named thread without
// electing that thread as a causal/diagnostic answer subject. It intentionally
// cannot be consumed as RuntimeTargets or as an exploration cursor.
type RuntimeThreadLookup struct {
	PID         int    `json:"pid,omitempty"`
	Thread      string `json:"thread,omitempty"`
	SourceQuote string `json:"source_quote"`
}

const RuntimeThreadLookupTeaching = "Runtime-artifact identities: distinguish an answer subject from a lookup input. A named process/thread whose own finite observed facts or causal behavior the answer must describe belongs in runtime_targets with named_target; being a fact-only question does not make that subject a lookup, and declaring it does not prove causality. A named thread used only to locate another answer subject, such as its containing process, belongs in runtime_thread_lookups with pid (TID) and/or exact thread plus a verbatim current-request source_quote containing that identity. Multiple submitted identities may share one contiguous quote that names them all. A lookup-only input does not require named_target; it is not discarded and does not become a causal focus. Generic all-thread phrases cannot authorize discovered identities; do not invent enclosing process IDs, derive identities from exploration cursors, or add lookups when no thread is named."
