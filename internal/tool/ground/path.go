package ground

import "github.com/hanchaoqun/codrax/internal/canonpath"

// CanonicalRepoRelative is retained as a thin shim for backward
// compatibility with the many call sites in tool/* and agent/*
// that grew up with the session-8 signature. The canonicaliser
// itself moved to internal/canonpath in session 22 so the types
// package (one layer below tool/*) can use the same implementation
// without creating an import cycle.
//
// New call sites — especially inside internal/types — should
// import canonpath directly; this shim stays only to minimise
// churn across ~8 historical callers.
func CanonicalRepoRelative(pathArg, repoRoot string) string {
	return canonpath.CanonicalRepoRelative(pathArg, repoRoot)
}
