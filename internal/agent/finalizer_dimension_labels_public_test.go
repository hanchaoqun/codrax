package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The first adapter response invokes the registered real emitter. The second
// request captures the actual post-acceptance repair message, not a helper's
// return value or a fabricated accepted document. No model service is used.
type dimensionLabelsPublicLLM struct {
	traceTeachingCaptureLLM
	initial, repair string
}

func (l *dimensionLabelsPublicLLM) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	var user []string
	for _, message := range messages {
		if message.Role == "user" {
			user = append(user, message.Content)
		}
	}
	if l.calls == 1 {
		l.initial = strings.Join(user, "\n")
		return llm.Response{StopReason: "tool_use", ToolCalls: []llm.ToolCall{{
			ID: "dimension-labels-emit", Name: "emit_answer_document",
			Params: json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"MODEL-OWNED ORIGINAL TEXT"}]}`),
		}}}, nil
	}
	l.repair = strings.Join(user, "\n")
	for _, message := range user {
		if strings.Contains(message, "Dimensions to check:") || strings.Contains(message, "待核对维度：") {
			l.repair = message // A trailing locale nudge is a separate user message.
		}
	}
	return llm.Response{}, l.stop
}

func dimensionLabelsPublicMessages(t *testing.T, lang string, dimensions []types.RequestedAnswerDimension, runtimeFallback bool, wantRepair bool) (string, string) {
	t.Helper()
	mu := types.NewMutableState("Explain the requested outputs.")
	rm := types.RequestModel{Intent: types.IntentExplain, Language: lang}
	if dimensions != nil {
		rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: dimensions}
	}
	if runtimeFallback {
		rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeRelationAnalysis, RuntimeWorkRelationRequested: true}
	}
	bus := &types.BusContext{Language: lang, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx := promptctx.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	before, err := json.Marshal(ctx.AnalysisIR)
	if err != nil {
		t.Fatal(err)
	}
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.EmitAnswerDocument{})
	reg.Register(&toolpkg.EmitAnswerDocumentPatch{})
	capture := &dimensionLabelsPublicLLM{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured real dimension repair")}}
	finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 2})
	_, err = finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if wantRepair {
		if capture.calls != 2 || !errors.Is(err, capture.stop) {
			t.Fatalf("HARNESS: expected real accepted emit then repair request; calls=%d err=%v repair=%s", capture.calls, err, capture.repair)
		}
		marker := "Dimensions to check:"
		if lang == "zh" {
			marker = "待核对维度："
		}
		if !strings.Contains(capture.repair, marker) {
			t.Fatalf("HARNESS: second request was not the dimension advisory: %s", capture.repair)
		}
	} else if err != nil || capture.calls != 1 || capture.repair != "" {
		t.Fatalf("empty/covered dimensions must not add a repair request: calls=%d err=%v repair=%s", capture.calls, err, capture.repair)
	}
	after, _ := json.Marshal(ctx.AnalysisIR)
	if string(after) != string(before) {
		t.Fatal("message assembly changed typed label/index/role/quote/order or contract")
	}
	if strings.Contains(capture.initial, "## Final Trace Decision Boundary") || strings.Contains(capture.initial, "## Trace Decision Inputs") {
		t.Fatal("ordinary presentation dimensions activated Trace-specific authority")
	}
	doc := mu.AnswerDocumentV2()
	want := []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "MODEL-OWNED ORIGINAL TEXT"}}
	if doc == nil || !reflect.DeepEqual(doc.Blocks, want) {
		t.Fatalf("real emitter did not preserve original model-owned block: %+v", doc)
	}
	e := finalizer.(*BaseAgent).eval.(*answerDocumentEvaluator)
	if e.retriesUsed != 0 || e.rejectHintsUsed != 0 || e.emitFullDocFailStreak != 0 {
		t.Fatal("soft dimension hint charged hard rejection accounting")
	}
	return capture.initial, capture.repair
}

func assertDimensionLabelDisplayRows(t *testing.T, message, lang string, dimensions []types.RequestedAnswerDimension) {
	t.Helper()
	policy := "'Internal order' only arranges the output sequence; do not include it in the answer"
	if lang == "zh" {
		policy = "“内部排序”只用于安排输出顺序，不要写进答案"
	}
	if strings.Count(message, policy) != 1 {
		t.Errorf("label/order visibility policy must occur once per message: %q", policy)
	}
	last := -1
	for _, dim := range dimensions {
		row := fmt.Sprintf("- User-facing label: %s\n  Internal order: %d", dim.Label, dim.Index)
		old := fmt.Sprintf("- Dimension %d: %s", dim.Index, dim.Label)
		if lang == "zh" {
			row = fmt.Sprintf("- 用户可见标签：%s\n  内部排序：%d", dim.Label, dim.Index)
			old = fmt.Sprintf("- 第 %d 维：%s", dim.Index, dim.Label)
		}
		at := strings.Index(message, row)
		if at < 0 {
			t.Errorf("LABEL_ORDER_CONFLATION: actual message lacks separate label/order row %q", row)
		} else if at <= last {
			t.Errorf("display row order changed for label %q", dim.Label)
		}
		last = at
		if strings.Contains(message, old) {
			t.Errorf("LABEL_ORDER_CONFLATION: actual message still publishes combined label %q", old)
		}
	}
}

func TestFinalizerDimensionLabelsActualInitialAndRepairMessages(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name string
			dims []types.RequestedAnswerDimension
		}{
			{"sparse_count", []types.RequestedAnswerDimension{{Label: "匹配条数 / Matched rows", SourceQuote: "how many matching entries", Index: 4, Role: types.RequestedAnswerDimensionCount, Required: true}}},
			{"literal_numbered_business_labels", []types.RequestedAnswerDimension{{Label: "第 4 维：实验参数 / Dimension 4: Experiment", Index: 9, Role: types.RequestedAnswerDimensionMemberSet, Required: true}}},
			{"multiple_input_order", []types.RequestedAnswerDimension{
				{Label: "Roster", Index: 9, Role: types.RequestedAnswerDimensionMemberSet, Required: true},
				{Label: "Boundary", Index: 1, Role: types.RequestedAnswerDimensionBoundary, Required: true},
				{Label: "Count", Index: 4, Role: types.RequestedAnswerDimensionCount, Required: true},
			}},
			{"legacy_nonpositive_index", []types.RequestedAnswerDimension{{Label: "Count", Index: 0, Role: types.RequestedAnswerDimensionCount, Required: true}}},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				initial, repair := dimensionLabelsPublicMessages(t, lang, tc.dims, false, true)
				assertDimensionLabelDisplayRows(t, initial, lang, tc.dims)
				if tc.name == "sparse_count" && !strings.Contains(initial, `source quote: "how many matching entries"`) {
					t.Fatal("original source quote disappeared from the initial message")
				}
				ordered := append([]types.RequestedAnswerDimension(nil), tc.dims...)
				if tc.name == "multiple_input_order" {
					ordered = []types.RequestedAnswerDimension{tc.dims[1], tc.dims[2], tc.dims[0]}
				}
				for i := range ordered {
					if ordered[i].Index <= 0 {
						ordered[i].Index = 1 // Existing repair-only fallback, not an IR edit.
					}
				}
				assertDimensionLabelDisplayRows(t, repair, lang, ordered)
				if tc.name == "multiple_input_order" && !strings.Contains(repair, `facet_ids:["member_set"]`) {
					t.Fatal("display repair lost exact roster ownership instruction")
				}
			})
		}
	}
}

func TestFinalizerDimensionLabelsActualEmptyAndFallbackControls(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, name := range []string{"no_dimensions", "empty_label", "already_covered", "runtime_fallback"} {
			t.Run(lang+"/"+name, func(t *testing.T) {
				var dims []types.RequestedAnswerDimension
				if name == "empty_label" {
					dims = []types.RequestedAnswerDimension{{Label: "  ", Index: 7, Role: types.RequestedAnswerDimensionCount, Required: true}}
				}
				if name == "already_covered" {
					dims = []types.RequestedAnswerDimension{{Label: "MODEL-OWNED ORIGINAL TEXT", Index: 4, Role: types.RequestedAnswerDimensionCount, Required: true}}
				}
				initial, repair := dimensionLabelsPublicMessages(t, lang, dims, name == "runtime_fallback", name == "runtime_fallback")
				if name == "no_dimensions" || name == "empty_label" {
					if strings.Contains(initial, "- 用户可见标签：") || strings.Contains(initial, "- User-facing label:") || strings.Contains(initial, "- 第 7 维：") || strings.Contains(initial, "- Dimension 7:") {
						t.Fatal("missing/blank label gained a visible row")
					}
				}
				if name == "runtime_fallback" {
					label := "runtime work-to-target relation"
					if lang == "zh" {
						label = "运行时工作与目标的关系"
					}
					assertDimensionLabelDisplayRows(t, repair, lang, []types.RequestedAnswerDimension{{Label: label, Index: 1}})
				}
			})
		}
	}
}
