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

func TestSystraceDisclosureRequiresTypedExactPrimary(t *testing.T) {
	ready := &TraceArtifactCapability{TraceQueryReady: true}
	inventory := &TraceArtifactCapability{TraceQueryReady: false}
	secondaryReady := Artifact{Type: ArtifactSystrace, Path: "secondary.systrace", Trace: ready}
	for _, test := range []struct {
		name          string
		result        Result
		wantInventory string
		wantReady     string
	}{
		{name: "output only", result: Result{OutputPath: "primary.systrace"}},
		{name: "type only", result: Result{OutputPath: "primary.systrace", Artifacts: []Artifact{{Type: ArtifactSystrace, Path: "primary.systrace"}}}},
		{name: "padded output", result: Result{OutputPath: " primary.systrace ", Artifacts: []Artifact{{Type: ArtifactSystrace, Path: " primary.systrace ", Trace: ready}}}},
		{name: "wrong type", result: Result{OutputPath: "primary.systrace", Artifacts: []Artifact{{Type: ArtifactPerfTrace, Path: "primary.systrace", Trace: ready}}}},
		{name: "secondary only", result: Result{OutputPath: "primary.systrace", Artifacts: []Artifact{secondaryReady}}},
		{
			name: "inventory primary with ready secondary",
			result: Result{OutputPath: "primary.systrace", Artifacts: []Artifact{
				{Type: ArtifactSystrace, Path: "primary.systrace", Trace: inventory},
				secondaryReady,
			}},
			wantInventory: "primary.systrace",
		},
		{
			name: "ready primary with inventory secondary",
			result: Result{OutputPath: "primary.systrace", Artifacts: []Artifact{
				{Type: ArtifactSystrace, Path: "primary.systrace", Trace: ready},
				{Type: ArtifactSystrace, Path: "secondary.systrace", Trace: inventory},
			}},
			wantInventory: "primary.systrace",
			wantReady:     "primary.systrace",
		},
		{
			name: "duplicate primary",
			result: Result{OutputPath: "primary.systrace", Artifacts: []Artifact{
				{Type: ArtifactSystrace, Path: "primary.systrace", Trace: ready},
				{Type: ArtifactSystrace, Path: "primary.systrace", Trace: ready},
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := SystraceInventoryPath(test.result); got != test.wantInventory {
				t.Fatalf("inventory path=%q want=%q", got, test.wantInventory)
			}
			if got := QueryReadySystracePath(test.result); got != test.wantReady {
				t.Fatalf("query-ready path=%q want=%q", got, test.wantReady)
			}
		})
	}
}

func TestCoverageDisclosureIndexesReserveExactSorterAndClosedReceipts(t *testing.T) {
	coverage := make([]TraceDBCoverage, 0, 13)
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
		TraceDBCoverage{Family: tracebundle.SystraceReceiptFamily, Table: "tracequery_future", Role: tracebundle.SystraceReceiptRole, ArtifactPath: "future.systrace"},
		TraceDBCoverage{Family: tracebundle.SystraceReceiptFamily, Table: tracebundle.SystraceReceiptTableBuiltin, Role: tracebundle.SystraceReceiptRole, ArtifactPath: "capture.systrace"},
	)
	if got, want := CoverageDisclosureIndexes(TraceCoverageLane, coverage, 5), []int{0, 1, 2, 3, 4, 8, 9, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("priority disclosure indexes=%v want=%v", got, want)
	}
	if got, want := CoverageDisclosureIndexes("trace_db_coverage", coverage, 5), []int{0, 1, 2, 3, 4, 8}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DB lane granted a perf receipt seat: got=%v want=%v", got, want)
	}
}

func TestCoverageDisclosureIndexesReserveSemanticQualitySeat(t *testing.T) {
	coverage := []TraceDBCoverage{
		{Family: "resolver", Table: "trace_range"},
		{Family: "resolver", Table: "process"},
		{Family: "resolver", Table: "thread"},
		{Family: "scheduler", Table: "sched_slice"},
		{Family: "scheduler", Table: "instant"},
		{Family: "slice", Table: "callstack"},
		{Family: traceDBSemanticQualityFamily, Table: traceDBSemanticQualityTable},
	}
	if got, want := CoverageDisclosureIndexes("trace_db_coverage", coverage, 5), []int{0, 1, 2, 3, 4, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("semantic quality disclosure indexes=%v want %v", got, want)
	}
}
