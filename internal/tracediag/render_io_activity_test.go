package tracediag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func activityRenderQuery(t *testing.T, body string, q tracequery.Query) tracequery.Result {
	t.Helper()
	path := filepath.Join(t.TempDir(), "activity.systrace")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q.View = "window_stats"
	res := tracequery.Run(idx, q)
	if res.WindowStats == nil || res.WindowStats.IOActivity == nil {
		t.Fatalf("native public query did not provide endpoint activity: %+v", res.WindowStats)
	}
	return res
}

func activityRenderReport(t *testing.T, res *tracequery.Result) string {
	t.Helper()
	before, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 3000}, stepOutcome{result: res}).lines, "\n")
	after, _ := json.Marshal(res)
	if !bytes.Equal(before, after) {
		t.Fatal("display mutated native activity measurements")
	}
	return report
}

func TestIOActivityDetailPublicUnpairedEventsAndIdleShortTail(t *testing.T) {
	body := "io-40 (40) [003] .... 0.020000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n" +
		"io-40 (40) [003] .... 0.080000: block_rq_issue: 8,0 R 8192 () 16 + 16 [io]\n" +
		"io-40 (40) [003] .... 0.220000: block_rq_issue: 8,0 W 8192 () 32 + 16 [io]\n"
	res := activityRenderQuery(t, body, tracequery.Query{TimeStartSet: true, TimeEndSet: true, TimeEnd: .25, PID: 99})
	stats := res.WindowStats.IOActivity
	if len(stats.Groups) != 1 {
		t.Fatalf("direction must not fragment native identity groups: %+v", stats)
	}
	g := stats.Groups[0]
	if g.Values.EventCount != 3 || g.Rates == nil || g.Rates.EventsPerSecond != 12 || g.Values.KnownBytes == nil || *g.Values.KnownBytes != 20480 || len(g.Buckets) != 3 {
		t.Fatalf("witness must contain full independent endpoints including unpaired: %+v", g)
	}
	report := activityRenderReport(t, &res)
	for _, want := range []string{
		"population=observed_endpoint_events", "query_pid=99", "source_path=activity.systrace",
		"endpoint_family=block_rq phase=start dev=8,0 byte_caliber=request_bytes",
		"io_activity.window: start_ts=0 end_ts=0.25",
		"known_bytes=20480", "events_per_second=12 known_bytes_per_second=81920",
		"read_event_share=0.6666666666666666", "read_known_byte_share=0.6",
		"buckets[1].window: start_ts=0.1 end_ts=0.2",
		"buckets[1].values: event_count=0 known_byte_event_count=0 unknown_byte_event_count=0 invalid_byte_event_count=0 overflow_byte_event_count=0 known_bytes=0",
		"buckets[1].rates: events_per_second=0 known_bytes_per_second=0",
		"buckets[2].window: start_ts=0.2 end_ts=0.25", "omitted_buckets=0",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("lost exact activity fact %q:\n%s", want, report)
		}
	}
	for i, b := range g.Buckets {
		want := fmt.Sprintf("io_activity.groups[0].buckets[%d].rates: events_per_second=%s", i, strconv.FormatFloat(b.Rates.EventsPerSecond, 'f', -1, 64))
		if !strings.Contains(report, want) {
			t.Errorf("bucket rate must retain native actual-width value: %q", want)
		}
	}
}

