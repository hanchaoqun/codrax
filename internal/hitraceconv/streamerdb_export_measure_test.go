package hitraceconv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestExportTraceDBCPUMeasuresStrictTuplesAndScalarBoundaries(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_idle', 0)",
		"INSERT INTO cpu_measure_filter VALUES (2, 'cpu_frequency', 4)",
		"INSERT INTO cpu_measure_filter VALUES (3, 'cpu_frequency_limits_min', 4)",
		"INSERT INTO cpu_measure_filter VALUES (4, 'cpu_frequency_limits_max', 4)",
		"INSERT INTO cpu_measure_filter VALUES (5, 'cpu_frequency_limits_min', 5)",
		"INSERT INTO cpu_measure_filter VALUES (6, 'cpu_frequency', 6)",
		"INSERT INTO cpu_measure_filter VALUES (7, 'cpu_frequency', NULL)",
		"INSERT INTO cpu_measure_filter VALUES (8, 'cpu_frequency', 4095)",
		"INSERT INTO cpu_measure_filter VALUES (9, 'cpu_frequency', 4096)",
		"INSERT INTO cpu_measure_filter VALUES (10, 'cpu_frequency', 7)",
		"INSERT INTO cpu_measure_filter VALUES (11, 'cpu_frequency', 8)",
		"INSERT INTO cpu_measure_filter VALUES (12, 'cpu_frequency', 9)",
		"INSERT INTO measure VALUES (1, 1000, 1.0, 1)",
		"INSERT INTO measure VALUES (2, 1100, 2200000.0, 2)",
		"INSERT INTO measure VALUES (3, 1200, 300000.0, 3)",
		"INSERT INTO measure VALUES (4, 1300, 2000000.0, 4)",
		"INSERT INTO measure VALUES (5, 1400, 400000.0, 3)",
		"INSERT INTO measure VALUES (6, 1500, 500000.0, 5)",
		"INSERT INTO measure VALUES (7, 1600, 1.5, 6)",
		"INSERT INTO measure VALUES (8, 1700, 1000000.0, 7)",
		"INSERT INTO measure VALUES (9, 1800, 2400000.0, 8)",
		"INSERT INTO measure VALUES (10, 1900, 2500000.0, 9)",
		"INSERT INTO measure VALUES (20, 2000, 1000000.0, 10)",
		"INSERT INTO measure VALUES (21, 1950, 1200000.0, 10)",
		"INSERT INTO measure VALUES (22, 2100, 1000000.0, 11)",
		"INSERT INTO measure VALUES (23, 2100, 1200000.0, 11)",
		"INSERT INTO measure VALUES (30, 2200, 1000000.0, 12)",
		"INSERT INTO measure VALUES (30, 2300, 1200000.0, 12)",
	})
	outPath := filepath.Join(t.TempDir(), "cpu-measure-strict.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export CPU measure fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"[000] ....     0.000001: cpu_idle: state=1 cpu_id=0",
		"[004] ....     0.000001100: cpu_frequency: state=2200000 cpu_id=4",
		"cpu_frequency_limits: min=300000 max=2000000 cpu_id=4",
		"cpu_frequency_limits: min=400000 max=2000000 cpu_id=4",
		"[4095] ....     0.000001800: cpu_frequency: state=2400000 cpu_id=4095",
		"cpu_frequency: state=1000000 cpu_id=9",
		"cpu_frequency: state=1200000 cpu_id=9",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("strict CPU measure output missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"min=500000 max=0", "min=0 max=", "cpu_id=5", "cpu_id=6", "cpu_id=7", "cpu_id=8", "cpu_id=4096"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("malformed/incomplete CPU measure leaked %q:\n%s", forbidden, body)
		}
	}
	coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	if coverage.RowsEmitted != 7 || coverage.FieldSources["limit_tuple"] == "" {
		t.Fatalf("CPU measure coverage/provenance mismatch: %+v", coverage)
	}
	for _, want := range []string{"incomplete_limit_tuple=1", "incomplete_limit_updates=1", "invalid_cpu_filter=2", "timestamp_rollback=1", "duplicate_lane_timestamp=1", "invalid_sample_scalar_or_identity", "limit_updates_waiting_for_peer=1"} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("CPU measure skip ledger missing %q: %+v", want, coverage)
		}
	}
}

