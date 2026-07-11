package hitraceconv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestExportTraceDBFrameSliceAsyncRoundTrip(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'MainApp')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 501, 1, 'WorkerThread', 0, 0, 2)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 1490000, 22000, 3, 'Running')",
		"INSERT INTO thread_state VALUES (2, 1512000, 18000, 7, 'Running')",
		"CREATE TABLE frame_slice (id INTEGER PRIMARY KEY, ts INT, dur INT, type INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT) WITHOUT ROWID",
		"INSERT INTO frame_slice VALUES (77, 1500000, 16000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (78, 1501000, 19000, 0, 'actural', 123, 1, 1, 2)",
	})

	coverage, outPath, body, idx := exportTraceDBFrameFixture(t, path, func(sink *traceDBRowSink) {
		if err := addTraceDBTestSyncSpanRows(sink, 1499000, 1508000, "WorkerThread", 501, 500, 3, "OuterSync"); err != nil {
			t.Fatalf("add crossing sync control span: %v", err)
		}
	})
	if coverage.RowsRead != 2 || coverage.RowsEmitted != 4 || coverage.Skipped != "" {
		t.Fatalf("unexpected frame coverage: %+v", coverage)
	}
	if !strings.Contains(coverage.FieldSources["stable_identity"], "frame_slice.id signed-int32 projection") ||
		!strings.Contains(coverage.FieldSources["wire_pairing"], "async S/F") {
		t.Fatalf("missing frame provenance: %+v", coverage.FieldSources)
	}
	if strings.Contains(body, "B|500|FrameActual-123") || strings.Contains(body, "E|500|FrameActual-123") {
		t.Fatalf("frame interval leaked into shared B/E stack:\n%s", body)
	}
	for _, endpoint := range []string{
		"S|500|FrameActual-123|hconv-frame-77",
		"F|500|FrameActual-123|hconv-frame-77",
		"S|500|FrameActual-123|hconv-frame-78",
		"F|500|FrameActual-123|hconv-frame-78",
	} {
		if !strings.Contains(body, endpoint) {
			t.Fatalf("missing frame endpoint %q:\n%s", endpoint, body)
		}
	}

	type endpointWant struct {
		action string
		cpu    int
		ts     float64
	}
	wants := map[string]endpointWant{
		"hconv-frame-77/S": {action: "S", cpu: 3, ts: 0.001500},
		"hconv-frame-77/F": {action: "F", cpu: 7, ts: 0.001516},
		"hconv-frame-78/S": {action: "S", cpu: 3, ts: 0.001501},
		"hconv-frame-78/F": {action: "F", cpu: 7, ts: 0.001520},
	}
	seen := map[string]bool{}
	for _, event := range idx.Events {
		if event.Type != tracequery.EventTraceMark || event.SpanName != "FrameActual-123" {
			continue
		}
		key := event.SpanValue + "/" + event.SpanAction
		want, ok := wants[key]
		if !ok {
			t.Fatalf("unexpected frame endpoint: %+v", event)
		}
		if event.SpanAction != want.action || event.CPU != want.cpu || !nearFloat(event.Ts, want.ts, 0.0000001) ||
			event.SpanPID != 500 || event.PID != 501 || event.TGID != 500 || event.Comm != "WorkerThread" {
			t.Fatalf("frame endpoint provenance mismatch: got=%+v want=%+v", event, want)
		}
		seen[key] = true
	}
	if len(seen) != len(wants) {
		t.Fatalf("frame endpoint set mismatch: seen=%v want=%v events=%+v", seen, wants, idx.Events)
	}

	q := tracequery.Query{
		PID: 501, SpanName: "FrameActual-123",
		TimeStart: 0.00149, TimeEnd: 0.00153, TimeStartSet: true, TimeEndSet: true,
	}
	spans, caveats := tracequery.FindSpanWindows(idx, q, 8)
	assertFrameAsyncSpans(t, outPath, spans)
	for _, caveat := range caveats {
		if strings.Contains(caveat, "duplicate") || strings.Contains(caveat, "pairing_incomplete") {
			t.Fatalf("stable frame cookies triggered async pairing failure: %v", caveats)
		}
	}
	stats := tracequery.ComputeWindowStats(idx, q)
	var frameStats []tracequery.TraceSpanSummary
	for _, span := range stats.TraceSpans {
		if span.Name == "FrameActual-123" {
			frameStats = append(frameStats, span)
		}
	}
	assertFrameAsyncSpans(t, outPath, frameStats)
	rank := tracequery.BuildRootCauseRank(idx, tracequery.Query{
		PID: 501, TimeStart: 0.00149, TimeEnd: 0.00153, TimeStartSet: true, TimeEndSet: true,
		MinDurationMs: 0.001, Limit: 12,
	})
	frameRankRows := 0
	for _, item := range rank.Items {
		if item.SpanName != "FrameActual-123" {
			continue
		}
		frameRankRows++
		if item.Rank != 0 || item.Tier != tracequery.RootCauseTierContextOnly || item.EffectiveImpactMs != 0 || item.SemanticClass != "" {
			t.Fatalf("frame pacing projection changed root-rank semantics: %+v", item)
		}
	}
	if frameRankRows == 0 {
		t.Fatalf("frame pacing span disappeared from rank context disclosure: %+v", rank)
	}

	outer, _ := tracequery.FindSpanWindows(idx, tracequery.Query{PID: 501, SpanName: "OuterSync"}, 4)
	if len(outer) != 1 || outer[0].Kind != "sync" || !nearFloat(outer[0].DurationMs, 0.009, 0.000001) {
		t.Fatalf("async frame crossing corrupted the independent B/E control span: %+v", outer)
	}
}

