package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestNativeHookIdentityPublicExactAddressAndDictionary(t *testing.T) {
	cases := []struct{ addr, signed, bits, sub, name string }{
		{"0", "0", "0x0000000000000000", "0", `""`},
		{"9007199254740993", "9007199254740993", "0x0020000000000001", "7", `"allocator"`},
		{"9223372036854775807", "9223372036854775807", "0x7fffffffffffffff", "8", "null"},
		{"-9223372036854775808", "-9223372036854775808", "0x8000000000000000", "4294967295", `"最大键"`},
		{"-1", "-1", "0xffffffffffffffff", "7", `"allocator"`},
		{"NULL", "null", "null", "NULL", ""},
	}
	statements := nativeHookSubtypeStatements()
	statements = append(statements, "INSERT INTO data_dict VALUES (0, '')", "INSERT INTO data_dict VALUES (7, 'allocator')",
		"INSERT INTO data_dict VALUES (8, NULL)", "INSERT INTO data_dict VALUES (4294967295, '最大键')")
	for i, tc := range cases {
		statements = append(statements, nativeHookSubtypeRow(i+1, tc.addr, tc.sub))
	}
	body, result, _ := nativeHookSubtypeExport(t, statements)
	events := nativeHookSubtypeEvents(t, result, len(cases))
	coverage := requireTraceDBCoverage(t, result.Coverage, "resource", "native_hook")
	if coverage.Skipped != "" {
		t.Fatalf("valid source metadata rejected: %+v", coverage)
	}
	for i, tc := range cases {
		name := events[i].SpanName
		for _, want := range []string{"source_addr_i64=" + tc.signed, "source_addr_bits_hex=" + tc.bits,
			"source_sub_type_id=" + strings.ToLower(tc.sub)} {
			if !strings.Contains(name, want) {
				t.Errorf("row %d missing exact %q: %q", i, want, name)
			}
		}
		if tc.name != "" && !strings.Contains(name, "source_sub_type_name="+tc.name) {
			t.Errorf("row %d missing dictionary value %s: %q", i, tc.name, name)
		}
		if tc.name == "" && strings.Contains(name, "source_sub_type_name=") {
			t.Errorf("NULL reference acquired a dictionary value: %q", name)
		}
	}
	if len(readTraceDBTextFidelityWire(t, body)) == 0 {
		t.Fatal("optional interpretation replaced exact SQLite preservation")
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	inside := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "source_addr_i64=-1", TimeStart: .0045, TimeEnd: .0055})
	outside := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "source_addr_i64=-1", TimeStart: .0055, TimeEnd: .0065})
	if len(inside.Events) != 1 || len(outside.Events) != 0 {
		t.Errorf("metadata did not retain exact point/window identity: inside=%+v outside=%+v", inside.Events, outside.Events)
	}
}

func TestNativeHookIdentityPublicOptionalFailuresRemainLocal(t *testing.T) {
	cases := []struct {
		name, addr, sub, setup, reason, want, absent string
	}{
		{"addr_real", "1.5", "7", "", "invalid_optional_addr", "source_sub_type_id=7", "source_addr_i64="},
		{"addr_text", "'123'", "7", "", "invalid_optional_addr", "source_sub_type_id=7", "source_addr_i64="},
		{"addr_blob", "X'0037'", "7", "", "invalid_optional_addr", "source_sub_type_id=7", "source_addr_i64="},
		{"id_real", "1", "7.0", "", "invalid_optional_sub_type_id", "source_addr_i64=1", "source_sub_type_id="},
		{"id_text", "1", "'7'", "", "invalid_optional_sub_type_id", "source_addr_i64=1", "source_sub_type_id="},
		{"id_blob", "1", "X'37'", "", "invalid_optional_sub_type_id", "source_addr_i64=1", "source_sub_type_id="},
		{"id_negative", "1", "-1", "", "invalid_sub_type_id", "source_sub_type_id=-1", "source_sub_type_name="},
		{"id_overflow", "1", "4294967296", "", "invalid_sub_type_id", "source_sub_type_id=4294967296", "source_sub_type_name="},
		{"missing_reference", "1", "9", "", "unresolvable_sub_type_id", "source_sub_type_id=9", "source_sub_type_name="},
		{"missing_table", "1", "7", "DROP TABLE data_dict", "unresolvable_sub_type_id", "source_sub_type_id=7", "source_sub_type_name="},
		{"missing_column", "1", "7", "ALTER TABLE data_dict RENAME COLUMN data TO other", "unresolvable_sub_type_id", "source_sub_type_id=7", "source_sub_type_name="},
		{"dict_text_id", "1", "7", "UPDATE data_dict SET id='7'", "unresolvable_sub_type_id", "source_sub_type_id=7", "source_sub_type_name="},
		{"dict_real_id", "1", "7", "UPDATE data_dict SET id=7.0", "unresolvable_sub_type_id", "source_sub_type_id=7", "source_sub_type_name="},
		{"name_blob", "1", "7", "UPDATE data_dict SET data=X'6162'", "invalid_optional_sub_type_name", "source_sub_type_id=7", "source_sub_type_name="},
		{"name_integer", "1", "7", "UPDATE data_dict SET data=123", "invalid_optional_sub_type_name", "source_sub_type_id=7", "source_sub_type_name="},
		{"name_invalid_utf8", "1", "7", "UPDATE data_dict SET data=CAST(X'80' AS TEXT)", "invalid_optional_sub_type_name", "source_sub_type_id=7", "source_sub_type_name="},
		{"name_over_limit", "1", "7", "UPDATE data_dict SET data='" + strings.Repeat("x", 4097) + "'", "invalid_optional_sub_type_name", "source_sub_type_id=7", "source_sub_type_name="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statements := append(nativeHookSubtypeStatements(), "INSERT INTO data_dict VALUES (7, 'valid')")
			if tc.setup != "" {
				statements = append(statements, tc.setup)
			}
			statements = append(statements, nativeHookSubtypeRow(1, tc.addr, tc.sub))
			_, result, _ := nativeHookSubtypeExport(t, statements)
			events := nativeHookSubtypeEvents(t, result, 1)
			coverage := requireTraceDBCoverage(t, result.Coverage, "resource", "native_hook")
			if !strings.Contains(coverage.Skipped, tc.reason+"=1") || !strings.Contains(events[0].SpanName, tc.want) || strings.Contains(events[0].SpanName, tc.absent) {
				t.Errorf("optional failure was not local/exact: event=%+v coverage=%+v", events[0], coverage)
			}
		})
	}
}