func TestExportTraceDBClockRatesPreserveClockIdentityWithoutCPUOwner(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value REAL, filter_id)",
		"CREATE TABLE measure_filter (id, name, type)",
		"INSERT INTO measure_filter VALUES (10, 'ddr_freq', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (11, 'cpu_freq', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (12, 'frac_clk', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (13, 'bad clock', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (14, 'rollback_clk', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (15, 'duplicate_ts_clk', 'clock_rate_filter')",
		"INSERT INTO measure VALUES (1, 1000, 400.0, 10)",
		"INSERT INTO measure VALUES (2, 1100, 2200000.0, 11)",
		"INSERT INTO measure VALUES (3, 1200, 1.5, 12)",
		"INSERT INTO measure VALUES (4, 1300, 100.0, 13)",
		"INSERT INTO measure VALUES (20, 2000, 100.0, 14)",
		"INSERT INTO measure VALUES (21, 1900, 200.0, 14)",
		"INSERT INTO measure VALUES (22, 2100, 100.0, 15)",
		"INSERT INTO measure VALUES (23, 2100, 200.0, 15)",
	})
	outPath := filepath.Join(t.TempDir(), "clock-measure-strict.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export clock measure fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{"clock_set_rate: ddr_freq 400", "clock_set_rate: cpu_freq 2200000"} {
		if !strings.Contains(body, want) {
			t.Fatalf("clock output missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "clock_set_rate:") && strings.Contains(body, "cpu_id=") {
		t.Fatalf("measure_filter has no CPU owner but exporter minted cpu_id:\n%s", body)
	}
	for _, forbidden := range []string{"frac_clk", "bad clock", "rollback_clk", "duplicate_ts_clk"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("invalid clock lane leaked %q:\n%s", forbidden, body)
		}
	}
	coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure_filter")
	if coverage.RowsEmitted != 2 || coverage.FieldSources["cpu_owner"] != "not present in measure_filter schema; cpu_id intentionally omitted" {
		t.Fatalf("clock coverage/provenance mismatch: %+v", coverage)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse strict clock output: %v", err)
	}
	seen := map[string]bool{}
	for _, event := range idx.Events {
		if event.Name != "clock_set_rate" {
			continue
		}
		seen[event.ClockName] = true
		if event.CPUForFieldPresent {
			t.Fatalf("keyless clock unexpectedly acquired a CPU owner: %+v", event)
		}
	}
	if !seen["ddr_freq"] || !seen["cpu_freq"] {
		t.Fatalf("clock identities lost after round trip: %+v", idx.Events)
	}
}

func TestExportTraceDBClockEventFiltersProvideExactCPUAndCrossCheckGeneric(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value REAL, filter_id)",
		"CREATE TABLE measure_filter (id, name, type)",
		"CREATE TABLE clock_event_filter (id, type, name, cpu)",
		"INSERT INTO measure_filter VALUES (10, 'ddr_freq', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (11, 'generic_name', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (14, 'generic_only', 'clock_rate_filter')",
		"INSERT INTO clock_event_filter VALUES (10, 'clock_set_rate', 'ddr_freq', 4)",
		"INSERT INTO clock_event_filter VALUES (11, 'clock_set_rate', 'specialized_name', 5)",
		"INSERT INTO clock_event_filter VALUES (12, 'clock_set_rate', 'specialized_only', 6)",
		"INSERT INTO clock_event_filter VALUES (13, 'clock_set_rate', 'bad_cpu', 4096)",
		"INSERT INTO measure VALUES (1, 1000, 400.0, 10)",
		"INSERT INTO measure VALUES (2, 1100, 500.0, 11)",
		"INSERT INTO measure VALUES (3, 1200, 600.0, 12)",
		"INSERT INTO measure VALUES (4, 1300, 700.0, 13)",
		"INSERT INTO measure VALUES (5, 1400, 800.0, 14)",
	})
	outPath := filepath.Join(t.TempDir(), "clock-event-filter.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export specialized clock fixture: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"[004] ....     0.000001: clock_set_rate: ddr_freq state=400 cpu_id=4",
		"[006] ....     0.000001200: clock_set_rate: specialized_only state=600 cpu_id=6",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("specialized clock output missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"generic_name", "specialized_name", "bad_cpu", "generic_only"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unproved specialized clock lane leaked %q:\n%s", forbidden, body)
		}
	}
	coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "clock_event_filter")
	if coverage.RowsEmitted != 2 ||
		coverage.FieldSources["cpu_owner"] != "clock_event_filter.cpu exact SQLite INTEGER in 0..4095; emitted as header CPU and cpu_id" ||
		strings.Join(coverage.SourceTables, ",") != "measure,clock_event_filter,measure_filter" {
		t.Fatalf("specialized clock coverage/provenance mismatch: %+v", coverage)
	}
	for _, want := range []string{
		"specialized_generic_filter_conflict=1",
		"invalid_specialized_clock_filter=1",
		"generic_only_clock_filter_withheld=1",
	} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("specialized clock skip ledger missing %q: %+v", want, coverage)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse specialized clock output: %v", err)
	}
	owners := map[string]int{}
	for _, event := range idx.Events {
		if event.Name == "clock_set_rate" && event.CPUForFieldPresent {
			owners[event.ClockName] = event.CPUForField
		}
	}
	if owners["ddr_freq"] != 4 || owners["specialized_only"] != 6 {
		t.Fatalf("specialized clock CPU owners lost after round trip: %+v", idx.Events)
	}
}

