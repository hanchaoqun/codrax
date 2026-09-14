package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Independent integration review: the provider's dynamic-schema compatibility
// runs before the real tool. An optional selector must survive that boundary
// without laundering an explicit version conflict into a new valid selection.
type b1675SelectionLoopLLM struct {
	b1672RootSelectionLoopLLM
	patch      bool
	extra      string
	selection  string
	repairBody bool
	envelope   string
}

func (l *b1675SelectionLoopLLM) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	l.schemas = append(l.schemas, schemas)
	l.messages = append(l.messages, messages)
	if l.calls > 2 {
		return llm.Response{}, fmt.Errorf("unexpected extra model call")
	}
	name, params := "emit_answer_document", `"blocks":[{"id":"summary","kind":"summary","text":"The model's original useful answer remains unchanged."}]`
	field := "trace_root_causes"
	if l.calls == 2 {
		name, params, field = "emit_answer_document_patch", `"unchanged_block_ids":["summary"]`, "replace_trace_root_causes"
	}
	if (l.patch && l.calls == 2) || (!l.patch && l.calls == 1) {
		if l.repairBody {
			if l.patch {
				params = `"unchanged_block_ids":"[\"summary\"]"`
			} else {
				params = `"blocks":"[{\"id\":\"summary\",\"kind\":\"summary\",\"text\":\"The model's original useful answer remains unchanged.\"}]"`
			}
		}
		selection := l.selection
		if selection == "" {
			selection = `[{"candidate_id":"candidate-sched"}]`
		}
		params += `,"` + field + `":` + selection
		if l.extra != "" {
			params += "," + l.extra
		}
	}
	raw := json.RawMessage("{" + params + "}")
	if (l.patch && l.calls == 2) || (!l.patch && l.calls == 1) {
		switch l.envelope {
		case "arguments":
			raw = json.RawMessage(`{"arguments":` + string(raw) + `}`)
		case "arguments_string":
			encoded, _ := json.Marshal(string(raw))
			raw = json.RawMessage(`{"arguments":` + string(encoded) + `}`)
		case "arguments_double_string", "arguments_triple_string":
			depth := 2
			if l.envelope == "arguments_triple_string" {
				depth = 3
			}
			encoded := b1675QuoteJSON(raw, depth)
			raw = json.RawMessage(`{"arguments":` + string(encoded) + `}`)
		case "arguments_function_metadata":
			raw = json.RawMessage(`{"arguments":` + string(raw) + `,"function":{"name":"metadata-only"}}`)
		case "arguments_repeated_metadata":
			raw = json.RawMessage(`{"id":"first","id":"second","arguments":` + string(raw) + `}`)
		case "function":
			raw = json.RawMessage(`{"type":"function","function":{"name":"` + name + `","arguments":` + string(raw) + `}}`)
		case "object_string":
			raw, _ = json.Marshal(string(raw))
		case "object_double_string":
			raw = b1675QuoteJSON(raw, 2)
		case "object_string_arguments":
			raw = b1675QuoteJSON(json.RawMessage(`{"arguments":`+string(raw)+`}`), 1)
		case "object_string_function_arguments_string":
			inner := b1675QuoteJSON(raw, 1)
			raw = b1675QuoteJSON(json.RawMessage(`{"function":{"arguments":`+string(inner)+`}}`), 1)
		}
	}
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("answer-%d", l.calls), Name: name, Params: raw}}}, nil
}

func b1675QuoteJSON(raw json.RawMessage, depth int) json.RawMessage {
	for i := 0; i < depth; i++ {
		raw, _ = json.Marshal(string(raw))
	}
	return raw
}

