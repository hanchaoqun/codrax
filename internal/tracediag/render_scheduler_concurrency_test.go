package tracediag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// Strip exactly the new optional wrapper for historical schema witnesses;
// additions must not bless unrelated old-field or tag changes.
func nonEventSchemaBeforeSchedulerConcurrency(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.WindowStats{}) {
		return schema
	}
	const added = "SchedulerConcurrency|*tracequery.SchedulerConcurrencyStats|scheduler_concurrency,omitempty"
	var prior []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
		} else {
			prior = append(prior, field)
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one reviewed scheduler concurrency field, got %d: %s", count, schema)
	}
	return strings.Join(prior, ";")
}

func TestSchedulerConcurrencySchemaEvolutionIsAdditive(t *testing.T) {
	typ := reflect.TypeOf(tracequery.WindowStats{})
	_, schema := detailSchemaFingerprint(typ)
	prior := nonEventSchemaBeforeSchedulerConcurrency(t, typ, schema)
	sum := sha256.Sum256([]byte(prior))
	const want = "2d8e73ee45e8c05c971f1ef962bbbdb70f3bd78609bc2102f8279900cbc94809"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("scheduler addition changed prior WindowStats: got=%s want=%s", got, want)
	}
}

func TestSchedulerConcurrencyDetailPreservesNativePrecision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nano.systrace")
	body := "idle-0 (0) [001] .... 0.000000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=41 next_prio=120\n" +
		"worker-41 (41) [001] .... 0.000000001: sched_switch: prev_comm=worker prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	res := tracequery.Run(idx, tracequery.Query{View: "window_stats", TimeStartSet: true, TimeEndSet: true, TimeEnd: .000000010})
	if res.WindowStats == nil || res.WindowStats.SchedulerConcurrency == nil {
		t.Fatalf("native public query must first supply scheduler concurrency: %+v", res.WindowStats)
	}
	stats := res.WindowStats.SchedulerConcurrency
	var measured bool
	before, _ := json.Marshal(res)
	report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: &res}).lines, "\n")
	for i, group := range stats.Groups {
		if group.Values == nil {
			continue
		}
		measured = true
		v := group.Values
		want := fmt.Sprintf("window_stats.scheduler_concurrency.groups[%d].values: peak_threads=%d mean_threads=%s busy_ms=%s thread_ms=%s", i,
			v.PeakThreads, formatFloatToken(v.MeanThreads), formatFloatToken(v.BusyMs), formatFloatToken(v.ThreadMs))
		if !strings.Contains(report, want) {
			t.Errorf("lost exact native statistics: want %q\n%s", want, report)
		}
		for j, segment := range group.Segments {
			want = fmt.Sprintf("window_stats.scheduler_concurrency.groups[%d].segments[%d]: start_ts=%s end_ts=%s threads=%d", i, j,
				strconv.FormatFloat(segment.StartTs, 'f', -1, 64), strconv.FormatFloat(segment.EndTs, 'f', -1, 64), segment.Threads)
			if !strings.Contains(report, want) {
				t.Errorf("lost exact native segment: want %q\n%s", want, report)
			}
		}
	}
	if !measured || !strings.Contains(report, "window_stats.scheduler_concurrency.window: start_ts=0 end_ts=0.00000001") {
		t.Fatalf("native measured interval or legal zero/nanosecond window missing: %s", report)
	}
	after, _ := json.Marshal(res)
	if !bytes.Equal(before, after) {
		t.Fatal("render mutated native statistics")
	}
}

func TestSchedulerConcurrencyBulkPreservesExistingDetailsAndUnknown(t *testing.T) {
	for _, measured := range []bool{true, false} {
		t.Run(fmt.Sprint(measured), func(t *testing.T) {
			res := &tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{
				Window:        tracequery.TimeWindow{StartTs: 1, EndTs: 2},
				TopRunning:    []tracequery.ThreadDuration{{Thread: tracequery.ThreadRef{PID: 41, Comm: "worker"}, DurationMs: 57.828, CPU: 12}},
				ComputeSupply: []tracequery.ComputeSupplySummary{{Thread: tracequery.ThreadRef{PID: 41, Comm: "worker"}, Summary: "existing frequency donor evidence"}},
				IOInFlight:    &tracequery.IOInFlightStats{Population: "accepted_complete_pairs", IssuerScope: "all_issuers"},
			}}
			baseline := renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: res})
			group := tracequery.SchedulerConcurrencyGroup{SourcePath: "/capture/one.systrace", State: "running", ValuesUnavailableReason: "no_accepted_closed_intervals"}
			if measured {
				group.ValuesUnavailableReason = ""
				group.Values = &tracequery.SchedulerConcurrencyValues{}
				group.Segments = []tracequery.SchedulerConcurrencySegment{{StartTs: 1, EndTs: 2, Threads: 0}}
			}
			res.WindowStats.SchedulerConcurrency = &tracequery.SchedulerConcurrencyStats{
				Population: "accepted_closed_intervals", ThreadScope: "all_positive_tids", GroupCount: 1,
				Groups: []tracequery.SchedulerConcurrencyGroup{group},
				Window: &tracequery.SchedulerConcurrencyWindow{StartTs: 1, EndTs: 2},
			}
			before, _ := json.Marshal(res)
			bounded := renderStepBody(&Step{View: "window_stats", effMaxLines: len(baseline.lines)}, stepOutcome{result: res})
			if !reflect.DeepEqual(bounded.lines, baseline.lines) {
				t.Fatalf("new scheduler bulk evicted old evidence:\n%s\nwant:\n%s", strings.Join(bounded.lines, "\n"), strings.Join(baseline.lines, "\n"))
			}
			full := renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: res})
			text := strings.Join(full.lines, "\n")
			if bounded.total != full.total || full.total <= len(bounded.lines) || !strings.Contains(text, "window_stats.scheduler_concurrency") {
				t.Fatalf("new data/omission accounting lost: bounded=%+v full=%+v", bounded, full)
			}
			if measured {
				if !strings.Contains(text, "peak_threads=0 mean_threads=0 busy_ms=0 thread_ms=0") || !strings.Contains(text, "start_ts=1 end_ts=2 threads=0") {
					t.Fatalf("measured population zero lost: %s", text)
				}
			} else if strings.Contains(text, "peak_threads") || !strings.Contains(text, "no_accepted_closed_intervals") {
				t.Fatalf("unknown became zero: %s", text)
			}
			after, _ := json.Marshal(res)
			if !bytes.Equal(before, after) {
				t.Fatal("render mutated native result")
			}
		})
	}
}
