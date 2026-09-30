package types

// RuntimeThreadLookup locates records through a user-named thread without
// electing that thread as a causal/diagnostic answer subject. It intentionally
// cannot be consumed as RuntimeTargets or as an exploration cursor.
type RuntimeThreadLookup struct {
	PID         int    `json:"pid,omitempty"`
	Thread      string `json:"thread,omitempty"`
	SourceQuote string `json:"source_quote"`
}

const RuntimeThreadLookupTeaching = "A concrete thread can be a query locator without being the diagnostic subject: e.g. asking about the process containing thread 42 still needs thread 42 to locate that process. Preserve such identities in runtime_thread_lookups with pid and/or exact thread plus one verbatim current-request source_quote containing that name/TID; generic phrases requesting all threads cannot authorize artifact-discovered identities. Do not invent an enclosing process ID. This independent lane does not elect a root-cause focus or require runtime_target_profile=named_target. Keep runtime_targets for actual answer subjects; omit lookup entries when no thread is named, and never derive them from a model's exploration cursor."
