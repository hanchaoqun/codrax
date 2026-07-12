package hitraceconv

import "github.com/hanchaoqun/codrax/internal/tracequery"

// directEROFSNameCandidate is a thin converter boundary over tracequery's
// source-neutral coverage-only gate.  A future source-pinned decoder may run
// before this fallback, but unverified descriptors must never reach either the
// compatibility renderer or generic field rendering.
func directEROFSNameCandidate(name string) bool {
	return tracequery.EROFSCoverageOnlyNameCandidate(name)
}
