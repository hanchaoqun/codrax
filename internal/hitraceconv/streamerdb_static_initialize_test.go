package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBStaticInitializeSourceAdmissionAndUnknownCPU(t *testing.T) {
	body, result := exportTraceDBSyncSpanIntegrationFixture(t, "static-cpu",
		"DELETE FROM thread_state",
		"INSERT INTO thread_state VALUES (1, 0, 2000000, 0, 'Running')",
		"INSERT INTO thread_state VALUES (1, 2000000, 1000000, 3, 'Running')",
		"INSERT INTO static_initalize(rowid,start_time,end_time,so_name,ipid,tid) VALUES (-9223372036854775808,0,1000000,'libzero.so',1,100)",
		"INSERT INTO static_initalize(rowid,start_time,end_time,so_name,ipid,tid) VALUES (0,1500000,2500000,'libmigration.so',1,100)",
		"INSERT INTO static_initalize(rowid,start_time,end_time,so_name,ipid,tid) VALUES (9223372036854775807,4000000,6000000,'lib业务|phase=2.so',1,100)",
		"INSERT INTO static_initalize(rowid,start_time,end_time,so_name,ipid,tid) VALUES (-1,7000000,7000000,'libpoint.so',1,100)",
	)
	c := requireTraceDBCoverage(t, result.Coverage, "slice", "static_initalize")
	if c.RowsRead != 4 || c.RowsEmitted != 8 {
		t.Fatalf("source census %+v", c)
	}
	path := filepath.Join(t.TempDir(), "static.systrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventTraceMark}, Limit: 1000})
	known, unknown := 0, 0
	for _, e := range query.Events {
		if !strings.HasPrefix(e.SpanName, "SoInit:") {
			continue
		}
		if e.PluginFields != nil && e.TraceMarkerCPUStatus == "unavailable" {
			unknown++
			if !strings.HasPrefix(e.Raw, "# codrax_trace_mark_cpu_unavailable/") || strings.Contains(e.Raw, "[000]") {
				t.Fatalf("unknown CPU invented %+v", e)
			}
		} else {
			known++
		}
	}
	if known != 2 || unknown != 2 {
		t.Fatalf("known=%d unknown=%d body=%s", known, unknown, body)
	}
	if !strings.Contains(body, "[000]") || !strings.Contains(body, "[003]") || !traceDBTestHasMarkerLabel(t, body, "SoInit:lib业务|phase=2.so") {
		t.Fatalf("CPU0/migration/name lost %s", body)
	}
}

func TestTraceDBStaticInitializeBadDerivedRowsDoNotPoisonHealthySources(t *testing.T) {
	for _, row := range []string{
		"NULL,2000000,'bad.so',1,100", "'1000000',2000000,'bad.so',1,100", "1000000.5,2000000,'bad.so',1,100",
		"1000000,NULL,'bad.so',1,100", "2000000,1000000,'bad.so',1,100", "-1,2000000,'bad.so',1,100",
		"1000000,2000000,NULL,1,100", "1000000,2000000,X'6c6962',1,100", "1000000,2000000,'',1,100",
		"1000000,2000000,'bad.so',NULL,100", "1000000,2000000,'bad.so',-1,100", "1000000,2000000,'bad.so',4294967295,100",
		"1000000,2000000,'bad.so',1,0", "1000000,2000000,'bad.so',2,100", "1000000,2000000,'bad.so',1,'100'",
	} {
		t.Run(row, func(t *testing.T) {
			body, result := exportTraceDBSyncSpanIntegrationFixture(t, "static-invalid",
				"DROP TABLE static_initalize", "CREATE TABLE static_initalize(start_time,end_time,so_name,ipid,tid)",
				"INSERT INTO static_initalize VALUES ("+row+")",
				"INSERT INTO static_initalize VALUES (4000000,5000000,'healthy.so',1,100)",
				"INSERT INTO callstack VALUES (1,6000000,1000000,1,NULL,'healthy-original','',NULL,NULL,0)")
			c := requireTraceDBCoverage(t, result.Coverage, "slice", "static_initalize")
			if c.RowsRead != 2 || c.RowsEmitted != 2 || c.Skipped == "" || !strings.Contains(body, "healthy-original") || !strings.Contains(body, "SoInit:healthy.so") || strings.Contains(body, "SoInit:bad.so") {
				t.Fatalf("unsafe derived rejection %+v\n%s", c, body)
			}
		})
	}
}

func TestTraceDBStaticInitializeCanonicalOwnerAndGeneration(t *testing.T) {
	for _, extra := range []string{
		"INSERT INTO thread VALUES (3,100,1,'ambiguous',0,0,0)",
		"INSERT INTO thread VALUES (1,999,1,'conflict',0,0,0)",
		"INSERT INTO process VALUES (1,300,'conflict')",
		"INSERT INTO instant VALUES (2000000,'sched_wakeup_new',1,2,'itid')",
		"INSERT INTO thread_state VALUES (1,1000000,1000,1,'X')",
	} {
		t.Run(extra, func(t *testing.T) {
			_, result := exportTraceDBSyncSpanIntegrationFixture(t, "static-owner", extra,
				"INSERT INTO static_initalize VALUES (0,2500000,'bad-generation.so',1,100)")
			c := requireTraceDBCoverage(t, result.Coverage, "slice", "static_initalize")
			if c.RowsRead != 1 || c.RowsEmitted != 0 || c.Skipped == "" {
				t.Fatalf("unproven owner/generation accepted %+v", c)
			}
		})
	}
}

func TestTraceDBStaticInitializeDuplicatesNeverDoubleCount(t *testing.T) {
	for _, call := range []string{"'liba.so'", "'dlopen liba.so'", "'SoInit:liba.so'"} {
		body, result := exportTraceDBSyncSpanIntegrationFixture(t, "static-duplicate",
			"INSERT INTO static_initalize VALUES (1000000,2000000,'liba.so',1,100)",
			fmt.Sprintf("INSERT INTO callstack VALUES (1,1000000,1000000,1,NULL,%s,'',NULL,NULL,0)", call),
			"INSERT INTO callstack VALUES (2,3000000,1000000,1,NULL,'healthy-original','',NULL,NULL,0)")
		c := requireTraceDBCoverage(t, result.Coverage, "slice", "static_initalize")
		original := requireTraceDBCoverage(t, result.Coverage, "slice", "callstack")
		if c.RowsEmitted != 0 || original.RowsEmitted != 4 || !traceDBTestHasMarkerLabel(t, body, strings.Trim(call, "'")) || !strings.Contains(body, "healthy-original") {
			t.Fatalf("duplicate source interval counted %+v\n%s", c, body)
		}
		if call == "'liba.so'" && !strings.Contains(c.Skipped, "duplicate_callstack_projection=1") {
			t.Fatalf("exact projection not disclosed %+v", c)
		}
	}
	body, result := exportTraceDBSyncSpanIntegrationFixture(t, "static-static-conflict",
		"INSERT INTO static_initalize VALUES (1000000,2000000,'a.so',1,100)",
		"INSERT INTO static_initalize VALUES (1000000,2000000,'b.so',1,100)")
	if requireTraceDBCoverage(t, result.Coverage, "slice", "static_initalize").RowsEmitted != 0 || strings.Contains(body, "SoInit:") {
		t.Fatal("conflicting derived rows became first-row-wins")
	}
}

func TestTraceDBStaticInitializeTypedStageParityAndClosedCPUAuthority(t *testing.T) {
	known := traceDBTestSyncSpanCandidate(traceDBSyncSpanProducerStaticInitialize, -1, 101, 100, 0, 100, "SoInit:lib.so")
	known.StartCPU, known.EndCPU = 0, 4095
	unknown := known
	unknown.StableID = 0
	unknown.Start, unknown.End = 200, 300
	unknown.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownEnd
	unknown.StartCPU, unknown.EndCPU = 0, 0
	unknown.StartCPUProvenance, unknown.EndCPUProvenance = traceDBSyncSpanCPUStaticUnavailable, traceDBSyncSpanCPUStaticUnavailable
	a := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{ResidentBytes: 1 << 20}, []traceDBSyncSpanCandidate{known, unknown}, nil, false)
	b := renderTraceDBSyncSpanStageCase(t, traceDBSyncSpanStageOptions{ResidentBytes: 1}, []traceDBSyncSpanCandidate{known, unknown}, nil, false)
	if a.body != b.body || !reflect.DeepEqual(a.report, b.report) {
		t.Fatal("static CPU provenance lost on spill")
	}
	for _, mutate := range []func(*traceDBSyncSpanCandidate){
		func(c *traceDBSyncSpanCandidate) { c.StartCPU = 1 },
		func(c *traceDBSyncSpanCandidate) { c.CanonicalITIDKnown = false; c.CanonicalITID = 0 },
		func(c *traceDBSyncSpanCandidate) { c.StartCPUProvenance = traceDBSyncSpanCPULegacyUnverified },
		func(c *traceDBSyncSpanCandidate) { c.StartCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable },
		func(c *traceDBSyncSpanCandidate) {
			c.DepthKnown = true
			c.Depth = 1
			c.DepthProvenance = traceDBSyncSpanDepthCallstack
		},
	} {
		bad := unknown
		mutate(&bad)
		if validateTraceDBSyncSpanCandidate(bad) == nil {
			t.Fatalf("invalid static authority %+v", bad)
		}
	}
}
