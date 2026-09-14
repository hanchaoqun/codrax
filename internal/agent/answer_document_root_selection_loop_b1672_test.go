package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only the LLM is scripted. BaseAgent.Execute assembles the live schemas,
// executes the real emit/patch tools, applies LoopPolicy, and renders output.
// The candidate contract is a frozen typed fixture, not a live trace query.
type b1672RootSelectionLoopLLM struct {
	responseMode       string
	mu                 *types.MutableState
	calls              int
	schemas            [][]llm.ToolSchema
	messages           [][]llm.Message
	acceptedAtFollowup *types.AnswerDocumentV2
}

func (l *b1672RootSelectionLoopLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	l.schemas = append(l.schemas, append([]llm.ToolSchema(nil), schemas...))
	l.messages = append(l.messages, append([]llm.Message(nil), messages...))
	if l.calls == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "first-answer", Name: "emit_answer_document", Params: json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"The model's original useful answer remains unchanged."}]}`)}}}, nil
	}
	if l.calls != 2 {
		return llm.Response{}, fmt.Errorf("unexpected third model request after the one optional supplement")
	}
	l.acceptedAtFollowup = l.mu.AnswerDocumentV2()
	var params string
	switch l.responseMode {
	case "choose":
		params = `{"unchanged_block_ids":["summary"],"replace_trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}}`
	case "omit":
		params = `{"unchanged_block_ids":["summary"]}`
	case "rejected-patch":
		params = `{"unchanged_block_ids":["nonexistent-model-block"]}`
	default:
		return llm.Response{}, fmt.Errorf("unknown scripted response %q", l.responseMode)
	}
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: "optional-supplement", Name: "emit_answer_document_patch", Params: json.RawMessage(params)}}}, nil
}

func (*b1672RootSelectionLoopLLM) ModelID() string               { return "b1672-scripted-finalizer" }
func (*b1672RootSelectionLoopLLM) MaxContextTokens() int         { return 200000 }
func (*b1672RootSelectionLoopLLM) MaxOutputTokens() int          { return 8192 }
func (*b1672RootSelectionLoopLLM) RequestTimeout() time.Duration { return 0 }
func (*b1672RootSelectionLoopLLM) RetryMaxAttempts() int         { return 0 }

func b1672RootSelectionLoopContext(merged bool) *types.AgentContext {
	mu := types.NewMutableState("Explain the selected trace observations")
	mu.SetTraceFindingContract(&types.TraceFindingContract{
		RootCauseReportEnabled: true, CandidateSetID: "b1672-fixed-candidate-set",
		Candidates: []types.TraceFindingCandidateV1{{PrimaryEligible: true, Decision: types.TraceCauseDecision{
			CandidateID: "candidate-sched", SubjectName: "RenderThread",
			Token:           types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
			Magnitude:       &types.TypedMagnitude{Value: 12.4, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution"},
			CausalQualifier: types.TraceCausalQualifierProven, EvidenceRefs: []string{"E-sched"},
		}}},
	})
	ctx := &types.AgentContext{AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Language: "en", Mutable: mu}
	if merged {
		ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain,
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Label: "Affected threads", Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 1}}},
		}}
	}
	return ctx
}

func b1672RootSelectionLoopSchema(t *testing.T, schemas []llm.ToolSchema, toolName string) map[string]json.RawMessage {
	t.Helper()
	for _, schema := range schemas {
		if schema.Name == toolName {
			var object struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			}
			if err := json.Unmarshal(schema.Parameters, &object); err != nil {
				t.Fatal(err)
			}
			for _, required := range object.Required {
				if required == "trace_root_causes" || required == "replace_trace_root_causes" {
					t.Fatalf("optional selection became mandatory: %s", schema.Parameters)
				}
			}
			return object.Properties
		}
	}
	return nil
}