func TestB1675SelectorAmbiguitySurvivesApprovedArgumentEnvelope(t *testing.T) {
	for _, envelope := range []string{"arguments", "arguments_string", "function", "object_string",
		"arguments_double_string", "arguments_triple_string", "arguments_function_metadata", "arguments_repeated_metadata",
		"object_double_string", "object_string_arguments", "object_string_function_arguments_string"} {
		for _, patch := range []bool{false, true} {
			for _, ambiguous := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/patch=%t/ambiguous=%t", envelope, patch, ambiguous), func(t *testing.T) {
					selection := `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`
					if ambiguous {
						selection = `{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"unknown","candidate_id":"candidate-sched"}]}`
					}
					ctx := b1672RootSelectionLoopContext(false)
					fake := &b1675SelectionLoopLLM{patch: patch, selection: selection, repairBody: true, envelope: envelope}
					registry := tool.NewRegistry()
					registry.Register(&tool.EmitAnswerDocument{})
					registry.Register(&tool.EmitAnswerDocumentPatch{})
					base := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 2,
						ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: types.ToolParamCompatRepair}}}, &answerDocumentEvaluator{})
					out, err := base.Execute(ctx, &skill.Config{Name: "b1675-envelope-review", ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
					if err != nil || out == nil || !strings.Contains(out.FinalAnswer, "The model's original useful answer remains unchanged.") || out.AnswerDegraded {
						t.Fatalf("optional selection prevented supported envelope/body repair: err=%v out=%+v", err, out)
					}
					report := ctx.Mutable.TraceRootCauseReport()
					if ambiguous {
						if report != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || !ctx.Mutable.TraceRootCauseSelectorRejected() {
							t.Fatalf("envelope normalization lost original selection ambiguity: report=%+v rejected=%t", report, ctx.Mutable.TraceRootCauseSelectorRejected())
						}
					} else if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "RenderThread" {
						t.Fatalf("ordinary native report in supported envelope was lost: %+v", report)
					}
				})
			}
		}
	}
}

func TestB1675NativeSelectionIntegrityAcrossCompatibilityModes(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatAudit, types.ToolParamCompatRepair} {
		for _, patch := range []bool{false, true} {
			for _, depth := range []int{0, 1, 2, 3} {
				for _, ambiguous := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/patch=%t/strings=%d/ambiguous=%t", mode, patch, depth, ambiguous), func(t *testing.T) {
						selection := `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`
						if ambiguous {
							selection = `{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"unknown","candidate_id":"candidate-sched"}]}`
						}
						ctx := b1672RootSelectionLoopContext(false)
						fake := &b1675SelectionLoopLLM{patch: patch, selection: string(b1675QuoteJSON(json.RawMessage(selection), depth))}
						registry := tool.NewRegistry()
						registry.Register(&tool.EmitAnswerDocument{})
						registry.Register(&tool.EmitAnswerDocumentPatch{})
						base := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 2,
							ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: mode}}}, &answerDocumentEvaluator{})
						out, err := base.Execute(ctx, &skill.Config{Name: "b1675-compat-modes", ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
						if err != nil || out == nil || !strings.Contains(out.FinalAnswer, "The model's original useful answer remains unchanged.") || out.AnswerDegraded {
							t.Fatalf("optional selector damaged the answer: err=%v out=%+v", err, out)
						}
						report := ctx.Mutable.TraceRootCauseReport()
						// Off/audit deliberately do not decode string-valued
						// selector objects. Integrity must not grant that repair
						// authority while protecting the still-useful answer.
						if ambiguous || (depth > 0 && mode != types.ToolParamCompatRepair) {
							if report != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || !ctx.Mutable.TraceRootCauseSelectorRejected() {
								t.Fatalf("original ambiguity was lost in mode %s: report=%+v", mode, report)
							}
						} else if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "RenderThread" {
							t.Fatalf("existing native selection compatibility lost: %+v", report)
						}
					})
				}
			}
		}
	}
}

