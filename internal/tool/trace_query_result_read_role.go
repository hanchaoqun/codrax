package tool

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// traceQueryResultReadTarget shares the readers' existing published-ref
// resolution, including its compatibility aliases. It grants no permission:
// callers use it only to choose advice after the normal read path. When an
// actual path is supplied, the resolved publication must name that same file.
// Result contents (including quoted scheduler events) cannot override this role.
func traceQueryResultReadTarget(ctx *types.BusContext, requested, actualPath string) bool {
	resolved, ok := resolveTraceQueryBlobRefPath(ctx, requested)
	if !ok {
		return false
	}
	if actualPath == "" {
		return true // grep uses that same resolver before executing its read.
	}
	clean := func(p string) string {
		return filepath.Clean(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"))
	}
	return clean(resolved) == clean(actualPath)
}

func traceQueryResultReadAdvisory(ctx *types.BusContext, requested string) string {
	if !traceQueryResultReadTarget(ctx, requested, "") {
		return ""
	}
	return "query_result_read_advisory=" + types.TraceQueryResultReadRoleGuidance + "\n"
}

// ArtifactReadReturnNavigationAdvisory supplies a way back to the published
// query result even when an agent boundary refuses a derived-file read before
// the reader runs. Only original publications use their existing compatibility
// resolver; derived refs must match the actual resolved path exactly. This
// helper is not a permission, grounding, or artifact-classification predicate.
func ArtifactReadReturnNavigationAdvisory(ctx *types.BusContext, requested string) string {
	if ctx == nil || ctx.Mutable == nil || strings.TrimSpace(requested) == "" {
		return ""
	}
	actual, ok := resolveTraceQueryBlobRefPath(ctx, requested)
	if !ok {
		actual = resolveToolPath(ctx, requested)
	}
	ticket, ok := ctx.Mutable.PrepareArtifactReadNavigation(actual)
	if !ok {
		return ""
	}
	return types.ArtifactReadNavigationAdvisory(ticket)
}

func prepareArtifactReadNavigation(ctx *types.BusContext, actualInput string) types.ToolArtifactReadNavigation {
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolArtifactReadNavigation{}
	}
	ticket, _ := ctx.Mutable.PrepareArtifactReadNavigation(actualInput)
	return ticket
}

// finishArtifactReadNavigation runs after the reader's normal result is built.
// It leaves stored bytes, refinement, read coverage, observations and all
// authority markers alone. A ticket captured before I/O must still be current;
// task reset or an ambiguous ancestor while I/O ran cannot acquire a new epoch.
func finishArtifactReadNavigation(ctx *types.BusContext, ticket types.ToolArtifactReadNavigation, result *types.ToolResult) {
	if ctx == nil || ctx.Mutable == nil || result == nil || ticket.InputRef == "" {
		return
	}
	current, ok := ctx.Mutable.PrepareArtifactReadNavigation(ticket.InputRef)
	if !ok || current != ticket {
		return
	}
	advice := types.ArtifactReadNavigationAdvisory(current)
	if advice != "" {
		result.Summary += "\n" + advice
		if result.Repair != nil {
			result.Repair.Hint += "\n" + advice
		}
	}
	if result.Success && result.RawRef != "" {
		ticket.OutputRef = result.RawRef
		result.ArtifactReadNavigation = ticket
	}
}
