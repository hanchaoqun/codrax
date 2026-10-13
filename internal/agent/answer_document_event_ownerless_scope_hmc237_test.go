package agent

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceEventOwnerlessWindowActualFinalizerHMC237(t *testing.T) {
	result := traceInventoryScopePublicQuery(t,
		"main-27599 (27599) [000] .... 1.000000: tracing_mark_write: B|27599|jank_event_sync: start_ts=920000000, end_ts=980000000, jank_frames=3, appid=27599\n"+
			"main-41002 (41002) [001] .... 1.081000: tracing_mark_write: B|41002|jank_event_sync: start_ts=980000000, end_ts=1080000000, jank_frames=4, appid=41002\n"+
			"main-27599 (27599) [000] .... 1.161000: tracing_mark_write: B|27599|jank_event_sync: start_ts=1100000000, end_ts=1160000000, jank_frames=5, appid=27599\n"+
			"main-41002 (41002) [001] .... 1.421000: tracing_mark_write: B|41002|jank_event_sync: start_ts=1320000000, end_ts=1420000000, jank_frames=7, appid=41002\n"+
			"main-41002 (41002) [001] .... 1.499000: tracing_mark_write: B|41002|jank_event_sync: start_ts=1430000000, end_ts=1470000000, jank_frames=6, appid=41002\n"+
			"main-27599 (27599) [000] .... 1.500000: tracing_mark_write: B|27599|jank_event_sync: start_ts=1440000000, end_ts=1490000000, jank_frames=8, appid=27599\n", 1, 1.5,
		map[string]any{"pattern": "jank_event_sync", "event_field_filters": []map[string]string{{"field": "jank_frames", "op": "gte", "value": "3"}}})
	for _, mode := range []string{"one_window", "disjoint_windows", "exploration_cursor"} {
		t.Run(mode, func(t *testing.T) {
			ctx := traceInventoryScopeContext(result, 1, 1.5)
			ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
			want := []string{"event-1", "event-2", "event-3", "event-4", "event-5"}
			if mode == "exploration_cursor" {
				ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 27599, Source: types.RuntimeTargetSourceExplicitToolCall}}
			}
			if mode == "disjoint_windows" {
				a, b, c, d := 1.0, 1.1, 1.4, 1.5
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeWindows: []types.RuntimeArtifactTimeWindow{{TimeStart: &a, TimeEnd: &b, SourceQuote: "[1,1.1)"}, {TimeStart: &c, TimeEnd: &d, SourceQuote: "[1.4,1.5)"}}}
				want = []string{"event-1", "event-2", "event-4", "event-5"}
			}
			original := traceInventoryScopeReceipt(t, ctx)
			if len(original.EventSearchInventory.Rows) != 6 {
				t.Fatal("fixture must preserve the inclusive native lookup")
			}
			before := traceInventoryScopeAuthority(t, ctx)
			prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
			views := traceEventInventoryPromptViews(t, prompt)
			if len(views) != 1 || !reflect.DeepEqual(views[0].SelectorRowIDs, want) || len(views[0].Inventory.Rows) != len(want) {
				t.Fatalf("ownerless requested-window selector mismatch: want=%v got=%+v", want, views)
			}
			view := views[0]
			if !reflect.DeepEqual(view.Source, original.SourceRef) || !reflect.DeepEqual(view.Inventory.Query, original.EventSearchInventory.Query) || !reflect.DeepEqual(view.Inventory.Coverage, original.EventSearchInventory.Coverage) {
				t.Fatal("display projection rewrote original query/source/coverage")
			}
			for n, start := range []int64{920000000, 980000000} {
				if view.Inventory.Rows[n].JankEvent == nil || view.Inventory.Rows[n].JankEvent.Values == nil || view.Inventory.Rows[n].JankEvent.Values.StartTSNS != start {
					t.Fatal("report-time projection filtered or rewrote late payload starts")
				}
			}
			fields := traceInventoryScopeProjectionFields(t, prompt)
			for name, expected := range map[string]int{"input_rows": 6, "retained_rows": len(want), "window_rows_excluded": 6 - len(want), "owner_rows_excluded": 0} {
				var got int
				if err := json.Unmarshal(fields[name], &got); err != nil || got != expected {
					t.Fatalf("scope accounting %s=%s want=%d", name, fields[name], expected)
				}
			}
			if before != traceInventoryScopeAuthority(t, ctx) {
				t.Fatal("ownerless display changed ledger, original facts or causal projection")
			}
		})
	}
}

func TestTraceEventOwnerlessComparisonRequiresCurrentSourceHMC237(t *testing.T) {
	ctx, _, _ := hmc225DisplayContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOccurrenceTime},
	}
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal(ledger)
	readViews := func(candidate types.ObservationLedger) []traceEventInventoryPromptView {
		t.Helper()
		prompt, _ := renderAnswerDocScopedTraceEventInventoryView(ctx, candidate, candidate)
		return traceEventInventoryPromptViews(t, prompt)
	}
	valid := readViews(ledger)
	if len(valid) != 4 {
		t.Fatalf("fixture needs four source-qualified queries, got %d", len(valid))
	}
	rows := 0
	for _, view := range valid {
		rows += len(view.Inventory.Rows)
		window, ok := ledger.RuntimeDisplayScope(types.ObservationRecord{SourceRef: view.Source}, ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile)
		if !ok {
			t.Fatal("healthy source lost its original-specific display scope")
		}
		for _, row := range view.Inventory.Rows {
			if !types.TraceEventRowWithinRequestedWindows(row, window) {
				t.Fatal("source borrowed its peer's time window")
			}
		}
	}
	if rows == 0 {
		t.Fatal("ownerless positive control lost every observed event")
	}
	t.Run("missing_private_source_receipt", func(t *testing.T) {
		var publicOnly types.ObservationLedger
		if err := json.Unmarshal(before, &publicOnly); err != nil {
			t.Fatal(err)
		}
		if views := readViews(publicOnly); len(views) != 0 {
			t.Fatal("public source metadata borrowed comparison display qualification")
		}
	})
	t.Run("foreign_source", func(t *testing.T) {
		foreign := ledger
		if err := json.Unmarshal(before, &foreign); err != nil {
			t.Fatal(err)
		}
		for n := range foreign.Records {
			foreign.Records[n].SourceRef.Path += ".foreign"
		}
		if views := readViews(foreign); len(views) != 0 {
			t.Fatal("foreign source borrowed a source-owned request window")
		}
	})
	t.Run("changed_original", func(t *testing.T) {
		path := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.Comparison.Baseline.Path
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(body, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		views := readViews(ledger)
		if len(views) != 2 {
			t.Fatalf("stale baseline must be excluded without removing the valid peer: got %d", len(views))
		}
		for _, view := range views {
			if view.Source.Path == path {
				t.Fatal("changed original retained its old display receipt")
			}
		}
	})
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("display qualification mutated the accepted evidence ledger")
	}
}
