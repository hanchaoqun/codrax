package tool

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc220PairPublicFixture(t *testing.T, states ...*types.MutableState) (*types.BusContext, types.ToolResult) {
	t.Helper()
	ctx, _, _ := hmc17NamedPathContext(t)
	if len(states) > 0 {
		ctx.Mutable = states[0]
	}
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{}, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
	ctx.Mutable.SetRequestModel(rm)
	args, _ := json.Marshal(map[string]any{"comparison": map[string]any{"baseline": map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2}, "current": map[string]any{"source": "path", "path": path + ".missing", "view": "measurements", "time_start": 4, "time_end": 4.5}}})
	if err := toolparam.Validate(args, (&TraceQuery{}).Parameters()); err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(ctx, args)
	if err != nil || !result.Success {
		t.Fatalf("pair: %v / %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{types.AttachToolHandoffCarrier(result)}})
	return ctx, result
}

func TestHMC220MeasurementPairPublicMergedForkUsesConsumerEpoch(t *testing.T) {
	parent := types.NewMutableState("compare")
	fork := parent.ForkForExploreDispatch()
	ctx, result := hmc220PairPublicFixture(t, fork)
	parent.MergeExploreFork(fork)
	ctx.Mutable = parent
	if n := len(types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract.Choices()); n != 4 {
		t.Fatalf("merged query lost private receipt: %d", n)
	}
	parent.ResetTurnAArtifacts()
	// Deliberately reinsert the old result: clearing the ordinary handoff alone
	// must not be what protects the new run from a still-live producer fork.
	parent.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	choices := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 1 || !strings.HasPrefix(choices[0].ObservationID, "trace_measurement_pair:") || choices[0].Rows[0][1] != "stale" {
		t.Fatalf("old producer fork restored current authority: %+v", choices)
	}
}

func TestHMC220MeasurementPairPublicEmitRenderAndLifecycle(t *testing.T) {
	ctx, result := hmc220PairPublicFixture(t)
	choices := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 4 {
		t.Fatalf("healthy table lost: %d", len(choices))
	}
	blocks := []any{map[string]any{"id": "lead", "kind": "summary", "text": "The two sources retain independent acquisition states."}}
	for i, table := range choices {
		if table.View != types.RuntimeMeasurementMembers && !strings.HasPrefix(table.ObservationID, "trace_measurement_pair:") {
			continue
		}
		blocks = append(blocks, map[string]any{"id": string(rune('a' + i)), "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
	}
	out := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": blocks}, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en"))
	for _, exact := range []string{"9007199254740993", "未知（NULL）", "failed", "baseline", "current", "设备未知", "负载未知"} {
		if !strings.Contains(visible, exact) {
			t.Errorf("final rendered answer lost %q", exact)
		}
	}
	// All report data is copy-out; neither producer caller nor read consumer
	// can change accepted tables by mutating an exported report slice.
	before, _ := result.RuntimeMeasurementPair.Report()
	mutated, _ := result.RuntimeMeasurementPair.Report()
	mutated.Sides[0].Publications[0].Tables[0].Rows[0][0] = "caller changed"
	mutated.Sides[1].Status = "available"
	after, _ := result.RuntimeMeasurementPair.Report()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("report aliases immutable producer data")
	}
	fork := ctx.Mutable.ForkForExploreDispatch()
	child := ctx.ShallowClone()
	child.Mutable = fork
	if n := len(types.BuildAnswerSemanticViewForBusContext(child).RuntimeMeasurementContract.Choices()); n != 4 {
		t.Fatalf("fork lost private receipt: %d", n)
	}
	ctx.Mutable.MergeExploreFork(fork)
	if n := len(types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract.Choices()); n != 4 {
		t.Fatalf("merge lost private receipt: %d", n)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "RuntimeMeasurementPair") || strings.Contains(string(encoded), "runtime_measurement_pair") {
		t.Fatal("private authority serialized")
	}
	var historical types.ToolResult
	if err := json.Unmarshal(encoded, &historical); err != nil {
		t.Fatal(err)
	}
	if types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{historical}}).Active() {
		t.Fatal("historical JSON resurrected pair authority")
	}
	if _, err := os.Stat(result.RawRef); err != nil {
		t.Fatal("public audit report missing", err)
	}
}

func TestHMC220MeasurementPairPublicShapeIsolation(t *testing.T) {
	for _, raw := range []string{
		`{"comparison":{"baseline":{"source":"path","path":"never-read","view":"measurements","pattern":"x"}}}`,
		`{"comparison":{"baseline":{"source":"path","path":"never-read","view":"measurements","comparison":{}}}}`,
		`{"comparison":{"baseline":{"source":"path","path":"never-read","view":"root_cause_rank"}}}`,
		`{"comparison":{"baseline":{"source":"path","path":"never-read","view":"measurements","device":"fast"}}}`,
		`{"comparison":{"baseline":{"path":"never-read","view":"measurements"}}}`,
		`{"comparison":{"baseline":null}}`,
	} {
		result, err := (&TraceQuery{}).Execute(nil, json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		report, ok := result.RuntimeMeasurementPair.Report()
		if !ok || report.Sides[0].Status != "failed" || report.Sides[1].Status != "not_requested" || len(report.Sides[0].Publications) != 0 {
			t.Fatalf("invalid side granted facts: %s / %+v", raw, report)
		}
		if err := toolparam.Validate(json.RawMessage(raw), (&TraceQuery{}).Parameters()); err == nil {
			t.Errorf("schema silently accepts invalid side %s", raw)
		}
	}
	for _, raw := range []string{`{"comparison":{}}`, `{"comparison":{"baseline":{},"unknown":1}}`, `{"source":"path","comparison":{"baseline":{}}}`} {
		result, _ := (&TraceQuery{}).Execute(nil, json.RawMessage(raw))
		if result.Success {
			t.Fatalf("invalid wrapper accepted: %s", raw)
		}
	}
}
