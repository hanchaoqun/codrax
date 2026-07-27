package hitraceconv

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceDBUnhandledTableInventorySurfacesNonemptyWholeTableLoss(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE process (ipid, pid)",
		"INSERT INTO process VALUES (1, 100)",
		`CREATE TABLE "vendor odd" (payload)`,
		`INSERT INTO "vendor odd" VALUES ('retained only in DB')`,
		"CREATE TABLE empty_future (payload)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	inventory, err := inspectTraceDBUnhandledTableInventory(context.Background(), tdb, []TraceDBCoverage{
		{Family: "resolver", Table: "PROCESS", Found: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 2 {
		t.Fatalf("inventory rows=%d want summary+one nonempty finding: %+v", len(inventory), inventory)
	}
	summary := inventory[0]
	for key, want := range map[string]int64{
		"tables_in_inventory":               3,
		"classified_tables":                 1,
		"unclassified_tables":               2,
		"unclassified_empty_tables":         1,
		"unclassified_nonempty_tables":      1,
		"unclassified_uninspectable_tables": 0,
		"inventory_truncated":               0,
	} {
		if got := summary.Metrics[key]; got != want {
			t.Fatalf("summary metric %s=%d want %d: %+v", key, got, want, summary)
		}
	}
	finding := inventory[1]
	if finding.Family != "conversion_inventory" || finding.Table != "vendor odd" ||
		finding.Role != "unsupported_input" || !finding.Found || finding.RowsRead != 1 ||
		!strings.Contains(finding.Skipped, "rows were not converted") ||
		!strings.Contains(summary.Skipped, `"vendor odd"`) {
		t.Fatalf("nonempty table loss was not explicit: summary=%+v finding=%+v", summary, finding)
	}
}

func TestTraceDBUnhandledTableInventoryConsumesTypedDependencyLineage(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE primary_rows (value)",
		"INSERT INTO primary_rows VALUES (1)",
		"CREATE TABLE dependency_filter (value)",
		"INSERT INTO dependency_filter VALUES (2)",
		"CREATE TABLE unsupported_rows (value)",
		"INSERT INTO unsupported_rows VALUES (3)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	inventory, err := inspectTraceDBUnhandledTableInventory(context.Background(), tdb, []TraceDBCoverage{{
		Family:       "counter",
		Table:        "synthetic_composite_face",
		SourceTables: []string{"primary_rows", "dependency_filter"},
		Found:        true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 2 ||
		inventory[0].Metrics["classified_tables"] != 2 ||
		inventory[0].Metrics["unclassified_nonempty_tables"] != 1 ||
		inventory[1].Table != "unsupported_rows" {
		t.Fatalf("typed dependency lineage did not close inventory classification: %+v", inventory)
	}
}

func TestTraceDBUnhandledTableInventoryFeedsCustomerCaveat(t *testing.T) {
	inventory := TraceDBCoverage{
		Family: "conversion_inventory",
		Table:  "__table_inventory__",
		Role:   "diagnostic_inventory",
		Found:  true,
		Metrics: map[string]int64{
			"unclassified_nonempty_tables":      2,
			"unclassified_uninspectable_tables": 1,
			"inventory_truncated":               1,
		},
		Skipped: `unclassified_nonempty_tables=2 roster="future_a","future_b"`,
	}
	quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{inventory})
	coverage := []TraceDBCoverage{inventory, quality}
	joined := strings.Join(traceDBSemanticQualityCaveats(coverage), "\n")
	for _, want := range []string{
		"2 nonempty table(s)",
		"their rows were not converted",
		`roster="future_a","future_b"`,
		"table inventory is incomplete",
		"retained-DB review is required",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("inventory caveat missing %q: %s", want, joined)
		}
	}
}

func TestTraceDBUnhandledTableInventoryIsWiredIntoConversion(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 101, 1, 'worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"CREATE TABLE device_info (physical_width, physical_height, physical_frame_rate)",
		"INSERT INTO device_info VALUES (1440, 2560, 120)",
		"CREATE TABLE meta (name, value)",
		"INSERT INTO meta VALUES ('parse_tool', 'trace_streamer')",
		"CREATE TABLE future_vendor_family (payload)",
		"INSERT INTO future_vendor_family VALUES ('not exported')",
	})
	result, err := exportTraceDBToSystrace(context.Background(), path,
		filepath.Join(t.TempDir(), "inventory.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	var summary, finding, device, parserMeta *TraceDBCoverage
	for index := range result.Coverage {
		item := &result.Coverage[index]
		switch {
		case item.Family == "conversion_inventory" && item.Table == "__table_inventory__":
			summary = item
		case item.Family == "conversion_inventory" && item.Table == "future_vendor_family":
			finding = item
		case item.Family == "metadata" && item.Table == "device_info":
			device = item
		case item.Family == "metadata" && item.Table == "meta":
			parserMeta = item
		}
	}
	if summary == nil || finding == nil || device == nil || parserMeta == nil ||
		summary.Metrics["unclassified_nonempty_tables"] != 1 ||
		finding.Role != "unsupported_input" || finding.RowsRead != 1 ||
		device.Metadata["physical_frame_rate"] != "120" ||
		parserMeta.Metadata["parse_tool"] != "trace_streamer" {
		t.Fatalf("production conversion lost inventory wiring: summary=%+v finding=%+v coverage=%+v",
			summary, finding, result.Coverage)
	}
	if caveats := strings.Join(traceDBSemanticQualityCaveats(result.Coverage), "\n"); !strings.Contains(caveats, "future_vendor_family") ||
		!strings.Contains(caveats, "rows were not converted") {
		t.Fatalf("production inventory caveat missing: %s", caveats)
	}
}
