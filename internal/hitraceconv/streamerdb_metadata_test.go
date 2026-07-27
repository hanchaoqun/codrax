package hitraceconv

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTraceDBDiagnosticMetadataPreservesBoundedDisplayOnlyValues(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE device_info (physical_width, physical_height, physical_frame_rate)",
		"INSERT INTO device_info VALUES (1440, 2560, 120)",
		"CREATE TABLE meta (name, value)",
		"INSERT INTO meta VALUES ('parse_tool', 'trace_streamer')",
		"INSERT INTO meta VALUES ('source_type', 'htrace')",
		"INSERT INTO meta VALUES ('future_key', 'display-only')",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	items, err := inspectTraceDBDiagnosticMetadata(context.Background(), tdb)
	if err != nil {
		t.Fatal(err)
	}
	device := requireTraceDBCoverage(t, items, "metadata", "device_info")
	if device.Role != "diagnostic_metadata" || device.RowsRead != 1 || device.RowsEmitted != 1 ||
		device.Metadata["physical_width"] != "1440" ||
		device.Metadata["physical_height"] != "2560" ||
		device.Metadata["physical_frame_rate"] != "120" ||
		device.Metrics["physical_frame_rate"] != 120 ||
		!strings.Contains(device.FieldSources["effect"], "never emitted as ftrace") {
		t.Fatalf("device metadata provenance drifted: %+v", device)
	}
	parser := requireTraceDBCoverage(t, items, "metadata", "meta")
	if parser.Role != "diagnostic_metadata" || parser.RowsRead != 3 || parser.RowsEmitted != 3 ||
		parser.Metadata["parse_tool"] != "trace_streamer" ||
		parser.Metadata["source_type"] != "htrace" ||
		parser.Metadata["future_key"] != "display-only" ||
		!strings.Contains(parser.FieldSources["effect"], "never become source-admission") {
		t.Fatalf("parser metadata provenance drifted: %+v", parser)
	}
	body, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"metadata":{"future_key":"display-only"`) {
		t.Fatalf("typed metadata missing from JSON coverage: %s", body)
	}
	inventory, err := inspectTraceDBUnhandledTableInventory(context.Background(), tdb, items)
	if err != nil {
		t.Fatal(err)
	}
	if inventory[0].Metrics["classified_tables"] != 2 ||
		inventory[0].Metrics["unclassified_nonempty_tables"] != 0 {
		t.Fatalf("metadata tables remained unclassified: %+v", inventory)
	}
}

func TestTraceDBDiagnosticMetadataRejectsAmbiguousAndMalformedRowsLocally(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE device_info (physical_width, physical_height, physical_frame_rate)",
		"INSERT INTO device_info VALUES (1440, 2560, 120)",
		"INSERT INTO device_info VALUES (1080, 1920, 60)",
		"CREATE TABLE meta (name, value)",
		"INSERT INTO meta VALUES ('good', 'kept')",
		"INSERT INTO meta VALUES ('duplicate', 'first')",
		"INSERT INTO meta VALUES ('duplicate', 'second')",
		"INSERT INTO meta VALUES ('bad name', 'rejected')",
		"INSERT INTO meta VALUES ('oversized', CAST(zeroblob(1025) AS TEXT))",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	items, err := inspectTraceDBDiagnosticMetadata(context.Background(), tdb)
	if err != nil {
		t.Fatal(err)
	}
	device := requireTraceDBCoverage(t, items, "metadata", "device_info")
	if len(device.Metadata) != 0 || device.RowsEmitted != 0 ||
		!strings.Contains(device.Skipped, "expected exactly one device_info row") {
		t.Fatalf("ambiguous device metadata acquired authority: %+v", device)
	}
	parser := requireTraceDBCoverage(t, items, "metadata", "meta")
	if parser.RowsEmitted != 1 || parser.Metadata["good"] != "kept" ||
		parser.Metadata["duplicate"] != "" || parser.Metadata["bad name"] != "" ||
		!strings.Contains(parser.Skipped, "duplicate_metadata_name=1") ||
		!strings.Contains(parser.Skipped, "invalid_or_oversized_metadata_row=2") {
		t.Fatalf("malformed parser metadata was not localized: %+v", parser)
	}
}