func TestExportTraceDBFrameSliceMalformedRowsSkipLocally(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'MainApp')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 501, 1, 'WorkerThread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (2, 0, 1000000, 3, 'Running')",
		"CREATE TABLE frame_slice (id, ts, dur, type, type_desc, vsync, flag, ipid, itid)",
		"INSERT INTO frame_slice VALUES (1, 0, 2000, 0, 'actural', NULL, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES ('bad-id', 10000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (-1, 15000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (19.5, 16000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (4294967295, 17000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (NULL, 18000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (2, 20000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (2, 30000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (3, -1, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (4, 0.5, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (5, NULL, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (6, 40000, 0, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (7, 40000, 1.5, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (8, 9223372036854775807, 1, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (9, 50000, 2000, 1, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (10, 60000, 2000, 0, 'mystery', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (19, 61000, 2000, NULL, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (20, 62000, 2000, 0.5, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (11, 70000, 2000, 0, 'actural', '123', 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (21, 71000, 2000, 0, 'actural', -1, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (22, 72000, 2000, 0, 'actural', 4294967295, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (12, 80000, 2000, 0, 'actural', 123, 2, 1, 2)",
		"INSERT INTO frame_slice VALUES (13, 90000, 2000, 0, 'actural', 123, '1', 1, 2)",
		"INSERT INTO frame_slice VALUES (14, 100000, 2000, 0, 'actural', 123, 1, 2, 2)",
		"INSERT INTO frame_slice VALUES (23, 101000, 2000, 0, 'actural', 123, 1, NULL, 2)",
		"INSERT INTO frame_slice VALUES (24, 102000, 2000, 0, 'actural', 123, 1, 0.5, 2)",
		"INSERT INTO frame_slice VALUES (25, 103000, 2000, 0, 'actural', 123, 1, '1', 2)",
		"INSERT INTO frame_slice VALUES (26, 104000, 2000, 0, 'actural', 123, 1, 0, 2)",
		"INSERT INTO frame_slice VALUES (27, 105000, 2000, 0, 'actural', 123, 1, 4294967295, 2)",
		"INSERT INTO frame_slice VALUES (15, 110000, 2000, 0, 'actural', 123, 1, 1, 999)",
		"INSERT INTO frame_slice VALUES (28, 111000, 2000, 0, 'actural', 123, 1, 1, NULL)",
		"INSERT INTO frame_slice VALUES (29, 112000, 2000, 0, 'actural', 123, 1, 1, 0.5)",
		"INSERT INTO frame_slice VALUES (30, 113000, 2000, 0, 'actural', 123, 1, 1, '2')",
		"INSERT INTO frame_slice VALUES (31, 114000, 2000, 0, 'actural', 123, 1, 1, 0)",
		"INSERT INTO frame_slice VALUES (32, 115000, 2000, 0, 'actural', 123, 1, 1, 4294967295)",
		"INSERT INTO frame_slice VALUES (16, 1000000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (17, 999000, 2000, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (18, 120000, 2000, 1, 'expect', 456, NULL, 1, 2)",
		"INSERT INTO frame_slice VALUES (33, 121000, 2000, 0, 'actural', 123, NULL, 1, 2)",
		"INSERT INTO frame_slice VALUES (34, 122000, 2000, 1, 'expect', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (35, 499, 999, 0, 'actural', 123, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (36, 499, 1000, 0, 'actural', 789, 1, 1, 2)",
	})
	coverage, _, body, _ := exportTraceDBFrameFixture(t, path, nil)
	if coverage.RowsRead != 42 || coverage.RowsEmitted != 8 {
		t.Fatalf("malformed siblings changed valid frame accounting: %+v", coverage)
	}
	for _, want := range []string{
		"invalid_row_identity=4",
		"duplicate_row_identity=2",
		"invalid_timestamp=3",
		"invalid_duration=2",
		"interval_overflow=1",
		"frame_kind_mismatch=3",
		"invalid_frame_kind=1",
		"invalid_vsync=3",
		"suppressed_frame_flag=1",
		"invalid_frame_flag=1",
		"frame_flag_kind_mismatch=2",
		"invalid_owner_ipid=5",
		"invalid_emitter_itid=5",
		"owner_identity_mismatch=1",
		"unresolved_emitter_thread=1",
		"unknown_start_cpu=1",
		"unknown_end_cpu=1",
		"wire_interval_collapsed=1",
	} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("missing typed skip %q: %+v", want, coverage)
		}
	}
	for _, want := range []string{
		"S|500|FrameActual-None|hconv-frame-1",
		"F|500|FrameActual-None|hconv-frame-1",
		"S|500|FrameActual-123|hconv-frame-4294967295",
		"F|500|FrameActual-123|hconv-frame-4294967295",
		"S|500|FrameExpected-456|hconv-frame-18",
		"F|500|FrameExpected-456|hconv-frame-18",
		"S|500|FrameActual-789|hconv-frame-36",
		"F|500|FrameActual-789|hconv-frame-36",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("valid sibling endpoint %q was lost:\n%s", want, body)
		}
	}
}

