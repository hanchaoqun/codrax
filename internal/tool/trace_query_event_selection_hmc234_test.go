package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc234SelectionContext(t *testing.T, raw string) *types.BusContext {
	t.Helper()
	dir := t.TempDir()
	trace := strings.Join([]string{
		`writer-10 (10) [000] .... 1.010000: print: B|10|jank_event_sync: start_ts=1010000000, end_ts=1020000000, jank_frames=2, appid=27599`,
		`writer-10 (10) [000] .... 1.030000: print: B|10|jank_event_sync: start_ts=1030000000, end_ts=1040000000, jank_frames=4, appid=27599`,
		`writer-10 (10) [000] .... 1.050000: print: B|10|jank_event_sync: start_ts=1050000000, end_ts=1060000000, jank_frames=3, appid=42`,
		`writer-10 (10) [000] .... 1.070000: print: B|10|jank_event_sync: start_ts=1070000000, end_ts=1080000000, jank_frames=5, appid=42`,
		`writer-10 (10) [000] .... 1.090000: print: B|10|jank_event_sync: start_ts=1090000000, end_ts=1100000000, jank_frames=2, appid=0`,
		`writer-10 (10) [000] .... 1.110000: print: B|10|jank_event_sync: start_ts=1110000000, end_ts=1120000000, jank_frames=1, appid=42`,
		`writer-10 (10) [000] .... 1.130000: print: B|10|jank_event_sync: start_ts=1130000000, end_ts=1140000000, jank_frames=bad, appid=42`,
		`writer-10 (10) [000] .... 1.300000: print: B|10|jank_event_sync: start_ts=1300000000, end_ts=1310000000, jank_frames=9, appid=42`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "capture.systrace"), []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{RepoRoot: dir, WorkDir: dir, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: raw, Predicates: types.SemanticPredicates{IsCountQuestion: true},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
	}}}
}

