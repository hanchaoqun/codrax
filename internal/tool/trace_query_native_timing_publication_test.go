package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeTimingTrace(t *testing.T, extra ...string) string {
	t.Helper()
	data, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	lines := append(strings.Split(strings.TrimSpace(string(data)), "\n"), extra...)
	stamp := func(line string) string {
		_, rest, ok := strings.Cut(line, ".... ")
		if !ok {
			return ""
		}
		ts, _, _ := strings.Cut(rest, ":")
		return ts
	}
	sort.SliceStable(lines, func(i, j int) bool { return stamp(lines[i]) < stamp(lines[j]) })
	return strings.Join(lines, "\n") + "\n"
}

func nativeTimingRankItem(t *testing.T, result tracequery.Result, kind string, pid int) tracequery.RootCauseRankItem {
	t.Helper()
	if result.RootCauseRank != nil {
		for _, item := range result.RootCauseRank.Items {
			if item.Type == kind && item.Thread.PID == pid {
				return item
			}
		}
	}
	var visible []string
	if result.RootCauseRank != nil {
		for _, item := range result.RootCauseRank.Items {
			visible = append(visible, fmt.Sprintf("%s/%d/%.3f", item.Type, item.Thread.PID, item.ImpactMs))
		}
		t.Fatalf("native %s/%d row absent in %s: visible=%v caveats=%v", kind, pid, result.View, visible, result.RootCauseRank.Caveats)
	}
	t.Fatalf("native %s/%d rank absent in %s", kind, pid, result.View)
	return tracequery.RootCauseRankItem{}
}

