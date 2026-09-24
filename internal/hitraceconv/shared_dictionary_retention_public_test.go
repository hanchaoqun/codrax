package hitraceconv

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestReferencedDictionaryBothPublicIntakesPreserveConsumerOutput(t *testing.T) {
	for _, intake := range []string{"existing_sqlite", "binary_through_streamer"} {
		t.Run(intake, func(t *testing.T) {
			if intake == "binary_through_streamer" && runtime.GOOS == "windows" {
				t.Skip("external TraceStreamer fixture uses a POSIX shell")
			}
			statements := append(traceDBSyncSpanIntegrationBaseStatements(),
				"INSERT INTO data_dict VALUES (0,'BOOTSTRAP'), (8,'EVENT'), (9,'SYS'), (60,'caller'), (61,'cache_read'), (7,'duplicate'), (7,'duplicate')",
				"WITH RECURSIVE n(x) AS (SELECT 1000 UNION ALL SELECT x+1 FROM n WHERE x<1999) INSERT INTO data_dict SELECT x, printf('unreferenced_%d',x) FROM n",
				"CREATE TABLE args (argset,key,datatype,value)", "INSERT INTO args VALUES (0,60,1,61)",
				"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT, argset INT)",
				"INSERT INTO app_startup VALUES (2000000,3000000,0,1),(4000000,5000000,5,1),(6000000,7000000,7,1)",
				"CREATE TABLE hisys_all_event (ts INT,tid INT,domain_id,event_name_id,contents TEXT)",
				"INSERT INTO hisys_all_event VALUES (3000000,100,9,8,'phase=ready'),(5000000,100,9,0,'phase=bootstrap')",
				"INSERT INTO sched_slice VALUES (0,8000000,1,1,'S',120),(8000000,1000000,1,2,'S',120)")
			dbPath := createTraceDBFixture(t, statements)
			dbBefore, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			opts := existingTraceDBOptions(t, dbPath)
			var result Result
			var binaryBefore []byte
			if intake == "existing_sqlite" {
				result, err = PrepareExistingTraceDB(t.Context(), opts)
			} else {
				// A genuine format-valid binary capture enters the real input and
				// conversion path. Only the external executable's DB is a fixture.
				binaryBefore = traceDBRawExactRecoveryCapture(t)
				opts.InputPath = filepath.Join(t.TempDir(), "capture.sys")
				if err = os.WriteFile(opts.InputPath, binaryBefore, 0o600); err != nil {
					t.Fatal(err)
				}
				opts.TraceEngine = traceEngineTraceStreamer
				opts.TraceStreamerPath = writeFakeTraceStreamer(t, t.TempDir(), 0)
				t.Setenv("TRACE_STREAMER_FIXTURE_DB", dbPath)
				result, err = ConvertFile(t.Context(), opts)
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(result.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"AppStartup:BOOTSTRAP", "AppStartup:coldStart", "AppStartup:startup", "print: SYS/EVENT: phase=ready", "print: SYS/BOOTSTRAP: phase=bootstrap"} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("retained consumer lost %q", want)
				}
			}
			seenCore, seenExtended := false, false
			for _, c := range result.TraceDBCoverage {
				if c.Family != "resolver" || c.Table != "data_dict" || c.Metrics == nil {
					continue
				}
				if c.RowsEmitted != 1006 {
					t.Fatalf("global valid population changed: %+v", c)
				}
				switch c.Metrics["dictionary_entries_retained"] {
				case 2:
					seenCore = true
				case 4:
					seenExtended = true
				default:
					t.Fatalf("dictionary retained unrelated names: %+v", c)
				}
			}
			if !seenCore || !seenExtended {
				t.Fatalf("both consumer resolvers were not exercised: core=%t extended=%t", seenCore, seenExtended)
			}
			idx, err := tracequery.BuildIndex(t.Context(), result.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			inside := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventHiSystemEvent}, TimeStart: .002, TimeEnd: .006, TimeStartSet: true, TimeEndSet: true})
			if len(inside.Events) != 2 {
				t.Fatalf("public query lost resolved system events: %+v", inside.Events)
			}
			after, err := os.ReadFile(dbPath)
			if err != nil || !bytes.Equal(after, dbBefore) {
				t.Fatalf("input database changed: %v", err)
			}
			if binaryBefore != nil {
				after, err = os.ReadFile(opts.InputPath)
				if err != nil || !bytes.Equal(after, binaryBefore) {
					t.Fatalf("input binary changed: %v", err)
				}
			}
		})
	}
}
