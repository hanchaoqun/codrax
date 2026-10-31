package agent

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc326BudgetCapture struct {
	traceTeachingCaptureLLM
	window, output int
}

func hmc326DynamicRequestMessage(t *testing.T, capture *hmc326BudgetCapture) string {
	t.Helper()
	var dynamic []string
	for _, m := range capture.messages {
		if m.Role == "user" && strings.HasPrefix(m.Content, "> Note — structural emit-time constraints are listed under PRE-EMIT CONSTRAINTS") {
			dynamic = append(dynamic, m.Content)
		}
	}
	if len(dynamic) != 1 {
		t.Fatalf("expected one actual dynamic request message, got %d", len(dynamic))
	}
	return dynamic[0]
}

func TestHMC326FinalizerPublicAtomicAccountsAndOriginalTable(t *testing.T) {
	ctx, path := hmc323BusinessBudgetContext(t)
	windows := [][2]float64{{5, 5.111487}, {5.001, 5.0069}, {5.2, 5.311487}, {5.201, 5.2069}}
	for _, w := range windows {
		hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700, "target_scope": "thread", "time_start": w[0], "time_end": w[1]})
	}
	hmc320BusinessQuery(t, ctx, path, map[string]any{"view": "event_search", "time_start": 5.0, "time_end": 5.4, "limit": 40})
	// Same business labels/bytes in another capture and another query window
	// must retain independent census, paths and denominators in navigation.
	other := filepath.Join(ctx.RepoRoot, "second-capture")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	other = filepath.Join(other, filepath.Base(path))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, data, 0600); err != nil {
		t.Fatal(err)
	}
	hmc320BusinessQuery(t, ctx, other, map[string]any{"view": "event_search", "time_start": 5.0, "time_end": 5.2, "limit": 40})
	ledger := answerDocObservationLedger(ctx)
	var accounts []answerDocBusinessAccount
	for _, w := range windows {
		r, f := hmc323FindBusinessAccount(t, ledger, w[0], w[0])
		if f.Node.Self == nil || f.Node.Inclusive == nil || r.SourceRef.QueryTargetPID != 700 {
			t.Fatal("native account prerequisite")
		}
		want := 3.938
		if w[0] == 5.001 || w[0] == 5.201 {
			want = 5.9
		}
		if math.Abs(f.Node.Self.DurationMs-want) > 1e-7 {
			t.Fatalf("independent self: got %g want %g", f.Node.Self.DurationMs, want)
		}
		accounts = append(accounts, answerDocBusinessAccount{record: r, fact: f})
	}
	var selected []types.AnswerRuntimeMeasurementReceipt
	var original types.RuntimeMeasurementTable
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.EventData != nil {
			original = table
			selected = []types.AnswerRuntimeMeasurementReceipt{{ObservationID: table.ObservationID, View: table.View}}
			break
		}
	}
	if len(selected) != 1 || len(original.Rows) < 32 {
		t.Fatalf("native event original prerequisite: rows=%d", len(original.Rows))
	}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	sk := traceTeachingSkill(t, "answer-document-skill")
	// A separate baseline dispatch sizes a deliberately pressured but feasible
	// local allocation. This is not production retry/rerender or a second query.
	unbounded := &hmc326BudgetCapture{window: 0}
	baseline := NewFinalizerAgent(&Dependencies{LLM: unbounded, Tools: registry, MaxIterations: 1}).(*BaseAgent)
	baselineMessages := baseline.buildInitialMessages(ctx, sk)
	baselineBytes := llm.EstimateRequestBytes(baselineMessages, llm.CompactToolSchemas(baseline.buildToolSchemas(sk, ctx)))
	// Run the sufficient-budget control first. The LAST writing context before
	// public Emit below must be the omitted-preview dispatch, not this control.
	roomy := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured sufficient-budget request")}, window: 2000000}
	_, err = NewFinalizerAgent(&Dependencies{LLM: roomy, Tools: registry, MaxIterations: 1}).Execute(ctx, sk)
	if roomy.calls != 1 || !errors.Is(err, roomy.stop) {
		t.Fatalf("sufficient-budget request: %v", err)
	}
	var roomyText strings.Builder
	for _, m := range roomy.messages {
		roomyText.WriteString(m.Content)
		roomyText.WriteByte('\n')
	}
	if !strings.Contains(roomyText.String(), "### Trace Event Search Inventories\n") {
		t.Fatal("sufficient budget still omitted native event group")
	}
	for _, r := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(r) && strings.Contains(roomyText.String(), "\n- `"+r.ID+"`: origin=") {
			t.Fatal("actually published inventory regained duplicate generic metadata")
		}
	}
	capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured budgeted request")}, window: (baselineBytes*5 + 15) / 16}
	before := readerFieldScopePublicSnapshot(t, ctx)
	finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1}).(*BaseAgent)
	_, err = finalizer.Execute(ctx, sk)
	if capture.calls != 1 || !errors.Is(err, capture.stop) {
		t.Fatalf("feasible atomic request: calls=%d err=%v", capture.calls, err)
	}
	var text strings.Builder
	dynamic := hmc326DynamicRequestMessage(t, capture)
	for _, m := range capture.messages {
		text.WriteString(m.Content)
		text.WriteByte('\n')
	}
	content := text.String()
	for _, a := range accounts {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, tool.TraceBusinessTreeNodeText(a.fact.Node)) && strings.Contains(line, "observation_id="+quotedTreeValue(a.record.ID)) && strings.Contains(line, a.record.SourceRef.QueryScopeID) {
				found = true
			}
		}
		if !found {
			t.Errorf("full atomic query account missing: %s %s", a.record.ID, tool.TraceBusinessTreeNodeText(a.fact.Node))
		}
	}
	if !strings.Contains(content, "Native event preview dependency group not published") || strings.Contains(content, "### Trace Event Search Inventories\n") {
		t.Fatal("fixture did not exercise atomic event-preview omission")
	}
	if strings.Contains(content, "selector_row_ids=") || strings.Contains(content, "row_refs=[") {
		t.Fatal("omitted event group leaked dangling per-row references")
	}
	hmc282SelectorNotes(t, content)
	var navigations []struct {
		Source     types.ObservationSourceRef              `json:"source"`
		Query      types.TraceEventSearchInventoryQuery    `json:"query"`
		Coverage   types.TraceEventSearchInventoryCoverage `json:"coverage"`
		PromptRows int                                     `json:"prompt_rows"`
	}
	for _, line := range strings.Split(content, "\n") {
		if raw, ok := strings.CutPrefix(line, "- original_query_navigation="); ok {
			var n struct {
				Source     types.ObservationSourceRef              `json:"source"`
				Query      types.TraceEventSearchInventoryQuery    `json:"query"`
				Coverage   types.TraceEventSearchInventoryCoverage `json:"coverage"`
				PromptRows int                                     `json:"prompt_rows"`
			}
			if json.Unmarshal([]byte(raw), &n) != nil {
				t.Fatal("malformed query navigation")
			}
			navigations = append(navigations, n)
		}
	}
	if len(navigations) != 2 {
		t.Fatalf("independent query navigations=%d want2", len(navigations))
	}
	for _, n := range navigations {
		matched := false
		for _, r := range ledger.Records {
			if types.IsValidTraceEventSearchInventoryRecord(r) && n.Source.QueryScopeID == r.SourceRef.QueryScopeID {
				matched = true
				if !reflect.DeepEqual(n.Source, r.SourceRef) || !reflect.DeepEqual(n.Query, r.EventSearchInventory.Query) || !reflect.DeepEqual(n.Coverage, r.EventSearchInventory.Coverage) || n.PromptRows != 0 {
					t.Fatal("same-name captures borrowed a query scope, origin or census")
				}
			}
		}
		if !matched {
			t.Fatal("navigation gained an unknown source/query")
		}
	}
	receipt := finalizer.eval.(*answerDocumentEvaluator).promptBudget
	if receipt == nil || receipt.DynamicPublishedBytes != len(dynamic) || receipt.PreviewOmittedGroups != 1 || receipt.PreviewOmittedBytes == 0 || receipt.OutputKnown || receipt.OutputReserveBytes != nil || receipt.SchemaBytes == 0 || receipt.TailBytes == 0 {
		t.Fatalf("publication/accounting receipt inaccurate: %+v dynamic=%d", receipt, len(dynamic))
	}
	if strings.Contains(content, "## Finalizer content-byte allocation receipt") || strings.Contains(content, `"window_derived_bytes":`) {
		t.Fatal("operator byte accounting leaked into model evidence")
	}
	if llm.EstimateRequestBytes(capture.messages, capture.tools) > receipt.RequestAllocationBytes {
		t.Fatal("actual schema/tails escaped the shared allocation")
	}
	if before != readerFieldScopePublicSnapshot(t, ctx) {
		t.Fatal("budget changed native data or authority")
	}
	// The model supplies only an exact selector, not preview rows. Public emit
	// performs the existing current-source binding and full native data fill.
	raw, _ := json.Marshal(map[string]any{"blocks": []types.AnswerBlock{
		{ID: "interpretation", Kind: types.BlockSummary, Text: "Only source-bound observed facts are reported; no causal relation is claimed."},
		{ID: "native", Kind: types.BlockTable, RuntimeMeasurement: &selected[0]},
	}})
	result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || !result.Success {
		t.Fatalf("native auto-completion: %v %s", err, result.Summary)
	}
	found := false
	for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.ObservationID == original.ObservationID && block.RuntimeMeasurement.View == original.View {
			found = true
			if !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable, &original) {
				t.Fatal("original retained table became preview data")
			}
		}
	}
	if !found {
		t.Fatal("budget suppressed native auto-completion")
	}
}