func TestExportTraceDBPresentMalformedClockEventFilterForbidsGenericFallback(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value, filter_id)",
		"CREATE TABLE measure_filter (id, name, type)",
		"CREATE TABLE clock_event_filter (id, type, name)",
		"INSERT INTO measure_filter VALUES (10, 'generic_clk', 'clock_rate_filter')",
		"INSERT INTO clock_event_filter VALUES (10, 'clock_set_rate', 'generic_clk')",
		"INSERT INTO measure VALUES (1, 1000, 400, 10)",
	})
	outPath := filepath.Join(t.TempDir(), "malformed-specialized-clock.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export malformed specialized clock fixture: %v", err)
	}
	if body, readErr := os.ReadFile(outPath); readErr == nil {
		if strings.Contains(string(body), "clock_set_rate:") {
			t.Fatalf("generic fallback hid a malformed specialized registry:\n%s", body)
		}
	} else if !os.IsNotExist(readErr) {
		t.Fatalf("read malformed specialized output: %v", readErr)
	}
	coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "clock_event_filter")
	if !strings.Contains(strings.Join(coverage.ColumnsMissing, ","), "cpu") ||
		!strings.Contains(coverage.Skipped, "generic fallback forbidden") {
		t.Fatalf("malformed specialized authority was not surfaced: %+v", coverage)
	}
}

func TestExportTraceDBMeasureFilterOwnershipConflictFailsClosedLocally(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"CREATE TABLE measure_filter (id, name, type)",
		"INSERT INTO cpu_measure_filter VALUES (1, 'vendor_cpu_counter', 0)",
		"INSERT INTO cpu_measure_filter VALUES (2, 'cpu_frequency', 2)",
		"INSERT INTO measure_filter VALUES (1, 'shared_clk', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (3, 'independent_clk', 'clock_rate_filter')",
		"INSERT INTO measure VALUES (1, 1000, 100.0, 1)",
		"INSERT INTO measure VALUES (2, 1100, 1200000.0, 2)",
		"INSERT INTO measure VALUES (3, 1200, 400.0, 3)",
	})
	body, result := exportTraceDBMeasureFixture(t, path, "cross-owner")
	for _, want := range []string{
		"cpu_frequency: state=1200000 cpu_id=2",
		"clock_set_rate: independent_clk 400",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("independent measure lane missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"shared_clk", "cpu_id=0"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("cross-owner ID leaked %q:\n%s", forbidden, body)
		}
	}
	cpuCoverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	clockCoverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure_filter")
	if cpuCoverage.RowsEmitted != 1 || !strings.Contains(cpuCoverage.Skipped, "cross_filter_owner_conflict=1") {
		t.Fatalf("CPU cross-owner conflict was not fail-closed locally: %+v", cpuCoverage)
	}
	if clockCoverage.RowsEmitted != 1 || !strings.Contains(clockCoverage.Skipped, "cross_filter_owner_conflict=1") {
		t.Fatalf("clock cross-owner conflict was not fail-closed locally: %+v", clockCoverage)
	}
}

