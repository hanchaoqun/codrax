package tracediag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestNativeStateAccountingDetailPreservesUnknownAndZero(t *testing.T) {
	idx, err := tracequery.BuildIndex(context.Background(), "../../eval/fixtures/hmosperf_scheduler_concurrency/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	r := tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 104, TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
	before, _ := json.Marshal(r)
	var lines []string
	renderNonEventResultDetail(&r, func(s string) { lines = append(lines, s) })
	text := strings.Join(lines, "\n")
	for _, want := range []string{"accounting: state=runnable caliber=cumulative_segments", "observed_end_count=0", "observed_end_ms=0", "open_tail_count=1", "unknown_closure_count=0", "start_clipped_count=0", "end_clipped_count=0", "boundary_continuation_count=0"} {
		if !strings.Contains(text, want) {
			t.Fatalf("native cumulative/tail metadata lost %q:\n%s", want, text)
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("display changed native account")
	}
	legacy := tracequery.Result{WindowStats: &tracequery.WindowStats{TopRunning: []tracequery.ThreadDuration{{Thread: tracequery.ThreadRef{PID: 9}, DurationMs: 1, LineStart: 1, LineEnd: 2}}}}
	lines = nil
	renderNonEventResultDetail(&legacy, func(s string) { lines = append(lines, s) })
	if strings.Contains(strings.Join(lines, "\n"), "accounting:") {
		t.Fatal("legacy display invented closure from endpoints")
	}
}
