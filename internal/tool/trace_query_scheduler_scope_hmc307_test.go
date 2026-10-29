package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC307SchedulerScopePublicToolHandoff(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"scheduler_latency_stats", "window_stats"} {
		t.Run(view, func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("scheduler scope")}
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": view, "pid": 100, "time_start": .999, "time_end": 1.051, "min_duration_ms": .05})
			if !result.Success {
				t.Fatalf("public tool failed: %s", result.Summary)
			}
			var native tracequery.Result
			foundNative := false
			for _, obs := range result.Observations {
				if obs.SourceRef.PayloadRef == "" {
					continue
				}
				data, err := os.ReadFile(obs.SourceRef.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &native); err != nil {
					t.Fatal(err)
				}
				foundNative = true
				break
			}
			if !foundNative {
				t.Fatal("native payload receipt absent")
			}
			if view == "scheduler_latency_stats" {
				if native.SchedulerLatency == nil || len(native.SchedulerLatency.Items) != 1 || native.SchedulerLatency.Items[0].ContextScope == nil {
					t.Fatalf("scope lost in persisted payload: %+v", native.SchedulerLatency)
				}
			} else {
				seen := 0
				for _, obs := range result.Observations {
					if obs.Predicate != "runnable_context" || obs.Subject != "app-main-100" {
						continue
					}
					seen++
					if obs.Value != "1.000" || obs.Object != "insufficient_signal" || obs.SourceRef.QueryScopeID == "" || obs.SourceRef.Path != path || obs.SourceRef.PayloadRef == "" {
						t.Fatalf("identity, wait or authority changed: %+v", obs)
					}
					for _, want := range []string{"query_window_same_cpu_busy=7.000", "query_window_same_cpu_idle=45.000", "query_window_other_cpu_idle=37.500cpu_ms", "query_window=0.999000..1.051000", "wait_interval=[1.045000,1.046000)"} {
						if !strings.Contains(obs.Summary, want) {
							t.Errorf("observation lost %q: %s", want, obs.Summary)
						}
					}
					notes := strings.Join(obs.RichNotes, "\n")
					for _, want := range []string{"same_cpu_busy=7.000ms (query-window background 0.999000..1.051000; not wait-interval)", "same_cpu_idle=45.000ms (query-window background", "other_cpu_idle=37.500cpu_ms (query-window background", "high_prio_running=0.000ms (query-window background", "system_or_kernel_running=0.000ms (query-window background"} {
						if !strings.Contains(notes, want) {
							t.Errorf("rich note lost %q: %s", want, notes)
						}
					}
					if len(obs.Summary) > 1400 || len(notes) > 2200 || strings.Count(obs.Summary, "query_window=0.999000..1.051000") != 1 {
						t.Fatalf("scope teaching duplicates/bloats the handoff: summary=%d notes=%d", len(obs.Summary), len(notes))
					}
					projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: []types.ObservationRecord{obs}})
					if projection.PrimaryRootCause != nil || len(projection.RankedSeats)+len(projection.OnChainCauses) != 0 {
						t.Fatal("supporting background acquired causal authority")
					}
				}
				if seen != 1 {
					t.Fatalf("native scoped context not published exactly once: %d", seen)
				}
			}
			for _, want := range []string{"query_window_same_cpu_busy=7.000ms", "query_window_same_cpu_idle=45.000ms", "query_window_other_cpu_idle=37.500cpu_ms", "query_window=0.999000..1.051000", "wait_interval=[1.045000,1.046000)"} {
				if !strings.Contains(result.Summary, want) {
					t.Errorf("tool summary lost %q: %s", want, result.Summary)
				}
			}
		})
	}
}

func TestHMC307SchedulerScopeLegacyUnknownIsNotInvented(t *testing.T) {
	item := tracequery.RunnableContextSummary{Thread: tracequery.ThreadRef{PID: 7}, SameCPUBusyMs: 9, OtherCPUIdleMs: 37.5, Summary: "legacy native context"}
	for _, pair := range [][2]string{{traceQueryTypedRunnableContextSummary(item), "query_window=unknown"}, {strings.Join(traceQueryTypedRunnableContextNotes(item), "\n"), "query-window background unknown"}} {
		text := pair[0]
		if !strings.Contains(text, pair[1]) || strings.Contains(text, "query_window=0.000000") || !strings.Contains(text, "37.500cpu_ms") {
			t.Fatalf("legacy payload invented a wait window or lost CPU-time ruler: %s", text)
		}
	}
}
