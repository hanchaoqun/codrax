package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc277MeasurementContext(t *testing.T, fixture, queryView string, start, end float64) (*types.AgentContext, string) {
	t.Helper()
	dir := t.TempDir()
	body, err := os.ReadFile("../../eval/fixtures/" + fixture + "/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "capture.systrace")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		DiagramHint:                 &types.DiagramHint{Kind: types.DiagramFlow, Required: true},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Describe the measured values and their scope, with a fact diagram."), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en", Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramFlow}}}}
	ctx.Mutable.SetRequestModel(rm)
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": queryView, "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return ctx, path
}

func TestHMC277NativeMeasurementSurfacesPublic(t *testing.T) {
	for _, tc := range []struct {
		fixture, queryView, predicate string
		start, end                    float64
	}{
		{"hmosperf_io_inflight", "window_stats", "#io_inflight:", 1, 1.01},
		{"hmosperf_io_activity", "window_stats", "#io_activity:", 2, 2.25},
		{"hmosperf_scheduler_concurrency", "window_stats", "#scheduler_concurrency", 1, 1.013},
		{"hmosperf_cpu_state_frequency", "cpu_state_frequency", "#cpu_state_frequency", 1, 1.04},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			ctx, _ := hmc277MeasurementContext(t, tc.fixture, tc.queryView, tc.start, tc.end)
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			before := hmc218CausalProjection(t, ctx)
			original, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
			seen := 0
			for _, table := range view.RuntimeMeasurementContract.Choices() {
				if !strings.Contains(table.ObservationID, tc.predicate) {
					continue
				}
				seen++
				if !table.HasNativeFixedFacts() {
					t.Fatalf("verified native table cannot reuse its facts in summary/diagram: %s/%s", table.Label, table.View)
				}
				for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
					receipt := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
					blocks := []types.AnswerBlock{{ID: "native", Kind: kind, RuntimeMeasurement: receipt}}
					if kind != types.BlockSummary {
						blocks = append([]types.AnswerBlock{{ID: "intro", Kind: types.BlockSummary, Text: "Independent interpretation remains evidence-limited."}}, blocks...)
					}
					raw, _ := json.Marshal(map[string]any{"blocks": blocks})
					if err := toolparam.Validate(raw, tool.BuildAnswerDocumentParametersFor(view)); err != nil {
						t.Fatalf("schema %s: %v", kind, err)
					}
					bus := types.ToolBusContext(ctx, types.AgentFinalizer)
					r, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
					if err != nil || !r.Success {
						t.Fatalf("emit %s: %v %s", kind, err, r.Summary)
					}
					var native types.AnswerBlock
					for _, b := range ctx.Mutable.AnswerDocumentV2().Blocks {
						if b.ID == "native" {
							native = b
						}
					}
					if native.RuntimeMeasurement == nil || !native.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(native.RuntimeMeasurement.BoundTable.Rows, table.Rows) || !reflect.DeepEqual(native.RuntimeMeasurement.BoundTable.Notes, table.Notes) {
						t.Fatal("display changed rows or boundaries")
					}
					first := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en")
					if strings.Contains(first, "-->") || strings.Contains(first, "->>") {
						t.Fatal("fixed facts acquired a relationship")
					}
					patch, _ := json.Marshal(map[string]any{"replace_blocks": []types.AnswerBlock{native}})
					if err := toolparam.Validate(patch, tool.BuildAnswerDocumentPatchParametersFor(view)); err != nil {
						t.Fatal(err)
					}
					r, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, patch)
					if err != nil || !r.Success || render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en") != first {
						t.Fatalf("patch changed native display: %v %s", err, r.Summary)
					}
				}
			}
			if seen == 0 {
				t.Fatalf("fixture did not publish %s", tc.predicate)
			}
			messages := dependencyObservationMessages(t, ctx)
			if !strings.Contains(messages, "Current source-bound measurements also support summary/table/diagram") {
				t.Fatal("actual Finalizer missing shared native reuse teaching")
			}
			after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
			if !bytes.Equal(original, after) || !bytes.Equal(before, hmc218CausalProjection(t, ctx)) {
				t.Fatal("display mutated observations or causal projection")
			}
		})
	}
}

func TestHMC277NativeMeasurementSurfacesRejectCompetingAuthority(t *testing.T) {
	ctx, _ := hmc277MeasurementContext(t, "hmosperf_io_inflight", "window_stats", 1, 1.01)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	var table types.RuntimeMeasurementTable
	for _, candidate := range view.RuntimeMeasurementContract.Choices() {
		if candidate.HasNativeMeasurementFacts() {
			table = candidate
			break
		}
	}
	if table.ObservationID == "" {
		t.Fatal("missing actual native measurement")
	}
	for _, kind := range []string{"summary", "table", "diagram"} {
		for field, value := range map[string]any{
			"text":                     "copied values imply hardware saturation",
			"columns":                  []string{"copied"},
			"requested_relation_scope": "invented dependency",
		} {
			t.Run(kind+"/"+field, func(t *testing.T) {
				block := map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}, field: value}
				blocks := []any{block}
				if kind != "summary" {
					blocks = append([]any{map[string]any{"id": "intro", "kind": "summary", "text": "Independent interpretation."}}, blocks...)
				}
				raw, _ := json.Marshal(map[string]any{"blocks": blocks})
				if toolparam.Validate(raw, tool.BuildAnswerDocumentParametersFor(view)) == nil {
					t.Fatal("schema accepted competing fact payload/authority")
				}
				r, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
				if err == nil && r.Success {
					t.Fatal("Execute accepted competing fact payload/authority")
				}
			})
		}
	}
}
