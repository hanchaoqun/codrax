package orchestrator

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only the producer's closed selection verdict is recoverable before explore.
// Namespace candidates are not readable facts. A joined close, cancellation,
// source-generation or other failure must not disappear behind errors.As.
func namedTraceArchiveSelectionPending(err error) bool {
	for depth := 0; err != nil && depth < 32; depth++ {
		if archive, ok := err.(*hitraceconv.TraceArchiveError); ok {
			return archive != nil && archive.Code == "multiple_trace_members"
		}
		switch cause := err.(type) {
		case interface{ Unwrap() []error }:
			var single error
			for _, child := range cause.Unwrap() {
				if child == nil {
					continue
				}
				if single != nil {
					return false
				}
				single = child
			}
			err = single
		case interface{ Unwrap() error }:
			err = cause.Unwrap()
		default:
			return false
		}
	}
	return false
}

func markNamedTraceArchiveSelectionPending(profile *types.RuntimeArtifactPreflightProfile, path, repo string) {
	key := typedNamedTraceAdmissionPathKey(path)
	for i := range profile.Artifacts {
		item := &profile.Artifacts[i]
		if item.Carrier == "request_path" && item.RuntimeArtifactKind() == "trace" &&
			typedNamedTraceAdmissionPathKey(resolveTypedNamedTraceSource(item.Source, repo)) == key {
			// Preserve the producer's bounded candidate data across the pending
			// selection outcome. JSON validity only chooses a display format;
			// it cannot grant a tool, selected material or evidence authority.
			if json.Valid([]byte(item.Detail)) {
				continue
			}
			item.Detail = "ZIP has multiple trace candidates; select the intended exact member with trace_query archive_member. Preparation remains pending; no query facts or complete input have been admitted."
		}
	}
}
