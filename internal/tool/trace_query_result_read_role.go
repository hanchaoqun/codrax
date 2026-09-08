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
	return "query_result_read_advisory=this file is a published query result, not the original trace capture. Search this result with grep or page it with read_file; quoted events and result rows do not turn the result file into a raw capture or current repository source.\n"
}
