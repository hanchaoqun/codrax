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
)

// The exact authored capture used by the natural evaluation ends at 1.051.
// A requested window ending at 1.055 is legitimate, but its last 4ms are not
// a measured continuation of the worker's open sleep state.
func TestHMC317PublicBusinessStateChurnKeepsPhysicalArtifactTail(t *testing.T) {
	idx, err := tracequery.BuildIndex(context.Background(), "../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(idx.LastTs-1.051) > 1e-9 || idx.Size <= 0 {
		t.Fatalf("public builder lost the actual capture boundary: last=%g size=%d", idx.LastTs, idx.Size)
	}
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank"} {
		t.Run(view, func(t *testing.T) {
			result := tracequery.Run(idx, tracequery.Query{View: view, PID: 100,
				TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true, Limit: 32})
			if result.WindowStats == nil {
				t.Fatal("public result omitted shared window statistics")
			}
			if result.WindowStats.Window.StartTs != .990 || result.WindowStats.Window.EndTs != 1.055 {
				t.Fatalf("do not rewrite the requested query window: %+v", result.WindowStats.Window)
			}
			for _, row := range result.WindowStats.StateChurn {
				if row.Thread.PID != 200 {
					continue
				}
				body, err := json.Marshal(row)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("native state_churn: %s", body)
				if math.Abs(row.SleepMs-36.5) > 1e-8 || math.Abs(row.RunningMs-9.5) > 1e-8 ||
					math.Abs(row.RunnableMs-1) > 1e-8 || math.Abs(row.TotalMs-47) > 1e-8 {
					t.Fatalf("open state escaped physical EOF: got sleep=%g running=%g runnable=%g total=%g; want 36.5/9.5/1/47ms", row.SleepMs, row.RunningMs, row.RunnableMs, row.TotalMs)
				}
				for _, account := range row.StateAccounting {
					if account.State == string(tracequery.StateSSleep) {
						if account.ObservedEndCount != 1 || account.OpenTailCount != 1 ||
							math.Abs(account.ObservedEndMs-31) > 1e-8 || math.Abs(account.OpenTailMs-5.5) > 1e-8 {
							t.Fatalf("artifact boundary is not an observed sleep closure: %+v", account)
						}
						return
					}
				}
				t.Fatal("shared churn row lost its independent sleep account")
			}
			t.Fatal("public result lost the actual worker state-churn row")
		})
	}
}

func TestHMC317PublicWindowedChurnDoesNotMistakeSelectedTailForEOF(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("noise-9 (9) [007] .... 2.000000: cpu_frequency: state=1000000 cpu_id=7\n")...)
	for _, warm := range []bool{false, true} {
		name := "cold"
		if warm {
			name = "full_cache_derived"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "late-physical-event.systrace")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			if warm {
				if _, err := tracequery.BuildIndex(context.Background(), path); err != nil {
					t.Fatal(err)
				}
			}
			idx, err := tracequery.BuildIndexWithOptions(context.Background(), path, tracequery.BuildOptions{
				TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !idx.Windowed || idx.LastTs != 1.051 {
				t.Fatalf("fixture must expose a selected, not physical, end: windowed=%t last=%g", idx.Windowed, idx.LastTs)
			}
			result := tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 100,
				TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true})
			row := hmc317WorkerChurn(t, result, 200)
			if math.Abs(row.SleepMs-40.5) > 1e-8 {
				t.Fatalf("selected-row tail is not EOF; legitimate later capture permits 40.5ms sleep, got %g", row.SleepMs)
			}
		})
	}
}

func hmc317WorkerChurn(t *testing.T, result tracequery.Result, pid int) tracequery.ThreadStateChurnSummary {
	t.Helper()
	if result.WindowStats != nil {
		for _, row := range result.WindowStats.StateChurn {
			if row.Thread.PID == pid {
				return row
			}
		}
	}
	t.Fatalf("no state-churn row for exact TID %d", pid)
	return tracequery.ThreadStateChurnSummary{}
}

