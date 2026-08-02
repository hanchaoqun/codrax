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
				RunningMs: 3, RunnableMs: 5, FragmentCount: 20, StateSwitches: 19,
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
				DominantState:    string(tracequery.StateRunnable),
				DominantImpactMs: 5,
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
}