func TestExportTraceDBFrameSliceStableIdentityCompatibility(t *testing.T) {
	t.Run("legacy hidden rowid and zero timestamp", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'MainApp')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (2, 501, 1, 'WorkerThread', 0, 0, 1)",
			"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
			"INSERT INTO thread_state VALUES (2, 0, 5000, 4, 'Running')",
			"CREATE TABLE frame_slice (ts INT, dur INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
			"INSERT INTO frame_slice VALUES (0, 2000, 'expect', NULL, NULL, 1, 2)",
		})
		coverage, _, body, idx := exportTraceDBFrameFixture(t, path, nil)
		if coverage.RowsEmitted != 2 || coverage.FieldSources["stable_identity"] != "frame_slice.hidden_rowid" ||
			!strings.Contains(body, "S|500|FrameExpected-None|hconv-frame-1") {
			t.Fatalf("legacy frame identity was not preserved: coverage=%+v\n%s", coverage, body)
		}
		spans := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 0, TimeEnd: 0.000005, TimeStartSet: true, TimeEndSet: true}).TraceSpans
		if len(spans) != 1 || spans[0].Kind != "async" || spans[0].Category != "frame_pacing" || spans[0].Subcategory != "expected" ||
			!nearFloat(spans[0].DurationMs, 0.002, 0.000001) {
			t.Fatalf("legacy expected frame did not round-trip: %+v", spans)
		}
	})

	t.Run("without rowid and without explicit id fails closed", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE frame_slice (key TEXT PRIMARY KEY, ts INT, dur INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT) WITHOUT ROWID",
			"INSERT INTO frame_slice VALUES ('one', 0, 2000, 'actural', 1, 1, 1, 1)",
			"INSERT INTO frame_slice VALUES ('two', 3000, 2000, 'actural', 2, 1, 1, 1)",
		})
		coverage, _, body, _ := exportTraceDBFrameFixture(t, path, nil)
		if coverage.RowsEmitted != 0 || coverage.Skipped != "stable_row_identity_unavailable=2" ||
			!strings.Contains(coverage.FieldSources["stable_identity"], "unavailable") {
			t.Fatalf("unkeyed WITHOUT ROWID frame table did not fail closed: %+v", coverage)
		}
		if strings.Contains(body, "FrameActual") {
			t.Fatalf("unkeyed WITHOUT ROWID frame leaked an endpoint:\n%s", body)
		}
	})
}

