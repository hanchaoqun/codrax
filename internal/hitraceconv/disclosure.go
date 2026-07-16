package hitraceconv

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const TraceCoverageLane = "trace_coverage"

// QueryReadyPerfTracePath consumes only the receipt-derived capability on a
// concrete perftrace Artifact. UI callers must never infer readiness from the
// Artifact type or filename alone.
func QueryReadyPerfTracePath(artifacts []Artifact) string {
	for _, artifact := range artifacts {
		if artifact.Type == ArtifactPerfTrace && artifact.Perf != nil && artifact.Perf.TraceQueryReady &&
			strings.TrimSpace(artifact.Path) != "" && artifact.Path == strings.TrimSpace(artifact.Path) {
			return artifact.Path
		}
	}
	return ""
}

func HasQueryReadyPerfTrace(artifacts []Artifact) bool {
	return QueryReadyPerfTracePath(artifacts) != ""
}

func IsPerfReceiptCoverage(coverage TraceDBCoverage) bool {
	return tracebundle.IsPerfReceiptCoverage(
		coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath,
	)
}

func IsTraceCoverageRowSorter(coverage TraceDBCoverage) bool {
	return coverage.Table == "__systrace_rows__" && coverage.Role == "systrace_text_output" &&
		(coverage.Family == "sorter" || coverage.Family == "builtin_modern_profiler")
}

// CoverageDisclosureIndexes preserves the source-order prefix and one bounded
// extra seat per exact priority class. CLI and REPL share this selector so a
// perf receipt or sorter resource proof cannot disappear in one surface only.
func CoverageDisclosureIndexes(lane string, coverage []TraceDBCoverage, prefixLimit int) []int {
	if prefixLimit < 0 {
		prefixLimit = 0
	}
	limit := len(coverage)
	if limit > prefixLimit {
		limit = prefixLimit
	}
	indexes := make([]int, 0, limit+2)
	seen := make(map[string]bool, 2)
	class := func(item TraceDBCoverage) string {
		if IsTraceCoverageRowSorter(item) {
			return "systrace_row_sorter"
		}
		if lane == TraceCoverageLane && IsPerfReceiptCoverage(item) {
			return "perf_receipt"
		}
		return ""
	}
	for index := 0; index < limit; index++ {
		indexes = append(indexes, index)
		if priority := class(coverage[index]); priority != "" {
			seen[priority] = true
		}
	}
	for index := limit; index < len(coverage); index++ {
		priority := class(coverage[index])
		if priority == "" || seen[priority] {
			continue
		}
		seen[priority] = true
		indexes = append(indexes, index)
	}
	return indexes
}
