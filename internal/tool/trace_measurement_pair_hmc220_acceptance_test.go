package tool

import (
	"bytes"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Independent acceptance uses the exact two live-candidate captures, actual
// native preparation/query, public JSON schemas, both mutations and rendering.
// No publication table or private receipt is fabricated by this test.
func TestHMC220DualMeasurementNativePublicEmitRender(t *testing.T) {
	dir := t.TempDir()
	paths := [2]string{}
	sourceBytes := [2][]byte{}
	for i, name := range []string{"baseline.data", "current.data"} {
		body, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_measurements/" + name)
		if err != nil {
			t.Fatal(err)
		}
		paths[i], sourceBytes[i] = filepath.Join(dir, name), body
		if err := os.WriteFile(paths[i], body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	rm.PerfTrace = &types.PerfBundle{}
	firstStart, firstEnd, secondStart, secondEnd := 1.0, 2.0, 4.0, 4.5
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, Confidence: 1,
		TimeWindows: []types.RuntimeArtifactTimeWindow{
			{TimeStart: &firstStart, TimeEnd: &firstEnd, SourceQuote: "baseline 1 to 2 seconds"},
			{TimeStart: &secondStart, TimeEnd: &secondEnd, SourceQuote: "current 4 to 4.5 seconds"},
		}}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: types.NewMutableState("list independent measurement records"),
		AnalysisIR:         &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	bus.Mutable.SetRequestModel(rm)
	params, _ := json.Marshal(map[string]any{"comparison": map[string]any{
		"baseline": map[string]any{"source": "path", "path": paths[0], "view": "measurements", "time_start": firstStart, "time_end": firstEnd},
		"current":  map[string]any{"source": "path", "path": paths[1], "view": "measurements", "time_start": secondStart, "time_end": secondEnd},
	}})
	if err := toolparam.Validate(params, (&TraceQuery{}).Parameters()); err != nil {
		t.Fatal("published query schema rejects its own comparison", err)
	}
	result, err := (&TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("native pair acquisition: %v / %s", err, result.Summary)
	}
	report, ok := result.RuntimeMeasurementPair.Report()
	if !ok || report.Sides[0].Status != "available" || report.Sides[1].Status != "available" {
		t.Fatalf("actual captures failed independently: %+v", report)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{types.AttachToolHandoffCarrier(result)}})
	ledgerBefore := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	projectionBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledgerBefore))
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	if len(view.RuntimeMeasurementContract.Choices()) != 7 {
		t.Fatalf("two complete three-view sources plus pair status absent: %d", len(view.RuntimeMeasurementContract.Choices()))
	}
	const prose = "模型解释单列；原始记录来自各自采集。"
	input := map[string]any{"blocks": []any{map[string]any{"id": "interpretation", "kind": "summary", "text": prose}}}
	encoded, _ := json.Marshal(input)
	if err := toolparam.Validate(encoded, BuildAnswerDocumentParametersFor(view)); err != nil {
		t.Fatal(err)
	}
	if out := b1659bExecuteAnswer(t, bus, input, false); !out.Success {
		t.Fatal(out.Summary)
	}
	assertDisplay := func() {
		t.Helper()
		doc := bus.Mutable.AnswerDocumentV2()
		counts := map[int]int{}
		for _, block := range doc.Blocks {
			if block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() {
				continue
			}
			counts[len(block.RuntimeMeasurement.BoundTable.Rows)]++
		}
		if len(counts) != 3 || counts[13] != 1 || counts[8] != 1 || counts[2] != 1 {
			t.Fatalf("independent native rows were omitted or duplicated: %v", counts)
		}
		visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
		// The native REAL formatting uses equivalent scientific notation;
		// neither spelling changes the original exact decimal value here.
		for _, value := range []string{prose, "9007199254740993", "9007199254740995", "3.342000005e+08", "2.5000000025e+08", "0 (integer)", "未知（NULL）", "baseline", "current", "无法定位时间1条"} {
			if !strings.Contains(visible, value) {
				t.Errorf("original independent fact or prose missing %q", value)
			}
		}
	}
	assertDisplay()
	doc := bus.Mutable.AnswerDocumentV2()
	unchanged := []string{}
	for _, block := range doc.Blocks {
		if block.ID != "interpretation" {
			unchanged = append(unchanged, block.ID)
		}
	}
	patch := map[string]any{"replace_blocks": input["blocks"], "unchanged_block_ids": unchanged}
	if out := b1659bExecuteAnswer(t, bus, patch, true); !out.Success {
		t.Fatal(out.Summary)
	}
	assertDisplay()
	// Report copies cannot modify the producer's private immutable facts.
	report.Sides[0].Publications[0].Tables[0].Rows[0][0] = "mutated report copy"
	fresh, _ := result.RuntimeMeasurementPair.Report()
	copyJSON, _ := json.Marshal(fresh)
	if strings.Contains(string(copyJSON), "mutated report copy") {
		t.Fatal("public audit copy mutated private authority")
	}
	// Same-run exploration forks retain private validity. A new run and JSON
	// replay do not acquire the private receipt simply by copying tool output.
	fork := bus.ShallowClone()
	fork.Mutable = bus.Mutable.ForkForExploreDispatch()
	if len(types.BuildAnswerSemanticViewForBusContext(fork).RuntimeMeasurementContract.Choices()) != 7 {
		t.Fatal("same-run fork lost valid pair")
	}
	fork.Mutable = types.NewMutableState("unrelated run")
	fork.Mutable.SetRequestModel(rm)
	fork.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	if len(types.BuildAnswerSemanticViewForBusContext(fork).RuntimeMeasurementContract.Choices()) != 1 {
		t.Fatal("unrelated run gained original measured values")
	}
	wire, _ := json.Marshal(result)
	var replay types.ToolResult
	if err := json.Unmarshal(wire, &replay); err != nil {
		t.Fatal(err)
	}
	fork.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{replay}})
	if types.BuildAnswerSemanticViewForBusContext(fork).RuntimeMeasurementContract.Active() {
		t.Fatal("JSON replay recreated private measurement authority")
	}
	ledgerAfter := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	projectionAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledgerAfter))
	if !bytes.Equal(projectionBefore, projectionAfter) || bus.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("pair display changed causal authority")
	}
	for i, path := range paths {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, sourceBytes[i]) {
			t.Fatal("read-mode pair changed its original source", err)
		}
	}
}
