package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWakeupMissingTargetIsInvocationRepairNotEmptyEvidence(t *testing.T) {
	for _, targets := range [][]types.RuntimeTarget{
		nil,
		{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}, {Kind: types.RuntimeTargetKindThread, PID: 200, Source: "user_explicit"}},
	} {
		ctx := suppCoreContext(t)
		ctx.AnalysisIR.RequestModel.RuntimeTargets = targets
		r, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"wakeup_chain","time_start":3,"time_end":3.2}`))
		if err != nil || r.Success || r.Repair == nil || r.Repair.Code != "trace_query_source_thread_required" {
			t.Fatalf("missing/ambiguous selector became capture evidence: err=%v success=%v repair=%+v observations=%d", err, r.Success, r.Repair, len(r.Observations))
		}
		rm := ctx.Mutable.RequestModel()
		if len(r.Observations) != 0 || r.RawRef != "" || (rm != nil && len(rm.RuntimeTargets) != 0) {
			t.Fatalf("input repair produced evidence or exploration cursor: %+v", r)
		}
		if r.Repair.Metadata["view"] != "wakeup_chain" || r.Repair.Metadata["retry_scope"] != "same_source_and_window" {
			t.Fatalf("repair lost invocation scope: %+v", r.Repair)
		}
	}
}

func TestWakeupTargetRepairPreservesAutocompleteAndExplicitSelection(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		ctx := suppCoreContext(t)
		params := map[string]any{"view": "wakeup_chain", "time_start": 3, "time_end": 3.2}
		if explicit {
			ctx.AnalysisIR.RequestModel.RuntimeTargets = append(ctx.AnalysisIR.RequestModel.RuntimeTargets, types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"})
			params["pid"] = 200
		}
		data, _ := json.Marshal(params)
		r, err := (&TraceQuery{}).Execute(ctx, data)
		if err != nil || !r.Success || len(r.Observations) == 0 {
			t.Fatalf("valid target blocked explicit=%v: %v %+v", explicit, err, r)
		}
		for _, row := range r.Observations {
			if row.SourceRef.QueryWindowKnown && (row.SourceRef.QueryWindowStartTs != 3 || row.SourceRef.QueryWindowEndTs != 3.2) {
				t.Fatalf("target repair changed requested window: %+v", row.SourceRef)
			}
		}
	}
	for _, view := range []string{"event_search", "window_stats", "ipc_graph"} {
		ctx := suppCoreContext(t)
		ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
		data, _ := json.Marshal(map[string]any{"view": view, "time_start": 3, "time_end": 3.2})
		r, err := (&TraceQuery{}).Execute(ctx, data)
		if err != nil || !r.Success {
			t.Fatalf("unscoped inventory blocked for %s: %v %+v", view, err, r)
		}
	}
}