func TestExportTraceDBPartialCPUFilterRegistryStillReservesClockOwner(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id)",
		"CREATE TABLE measure_filter (id, name, type)",
		"INSERT INTO cpu_measure_filter VALUES (1)",
		"INSERT INTO measure_filter VALUES (1, 'shared_clk', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (2, 'independent_clk', 'clock_rate_filter')",
		"INSERT INTO measure VALUES (1000, 300.0, 1)",
		"INSERT INTO measure VALUES (1100, 400.0, 2)",
	})
	body, result := exportTraceDBMeasureFixture(t, path, "partial-cpu-owner")
	if strings.Contains(body, "shared_clk") {
		t.Fatalf("partial CPU registry ID must reserve the shared namespace and suppress its clock peer:\n%s", body)
	}
	if !strings.Contains(body, "clock_set_rate: independent_clk 400") {
		t.Fatalf("unrelated clock owner was suppressed by a local partial-registry conflict:\n%s", body)
	}
	clockCoverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure_filter")
	if clockCoverage.RowsEmitted != 1 || !strings.Contains(clockCoverage.Skipped, "cross_filter_owner_conflict=1") {
		t.Fatalf("partial CPU registry conflict was not surfaced on the clock lane: %+v", clockCoverage)
	}
}

func TestExportTraceDBPartialClockFilterRegistryStillReservesCPUOwner(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"CREATE TABLE measure_filter (id, type)",
		"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_frequency', 0)",
		"INSERT INTO cpu_measure_filter VALUES (2, 'cpu_frequency', 1)",
		"INSERT INTO measure_filter VALUES (1, 'clock_rate_filter')",
		"INSERT INTO measure VALUES (1000, 1000000.0, 1)",
		"INSERT INTO measure VALUES (1100, 1200000.0, 2)",
	})
	body, result := exportTraceDBMeasureFixture(t, path, "partial-clock-owner")
	if strings.Contains(body, "cpu_id=0") {
		t.Fatalf("partial clock registry ID must reserve the shared namespace and suppress its CPU peer:\n%s", body)
	}
	if !strings.Contains(body, "cpu_frequency: state=1200000 cpu_id=1") {
		t.Fatalf("unrelated CPU owner was suppressed by a local partial-registry conflict:\n%s", body)
	}
	cpuCoverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	if cpuCoverage.RowsEmitted != 1 || !strings.Contains(cpuCoverage.Skipped, "cross_filter_owner_conflict=1") {
		t.Fatalf("partial clock registry conflict was not surfaced on the CPU lane: %+v", cpuCoverage)
	}
}

func TestExportTraceDBMalformedEquivalentFilterIDTaintsOnlyItsLane(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"CREATE TABLE measure_filter (id, name, type)",
		"INSERT INTO cpu_measure_filter VALUES (10, 'cpu_frequency', 1)",
		"INSERT INTO cpu_measure_filter VALUES (11, 'cpu_frequency', 2)",
		"INSERT INTO measure_filter VALUES (20, 'tainted_clk', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (21, 'independent_clk', 'clock_rate_filter')",
		"INSERT INTO measure VALUES (1, 1000, 1000000.0, 10)",
		"INSERT INTO measure VALUES (2, 1100, 1100000.0, CAST(10 AS REAL))",
		"INSERT INTO measure VALUES (3, 1200, 1200000.0, '10')",
		"INSERT INTO measure VALUES (4, 1300, 1300000.0, 10)",
		"INSERT INTO measure VALUES (5, 1400, 1400000.0, 11)",
		"INSERT INTO measure VALUES (6, 1500, 100.0, 20)",
		"INSERT INTO measure VALUES (7, 1600, 110.0, CAST(20 AS REAL))",
		"INSERT INTO measure VALUES (8, 1700, 120.0, '20')",
		"INSERT INTO measure VALUES (9, 1800, 130.0, 20)",
		"INSERT INTO measure VALUES (10, 1900, 140.0, 21)",
	})
	body, result := exportTraceDBMeasureFixture(t, path, "equivalent-filter-id")
	for _, want := range []string{
		"cpu_frequency: state=1400000 cpu_id=2",
		"clock_set_rate: independent_clk 140",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("unrelated lane missing after malformed equivalent ID %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"cpu_id=1", "tainted_clk"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("malformed numeric-equivalent filter ID failed to taint %q:\n%s", forbidden, body)
		}
	}
	cpuCoverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	clockCoverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure_filter")
	for label, coverage := range map[string]TraceDBCoverage{"cpu": cpuCoverage, "clock": clockCoverage} {
		if coverage.RowsEmitted != 1 || !strings.Contains(coverage.Skipped, "invalid_filter_id_scalar=2") ||
			!strings.Contains(coverage.Skipped, "lane_fail_closed=2") {
			t.Fatalf("%s malformed-equivalent lane audit mismatch: %+v", label, coverage)
		}
	}
}

