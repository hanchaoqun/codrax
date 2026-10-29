package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC310NativeBoundaryActualFinalizerInstruction(t *testing.T) {
	ctx := runtimeRequiredDiagramContext(t, types.DiagramSequence, nil)
	ctx.AnalysisIR.RequestModel.DiagramHint.Participants = []types.DiagramParticipantHint{{Identity: "业务发送方", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "业务发送方"}, {Identity: "业务接收方", Role: types.DiagramParticipantIncidentRequired, SourceQuote: "业务接收方"}}
	ctx.AnalysisIR.AnswerContract.Diagram.Participants = ctx.AnalysisIR.RequestModel.DiagramHint.Participants
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"path": path, "view": "event_search", "event_types": []string{"sched_wakeup"}, "time_start": 5, "time_end": 5.041})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("public native query: %v %s", err, result.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{`requested_native_identity_unverified=["业务发送方","业务接收方"]`, "not the existence of the separately listed verified event pairs", "second <br/> line", "rather than creating a duplicate", "edge_anchor="} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("actual finalizer instruction lost %q", want)
		}
	}
	if strings.Contains(prompt, "requested_participant_binding=") {
		t.Fatal("unproved roles borrowed exact native identity")
	}
}
