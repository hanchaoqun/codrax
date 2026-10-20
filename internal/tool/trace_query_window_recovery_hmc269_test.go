package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This public-tool fixture retains the real 250K event bound and 64MiB byte
// threshold: a small but dense prefix is unrelated to the explicit target
// window. Lowering either threshold would not reproduce the production gap.
func hmc269DenseOutsideWindow(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ordinary.ftrace")
	prefix := "noise-9 (9) [001] .... 0.100000: cpu_frequency: state=1000000 cpu_id=1\n"
	body := strings.Repeat(prefix, defaultTraceIndexEventCapForTest+1) + strings.Join([]string{
		"target-41 (40) [001] .... 0.900000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120",
		"target-41 (40) [001] .... 0.950000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
		"loader-2 (2) [002] .... 1.010000: sched_wakeup: comm=target pid=41 prio=120 target_cpu=001",
		"target-41 (40) [001] .... 1.020000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120",
		"target-41 (40) [001] .... 1.040000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
		"noise-9 (9) [001] .... 1.200000: cpu_frequency: state=1000000 cpu_id=1",
		"",
	}, "\n")
	if len(body) >= 64<<20 {
		t.Fatalf("fixture is not the small-file branch: %d", len(body))
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func TestHMC269SmallDenseExplicitWindowPublicRecovery(t *testing.T) {
	path := hmc269DenseOutsideWindow(t)
	for _, view := range []string{"thread_timeline", "wakeup_chain", "window_stats", "interaction_stats"} {
		t.Run(view, func(t *testing.T) {
			for attempt := 0; attempt < 2; attempt++ {
				bus := &types.BusContext{RepoRoot: filepath.Dir(path), WorkDir: t.TempDir(), Mutable: types.NewMutableState("target state")}
				result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": 41, "time_start": 1.0, "time_end": 1.05})
				payload := hmc17NamedPayload(t, result)
				if payload.View != view || !payload.IndexWindowed || strings.Contains(result.Summary, "mode=index_event_limit") || strings.Contains(strings.Join(payload.Caveats, " "), "stream_state_cluster") {
					t.Fatalf("attempt=%d requested heavy view must execute, not a state-cluster fallback: view=%s windowed=%t caveats=%v summary=%s", attempt, payload.View, payload.IndexWindowed, payload.Caveats, result.Summary)
				}
				if payload.SourcePath != path || math.Abs(payload.TimeStart-1.0) > 1e-9 || math.Abs(payload.TimeEnd-1.05) > 1e-9 {
					t.Fatalf("source or requested scope drift: %+v", payload)
				}
				if payload.EventCount > defaultTraceIndexEventCapForTest {
					t.Fatalf("recovery raised the original event budget: %d", payload.EventCount)
				}
				switch view {
				case "thread_timeline":
					if payload.Timeline == nil || len(payload.Timeline.Intervals) == 0 {
						t.Fatal("native timeline absent")
					}
				case "wakeup_chain":
					chain, _ := json.Marshal(payload.WakeupChain)
					if payload.WakeupChain == nil || !strings.Contains(string(chain), "loader") {
						t.Fatalf("native waker chain absent: %+v", payload.WakeupChain)
					}
				case "window_stats":
					if payload.WindowStats == nil {
						t.Fatal("native window statistics absent")
					}
				case "interaction_stats":
					if payload.InteractionStats == nil {
						t.Fatal("native interaction statistics absent")
					}
				}
			}
		})
	}
}

func TestHMC269SmallDenseRecoveryDoesNotRaiseBudgetOrGuessWindow(t *testing.T) {
	path := hmc269DenseOutsideWindow(t)
	for _, tc := range []struct {
		name     string
		p        traceQueryParams
		from, to float64
	}{
		{"no_window", traceQueryParams{View: "window_stats", PID: 41}, 0, 0},
		{"dense_explicit_window", traceQueryParams{View: "window_stats", PID: 41, TimeStart: traceSecondFromAutoWindow(.05), TimeEnd: traceSecondFromAutoWindow(.2)}, .05, .2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := traceQueryBuildIndex(context.Background(), path, tc.p, tc.from, tc.to)
			var limit *tracequery.IndexEventLimitError
			if !errors.As(err, &limit) || limit.MaxEvents != defaultTraceIndexEventCapForTest {
				t.Fatalf("must retain the actual default budget, not guess/narrow a window or raise it: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := traceQueryParams{View: "thread_timeline", PID: 41, TimeStart: traceSecondFromAutoWindow(1), TimeEnd: traceSecondFromAutoWindow(1.05)}
	if _, err := traceQueryBuildIndex(ctx, path, p, 1, 1.05); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must not recover: %v", err)
	}
	if _, err := traceQueryBuildIndex(context.Background(), path+".absent", p, 1, 1.05); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing physical source must not recover: %v", err)
	}
}

type hmc269ErrorCycle struct{}

func (e *hmc269ErrorCycle) Error() string { return "cycle" }
func (e *hmc269ErrorCycle) Unwrap() error { return e }

func TestHMC269WindowRecoveryOnlySoleTypedBudgetFailure(t *testing.T) {
	limit := &tracequery.IndexEventLimitError{MaxEvents: defaultTraceIndexEventCapForTest}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"typed", limit, true},
		{"wrapped", fmt.Errorf("build: %w", limit), true},
		{"single_join", errors.Join(limit), true},
		{"physical_join", errors.Join(limit, os.ErrPermission), false},
		{"canceled_join", errors.Join(limit, context.Canceled), false},
		{"deadline_join", errors.Join(limit, context.DeadlineExceeded), false},
		{"text_only", errors.New(limit.Error()), false},
		{"cycle", &hmc269ErrorCycle{}, false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if (traceQuerySoleIndexLimit(tc.err) != nil) != tc.want {
				t.Fatalf("wrong recovery authority: %v", tc.err)
			}
			p := traceQueryParams{View: "thread_timeline", TimeStart: traceSecondFromAutoWindow(1), TimeEnd: traceSecondFromAutoWindow(1.05)}
			opts := traceQueryWindowedIndexOptions(p, 1, 1.05)
			opts.MaxEvents = defaultTraceIndexEventCapForTest
			retry, allowed := traceQueryReducedPaddingRetryOptions(p, opts, tc.err, 1, 1.05)
			if allowed != tc.want || (allowed && retry.MaxEvents != opts.MaxEvents) {
				t.Fatalf("padding recovery changed failure authority/budget: allowed=%t retry=%+v", allowed, retry)
			}
		})
	}
}

func TestHMC269SmallDensePublicQueryThenExplicitWindowSupplement(t *testing.T) {
	// Keep the real 250K-event fixture under the race detector without making
	// instrumentation overhead a product deadline test. Production limits and
	// all event/window budgets are unchanged; this local knob is restored.
	suppCoreSetConfig(t, true, 32<<20, 2*time.Minute, 120)
	path := hmc269DenseOutsideWindow(t)
	bus := &types.BusContext{RepoRoot: filepath.Dir(path), WorkDir: t.TempDir(), Mutable: types.NewMutableState("target state")}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:                      types.IntentRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: requestWindowTestProfile(1, 1.05),
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
	}}
	// A wider exploratory request must not replace the explicit requested
	// window used by the normal supplement.
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 41, "time_start": .9, "time_end": 1.1})
	_ = hmc17NamedPayload(t, result)
	bus.ToolResults = append(bus.ToolResults, result)
	before, _ := json.Marshal(bus.ToolResults)
	out := RunTraceQuerySystemSupplement(bus)
	if len(out.Executed) == 0 {
		t.Fatalf("normal supplement did not execute: %+v", out)
	}
	found := false
	for _, result := range bus.Mutable.SystemTraceSupplementResults() {
		payload := hmc17NamedPayload(t, result)
		if payload.SourcePath != path || payload.TimeStart != 1 || payload.TimeEnd != 1.05 || !payload.IndexWindowed {
			t.Fatalf("supplement lost native exact source/window: %+v", payload)
		}
		if payload.RootCauseRank != nil {
			found = payload.RootCauseRank.Target.PID == 41 && payload.EventCount > 0
		}
	}
	if !found {
		t.Fatalf("native causal supplement missing: %+v", out)
	}
	after, _ := json.Marshal(bus.ToolResults)
	if string(before) != string(after) {
		t.Fatal("supplement rewrote exploration")
	}
}
