package hitraceconv

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func TestQueryReadyPerfTracePathRequiresTypedReadyCapability(t *testing.T) {
	ready := &PerfArtifactCapability{TraceQueryReady: true}
	for _, test := range []struct {
		name      string
		artifacts []Artifact
		want      string
	}{
		{name: "type only", artifacts: []Artifact{{Type: ArtifactPerfTrace, Path: "type-only.perftrace"}}},
		{name: "not ready", artifacts: []Artifact{{Type: ArtifactPerfTrace, Path: "closed.perftrace", Perf: &PerfArtifactCapability{}}}},
		{name: "wrong type", artifacts: []Artifact{{Type: ArtifactPerfData, Path: "capture.perftrace", Perf: ready}}},
		{name: "empty path", artifacts: []Artifact{{Type: ArtifactPerfTrace, Perf: ready}}},
		{name: "ready", artifacts: []Artifact{{Type: ArtifactPerfTrace, Path: "capture.perftrace", Perf: ready}}, want: "capture.perftrace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := QueryReadyPerfTracePath(test.artifacts); got != test.want || HasQueryReadyPerfTrace(test.artifacts) != (test.want != "") {
				t.Fatalf("ready perf disclosure got=%q bool=%t want=%q", got, HasQueryReadyPerfTrace(test.artifacts), test.want)
			}
		})
	}
}

func TestCoverageDisclosureIndexesReserveExactSorterAndPerfReceipt(t *testing.T) {
	coverage := make([]TraceDBCoverage, 0, 11)
	for index := 0; index < 5; index++ {
		coverage = append(coverage, TraceDBCoverage{Family: "regular", Table: "table"})
	}
	coverage = append(coverage,
		TraceDBCoverage{Family: tracebundle.PerfReceiptFamily, Table: "perftrace_future", Role: tracebundle.PerfReceiptRole, ArtifactPath: "future.perftrace"},
		TraceDBCoverage{Family: "sorter_v2", Table: "__systrace_rows__", Role: "systrace_text_output"},
		TraceDBCoverage{Family: tracebundle.PerfReceiptFamily, Table: tracebundle.PerfReceiptTableRawPerf, Role: tracebundle.PerfReceiptRole + "_v2", ArtifactPath: "fuzzy.perftrace"},
		TraceDBCoverage{Family: "sorter", Table: "__systrace_rows__", Role: "systrace_text_output"},
		TraceDBCoverage{Family: tracebundle.PerfReceiptFamily, Table: tracebundle.PerfReceiptTableRawPerf, Role: tracebundle.PerfReceiptRole, ArtifactPath: "capture.perftrace"},
		TraceDBCoverage{Family: tracebundle.PerfReceiptFamily, Table: tracebundle.PerfReceiptTableSimpleperfText, Role: tracebundle.PerfReceiptRole, ArtifactPath: "second.perftrace"},
	)
	if got, want := CoverageDisclosureIndexes(TraceCoverageLane, coverage, 5), []int{0, 1, 2, 3, 4, 8, 9}; !reflect.DeepEqual(got, want) {
		t.Fatalf("priority disclosure indexes=%v want=%v", got, want)
	}
	if got, want := CoverageDisclosureIndexes("trace_db_coverage", coverage, 5), []int{0, 1, 2, 3, 4, 8}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DB lane granted a perf receipt seat: got=%v want=%v", got, want)
	}
}