func TestExportTraceDBTargetAndUnknownFilterDefinitionSharingIDPoisonTarget(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"CREATE TABLE measure_filter (id, name, type)",
		"INSERT INTO cpu_measure_filter VALUES (30, 'cpu_frequency', 3)",
		"INSERT INTO cpu_measure_filter VALUES (30, 'vendor_counter', 9)",
		"INSERT INTO cpu_measure_filter VALUES (31, 'cpu_frequency', 4)",
		"INSERT INTO measure_filter VALUES (40, 'poisoned_clk', 'clock_rate_filter')",
		"INSERT INTO measure_filter VALUES (40, 'vendor_metric', 'counter_filter')",
		"INSERT INTO measure_filter VALUES (41, 'independent_clk', 'clock_rate_filter')",
		"INSERT INTO measure VALUES (1, 1000, 1300000.0, 30)",
		"INSERT INTO measure VALUES (2, 1100, 1400000.0, 31)",
		"INSERT INTO measure VALUES (3, 1200, 300.0, 40)",
		"INSERT INTO measure VALUES (4, 1300, 400.0, 41)",
	})
	body, result := exportTraceDBMeasureFixture(t, path, "filter-definition-poison")
	for _, want := range []string{
		"cpu_frequency: state=1400000 cpu_id=4",
		"clock_set_rate: independent_clk 400",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("independent filter definition missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"cpu_id=3", "poisoned_clk"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("target sharing an ID with unknown definition leaked %q:\n%s", forbidden, body)
		}
	}
	cpuCoverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	clockCoverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure_filter")
	for label, coverage := range map[string]TraceDBCoverage{"cpu": cpuCoverage, "clock": clockCoverage} {
		if coverage.RowsEmitted != 1 || !strings.Contains(coverage.Skipped, "duplicate_or_invalid_filter_id=1") {
			t.Fatalf("%s shared filter-ID definition audit mismatch: %+v", label, coverage)
		}
	}
}

func TestTraceDBMeasureHiddenRowIDSelectionAndWithoutRowIDFailure(t *testing.T) {
	t.Run("declared rowid falls through to hidden _rowid_", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE measure (rowid INTEGER, ts, value REAL, filter_id)",
			"INSERT INTO measure VALUES (77, 1000, 100.0, 1)",
		})
		tdb, err := openTraceDB(context.Background(), path)
		if err != nil {
			t.Fatalf("open rowid-shadow fixture: %v", err)
		}
		defer tdb.close()
		expr, source, err := traceDBHiddenRowIDExpr(context.Background(), tdb.db, "measure")
		if err != nil {
			t.Fatalf("select unshadowed hidden rowid alias: %v", err)
		}
		if expr != "_rowid_" || source != "measure.hidden__rowid_" {
			t.Fatalf("declared rowid must fall through to hidden _rowid_: expr=%q source=%q", expr, source)
		}
	})

	t.Run("explicit negative zero and positive hidden rowids preserve source order", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE measure (ts, value REAL, filter_id)",
			"CREATE TABLE cpu_measure_filter (id, name, cpu)",
			"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_frequency', 0)",
			"INSERT INTO measure(rowid, ts, value, filter_id) VALUES (-1, 1000, 1000000.0, 1)",
			"INSERT INTO measure(rowid, ts, value, filter_id) VALUES (0, 1100, 1100000.0, 1)",
			"INSERT INTO measure(rowid, ts, value, filter_id) VALUES (1, 1200, 1200000.0, 1)",
		})
		body, result := exportTraceDBMeasureFixture(t, path, "signed-hidden-rowids")
		last := -1
		for _, state := range []string{"state=1000000", "state=1100000", "state=1200000"} {
			at := strings.Index(body, state)
			if at < 0 || at <= last {
				t.Fatalf("signed hidden rowids did not preserve -1/0/1 source order at %q:\n%s", state, body)
			}
			last = at
		}
		coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
		if coverage.RowsEmitted != 3 || coverage.FieldSources["stable_identity"] != "measure.hidden_rowid" {
			t.Fatalf("signed hidden rowid provenance mismatch: %+v", coverage)
		}
	})

	t.Run("WITHOUT ROWID fails loud", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE measure (id INTEGER PRIMARY KEY, ts, value REAL, filter_id) WITHOUT ROWID",
			"CREATE TABLE cpu_measure_filter (id, name, cpu)",
			"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_frequency', 0)",
			"INSERT INTO measure VALUES (1, 1000, 1000000.0, 1)",
		})
		outPath := filepath.Join(t.TempDir(), "without-rowid.systrace")
		_, err := exportTraceDBToSystrace(context.Background(), path, outPath)
		if err == nil || !strings.Contains(err.Error(), "measure has no provable hidden rowid source order") {
			t.Fatalf("WITHOUT ROWID measure table must fail loud, got: %v", err)
		}
		if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
			t.Fatalf("failed export must not publish an artifact: %v", statErr)
		}
	})

	t.Run("empty WITHOUT ROWID measure does not block unrelated families", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE measure (id INTEGER PRIMARY KEY, ts, value REAL, filter_id) WITHOUT ROWID",
			"CREATE TABLE cpu_measure_filter (id, name, cpu)",
			"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_frequency', 0)",
			"CREATE TABLE network (ts, tx_speed REAL, rx_speed REAL)",
			"INSERT INTO network VALUES (2000, 1.5, 2.5)",
		})
		body, result := exportTraceDBMeasureFixture(t, path, "empty-without-rowid")
		for _, want := range []string{
			"tracing_mark_write: C|0|net_tx_speed|1.5",
			"tracing_mark_write: C|0|net_rx_speed|2.5",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("empty WITHOUT ROWID measure blocked unrelated export %q:\n%s", want, body)
			}
		}
		coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
		if coverage.Error != "" || coverage.RowsEmitted != 0 ||
			!strings.Contains(coverage.Skipped, "no_strict_exportable_measure_rows=1") {
			t.Fatalf("empty WITHOUT ROWID measure should be a local no-op: %+v", coverage)
		}
	})
}