func TestHMC317PublicOpenTailBoundAppliesToEveryActiveState(t *testing.T) {
	for _, state := range []string{"running", "runnable", "s_sleep", "d_sleep", "io_wait"} {
		t.Run(state, func(t *testing.T) {
			rows := hmc317ActivePrefix()
			tailStart := 1.005
			if state != "runnable" {
				rows = append(rows, hmc317Switch(1.006, 0, 41, "S"))
				tailStart = 1.006
			}
			if state != "runnable" && state != "running" {
				prev := "S"
				if state == "d_sleep" || state == "io_wait" {
					prev = "D"
				}
				rows = append(rows, hmc317Switch(1.007, 41, 0, prev))
				tailStart = 1.007
				if state == "io_wait" {
					rows = append(rows, "target-41 (40) [001] .... 1.007500: sched_blocked_reason: pid=41 iowait=1 caller=wait_on_page")
				}
			}
			rows = append(rows, "noise-9 (9) [007] .... 1.008000: cpu_frequency: state=1000000 cpu_id=7")
			path := hmc317WriteTrace(t, strings.Join(rows, "\n")+"\n")
			idx, err := tracequery.BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			q := tracequery.Query{View: "window_stats", PID: 41, TimeStart: 1, TimeEnd: 1.012, TimeStartSet: true, TimeEndSet: true}
			indexed := tracequery.Run(idx, q)
			streamed, err := tracequery.StreamStateCluster(context.Background(), path, q, 20)
			if err != nil {
				t.Fatal(err)
			}
			for _, result := range []tracequery.Result{indexed, streamed} {
				row := hmc317WorkerChurn(t, result, 41)
				if math.Abs(row.TotalMs-8) > 1e-8 || result.WindowStats.Window.EndTs != q.TimeEnd {
					t.Fatalf("measured and requested scopes mixed: total=%g window=%+v", row.TotalMs, result.WindowStats.Window)
				}
				found := false
				for _, a := range row.StateAccounting {
					if a.State == state {
						found = true
						if a.OpenTailCount != 1 || math.Abs(a.OpenTailMs-(1.008-tailStart)*1000) > 1e-8 || a.UnknownClosureCount != 0 {
							t.Fatalf("EOF must not close or extend %s: %+v", state, a)
						}
					}
				}
				if !found {
					t.Fatalf("missing %s tail accounting", state)
				}
			}
		})
	}
}

func TestHMC317PublicChurnTailScopeCompatibility(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name               string
		end                float64
		endSet             bool
		lineStart, lineEnd int
		unproven           string
		wantSleep          float64
	}{
		{"explicit_eof", 1.055, true, 0, 0, "", 36.5},
		{"line_scope_does_not_replace_physical_tail", 1.055, true, 7, 16, "", 36.5},
		{"legacy_unset_flag", 1.055, false, 0, 0, "", 40.5},
		{"legacy_line_scope_unset_flag", 1.055, false, 7, 16, "", 40.5},
		{"whole_capture_default", 0, false, 0, 0, "", 36.5},
		{"in_capture_window", 1.050, true, 0, 0, "", 35.5},
		{"no_physical_receipt", 1.055, true, 0, 0, "size", 40.5},
		{"unknown_complete_order", 1.055, true, 0, 0, "order", 40.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local, err := tracequery.BuildIndex(context.Background(), hmc317WriteTrace(t, string(body)))
			if err != nil {
				t.Fatal(err)
			}
			switch tc.unproven {
			case "size":
				local.Size = 0
			case "order":
				local.TimestampOrder = tracequery.TraceTimestampOrderUnknown
			}
			result := tracequery.Run(local, tracequery.Query{View: "window_stats", PID: 100,
				TimeStart: .990, TimeEnd: tc.end, TimeStartSet: true, TimeEndSet: tc.endSet, LineStart: tc.lineStart, LineEnd: tc.lineEnd})
			row := hmc317WorkerChurn(t, result, 200)
			if math.Abs(row.SleepMs-tc.wantSleep) > 1e-8 {
				t.Fatalf("scope/EOF eligibility changed: sleep=%g want=%g", row.SleepMs, tc.wantSleep)
			}
			if row.MeasurementDomain == nil || row.MeasurementDomain.QueryLineStart != tc.lineStart || row.MeasurementDomain.QueryLineEnd != tc.lineEnd {
				t.Fatalf("exact original line scope lost: %+v", row.MeasurementDomain)
			}
		})
	}
}

