package tool

// RegisterDefaults instantiates and registers all built-in tools.
// Note: RepoMapV2 is registered from main.go to avoid circular imports
// (repomap package imports tool for ReadOnly/StoreBlob).
func RegisterDefaults(r *Registry) {
	r.Register(&ExecCommand{})
	r.Register(&GrepTool{})
	r.Register(&TraceQuery{})
	r.Register(&ReadFile{})
	r.Register(&ListFiles{})
	r.Register(&GitDiff{})
	r.Register(&GitShow{})
	r.Register(&GitLog{})
	r.Register(&GitHistorySearch{})
	r.Register(&EmitMultiRepoFocus{})
	r.Register(&EmitAnalysis{})

	// Write-mode tools. All real as of B2: emit_change_plan writes
	// Mutable.ChangePlan; apply_patch handles create/modify/delete
	// and pipes kind=patch through `git apply`; run_tests
	// auto-detects Go/Node/Python/Rust runners and parses their
	// output into a ChangeReport; emit_test_results optionally
	// decorates the report with a prose FailureSummary. Registered
	// here so their schemas surface to the planner / coder /
	// verifier skills via ToolSuggestions.
	r.Register(&EmitChangePlan{})
	r.Register(&EmitPlanSkeleton{})
	r.Register(&EmitPlanChange{})
	r.Register(&ApplyPatch{})
	r.Register(&RunTests{})
	r.Register(&EmitTestResults{})

	// recall_memory — agent-driven retrieval of REPL prior-conversation
	// memory. Read-only; nil-safe when BusContext.Memory is unwired
	// (single-shot CLI, tests). Available to read-mode agents
	// (analyzer / explorer); skill prose teaches them when to call.
	r.Register(&RecallMemory{})

	// list_memory — agent-driven enumeration of recent memory entries
	// by time. Use when the user asks for a LISTING ("都有哪些 / what's
	// in memory / list all") rather than a TOPIC search. Capability-
	// gated: requires the wired MemoryReader to also implement
	// MemoryLister (production *memory.Adapter does; test stubs
	// surface a typed unavailable reply).
	r.Register(&ListMemory{})
}