func TestExportTraceDBInterleavedInvalidSemanticValueFailsClosedLane(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"INSERT INTO cpu_measure_filter VALUES (50, 'cpu_frequency', 5)",
		"INSERT INTO cpu_measure_filter VALUES (51, 'cpu_frequency', 6)",
		"INSERT INTO measure VALUES (1, 1000, 1000000.0, 50)",
		"INSERT INTO measure VALUES (2, 1100, 0.0, 50)",
		"INSERT INTO measure VALUES (3, 1200, 1200000.0, 50)",
		"INSERT INTO measure VALUES (4, 1300, 1300000.0, 51)",
	})
	body, result := exportTraceDBMeasureFixture(t, path, "semantic-invalid")
	if strings.Contains(body, "cpu_id=5") {
		t.Fatalf("interleaved invalid frequency must fail-close its entire lane:\n%s", body)
	}
	if !strings.Contains(body, "cpu_frequency: state=1300000 cpu_id=6") {
		t.Fatalf("unrelated frequency lane was suppressed:\n%s", body)
	}
	coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	if coverage.RowsEmitted != 1 || !strings.Contains(coverage.Skipped, "invalid_semantic_value=1") ||
		!strings.Contains(coverage.Skipped, "lane_fail_closed=2") {
		t.Fatalf("semantic lane fail-close coverage mismatch: %+v", coverage)
	}
}

func TestExportTraceDBLimitTupleCrossLaneRollbackAndInvalidRangeFailClosed(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (id, ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"INSERT INTO cpu_measure_filter VALUES (60, 'cpu_frequency_limits_min', 6)",
		"INSERT INTO cpu_measure_filter VALUES (61, 'cpu_frequency_limits_max', 6)",
		"INSERT INTO cpu_measure_filter VALUES (62, 'cpu_frequency_limits_min', 7)",
		"INSERT INTO cpu_measure_filter VALUES (63, 'cpu_frequency_limits_max', 7)",
		"INSERT INTO cpu_measure_filter VALUES (64, 'cpu_frequency_limits_min', 8)",
		"INSERT INTO cpu_measure_filter VALUES (65, 'cpu_frequency_limits_max', 8)",
		"INSERT INTO measure VALUES (1, 2000, 300000.0, 60)",
		"INSERT INTO measure VALUES (2, 1900, 2000000.0, 61)",
		"INSERT INTO measure VALUES (3, 3000, 2000000.0, 62)",
		"INSERT INTO measure VALUES (4, 3000, 1000000.0, 63)",
		"INSERT INTO measure VALUES (5, 4000, 500000.0, 64)",
		"INSERT INTO measure VALUES (6, 4000, 1500000.0, 65)",
	})
	body, result := exportTraceDBMeasureFixture(t, path, "limit-audit")
	if !strings.Contains(body, "cpu_frequency_limits: min=500000 max=1500000 cpu_id=8") {
		t.Fatalf("valid independent limit tuple missing:\n%s", body)
	}
	for _, forbidden := range []string{"cpu_id=6", "cpu_id=7"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("invalid limit tuple leaked %q:\n%s", forbidden, body)
		}
	}
	coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	const wantSkipped = "invalid_limit_tuple=2,limit_lane_fail_closed=4,limit_timestamp_rollback=2,limit_updates_coalesced=1"
	if coverage.RowsRead != 6 || coverage.RowsEmitted != 1 || coverage.Skipped != wantSkipped {
		t.Fatalf("limit tuple exact coverage mismatch: got=%+v want_skipped=%q", coverage, wantSkipped)
	}
}

