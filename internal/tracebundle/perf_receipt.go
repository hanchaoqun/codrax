package tracebundle

import "strings"

const (
	PerfReceiptFamily               = "trace_cross_validation"
	PerfReceiptRole                 = "tracequery_cross_validation"
	PerfReceiptTableSimpleperfText  = "perftrace_simpleperf_text"
	PerfReceiptTableSimpleperfProto = "perftrace_simpleperf_proto"
	PerfReceiptTableHiperfProto     = "perftrace_hiperf_proto"
	PerfReceiptTableRawPerf         = "perftrace_raw_perf"
)

// IsClosedPerfReceiptTable is the single wire-level closed set for converter
// perf validation receipts. Consumers must not grant receipt semantics to a
// fuzzy prefix or a future table before its producer contract is implemented.
func IsClosedPerfReceiptTable(table string) bool {
	switch table {
	case PerfReceiptTableSimpleperfText,
		PerfReceiptTableSimpleperfProto,
		PerfReceiptTableHiperfProto,
		PerfReceiptTableRawPerf:
		return true
	default:
		return false
	}
}

// IsPerfReceiptCoverage identifies only the exact typed receipt disclosure.
// Artifact paths may be absolute in an in-memory conversion Result and
// bundle-relative on the wire, but they must be non-empty and unpadded.
func IsPerfReceiptCoverage(family, table, role, artifactPath string) bool {
	return family == PerfReceiptFamily && role == PerfReceiptRole &&
		IsClosedPerfReceiptTable(table) && artifactPath != "" &&
		artifactPath == strings.TrimSpace(artifactPath)
}