func TestIOActivityDetailPublicZeroUnknownAndLineScope(t *testing.T) {
	flush := "io-40 (40) [003] .... 0.000000001: block_rq_issue: 8,0 F 0 () 0 + 0 [io]\n"
	res := activityRenderQuery(t, flush, tracequery.Query{TimeStartSet: true, TimeEndSet: true, TimeEnd: .00000001})
	report := activityRenderReport(t, &res)
	for _, want := range []string{"io_activity.window: start_ts=0 end_ts=0.00000001", "known_bytes=0 bytes_overflow=false known_size_mean_bytes=0", "direction=other", "event_denominator=0"} {
		if !strings.Contains(report, want) {
			t.Errorf("known zero/coordinate lost %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "read_event_share=0") {
		t.Fatal("zero R+W denominator invented zero-percent read share")
	}
	sync := "io-40 (40) [003] .... 1.001000: f2fs_sync_file_enter: dev=8:0 ino=0x9 pino=0x1 i_mode=0x81a4 i_size=4096 i_nlink=1 i_blocks=8 i_advise=0x0\n"
	res = activityRenderQuery(t, sync, tracequery.Query{TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true, LineStart: 1, LineEnd: 1})
	if res.WindowStats.IOActivity.Window != nil || res.WindowStats.IOActivity.WindowUnavailableReason == "" || len(res.WindowStats.IOActivity.Groups) != 1 {
		t.Fatalf("line bounds must remain authoritative: %+v", res.WindowStats.IOActivity)
	}
	report = activityRenderReport(t, &res)
	for _, want := range []string{"window_unavailable_reason=", "unknown_byte_event_count=1", "byte_caliber=unspecified", "line_start=1 line_end=1"} {
		if !strings.Contains(report, want) {
			t.Errorf("unknown/line fact lost %q:\n%s", want, report)
		}
	}
	for _, absent := range []string{"io_activity.window:", "io_activity.groups[0].rates:", "read_event_share=0"} {
		if strings.Contains(report, absent) {
			t.Errorf("unknown measured as zero or continuous window: %q\n%s", absent, report)
		}
	}
	for _, line := range strings.Split(report, "\n") {
		if strings.Contains(line, "io_activity.groups[0].values:") && (strings.Contains(line, "known_bytes=") || strings.Contains(line, "known_size_mean_bytes=")) {
			t.Errorf("unknown-size group acquired a measurement: %s", line)
		}
	}
}

func TestIOActivityBulkPreservesAllOlderDeferredDetails(t *testing.T) {
	res := &tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{
		TopRunning:           []tracequery.ThreadDuration{{Thread: tracequery.ThreadRef{PID: 41}, DurationMs: 1}},
		IOInFlight:           &tracequery.IOInFlightStats{Population: "accepted_complete_pairs"},
		SchedulerConcurrency: &tracequery.SchedulerConcurrencyStats{Population: "accepted_closed_intervals"},
		BusinessTree:         &tracequery.TraceMarkerTreeStats{NodeCount: 1, Coverage: "partial"},
	}}
	baseline := renderStepBody(&Step{View: "window_stats", effMaxLines: 3000}, stepOutcome{result: res})
	res.WindowStats.IOActivity = &tracequery.IOActivityStats{Population: tracequery.IOActivityPopulationEndpointEvents, Groups: []tracequery.IOActivityGroup{{SourcePath: "/private/activity.systrace"}}}
	bounded := renderStepBody(&Step{View: "window_stats", effMaxLines: len(baseline.lines)}, stepOutcome{result: res})
	if !reflect.DeepEqual(bounded.lines, baseline.lines) {
		t.Fatalf("endpoint bulk evicted existing detail: got=%v want=%v", bounded.lines, baseline.lines)
	}
	full := renderStepBody(&Step{View: "window_stats", effMaxLines: 3000}, stepOutcome{result: res})
	if bounded.total != full.total || full.total <= len(bounded.lines) || strings.Contains(strings.Join(full.lines, "\n"), "/private/") {
		t.Fatal("activity omission accounting or source display changed")
	}
}

func TestIOActivityDetailPublicDefaultCaptureEndAndExplicitPoint(t *testing.T) {
	body := "io-40 (40) [003] .... 0.020000: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n" +
		"io-40 (40) [003] .... 0.220000: block_rq_issue: 8,0 W 8192 () 32 + 16 [io]\n"
	res := activityRenderQuery(t, body, tracequery.Query{})
	stats := res.WindowStats.IOActivity
	if stats.Window == nil || !stats.Window.EndInclusive || len(stats.Groups) != 1 || stats.Groups[0].Values.EventCount != 2 {
		t.Fatalf("native full-capture selection lost its last endpoint: %+v", stats)
	}
	report := activityRenderReport(t, &res)
	for _, want := range []string{"io_activity.window: start_ts=0.02 end_ts=0.22 end_inclusive=true", "buckets[1].window: start_ts=0.12 end_ts=0.22 end_inclusive=true"} {
		if !strings.Contains(report, want) {
			t.Errorf("default end-boundary disclosure lost %q:\n%s", want, report)
		}
	}
	res = activityRenderQuery(t, body, tracequery.Query{TimeStart: .22, TimeEnd: .22, TimeStartSet: true, TimeEndSet: true})
	stats = res.WindowStats.IOActivity
	if stats.Window != nil || len(stats.Groups) != 1 || stats.Groups[0].Values.EventCount != 1 || stats.Groups[0].Rates != nil {
		t.Fatalf("point inventory must retain endpoint without inventing duration: %+v", stats)
	}
	report = activityRenderReport(t, &res)
	if strings.Contains(report, "io_activity.window:") || strings.Contains(report, "io_activity.groups[0].rates:") || !strings.Contains(report, "window_unavailable_reason=") {
		t.Fatalf("point display lost count-only scope: %s", report)
	}
}
