package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestExportTraceDBOfficialRelationsExactAndFailClosed(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE frame_slice (id INTEGER PRIMARY KEY, ts INTEGER, callstack_id INTEGER) WITHOUT ROWID",
		"INSERT INTO frame_slice VALUES (0, 100, 7)",
		"INSERT INTO frame_slice VALUES (1, 200, 999)",
		"CREATE TABLE callstack (id INTEGER)",
		"INSERT INTO callstack VALUES (7)",
		"CREATE TABLE gpu_slice (id INTEGER, frame_row INTEGER, dur INTEGER)",
		"INSERT INTO gpu_slice VALUES (0, 0, 50)",
		"INSERT INTO gpu_slice VALUES (1, 99, 60)",
		"CREATE TABLE perf_sample (id INTEGER, timestamp_trace INTEGER, cpu_id INTEGER, thread_id INTEGER, callchain_id INTEGER, event_count INTEGER, event_type_id INTEGER)",
		"INSERT INTO perf_sample VALUES (5, 300, 0, 20, 4, 6, 7)",
		"CREATE TABLE perf_thread (thread_id INTEGER, process_id INTEGER)",
		"INSERT INTO perf_thread VALUES (20, 10)",
		"CREATE TABLE perf_napi_async (id INTEGER, ts INTEGER, traceid TEXT, cpu_id INTEGER, thread_id INTEGER, process_id INTEGER, caller_callchainid INTEGER, callee_callchainid INTEGER, perf_sample_id INTEGER, event_count INTEGER, event_type_id INTEGER)",
		"INSERT INTO perf_napi_async VALUES (0, 300, '0xabc|napi', 0, 20, 10, 3, 4, 5, 6, 7)",
		"INSERT INTO perf_napi_async VALUES (1, 301, 'mismatch', 0, 20, 10, 3, 4, 5, 6, 7)",
	})
	ctx := context.Background()
	tdb, err := openTraceDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	authority := traceDBTestCompleteSchedulerAuthority(newTraceDBThreadIndex(0, true))
	authority.frameProfile = traceDBActivityITIDSignedInt32
	authority.frameProfileSource = "official current id/type profile"
	sink, err := newTraceDBRowSink(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()

	rosterCoverage, frames, err := loadTraceDBFrameRelationRoster(ctx, tdb, authority)
	if err != nil {
		t.Fatal(err)
	}
	if rosterCoverage.RowsRead != 2 || rosterCoverage.RowsEmitted != 2 {
		t.Fatalf("frame relation roster coverage drifted: %+v", rosterCoverage)
	}
	callstackCoverage, err := exportTraceDBFrameCallstackRelations(ctx, tdb, sink, frames)
	if err != nil {
		t.Fatal(err)
	}
	gpuCoverage, err := exportTraceDBFrameGPURelations(ctx, tdb, sink, authority, frames)
	if err != nil {
		t.Fatal(err)
	}
	napiCoverage, err := exportTraceDBPerfNAPIAsyncRelations(ctx, tdb, sink)
	if err != nil {
		t.Fatal(err)
	}
	if callstackCoverage.RowsEmitted != 1 ||
		!strings.Contains(callstackCoverage.Skipped, "unavailable_callstack_endpoint=1") {
		t.Fatalf("frame-callstack coverage drifted: %+v", callstackCoverage)
	}
	if gpuCoverage.RowsEmitted != 1 ||
		!strings.Contains(gpuCoverage.Skipped, "unavailable_frame_endpoint=1") {
		t.Fatalf("frame-GPU coverage drifted: %+v", gpuCoverage)
	}
	if napiCoverage.RowsEmitted != 1 ||
		!strings.Contains(napiCoverage.Skipped, "perf_sample_endpoint_mismatch=1") {
		t.Fatalf("perf-NAPI coverage drifted: %+v", napiCoverage)
	}

	outPath := filepath.Join(t.TempDir(), "official-relations.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.prepareAndWriteForTest(ctx, out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{
		"# codrax_frame_callstack/v1 ",
		"# codrax_frame_gpu/v1 ",
		"# codrax_perf_napi_async/v1 ",
	} {
		if strings.Count(string(body), prefix) != 1 {
			t.Fatalf("typed relation count for %q drifted:\n%s", prefix, body)
		}
	}
	idx, err := tracequery.BuildIndex(ctx, outPath)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[tracequery.EventType]int{}
	for _, event := range idx.Events {
		counts[event.Type]++
	}
	for _, eventType := range []tracequery.EventType{
		tracequery.EventFrameCallstack, tracequery.EventFrameGPU, tracequery.EventPerfNAPIAsync,
	} {
		if counts[eventType] != 1 {
			t.Fatalf("typed event %s count=%d, want 1: %+v", eventType, counts[eventType], idx.Events)
		}
	}
}

func TestFrameRelationsSurviveFrameSpanAdmissionFailure(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE frame_slice (id INTEGER PRIMARY KEY, ts INTEGER, callstack_id INTEGER, dur INTEGER, type INTEGER, type_desc TEXT, vsync INTEGER, flag INTEGER, ipid INTEGER, itid INTEGER) WITHOUT ROWID",
		"INSERT INTO frame_slice VALUES (0, 100, 7, 10, 0, 'actural', 1, 1, 999, 999)",
		"CREATE TABLE frame_maps (id INTEGER, src_row INTEGER, dst_row INTEGER)",
		"INSERT INTO frame_maps VALUES (0, 0, 1)",
		"INSERT INTO frame_slice VALUES (1, 200, NULL, 10, 0, 'actural', 2, 1, 999, 999)",
	})
	ctx := context.Background()
	tdb, err := openTraceDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	authority := traceDBTestCompleteSchedulerAuthority(newTraceDBThreadIndex(0, true))
	authority.frameProfile = traceDBActivityITIDSignedInt32
	authority.frameProfileSource = "official current id/type profile"
	sink, err := newTraceDBRowSink(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	framesCoverage, emitted, err := exportTraceDBFrameSliceWithRows(ctx, tdb, sink, authority, traceDBSchedulerRunningIndex{})
	if err != nil {
		t.Fatal(err)
	}
	if framesCoverage.RowsEmitted != 0 || len(emitted) != 0 {
		t.Fatalf("invalid frame identities unexpectedly admitted a span: %+v %v", framesCoverage, emitted)
	}
	_, roster, err := loadTraceDBFrameRelationRoster(ctx, tdb, authority)
	if err != nil {
		t.Fatal(err)
	}
	mapCoverage, err := exportTraceDBFrameMaps(ctx, tdb, sink, authority, roster)
	if err != nil {
		t.Fatal(err)
	}
	if mapCoverage.RowsEmitted != 1 {
		t.Fatalf("exact frame relation was coupled to span admission: %+v", mapCoverage)
	}
}