func TestHMC317PublicStoppedAndDeadTasksNeverGainTail(t *testing.T) {
	for _, prev := range []string{"T", "t", "X", "Z"} {
		t.Run(prev, func(t *testing.T) {
			rows := append(hmc317ActivePrefix(), hmc317Switch(1.006, 0, 41, "S"), hmc317Switch(1.007, 41, 0, prev),
				"noise-9 (9) [007] .... 1.008000: cpu_frequency: state=1000000 cpu_id=7")
			idx, err := tracequery.BuildIndex(context.Background(), hmc317WriteTrace(t, strings.Join(rows, "\n")+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			result := tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 41, TimeStart: 1, TimeEnd: 1.012, TimeStartSet: true, TimeEndSet: true})
			row := hmc317WorkerChurn(t, result, 41)
			if math.Abs(row.TotalMs-7) > 1e-8 {
				t.Fatalf("inactive tail became active contribution: %+v", row)
			}
			for _, a := range row.StateAccounting {
				if a.OpenTailCount != 0 {
					t.Fatalf("terminated task gained open tail: %+v", a)
				}
			}
		})
	}
}

// This records an intentionally unclosed scope, not a contract requiring the
// old overcount forever. A windowed index needs independent physical-source
// coverage before its local LastTs can safely be used as an EOF bound. A later
// fix may make its values equal the full-index result without failing this test.
func TestHMC317PublicWindowedEOFRemainingBoundaryCharacterization(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	path := hmc317WriteTrace(t, string(body))
	idx, err := tracequery.BuildIndexWithOptions(context.Background(), path, tracequery.BuildOptions{
		TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true})
	if err != nil {
		t.Fatal(err)
	}
	q := tracequery.Query{View: "window_stats", PID: 100, TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true}
	windowed := hmc317WorkerChurn(t, tracequery.Run(idx, q), 200)
	full, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	complete := hmc317WorkerChurn(t, tracequery.Run(full, q), 200)
	if !idx.Windowed || math.Abs(complete.SleepMs-36.5) > 1e-8 {
		t.Fatal("invalid scope characterization")
	}
	t.Logf("remaining windowed EOF handoff: local_last=%g windowed_sleep_ms=%g full_sleep_ms=%g; no windowed completion claim", idx.LastTs, windowed.SleepMs, complete.SleepMs)
}

func hmc317ActivePrefix() []string {
	return []string{hmc317Switch(1, 0, 41, "S"), hmc317Switch(1.001, 41, 0, "S"), hmc317Wake(1.002),
		hmc317Switch(1.003, 0, 41, "S"), hmc317Switch(1.004, 41, 0, "S"), hmc317Wake(1.005)}
}

func hmc317Switch(ts float64, prev, next int, state string) string {
	return fmt.Sprintf("emitter-7 (7) [001] .... %.6f: sched_switch: prev_comm=target prev_pid=%d prev_prio=120 prev_state=%s ==> next_comm=target next_pid=%d next_prio=120", ts, prev, state, next)
}

func hmc317Wake(ts float64) string {
	return fmt.Sprintf("waker-9 (9) [007] .... %.6f: sched_wakeup: comm=target pid=41 prio=120 target_cpu=001", ts)
}

func hmc317WriteTrace(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
