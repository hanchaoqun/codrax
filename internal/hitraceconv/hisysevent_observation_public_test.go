package hitraceconv

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestHiSysObservationPublicUnknownNamesRemainTimedEvents(t *testing.T) {
	for _, entry := range []string{"existing_sqlite", "binary_provider"} {
		t.Run(entry, func(t *testing.T) {
			statements := append(traceDBSyncSpanIntegrationBaseStatements(),
				"INSERT INTO data_dict VALUES (81, '域/组件'), (82, '')",
				"CREATE TABLE hisys_all_event (ts INT, tid INT, domain_id, event_name_id, contents)",
				"INSERT INTO hisys_all_event VALUES (0, 0, NULL, 81, 'zero'), (1000001, 100, 81, 82, 'first' || char(10) || 'second'), (2000000, 100, 99999, 81, NULL)")
			fixture := createTraceDBFixture(t, statements)
			before, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			opts := Options{InputPath: fixture, OutputPath: filepath.Join(dir, "capture.systrace")}
			var result Result
			if entry == "binary_provider" {
				opts.InputPath = filepath.Join(dir, "capture.htrace")
				if err := os.WriteFile(opts.InputPath, []byte("modern profiler payload"), 0o600); err != nil {
					t.Fatal(err)
				}
				opts.TraceEngine, opts.TraceStreamerPath = traceEngineTraceStreamer, writeFakeTraceStreamer(t, dir, 0)
				t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixture)
				result, err = ConvertFile(context.Background(), opts)
			} else {
				result, err = PrepareExistingTraceDB(context.Background(), opts)
			}
			if err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			got := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}, TimeStart: 0, TimeStartSet: true, TimeEnd: .003})
			if len(got.Events) != 3 {
				t.Fatalf("actual SQL event population lost: got %d, want 3: %+v", len(got.Events), got.Events)
			}
			if row := got.Events[0].HiSysEvent; row == nil || row.TimestampNS != 0 || row.SourceTID == nil || *row.SourceTID != 0 || row.Domain.Status != "null_reference" {
				t.Fatalf("real zero/NULL identity lost: %+v", row)
			}
			if row := got.Events[1].HiSysEvent; row == nil || row.TimestampNS != 1000001 || row.Event.Name == nil || *row.Event.Name != "" || row.Event.Status != "resolved" || row.Contents.Text == nil || *row.Contents.Text != "first\nsecond" {
				t.Fatalf("precise row semantics lost: %+v", row)
			}
			if row := got.Events[2].HiSysEvent; row == nil || row.Domain.Status != "unresolved_reference" || row.Contents.StorageClass != "null" || row.Contents.Text != nil {
				t.Fatalf("unknown identity/content lost: %+v", row)
			}
			after, err := os.ReadFile(fixture)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("source database mutated")
			}
		})
	}
}

func TestHiSysObservationPublicNullTIDAndWhitespaceStayExact(t *testing.T) {
	statements := append(traceDBSyncSpanIntegrationBaseStatements(),
		"INSERT INTO data_dict VALUES (81, 'SYS'), (82, 'EVENT')",
		"CREATE TABLE hisys_all_event (ts INT, tid INT, domain_id, event_name_id, contents)",
		"INSERT INTO hisys_all_event VALUES (1000000, NULL, 81, 82, 'known'), (1100000, 100, 81, 82, '  padded  ')")
	result, err := PrepareExistingTraceDB(context.Background(), Options{InputPath: createTraceDBFixture(t, statements), OutputPath: filepath.Join(t.TempDir(), "out.systrace")})
	if err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	r := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}})
	if len(r.Events) != 2 || r.Events[0].HiSysEvent == nil || r.Events[0].HiSysEvent.SourceTID != nil || r.Events[1].HiSysEvent == nil || *r.Events[1].HiSysEvent.Contents.Text != "  padded  " {
		t.Fatalf("NULL/whitespace collapsed: %+v", r.Events)
	}
}

func TestHiSysObservationPublicContentStorageAndLegacyPrint(t *testing.T) {
	statements := append(traceDBSyncSpanIntegrationBaseStatements(),
		"INSERT INTO data_dict VALUES (81, 'SYS'), (82, 'EVENT')",
		"CREATE TABLE hisys_all_event (ts INT, tid INT, domain_id, event_name_id, contents)",
		"INSERT INTO hisys_all_event VALUES (1000000, 100, 81, 82, 'unchanged'), (1100000, NULL, 81, 82, NULL), (1200000, 0, NULL, 82, ''), (1300000, 100, 81, 82, 'row' || char(10) || 'print: SYS/FAKE: injected' || char(13) || char(0)), (1400000,100,81,82,X'00FF'), (1500000,100,81,82,0), (1600000,100,81,82,1.25)")
	result, err := PrepareExistingTraceDB(context.Background(), Options{InputPath: createTraceDBFixture(t, statements), OutputPath: filepath.Join(t.TempDir(), "out.systrace")})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "print: SYS/EVENT: unchanged\n") || strings.Contains(string(body), "print: SYS/FAKE: injected") {
		t.Fatal("legacy bytes changed or encoded payload injected a row")
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	r := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}, TimeStart: .0009, TimeEnd: .0017})
	if len(r.Events) != 7 {
		t.Fatalf("storage population=%d", len(r.Events))
	}
	if r.Events[0].HiSysEvent != nil || r.Events[0].Domain != "SYS" || r.Events[0].PluginFields.EventName != "EVENT" {
		t.Fatal("legacy print path replaced")
	}
	for i, want := range []string{"null", "text", "text", "blob", "integer", "real"} {
		if r.Events[i+1].HiSysEvent == nil || r.Events[i+1].HiSysEvent.Contents.StorageClass != want {
			t.Fatalf("storage[%d] not %s", i, want)
		}
	}
	if r.Events[1].HiSysEvent.SourceTID != nil || r.Events[2].HiSysEvent.Contents.Text == nil || *r.Events[2].HiSysEvent.Contents.Text != "" {
		t.Fatal("NULL/empty lost")
	}
	if *r.Events[3].HiSysEvent.Contents.Text != "row\nprint: SYS/FAKE: injected\r\x00" {
		t.Fatal("special bytes changed")
	}
	if r.Events[4].HiSysEvent.Contents.BytesBase64 != base64.StdEncoding.EncodeToString([]byte{0, 255}) || *r.Events[5].HiSysEvent.Contents.Text != "0" || *r.Events[6].HiSysEvent.Contents.Text != "1.25" {
		t.Fatal("typed storage values changed")
	}
}
