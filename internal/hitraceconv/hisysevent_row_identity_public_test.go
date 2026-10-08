package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestHiSysObservationPhysicalRowIdentityPublic(t *testing.T) {
	for _, entry := range []string{"existing_sqlite", "binary_provider"} {
		t.Run(entry, func(t *testing.T) {
			statements := append(traceDBSyncSpanIntegrationBaseStatements(),
				"CREATE TABLE hisys_all_event (id, ts, tid, domain_id, event_name_id, contents)",
				// Identical business fields and repeated declared ids are still different physical rows.
				"INSERT INTO hisys_all_event(rowid,id,ts,tid,contents) VALUES (9007199254740993,7,1000000,100,'same'), (0,7,1000000,100,'same'), (-1,7,1000000,100,'same'), (-9223372036854775808,7,1000000,100,'same'), (9223372036854775807,7,1000000,100,'same')",
				"INSERT INTO hisys_all_event(rowid,ts,contents) VALUES (1,NULL,'bad'), (2,'1000000','bad'), (3,-1,'bad')",
				"CREATE INDEX hisys_reversed ON hisys_all_event(ts, id DESC, contents DESC)")
			source := createTraceDBFixture(t, statements)
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			opts := Options{InputPath: source, OutputPath: filepath.Join(dir, "out.systrace")}
			var result Result
			if entry == "binary_provider" {
				opts.InputPath = filepath.Join(dir, "capture.htrace")
				if err = os.WriteFile(opts.InputPath, []byte("modern profiler payload"), 0o600); err != nil {
					t.Fatal(err)
				}
				opts.TraceEngine, opts.TraceStreamerPath = traceEngineTraceStreamer, writeFakeTraceStreamer(t, dir, 0)
				t.Setenv("TRACE_STREAMER_FIXTURE_DB", source)
				result, err = ConvertFile(context.Background(), opts)
			} else {
				result, err = PrepareExistingTraceDB(context.Background(), opts)
			}
			if err != nil {
				t.Fatal(err)
			}
			q := tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}, TimeStart: .001, TimeEnd: .001001}
			idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			indexed := tracequery.Run(idx, q)
			want := []string{"-9223372036854775808", "-1", "0", "9007199254740993", "9223372036854775807"}
			for name, events := range map[string][]tracequery.EventView{"index": indexed.Events} {
				var got []string
				for _, e := range events {
					if e.PID != 0 || e.TGID != 0 || e.CPU != -1 {
						t.Fatalf("row identity became a scheduler emitter: %+v", e)
					}
					encoded, err := json.Marshal(e.HiSysEvent)
					if err != nil {
						t.Fatal(err)
					}
					var fields map[string]any
					if err = json.Unmarshal(encoded, &fields); err != nil {
						t.Fatal(err)
					}
					id, _ := fields["source_rowid"].(string)
					got = append(got, id)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s lost exact physical row identity/order: got %q want %q", name, got, want)
				}
			}
			c := coverageForTable(result.TraceDBCoverage, "hisys_all_event")
			if c == nil || c.RowsRead != 8 || c.RowsEmitted != 5 || !strings.Contains(c.Skipped, "invalid_timestamp=3") || c.FieldSources["stable_identity"] != "hisys_all_event.hidden_rowid" {
				t.Fatalf("identity/time quality coverage lost: %+v", c)
			}
			after, err := os.ReadFile(source)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("source database mutated")
			}
		})
	}
}

