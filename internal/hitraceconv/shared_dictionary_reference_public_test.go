package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestSharedDictionaryReferencePublicStoredClassesStayDistinct(t *testing.T) {
	for field, column := range []string{"start_name", "domain_id", "event_name_id"} {
		for _, tc := range []struct{ literal, storage string }{{"NULL", "null"}, {"'0'", "text"}, {"0.0", "real"}, {"X'30'", "blob"}} {
			t.Run(column+"/"+tc.storage, func(t *testing.T) {
				refs := [3]string{"0", "0", "0"}
				refs[field] = tc.literal
				body, result := dictionaryReferencePublicConvert(t, refs, false)
				table, family, ordinal := "hisys_all_event", "log", uint64(2)
				if field == 0 {
					table, family, ordinal = "app_startup", "slice", 1
					if strings.Contains(body, "AppStartup:ZERO") || !strings.Contains(body, "AppStartup:startup") {
						t.Error("non-INTEGER startup reference borrowed the valid zero key or lost its unnamed span")
					}
				} else if strings.Contains(body, "print: ZERO/ZERO: candidate-row") {
					t.Error("non-INTEGER HiSys reference borrowed the valid zero key")
				}
				coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, family, table)
				if coverage.Error != "" || coverage.Skipped == "" {
					t.Errorf("invalid reference needs a local, nonfatal diagnostic: %+v", coverage)
				}
				cell := dictionaryReferencePublicCell(t, body, table, column, ordinal)
				if cell.Storage != tc.storage {
					t.Fatalf("SQL fidelity changed the stored reference class: %+v want=%s", cell, tc.storage)
				}
				if tc.storage == "text" || tc.storage == "blob" {
					if got := string(traceDBTextFidelityDecodedBytes(t, cell)); got != "0" {
						t.Fatalf("raw reference bytes lost: %q", got)
					}
				}
			})
		}
	}
}

func TestSharedDictionaryReferencePublicIntegerCompatibility(t *testing.T) {
	for _, tc := range []struct {
		literal, integer string
		affinity         bool
	}{
		{"0", "0", false}, {"4294967296", "4294967296", false},
		{"-9223372036854775808", "-9223372036854775808", false}, {"9223372036854775807", "9223372036854775807", false},
		{"'0'", "0", true},
	} {
		t.Run(fmt.Sprintf("%s_affinity_%t", tc.literal, tc.affinity), func(t *testing.T) {
			rows := []string{}
			if tc.integer != "0" {
				rows = append(rows, "INSERT INTO data_dict VALUES ("+tc.integer+", 'ZERO')")
			}
			body, result := dictionaryReferencePublicConvert(t, [3]string{tc.literal, tc.literal, tc.literal}, tc.affinity, rows...)
			for _, want := range []string{"AppStartup:ZERO", "print: ZERO/ZERO: candidate-row"} {
				if !strings.Contains(body, want) {
					t.Errorf("legal stored INTEGER reference lost %q", want)
				}
			}
			for _, item := range []struct{ family, table string }{{"slice", "app_startup"}, {"log", "hisys_all_event"}} {
				coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, item.family, item.table)
				if coverage.Error != "" || coverage.Skipped != "" {
					t.Errorf("legal reference was rejected: %+v", coverage)
				}
			}
			for _, item := range []struct {
				table, column string
				ordinal       uint64
			}{{"app_startup", "start_name", 1}, {"hisys_all_event", "domain_id", 2}, {"hisys_all_event", "event_name_id", 2}} {
				cell := dictionaryReferencePublicCell(t, body, item.table, item.column, item.ordinal)
				if cell.Storage != "integer" || cell.Integer != tc.integer {
					t.Fatalf("actual INTEGER storage was narrowed/coerced: %+v want=%s", cell, tc.integer)
				}
			}
		})
	}
}

func TestSharedDictionaryReferencePublicUnresolvedOrNonwireNamesRemainLocal(t *testing.T) {
	for _, tc := range []struct{ name, mutation, startup string }{
		{"missing", "DELETE FROM data_dict WHERE id=0", "startup"},
		{"duplicate", "INSERT INTO data_dict VALUES (0, 'OTHER')", "startup"},
		{"empty_text", "UPDATE data_dict SET data='' WHERE id=0", "startup"},
		{"nonwire_text", "UPDATE data_dict SET data='not-a-wire-name' WHERE id=0", "not-a-wire-name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, result := dictionaryReferencePublicConvert(t, [3]string{"0", "0", "0"}, false, tc.mutation)
			if !strings.Contains(body, "AppStartup:"+tc.startup) || strings.Contains(body, "candidate-row\n") {
				t.Error("unresolved name either lost startup fallback or leaked an unencoded HiSys row")
			}
			coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "log", "hisys_all_event")
			if coverage.Error != "" || coverage.RowsEmitted != 2 || coverage.Skipped == "" {
				t.Errorf("HiSys name failure was not isolated from the healthy record: %+v", coverage)
			}
			if cell := dictionaryReferencePublicCell(t, body, "hisys_all_event", "contents", 2); string(traceDBTextFidelityDecodedBytes(t, cell)) != "candidate-row" {
				t.Fatalf("rejected semantic row was lost from SQL fidelity: %+v", cell)
			}
			if tc.name == "duplicate" {
				coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "resolver", "data_dict")
				if !strings.Contains(coverage.Skipped, "duplicate_id=1") {
					t.Fatalf("global ambiguous dictionary diagnostic changed: %+v", coverage)
				}
			}
		})
	}
}

