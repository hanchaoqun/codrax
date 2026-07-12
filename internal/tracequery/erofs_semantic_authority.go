package tracequery

import "strings"

// EROFSCoverageOnlyNameCandidate is the single source-neutral authority for
// EROFS names that currently have no pinned binary descriptor profile.
//
// The legacy converter vocabulary and the upstream EROFS tracepoint families
// are deliberately kept as raw/searchable inventory until a producer-matched
// events_format plus binary-page witness proves their field layout.  This
// predicate only inspects the event name: payload text such as a
// sched_blocked_reason caller containing "z_erofs" must remain unrelated.
func EROFSCoverageOnlyNameCandidate(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "erofs_") || strings.HasPrefix(name, "z_erofs_")
}
