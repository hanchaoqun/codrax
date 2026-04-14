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
}