func TestExportTraceDBLimitTupleCarriesInitialSingleSideAcrossTimestamps(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE measure (ts, value REAL, filter_id)",
		"CREATE TABLE cpu_measure_filter (id, name, cpu)",
		"INSERT INTO cpu_measure_filter VALUES (70, 'cpu_frequency_limits_min', 9)",
		"INSERT INTO cpu_measure_filter VALUES (71, 'cpu_frequency_limits_max', 9)",
		"INSERT INTO measure VALUES (1000, 300000.0, 70)",
		"INSERT INTO measure VALUES (2000, 1800000.0, 71)",
	})
	body, result := exportTraceDBMeasureFixture(t, path, "limit-initial-carry")
	want := "cpu_frequency_limits: min=300000 max=1800000 cpu_id=9"
	if !strings.Contains(body, want) {
		t.Fatalf("initial one-sided limit update was not carried to its later peer:\n%s", body)
	}
	coverage := requireTraceDBCoverage(t, result.Coverage, "counter", "measure")
	if coverage.RowsRead != 2 || coverage.RowsEmitted != 1 || coverage.Skipped != "limit_updates_waiting_for_peer=1" {
		t.Fatalf("cross-timestamp limit carry coverage mismatch: %+v", coverage)
	}
}

func TestTraceDBExactIntegralMeasureValueBoundaries(t *testing.T) {
	maxInt64 := int64(^uint64(0) >> 1)
	minInt64 := -maxInt64 - 1
	twoTo63 := math.Ldexp(1, 63)
	justBelowTwoTo63 := math.Nextafter(twoTo63, 0)
	tests := []struct {
		name  string
		value any
		want  int64
		ok    bool
	}{
		{name: "integer minimum", value: minInt64, want: minInt64, ok: true},
		{name: "integer maximum", value: maxInt64, want: maxInt64, ok: true},
		{name: "REAL negative 2^63", value: -twoTo63, want: minInt64, ok: true},
		{name: "REAL immediately below 2^63", value: justBelowTwoTo63, want: int64(justBelowTwoTo63), ok: true},
		{name: "REAL positive 2^63", value: twoTo63, ok: false},
		{name: "REAL below negative 2^63", value: math.Nextafter(-twoTo63, math.Inf(-1)), ok: false},
		{name: "fraction", value: 1.5, ok: false},
		{name: "NaN", value: math.NaN(), ok: false},
		{name: "positive infinity", value: math.Inf(1), ok: false},
		{name: "negative infinity", value: math.Inf(-1), ok: false},
		{name: "numeric text is not a value scalar", value: "1", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := traceDBExactIntegralMeasureValue(test.value)
			if ok != test.ok || ok && got != test.want {
				t.Fatalf("traceDBExactIntegralMeasureValue(%v) = (%d, %t), want (%d, %t)", test.value, got, ok, test.want, test.ok)
			}
		})
	}
}

func exportTraceDBMeasureFixture(t *testing.T, path, name string) (string, traceDBSystraceExport) {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), name+".systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export %s fixture: %v", name, err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read %s fixture output: %v", name, err)
	}
	return string(body), result
}

func requireTraceDBCoverage(t *testing.T, coverage []TraceDBCoverage, family, table string) TraceDBCoverage {
	t.Helper()
	for _, item := range coverage {
		if item.Family == family && item.Table == table {
			return item
		}
	}
	t.Fatalf("missing coverage %s/%s: %+v", family, table, coverage)
	return TraceDBCoverage{}
}