func TestHMC326DeferredNativeGroupsHonorOriginalStopBoundary(t *testing.T) {
	for _, expired := range []bool{false, true} {
		ctx := &types.AgentContext{}
		trace := newAnswerDocDynamicTrace(ctx)
		trace.promptBudget = &finalizerPromptBudget{RequestAllocationBytes: 100000}
		calls := 0
		var builder strings.Builder
		builder.WriteString("protected-before\n")
		if !trace.appendSection(&builder, "observation_ledger", func() string { calls++; return "must-not-publish" }) {
			t.Fatal("fixture stopped before deferral")
		}
		builder.WriteString("protected-after\n")
		if expired {
			trace.started = time.Now().Add(-answerDocDynamicSoftBudget - time.Second)
		} else {
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			ctx.Ctx = cancelled
		}
		got := trace.finishBudgeted(builder.String())
		if calls != 0 || strings.Contains(got, "must-not-publish") || trace.promptBudget.DynamicUnexecutedGroups != 1 {
			t.Fatalf("unexecuted native group was rendered/signed: calls=%d receipt=%+v", calls, trace.promptBudget)
		}
		if strings.Count(got, "protected-before") != 1 || strings.Count(got, "protected-after") != 1 {
			t.Fatal("stopped deferred construction repeated protected sections")
		}
	}
}