func b1672RootSelectionLoopTeaching(t *testing.T, raw json.RawMessage, messages []llm.Message) {
	t.Helper()
	var selector struct {
		Type        string                     `json:"type"`
		Description string                     `json:"description"`
		Properties  map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &selector); err != nil {
		t.Fatal(err)
	}
	teaching := types.TraceRootCauseSelectorOutcomeTeaching()
	if selector.Type != "object" || selector.Description != teaching {
		t.Fatalf("live schema lost the native-object/shared optional contract: %s", raw)
	}
	var causes struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(selector.Properties["root_causes"], &causes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(causes.Description, "not every offered candidate") || !strings.Contains(causes.Description, "explicit empty choice or withdrawal") {
		t.Fatalf("live array teaching lost model-owned selection/empty distinction: %s", causes.Description)
	}
	foundShared := false
	for _, message := range messages {
		foundShared = foundShared || strings.Contains(message.Content, teaching)
		if strings.Contains(message.Content, "Omit the field when no candidate should be selected.") {
			t.Fatal("actual model context still conflates omission with an explicit empty choice")
		}
	}
	if !foundShared {
		t.Fatal("actual model context and live schema do not share the optional selection teaching")
	}
}

func TestB1672BaseAgentRootSelectionOptionalRoundHonorsIterationBudgetAndAnswerOwnership(t *testing.T) {
	for _, maxIterations := range []int{1, 2, 4} {
		for _, responseMode := range []string{"choose", "omit", "rejected-patch"} {
			for _, merged := range []bool{false, true} {
				t.Run(fmt.Sprintf("iterations=%d/%s/merged=%t", maxIterations, responseMode, merged), func(t *testing.T) {
					ctx := b1672RootSelectionLoopContext(merged)
					contractBefore, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
					fake := &b1672RootSelectionLoopLLM{responseMode: responseMode, mu: ctx.Mutable}
					registry := tool.NewRegistry()
					registry.Register(&tool.EmitAnswerDocument{})
					registry.Register(&tool.EmitAnswerDocumentPatch{})
					evaluator := &answerDocumentEvaluator{}
					base := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: maxIterations}, evaluator)
					out, err := base.Execute(ctx, &skill.Config{Name: "b1672-finalizer", ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
					if err != nil {
						t.Fatalf("real finalizer loop failed: %v", err)
					}
					if out == nil || !strings.Contains(out.FinalAnswer, "The model's original useful answer remains unchanged.") {
						t.Fatalf("accepted answer was not shipped: %+v", out)
					}
					doc := ctx.Mutable.AnswerDocumentV2()
					if doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].ID != "summary" || doc.Blocks[0].Text != "The model's original useful answer remains unchanged." {
						t.Fatalf("optional round changed model-owned document: %+v", doc)
					}
					if len(out.ToolResults) == 0 || !strings.Contains(out.ToolResults[0].Summary, "emit_answer_document accepted") || len(out.ToolResults[0].OptionalCarrierOutcomes) != 0 {
						t.Fatalf("fixture must begin with a real accepted optional omission: %+v", out.ToolResults)
					}
					wantCalls := 2
					if maxIterations == 1 {
						wantCalls = 1
					}
					if fake.calls != wantCalls {
						t.Fatalf("optional supplement calls=%d, want=%d (hard budget %d)", fake.calls, wantCalls, maxIterations)
					}
					if fake.calls > maxIterations {
						t.Fatalf("optional round exceeded model iteration budget: %d > %d", fake.calls, maxIterations)
					}
					first := b1672RootSelectionLoopSchema(t, fake.schemas[0], "emit_answer_document")
					if first == nil || first["trace_root_causes"] == nil {
						t.Fatal("initial live schema failed to offer optional selection")
					}
					b1672RootSelectionLoopTeaching(t, first["trace_root_causes"], fake.messages[0])
					if b1672RootSelectionLoopSchema(t, fake.schemas[0], "emit_answer_document_patch") != nil {
						t.Fatal("initial live schema exposed patch without accepted base")
					}
					if fake.calls > 1 {
						if len(out.ToolResults) != 2 || out.ToolResults[1].ToolName != "emit_answer_document_patch" {
							t.Fatalf("followup did not execute the real patch tool: %+v", out.ToolResults)
						}
						if out.ToolResults[1].Success == (responseMode == "rejected-patch") {
							t.Fatalf("unexpected actual patch outcome for %s: %+v", responseMode, out.ToolResults[1])
						}
						second := b1672RootSelectionLoopSchema(t, fake.schemas[1], "emit_answer_document_patch")
						if second == nil || second["replace_trace_root_causes"] == nil {
							t.Fatal("followup live schema failed to expose executable root-cause patch")
						}
						b1672RootSelectionLoopTeaching(t, second["replace_trace_root_causes"], fake.messages[1])
						if fake.acceptedAtFollowup == nil || len(fake.acceptedAtFollowup.Blocks) != 1 || fake.acceptedAtFollowup.Blocks[0].Text != doc.Blocks[0].Text {
							t.Fatal("first answer was not accepted before optional followup")
						}
						var supplement string
						for _, message := range fake.messages[1] {
							if message.Role == "user" {
								supplement = message.Content
							}
						}
						if !strings.Contains(supplement, "replace_trace_root_causes") {
							t.Fatalf("actual second model request lost optional-selection guidance: %s", supplement)
						}
						if merged && !strings.Contains(supplement, "Affected threads") {
							t.Fatalf("existing advice was not merged into same model opportunity: %s", supplement)
						}
					}
					if evaluator.retriesUsed != 0 || evaluator.rejectHintsUsed != 0 || evaluator.emitFullDocFailStreak != 0 {
						t.Fatal("optional round charged or entered hard-reject repair accounting")
					}
					report := ctx.Mutable.TraceRootCauseReport()
					if maxIterations > 1 && responseMode == "choose" {
						if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "RenderThread" {
							t.Fatalf("model-selected root was not retained: %+v", report)
						}
					} else if report != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil {
						t.Fatal("system fabricated a selection when model omitted or patch failed")
					}
					contractAfter, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
					if string(contractBefore) != string(contractAfter) {
						t.Fatal("optional opportunity changed frozen candidate authority")
					}
				})
			}
		}
	}
}
