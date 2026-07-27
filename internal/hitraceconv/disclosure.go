package hitraceconv

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const TraceCoverageLane = "trace_coverage"

// SystraceInventoryPath returns the public spelling of the Result's primary
// converter-owned systrace only when that exact Artifact carries a typed trace
// capability. OutputPath existence and a ready secondary systrace are not
// sufficient: UI callers must preserve the primary-selection authority that
// was reconciled by the conversion finalizer.
func SystraceInventoryPath(result Result) string {
	return primarySystraceCapabilityPath(result, false)
}

// QueryReadySystracePath consumes only the receipt-derived capability on the
// exact primary systrace selected by Result.OutputPath. It deliberately does
// not search for a ready secondary artifact.
func QueryReadySystracePath(result Result) string {
	return primarySystraceCapabilityPath(result, true)
}

func primarySystraceCapabilityPath(result Result, requireReady bool) string {
	path := result.OutputPath
	if path == "" || path != strings.TrimSpace(path) {
		return ""
	}
	found := ""
	for _, artifact := range result.Artifacts {
		if artifact.Type != ArtifactSystrace || artifact.Path != path {
			continue
		}
		// Duplicate primary labels are malformed even if byte-identical. The
		// Q4a reconciler rejects them in production; keep disclosure fail-closed
		// for synthetic or future callers too.
		if found != "" || artifact.Trace == nil || requireReady && !artifact.Trace.TraceQueryReady {
			return ""
		}
		found = artifact.Path
	}
	return found
}

// QueryReadyPerfTracePath consumes only the receipt-derived capability on a
// concrete perftrace Artifact. UI callers must never infer readiness from the
// Artifact type or filename alone.
func QueryReadyPerfTracePath(artifacts []Artifact) string {
	rawDisclosures := make([]PerfCaptureDisclosure, len(artifacts))
	rawPathCounts := make(map[string]int)
	for index, artifact := range artifacts {
		disclosure := PerfCaptureDisclosureForArtifact(artifact)
		rawDisclosures[index] = disclosure
		if disclosure.Present && strings.TrimSpace(artifact.Path) != "" &&
			artifact.Path == strings.TrimSpace(artifact.Path) {
			rawPathCounts[artifact.Path]++
		}
	}
	for index, artifact := range artifacts {
		if artifact.Type != ArtifactPerfTrace || strings.TrimSpace(artifact.Path) == "" ||
			artifact.Path != strings.TrimSpace(artifact.Path) {
			continue
		}
		// Raw converter output has a stricter typed contract than official or
		// plugin-produced perftrace. Delegate that lane to the single raw
		// classifier so a forged readiness bit cannot make CLI/REPL claim
		// queryability immediately before the shared disclosure rejects the
		// same artifact. Duplicate raw paths are fail-closed as a set, matching
		// PerfCaptureDisclosures. Nonraw providers retain the old selector.
		disclosure := rawDisclosures[index]
		if disclosure.Present {
			if rawPathCounts[artifact.Path] == 1 && disclosure.Valid && disclosure.QueryReady {
				return artifact.Path
			}
			continue
		}
		if artifact.Perf != nil && artifact.Perf.TraceQueryReady {
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

func IsSystraceReceiptCoverage(coverage TraceDBCoverage) bool {
	return tracebundle.IsSystraceReceiptCoverage(
		coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath,
	)
}

func IsTraceCoverageRowSorter(coverage TraceDBCoverage) bool {
	return coverage.Table == "__systrace_rows__" && coverage.Role == "systrace_text_output" &&
		(coverage.Family == "sorter" || coverage.Family == "builtin_modern_profiler")
}

// CoverageDisclosureIndexes preserves the source-order prefix and one bounded
// extra seat per exact priority class. CLI and REPL share this selector so a
// closed perf/systrace receipt or sorter resource proof cannot disappear in
// one surface only.
func CoverageDisclosureIndexes(lane string, coverage []TraceDBCoverage, prefixLimit int) []int {
	if prefixLimit < 0 {
		prefixLimit = 0
	}
	limit := len(coverage)
	if limit > prefixLimit {
		limit = prefixLimit
	}
	indexes := make([]int, 0, limit+3)
	seen := make(map[string]bool, 3)
	class := func(item TraceDBCoverage) string {
		if lane == "trace_db_coverage" && item.Family == traceDBSemanticQualityFamily &&
			item.Table == traceDBSemanticQualityTable {
			return "semantic_quality"
		}
		if IsTraceCoverageRowSorter(item) {
			return "systrace_row_sorter"
		}
		if lane == TraceCoverageLane && IsPerfReceiptCoverage(item) {
			return "perf_receipt"
		}
		if lane == TraceCoverageLane && IsSystraceReceiptCoverage(item) {
			return "systrace_receipt"
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
