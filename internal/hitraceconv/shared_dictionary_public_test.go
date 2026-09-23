package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Exercise the actual conversion boundary; only the external TraceStreamer
// executable is replaced by the existing fixture exporter. Dictionary rows
// must not decide whether unrelated scheduler/resource families are published.
func TestSharedDictionaryPublicUnrelatedBadIDsRemainLocal(t *testing.T) {
	for _, key := range []string{"'not-an-id'", "1.5", "5.0", "'5'", "X'35'", "NULL"} {
		t.Run(key, func(t *testing.T) {
			body, result := sharedDictionaryPublicConvert(t, "5",
				"INSERT INTO data_dict VALUES (5, 'coldStart')",
				fmt.Sprintf("INSERT INTO data_dict VALUES (%s, 'bad-key-name')", key))
			for _, want := range []string{"AppStartup:coldStart"} {
				if !strings.Contains(body, want) {
					t.Errorf("unrelated dictionary row changed valid name %q", want)
				}
			}
			if strings.Contains(body, "bad-key-name") {
				t.Fatal("non-INTEGER key acquired an INTEGER reference")
			}
			coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "resolver", "data_dict")
			if coverage.Error != "" || !strings.Contains(coverage.Skipped, "invalid_id=1") {
				t.Fatalf("bad key was not disclosed locally: %+v", coverage)
			}
		})
	}
}

func TestSharedDictionaryPublicAmbiguousNamesNeverWin(t *testing.T) {
	for _, value := range []string{"NULL", "X'636F6C645374617274'", "123", "1.5", "'conflicting-name'", "'coldStart'"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_reverse_%t", value, reverse), func(t *testing.T) {
				rows := []string{"INSERT INTO data_dict VALUES (5, 'coldStart')", "INSERT INTO data_dict VALUES (5, " + value + ")"}
				if reverse {
					rows[0], rows[1] = rows[1], rows[0]
				}
				body, result := sharedDictionaryPublicConvert(t, "5", rows...)
				if !strings.Contains(body, "AppStartup:startup") {
					t.Fatal("ambiguous key won a name")
				}
				for _, forbidden := range []string{"AppStartup:coldStart", "AppStartup:conflicting-name"} {
					if strings.Contains(body, forbidden) {
						t.Errorf("duplicate resolver identity leaked %q", forbidden)
					}
				}
				coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "resolver", "data_dict")
				if coverage.Error != "" || !strings.Contains(coverage.Skipped, "duplicate_id=1") {
					t.Fatalf("duplicate not disclosed: %+v", coverage)
				}
			})
		}
	}
}

func TestSharedDictionaryPublicNonTextNamesStayAbsent(t *testing.T) {
	for _, value := range []string{"NULL", "X'636F6C645374617274'", "123", "1.5"} {
		t.Run(value, func(t *testing.T) {
			body, result := sharedDictionaryPublicConvert(t, "5", "INSERT INTO data_dict VALUES (5, "+value+")")
			if !strings.Contains(body, "AppStartup:startup") {
				t.Fatal("non-TEXT value became a display name")
			}
			coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "resolver", "data_dict")
			if coverage.Error != "" || !strings.Contains(coverage.Skipped, "invalid_value=1") {
				t.Fatalf("non-TEXT value not disclosed: %+v", coverage)
			}
		})
	}
}

func TestSharedDictionaryPublicPreservesIntegerCompatibility(t *testing.T) {
	for _, key := range []string{"0", "4294967296", "9223372036854775807", "-9223372036854775808"} {
		t.Run(key, func(t *testing.T) {
			body, result := sharedDictionaryPublicConvert(t, key, "INSERT INTO data_dict VALUES ("+key+", '启动标签')")
			if !strings.Contains(body, "AppStartup:启动标签") {
				t.Fatal("shared INTEGER/TEXT compatibility was narrowed to a native subtype profile")
			}
			coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "resolver", "data_dict")
			if coverage.Error != "" || coverage.Skipped != "" {
				t.Fatalf("valid INTEGER/TEXT row was rejected: %+v", coverage)
			}
		})
	}
}

func sharedDictionaryPublicConvert(t *testing.T, key string, dictionaryRows ...string) (string, Result) {
	if runtime.GOOS == "windows" {
		t.Skip("existing external TraceStreamer fixture uses /bin/sh; loader tests remain platform-neutral")
	}
	t.Helper()
	statements := append(traceDBSyncSpanIntegrationBaseStatements(), "DROP TABLE data_dict", "CREATE TABLE data_dict (id, data)")
	statements = append(statements, dictionaryRows...)
	statements = append(statements,
		"INSERT INTO data_dict VALUES (8, 'EVENT')",
		"INSERT INTO data_dict VALUES (9, 'SYS')",
		"INSERT INTO app_startup VALUES (2000000, 3000000, "+key+", 1)",
		"INSERT INTO app_startup VALUES (4000000, 5000000, 8, 1)",
		"CREATE TABLE hisys_all_event (ts INT, tid INT, domain_id INT, event_name_id INT, contents TEXT)",
		"INSERT INTO hisys_all_event VALUES (6000000, 100, 9, 8, 'marker')",
		"INSERT INTO native_hook VALUES (1, 7000000, 0, 'malloc', 4096, 1, 1)",
		"UPDATE thread_state SET dur=8000000 WHERE itid=1",
		"UPDATE thread_state SET ts=8000000, dur=1000000, cpu=1 WHERE itid=2",
		"INSERT INTO sched_slice VALUES (0, 8000000, 1, 1, 'S', 120)",
		"INSERT INTO sched_slice VALUES (8000000, 1000000, 1, 2, 'S', 120)")
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
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output,
		TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: writeFakeTraceStreamer(t, dir, 0)})
	if err != nil {
		t.Fatalf("actual conversion failed: %v", err)
	}
	for path, want := range map[string][]byte{input: payload, fixtureDB: before} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("source changed: %s, error=%v", path, err)
		}
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sched_switch:", "NativeHook:AllocEvent", "HeapSize|4096", "AppStartup:EVENT", "print: SYS/EVENT: marker"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("unrelated scheduler/resource output lost %q", want)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	in := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "NativeHook:", TimeStart: .006, TimeEnd: .008})
	out := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "NativeHook:", TimeStart: .008, TimeEnd: .009})
	if len(in.Events) != 1 || len(out.Events) != 0 {
		t.Fatalf("unrelated event/window changed: inside=%+v outside=%+v", in.Events, out.Events)
	}
	return string(body), result
}
