package tool

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC237AnalysisTeachingSeparatesObservationWindowAndBusinessCondition(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	description := schema["properties"].(map[string]any)["runtime_artifact_scope_profile"].(map[string]any)["description"].(string)
	for _, surface := range []string{description, skill.BuildAnalysisSkill().OutputFormat} {
		for _, shared := range []string{skill.AnalysisRuntimeArtifactScopeTeaching, skill.AnalysisRuntimeWindowMembersTeaching} {
			if strings.Count(surface, shared) != 1 {
				t.Fatal("real analyzer schema/skill must share exactly one scope decision")
			}
		}
		for _, want := range []string{"observation/reporting-time window", "business payload endpoints", "even when their values are literal timestamps", "independently requests both"} {
			if !strings.Contains(surface, want) {
				t.Errorf("scope teaching omits %q", want)
			}
		}
		if strings.Contains(surface, "bounded_selector for a narrower frame/span/event without exact user-stated time bounds") {
			t.Error("old rule incorrectly forces a literal business timestamp into an observation window")
		}
	}
}

// These are deliberately delayed reports on the same source clock. A payload
// selection and a reporting-time selection have different, overlapping sets;
// this is not a timezone or cross-capture alignment fixture.
const hmc237DelayedBusinessReports = `writer-10 (10) [000] .... 1.100000: print: B|10|jank_event_sync: start_ts=980000000, end_ts=1090000000, jank_frames=3, appid=41
writer-10 (10) [000] .... 1.300000: print: B|10|jank_event_sync: start_ts=1200000000, end_ts=1290000000, jank_frames=4, appid=42
writer-10 (10) [000] .... 1.500000: print: B|10|jank_event_sync: start_ts=1100000000, end_ts=1300000000, jank_frames=7, appid=45
writer-10 (10) [000] .... 1.700000: print: B|10|jank_event_sync: start_ts=1250000000, end_ts=1450000000, jank_frames=5, appid=43
writer-10 (10) [000] .... 1.800000: print: B|10|jank_event_sync: start_ts=1500000000, end_ts=1600000000, jank_frames=6, appid=44
`