func TestB1675SelectorAmbiguitySurvivesUnrelatedAgentRepair(t *testing.T) {
	cases := []struct {
		name, selection, extra string
		valid                  bool
	}{
		{"bare valid", `[{"candidate_id":"candidate-sched"}]`, "", true},
		{"bare wrong outer version", `[{"candidate_id":"candidate-sched"}]`, `"schema_version":3`, false},
		{"bare repeated outer version", `[{"candidate_id":"candidate-sched"}]`, `"schema_version":3,"schema_version":2`, false},
		{"bare case competing outer version", `[{"candidate_id":"candidate-sched"}]`, `"SCHEMA_VERSION":3,"schema_version":2`, false},
		{"native valid", `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`, "", true},
		{"native wrong version", `{"schema_version":3,"root_causes":[{"candidate_id":"candidate-sched"}]}`, "", false},
		{"native repeated version", `{"schema_version":3,"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`, "", false},
		{"native case competing version", `{"SCHEMA_VERSION":3,"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`, "", false},
		{"native repeated identity", `{"schema_version":2,"root_causes":[{"candidate_id":"unknown","candidate_id":"candidate-sched"}]}`, "", false},
		{"native case competing identity", `{"schema_version":2,"root_causes":[{"CANDIDATE_ID":"unknown","candidate_id":"candidate-sched"}]}`, "", false},
	}
	for _, patch := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("patch=%t/%s", patch, tc.name), func(t *testing.T) {
				ctx := b1672RootSelectionLoopContext(false)
				before, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
				fake := &b1675SelectionLoopLLM{patch: patch, extra: tc.extra, selection: tc.selection, repairBody: true}
				registry := tool.NewRegistry()
				registry.Register(&tool.EmitAnswerDocument{})
				registry.Register(&tool.EmitAnswerDocumentPatch{})
				base := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 2,
					ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: types.ToolParamCompatRepair}}}, &answerDocumentEvaluator{})
				out, err := base.Execute(ctx, &skill.Config{Name: "b1675-ambiguity-review", ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
				if err != nil || out == nil || !strings.Contains(out.FinalAnswer, "The model's original useful answer remains unchanged.") || out.AnswerDegraded {
					t.Fatalf("optional selector prevented useful body repair: err=%v out=%+v", err, out)
				}
				report := ctx.Mutable.TraceRootCauseReport()
				if tc.valid {
					if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "RenderThread" {
						t.Fatalf("safe container/answer repair was lost: %+v", report)
					}
				} else if report != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || !ctx.Mutable.TraceRootCauseSelectorRejected() {
					t.Fatalf("unrelated body repair laundered selector ambiguity: report=%+v rejected=%t", report, ctx.Mutable.TraceRootCauseSelectorRejected())
				}
				after, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
				if string(before) != string(after) {
					t.Fatal("normalization changed the candidate authority")
				}
			})
		}
	}
}

func TestB1675BareSelectionThroughActualAgentCompatibility(t *testing.T) {
	for _, mode := range []string{types.ToolParamCompatOff, types.ToolParamCompatRepair} {
		for _, patch := range []bool{false, true} {
			for _, extra := range []string{"", `"schema_version":3`, `"schema_version":3,"schema_version":2`} {
				t.Run(fmt.Sprintf("%s/patch=%t/%s", mode, patch, extra), func(t *testing.T) {
					ctx := b1672RootSelectionLoopContext(false)
					before, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
					fake := &b1675SelectionLoopLLM{patch: patch, extra: extra}
					registry := tool.NewRegistry()
					registry.Register(&tool.EmitAnswerDocument{})
					registry.Register(&tool.EmitAnswerDocumentPatch{})
					base := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: fake, Tools: registry, MaxIterations: 2,
						ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: mode}}}, &answerDocumentEvaluator{})
					out, err := base.Execute(ctx, &skill.Config{Name: "b1675-review", ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
					if err != nil || out == nil || !strings.Contains(out.FinalAnswer, "The model's original useful answer remains unchanged.") || out.AnswerDegraded {
						t.Fatalf("optional selection damaged the answer: err=%v out=%+v", err, out)
					}
					props := b1672RootSelectionLoopSchema(t, fake.schemas[0], "emit_answer_document")
					b1672RootSelectionLoopTeaching(t, props["trace_root_causes"], fake.messages[0])
					report := ctx.Mutable.TraceRootCauseReport()
					if extra == "" {
						if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "RenderThread" {
							t.Fatalf("unambiguous model selection was lost: %+v", report)
						}
						wantCalls := 1
						if patch {
							wantCalls = 2
						}
						if fake.calls != wantCalls {
							t.Fatalf("recovery charged extra model round: %d != %d", fake.calls, wantCalls)
						}
					} else if report != nil || ctx.Mutable.PendingTraceRootCauseReport() != nil || !ctx.Mutable.TraceRootCauseSelectorRejected() {
						t.Fatalf("agent compatibility hid version ambiguity: report=%+v rejected=%t", report, ctx.Mutable.TraceRootCauseSelectorRejected())
					}
					after, _ := json.Marshal(ctx.Mutable.TraceFindingContract())
					if string(before) != string(after) {
						t.Fatal("recovery changed candidate authority")
					}
				})
			}
		}
	}
}