// Exercise the real parser, ranking cap and all three public rank-bearing
// views. The uncapped amount must reach the wire and typed observation, but
// the engine-owned score, ordering and causal lane must not change.
func TestNativeTimingPublicationPublicViews(t *testing.T) {
	extra := []string{
		"kernel-901 (901) [004] .... 1.003000: irq_handler_entry: irq=17 name=timer",
		"kernel-901 (901) [004] .... 1.048000: irq_handler_exit: irq=17 ret=handled",
		"kernel-902 (902) [005] .... 1.003000: ipi_entry: (Rescheduling interrupts)",
		"kernel-902 (902) [005] .... 1.048000: ipi_exit: (Rescheduling interrupts)",
		"worker-903 (903) [006] .... 1.003000: workqueue_execute_start: work=0xff function=flush_cookie",
		"worker-903 (903) [006] .... 1.048000: workqueue_execute_end: work=0xff function=flush_cookie",
		"display-904 (904) [007] .... 1.003000: dma_fence_wait_start: driver=display timeline=present context=7 seqno=9",
		"display-904 (904) [007] .... 1.048000: dma_fence_wait_end: driver=display timeline=present context=7 seqno=9",
	}
	for _, view := range []string{"root_cause_rank", "trace_perf_bundle", "frame_root_cause_bundle"} {
		for index, tc := range []struct {
			kind string
			pid  int
			ms   float64
		}{{"io_latency", 900, 47}, {"irq_activity", 0, 45}, {"ipi_activity", 0, 45}, {"workqueue_activity", 903, 45}, {"dma_fence_activity", 904, 45}} {
			t.Run(view+"/"+tc.kind, func(t *testing.T) {
				body := nativeTimingTrace(t)
				if index > 0 {
					body = nativeTimingTrace(t, extra[(index-1)*2:index*2]...)
				}
				if tc.pid == 0 {
					// A two-thread, fully observed dependency avoids filling the
					// existing side budget with unrelated missing-thread gaps.
					body = strings.Join([]string{
						"worker-200 (200) [002] .... 0.998000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=120",
						"app-main-100 (100) [001] .... 0.999000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app-main next_pid=100 next_prio=53",
						"app-main-100 (100) [001] .... 1.001000: sched_switch: prev_comm=app-main prev_pid=100 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120",
						extra[(index-1)*2],
						"worker-200 (200) [002] .... 1.045000: sched_wakeup: comm=app-main pid=100 prio=53 target_cpu=001",
						"app-main-100 (100) [001] .... 1.046000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app-main next_pid=100 next_prio=53",
						extra[index*2-1],
						"worker-200 (200) [002] .... 1.053000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120",
						"app-main-100 (100) [001] .... 1.054000: sched_switch: prev_comm=app-main prev_pid=100 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120",
					}, "\n") + "\n"
				}
				ctx, path := businessRefTestContext(t, body)
				path, err := filepath.EvalSymlinks(path)
				if err != nil {
					t.Fatal(err)
				}
				idx, err := tracequery.BuildIndex(context.Background(), path)
				if err != nil {
					t.Fatal(err)
				}
				query := tracequery.Query{View: view, PID: 100, TimeStart: .999, TimeEnd: 1.052, Limit: 64}
				native := tracequery.Run(idx, query)
				before, err := json.Marshal(native)
				if err != nil {
					t.Fatal(err)
				}
				params, _ := json.Marshal(map[string]any{"path": path, "view": view, "pid": 100, "time_start": .999, "time_end": 1.052, "limit": 64})
				published, err := (&TraceQuery{}).Execute(ctx, params)
				if err != nil || !published.Success {
					t.Fatalf("public query failed: %v %+v", err, published)
				}
				original := nativeTimingRankItem(t, native, tc.kind, tc.pid)
				if math.Abs(original.ImpactMs-18.55) > 1e-6 || math.Abs(original.CumulativeImpactMs-tc.ms) > 1e-6 {
					t.Fatalf("fixture must exercise a real cap over original timing: %+v", original)
				}
				var record *types.ObservationRecord
				for i := range published.Observations {
					r := &published.Observations[i]
					if r.Object == tc.kind && strings.Contains(r.ID, "#root_cause_rank:") {
						if tc.pid == 0 || r.Subject == traceThreadLabel(original.Thread) {
							record = r
							break
						}
					}
				}
				if record == nil {
					t.Fatalf("typed background %s row missing", tc.kind)
				}
				if record.Value != fmt.Sprintf("%.3f", tc.ms) || record.Unit != "ms" {
					t.Errorf("ranking cap escaped as physical timing: kind=%s measured=%.3f cap=%.3f public=%s%s", tc.kind, tc.ms, original.ImpactMs, record.Value, record.Unit)
				}
				if !strings.Contains(strings.Join(record.RichNotes, "\n"), "rank_value_caliber=native_duration") {
					t.Error("published measured amount lacks its display-only caliber")
				}
				if record.Role != types.AnswerAggregateRoleSupportingCoverage || record.SourceRef.Path != path || record.SourceRef.QueryWindowStartTs != .999 || record.SourceRef.QueryWindowEndTs != 1.052 || !strings.Contains(strings.Join(record.RichNotes, "\n"), "effective_impact_ms=0.000") || !strings.Contains(strings.Join(record.RichNotes, "\n"), "chain_relevance=background") {
					t.Errorf("scope or noncausal qualification changed: %+v", record)
				}
				data, err := os.ReadFile(record.SourceRef.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				var wire tracequery.Result
				if err := json.Unmarshal(data, &wire); err != nil {
					t.Fatal(err)
				}
				item := nativeTimingRankItem(t, wire, tc.kind, tc.pid)
				if math.Abs(item.ImpactMs-tc.ms) > 1e-6 || math.Abs(item.ProjectedImpactMs-tc.ms) > 1e-6 || item.EffectiveImpactMs != 0 || item.Score != original.Score || item.Rank != original.Rank || item.Source != original.Source || item.ChainRelevance != original.ChainRelevance {
					t.Errorf("published timing/ordering/authority drift: native=%+v wire=%+v", original, item)
				}
				if view == "root_cause_rank" {
					// The compact banner must agree with the typed/wire face on
					// this exact row, not merely contain the amount elsewhere.
					summary := traceQuerySummary(native, traceQueryParams{View: view}, path, record.SourceRef.PayloadRef)
					found := false
					for _, line := range strings.Split(summary, "\n") {
						if strings.HasPrefix(line, "- rank=") && strings.Contains(line, " type="+tc.kind+" ") && strings.Contains(line, " thread="+traceThreadLabel(original.Thread)+" ") {
							found = true
							if !strings.Contains(line, fmt.Sprintf(" impact=%.3fms ", tc.ms)) {
								t.Errorf("banner retained another ruler: %s", line)
							}
						}
					}
					if !found {
						t.Error("native timing row missing from direct rank banner")
					}
				}
				if tc.pid != 0 {
					chainIO := nativeTimingRankItem(t, native, "io_latency", 200)
					if math.Abs(chainIO.ImpactMs-31) > 1e-6 {
						t.Fatalf("on-chain blocked account drift: %+v", chainIO)
					}
					chainWire := nativeTimingRankItem(t, wire, "io_latency", 200)
					if chainWire.ImpactMs != chainIO.ImpactMs || chainWire.EffectiveImpactMs != chainIO.EffectiveImpactMs {
						t.Fatal("publication altered chain IO blocked account")
					}
					requestFound := false
					if wire.WindowStats != nil {
						for _, request := range wire.WindowStats.IOLatencies {
							if request.IssueThread.PID == 200 {
								requestFound = true
								if math.Abs(request.DurationMs-35) > 1e-6 || math.Abs(request.IssuerBlockedMs-31) > 1e-6 {
									t.Errorf("separate physical request/blocked accounts changed: %+v", request)
								}
							}
						}
					}
					if !requestFound {
						t.Fatal("physical chain-host request lost")
					}
				}
				_ = traceQueryPriorityResultForPublication(native)
				after, _ := json.Marshal(native)
				if string(before) != string(after) {
					t.Fatal("publication mutated engine-owned result")
				}
			})
		}
	}
}

func TestNativeTimingPublicationIOFoldsAndWindow(t *testing.T) {
	for _, tc := range []struct {
		name           string
		extra          []string
		start, end, ms float64
		caliber        string
	}{
		{name: "single", start: .999, end: 1.052, ms: 47},
		{name: "clipped_request", start: 1.01, end: 1.048, ms: 38},
		{name: "no_cap", start: .8, end: 1.2, ms: 47},
		{name: "disjoint", start: .999, end: 1.1, ms: 67, caliber: tracequery.RootCauseMemberFoldCaliberSumDisjoint, extra: []string{
			"backup-900 (900) [003] .... 1.060000: block_rq_issue: 12,80 W 65536 () 800128 + 128 [backup]",
			"backup-irq-81 (2) [003] .... 1.080000: block_rq_complete: 12,80 W () 800128 + 128 [0]",
		}},
		{name: "overlap_union", start: .999, end: 1.052, ms: 47, caliber: tracequery.RootCauseMemberFoldCaliberIntervalUnion, extra: []string{
			"backup-900 (900) [003] .... 1.010000: block_rq_issue: 12,80 W 65536 () 800128 + 128 [backup]",
			"backup-irq-81 (2) [003] .... 1.040000: block_rq_complete: 12,80 W () 800128 + 128 [0]",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, nativeTimingTrace(t, tc.extra...))
			params, _ := json.Marshal(map[string]any{"path": path, "view": "root_cause_rank", "pid": 100, "time_start": tc.start, "time_end": tc.end})
			result, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil || !result.Success {
				t.Fatalf("query: %v %s", err, result.Summary)
			}
			found := false
			for _, record := range result.Observations {
				if record.Object != "io_latency" || record.Subject != "backup-900" || record.Predicate != "root_cause_background" {
					continue
				}
				found = true
				if record.Value != fmt.Sprintf("%.3f", tc.ms) {
					t.Errorf("query/fold measured ruler not preserved: want=%.3f got=%s notes=%v", tc.ms, record.Value, record.RichNotes)
				}
				data, _ := os.ReadFile(record.SourceRef.PayloadRef)
				var wire tracequery.Result
				if err := json.Unmarshal(data, &wire); err != nil {
					t.Fatal(err)
				}
				item := nativeTimingRankItem(t, wire, "io_latency", 900)
				if item.MemberFoldCaliber != tc.caliber || math.Abs(item.CumulativeImpactMs-tc.ms) > 1e-6 {
					t.Errorf("raw fold accounting changed: %+v", item)
				}
			}
			if !found {
				t.Fatal("background request record missing")
			}
		})
	}
}