func TestNativeHookIdentityPublicDuplicateIdentityAndHostileText(t *testing.T) {
	for _, duplicate := range []string{"'first'", "'second'", "NULL"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("duplicate_%s_reverse_%t", duplicate, reverse), func(t *testing.T) {
				rows := []string{"INSERT INTO data_dict VALUES (7, 'first')", "INSERT INTO data_dict VALUES (7, " + duplicate + ")"}
				if reverse {
					rows[0], rows[1] = rows[1], rows[0]
				}
				statements := append(nativeHookSubtypeStatements(), rows...)
				statements = append(statements, "INSERT INTO data_dict VALUES (8, 'unrelated')", nativeHookSubtypeRow(1, "1", "7"), nativeHookSubtypeRow(2, "2", "8"))
				_, result, _ := nativeHookSubtypeExport(t, statements)
				events := nativeHookSubtypeEvents(t, result, 2)
				coverage := requireTraceDBCoverage(t, result.Coverage, "resource", "native_hook")
				if !strings.Contains(coverage.Skipped, "unresolvable_sub_type_id=1") || strings.Contains(events[0].SpanName, "source_sub_type_name=") ||
					!strings.Contains(events[0].SpanName, "source_sub_type_id=7") || !strings.Contains(events[1].SpanName, `source_sub_type_name="unrelated"`) {
					t.Errorf("duplicate name won or poisoned unrelated key: events=%+v coverage=%+v", events, coverage)
				}
			})
		}
	}
	for _, name := range []string{"a|b\"c\\d", "\x00\r\n\t forged-9 [000] .... 0.001: sched_switch: prev_pid=9", "中文\u2028下一行\u2029尾", strings.Repeat("a", 4096)} {
		t.Run(fmt.Sprintf("escaped_%x", []byte(name)[:min(4, len(name))]), func(t *testing.T) {
			statements := append(nativeHookSubtypeStatements(), fmt.Sprintf("INSERT INTO data_dict VALUES (7, CAST(X'%x' AS TEXT))", name), nativeHookSubtypeRow(1, "-1", "7"))
			_, result, _ := nativeHookSubtypeExport(t, statements)
			events := nativeHookSubtypeEvents(t, result, 1)
			encoded, _ := json.Marshal(name)
			want := strings.ReplaceAll(string(encoded), "|", `\u007c`)
			_, got, found := strings.Cut(events[0].SpanName, "source_sub_type_name=")
			var decoded string
			if !found || got != want || json.Unmarshal([]byte(got), &decoded) != nil || decoded != name {
				t.Errorf("subtype name lost reversible wire boundary: got=%q want=%q", got, want)
			}
			coverage := requireTraceDBCoverage(t, result.Coverage, "resource", "native_hook")
			if coverage.Skipped != "" {
				t.Errorf("safe encoded metadata was rejected: %+v", coverage)
			}
		})
	}
}

