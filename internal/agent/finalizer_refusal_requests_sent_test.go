package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The byte guard's typed error says how many requests its dispatch had already
// sent. Callers decide on that count whether anything reached the model; the
// error text is not a signal and must not change.

func TestAnswerRequestRefusedFirstReportsNoRequestSent(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
	capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("adapter must not receive an impossible request")}, window: 1000}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1,
		AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}})
	_, err := finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	var refused *AnswerContextByteBudgetError
	if !errors.As(err, &refused) || capture.calls != 0 {
		t.Fatalf("fixture must refuse the first request locally: calls=%d err=%v", capture.calls, err)
	}
	if refused.RequestsSent != 0 {
		t.Fatalf("a refused first request sent nothing; RequestsSent=%d", refused.RequestsSent)
	}
	if strings.Contains(err.Error(), "RequestsSent") || !strings.Contains(err.Error(), "no request sent") {
		t.Fatalf("the error text is not the carrier of the count: %v", err)
	}
}

func TestAnswerRequestRefusedOnRepairReportsRequestsAlreadySent(t *testing.T) {
	mu := types.NewMutableState("列出参与线程")
	ctx := &types.AgentContext{Stage: types.StageFinalize, Language: "zh", Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain,
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Label: "涉及线程清单", Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 1}}}}}}
	fake := &hmc278TransportLLM{call: llm.ToolCall{ID: "emit", Name: "emit_answer_document", Params: json.RawMessage(`{"document_model":"v2","blocks":[]}`)}, window: 200000, output: 4096}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewFinalizerAgent(&Dependencies{LLM: fake, Tools: registry, MaxIterations: 3, AgentSettings: types.AgentSettings{ContextPressureHardRatio: .99,
		FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}}).(*BaseAgent)
	base.eval = &hmc326RepairBudgetEvaluator{answerDocumentEvaluator: base.eval.(*answerDocumentEvaluator), llm: fake}
	_, err := base.Execute(ctx, &skill.Config{ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
	var refused *AnswerContextByteBudgetError
	if !errors.As(err, &refused) || fake.calls != 1 {
		t.Fatalf("fixture must refuse the repair request after one sent request: calls=%d err=%v", fake.calls, err)
	}
	if refused.RequestsSent != fake.calls {
		t.Fatalf("RequestsSent=%d, the adapter received %d request(s)", refused.RequestsSent, fake.calls)
	}
}
