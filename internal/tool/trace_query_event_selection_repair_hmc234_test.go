package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc234RepairContext(t *testing.T) *types.BusContext {
	ctx := hmc234SelectionContext(t, "列出1.0到1.2秒至少2帧的全部卡顿，展示appid，总共有几条？")
	ctx.Mutable = types.NewMutableState(ctx.AnalysisIR.RequestModel.RawRequest)
	ctx.AttachedHitrace = "preview: two selected example rows"
	return ctx
}

func TestHMC234EventSelectionRepairNativeClearanceMatrix(t *testing.T) {
	for _, tt := range []struct {
		name, args string
		clears     bool
		mutate     string
	}{
		{"corrected", `"time_start":1,"time_end":1.2,"event_field_filters":[{"field":"jank_frames","op":"gte","value":"2","source_quote":"至少2帧"}]`, true, ""},
		{"same_coordinates_units", `"time_start":"1000ms","time_end":"1200ms"`, true, ""},
		{"zero_matches", `"time_start":1,"time_end":1.2,"patterns":["nothing_recorded_here"]`, true, ""},
		{"paged_count", `"time_start":1,"time_end":1.2,"limit":1`, true, ""},
		{"smaller_window", `"time_start":1.01,"time_end":1.05`, false, ""},
		{"different_line_range", `"time_start":1,"time_end":1.2,"line_start":2,"line_end":4`, false, ""},
		{"different_source", `"time_start":1,"time_end":1.2`, false, "source"},
		{"different_generation", `"time_start":1,"time_end":1.2`, false, "generation"},
		{"json_replay", `"time_start":1,"time_end":1.2`, false, "json"},
		{"inventory_mutation", `"time_start":1,"time_end":1.2`, false, "inventory"},
		{"foreign_run", `"time_start":1,"time_end":1.2`, false, "run"},
		{"unsuccessful", `"time_start":1,"time_end":1.2`, false, "failed"},
		{"canceled", `"time_start":1,"time_end":1.2`, false, "canceled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := hmc234RepairContext(t)
			bad := hmc234SelectionQuery(t, ctx, `[{"field":"jank_frames","op":"gte","value":"2"}]`)
			ctx.Mutable.AppendDispatchToolResult(bad)
			if ctx.Mutable.PendingTraceEventSelectionRepair(ctx.Mutable.DispatchToolResults()) == nil {
				t.Fatal("real rejection has no pending execution repair")
			}
			queryCtx, path := ctx, "capture.systrace"
			switch tt.mutate {
			case "source":
				path = "other.systrace"
				raw, _ := os.ReadFile(filepath.Join(ctx.WorkDir, "capture.systrace"))
				if err := os.WriteFile(filepath.Join(ctx.WorkDir, path), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "generation":
				raw, _ := os.ReadFile(filepath.Join(ctx.WorkDir, path))
				if err := os.WriteFile(filepath.Join(ctx.WorkDir, path), append(raw, '\n'), 0600); err != nil {
					t.Fatal(err)
				}
			case "run":
				copyCtx := ctx.ShallowClone()
				copyCtx.Mutable = types.NewMutableState(ctx.AnalysisIR.RequestModel.RawRequest)
				queryCtx = copyCtx
			}
			good, err := (&TraceQuery{}).Execute(queryCtx, json.RawMessage(`{"source":"path","path":"`+path+`","view":"event_search",`+tt.args+`}`))
			if err != nil || !good.Success {
				t.Fatalf("native query: %v %+v", err, good)
			}
			switch tt.mutate {
			case "json":
				raw, _ := json.Marshal(good)
				good = types.ToolResult{}
				if err := json.Unmarshal(raw, &good); err != nil {
					t.Fatal(err)
				}
			case "inventory":
				for i := range good.Observations {
					if inv := good.Observations[i].EventSearchInventory; inv != nil {
						inv.Coverage.MatchedTotal++
					}
				}
			case "failed":
				good.Success = false
			case "canceled":
				good.TraceViewCancellation = &types.TraceViewCancellation{}
			}
			ctx.Mutable.AppendDispatchToolResult(good)
			pending := ctx.Mutable.PendingTraceEventSelectionRepair(ctx.Mutable.DispatchToolResults()) != nil
			if pending == tt.clears {
				t.Fatalf("clears=%v pending=%v good=%+v", tt.clears, pending, good)
			}
		})
	}
}

