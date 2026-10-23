package agent

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The original query, lookup population and user half-open display window are
// three different rulers. Exercise the public query and all native surfaces.
func TestHMC283EventSourceSelectionPublicSurfaces(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    map[string]any
		want    string
		request bool
	}{
		{"unbounded", nil, "no time or line filter", false},
		{"closed", map[string]any{"time_start": 1., "time_end": 2.}, "time [1,2] s (both endpoints included)", false},
		{"lower", map[string]any{"time_start": 1.}, "time >= 1 s", false},
		{"upper", map[string]any{"time_end": 2.}, "time <= 2 s", false},
		{"line_precedence", map[string]any{"time_start": 10., "time_end": 20., "line_start": 2, "line_end": 3}, "lines [2,3] (inclusive; time arguments not applied)", false},
		{"zero", map[string]any{"time_start": 0., "time_end": 2.}, "time <= 2 s", false},
		{"empty", map[string]any{"time_start": 5., "time_end": 6.}, "time [5,6] s (both endpoints included)", false},
		{"request_projection", nil, "no time or line filter", true},
		{"lookup_wider_than_request", map[string]any{"time_start": "0.900000", "time_end": "2.100000"}, "time [0.9,2.1] s (both endpoints included)", true},
		{"lookup_tolerance", map[string]any{"time_start": .9, "time_end": 2.1}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "events.ftrace")
			body := "writer-101 (101) [000] .... 0.000000: tracing_mark_write: I|101|zero\n" +
				"writer-101 (101) [000] .... 1.000000: tracing_mark_write: I|101|left\n" +
				"writer-101 (101) [000] .... 2.000000: tracing_mark_write: I|101|right\n" +
				"writer-101 (101) [000] .... 3.000000: tracing_mark_write: I|101|last\n"
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
				RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}}}
			if tc.request {
				start, end := 1., 2.
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[1,2) seconds"}
			}
			ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("Describe the recorded events."), AnalysisIR: &types.AnalysisIR{RequestModel: rm},
				TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
			ctx.Mutable.SetRequestModel(rm)
			args := map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 40}
			for key, value := range tc.args {
				args[key] = value
			}
			raw, _ := json.Marshal(args)
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !result.Success {
				t.Fatalf("public query: %v %s", err, result.Summary)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			var selected *types.RuntimeMeasurementTable
			for _, table := range view.RuntimeMeasurementContract.Choices() {
				if table.EventData != nil {
					copy := table
					selected = &copy
				}
			}
			if selected == nil {
				t.Fatal("public lookup lost its native event table")
			}
			before, _ := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), hmc218CausalProjection(t, ctx)})
			want := tc.want
			if tc.name == "lookup_tolerance" {
				scan := selected.EventData.Coverage.ScanScope
				if scan == nil || !scan.TimeStartApplied || !scan.TimeEndApplied || scan.TimeStart >= .9 || scan.TimeEnd <= 2.1 {
					t.Fatalf("fixture lost the actual lookup expansion: %+v", scan)
				}
				want = fmt.Sprintf("time [%g,%g] s (both endpoints included)", scan.TimeStart, scan.TimeEnd)
			}
			basis, owned := selected.NativeSourceTitleBasis()
			encoded, _ := json.Marshal(basis)
			var fields map[string]any
			_ = json.Unmarshal(encoded, &fields)
			if !owned || fields["query_selection"] != want || fields["query_window"] != nil {
				t.Errorf("lookup source title invented a continuous window or lost actual selector: %s", encoded)
			}
			var restored types.RuntimeMeasurementTable
			tableJSON, _ := json.Marshal(selected)
			if err := json.Unmarshal(tableJSON, &restored); err != nil {
				t.Fatal(err)
			}
			if _, ok := restored.NativeSourceTitleBasis(); ok {
				t.Fatal("JSON reconstructed a private selection/title credential")
			}
			changed := selected.Clone()
			changed.EventData.Coverage.ScanScope = nil
			if _, ok := changed.NativeSourceTitleBasis(); ok {
				t.Fatal("modified scan scope borrowed the original title credential")
			}
			if strings.Contains(selected.Label, "time scope unverified") {
				t.Error("verified event-point projection was mislabeled as an unverified statistics window")
			}
			if tc.request {
				if len(selected.EventData.Rows) != 1 {
					t.Fatalf("half-open request did not preserve only left point: %d", len(selected.EventData.Rows))
				}
			}
			prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
			wantJSON, _ := json.Marshal(want)
			if !strings.Contains(prompt, `"query_selection":`+string(wantJSON)) {
				t.Error("actual finalizer lost query-local selection disclosure")
			}
			assertSourceTitleNavigationHMC269(t, prompt, view.RuntimeMeasurementContract.Choices())
			for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
				blocks := []types.AnswerBlock{{ID: "native", Kind: kind, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: selected.ObservationID, View: selected.View, EventSelection: &types.RuntimeEventSelection{}}}}
				if kind != types.BlockSummary {
					blocks = append([]types.AnswerBlock{{ID: "lead", Kind: types.BlockSummary, Text: "Recorded events."}}, blocks...)
				}
				wire, _ := json.Marshal(map[string]any{"blocks": blocks})
				out, e := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), wire)
				if e != nil || !out.Success {
					t.Fatalf("public %s: %v %s", kind, e, out.Summary)
				}
				visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en"))
				if !strings.Contains(visible, "query selection: "+want) || strings.Contains(visible, "actual query window:") {
					t.Errorf("%s source scope drift: %s", kind, visible)
				}
			}
			after, _ := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), hmc218CausalProjection(t, ctx)})
			if string(before) != string(after) {
				t.Fatal("presentation changed source facts or causal projection")
			}
		})
	}
}

func TestHMC283StatisticsSourceWindowUnchanged(t *testing.T) {
	ctx, _ := hmc277MeasurementContext(t, "hmosperf_io_inflight", "window_stats", 1, 1.01)
	seen := 0
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if !strings.Contains(table.ObservationID, "#io_inflight:") {
			continue
		}
		basis, ok := table.NativeSourceTitleBasis()
		if !ok || basis.QueryWindow != "[1,1.01) s" {
			t.Fatalf("statistics lost its effective denominator window: %+v", basis)
		}
		seen++
	}
	if seen == 0 {
		t.Fatal("missing native IO statistic control")
	}
}