func (a *hmc326BudgetCapture) MaxContextTokens() int { return a.window }
func (a *hmc326BudgetCapture) MaxOutputTokens() int  { return a.output }

// A real native query and the normal finalizer skill/schema reach the normal
// BaseAgent entry. An impossible byte allocation must not send a known no-fit
// request; this is neither a model test nor an assertion about tokenization.
func TestHMC326FinalizerPublicNoFitDoesNotCallAdapter(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
	before := readerFieldScopePublicSnapshot(t, ctx)
	capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("adapter must not receive an impossible request")}, window: 1000}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1,
		AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}})
	_, err := finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	var allocation *AnswerContextByteBudgetError
	if capture.calls != 0 || !errors.As(err, &allocation) || allocation.AllocatedBytes != 3500 || allocation.ActualBytes <= allocation.AllocatedBytes {
		t.Fatalf("no-fit request was not stopped locally: calls=%d err=%v", capture.calls, err)
	}
	if before != readerFieldScopePublicSnapshot(t, ctx) || ctx.Mutable.AnswerDocumentV2() != nil {
		t.Fatal("resource boundary changed native facts, source authority or fabricated an answer")
	}
	// Generic/custom agents deliberately keep the existing three primitives
	// and HARD last-turn behavior; embedded methods do not imply policy opt-in.
	legacy := NewBaseAgent(types.AgentFinalizer, &Dependencies{LLM: capture, Tools: registry, MaxIterations: 1}, &answerDocumentEvaluator{})
	_, err = legacy.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if legacy.finalizerContentBudget || capture.calls != 1 || !errors.Is(err, capture.stop) {
		t.Fatalf("generic entry accidentally opted in: calls=%d err=%v", capture.calls, err)
	}
}

