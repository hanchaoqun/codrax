package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Compare every old field, including private scope/authority. The new
// source-presentation seal changes when an optional provider note changes and
// is checked independently; JSON-only comparison would hide old credentials.
func differingMeasurementFieldExceptPresentationHMC269(a, b types.RuntimeMeasurementTable) string {
	left, right := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := 0; i < left.NumField(); i++ {
		name := left.Type().Field(i).Name
		if name == "nativeSourcePresentation" || name == "nativeMeasurementFactsDigest" {
			continue
		}
		l, r := left.Field(i), right.Field(i)
		if l.CanInterface() {
			if !reflect.DeepEqual(l.Interface(), r.Interface()) {
				return name
			}
			continue
		}
		if l.Kind() == reflect.Pointer {
			if l.IsNil() || r.IsNil() {
				if l.IsNil() != r.IsNil() {
					return name
				}
				continue
			}
			l, r = l.Elem(), r.Elem()
		}
		if !l.Comparable() || !l.Equal(r) {
			return name
		}
	}
	return ""
}

func TestHMC269SourceTitleHandoffUsesOneOwnerPerObservation(t *testing.T) {
	dir := t.TempDir()
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("两份采集的原始量测"), AnalysisIR: &types.AnalysisIR{RequestModel: rm},
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	ctx.Mutable.SetRequestModel(rm)
	sides := map[string]any{}
	for _, side := range []string{"baseline", "current"} {
		raw, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_capture_metadata/" + side + ".data")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, side+".data")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		sides[side] = map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2}
	}
	raw, _ := json.Marshal(map[string]any{"comparison": sides})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !result.Success {
		t.Fatalf("actual query: %v %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	before := hmc218CausalProjection(t, ctx)
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	owners, views := map[string]int{}, 0
	for _, table := range choices {
		if _, _, owned := table.NativeSourcePresentation(""); owned {
			owners[table.ObservationID]++
			views++
		}
	}
	if len(owners) != 3 || views != 7 {
		t.Fatalf("expected two three-view sources and one status, owners=%v views=%d", owners, views)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	bases, refs := assertSourceTitleNavigationHMC269(t, prompt, choices)
	suppliedBytes := 0
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "- native_title_source ") {
			suppliedBytes += len(line) + 1
		}
		if _, raw, found := strings.Cut(line, "; source_title="); found {
			decoder := json.NewDecoder(strings.NewReader(raw))
			var ref json.RawMessage
			if err := decoder.Decode(&ref); err != nil {
				t.Fatal(err)
			}
			suppliedBytes += len("; source_title=") + int(decoder.InputOffset())
		}
	}
	if bases != 2 || refs != 7 || strings.Count(prompt, types.RuntimeMeasurementSourceTitleTeaching) != 1 {
		t.Fatalf("source join or teaching repeated: bases=%d refs=%d", bases, refs)
	}
	if string(before) != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("presentation changed causal projection")
	}
	t.Logf("current_prompt_bytes=%d source_navigation_bytes=%d title_teaching_bytes=%d source_bases=%d source_owned_observations=%d native_views=%d",
		len(prompt), suppliedBytes, len(types.RuntimeMeasurementSourceTitleTeaching)+3, bases, len(owners), views)
}
