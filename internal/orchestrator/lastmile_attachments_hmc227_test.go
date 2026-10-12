package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type lastMileHMC227LLM struct{ params json.RawMessage }

func (m *lastMileHMC227LLM) Chat(context.Context, []llm.Message, []llm.ToolSchema, llm.ChatOptions) (llm.Response, error) {
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: "actual-emit", Name: "emit_answer_document", Params: m.params}}}, nil
}
func (*lastMileHMC227LLM) ModelID() string               { return "last-mile-public" }
func (*lastMileHMC227LLM) MaxContextTokens() int         { return 128000 }
func (*lastMileHMC227LLM) MaxOutputTokens() int          { return 4096 }
func (*lastMileHMC227LLM) RequestTimeout() time.Duration { return 0 }
func (*lastMileHMC227LLM) RetryMaxAttempts() int         { return 0 }

func TestHMC227LastMileRunRecoveryDoesNotReattachFirstGraph(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolSuccessCriterion)})
	ir := dagIR(types.AnswerContract{Language: "en"})
	for i := range ir.TaskGraph.Nodes {
		if ir.TaskGraph.Nodes[i].Type == types.NodeFinalize {
			ir.TaskGraph.Nodes[i].SuccessCriteria = []types.Criterion{{Kind: types.CritCitationCountGE, Expr: "3"}}
		}
	}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	var outputs []*agent.StageOutput
	var firstText string
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			payload := `{"blocks":[{"id":"summary","kind":"summary","text":"First draft prose remains useful."},{"id":"old","kind":"diagram","diagram":{"kind":"flow","language":"mermaid","body":"flowchart TD\n old_first_node"}}]}`
			if len(outputs) > 0 {
				payload = `{"blocks":[{"id":"summary","kind":"summary","text":"Second recovery prose remains useful."},{"id":"bad","kind":"diagram","diagram":{"kind":"flow","language":"mermaid","body":"flowchart TD\n new_wrong_from --> new_wrong_to"},"edge_anchors":[{"from_node":"new_wrong_from","to_node":"new_wrong_to","from_identity":"AbsentFrom","to_identity":"AbsentTo","relation_kind":"call"}]}]}`
			}
			finalizer := agent.NewFinalizerAgent(&agent.Dependencies{LLM: &lastMileHMC227LLM{params: json.RawMessage(payload)}, Tools: registry, MaxIterations: 1, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
			out, err := finalizer.Execute(ctx, &skill.Config{Name: "answer-document-skill", ToolSuggestions: []string{"emit_answer_document"}})
			if err != nil || out == nil {
				t.Fatalf("actual finalizer failed: %v %+v", err, out)
			}
			if len(outputs) == 0 {
				if ctx.Mutable.AnswerDocumentV2() == nil || out.AnswerDegraded || !strings.Contains(out.FinalAnswer, "old_first_node") {
					t.Fatalf("first actual emit must be accepted before the scheduler criterion rejects it: %+v", out)
				}
				firstText = out.FinalAnswer
			} else if !out.SkipAnswerChecks || !out.AnswerDegraded || ctx.Mutable.AnswerDocumentV2() != nil || strings.Contains(out.FinalAnswer, "new_wrong_from") {
				t.Fatalf("second actual emit must recover with the invalid relation isolated: %+v", out)
			}
			outputs = append(outputs, out)
			return out, nil
		},
	})
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetMaxSteps(20)
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	bus, err := o.Run("Explain the recorded flow", repo, "main")
	if err != nil || len(outputs) != 2 || outputs[0] == outputs[1] {
		t.Fatalf("need actual two-output scheduler recovery: err=%v rounds=%d", err, len(outputs))
	}
	final := bus.Mutable.Result()
	if strings.Contains(final, "old_first_node") || strings.Contains(final, "new_wrong_from") {
		t.Fatalf("scheduler reattached an unchecked old graph after ParseOutput isolation: %s", final)
	}
	if !strings.Contains(final, "First draft prose remains useful") || strings.Count(final, tool.RecoveryDiagramIsolationNotice("en")) != 1 {
		t.Fatalf("recovery lost useful prose or isolation disclosure: %s", final)
	}
	if !strings.Contains(firstText, "old_first_node") || !strings.Contains(outputs[0].FinalAnswer, "old_first_node") || bus.Mutable.LastRejectedAnswerDocumentV2().Blocks[1].Diagram == nil {
		t.Fatal("raw failed draft evidence changed")
	}
	for _, attachment := range bus.Mutable.AnswerDisplayAttachments() {
		if strings.Contains(attachment.Body, "old_first_node") || strings.Contains(attachment.Body, "new_wrong_from") {
			t.Fatal("old graph retained in visible attachment carrier")
		}
	}
}

func TestHMC227LastMileNilDocAttachmentFields(t *testing.T) {
	for _, field := range []string{"diagram", "body", "title"} {
		t.Run(field, func(t *testing.T) {
			mut := types.NewMutableState("fallback")
			o := &Orchestrator{busCtx: &types.BusContext{Language: "en", Mutable: mut}}
			out := &agent.StageOutput{FinalAnswer: "Existing native evidence remains visible."}
			att := types.AnswerDisplayAttachment{Kind: types.AnswerDisplayAttachmentMarkdown, Title: "Ordinary title", Body: "Useful reference prose.", Source: types.AnswerDisplayAttachmentSourceSystemCrossCheck}
			fence := "\n```mermaid\nflowchart TD\n hidden_from --> hidden_to\n```"
			switch field {
			case "diagram":
				att.Kind, att.Body = types.AnswerDisplayAttachmentDiagram, "flowchart TD\n hidden_from --> hidden_to"
			case "body":
				att.Body += fence
			case "title":
				att.Title += fence
			}
			before := att
			o.appendAnswerDisplayAttachment(out, att)
			if strings.Contains(out.FinalAnswer, "hidden_from") || !strings.Contains(out.FinalAnswer, "Existing native evidence") || !strings.Contains(out.FinalAnswer, tool.RecoveryDiagramIsolationNotice("en")) {
				t.Fatalf("late attachment bypassed isolation: %s", out.FinalAnswer)
			}
			if strings.Contains(out.FinalAnswer, "\n****\n") {
				t.Fatal("empty disclosure title became a Markdown separator")
			}
			if field != "diagram" && !strings.Contains(out.FinalAnswer, "Useful reference prose") {
				t.Fatal("ordinary attachment prose removed")
			}
			if att != before {
				t.Fatal("raw attachment input mutated")
			}
			for _, got := range mut.AnswerDisplayAttachments() {
				if strings.Contains(got.Body+got.Title, "hidden_from") {
					t.Fatal("visible attachment state retained graph")
				}
				if field == "body" && got.Source == before.Source && got.Title != before.Title {
					t.Fatal("ordinary title changed during body isolation")
				}
			}
			o.appendAnswerDisplayAttachment(out, att)
			if strings.Count(out.FinalAnswer, tool.RecoveryDiagramIsolationNotice("en")) != 1 {
				t.Fatal("repeated late append duplicated isolation disclosure")
			}
		})
	}
}