// Each work item has real paired executions, but the first work item's two
// executions enclose the second item's hull. The established family rule can
// only publish the largest measured member, not the hull or naive member sum.
func TestNativeTimingPublicationNativeMaxOverlapFallback(t *testing.T) {
	body := nativeTimingTrace(t,
		"worker-903 (903) [006] .... 1.002000: workqueue_execute_start: work=0xaa function=first",
		"worker-903 (903) [006] .... 1.023000: workqueue_execute_end: work=0xaa function=first",
		"worker-903 (903) [006] .... 1.025000: workqueue_execute_start: work=0xbb function=second",
		"worker-903 (903) [006] .... 1.047000: workqueue_execute_end: work=0xbb function=second",
		"worker-903 (903) [006] .... 1.048000: workqueue_execute_start: work=0xaa function=first",
		"worker-903 (903) [006] .... 1.050000: workqueue_execute_end: work=0xaa function=first",
	)
	ctx, path := businessRefTestContext(t, body)
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	native := tracequery.Run(idx, tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: .999, TimeEnd: 1.052})
	owner := nativeTimingRankItem(t, native, "workqueue_activity", 903)
	if owner.MemberFoldCaliber != tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback || owner.MemberCount != 2 || math.Abs(owner.CumulativeImpactMs-23) > 1e-6 || math.Abs(owner.ImpactMs-18.55) > 1e-6 || math.Abs(owner.MemberSumMs-45) > 1e-6 {
		t.Fatalf("fixture must exercise native lower-bound fold, not a hull/naive sum: %+v", owner)
	}
	params, _ := json.Marshal(map[string]any{"path": path, "view": "root_cause_rank", "pid": 100, "time_start": .999, "time_end": 1.052})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	for _, record := range result.Observations {
		if record.Object != "workqueue_activity" || record.Subject != "worker-903" || record.Predicate != "root_cause_background" {
			continue
		}
		if record.Value != "23.000" || !strings.Contains(strings.Join(record.RichNotes, "\n"), "member_fold_caliber=max_overlap_fallback") || !strings.Contains(strings.Join(record.RichNotes, "\n"), "member_sum_ms=45.000") {
			t.Fatalf("lost native lower bound or its explicit non-additive ruler: %+v", record)
		}
		return
	}
	t.Fatal("native work family was not published")
}

