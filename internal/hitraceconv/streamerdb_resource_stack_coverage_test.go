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

func resourceStackCoverageFixture(t *testing.T, extra ...string) ([]TraceDBCoverage, string) {
	t.Helper()
	schema, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.sql")
	if err != nil {
		t.Fatal(err)
	}
	path := createTraceDBFixture(t, append([]string{string(schema)}, extra...))
	out := filepath.Join(t.TempDir(), "resource.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, out)
	if err != nil {
		t.Fatal(err)
	}
	return result.Coverage, out
}

func resourceFrameCoverage(t *testing.T, items []TraceDBCoverage) TraceDBCoverage {
	t.Helper()
	var found []TraceDBCoverage
	for _, item := range items {
		if item.Table == "native_hook_frame" {
			if item.Role == "unsupported_input" {
				t.Fatalf("decoded frame source is simultaneously unsupported: %+v", item)
			}
			if item.Family == "resource_stack.frames" {
				found = append(found, item)
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected one exact frame coverage, got %d", len(found))
	}
	return found[0]
}

func TestNativeResourceStackCoverageMatchesPublishedFrames(t *testing.T) {
	items, path := resourceStackCoverageFixture(t)
	c := resourceFrameCoverage(t, items)
	if !c.Found || c.RowsRead != 8 || c.RowsEmitted != 14 || c.Skipped != "" {
		t.Fatalf("frame table and emitted records were not separately counted: %+v", c)
	}
	for metric, want := range map[string]int64{"referenced_frame_rows": 8, "published_source_frame_rows": 8, "symbol_ambiguous": 1, "symbol_unavailable": 1, "vaddr_null": 1} {
		if c.Metrics[metric] != want {
			t.Errorf("%s=%d want %d: %+v", metric, c.Metrics[metric], want, c)
		}
	}
	var events int
	for _, item := range items {
		if item.Family == "resource_stack" {
			events += item.RowsEmitted
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var eventRecords, frameRecords int
	for _, line := range strings.Split(string(body), "\n") {
		if record, ok := tracewire.ParseResourceStack(line); ok {
			if record.Event != nil {
				eventRecords++
			} else {
				frameRecords++
			}
		}
	}
	if events != 5 || eventRecords != events || frameRecords != c.RowsEmitted {
		t.Fatalf("coverage double-counted or lost carriers: events=%d/%d frames=%d/%d", events, eventRecords, c.RowsEmitted, frameRecords)
	}
}

func TestNativeResourceStackCoveragePreservesUnconvertedRowsAndFields(t *testing.T) {
	items, _ := resourceStackCoverageFixture(t,
		"INSERT INTO native_hook_frame VALUES (90,99,0,0,100,200,0,0,'0x0')",
		"INSERT INTO native_hook_frame VALUES (91,'bad',0,0,100,200,0,0,'0x0')",
		"UPDATE native_hook SET ipid=2 WHERE id=3",
		"ALTER TABLE native_hook_frame ADD COLUMN vendor_detail TEXT",
		"UPDATE native_hook_frame SET ip='bad',vaddr=printf('%5000s','x') WHERE id=1")
	c := resourceFrameCoverage(t, items)
	for metric, want := range map[string]int64{"referenced_frame_rows": 8, "published_source_frame_rows": 5, "unreferenced_frame_rows": 1, "invalid_callchain_rows": 1, "referenced_unpublished_frame_rows": 3, "ip_invalid": 1, "vaddr_invalid": 1} {
		if c.Metrics[metric] != want {
			t.Errorf("%s=%d want %d: %+v", metric, c.Metrics[metric], want, c)
		}
	}
	if c.RowsRead != 10 || c.RowsEmitted != 11 || !strings.Contains(c.FieldSources["unconverted_columns"], "vendor_detail") {
		t.Fatalf("lost residual source coverage: %+v", c)
	}
	for _, want := range []string{"unreferenced_frame_rows=1", "invalid_callchain_rows=1", "referenced_unpublished_frame_rows=3", "unconverted_columns=1"} {
		if !strings.Contains(c.Skipped, want) {
			t.Errorf("missing residual %s in %+v", want, c)
		}
	}
}

func TestNativeResourceStackCoverageUnavailableAndEmptyFrames(t *testing.T) {
	for _, tc := range []struct {
		name, sql, skipped, status string
		found                      bool
		rows                       int
	}{
		{"missing_table", "DROP TABLE native_hook_frame", "missing table", "frame_table_unavailable", false, 0},
		{"missing_schema", "ALTER TABLE native_hook_frame DROP COLUMN depth", "missing required columns: depth", "frame_table_unavailable", true, 8},
		{"empty", "DELETE FROM native_hook_frame", "", "no_frames", true, 0},
		{"missing_physical_identity", "DROP TABLE native_hook_frame; CREATE TABLE native_hook_frame (id INTEGER PRIMARY KEY,callchain_id INT,depth INT) WITHOUT ROWID; INSERT INTO native_hook_frame VALUES(1,7,0)", "stable_resource_frame_row_unavailable=1", "frame_table_unavailable", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, path := resourceStackCoverageFixture(t, tc.sql)
			c := resourceFrameCoverage(t, items)
			if c.Found != tc.found || c.RowsRead != tc.rows || c.RowsEmitted != 0 || c.Skipped != tc.skipped {
				t.Fatalf("unavailable source was hidden or called fully converted: %+v", c)
			}
			idx, err := tracequery.BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewResourceStack, PID: 101, TimeStart: 10, TimeEnd: 10.05}).ResourceStack
			if !tracequery.ValidResourceStack(*p) || len(p.Events) != 3 {
				t.Fatalf("lost independent resource events: %+v", p)
			}
			for _, event := range p.Events {
				if event.Source.StackStatus != tc.status || len(event.Frames) != 0 {
					t.Fatalf("coverage and payload disagree: %+v", event)
				}
			}
		})
	}
}

func TestNativeResourceStackCoverageKeepsDictionaryUnknownStates(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		want      map[string]int64
	}{
		{"missing", "DROP TABLE data_dict", map[string]int64{"symbol_unavailable": 8, "library_unavailable": 8}},
		{"missing_column", "ALTER TABLE data_dict DROP COLUMN data", map[string]int64{"symbol_unavailable": 8, "library_unavailable": 8}},
		{"invalid_null_long_ambiguous", "UPDATE data_dict SET data=NULL WHERE id=100; UPDATE data_dict SET data=printf('%5000s','x') WHERE id=101; UPDATE data_dict SET data=x'ff' WHERE id=201", map[string]int64{"symbol_null": 2, "symbol_invalid": 1, "symbol_ambiguous": 1, "symbol_unavailable": 1, "library_invalid": 5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items, path := resourceStackCoverageFixture(t, tc.sql)
			c := resourceFrameCoverage(t, items)
			if c.RowsRead != 8 || c.RowsEmitted != 14 || c.Metrics["published_source_frame_rows"] != 8 {
				t.Fatalf("unknown dictionary should not erase raw frames: %+v", c)
			}
			for key, want := range tc.want {
				if c.Metrics[key] != want {
					t.Errorf("%s=%d want %d: %+v", key, c.Metrics[key], want, c)
				}
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			observed := map[string]tracewire.ResourceFrame{}
			for _, line := range strings.Split(string(body), "\n") {
				if record, ok := tracewire.ParseResourceStack(line); ok && record.Frame != nil {
					observed[record.Frame.SourceID.Value] = *record.Frame
				}
			}
			if len(observed) != 8 || tc.name == "invalid_null_long_ambiguous" && (observed["1"].Symbol.Status != "null" || observed["2"].Symbol.Status != "invalid" || observed["8"].Symbol.Status != "ambiguous") {
				t.Fatalf("coverage repaired or truncated unknown source values: %+v", observed)
			}
		})
	}
}

func TestNativeResourceStackCoverageDoesNotClassifyUnreadFrameTable(t *testing.T) {
	items, _ := resourceStackCoverageFixture(t, "ALTER TABLE native_hook DROP COLUMN callchain_id", "CREATE TABLE unrelated_vendor (payload); INSERT INTO unrelated_vendor VALUES ('not exported')")
	found := map[string]bool{}
	for _, item := range items {
		if item.Role == "unsupported_input" {
			found[item.Table] = true
		}
		if item.Family == "resource_stack.frames" {
			t.Fatal("unread frame source should not gain exporter coverage")
		}
	}
	if !found["native_hook_frame"] || !found["unrelated_vendor"] {
		t.Fatalf("blanket classification hid unread source tables: %v", found)
	}
}

func TestNativeResourceStackCoverageInvalidKeysAndMissingOptionalField(t *testing.T) {
	items, _ := resourceStackCoverageFixture(t,
		"INSERT INTO native_hook_frame (id,callchain_id,depth) VALUES (90,NULL,0),(91,-1,0),(92,4294967295,0),(93,x'07',0)",
		"ALTER TABLE native_hook_frame DROP COLUMN symbol_id",
		"ALTER TABLE native_hook_frame RENAME COLUMN depth TO DEPTH")
	c := resourceFrameCoverage(t, items)
	if c.RowsRead != 12 || c.RowsEmitted != 14 || c.Metrics["invalid_callchain_rows"] != 4 || c.Metrics["unreferenced_frame_rows"] != 0 ||
		c.Metrics["symbol_id_unavailable"] != 8 || c.Metrics["symbol_unavailable"] != 8 || c.Metrics["unconverted_columns"] != 0 {
		t.Fatalf("invalid join keys or optional absence became complete source rows: %+v", c)
	}
	if traceDBStringSliceContains(c.ColumnsPresent, "symbol_id") || !traceDBStringSliceContains(c.ColumnsPresent, "depth") {
		t.Fatalf("column coverage did not describe actual SQL projection: %+v", c)
	}
}
