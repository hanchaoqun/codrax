package hitraceconv

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestNativeHookMetadataPublicExportAndSearch(t *testing.T) {
	body, exported := exportTraceDBSyncSpanIntegrationFixture(t, "native-resource-metadata",
		"ALTER TABLE native_hook ADD COLUMN heap_size",
		"ALTER TABLE native_hook ADD COLUMN callchain_id",
		"INSERT INTO native_hook VALUES (1, 1000000, 9223372036854775807, 'AllocEvent', 8192, 1, 1, 9007199254740993, 9007199254740995)",
		"INSERT INTO native_hook VALUES (2, 2000000, 0, 'FreeEvent', 4096, 1, 1, 0, -1)",
		"INSERT INTO native_hook VALUES (3, 3000000, NULL, 'MmapEvent', 4096, 1, 1, NULL, NULL)",
	)
	for _, want := range []string{
		"NativeHook:AllocEvent resource_end_ts_ns=9223372036854775807 source_heap_size=9007199254740993 source_callchain_id=9007199254740995",
		"NativeHook:FreeEvent resource_end_ts_ns=0 source_heap_size=0 source_callchain_id=-1",
		"NativeHook:MmapEvent resource_end_ts_ns=null source_heap_size=null source_callchain_id=null",
		"C|100|HeapSize|8192", "C|100|HeapSize|4096",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing exact resource fact %q", want)
		}
	}
	coverage := requireTraceDBCoverage(t, exported.Coverage, "resource", "native_hook")
	if coverage.RowsEmitted != 6 || coverage.Skipped != "" {
		t.Fatalf("changed I/C coverage: %+v", coverage)
	}
	if !strings.Contains(coverage.FieldSources["resource_metadata"], "not execution") {
		t.Error("metadata semantics absent from coverage")
	}
	idx, err := tracequery.BuildIndex(context.Background(), exported.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	result := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "source_callchain_id=9007199254740995", TimeStart: 0.0005, TimeEnd: 0.0015})
	if len(result.Events) != 1 || result.Events[0].SpanAction != "I" {
		t.Fatalf("resource metadata not searchable in exact window: %+v", result.Events)
	}
	outside := tracequery.Run(idx, tracequery.Query{View: "event_search", Pattern: "source_callchain_id=9007199254740995", TimeStart: 0.0025, TimeEnd: 0.0035})
	if len(outside.Events) != 0 {
		t.Fatalf("resource lifetime widened explicit event window: %+v", outside.Events)
	}
	for _, event := range idx.Events {
		if strings.Contains(event.SpanName, "NativeHook:") && event.SpanAction != "I" {
			t.Fatalf("resource lifetime became execution evidence: %+v", event)
		}
	}
	rank := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: 0.0005, TimeEnd: 0.0035})
	if rank.RootCauseRank == nil || rank.RootCauseRank.Window.StartTs != 0.0005 || rank.RootCauseRank.Window.EndTs != 0.0035 {
		t.Fatalf("resource metadata changed explicit root-cause window: %+v", rank.RootCauseRank)
	}
	for _, item := range append(rank.RootCauseRank.Items, rank.RootCauseRank.AbsorbedItems...) {
		if strings.HasPrefix(item.SpanName, "NativeHook:") {
			t.Fatalf("resource observation became a root-cause span: %+v", item)
		}
	}
	// The whole-table preservation lane remains independent of the visible adapter.
	if len(readTraceDBTextFidelityWire(t, body)) == 0 {
		t.Fatal("lost exact SQLite preservation")
	}
}

func TestNativeHookMetadataBadOptionalCellsKeepGoodInstantsAndCounters(t *testing.T) {
	body, exported := exportTraceDBSyncSpanIntegrationFixture(t, "native-invalid-metadata",
		"ALTER TABLE native_hook ADD COLUMN heap_size",
		"ALTER TABLE native_hook ADD COLUMN callchain_id",
		"INSERT INTO native_hook VALUES (1, 1000000, NULL, 'AllocEvent', 10, 1, 1, -1, 12.5)",
		"INSERT INTO native_hook VALUES (2, 2000000, NULL, 'FreeEvent', 0, 1, 1, '123', '7')",
		"INSERT INTO native_hook VALUES (3, 3000000, NULL, 'MmapEvent', 20, 1, 1, 1.5, X'0037')",
		"INSERT INTO native_hook VALUES (4, 4000000, NULL, 'MunmapEvent', 0, 1, 1, 20, 9)",
	)
	coverage := requireTraceDBCoverage(t, exported.Coverage, "resource", "native_hook")
	if coverage.RowsEmitted != 8 || !strings.Contains(coverage.Skipped, "invalid_optional_heap_size=3") || !strings.Contains(coverage.Skipped, "invalid_optional_callchain_id=3") {
		t.Fatalf("optional metadata must fail locally: %+v", coverage)
	}
	if strings.Count(body, "source_heap_size=") != 1 || strings.Count(body, "source_callchain_id=") != 1 || !strings.Contains(body, "source_heap_size=20 source_callchain_id=9") {
		t.Fatalf("invalid metadata coerced or valid sibling lost:\n%s", body)
	}
}

func TestNativeHookMetadataOldSchemaDoesNotInventOptionalColumns(t *testing.T) {
	body, exported := exportTraceDBSyncSpanIntegrationFixture(t, "native-old-schema",
		"INSERT INTO native_hook VALUES (1, 1000000, NULL, 'AllocEvent', 10, 1, 1)",
	)
	coverage := requireTraceDBCoverage(t, exported.Coverage, "resource", "native_hook")
	if coverage.RowsEmitted != 2 || coverage.Skipped != "" || len(coverage.ColumnsMissing) != 0 {
		t.Fatalf("old schema regressed: %+v", coverage)
	}
	if strings.Contains(body, "source_heap_size=") || strings.Contains(body, "source_callchain_id=") {
		t.Fatal("absent column turned into a value")
	}
	if !strings.Contains(body, "resource_end_ts_ns=null") {
		t.Fatal("NULL resource end not disclosed")
	}
}

func TestNativeHookMetadataNonMemoryResourcesDoNotAcquireByteUnits(t *testing.T) {
	body, exported := exportTraceDBSyncSpanIntegrationFixture(t, "native-nonmemory-metadata",
		"ALTER TABLE native_hook ADD COLUMN heap_size",
		"INSERT INTO native_hook VALUES (1, 1000000, NULL, 'FD_Open_Event', 1, 1, 1, 1)",
		"INSERT INTO native_hook VALUES (2, 2000000, NULL, 'ARK_GLOBAL_HANDLE_Alloc_Event', 2, 1, 1, 1)",
		"INSERT INTO native_hook VALUES (3, 3000000, NULL, 'THREAD_Create_Event', 3, 1, 1, 1)",
	)
	coverage := requireTraceDBCoverage(t, exported.Coverage, "resource", "native_hook")
	if coverage.RowsEmitted != 6 || coverage.Skipped != "" || strings.Count(body, "source_heap_size=1") != 3 {
		t.Fatalf("resource source value lost: %+v", coverage)
	}
	if strings.Contains(body, "allocation_bytes=") || strings.Contains(body, "size_bytes=") {
		t.Fatal("non-memory resource observation acquired unsupported byte units")
	}
}