func TestNativeTimingPublicationClosedProducerAndCaliber(t *testing.T) {
	sources := map[string]string{"io_latency": "window_stats", "irq_activity": "window_stats.irq_activity", "ipi_activity": "window_stats.ipi_activity", "workqueue_activity": "window_stats.workqueue_activity", "dma_fence_activity": "window_stats.dma_fence_activity"}
	for kind, source := range sources {
		for _, caliber := range []string{"", tracequery.RootCauseMemberFoldCaliberSumDisjoint, tracequery.RootCauseMemberFoldCaliberIntervalUnion, tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback} {
			t.Run(kind+"/"+caliber, func(t *testing.T) {
				item := tracequery.RootCauseRankItem{Type: kind, Source: source, ImpactMs: 7, ProjectedImpactMs: 7, CumulativeImpactMs: 47, EffectiveImpactMs: 7, Score: 3, ChainRelevance: "background", MemberFoldCaliber: caliber}
				if caliber != "" {
					item.MemberCount = 2
				}
				want := item
				want.ImpactMs, want.ProjectedImpactMs, want.EffectiveImpactMs = 47, 47, 0
				got := traceQueryRootCauseRankWireItemForPublicationInUniverse(item, traceQueryPriorityOpenArtifactUniverse())
				if !reflect.DeepEqual(got, want) {
					t.Errorf("native producer-owned measure not restored: got=%+v want=%+v", got, want)
				}
				if again := traceQueryRootCauseRankWireItemForPublicationInUniverse(got, traceQueryPriorityOpenArtifactUniverse()); !reflect.DeepEqual(again, got) {
					t.Fatal("publication is not idempotent")
				}
			})
		}
	}
	base := tracequery.RootCauseRankItem{Type: "io_latency", Source: "window_stats", ImpactMs: 7, ProjectedImpactMs: 7, CumulativeImpactMs: 47, Score: 3, ChainRelevance: "background"}
	for name, mutate := range map[string]func(*tracequery.RootCauseRankItem){
		"absent_measure":   func(i *tracequery.RootCauseRankItem) { i.CumulativeImpactMs = 0 },
		"negative_measure": func(i *tracequery.RootCauseRankItem) { i.CumulativeImpactMs = -47 },
		"nan_measure":      func(i *tracequery.RootCauseRankItem) { i.CumulativeImpactMs = math.NaN() },
		"infinite_measure": func(i *tracequery.RootCauseRankItem) { i.CumulativeImpactMs = math.Inf(1) },
		"unknown_source":   func(i *tracequery.RootCauseRankItem) { i.Source = "window_stats.future" },
		"wrong_source":     func(i *tracequery.RootCauseRankItem) { i.Source = "wakeup_chain" },
		"missing_source":   func(i *tracequery.RootCauseRankItem) { i.Source = "" },
		"missing_fold":     func(i *tracequery.RootCauseRankItem) { i.MemberCount = 2 },
		"unknown_fold":     func(i *tracequery.RootCauseRankItem) { i.MemberCount = 2; i.MemberFoldCaliber = "future" },
		"count_fold": func(i *tracequery.RootCauseRankItem) {
			i.MemberCount = 2
			i.MemberFoldCaliber = tracequery.RootCauseMemberFoldCaliberCountSum
		},
		"on_chain": func(i *tracequery.RootCauseRankItem) { i.ChainRelevance = "on_chain" },
		"adjacent": func(i *tracequery.RootCauseRankItem) { i.ChainRelevance = "adjacent" },
	} {
		t.Run(name, func(t *testing.T) {
			item := base
			mutate(&item)
			got := traceQueryRootCauseRankWireItemForPublicationInUniverse(item, traceQueryPriorityOpenArtifactUniverse())
			if got.ImpactMs != 7 || got.ProjectedImpactMs != 7 {
				t.Fatalf("unqualified amount relabeled measured: %+v", got)
			}
		})
	}
	for _, kind := range []string{"page_cache_churn", "block_io_by_inode", "io_pressure", "file_io_hot_inode", "blocking_span", "io_burst_episode", "cpu_pressure", "supply_pressure", "sched_stat_accounting", "low_frequency", "future"} {
		item := base
		item.Type = kind
		got := traceQueryRootCauseRankWireItemForPublicationInUniverse(item, traceQueryPriorityOpenArtifactUniverse())
		if got.ImpactMs != 7 || got.ProjectedImpactMs != 7 {
			t.Errorf("non-native-timing family borrowed cumulative measure: %s %+v", kind, got)
		}
	}
}