func TestHMC237PublicBusinessTimeSelectionPreservesDelayedNativeMembers(t *testing.T) {
	for _, tc := range []struct {
		name, request, scopeQuote, businessQuote string
		reportWindow, straySelectorCoordinates   bool
		businessStart, businessEnd               string
		wantNativeCount                          int
		wantRows                                 [][]string
	}{
		{
			name:       "report_window_is_inherited",
			request:    "列出这份 trace 在 1 到 1.5 秒记录的至少3帧卡顿，展示应用、原始起止时间和耗时。",
			scopeQuote: "1 到 1.5 秒", reportWindow: true,
			// Native lookup retains its closed right boundary. Answer scope
			// projection, not this native query, applies the half-open window.
			wantNativeCount: 3,
			wantRows:        [][]string{{"41", "980000000", "1090000000", "110"}, {"42", "1200000000", "1290000000", "90"}},
		},
		{
			name:       "business_start_without_report_window",
			request:    "按卡顿实际开始时间，列出这份 trace 中 1 到 1.5 秒开始且至少3帧的卡顿，展示应用与耗时。",
			scopeQuote: "1 到 1.5 秒开始且至少3帧的卡顿", businessQuote: "1 到 1.5 秒开始",
			businessStart: "1000000000", businessEnd: "1500000000",
			wantRows: [][]string{{"42", "1200000000", "1290000000", "90"}, {"45", "1100000000", "1300000000", "200"}, {"43", "1250000000", "1450000000", "200"}},
		},
		{
			name:       "selector_stray_coordinates_do_not_become_report_window",
			request:    "按卡顿实际开始时间，列出这份 trace 中 1 到 1.5 秒开始且至少3帧的卡顿，展示应用与耗时。",
			scopeQuote: "1 到 1.5 秒开始且至少3帧的卡顿", businessQuote: "1 到 1.5 秒开始", straySelectorCoordinates: true,
			businessStart: "1000000000", businessEnd: "1500000000",
			wantRows: [][]string{{"42", "1200000000", "1290000000", "90"}, {"45", "1100000000", "1300000000", "200"}, {"43", "1250000000", "1450000000", "200"}},
		},
		{
			name:       "independent_report_and_business_conditions_are_anded",
			request:    "列出这份 trace 在 1 到 1.5 秒记录、实际开始时间在 1.2 到 1.4 秒且至少3帧的卡顿。",
			scopeQuote: "1 到 1.5 秒记录", businessQuote: "实际开始时间在 1.2 到 1.4 秒", reportWindow: true,
			businessStart: "1200000000", businessEnd: "1400000000",
			wantRows: [][]string{{"42", "1200000000", "1290000000", "90"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var args map[string]any
			if err := json.Unmarshal(b1626MultiWindowAnalysisParams(t, `[]`, "bounded_fact_set", tc.request), &args); err != nil {
				t.Fatal(err)
			}
			scope := map[string]any{"requested_scope": "bounded_selector", "source_quote": tc.scopeQuote, "confidence": 1.0}
			if tc.reportWindow {
				scope["requested_scope"] = "explicit_time_window"
			}
			if tc.reportWindow || tc.straySelectorCoordinates {
				scope["time_start"], scope["time_end"] = 1.0, 1.5
			}
			args["runtime_artifact_scope_profile"] = scope
			args["runtime_question_profile"].(map[string]any)["fact_families"] = []string{"occurrence_time", "count_or_duration", "other_observed_value"}
			args["predicates"].(map[string]any)["is_count_question"] = false
			params, _ := json.Marshal(args)
			dir := t.TempDir()
			ctx := &types.BusContext{Ctx: context.Background(), RepoRoot: dir, WorkDir: dir,
				Mutable: types.NewMutableState(tc.request), AttachedHitrace: hmc237DelayedBusinessReports}
			analysis, err := (&EmitAnalysis{}).Execute(ctx, params)
			if err != nil || !analysis.Success {
				t.Fatalf("public analysis rejected: %v %+v", err, analysis)
			}
			rm := ctx.Mutable.RequestModel()
			if rm == nil || !rm.RuntimeQuestionProfile.BoundedFactSet() || rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() != tc.reportWindow {
				t.Fatalf("accepted analysis conflated a business condition and report window: %+v", rm)
			}
			if !tc.reportWindow && (rm.RuntimeArtifactScopeProfile.RequestedScope != types.RuntimeArtifactScopeBoundedSelector ||
				rm.RuntimeArtifactScopeProfile.TimeStart != nil || rm.RuntimeArtifactScopeProfile.TimeEnd != nil) {
				t.Fatal("accepted selector acquired observation-window coordinates")
			}
			filters := []map[string]string{{"field": "jank_frames", "op": "gte", "value": "3", "source_quote": "至少3帧"}}
			if tc.businessQuote != "" {
				filters = append(filters, map[string]string{"field": "start_ts", "op": "gte", "value": tc.businessStart, "source_quote": tc.businessQuote},
					map[string]string{"field": "start_ts", "op": "lt", "value": tc.businessEnd, "source_quote": tc.businessQuote})
			}
			query, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": "event_search", "event_field_filters": filters})
			result, err := (&TraceQuery{}).Execute(ctx, query)
			if err != nil || !result.Success {
				t.Fatalf("native query rejected: %v %+v", err, result)
			}
			if strings.Contains(result.Summary, "trace_query_request_window_inherited=") != tc.reportWindow {
				t.Fatalf("wrong report-window inheritance: %s", result.Summary)
			}
			var record *types.ObservationRecord
			for i := range result.Observations {
				if types.IsValidTraceEventSearchInventoryRecord(result.Observations[i]) {
					record = &result.Observations[i]
				}
			}
			if record == nil {
				t.Fatal("native inventory missing")
			}
			inventory := record.EventSearchInventory
			wantNativeCount := tc.wantNativeCount
			if wantNativeCount == 0 {
				wantNativeCount = len(tc.wantRows)
			}
			if inventory.Query.TimeStartSet != tc.reportWindow || inventory.Query.TimeEndSet != tc.reportWindow ||
				len(inventory.Query.EventFieldFilters) != len(filters) || !inventory.RowsComplete || inventory.Coverage.MatchedTotal != wantNativeCount ||
				len(inventory.Rows) != wantNativeCount || inventory.HandoffRowsOmitted != 0 {
				t.Fatalf("query or complete population changed: %+v", inventory)
			}
			originalInventory, _ := json.Marshal(inventory)
			contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{RequestModel: rm, RepoRoot: dir, ToolResults: []types.ToolResult{result}})
			receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: record.ID, View: types.RuntimeMeasurementMembers,
				EventSelection: &types.RuntimeEventSelection{FieldKeys: []string{"jank.appid", "jank.start_ts_ns", "jank.end_ts_ns", "jank.reported_duration_ms"}}}
			if !types.BindRuntimeMeasurementReceipt(receipt, contract) || !receipt.IsBound() || !reflect.DeepEqual(receipt.BoundTable.Rows, tc.wantRows) {
				t.Fatalf("shared native display lost delayed members or exact same-clock values: got=%+v want=%v", receipt.BoundTable, tc.wantRows)
			}
			postProjectionInventory, _ := json.Marshal(inventory)
			if string(postProjectionInventory) != string(originalInventory) {
				t.Fatal("display projection mutated the native query population or coverage")
			}
			if wantNativeCount != len(tc.wantRows) {
				notes := strings.Join(receipt.BoundTable.Notes, "\n")
				if !strings.Contains(notes, "查询匹配：3 条") || !strings.Contains(notes, "交接省略：0 条") ||
					!strings.Contains(notes, "1条查询记录不满足请求时间窗") {
					t.Fatalf("half-open display confused excluded native members with omissions: %s", notes)
				}
			}
			if len(ctx.Mutable.TraceQueryCallWindows()) != 0 {
				t.Fatal("automatic report window was recorded as explicit model scope")
			}
		})
	}
}
