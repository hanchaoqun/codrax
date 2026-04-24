package tool

// RegisterDefaults instantiates and registers all built-in tools.
// Note: RepoMapV2 is registered from main.go to avoid circular imports
// (repomap package imports tool for ReadOnly/StoreBlob).
func RegisterDefaults(r *Registry) {
	r.Register(&ExecCommand{})
	r.Register(&GrepTool{})
	r.Register(&ReadFile{})
	r.Register(&ListFiles{})
	r.Register(&GitDiff{})
	r.Register(&GitLog{})
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
	r.Register(&ApplyPatch{})
	r.Register(&RunTests{})
	r.Register(&EmitTestResults{})
}