func hmc234SelectionQuery(t *testing.T, ctx *types.BusContext, filters string) types.ToolResult {
	t.Helper()
	result, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"path","path":"capture.systrace","view":"event_search","time_start":1,"time_end":1.2,"event_field_filters":`+filters+`}`))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHMC234EventSelectionRejectsPreviewNarrowingThenCountsWholeRequestedPopulation(t *testing.T) {
	ctx := hmc234SelectionContext(t, "列出1.0到1.2秒至少2帧的全部卡顿，展示appid，总共有几条？")
	bad := hmc234SelectionQuery(t, ctx, `[{"field":"jank_frames","op":"gte","value":"2"},{"field":"appid","op":"eq","value":"27599"}]`)
	if bad.Success || bad.Repair == nil || bad.Repair.Code != "trace_query_event_selection_source_required" || len(bad.Observations) != 0 {
		t.Fatalf("preview identity narrowed all-app population without repair: %+v", bad)
	}
	good := hmc234SelectionQuery(t, ctx, `[{"field":"jank_frames","op":"gte","value":"2","source_quote":"至少2帧"}]`)
	if !good.Success || !strings.Contains(good.Summary, "matched_total=5") {
		t.Fatalf("threshold 2 must not become count 2; all apps and exact window must remain: %+v", good)
	}
	for _, observation := range good.Observations {
		if inventory := observation.EventSearchInventory; inventory != nil {
			if inventory.Coverage.MatchedTotal != 5 || len(inventory.Query.EventFieldFilters) != 1 || len(inventory.Rows) != 5 {
				t.Fatalf("receipt lost population/unknown/window contract: %+v", inventory)
			}
			return
		}
	}
	t.Fatal("native inventory missing")
}

func TestHMC234EventSelectionExactQuoteMatrix(t *testing.T) {
	for _, tt := range []struct {
		name, raw, filter, count string
		ok                       bool
	}{
		{"explicit_app", "appid=42 的全部卡顿有几条", `{"field":"appid","op":"eq","value":"42","source_quote":"appid=42"}`, "3", true},
		{"explicit_zero", "appid=0 的全部卡顿有几条", `{"field":"appid","op":"eq","value":"0","source_quote":"appid=0"}`, "1", true},
		{"known_zero_result", "appid=99 的全部卡顿有几条", `{"field":"appid","op":"eq","value":"99","source_quote":"appid=99"}`, "0", true},
		{"invented_quote", "至少2帧的卡顿有几条", `{"field":"appid","op":"eq","value":"27599","source_quote":"appid=27599"}`, "", false},
		{"empty_quote", "至少2帧的卡顿有几条", `{"field":"appid","op":"eq","value":"27599","source_quote":""}`, "", false},
		{"native_number_words", "至少三帧的卡顿有几条", `{"field":"jank_frames","op":"gte","value":"3","source_quote":"至少三帧"}`, "3", true},
		{"scientific_notation", "至少2e0帧的卡顿有几条", `{"field":"jank_frames","op":"gte","value":"2","source_quote":"至少2e0帧"}`, "5", true},
		// The request provenance boundary must not require a resolved app ID
		// literally in the request. Native metadata owns name→ID resolution;
		// this gate neither supplies nor validates that independent evidence.
		{"application_name", "图库应用的全部卡顿有几条", `{"field":"appid","op":"eq","value":"42","source_quote":"图库应用"}`, "3", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := hmc234SelectionQuery(t, hmc234SelectionContext(t, tt.raw), "["+tt.filter+"]")
			if result.Success != tt.ok || tt.ok && !strings.Contains(result.Summary, "matched_total="+tt.count) {
				t.Fatalf("selection provenance mismatch: %+v", result)
			}
			if !tt.ok && (result.Repair == nil || result.Repair.Code != "trace_query_event_selection_source_required") {
				t.Fatalf("missing actionable provenance repair: %+v", result)
			}
		})
	}
}

func TestHMC234EventSelectionPreservesCausalQueriesButProtectsBoundedListings(t *testing.T) {
	for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeCausalDiagnosis, types.RuntimeQuestionScopeBoundedFactSet} {
		ctx := hmc234SelectionContext(t, "analyze the captured work")
		ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = scope
		ctx.AnalysisIR.RequestModel.Predicates.IsCountQuestion = false
		got := hmc234SelectionQuery(t, ctx, `[{"field":"appid","op":"eq","value":"42"}]`)
		if scope == types.RuntimeQuestionScopeBoundedFactSet {
			if got.Success || got.Repair == nil || got.Repair.Code != types.ToolRepairCodeTraceEventSelectionSourceRequired {
				t.Fatalf("bounded listing bypassed the population check through count=false: %+v", got)
			}
			continue
		}
		if !got.Success || !strings.Contains(got.Summary, "matched_total=3") {
			t.Fatalf("non-enumeration query changed: %+v", got)
		}
	}
}

func TestHMC234EventSelectionCompletenessAndSystemSupplement(t *testing.T) {
	ctx := hmc234SelectionContext(t, "列出全部卡顿的appid")
	ctx.AnalysisIR.RequestModel.Predicates.IsCountQuestion = false
	ctx.AnalysisIR.RequestModel.CompletenessObligation = &types.CompletenessObligation{Required: true, SourceQuote: "全部卡顿"}
	filters := `[{"field":"appid","op":"eq","value":"42"}]`
	if got := hmc234SelectionQuery(t, ctx, filters); got.Success || got.Repair == nil {
		t.Fatalf("exhaustive fact contract missed same population boundary: %+v", got)
	}
	ctx.Mutable = types.NewMutableState(ctx.AnalysisIR.RequestModel.RawRequest)
	ctx.Mutable.BeginSystemTraceSupplementExecution()
	defer ctx.Mutable.EndSystemTraceSupplementExecution()
	if got := hmc234SelectionQuery(t, ctx, filters); !got.Success || !strings.Contains(got.Summary, "matched_total=3") {
		t.Fatalf("native supplementation changed: %+v", got)
	}
}

func TestHMC234EventSelectionAcceptedAnalysisAndPaging(t *testing.T) {
	const request = "列出trace在1..1.2秒至少2帧的全部卡顿，展示appid，共有几条？"
	ctx := hmc234SelectionContext(t, request)
	ctx.AnalysisIR = nil
	ctx.Mutable = types.NewMutableState(request)
	ctx.AttachedHitrace = "trace attachment present"
	var payload map[string]any
	if err := json.Unmarshal(b1626MultiWindowAnalysisParams(t, `[{"time_start":1,"time_end":1.2,"source_quote":"1..1.2秒"}]`, "bounded_fact_set", request), &payload); err != nil {
		t.Fatal(err)
	}
	payload["predicates"].(map[string]any)["is_count_question"] = true
	payload["predicates"].(map[string]any)["is_scalar_answer"] = true
	payload["enumeration_boundary"] = map[string]any{"declared_count": 2, "source_quote": "共两条"}
	params, _ := json.Marshal(payload)
	analysis, err := (&EmitAnalysis{}).Execute(ctx, params)
	if err != nil || !analysis.Success {
		t.Fatalf("public analysis failed: %+v %v", analysis, err)
	}
	rm := ctx.Mutable.RequestModel()
	if rm.EnumerationBoundary != nil || !rm.RuntimeQuestionProfile.BoundedFactSet() || !rm.Predicates.IsCountQuestion {
		t.Fatalf("accepted analysis promoted preview count or lost fact scope: %+v", rm)
	}
	bad := hmc234SelectionQuery(t, ctx, `[{"field":"appid","op":"eq","value":"27599"}]`)
	if bad.Success || bad.Repair == nil {
		t.Fatalf("accepted analysis did not protect requested population: %+v", bad)
	}
	result, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"path","path":"capture.systrace","view":"event_search","time_start":1,"time_end":1.2,"limit":1,"event_field_filters":[{"field":"jank_frames","op":"gte","value":"2","source_quote":"至少2帧"}]}`))
	if err != nil || !result.Success || !strings.Contains(result.Summary, "matched_total=5") || result.Refinement == nil {
		t.Fatalf("requested population/paging failed: %+v %v", result, err)
	}
	if !strings.Contains(result.Refinement.PreferredParams["event_field_filters"], `"source_quote":"至少2帧"`) {
		t.Fatalf("continuation lost exact selection source: %+v", result.Refinement)
	}
}
