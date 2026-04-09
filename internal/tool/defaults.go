package tool

// RegisterDefaults instantiates and registers all built-in tools.
func RegisterDefaults(r *Registry) {
	r.Register(&ExecCommand{})
	r.Register(&GrepTool{})
	r.Register(&ReadFile{})
	r.Register(&ApplyPatch{})
	r.Register(&ListFiles{})
	r.Register(&RepoMap{})
	r.Register(&RunTests{})
	r.Register(&GitDiff{})
	r.Register(&GitLog{})
	r.Register(&TodoWrite{})
}
