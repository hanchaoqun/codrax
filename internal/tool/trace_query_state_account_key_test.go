package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryPublishesStateAccountKeyOnBothTypedRows(t *testing.T) {
	const key = "state_account:v1:typed-wire-pin"
	result := tracequery.Result{
		View:       "root_cause_rank",
		SourcePath: "/traces/app.systrace",
		WindowStats: &tracequery.WindowStats{
			Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
			StateChurn: []tracequery.ThreadStateChurnSummary{{
				Thread: tracequery.ThreadRef{Comm: "app", PID: 20}, StateAccountKey: key,
				DominantState: string(tracequery.StateRunnable), DominantImpactMs: 5,
				TotalMs: 8, RunningMs: 3, RunnableMs: 5, FragmentCount: 20, StateSwitches: 19,
				MaxSegmentMs: 0.5, P95SegmentMs: 0.5,
				Summary: "dominant_state=runnable impact=5.000ms total=8.000ms fragments=20 switches=19 max_segment=0.500ms p95_segment=0.500ms totals running=3.000ms runnable=5.000ms sleep=0.000ms d_state=0.000ms io_wait=0.000ms",
			}},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
			Items: []tracequery.RootCauseRankItem{{
				Rank:              1,
				Tier:              "primary",
				Type:              "runnable_wait",
				Thread:            tracequery.ThreadRef{Comm: "app", PID: 20},
				StateAccountKey:   key,
				DominantState:     string(tracequery.StateRunnable),
				RunnableMs:        5,
				ImpactMs:          5,
				EffectiveImpactMs: 5,
				LineStart:         4,
				LineEnd:           23,
			}},
		},
		WakeupChain: &tracequery.ChainResult{
			Target: tracequery.ThreadRef{Comm: "app", PID: 20},
			Window: tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
			CausalImpacts: []tracequery.WakeupCausalImpact{{
				Thread:           tracequery.ThreadRef{Comm: "app", PID: 20},
				Window:           tracequery.TimeWindow{StartTs: 11, EndTs: 11.008},
				StateAccountKey:  key,
				OnChain:          true,
				DominantState:    string(tracequery.StateRunnable),
				DominantImpactMs: 5,
				TotalMs:          8,
				RunningMs:        3,
				RunnableMs:       5,
				FragmentCount:    21,
				StateSwitches:    20,
				MaxSegmentMs:     0.5,
				P95SegmentMs:     0.5,
				Summary:          "dominant_state=runnable impact=5.000ms total=8.000ms fragments=21 switches=20 max_segment=0.500ms p95_segment=0.500ms totals running=3.000ms runnable=5.000ms sleep=0.000ms d_state=0.000ms io_wait=0.000ms",
				LineStart:        3,
				LineEnd:          23,
			}},
		},
	}

	rows := traceQueryTypedObservations(result, "trace", "/blobs/trace.json", "", "", time.Now())
	found := map[string]bool{}
	for _, row := range rows {
		if row.Predicate != "root_cause_primary" && row.Predicate != "wakeup_causal_impact" && row.Predicate != "state_churn" {
			continue
		}
		want := types.TraceNoteKeyStateAccountKey + "=" + key
		for _, note := range row.RichNotes {
			if strings.TrimSpace(note) == want {
				found[row.Predicate] = true
			}
		}
	}
	for _, predicate := range []string{"root_cause_primary", "wakeup_causal_impact", "state_churn"} {
		if !found[predicate] {
			t.Fatalf("%s must publish the exact state-account wire; rows=%+v", predicate, rows)
		}
	}

	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: rows,
	}}
	items := runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 1 || !strings.Contains(items[0].Text, "19 次切换/20 段") {
		t.Fatalf("typed result must publish the canonical whole-window account once: %+v", items)
	}

	// The system supplement executes the same exact bounded query into a
	// distinct payload carrier. The state-account key and source path, not the
	// payload filename, must keep the publication single-seated.
	supplementRows := traceQueryTypedObservations(result, "trace", "/blobs/supplement.json", "", "", time.Now())
	bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{}, []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: supplementRows,
	}})
	items = runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 1 || !strings.Contains(items[0].Text, "19 次切换/20 段") {
		t.Fatalf("model query plus exact system supplement must retain one canonical account: %+v", items)
	}
}
