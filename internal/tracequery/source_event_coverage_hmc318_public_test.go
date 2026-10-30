package tracequery_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// Exercise the public builders, not a hand-built Index or a copied mutex.
// The actual capture ends at 1.051; query scope remains [.990, 1.055].
func TestHMC318PublicWindowedEOFCoverage(t *testing.T) {
	for _, mode := range []string{"cold", "exact_cache", "full_derived", "anchor_seeded"} {
		t.Run(mode, func(t *testing.T) {
			idx := hmc318PublicIndex(t, mode, false)
			for _, view := range []string{"thread_timeline", "window_stats", "wakeup_chain", "root_cause_rank"} {
				t.Run(view, func(t *testing.T) {
					q := tracequery.Query{View: view, PID: 100, TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true, Limit: 32}
					if view == "thread_timeline" {
						q.PID = 200
					}
					result := tracequery.Run(idx, q)
					if view == "thread_timeline" {
						hmc318AssertTimelineSleep(t, result, 36.5, 1.051)
						return
					}
					row := hmc317WorkerChurn(t, result, 200)
					body, err := json.Marshal(row)
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("native %s state_churn: %s", view, body)
					if result.WindowStats.Window.EndTs != q.TimeEnd || math.Abs(row.SleepMs-36.5) > 1e-8 || math.Abs(row.TotalMs-47) > 1e-8 {
						t.Fatalf("windowed physical EOF lost: sleep=%g total=%g window=%+v, want 36.5/47 and original query", row.SleepMs, row.TotalMs, result.WindowStats.Window)
					}
					for _, account := range row.StateAccounting {
						if account.State == string(tracequery.StateSSleep) {
							if account.ObservedEndCount != 1 || account.OpenTailCount != 1 || math.Abs(account.ObservedEndMs-31) > 1e-8 || math.Abs(account.OpenTailMs-5.5) > 1e-8 {
								t.Fatalf("EOF is not a state-closing event: %+v", account)
							}
							return
						}
					}
					t.Fatal("missing sleep account")
				})
			}
		})
	}
}

// A different valid physical event after the selected window must keep the
// legal open state through the query end, for both timeline and churn.
func TestHMC318PublicWindowedLatePhysicalTail(t *testing.T) {
	for _, mode := range []string{"cold", "exact_cache", "full_derived", "anchor_seeded"} {
		t.Run(mode, func(t *testing.T) {
			idx := hmc318PublicIndex(t, mode, true)
			q := tracequery.Query{View: "thread_timeline", PID: 200, TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true, Limit: 32}
			hmc318AssertTimelineSleep(t, tracequery.Run(idx, q), 40.5, 1.055)
			q.View = "window_stats"
			row := hmc317WorkerChurn(t, tracequery.Run(idx, q), 200)
			if math.Abs(row.SleepMs-40.5) > 1e-8 {
				t.Fatalf("selected-row end was promoted to source EOF: %+v", row)
			}
		})
	}
}

// A recognized typed carrier is not a comment and its start (including an
// unknown start represented by the parser's zero placeholder) does not prove a
// scheduler-clock EOF. A completed interval's end must not be silently dropped.
func TestHMC318PublicMixedCarrierCannotCertifyEarlierFtraceEOF(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	known := tracewire.MeasureInterval{RowID: 1, StartNS: tracewire.MeasureScalar{StorageClass: "integer", Value: "2000000000"},
		DurationNS: tracewire.MeasureScalar{StorageClass: "integer", Value: "0"}, Value: tracewire.MeasureScalar{StorageClass: "integer", Value: "1"},
		FilterID: tracewire.MeasureScalar{StorageClass: "null"}, MeasureType: tracewire.MeasureScalar{StorageClass: "text", Value: "measure"}, FilterStatus: "unknown"}
	late, err := tracewire.FormatMeasureInterval(known)
	if err != nil {
		t.Fatal(err)
	}
	unknown := known
	unknown.StartNS = tracewire.MeasureScalar{StorageClass: "null"}
	unknownLine, err := tracewire.FormatMeasureInterval(unknown)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := tracequery.FormatCompletedAsyncInterval(tracequery.CompletedAsyncInterval{
		StartTimestampNS: 1, EndTimestampNS: 2_000_000_000, SourceRow: 1, TID: 900, TGID: 900, SpanPID: 900,
		StartCPU: 0, StartCPUStatus: tracequery.TraceAsyncIntervalCPUStatusKnown, Comm: "other", Name: "bounded", Cookie: "7"})
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"known_late":             string(body) + late + "\n",
		"unknown_start":          unknownLine + "\n" + string(body),
		"completed_interval_end": interval + "\n" + string(body),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mixed.systrace")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndexWithOptions(context.Background(), path, tracequery.BuildOptions{
				AllowWindowedParse: true, TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true})
			if err != nil {
				t.Fatal(err)
			}
			q := tracequery.Query{View: "thread_timeline", PID: 200, TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true, Limit: 32}
			hmc318AssertTimelineSleep(t, tracequery.Run(idx, q), 40.5, 1.055)
		})
	}
}

func hmc318AssertTimelineSleep(t *testing.T, result tracequery.Result, wantSleep, wantEnd float64) {
	t.Helper()
	if result.Timeline == nil {
		t.Fatalf("timeline unavailable: %+v", result)
	}
	var sleep, last float64
	for _, interval := range result.Timeline.Intervals {
		if interval.State == tracequery.StateSSleep {
			sleep += interval.DurationMs
		}
		last = math.Max(last, interval.EndTs)
	}
	body, err := json.Marshal(result.Timeline)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("native timeline: %s", body)
	if result.TimeEnd != 1.055 || math.Abs(sleep-wantSleep) > 1e-8 || math.Abs(last-wantEnd) > 1e-9 {
		t.Fatalf("timeline conflated selected and physical end: sleep=%g end=%g request=%g want=%g/%g/1.055", sleep, last, result.TimeEnd, wantSleep, wantEnd)
	}
}

func hmc318PublicIndex(t *testing.T, mode string, late bool) *tracequery.Index {
	t.Helper()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	var content strings.Builder
	if mode == "anchor_seeded" {
		for i := 0; i < 9000; i++ {
			fmt.Fprintf(&content, "noise-9 (9) [007] .... %.6f: cpu_frequency: state=1000000 cpu_id=7\n", .1+float64(i)*.00001)
		}
	}
	content.Write(body)
	if late {
		content.WriteString("noise-9 (9) [007] .... 2.000000: cpu_frequency: state=1000000 cpu_id=7\n")
	}
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err := os.WriteFile(path, []byte(content.String()), 0600); err != nil {
		t.Fatal(err)
	}
	opts := tracequery.BuildOptions{TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true}
	switch mode {
	case "full_derived":
		_, err = tracequery.BuildIndex(context.Background(), path)
	case "exact_cache":
		_, err = tracequery.BuildIndexWithOptions(context.Background(), path, opts)
	case "anchor_seeded":
		_, err = tracequery.BuildIndexWithOptions(context.Background(), path, tracequery.BuildOptions{TimeStart: .1, TimeEnd: .2, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true})
	}
	if err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed || math.Abs(idx.LastTs-1.051) > 1e-9 {
		t.Fatalf("expected local selected window distinct from source receipt: windowed=%t last=%g", idx.Windowed, idx.LastTs)
	}
	return idx
}