func TestHiSysObservationShadowedAliasesRetainSemanticRows(t *testing.T) {
	source := createTraceDBFixture(t, []string{
		"CREATE TABLE hisys_all_event(rowid TEXT,_rowid_ TEXT,oid TEXT,ts,tid,domain_id,event_name_id,contents)",
		"INSERT INTO hisys_all_event(rowid,_rowid_,oid,ts,contents) VALUES ('1','2','3',1000000,'same'),('1','2','3',1000000,'same')",
	})
	tdb, err := openTraceDB(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	c, err := exportTraceDBHiSysEvent(context.Background(), tdb, sink, traceDBThreadIndex{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.RowsRead != 2 || c.RowsEmitted != 2 || !strings.Contains(c.Skipped, "stable_row_identity_unavailable=2") {
		t.Fatalf("semantic observations dropped because identity was unavailable: %+v", c)
	}
	var out bytes.Buffer
	if _, err = sink.prepareAndWriteForTest(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "rows.systrace")
	if err = os.WriteFile(path, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := tracequery.StreamEventSearch(context.Background(), path, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}})
	if err != nil || len(r.Events) != 2 {
		t.Fatalf("stream query lost observations: %+v, %v", r, err)
	}
	for _, e := range r.Events {
		if e.HiSysEvent == nil || e.HiSysEvent.SourceRowID != nil {
			t.Fatalf("declared alias or scan ordinal minted physical identity: %+v", e)
		}
	}
}

func TestHiSysObservationRowIDUnavailableKeepsPublicEvents(t *testing.T) {
	for _, tc := range []struct{ name, schema, insert, identity string }{
		{"declared_rowid", "CREATE TABLE hisys_all_event(rowid TEXT, ts,tid,domain_id,event_name_id,contents)", "INSERT INTO hisys_all_event(_rowid_,rowid,ts,contents) VALUES (0,'fake',1000000,'same'),(-1,'fake',1000000,'same')", "hisys_all_event.hidden__rowid_"},
		{"generated_virtual_rowid", "CREATE TABLE hisys_all_event(rowid INTEGER GENERATED ALWAYS AS(7) VIRTUAL,ts,tid,domain_id,event_name_id,contents)", "INSERT INTO hisys_all_event(_rowid_,ts,contents) VALUES (0,1000000,'same'),(-1,1000000,'same')", "hisys_all_event.hidden__rowid_"},
		{"generated_stored_ROWID", "CREATE TABLE hisys_all_event(ROWID INTEGER GENERATED ALWAYS AS(7) STORED,ts,tid,domain_id,event_name_id,contents)", "INSERT INTO hisys_all_event(_rowid_,ts,contents) VALUES (0,1000000,'same'),(-1,1000000,'same')", "hisys_all_event.hidden__rowid_"},
		{"all_aliases_shadowed", "CREATE TABLE hisys_all_event(rowid TEXT,_rowid_ TEXT,oid TEXT,ts,tid,domain_id,event_name_id,contents)", "INSERT INTO hisys_all_event(rowid,_rowid_,oid,ts,contents) VALUES ('1','2','3',1000000,'same'),('1','2','3',1000000,'same')", ""},
		{"all_aliases_generated", "CREATE TABLE hisys_all_event(rowid INTEGER GENERATED ALWAYS AS(7) VIRTUAL,_ROWID_ INTEGER GENERATED ALWAYS AS(8) STORED,OID INTEGER GENERATED ALWAYS AS(9) VIRTUAL,ts,tid,domain_id,event_name_id,contents)", "INSERT INTO hisys_all_event(ts,contents) VALUES (1000000,'same'),(1000000,'same')", ""},
		{"without_rowid", "CREATE TABLE hisys_all_event(id INTEGER PRIMARY KEY,ts,tid,domain_id,event_name_id,contents) WITHOUT ROWID", "INSERT INTO hisys_all_event(id,ts,contents) VALUES (0,1000000,'same'),(-1,1000000,'same')", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := createTraceDBFixture(t, append(traceDBSyncSpanIntegrationBaseStatements(), tc.schema, tc.insert))
			r, err := PrepareExistingTraceDB(context.Background(), Options{InputPath: source, OutputPath: filepath.Join(t.TempDir(), "out.systrace")})
			if strings.HasPrefix(tc.name, "all_aliases_") {
				// Existing whole-table fidelity refuses this shape; HiSys must not
				// weaken that transaction-wide losslessness boundary.
				if err == nil || !strings.Contains(err.Error(), "trace_db_text_fidelity_rowid_alias_shadowed") {
					t.Fatalf("lost existing fidelity boundary: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndex(context.Background(), r.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			got := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}})
			if len(got.Events) != 2 {
				t.Fatalf("unavailable identity must not remove observations: %+v", got)
			}
			c := coverageForTable(r.TraceDBCoverage, "hisys_all_event")
			if tc.identity != "" {
				if c.FieldSources["stable_identity"] != tc.identity {
					t.Fatalf("shadowed alias not bypassed: %+v", c)
				}
				for i, e := range got.Events {
					want := int64(i - 1)
					if e.HiSysEvent == nil || e.HiSysEvent.SourceRowID == nil || *e.HiSysEvent.SourceRowID != want {
						t.Fatalf("declared/generated alias replaced real hidden rowid: %+v", e.HiSysEvent)
					}
				}
			} else {
				if !strings.Contains(c.Skipped, "stable_row_identity_unavailable=2") {
					t.Fatalf("unknown identity not disclosed: %+v", c)
				}
				for _, e := range got.Events {
					b, _ := json.Marshal(e.HiSysEvent)
					if strings.Contains(string(b), "source_rowid") {
						t.Fatalf("scan ordinal or declared id invented as physical row identity: %s", b)
					}
				}
			}
		})
	}
}
