package context

import (
	stdcontext "context"
	"strings"
)

// Scoped extraction never writes a temporary attached_trace.txt or replaces
// the parent query receipt. Local gutters are rebased by emit_perf_trace.
func formatTraceExcerpt(parent string, state attachedRuntimeTriageState, opts attachedTraceRenderOptions) string {
	preamble := ""
	if opts.Material != nil {
		if err := opts.Material.Validate(stdcontext.Background(), parent); err != nil {
			return "The attached Trace materials changed or became unavailable. Reattach the original source before deriving new evidence.\n"
		}
		info := preparedTraceBundlePromptInfo(opts.Material.QueryPath())
		opts.SampleOnly = info.sampleOnly
		preamble = info.text
	}
	raw, scope, err := opts.Excerpt.Resolve(stdcontext.Background(), parent, opts.Material)
	if err != nil {
		return "The attached Trace or its extraction view changed or became unavailable. Reattach the original source before deriving new evidence.\n"
	}
	return preamble + attachedTracePreamble(state, opts) + scope.Description() +
		" Emit line_start/line_end using the fragment-local gutters below (starting at 1); the system maps them to the parent preview. Keep timestamps on the original trace axis. No result here establishes full-attachment coverage.\n```text\n" +
		renderAttachedArtifactLines(strings.TrimSuffix(raw, "\n"), 1) + "\n```"
}
