package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc366RepairAdapter struct {
	traceTeachingCaptureLLM
	window int
	stop   error
	before func()
	msgs   [][]llm.Message
	tools  [][]llm.ToolSchema
}

func (a *hmc366RepairAdapter) MaxContextTokens() int { return a.window }
func (a *hmc366RepairAdapter) MaxOutputTokens() int  { return 0 }
func (a *hmc366RepairAdapter) ModelID() string       { return "repair-budget-test" }
func (a *hmc366RepairAdapter) Chat(_ context.Context, msgs []llm.Message, tools []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.msgs = append(a.msgs, append([]llm.Message(nil), msgs...))
	a.tools = append(a.tools, append([]llm.ToolSchema(nil), tools...))
	if len(a.msgs) != 1 {
		return llm.Response{}, a.stop
	}
	if a.before != nil {
		a.before()
	}
	return llm.Response{Content: strings.Repeat("untrusted draft thought ", 1700), ToolCalls: []llm.ToolCall{{
		ID: "public-emit-1", Name: "emit_answer_document", Params: json.RawMessage(`{"blocks":[]}`),
	}}}, nil
}

func TestHMC366FinalizerPublicRepairRebudgetsStaticTeachingOnly(t *testing.T) {
	run := func(window int, custom bool) (*hmc366RepairAdapter, *types.AgentContext, error) {
		ctx := hmc362RosterContext(t)
		ctx.Constraints = []string{"Unrelated global constraint remains active in repair."}
		ctx.Preferences = []string{"Keep the exact target window."}
		ctx.ThinkAloud = true
		// Supply a real retained draft as the empty-block validator's previous
		// input. Install it only after iteration 0 so that its active tool
		// catalog models the transition of the real first-reject request.
		adapter := &hmc366RepairAdapter{window: window, stop: errors.New("reached adapter after actual tool rejection"), before: func() {
			ctx.Mutable.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "original", Kind: types.BlockSummary, Text: "The prior model-authored content must remain in the base.",
			}}})
		}}
		reg := tool.NewRegistry()
		reg.Register(&tool.EmitAnswerDocument{})
		reg.Register(&tool.EmitAnswerDocumentPatch{})
		base := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: reg, MaxIterations: 2,
			AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}})
		sk := traceTeachingSkill(t, "answer-document-skill")
		if custom {
			copy := *sk
			copy.Workflow = append(append([]string(nil), sk.Workflow...), "Custom mandatory workflow. Keep this exact.")
			sk = &copy
		}
		_, err := base.Execute(ctx, sk)
		return adapter, ctx, err
	}
	roomy, _, err := run(1_000_000, false)
	if !errors.Is(err, roomy.stop) || len(roomy.msgs) != 2 || len(roomy.tools[1]) != 1 || roomy.tools[1][0].Name != "emit_answer_document_patch" {
		t.Fatalf("baseline must exercise a real patch-first transition: calls=%d err=%v", len(roomy.msgs), err)
	}
	first := llm.EstimateRequestBytes(roomy.msgs[0], roomy.tools[0])
	second := llm.EstimateRequestBytes(roomy.msgs[1], roomy.tools[1])
	if second-first < 20000 {
		t.Fatalf("fixture did not create retry-only pressure: first=%d second=%d", first, second)
	}
	window := (first + 7000) * 2 / 7 // fit the initial request, not expanded repair
	tight, ctx, err := run(window, false)
	if !errors.Is(err, tight.stop) || len(tight.msgs) != 2 {
		t.Fatalf("local repair was never sent: first=%d expanded=%d calls=%d err=%v", first, second, len(tight.msgs), err)
	}
	if !reflect.DeepEqual(tight.msgs[0], roomy.msgs[0]) || !reflect.DeepEqual(tight.tools, roomy.tools) {
		t.Fatal("budget projection changed the first draft or canonical tool choices")
	}
	if !reflect.DeepEqual(tight.msgs[1][1:], roomy.msgs[1][1:]) {
		t.Fatal("repair budgeting changed evidence, tool history, or validator feedback")
	}
	if system := tight.msgs[1][0].Content; !strings.Contains(system, "Unrelated global constraint remains active in repair.") ||
		!strings.Contains(system, "Keep the exact target window.") || !strings.Contains(system, "projected patch tool schema") ||
		strings.Contains(system, "TRACE ANSWER SKELETON:") || strings.Contains(system, "First dispatch / no prior emit") {
		t.Fatal("repair instructions lost global constraints or still teach first draft")
	}
	if llm.EstimateRequestBytes(tight.msgs[1], tight.tools[1]) > window*4-window*4/8 ||
		ctx.Mutable.AnswerDocumentV2() != nil || ctx.Mutable.LastRejectedAnswerDocumentV2().Blocks[0].ID != "original" {
		t.Fatal("repair bypassed the allocation or accepted/rewrote the draft")
	}
	custom, _, err := run(window, true)
	var budgetErr *AnswerContextByteBudgetError
	if len(custom.msgs) != 1 || !errors.As(err, &budgetErr) {
		t.Fatalf("cannot replace customer-authored workflow: calls=%d err=%v", len(custom.msgs), err)
	}
}
