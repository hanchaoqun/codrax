package tracebundle

import "strings"

const (
	SystraceReceiptFamily        = "trace_cross_validation"
	SystraceReceiptRole          = "tracequery_cross_validation"
	SystraceReceiptTableSQL      = "tracequery_build_index"
	SystraceReceiptTableBuiltin  = "builtin_systrace"
	SystraceReceiptTableProfiler = "profiler_systrace"
)

// IsClosedSystraceReceiptTable is the single wire-level closed set for
// converter-owned systrace validation receipts. Fuzzy/future names and the
// trace_db_coverage lane never inherit these semantics.
func IsClosedSystraceReceiptTable(table string) bool {
	switch table {
	case SystraceReceiptTableSQL,
		SystraceReceiptTableBuiltin,
		SystraceReceiptTableProfiler:
		return true
	default:
		return false
	}
}

// IsSystraceReceiptCoverage recognizes an exact receipt disclosure. The path
// is public in an in-memory Result and bundle-relative on the wire, but must
// always be non-empty and canonical with respect to surrounding whitespace.
func IsSystraceReceiptCoverage(family, table, role, artifactPath string) bool {
	return family == SystraceReceiptFamily && role == SystraceReceiptRole &&
		IsClosedSystraceReceiptTable(table) && artifactPath != "" &&
		artifactPath == strings.TrimSpace(artifactPath)
}
