package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func exportCPUIntervalFixture(t *testing.T, inserts []string) (string, []tracewire.CPUMeasureInterval, traceDBSystraceExport) {
	t.Helper()
	statements := []string{
		"CREATE TABLE measure (id, ts, dur, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_idle', 0), (2, 'cpu_frequency', 0), (3, 'cpu_idle', 1), (4, 'cpu_frequency', 1)",
	}
	path := createTraceDBFixture(t, append(statements, inserts...))
	out := filepath.Join(t.TempDir(), "cpu-interval.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, out)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var records []tracewire.CPUMeasureInterval
	for _, line := range strings.Split(string(body), "\n") {
		if r, ok := tracewire.ParseCPUMeasureInterval(line); ok {
			records = append(records, r)
		}
	}
	for _, forbidden := range []string{": cpu_idle:", ": cpu_frequency:"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("explicit durations turned into next-update controls: %s", forbidden)
		}
	}
	return out, records, result
}

func TestExportTraceDBCPUIntervalsPreserveDurationIdentityAndEncoding(t *testing.T) {
	out, records, result := exportCPUIntervalFixture(t, []string{
		"INSERT INTO measure VALUES (99, 20000000, 20000000, 2000000.0, 2)",
		"INSERT INTO measure VALUES (99, 0, 40000000, 0.0, 1)",
		"INSERT INTO measure VALUES (99, -5000000, 15000000, 1000000.0, 2)",
		"INSERT INTO measure VALUES (99, 0, 40000000, 1500000.0, 4)",
		"INSERT INTO measure VALUES (99, 5000000, 15000000, 4294967295.0, 3)",
	})
	if len(records) != 5 {
		t.Fatalf("rows lost: %+v", records)
	}
	seen := map[int64]bool{}
	negative := false
	for _, r := range records {
		if seen[r.RowID] || r.RowID == 99 || r.Issue != "" || r.DurationNS == nil {
			t.Fatalf("unstable SQL identity/duration: %+v", r)
		}
		seen[r.RowID] = true
		if *r.StartNS < 0 {
			negative = true
			if *r.DurationNS != 15000000 {
				t.Fatal("carry-in duration rebased")
			}
		}
		if r.Kind == "idle" && r.Encoding != "native_sql_idle" {
			t.Fatal("native idle encoding disappeared")
		}
	}
	if !negative {
		t.Fatal("signed carry-in timestamp discarded")
	}
	coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	if coverage.RowsRead != 5 || coverage.RowsEmitted != 5 || coverage.FieldSources["cpu_interval"] == "" {
		t.Fatalf("interval coverage missing: %+v", coverage)
	}
	idx, err := tracequery.BuildIndex(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewCPUStateFrequency, TimeStart: 0, TimeStartSet: true, TimeEnd: .04}).CPUStateFrequency
	if !tracequery.ValidCPUStateFrequency(*p) || p.Status != "available" || p.CPUCount != 2 || p.KnownJointMs != 45 || p.UnknownJointMs != 35 {
		t.Fatalf("source-to-joint semantics: %+v", p)
	}
}

func TestExportTraceDBCPUIntervalsRetainUnknownAndInvalidRows(t *testing.T) {
	out, records, _ := exportCPUIntervalFixture(t, []string{
		"INSERT INTO measure VALUES (1, 0, 40000000, 0.0, 1)",
		"INSERT INTO measure VALUES (2, 0, 40000000, 1000000.0, 2)",
		"INSERT INTO measure VALUES (3, 10000000, NULL, 2000000.0, 2)",
		"INSERT INTO measure VALUES (4, 15000000, -1, 2.5, 2)",
		"INSERT INTO measure VALUES (5, 40000000, NULL, 2000000.0, 4)",
	})
	if len(records) != 5 {
		t.Fatalf("invalid records disappeared: %+v", records)
	}
	for _, r := range records {
		if r.RowID == 3 && (r.DurationNS != nil || r.Issue != "unknown_duration") {
			t.Fatal("NULL duration filled")
		}
		if r.RowID == 4 && (r.DurationNS != nil || r.Value != nil || r.Issue == "") {
			t.Fatal("multiple invalid fields hidden")
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewCPUStateFrequency, TimeStart: 0, TimeStartSet: true, TimeEnd: .04}).CPUStateFrequency
	if !tracequery.ValidCPUStateFrequency(*p) || p.CPUCount != 1 || p.KnownJointMs != 10 || p.UnknownJointMs != 30 {
		t.Fatalf("unknown-end record ignored instead of masking possible overlap: %+v", p)
	}
}

func TestExportTraceDBCPUIntervalsKeepLegacyLimitsAndClocks(t *testing.T) {
	out, records, _ := exportCPUIntervalFixture(t, []string{
		"INSERT INTO cpu_measure_filter VALUES (5, 'cpu_frequency_limits_min', 0), (6, 'cpu_frequency_limits_max', 0)",
		"CREATE TABLE measure_filter (id, name, type)",
		"INSERT INTO measure_filter VALUES (7, 'ddr_freq', 'clock_rate_filter')",
		"INSERT INTO measure VALUES (1, 0, 40000000, 0.0, 1), (2, 0, 40000000, 1000000.0, 2)",
		"INSERT INTO measure VALUES (3, 0, NULL, 300000.0, 5), (4, 0, NULL, 2000000.0, 6), (5, 0, NULL, 400.0, 7)",
	})
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || !strings.Contains(string(body), "cpu_frequency_limits: min=300000 max=2000000 cpu_id=0") || !strings.Contains(string(body), "clock_set_rate: ddr_freq 400") {
		t.Fatalf("native duration adoption changed unrelated limit/clock paths: %s", body)
	}
}