func TestSharedDictionaryReferencePublicHiSysNamesAreWholeFields(t *testing.T) {
	for _, tc := range []struct {
		column, name string
		valid        bool
	}{
		{"domain_id", "A_1", true}, {"event_name_id", "A_1", true},
		{"domain_id", "SYS/EVENT: forged", false}, {"event_name_id", "EVENT: forged", false},
	} {
		t.Run(tc.column+"/"+tc.name, func(t *testing.T) {
			refs := [3]string{"0", "9", "8"}
			domain, event := "SYS", "EVENT"
			if tc.column == "domain_id" {
				refs[1], domain = "10", tc.name
			} else {
				refs[2], event = "10", tc.name
			}
			body, result := dictionaryReferencePublicConvert(t, refs, false, "INSERT INTO data_dict VALUES (10, '"+tc.name+"')")
			idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			candidate := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}, TimeStart: .0064, TimeEnd: .0066})
			coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "log", "hisys_all_event")
			if tc.valid {
				if len(candidate.Events) != 1 || candidate.Events[0].PluginFields == nil || candidate.Events[0].PluginFields.Domain != domain || candidate.Events[0].PluginFields.EventName != event || coverage.Skipped != "" {
					t.Fatalf("valid whole-field identity changed: events=%+v coverage=%+v", candidate.Events, coverage)
				}
			} else if len(candidate.Events) != 1 || candidate.Events[0].PluginFields == nil || candidate.Events[0].HiSysEvent == nil || candidate.Events[0].Domain != domain || candidate.Events[0].PluginFields.EventName != event || coverage.RowsEmitted != 2 || coverage.Skipped == "" {
				t.Fatalf("reversible wire failed to preserve the complete non-print name: events=%+v coverage=%+v", candidate.Events, coverage)
			}
			if cell := dictionaryReferencePublicCell(t, body, "data_dict", "data", 5); string(traceDBTextFidelityDecodedBytes(t, cell)) != tc.name {
				t.Fatalf("raw dictionary name was changed to fit semantic wire grammar: %+v", cell)
			}
		})
	}
}

func TestSharedDictionaryReferencePublicBadNameCannotHideMalformedRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("existing external fixture exporter uses /bin/sh")
	}
	for _, tc := range []struct{ name, row, reason string }{
		{"null_name_negative_time", "(-1, 100, NULL, 8, 'payload')", "invalid_timestamp"},
		{"nonwire_name_negative_tid", "(1000000, -1, 5, 8, 'payload')", "invalid_tid"},
		{"null_name_invalid_utf8_contents", "(1000000, 100, NULL, 8, CAST(X'FF' AS TEXT))", "invalid_body"},
		{"nonwire_name_oversize_contents", "(1000000, 100, 5, 8, replace(hex(zeroblob(600000)), '0', 'x'))", "line_too_long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statements := append(traceDBSyncSpanIntegrationBaseStatements(),
				"INSERT INTO data_dict VALUES (8, 'EVENT')",
				"CREATE TABLE hisys_all_event (ts INT, tid INT, domain_id, event_name_id, contents TEXT)",
				"INSERT INTO hisys_all_event VALUES "+tc.row)
			fixtureDB := createTraceDBFixture(t, statements)
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.htrace")
			if err := os.WriteFile(input, []byte("modern profiler payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
			_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: filepath.Join(dir, "capture.systrace"), TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: writeFakeTraceStreamer(t, dir, 0)})
			if reason, ok := traceDBOutputInvariantReason(err); !ok || reason != tc.reason {
				t.Fatalf("name-reference rejection hid the original record failure: reason=%q typed=%t want=%q err=%v", reason, ok, tc.reason, err)
			}
		})
	}
}

