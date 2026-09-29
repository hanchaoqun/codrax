package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceTeachingScopeActualFinalizerPreservesEvidence(t *testing.T) {
	result := traceSemanticPublicResult(t, "worker-100 (100) [000] .... 2.010000: tracing_mark_write: I|100|business-note\n")
	ctx := traceEventInventoryPublicContext([]types.ToolResult{result})
	ctx.AttachedHitraceSource = "capture.trace"
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOccurrenceTime, types.RuntimeQuestionFactOtherObservedValue}}
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured scoped teaching")}
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocument{})
	_, err := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("capture: %v", err)
	}
	var body strings.Builder
	for _, message := range capture.messages {
		body.WriteString(message.Content)
	}
	for _, absent := range []string{"TRACE ANSWER SKELETON:", "ROOT-CAUSE BOARD ORDER:", "TARGET WAIT OCCURRENCE AUTHORITY:", "Scheduler transition interval hint:", "Target CPU and state-time caliber hint:"} {
		if strings.Contains(body.String(), absent) {
			t.Errorf("irrelevant teaching: %s", absent)
		}
	}
	for _, present := range []string{"business-note", "Trace Event Search Inventories", "PROSE NUMBER GROUNDING:", "INFERRED ATTRIBUTION DISCLOSURE:", "CHANNEL WORDS PER ROW:"} {
		if !strings.Contains(body.String(), present) {
			t.Errorf("lost facts/essential boundary: %s", present)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("teaching changed evidence")
	}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	if !strings.Contains(renderAnswerDocRuntimeTraceAnswerGuidance(ctx), "Scheduler transition interval hint:") {
		t.Fatal("causal teaching lost")
	}
}
