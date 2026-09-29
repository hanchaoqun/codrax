package hitraceconv

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestStartupSourceRecordsDoNotRequireOrInventThreadExecution(t *testing.T) {
	_, result := dictionaryReferencePublicConvert(t, [3]string{"0", "9", "8"}, false,
		"DROP TABLE app_startup", "CREATE TABLE app_startup (start_time, end_time, start_name, ipid)",
		"INSERT INTO app_startup VALUES (2100000, 2900000, 0, NULL), (2200000, 2800000, 0, '1'), (2300000, 2700000, 0, 0), (2400000, 3100000, 0, 999)",
		"INSERT INTO app_startup VALUES (NULL, 3000000, 0, 1), ('2000000',3000000,0,1), (2000000.0,3000000,0,1), (3000000,2000000,0,1), (0,0,0,1)",
		// A real callstack on the main thread crosses process-owned records;
		// neither lane may poison/suppress the other.
		"INSERT INTO callstack VALUES (990, 2000000, 2500000, 1, NULL, 'RealWork', '', NULL, NULL, 0)")
	coverage := requireTraceDBCoverage(t, result.TraceDBCoverage, "slice", "app_startup")
	if coverage.RowsRead != 11 || coverage.RowsEmitted != 12 || !strings.Contains(coverage.Skipped, "invalid_source_interval=5") || !strings.Contains(coverage.Skipped, "owner_null_reference=1") || !strings.Contains(coverage.Skipped, "owner_invalid_reference=1") {
		t.Fatalf("source row census lost invalid rows or process-only records: %+v", coverage)
	}
	idx, err := tracequery.BuildIndex(context.Background(), result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	got := tracequery.Run(idx, tracequery.Query{View: "event_search", EventTypes: []tracequery.EventType{tracequery.EventTraceMark}, Limit: 100})
	count, realCount := 0, 0
	for _, e := range got.Events {
		if e.SpanName == "RealWork" {
			realCount++
		}
		if e.PluginFields == nil || e.MarkerNameOrigin == nil {
			continue
		}
		count++
		if e.CPU != -1 || e.PID != 0 || e.TGID != 0 || e.SpanPID != 0 || (e.SpanAction != "source_begin" && e.SpanAction != "source_end") {
			t.Fatalf("process record minted physical execution: %+v", e)
		}
		record := e.MarkerNameOrigin.Record
		if record.OwnerIssue == "" && record.OwnerIPID == 1 {
			if record.OwnerPID == nil || *record.OwnerPID != 100 {
				t.Fatalf("unique process mapping lost: %+v", record)
			}
		} else if record.OwnerPID != nil {
			t.Fatalf("unknown/invalid owner acquired process identity: %+v", record)
		}
	}
	if count != 12 || realCount != 1 {
		t.Fatalf("lost independent lanes: process endpoints=%d real begins=%d", count, realCount)
	}
}
