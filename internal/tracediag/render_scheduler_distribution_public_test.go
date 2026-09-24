package tracediag

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestSchedulerConcurrencyNewDetailsPreservePublicZeroNanosecondsAndNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native.systrace")
	body := "idle-0 (0) [000] .... 0.000000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=S ==> next_comm=worker next_pid=10 next_prio=120\nworker-10 (10) [000] .... 0.000000001: sched_switch: prev_comm=worker prev_pid=10 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []bool{false, true} {
		q := tracequery.Query{View: "window_stats", TimeStartSet: true, TimeEndSet: true, TimeEnd: .00000001}
		if line {
			q.LineStart, q.LineEnd = 1, 2
		}
		res := tracequery.Run(idx, q)
		before, _ := json.Marshal(res)
		text := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 3000}, stepOutcome{result: &res}).lines, "\n")
		if !strings.Contains(text, "actual_start_ts=0 actual_end_ts=0.000000001") || !strings.Contains(text, "cpu=0") {
			t.Fatalf("source zero/nanosecond/CPU identity lost: %s", text)
		}
		if line {
			if strings.Contains(text, "window_contribution_ms=") || strings.Contains(text, "p50_threads=") {
				t.Fatalf("line-only inventory gained numbers: %s", text)
			}
		} else {
			g := res.WindowStats.SchedulerConcurrency.Groups[0]
			ms := *g.Members[0].WindowContributionMs
			if !strings.Contains(text, "window_contribution_ms="+strconv.FormatFloat(ms, 'f', -1, 64)) || !strings.Contains(text, "p50_threads=0") || !strings.Contains(text, "window_share=") || !strings.Contains(text, ".buckets[0].window: start_ts=0 end_ts=0.00000001") {
				t.Fatalf("new native measurement omitted/rounded: %s", text)
			}
		}
		after, _ := json.Marshal(res)
		if string(before) != string(after) {
			t.Fatal("renderer mutated native result")
		}
	}
}