// Only the external executable is replaced. The real converter exports,
// seals, normalizes and postvalidates the DB before real query consumption.
func dictionaryReferencePublicConvert(t *testing.T, refs [3]string, affinity bool, mutations ...string) (string, Result) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("existing external fixture exporter uses /bin/sh")
	}
	decl := ""
	if affinity {
		decl = " INTEGER"
	}
	statements := append(traceDBSyncSpanIntegrationBaseStatements(),
		"DROP TABLE data_dict", "CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (0, 'ZERO'), (8, 'EVENT'), (9, 'SYS'), ('unrelated-id', 'UNRELATED')",
		"DROP TABLE app_startup", "CREATE TABLE app_startup (start_time INT, end_time INT, start_name"+decl+", ipid INT)",
		"CREATE TABLE hisys_all_event (ts INT, tid INT, domain_id"+decl+", event_name_id"+decl+", contents TEXT)")
	statements = append(statements, mutations...)
	statements = append(statements,
		"INSERT INTO app_startup VALUES (2000000, 3000000, "+refs[0]+", 1), (4000000, 5000000, 8, 1)",
		"INSERT INTO hisys_all_event VALUES (6000000, 100, 9, 8, 'healthy-row'), (6500000, 100, "+refs[1]+", "+refs[2]+", 'candidate-row')",
		"INSERT INTO native_hook VALUES (1, 7000000, 0, 'malloc', 4096, 1, 1)",
		"UPDATE thread_state SET dur=8000000 WHERE itid=1", "UPDATE thread_state SET ts=8000000, dur=1000000, cpu=1 WHERE itid=2",
		"INSERT INTO sched_slice VALUES (0, 8000000, 1, 1, 'S', 120), (8000000, 1000000, 1, 2, 'S', 120)")
	fixtureDB := createTraceDBFixture(t, statements)
	before, err := os.ReadFile(fixtureDB)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	input, output := filepath.Join(dir, "capture.htrace"), filepath.Join(dir, "capture.systrace")
	payload := []byte("modern profiler payload")
	if err := os.WriteFile(input, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: writeFakeTraceStreamer(t, dir, 0)})
	if err != nil {
		t.Fatalf("actual conversion failed instead of isolating a name reference: %v", err)
	}
	for path, want := range map[string][]byte{input: payload, fixtureDB: before} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("source bytes changed: %s error=%v", path, err)
		}
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sched_switch:", "NativeHook:AllocEvent", "HeapSize|4096", "AppStartup:EVENT", "print: SYS/EVENT: healthy-row"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("healthy semantic output lost %q", want)
		}
	}
	coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "resolver", "data_dict")
	if coverage.Error != "" || !strings.Contains(coverage.Skipped, "invalid_id=1") {
		t.Fatalf("unchanged global loader diagnostic was lost: %+v", coverage)
	}
	idx, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	inside := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "NativeHook:", TimeStart: .006, TimeEnd: .008})
	outside := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "NativeHook:", TimeStart: .008, TimeEnd: .009})
	healthy := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}, TimeStart: .006, TimeEnd: .0062})
	if len(inside.Events) != 1 || len(outside.Events) != 0 || len(healthy.Events) != 1 || healthy.Events[0].PluginFields == nil || healthy.Events[0].PluginFields.Domain != "SYS" || healthy.Events[0].PluginFields.EventName != "EVENT" {
		t.Fatalf("healthy real query/window lost native/HiSys identity: inside=%+v outside=%+v healthy=%+v", inside.Events, outside.Events, healthy.Events)
	}
	return string(body), result
}

func dictionaryReferencePublicCell(t *testing.T, body, table, column string, ordinal uint64) traceDBTextFidelityCell {
	t.Helper()
	records := readTraceDBTextFidelityWire(t, body)
	tableID, columnIndex := -1, -1
	for key, record := range records {
		if key.Kind != "schema" {
			continue
		}
		var schema traceDBTextFidelitySchema
		if err := json.Unmarshal(traceDBTextFidelityWirePayload(t, key, record), &schema); err != nil {
			t.Fatal(err)
		}
		if string(traceDBTextFidelityDecodedBytes(t, schema.Table)) == table {
			tableID = schema.TableID
			for i, field := range schema.Columns {
				if string(traceDBTextFidelityDecodedBytes(t, field.Name)) == column {
					columnIndex = i
				}
			}
		}
	}
	for key, record := range records {
		if key.Kind == "row" && key.TableID == tableID && key.Ordinal == ordinal {
			var row traceDBTextFidelityRow
			if err := json.Unmarshal(traceDBTextFidelityWirePayload(t, key, record), &row); err != nil {
				t.Fatal(err)
			}
			if columnIndex >= 0 && columnIndex < len(row.Cells) {
				return row.Cells[columnIndex]
			}
		}
	}
	t.Fatalf("exact SQL fidelity cell missing: %s.%s row=%d", table, column, ordinal)
	return traceDBTextFidelityCell{}
}
