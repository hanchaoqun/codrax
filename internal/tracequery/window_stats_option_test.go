package tracequery

// Exercise public option construction without depending on its private flag.
// Reuses the real parsed scheduler fixture from run_cancel_test.go; no stubbed
// Run, synthetic Result, reference service, or model evaluation is involved.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWindowStatsOptionDefaultExplicitAndRepeatedNormalization(t *testing.T) {
	idx := runCancelIndex(t)
	base := runCancelQuery("wakeup_chain")
	before := base
	legacyTrue := base
	legacyTrue.IncludeWindowStats = true
	var chainJSON []byte
	for _, tc := range []struct {
		name string
		q    Query
		want bool
	}{
		{"omitted", base, true},
		// A legacy bool literal has no presence bit and keeps its old default.
		{"legacy_false_literal", Query{View: "wakeup_chain", PID: 200, Thread: "worker", TimeStart: 3, TimeEnd: 3.2, TimeStartSet: true, TimeEndSet: true, IncludeWindowStats: false}, true},
		{"legacy_true_literal", legacyTrue, true},
		{"explicit_false", base.WithWindowStats(false), false},
		{"explicit_true", base.WithWindowStats(true), true},
		{"true_then_false", base.WithWindowStats(true).WithWindowStats(false), false},
		{"false_then_true", base.WithWindowStats(false).WithWindowStats(true), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputBefore := tc.q
			once := normalizeQuery(idx, tc.q)
			twice := normalizeQuery(idx, once)
			if once.IncludeWindowStats != tc.want || twice.IncludeWindowStats != tc.want {
				t.Fatalf("normalization lost explicit/default option: once=%v twice=%v want=%v", once.IncludeWindowStats, twice.IncludeWindowStats, tc.want)
			}
			if !reflect.DeepEqual(once, twice) {
				t.Fatal("normalizing the already normalized query changed it")
			}
			if !reflect.DeepEqual(tc.q, inputBefore) {
				t.Fatal("normalization mutated its caller's query")
			}
			result := Run(idx, tc.q)
			if (result.WindowStats != nil) != tc.want {
				t.Fatalf("Run WindowStats present=%v, want %v", result.WindowStats != nil, tc.want)
			}
			if result.WakeupChain == nil || len(result.WakeupChain.Edges) == 0 {
				t.Fatalf("fixture did not retain a real nonempty wakeup chain: %+v", result.WakeupChain)
			}
			got := windowStatsOptionEngineJSON(t, result.WakeupChain)
			if chainJSON == nil {
				chainJSON = got
			} else if !bytes.Equal(chainJSON, got) {
				t.Fatal("optional statistics changed the published causal chain")
			}
		})
	}
	if !reflect.DeepEqual(base, before) {
		t.Fatal("WithWindowStats or Run mutated the original query")
	}
	// The default is specific to wakeup_chain, not a new all-view default.
	if got := normalizeQuery(idx, runCancelQuery("event_search")); got.IncludeWindowStats {
		t.Fatal("optional wakeup statistics default leaked into another view")
	}
}

func TestWindowStatsOptionCopiesAndRunContextComposition(t *testing.T) {
	idx := runCancelIndex(t)
	base := runCancelQuery("wakeup_chain")
	for _, name := range []string{"option_then_context", "context_then_option"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var q Query
			if name == "option_then_context" {
				q = base.WithWindowStats(false).WithRunContext(ctx)
			} else {
				q = base.WithRunContext(ctx).WithWindowStats(false)
			}
			// Same value-copy pattern used by bounded/nested engine queries.
			bounded := q
			bounded.TimeStart, bounded.TimeEnd = 3.01, 3.08
			if got := normalizeQuery(idx, bounded); got.IncludeWindowStats {
				t.Fatal("bounded value copy lost explicit false")
			}
			if q.TimeStart != base.TimeStart || q.TimeEnd != base.TimeEnd {
				t.Fatal("editing a bounded copy mutated its source query")
			}
			plain := Run(idx, base.WithWindowStats(false))
			live := Run(idx, q)
			if !bytes.Equal(windowStatsOptionEngineJSON(t, plain), windowStatsOptionEngineJSON(t, live)) {
				t.Fatal("live cancellation context changed the optional-statistics result")
			}
			if live.WindowStats != nil || live.WakeupChain == nil || len(live.WakeupChain.Edges) == 0 {
				t.Fatal("composed query did not retain chain-only output")
			}
			cancel()
			canceled := Run(idx, q)
			if canceled.ViewCancellation == nil || canceled.ViewCancellation.Reason != "canceled" {
				t.Fatalf("WithWindowStats lost the cancellation carrier: %+v", canceled.ViewCancellation)
			}
			assertRunCancelNoPartialFaces(t, "wakeup_chain", canceled)
		})
	}
}

