package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFinalizerCompactsTraceDraftTeachingWithinOriginalAllocation(t *testing.T) {
	ctx := traceRootCauseTestContext("Diagnose the attached Trace without analyzing source code")
	ctx.Stage = types.StageFinalize
	ctx.TraceRootCauseBoard = "bound Trace evidence"
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"without analyzing source code"},
	}
	sk := traceTeachingSkill(t, "answer-document-skill")
	pc := &types.PromptContext{Stage: types.StageFinalize, SystemSections: []types.PromptSection{
		{Title: promptctx.SectionSkillGoal, Content: sk.Goal},
		{Title: promptctx.SectionWorkflow, Content: strings.Join(sk.Workflow, "\n")},
		{Title: promptctx.SectionOutputFormat, Content: sk.OutputFormat},
	}}
	draft := promptctx.ToMessages(pc)[0].Content
	teaching := &finalizerRepairTeaching{initialContext: pc, promptSkill: sk, draftSystem: draft}
	const allocation = 700000
	eval := &answerDocumentEvaluator{mu: ctx.Mutable, requestBytePolicy: types.FinalizerRequestByteEnforce,
		promptBudget: &finalizerPromptBudget{RequestAllocationBytes: allocation}}
	tools := []llm.ToolSchema{{Name: "emit_answer_document", Parameters: json.RawMessage(`{"type":"object"}`)}}
	for _, recordedBytes := range []int{700734, 729497} {
		messages := []llm.Message{{Role: "system", Content: draft}, {Role: "user", Content: ctx.TraceRootCauseBoard}}
		messages[1].Content += strings.Repeat("e", recordedBytes-llm.EstimateRequestBytes(messages, tools))
		projected := teaching.project(ctx, eval, messages, tools)
		if err := eval.finalizerRequestByteError(ctx, projected, tools); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(projected[1], messages[1]) || messages[0].Content != draft ||
			!strings.Contains(projected[0].Content, "Omit optional notes") {
			t.Fatal("projection must preserve evidence, keep history intact and omit optional notes")
		}
	}
}

func TestFinalizerCompactsOversizeTraceContextAndWritesHTML(t *testing.T) {
	const (
		requestBytes = 836260
		allocation   = 700000
		question     = "What delayed the selected operation? Analyze the attached Trace without analyzing source code."
		board        = "Worker 71 waited 14.2 ms in the selected window (capture.systrace lines 121–127)."
		boundary     = "The wait was measured; the initiating dependency has not been established."
		answer       = "Worker 71 waited 14.2 ms. The initiating dependency remains unresolved."
		appendix     = "Historical investigation detail for audit only.\n"
	)
	ctx := traceRootCauseTestContext(question)
	ctx.Stage, ctx.AgentName, ctx.Language = types.StageFinalize, types.AgentFinalizer, "en"
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"without analyzing source code"},
		Confidence:        1,
	}
	ctx.TraceRootCauseBoard = board
	ctx.TraceWaitEvidence = boundary
	ctx.PriorReports = []types.StageReport{{Stage: types.StageExplore, Agent: types.AgentExplorer, Findings: appendix}}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	skill := traceTeachingSkill(t, "answer-document-skill")
	newAgent := func(adapter llm.Adapter) *BaseAgent {
		return NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: 1,
			AgentSettings: types.ResolvedAgentSettings(types.AgentSettings{}),
		}).(*BaseAgent)
	}
	stop := errors.New("captured offline control")
	control := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: stop}, window: 2_000_000}
	if _, err := newAgent(control).Execute(ctx, skill); !errors.Is(err, stop) || control.calls != 1 {
		t.Fatalf("size unpadded control: calls=%d err=%v", control.calls, err)
	}
	padding := requestBytes - llm.EstimateRequestBytes(control.messages, control.tools)
	if padding <= 0 {
		t.Fatalf("control already exceeds fixture size by %d bytes", -padding)
	}
	ctx.PriorReports[0].Findings += strings.Repeat(appendix, padding/len(appendix)) + appendix[:padding%len(appendix)]
	if _, err := newAgent(control).Execute(ctx, skill); !errors.Is(err, stop) || control.calls != 2 {
		t.Fatalf("capture full context: calls=%d err=%v", control.calls, err)
	}
	if got := llm.EstimateRequestBytes(control.messages, control.tools); got != requestBytes {
		t.Fatalf("full request bytes=%d, want %d", got, requestBytes)
	}
	adapter := &hmc376Adapter{window: 200000, script: func(int) (llm.Response, error) {
		params, err := json.Marshal(map[string]any{"blocks": []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: answer}}})
		if err != nil {
			return llm.Response{}, err
		}
		return llm.Response{StopReason: "tool_use", ToolCalls: []llm.ToolCall{{ID: "emit-summary", Name: "emit_answer_document", Params: params}}}, nil
	}}
	finalizer := newAgent(adapter)
	out, err := finalizer.Execute(ctx, skill)
	if err != nil || len(adapter.messages) != 1 {
		t.Fatalf("compacted request should reach Chat once: calls=%d err=%v", len(adapter.messages), err)
	}
	budget := finalizer.eval.(*answerDocumentEvaluator).promptBudget
	if got := adapter.size(0); budget.RequestAllocationBytes != allocation || got > allocation || !reflect.DeepEqual(adapter.tools[0], control.tools) {
		t.Fatalf("compacted request=%d allocation=%d, tools must match the original request", got, budget.RequestAllocationBytes)
	}
	var request strings.Builder
	for _, m := range adapter.messages[0] {
		request.WriteString(m.Content)
	}
	for _, required := range []string{question, board, boundary} {
		if !strings.Contains(request.String(), required) {
			t.Errorf("compaction removed %q", required)
		}
	}
	if strings.Contains(request.String(), strings.TrimSpace(appendix)) || len(ctx.PriorReports[0].Findings) <= padding {
		t.Fatal("advisory appendix must be removed from the request only")
	}
	if out == nil || out.AnswerDegraded || ctx.Mutable.AnswerDocumentV2() == nil || !strings.Contains(out.FinalAnswer, answer) {
		t.Fatalf("no accepted structured answer: %+v", out)
	}
	reportPath := filepath.Join(t.TempDir(), "report.html")
	outputdump.SetExplicitReport(outputdump.ExplicitReport{HTMLPath: reportPath, SuppressDefaultDir: true})
	t.Cleanup(func() { outputdump.SetExplicitReport(outputdump.ExplicitReport{}) })
	outputdump.WriteResult(outputdump.Args{Request: question, Answer: out.FinalAnswer, HasTrace: true})
	html, err := os.ReadFile(reportPath)
	if err != nil || !strings.Contains(string(html), answer) {
		t.Fatalf("report.html missing accepted answer: %v", err)
	}
	t.Logf("full=%d sent=%d allocation=%d report_bytes=%d", requestBytes, adapter.size(0), allocation, len(html))
}