func TestExportTraceDBFrameSliceFlagKindClosedMatrix(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'MainApp')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 501, 1, 'WorkerThread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 0, 100000, 4, 'Running')",
		"CREATE TABLE frame_slice (id INT, ts INT, dur INT, type INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
		"INSERT INTO frame_slice VALUES (1, 0, 2000, 0, 'actural', 1, 0, 1, 2)",
		"INSERT INTO frame_slice VALUES (2, 3000, 2000, 0, 'actural', 2, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (3, 6000, 2000, 0, 'actural', 3, 3, 1, 2)",
		"INSERT INTO frame_slice VALUES (4, 9000, 2000, 0, 'actural', 4, 2, 1, 2)",
		"INSERT INTO frame_slice VALUES (5, 12000, 2000, 0, 'actural', 5, NULL, 1, 2)",
		"INSERT INTO frame_slice VALUES (6, 15000, 2000, 1, 'expect', 6, NULL, 1, 2)",
		"INSERT INTO frame_slice VALUES (7, 18000, 2000, 1, 'expect', 7, 0, 1, 2)",
		"INSERT INTO frame_slice VALUES (8, 21000, 2000, 1, 'expect', 8, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (9, 24000, 2000, 1, 'expect', 9, 3, 1, 2)",
	})
	coverage, _, body, _ := exportTraceDBFrameFixture(t, path, nil)
	if coverage.RowsRead != 9 || coverage.RowsEmitted != 8 ||
		!strings.Contains(coverage.Skipped, "frame_flag_kind_mismatch=4") ||
		!strings.Contains(coverage.Skipped, "suppressed_frame_flag=1") {
		t.Fatalf("flag/kind producer matrix was not enforced: %+v", coverage)
	}
	for _, id := range []string{"1", "2", "3", "6"} {
		if !strings.Contains(body, "hconv-frame-"+id) {
			t.Fatalf("valid flag/kind row %s was lost:\n%s", id, body)
		}
	}
	for _, id := range []string{"4", "5", "7", "8", "9"} {
		if strings.Contains(body, "hconv-frame-"+id) {
			t.Fatalf("invalid/suppressed flag/kind row %s leaked:\n%s", id, body)
		}
	}
}

func TestExportTraceDBFrameSliceSameTimestampUsesStableIDOrder(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 500, 'MainApp')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 501, 1, 'WorkerThread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 0, 10000, 4, 'Running')",
		"CREATE TABLE frame_slice (id INT, ts INT, dur INT, type INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
		"INSERT INTO frame_slice VALUES (9, 0, 2000, 0, 'actural', 9, 1, 1, 2)",
		"INSERT INTO frame_slice VALUES (3, 0, 2000, 0, 'actural', 3, 1, 1, 2)",
	})
	coverage, _, body, _ := exportTraceDBFrameFixture(t, path, nil)
	first := strings.Index(body, "S|500|FrameActual-3|hconv-frame-3")
	second := strings.Index(body, "S|500|FrameActual-9|hconv-frame-9")
	if coverage.RowsEmitted != 4 || first < 0 || second < 0 || first >= second {
		t.Fatalf("same-timestamp frame order did not follow stable source id: coverage=%+v first=%d second=%d\n%s", coverage, first, second, body)
	}
}

