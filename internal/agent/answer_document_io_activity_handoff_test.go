package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestIOActivityActualFinalizerHandoffAndScope(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 2.0, 2.25
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Compare IO event sizes, rates and directions in the selected window"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
			RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Thread: "block", Source: "user_request"}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[2,2.25) seconds"}}}}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end, "bucket_ms": 100})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("real query failed: %v / %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	before, _ := json.Marshal(result)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	choices := view.RuntimeMeasurementContract.Choices()
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	activityTables := 0
	for _, table := range choices {
		if !strings.Contains(table.ObservationID, "#io_activity:") {
			continue
		}
		activityTables++
		selector := "observation_id=\"" + table.ObservationID + "\" view=\"" + string(table.View) + "\""
		if !strings.Contains(prompt, selector) || !strings.Contains(prompt, table.Label) {
			t.Fatalf("actual finalizer lost selector identity beyond data preview cap: %s", selector)
		}
	}
	if activityTables != 24 {
		t.Fatalf("expected 8 endpoint groups × 3 views, got %d", activityTables)
	}
	for _, row := range result.Observations {
		if row.Predicate == "io_activity" {
			fact := answerDocBoundedRuntimeFactAuthorityRow(row, &ctx.AnalysisIR.RequestModel, "zh")
			if strings.Contains(fact, "target_owned") || !strings.Contains(fact, "selected_window_context") {
				t.Fatal("all-issuer activity acquired target ownership through a matching thread name")
			}
		}
	}
	if strings.Contains(prompt, types.TraceNoteKeyRuntimeMeasurement+"={") || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("publication JSON leaked or display minted a root cause")
	}
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Fatal("handoff mutated native result")
	}
	start, end = 2.05, 2.15
	view = types.BuildAnswerSemanticViewForAgentContext(ctx)
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if strings.Contains(table.ObservationID, "#io_activity:") {
			t.Fatal("wider endpoint measurement borrowed into narrower requested window")
		}
	}
}