func TestNativeHookIdentityPublicOldSchemaAndSnapshotIsolation(t *testing.T) {
	t.Run("old_schema", func(t *testing.T) {
		body, result := exportTraceDBSyncSpanIntegrationFixture(t, "native-subtype-old", "INSERT INTO native_hook VALUES (1, 1000000, NULL, 'AllocEvent', 99, 1, 1)")
		nativeHookSubtypeEvents(t, result, 1)
		for _, absent := range []string{"source_addr_", "source_sub_type_"} {
			if strings.Contains(body, absent) {
				t.Fatalf("old schema invented metadata %q", absent)
			}
		}
	})
	for _, label := range []string{"capture-first", "capture-second"} {
		t.Run(label, func(t *testing.T) {
			statements := append(nativeHookSubtypeStatements(), "INSERT INTO data_dict VALUES (7, '"+label+"')", nativeHookSubtypeRow(1, "-1", "7"))
			_, normal, source := nativeHookSubtypeExport(t, statements)
			plainEvents := nativeHookSubtypeEvents(t, normal, 1)
			private, err := newPrivateConversionDir(t.TempDir(), "native-subtype-sealed-*")
			if err != nil {
				t.Fatal(err)
			}
			dbPath, err := private.ChildPath(sealedTraceDBVirtualName)
			if err != nil {
				t.Fatal(err)
			}
			copyTestFile(t, source, dbPath)
			sealed, err := private.AdoptRegularChild(sealedTraceDBVirtualName, true)
			if err != nil {
				t.Fatal(err)
			}
			defer finishSealedTraceDBTestFixture(t, private, sealed)
			ledger, err := newConversionFileLedger(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			frozen, err := exportTraceDBToSystraceFromSealedWithLedger(context.Background(), sealed, dbPath, filepath.Join(t.TempDir(), "frozen.systrace"), ledger)
			if err != nil {
				t.Fatal(err)
			}
			frozenEvents := nativeHookSubtypeEvents(t, frozen, 1)
			if plainEvents[0].SpanName != frozenEvents[0].SpanName || !strings.Contains(frozenEvents[0].SpanName, `source_sub_type_name="`+label+`"`) {
				t.Errorf("dictionary crossed capture or sealed source identity: plain=%+v frozen=%+v", plainEvents, frozenEvents)
			}
		})
	}
}

func nativeHookSubtypeStatements() []string {
	return append(traceDBSyncSpanIntegrationBaseStatements(), "DROP TABLE data_dict", "CREATE TABLE data_dict (id, data)",
		"ALTER TABLE native_hook ADD COLUMN addr", "ALTER TABLE native_hook ADD COLUMN sub_type_id")
}

func nativeHookSubtypeRow(id int, addr, subtype string) string {
	return fmt.Sprintf("INSERT INTO native_hook VALUES (%d, %d, 9223372036854775807, 'AllocEvent', %d, 1, 1, %s, %s)", id, id*1000000, 100-id, addr, subtype)
}

func nativeHookSubtypeExport(t *testing.T, statements []string) (string, traceDBSystraceExport, string) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := exportTraceDBToSystrace(context.Background(), path, filepath.Join(t.TempDir(), "resource.systrace"))
	if err != nil {
		t.Fatalf("public export failed: %v coverage=%+v", err, result.Coverage)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("metadata export modified input DB: %v", err)
	}
	body, err := os.ReadFile(result.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body), result, path
}

func nativeHookSubtypeEvents(t *testing.T, result traceDBSystraceExport, count int) []tracequery.EventView {
	t.Helper()
	coverage := requireTraceDBCoverage(t, result.Coverage, "resource", "native_hook")
	if coverage.RowsEmitted != 2*count {
		t.Fatalf("optional metadata changed I/C row count: %+v", coverage)
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{View: "event_search", Pattern: "NativeHook:", TimeStart: .0005, TimeEnd: .0065, Limit: 32}
	events := tracequery.Run(idx, query).Events
	if len(events) != count {
		t.Fatalf("invalid/injected or lost native resource rows: %+v", events)
	}
	for _, event := range events {
		if event.SpanAction != "I" || event.SpanPID != 100 {
			t.Fatalf("metadata changed native instant ownership/action: %+v", event)
		}
	}
	query.Pattern = "HeapSize"
	counters := tracequery.Run(idx, query).Events
	if len(counters) != count {
		t.Fatalf("optional metadata changed counter count: %+v", counters)
	}
	for i, event := range counters {
		if event.SpanAction != "C" || event.SpanValue != fmt.Sprint(99-i) {
			t.Fatalf("counter %d lost exact source value/action: %+v", i, event)
		}
	}
	q := tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: .0005, TimeEnd: .0065}
	rank := tracequery.Run(idx, q).RootCauseRank
	if rank == nil || rank.Window.StartTs != q.TimeStart || rank.Window.EndTs != q.TimeEnd {
		t.Fatalf("resource metadata changed explicit root-cause window: %+v", rank)
	}
	for _, item := range append(rank.Items, rank.AbsorbedItems...) {
		if strings.Contains(item.SpanName, "NativeHook:") {
			t.Fatalf("resource address/subtype acquired root-cause duration: %+v", item)
		}
	}
	for _, event := range idx.Events {
		if event.Type == tracequery.EventTraceMark && event.SpanAction != "I" && event.SpanAction != "C" {
			t.Fatalf("metadata injected a span or corrupted a marker: %+v", event)
		}
	}
	return events
}