func TestExportTraceDBFrameSliceSchemaProfilesFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		create string
		insert string
		want   string
	}{
		{
			name:   "missing flag",
			create: "CREATE TABLE frame_slice (id INT, ts INT, dur INT, type INT, type_desc TEXT, vsync INT, ipid INT, itid INT)",
			insert: "INSERT INTO frame_slice VALUES (1, 0, 2000, 0, 'actural', 1, 1, 1)",
			want:   "missing_required_columns=flag",
		},
		{
			name:   "id without type",
			create: "CREATE TABLE frame_slice (id INT, ts INT, dur INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
			insert: "INSERT INTO frame_slice VALUES (1, 0, 2000, 'actural', 1, 1, 1, 1)",
			want:   "id_present=true type_present=false",
		},
		{
			name:   "type without id",
			create: "CREATE TABLE frame_slice (ts INT, dur INT, type INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
			insert: "INSERT INTO frame_slice VALUES (0, 2000, 0, 'actural', 1, 1, 1, 1)",
			want:   "id_present=false type_present=true",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := createTraceDBFixture(t, []string{
				"CREATE TABLE trace_range (start_ts INT)",
				"INSERT INTO trace_range VALUES (0)",
				tc.create,
				tc.insert,
			})
			coverage, _, body, _ := exportTraceDBFrameFixture(t, path, nil)
			if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "unsupported_schema_profile=1") ||
				!strings.Contains(coverage.Skipped, tc.want) || strings.Contains(body, "FrameActual") {
				t.Fatalf("unsupported frame schema profile did not fail closed: %+v\n%s", coverage, body)
			}
		})
	}
}

func TestExportTraceDBFrameSliceRegistrationHintsAndRunningIntegrity(t *testing.T) {
	t.Run("thread registration hint does not manufacture a generation cut", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'MainApp')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (1, 501, 1, 'old-name', 0, 0, 1)",
			"INSERT INTO thread VALUES (2, 501, 1, 'new-name', 50000, 0, 1)",
			"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
			"INSERT INTO thread_state VALUES (1, 0, 50001, 2, 'Running')",
			"INSERT INTO thread_state VALUES (2, 50000, 50000, 3, 'Running')",
			"CREATE TABLE frame_slice (id INT, ts INT, dur INT, type INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
			"INSERT INTO frame_slice VALUES (1, 40000, 10000, 0, 'actural', 1, 1, 1, 1)",
			"INSERT INTO frame_slice VALUES (2, 50000, 10000, 0, 'actural', 2, 1, 1, 2)",
		})
		coverage, _, body, _ := exportTraceDBFrameFixture(t, path, nil)
		if coverage.RowsEmitted != 4 || coverage.Skipped != "" ||
			!strings.Contains(body, "hconv-frame-1") || !strings.Contains(body, "hconv-frame-2") {
			t.Fatalf("registration hint was promoted into a thread generation cut: coverage=%+v\n%s", coverage, body)
		}
	})

	t.Run("process registration hint does not manufacture a generation cut", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'old-process')",
			"INSERT INTO process VALUES (2, 500, 'new-process')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (1, 501, 1, 'old-thread', 0, 0, 1)",
			"INSERT INTO thread VALUES (2, 601, 2, 'new-thread', 50000, 0, 1)",
			"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
			"INSERT INTO thread_state VALUES (1, 0, 50001, 2, 'Running')",
			"INSERT INTO thread_state VALUES (2, 50000, 50000, 3, 'Running')",
			"CREATE TABLE frame_slice (id INT, ts INT, dur INT, type INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
			"INSERT INTO frame_slice VALUES (1, 40000, 10000, 0, 'actural', 1, 1, 1, 1)",
			"INSERT INTO frame_slice VALUES (2, 50000, 10000, 0, 'actural', 2, 1, 2, 2)",
		})
		coverage, _, body, _ := exportTraceDBFrameFixture(t, path, nil)
		if coverage.RowsEmitted != 4 || coverage.Skipped != "" ||
			!strings.Contains(body, "hconv-frame-1") || !strings.Contains(body, "hconv-frame-2") {
			t.Fatalf("thread hint was promoted into a process generation cut: coverage=%+v\n%s", coverage, body)
		}
	})

	t.Run("malformed Running witness taints the frame lane", func(t *testing.T) {
		path := createTraceDBFixture(t, []string{
			"CREATE TABLE trace_range (start_ts INT)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
			"INSERT INTO process VALUES (1, 500, 'MainApp')",
			"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
			"INSERT INTO thread VALUES (2, 501, 1, 'WorkerThread', 0, 0, 1)",
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"INSERT INTO thread_state VALUES (2, 0, 10000, 3, 'Running')",
			"INSERT INTO thread_state VALUES (2, 1000, 1000, 5000, 'Running')",
			"CREATE TABLE frame_slice (id INT, ts INT, dur INT, type INT, type_desc TEXT, vsync INT, flag INT, ipid INT, itid INT)",
			"INSERT INTO frame_slice VALUES (1, 0, 2000, 0, 'actural', 1, 1, 1, 2)",
		})
		coverage, _, body, _ := exportTraceDBFrameFixture(t, path, nil)
		if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "tainted_running_cpu_witness=1") || strings.Contains(body, "FrameActual") {
			t.Fatalf("tainted CPU witness minted a frame: coverage=%+v\n%s", coverage, body)
		}
	})
}