type hmc326RepairBudgetEvaluator struct {
	*answerDocumentEvaluator
	llm *hmc278TransportLLM
}

func (e *hmc326RepairBudgetEvaluator) FilterToolSchemas(ctx *types.AgentContext, schemas []llm.ToolSchema) []llm.ToolSchema {
	out := e.answerDocumentEvaluator.FilterToolSchemas(ctx, schemas)
	if e.llm.calls > 0 && len(out) > 0 {
		out = append([]llm.ToolSchema(nil), out...)
		out[0].Description += strings.Repeat("Exact current repair tool schema. ", 20000)
	}
	return out
}

func TestHMC326FinalizerPublicRepairBudgetPreservesOnlyAcceptedAnswer(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		mu := types.NewMutableState("列出参与线程")
		ctx := &types.AgentContext{Stage: types.StageFinalize, Language: "zh", Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain,
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Label: "涉及线程清单", Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 1}}}}}}
		raw := json.RawMessage(`{"document_model":"v2","blocks":[]}`)
		if accepted {
			raw = json.RawMessage(`{"document_model":"v2","blocks":[{"id":"summary","kind":"summary","text":"已有材料尚不足以列出参与线程。"}]}`)
		}
		fake := &hmc278TransportLLM{call: llm.ToolCall{ID: "emit", Name: "emit_answer_document", Params: raw}, window: 200000, output: 4096}
		registry := tool.NewRegistry()
		registry.Register(&tool.EmitAnswerDocument{})
		registry.Register(&tool.EmitAnswerDocumentPatch{})
		base := NewFinalizerAgent(&Dependencies{LLM: fake, Tools: registry, MaxIterations: 3, AgentSettings: types.AgentSettings{ContextPressureHardRatio: .99,
			FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}}).(*BaseAgent)
		eval := &hmc326RepairBudgetEvaluator{answerDocumentEvaluator: base.eval.(*answerDocumentEvaluator), llm: fake}
		base.eval = eval
		out, err := base.Execute(ctx, &skill.Config{ToolSuggestions: []string{"emit_answer_document", "emit_answer_document_patch"}})
		if fake.calls != 1 {
			t.Fatalf("expanded repair schema escaped same dispatch byte allocation: accepted=%t calls=%d err=%v", accepted, fake.calls, err)
		}
		if accepted {
			doc := mu.AnswerDocumentV2()
			if err != nil || out == nil || doc == nil || len(doc.Blocks) != 1 || doc.Blocks[0].Text != "已有材料尚不足以列出参与线程。" || !eval.postEmitAdvisoryDelivered {
				t.Fatalf("accepted answer not preserved: %v", err)
			}
		} else {
			var allocation *AnswerContextByteBudgetError
			if !errors.As(err, &allocation) || allocation.AllocatedBytes <= 0 || allocation.ActualBytes <= allocation.AllocatedBytes || mu.AnswerDocumentV2() != nil || eval.postEmitAdvisoryDelivered {
				t.Fatalf("unaccepted draft bypassed explicit no-fit: %v", err)
			}
		}
	}
}

func TestHMC326FinalizerPublicUnknownWindowKeepsLegacyPath(t *testing.T) {
	for _, window := range []int{0, -1, int(^uint(0) >> 1)} {
		ctx, path := hmc320BusinessFinalizer(t)
		hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
		capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured unknown-window request")}, window: window}
		registry := tool.NewRegistry()
		registry.Register(&tool.EmitAnswerDocument{})
		finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1})
		_, err := finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
		if capture.calls != 1 || !errors.Is(err, capture.stop) {
			t.Fatalf("unknown/invalid window invented a byte allocation: window=%d calls=%d err=%v", window, capture.calls, err)
		}
		if got := measureContextRequestPressure(capture, capture.messages, capture.tools); got.OutputKnown || got.WindowBytes != 0 {
			t.Fatalf("unknown limits became known: %+v", got)
		}
	}
}