func TestWindowStatsOptionDoesNotAddQueryJSONFields(t *testing.T) {
	base := runCancelQuery("wakeup_chain")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, include := range []bool{false, true} {
		want := base
		want.IncludeWindowStats = include
		got := base.WithWindowStats(include).WithRunContext(ctx)
		if !bytes.Equal(windowStatsOptionEngineJSON(t, want), windowStatsOptionEngineJSON(t, got)) {
			t.Fatalf("presence/context plumbing changed the exported Query JSON for include=%v", include)
		}
	}
	// This intentionally does NOT promise Query JSON round-trip restoration of
	// presence. The tool boundary retains presence and publishes an effective
	// bool; raw engine callers opt in through WithWindowStats.
}

func TestWindowStatsOptionDoesNotChangeBoardOrRequiredStatistics(t *testing.T) {
	idx := runCancelIndex(t)
	base := runCancelQuery("root_cause_rank")
	wantFingerprint := rootCauseBoardParamsFingerprint(normalizeQuery(idx, base))
	if wantFingerprint == "" {
		t.Fatal("fixture has no board fingerprint")
	}
	for _, q := range []Query{base, base.WithWindowStats(false), base.WithWindowStats(true)} {
		if got := rootCauseBoardParamsFingerprint(normalizeQuery(idx, q)); got != wantFingerprint {
			t.Fatalf("output option split board identity: got=%s want=%s", got, wantFingerprint)
		}
	}
	for _, view := range []string{"root_cause_rank", "recipe"} {
		t.Run(view, func(t *testing.T) {
			q := runCancelQuery(view)
			if view == "recipe" {
				q.RecipeName = "sleep_root_cause"
			}
			baseline := Run(idx, q)
			if baseline.WindowStats == nil || baseline.RootCauseRank == nil || len(baseline.RootCauseRank.Items) == 0 || baseline.RootCauseRank.BoardParamsFingerprint == "" {
				t.Fatal("fixture did not build required statistics and a nonempty real rank board")
			}
			for _, include := range []bool{false, true} {
				result := Run(idx, q.WithWindowStats(include))
				if result.WindowStats == nil || result.RootCauseRank == nil {
					t.Fatalf("include=%v removed this view's required statistics/rank", include)
				}
				if !bytes.Equal(windowStatsOptionEngineJSON(t, baseline), windowStatsOptionEngineJSON(t, result)) {
					t.Fatalf("include=%v changed required statistics, board identity, or result content", include)
				}
			}
		})
	}
}

func TestWindowStatsOptionRelationScopedUnavailableOnlyWhenRequested(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-stats-option.systrace")
	body := strings.Join([]string{
		`noise-99 ( 99) [003] .... 0.500000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=noise next_pid=99 next_prio=20`,
		`app-20 ( 20) [001] .... 0.600000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 ( 10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`,
		`app-20 ( 20) [001] .... 1.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`noise-99 ( 99) [003] .... 1.100000: sched_switch: prev_comm=noise prev_pid=99 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, scoped := range []bool{false, true} {
		name := "full_window_index"
		if scoped {
			name = "relation_scoped_index"
		}
		t.Run(name, func(t *testing.T) {
			idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
				TimeStart: 1, TimeEnd: 1.2, TimeStartSet: true, TimeEndSet: true,
				AllowWindowedParse: true, ScopePID: 20, RelationScoped: scoped,
			})
			if err != nil {
				t.Fatal(err)
			}
			if idx.RelationScoped != scoped {
				t.Fatalf("fixture index scope=%v, want %v", idx.RelationScoped, scoped)
			}
			base := Query{View: "wakeup_chain", PID: 20, TimeStart: 1, TimeEnd: 1.2, TimeStartSet: true, TimeEndSet: true}
			var chainJSON []byte
			for _, tc := range []struct {
				name string
				q    Query
				want bool
			}{
				{"omitted", base, true},
				{"false", base.WithWindowStats(false), false},
				{"true", base.WithWindowStats(true), true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					result := Run(idx, tc.q)
					if (result.WindowStats != nil) != (tc.want && !scoped) {
						t.Fatalf("statistics availability disagrees with option/index authority: scoped=%v want=%v stats=%v", scoped, tc.want, result.WindowStats != nil)
					}
					unavailable := strings.Contains(strings.Join(result.Caveats, "\n"), "relation_scoped_window_stats_unavailable")
					if unavailable != (scoped && tc.want) {
						t.Fatalf("requested-unavailable disclosure=%v, want %v; caveats=%v", unavailable, scoped && tc.want, result.Caveats)
					}
					if result.WakeupChain == nil || len(result.WakeupChain.Edges) == 0 {
						t.Fatal("real indexed fixture did not retain wakeup edges")
					}
					got := windowStatsOptionEngineJSON(t, result.WakeupChain)
					if chainJSON == nil {
						chainJSON = got
					} else if !bytes.Equal(chainJSON, got) {
						t.Fatal("statistics option changed the same index's causal chain")
					}
				})
			}
		})
	}
}

func windowStatsOptionEngineJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(payload) {
		t.Fatal("fixture/result serialization produced invalid JSON")
	}
	return payload
}