func exportTraceDBFrameFixture(t *testing.T, path string, decorate func(*traceDBRowSink)) (TraceDBCoverage, string, string, *tracequery.Index) {
	t.Helper()
	ctx := context.Background()
	tdb, err := openTraceDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	running, integrity, _, err := tdb.loadRunningIntervals(ctx, index)
	if err != nil {
		t.Fatal(err)
	}
	profile, profileSource, err := traceDBActivityProfile(ctx, tdb.db, "frame_slice")
	if err != nil {
		t.Fatal(err)
	}
	authority := newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		FrameProfile:       profile,
		FrameProfileSource: profileSource,
		CreationComplete:   true,
		TerminalComplete:   true,
		ActivityComplete:   true,
	})
	typedRunning := newTraceDBSchedulerRunningIndex(authority, running, integrity, nil)
	sink, err := newTraceDBRowSink(t.TempDir(), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := exportTraceDBFrameSlice(ctx, tdb, sink, authority, typedRunning)
	if err != nil {
		t.Fatalf("export frame_slice: %v", err)
	}
	if decorate != nil {
		decorate(sink)
	}
	outPath := filepath.Join(t.TempDir(), "frame.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.writeTo(ctx, out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write frame systrace: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(ctx, outPath)
	if err != nil {
		t.Fatalf("tracequery parse frame output: %v", err)
	}
	return coverage, outPath, string(bodyBytes), idx
}

func assertFrameAsyncSpans(t *testing.T, sourcePath string, spans []tracequery.TraceSpanSummary) {
	t.Helper()
	if len(spans) != 2 {
		t.Fatalf("got %d frame spans, want 2: %+v", len(spans), spans)
	}
	resolvedSourcePath, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		resolvedSourcePath = sourcePath
	}
	durations := map[float64]bool{0.016: false, 0.019: false}
	for _, span := range spans {
		if (span.SourcePath != sourcePath && span.SourcePath != resolvedSourcePath) || span.Kind != "async" || span.Name != "FrameActual-123" || span.SpanPID != 500 ||
			span.Thread.Comm != "WorkerThread" || span.Thread.PID != 501 || span.Thread.TGID != 500 ||
			span.Category != "frame_pacing" || span.Subcategory != "actual" || span.StartLine <= 0 || span.EndLine <= span.StartLine {
			t.Fatalf("frame span provenance mismatch: %+v", span)
		}
		matched := false
		for duration := range durations {
			if nearFloat(span.DurationMs, duration, 0.000001) {
				durations[duration] = true
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("unexpected frame span duration: %+v", span)
		}
	}
	for duration, seen := range durations {
		if !seen {
			t.Fatalf("missing %.3fms frame span: %+v", duration, spans)
		}
	}
}

func nearFloat(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}