func TestHMC234EventSelectionRepairCallOrderAndScopeIsolation(t *testing.T) {
	ctx := hmc234RepairContext(t)
	bad := hmc234SelectionQuery(t, ctx, `[{"field":"appid","op":"eq","value":"42"}]`)
	good := hmc234SelectionQuery(t, ctx, `[]`)
	otherBad, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"path","path":"capture.systrace","view":"event_search","time_start":1,"time_end":1.1,"event_field_filters":[{"field":"appid","op":"eq","value":"42"}]}`))
	if err != nil || otherBad.Success {
		t.Fatalf("second failed scope: %v %+v", err, otherBad)
	}
	newBad := hmc234SelectionQuery(t, ctx, `[{"field":"appid","op":"eq","value":"42"}]`)
	for _, tt := range []struct {
		name    string
		results []types.ToolResult
		pending bool
	}{
		{"retry_clears", []types.ToolResult{bad, good}, false},
		{"new_rejection_reopens", []types.ToolResult{bad, good, newBad}, true},
		{"two_scopes", []types.ToolResult{bad, otherBad, good}, true},
		{"repeated_rejection", []types.ToolResult{bad, bad, good}, false},
		{"fake_success", []types.ToolResult{bad, {ToolName: "trace_query", Success: true, Observations: good.Observations}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ctx.Mutable.PendingTraceEventSelectionRepair(tt.results) != nil; got != tt.pending {
				t.Fatalf("pending=%v want %v", got, tt.pending)
			}
		})
	}
	ctx.Mutable.AppendDispatchToolResult(bad)
	ctx.Mutable.AppendDispatchToolResult(good)
	closed, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"confidence":"high","result_kind":"resolved","reason":"Native query completed with seven events in range.","aggregate_facts":[{"kind":"scalar_value","label":"matching events","value":"7","unit":"events","role":"principal_answer","provenance":"runtime_artifact"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	// Execution repair clearance does not endorse the model's chosen count.
	// Other answer contracts still own value and full-member validation.
	if !closed.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("successful actual retry did not clear execution repair: %+v", closed)
	}
}

func TestHMC234EventSelectionRepairDefaultViewAndWrapper(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		ctx := hmc234RepairContext(t)
		args := json.RawMessage(`{"source":"path","path":"capture.systrace","time_start":1,"time_end":1.2,"event_field_filters":[{"field":"appid","op":"eq","value":"42"}]}`)
		if wrapped {
			args = append(append([]byte(`{"payload":`), args...), '}')
		}
		bad, err := (&TraceQuery{}).Execute(ctx, args)
		if wrapped {
			// A direct tool payload wrapper is not accepted by this schema;
			// it must remain a parse rejection, never query/census authority.
			if err == nil || bad.Success {
				t.Fatalf("unknown wrapper acquired authority: %+v", bad)
			}
			continue
		}
		if err != nil || bad.Repair == nil || bad.Repair.Code != types.ToolRepairCodeTraceEventSelectionSourceRequired {
			t.Fatalf("wrapped=%v query did not reach precise source repair: %v %+v", wrapped, err, bad)
		}
		ctx.Mutable.AppendDispatchToolResult(bad)
		if ctx.Mutable.PendingTraceEventSelectionRepair(ctx.Mutable.DispatchToolResults()) == nil {
			t.Fatalf("wrapped=%v omitted view skipped native pending receipt", wrapped)
		}
	}
}

func TestHMC234EventSelectionRepairPreparedUnknownTimeMeasurementTail(t *testing.T) {
	ctx := hmc234RepairContext(t)
	_, preparer, _ := hmc17NamedPathContext(t)
	ctx.TraceInputPreparer = preparer
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_dual_measurements/current.data")
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]any{"source": "path", "path": path, "view": "event_search", "event_types": []string{"measure_interval"}, "event_field_filters": []map[string]string{{"field": "appid", "op": "eq", "value": "42"}}}
	raw, _ := json.Marshal(params)
	bad, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || bad.Success || bad.Repair == nil || bad.Repair.Code != types.ToolRepairCodeTraceEventSelectionSourceRequired {
		t.Fatalf("prepared rejection: %v %+v", err, bad)
	}
	ctx.Mutable.AppendDispatchToolResult(bad)
	if ctx.Mutable.PendingTraceEventSelectionRepair(ctx.Mutable.DispatchToolResults()) == nil {
		t.Fatal("prepared rejected query did not acquire pending execution debt")
	}
	delete(params, "event_field_filters")
	raw, _ = json.Marshal(params)
	good, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !good.Success {
		t.Fatalf("prepared native repair: %v %+v", err, good)
	}
	_ = unpositionedMeasurementRecord(t, good)
	ctx.Mutable.AppendDispatchToolResult(good)
	if repair := ctx.Mutable.PendingTraceEventSelectionRepair(ctx.Mutable.DispatchToolResults()); repair != nil {
		t.Fatalf("publication-tail source normalization invalidated current native census: %+v", repair)
	}
}

func hmc234PreviewClosure(t *testing.T, ctx *types.BusContext) types.ToolResult {
	t.Helper()
	r, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"confidence":"high","result_kind":"resolved","reason":"The two preview rows form the complete requested population.","aggregate_facts":[{"kind":"scalar_value","label":"matching events","value":"2","unit":"events","role":"principal_answer","provenance":"runtime_artifact"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestHMC234EventSelectionRepairBlocksPreviewClosure(t *testing.T) {
	ctx := hmc234RepairContext(t)
	bad := hmc234SelectionQuery(t, ctx, `[{"field":"jank_frames","op":"gte","value":"2"}]`)
	if bad.Success || bad.Repair == nil {
		t.Fatalf("expected real rejected native query: %+v", bad)
	}
	ctx.Mutable.AppendDispatchToolResult(bad)
	closed := hmc234PreviewClosure(t, ctx)
	if ctx.Mutable.IsInvestigationComplete() || len(ctx.Mutable.StableInvestigationAggregateFacts()) != 0 || closed.Repair == nil {
		t.Fatalf("unexecuted query became principal count/completed: %+v", closed)
	}
}

func TestHMC234EventSelectionRepairSurvivesAutomaticDispatchRetry(t *testing.T) {
	ctx := hmc234RepairContext(t)
	bad := hmc234SelectionQuery(t, ctx, `[{"field":"jank_frames","op":"gte","value":"2"}]`)
	ctx.Mutable.AppendDispatchToolResult(bad)
	ctx.Mutable.ResetDispatchToolResults()
	closed := hmc234PreviewClosure(t, ctx)
	if closed.Success || ctx.Mutable.IsInvestigationComplete() || closed.Repair == nil {
		t.Fatalf("automatic same-run dispatch retry forgot unexecuted query: %+v", closed)
	}
	good := hmc234SelectionQuery(t, ctx, `[{"field":"jank_frames","op":"gte","value":"2","source_quote":"至少2帧"}]`)
	ctx.Mutable.AppendDispatchToolResult(good)
	ctx.Mutable.ResetDispatchToolResults()
	if repair := ctx.Mutable.PendingTraceEventSelectionRepair(nil); repair != nil {
		t.Fatalf("real successful retry was forgotten: %+v", repair)
	}
	bad = hmc234SelectionQuery(t, ctx, `[{"field":"appid","op":"eq","value":"42"}]`)
	ctx.Mutable.AppendDispatchToolResult(bad)
	ctx.Mutable.ResetDispatchToolResults()
	if ctx.Mutable.PendingTraceEventSelectionRepair(nil) == nil {
		t.Fatal("new rejected call did not reopen repair")
	}
	ctx.Mutable.AppendDispatchToolResult(good)
	if ctx.Mutable.PendingTraceEventSelectionRepair(nil) == nil {
		t.Fatal("reappending an earlier success cleared a later native rejection")
	}
	retry := hmc234SelectionQuery(t, ctx, `[{"field":"jank_frames","op":"gte","value":"2","source_quote":"至少2帧"}]`)
	ctx.Mutable.AppendDispatchToolResult(retry)
	ctx.Mutable.ResetDispatchToolResults()
	if repair := ctx.Mutable.PendingTraceEventSelectionRepair(nil); repair != nil {
		t.Fatalf("second actual good retry could not recover: %+v (memo=%v)", repair, retry.ReusedFromRunMemo)
	}
	ctx.Mutable.AppendDispatchToolResult(bad)
	if ctx.Mutable.PendingTraceEventSelectionRepair(nil) != nil {
		t.Fatal("reappending an earlier rejection reopened a repaired call")
	}
	path := filepath.Join(ctx.WorkDir, "capture.systrace")
	raw, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if ctx.Mutable.PendingTraceEventSelectionRepair(nil) == nil {
		t.Fatal("expired source version left a previously cleared repair silently complete")
	}
	ctx.Mutable.ResetTurnAArtifacts()
	if ctx.Mutable.PendingTraceEventSelectionRepair([]types.ToolResult{bad}) != nil {
		t.Fatal("new request epoch inherited prior repair")
	}
}

func TestHMC234EventSelectionRepairForkEpochIsolation(t *testing.T) {
	ctx := hmc234RepairContext(t)
	child := ctx.ShallowClone()
	child.Mutable = ctx.Mutable.ForkForExploreDispatch()
	bad := hmc234SelectionQuery(t, child, `[{"field":"appid","op":"eq","value":"42"}]`)
	child.Mutable.AppendDispatchToolResult(bad)
	child.Mutable.ResetDispatchToolResults()
	if ctx.Mutable.PendingTraceEventSelectionRepair(nil) == nil {
		t.Fatal("native child query failure was lost across shared investigation epoch")
	}
	good := hmc234SelectionQuery(t, ctx, `[]`)
	ctx.Mutable.AppendDispatchToolResult(good)
	if child.Mutable.PendingTraceEventSelectionRepair(nil) != nil {
		t.Fatal("same-epoch native repair did not reach child")
	}
	ctx.Mutable.ResetTurnAArtifacts()
	staleBad := hmc234SelectionQuery(t, child, `[{"field":"appid","op":"eq","value":"42"}]`)
	child.Mutable.AppendDispatchToolResult(staleBad)
	ctx.Mutable.AppendDispatchToolResult(staleBad)
	if ctx.Mutable.PendingTraceEventSelectionRepair(nil) != nil {
		t.Fatal("old fork contaminated the new request epoch")
	}
}
