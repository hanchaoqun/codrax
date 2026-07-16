package tracebundle

import "testing"

func TestPerfReceiptCoverageUsesExactClosedIdentity(t *testing.T) {
	for _, table := range []string{
		PerfReceiptTableSimpleperfText,
		PerfReceiptTableSimpleperfProto,
		PerfReceiptTableHiperfProto,
		PerfReceiptTableRawPerf,
	} {
		if !IsClosedPerfReceiptTable(table) || !IsPerfReceiptCoverage(PerfReceiptFamily, table, PerfReceiptRole, "capture.perftrace") {
			t.Fatalf("closed perf receipt identity was rejected: %s", table)
		}
	}
	for _, test := range []struct {
		name, family, table, role, path string
	}{
		{name: "future table", family: PerfReceiptFamily, table: "perftrace_future", role: PerfReceiptRole, path: "capture.perftrace"},
		{name: "fuzzy table", family: PerfReceiptFamily, table: PerfReceiptTableRawPerf + "_v2", role: PerfReceiptRole, path: "capture.perftrace"},
		{name: "fuzzy family", family: PerfReceiptFamily + "_v2", table: PerfReceiptTableRawPerf, role: PerfReceiptRole, path: "capture.perftrace"},
		{name: "fuzzy role", family: PerfReceiptFamily, table: PerfReceiptTableRawPerf, role: PerfReceiptRole + "_v2", path: "capture.perftrace"},
		{name: "empty path", family: PerfReceiptFamily, table: PerfReceiptTableRawPerf, role: PerfReceiptRole},
		{name: "padded path", family: PerfReceiptFamily, table: PerfReceiptTableRawPerf, role: PerfReceiptRole, path: " capture.perftrace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if IsPerfReceiptCoverage(test.family, test.table, test.role, test.path) {
				t.Fatal("near-match gained perf receipt identity")
			}
		})
	}
}
