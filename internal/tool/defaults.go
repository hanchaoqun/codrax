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

	// B0 write-mode tools. emit_change_plan is a real Day-5 tool
	// that writes Mutable.ChangePlan; apply_patch / run_tests /
	// emit_test_results ship as stubs with clear "not yet
	// implemented" responses (real bodies land in B2/B3). Register
	// them all here so their schemas are visible to the planner /
	// coder / verifier skills that list them in ToolSuggestions.
	r.Register(&EmitChangePlan{})
	r.Register(&ApplyPatch{})
	r.Register(&RunTests{})
	r.Register(&EmitTestResults{})
}
