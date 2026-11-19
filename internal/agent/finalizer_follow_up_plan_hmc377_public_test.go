package agent

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC §377 D1 — the optional follow-up after an accepted answer is planned
// inside the unchanged request allocation. Only the model is scripted:
// BaseAgent.Execute builds the live schemas and messages, runs the real
// emit/patch tools, applies the loop policy and the unchanged dispatch guard.

const hmc377Candidate = "candidate-collection-wait"

const hmc377Answer = "The main thread waits for the collection to finish; this conclusion is the model's own."

func hmc377Context(dimensions bool) func(*testing.T) *types.AgentContext {
	return func(*testing.T) *types.AgentContext {
		mutable := types.NewMutableState("Where did the main thread spend this frame?")
		mutable.SetTraceFindingContract(&types.TraceFindingContract{
			RootCauseReportEnabled: true, CandidateSetID: "hmc377-fixed-roster",
			Candidates: []types.TraceFindingCandidateV1{{PrimaryEligible: true, Decision: types.TraceCauseDecision{
				CandidateID: hmc377Candidate, SubjectName: "main-11077",
				Token:           types.TraceCausalTokenSnapshot{Token: "gc_pause", Lane: "deterministic_runtime_work"},
				Magnitude:       &types.TypedMagnitude{Value: 123.003, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution"},
				CausalQualifier: types.TraceCausalQualifierProven, EvidenceRefs: []string{"E-wait"},
			}}},
		})
		ctx := &types.AgentContext{Objective: mutable.Objective(), AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Language: "en", Mutable: mutable}
		if dimensions {
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain,
				RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Label: "Affected threads", Role: types.RequestedAnswerDimensionMemberSet, Required: true, Index: 1}}},
			}}
		}
		return ctx
	}
}

type hmc377Live struct {
	base  *BaseAgent
	ctx   *types.AgentContext
	skill *skill.Config
	// broad is the whole compatibility catalog the follow-up request carried
	// before HMC §377: the dispatch schemas, complete answer tool dropped.
	broad []llm.ToolSchema
}

type hmc377Run struct {
	window     int
	settings   types.AgentSettings
	dimensions bool
	script     func(n int, live *hmc377Live) (llm.Response, error)
}

type hmc377Result struct {
	adapter  *hmc376Adapter
	live     *hmc377Live
	accepted *types.AnswerDocumentV2
	err      error
}

func (r hmc377Run) execute(t *testing.T) hmc377Result {
	t.Helper()
	ctx := hmc377Context(r.dimensions)(t)
	settings := r.settings
	if settings.FinalizerRequestBytePolicy == "" {
		settings.FinalizerRequestBytePolicy = types.FinalizerRequestByteEnforce
	}
	live := &hmc377Live{ctx: ctx, skill: traceTeachingSkill(t, "answer-document-skill")}
	result := hmc377Result{live: live}
	adapter := &hmc376Adapter{window: r.window}
	adapter.script = func(n int) (llm.Response, error) {
		if n == 2 {
			result.accepted = ctx.Mutable.AnswerDocumentV2()
			for _, schema := range llm.CompactToolSchemas(live.base.buildToolSchemas(live.skill, ctx)) {
				if schema.Name != hmc376Full {
					live.broad = append(live.broad, schema)
				}
			}
		}
		return r.script(n, live)
	}
	result.adapter = adapter
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	live.base = NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: 4, AgentSettings: settings}).(*BaseAgent)
	_, result.err = live.base.Execute(ctx, live.skill)
	return result
}

var hmc377Stop = errors.New("a third request reached the adapter")

// The accepted first answer arrives with the model's own working notes, as in
// batch 376 B (7,836 B of free text beside a 9,300 B call); the notes are
// sized so that the exchange outweighs the catalog difference of this
// fixture, which is what pushed the sealed follow-up past its allocation.
func hmc377Emit(reasoning string) llm.Response {
	return llm.Response{Content: hmc376Notes("DRAFT", 1250), ReasoningContent: reasoning, ToolCalls: []llm.ToolCall{{ID: "answer-1", Name: hmc376Full,
		Params: json.RawMessage(`{"blocks":[{"id":"summary","kind":"summary","text":"` + hmc377Answer + `"}]}`)}}}
}

func hmc377Select() llm.Response {
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: "follow-up-1", Name: hmc376Patch,
		Params: json.RawMessage(`{"unchanged_block_ids":["summary"],"replace_trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"` + hmc377Candidate + `"}]}}`)}}}
}

func hmc377Script(reasoning string) func(int, *hmc377Live) (llm.Response, error) {
	return func(n int, _ *hmc377Live) (llm.Response, error) {
		switch n {
		case 1:
			return hmc377Emit(reasoning), nil
		case 2:
			return hmc377Select(), nil
		}
		return llm.Response{}, hmc377Stop
	}
}

func hmc377Properties(t *testing.T, schema llm.ToolSchema) []string {
	t.Helper()
	var object struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema.Parameters, &object); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(object.Properties))
	for name := range object.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func hmc377Snapshot(t *testing.T, doc *types.AnswerDocumentV2) string {
	t.Helper()
	if doc == nil {
		t.Fatal("no accepted answer")
	}
	raw, err := json.Marshal(doc.Blocks)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The root-cause selection follow-up publishes only the two operations it
// offers. At the allocation where the whole compatibility catalog is refused
// locally (the batch 376 B shape), the follow-up is sent, the model's own
// selection reaches the report, and the accepted answer stays byte-identical.
func TestHMC377FollowUpSelectionRequestFitsAndKeepsTheAcceptedAnswer(t *testing.T) {
	wide := hmc377Run{window: 1_000_000, script: hmc377Script("")}.execute(t)
	if !errors.Is(wide.err, hmc377Stop) && wide.err != nil {
		t.Fatalf("fixture: %v", wide.err)
	}
	if len(wide.adapter.messages) != 2 || !reflect.DeepEqual(wide.adapter.names(0), []string{hmc376Full}) || !reflect.DeepEqual(wide.adapter.names(1), []string{hmc376Patch}) {
		t.Fatalf("fixture: calls=%d tools=%v", len(wide.adapter.messages), wide.adapter.tools)
	}
	followUp := wide.adapter.tools[1][0]
	if got := hmc377Properties(t, followUp); !reflect.DeepEqual(got, []string{"replace_trace_root_causes", "unchanged_block_ids"}) {
		t.Fatalf("the selection follow-up must publish only the operations it offers, got %v", got)
	}
	var broad llm.ToolSchema
	for _, schema := range wide.live.broad {
		if schema.Name == hmc376Patch {
			broad = schema
		}
	}
	broadProperties := hmc377Properties(t, broad)
	if len(broadProperties) < 10 {
		t.Fatalf("fixture: the compatibility catalog must be the whole surface, got %v", broadProperties)
	}
	// Each kept property is the dispatch-projected node, byte for byte.
	var kept, whole struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(followUp.Parameters, &kept) != nil || json.Unmarshal(broad.Parameters, &whole) != nil {
		t.Fatal("schemas are not JSON objects")
	}
	for name, node := range kept.Properties {
		if string(node) != string(whole.Properties[name]) {
			t.Fatalf("follow-up rewrote the %s schema", name)
		}
	}
	if !strings.Contains(string(kept.Properties["replace_trace_root_causes"]), hmc377Candidate) {
		t.Fatal("the follow-up lost the exact candidate roster")
	}
	sent := wide.adapter.size(1)
	whole1 := llm.EstimateRequestBytes(wide.adapter.messages[1], wide.live.broad)
	allocation := max(sent, wide.adapter.size(0)) + 512
	if whole1 <= allocation {
		t.Fatalf("fixture: the whole catalog (%d B) must exceed the allocation (%d B) as in batch 376 B", whole1, allocation)
	}
	t.Logf("first request %d B; follow-up with the whole catalog %d B; follow-up with the offered operations %d B; allocation %d B", wide.adapter.size(0), whole1, sent, allocation)

	window := hmc376Window(allocation)
	tight := hmc377Run{window: window, script: hmc377Script("")}.execute(t)
	if tight.err != nil && !errors.Is(tight.err, hmc377Stop) {
		t.Fatalf("tight run: %v", tight.err)
	}
	if len(tight.adapter.messages) != 2 {
		t.Fatalf("the follow-up request was not sent within the unchanged allocation: calls=%d", len(tight.adapter.messages))
	}
	if got := tight.adapter.size(1); got > hmc376Allocation(window) {
		t.Fatalf("sent follow-up %d B exceeds allocation %d B", got, hmc376Allocation(window))
	}
	if !reflect.DeepEqual(tight.adapter.messages[0], wide.adapter.messages[0]) || !reflect.DeepEqual(tight.adapter.messages[1], wide.adapter.messages[1]) {
		t.Fatal("planning the follow-up changed the first request or the follow-up messages")
	}
	report := tight.live.ctx.Mutable.TraceRootCauseReport()
	if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].ThreadName != "main-11077" {
		t.Fatalf("the model's selection did not reach the report: %+v", report)
	}
	if hmc377Snapshot(t, tight.accepted) != hmc377Snapshot(t, tight.live.ctx.Mutable.AnswerDocumentV2()) {
		t.Fatal("the follow-up altered the accepted answer")
	}
}

// A follow-up whose rows need the whole patch surface (here a requested
// answer dimension beside the selection) keeps that surface, and when it is
// over the allocation the batch 376 ladder now applies to it: the built-in
// static teaching is replaced by the accepted-answer follow-up representation.
func TestHMC377FollowUpWholeSurfaceUsesTheLaneMatchedTeaching(t *testing.T) {
	wide := hmc377Run{window: 1_000_000, dimensions: true, script: hmc377Script("")}.execute(t)
	if len(wide.adapter.messages) != 2 {
		t.Fatalf("fixture: calls=%d err=%v", len(wide.adapter.messages), wide.err)
	}
	if got := hmc377Properties(t, wide.adapter.tools[1][0]); len(got) < 10 {
		t.Fatalf("a follow-up with a whole-surface row must keep the whole patch surface, got %v", got)
	}
	draft := wide.adapter.messages[1][0].Content
	follow := hmc377FollowUpSystem(t, wide.live, true)
	if follow == "" || len(follow) >= len(draft) {
		t.Fatalf("fixture: follow-up teaching %d B vs draft %d B", len(follow), len(draft))
	}
	dividend := len(draft) - len(follow)
	need := wide.adapter.size(1)
	allocation := max(wide.adapter.size(0), need-dividend) + 512
	if need <= allocation {
		t.Fatalf("fixture: the follow-up (%d B) must exceed the allocation (%d B) before the teaching rung", need, allocation)
	}
	window := hmc376Window(allocation)
	tight := hmc377Run{window: window, dimensions: true, script: hmc377Script("")}.execute(t)
	if len(tight.adapter.messages) != 2 {
		t.Fatalf("the whole-surface follow-up was not sent: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	sent := tight.adapter.messages[1]
	if sent[0].Content != follow || tight.adapter.size(1) > hmc376Allocation(window) {
		t.Fatalf("follow-up teaching not applied or still over: %d B against %d B", tight.adapter.size(1), hmc376Allocation(window))
	}
	if !reflect.DeepEqual(sent[1:], wide.adapter.messages[1][1:]) || !reflect.DeepEqual(tight.adapter.tools[1], wide.adapter.tools[1]) {
		t.Fatal("the teaching rung changed something besides the static system teaching")
	}
	if hmc377Snapshot(t, tight.accepted) != hmc377Snapshot(t, tight.live.ctx.Mutable.AnswerDocumentV2()) || tight.live.ctx.Mutable.TraceRootCauseReport() == nil {
		t.Fatal("the follow-up altered the accepted answer or lost the model's selection")
	}
}

// The lane-matched teaching the dispatch prepared, rebuilt from the same
// built-in skill and an identical context.
func hmc377FollowUpSystem(t *testing.T, live *hmc377Live, dimensions bool) string {
	t.Helper()
	_, repair := live.base.buildInitialMessagesAndRepairTeaching(hmc377Context(dimensions)(t), live.skill)
	if repair == nil || repair.followUpSystem == "" || !strings.Contains(repair.followUpSystem, live.skill.AnswerDocumentFollowUpTeaching().Goal) {
		t.Fatal("an enforced built-in finalizer dispatch must prepare the follow-up teaching")
	}
	return repair.followUpSystem
}

// On the three pinned whole originals, through the actual finalizer loop with
// the batch's 800,000-byte window: the first answer omits the selection and is
// accepted, the follow-up request publishes only the selection operations for
// the real roster, and the model's selection of the first offered candidate
// reaches the report while the accepted answer stays byte-identical. The
// whole compatibility catalog the follow-up carried before is measured beside.
func TestHMC377WholeOriginalSelectionFollowUpIsPlannedAndDelivered(t *testing.T) {
	for _, c := range hmc377Captures {
		t.Run(c.name, func(t *testing.T) {
			path := hmc377PinnedCapture(t, c)
			ctx := hmc377CaptureFinalizerContext(t, c, path)
			registry := tool.NewRegistry()
			registry.Register(&tool.EmitAnswerDocument{})
			registry.Register(&tool.EmitAnswerDocumentPatch{})
			sk := traceTeachingSkill(t, "answer-document-skill")
			var base *BaseAgent
			var accepted *types.AnswerDocumentV2
			var broad []llm.ToolSchema
			var first string
			adapter := &hmc376Adapter{window: 200000}
			adapter.script = func(n int) (llm.Response, error) {
				switch n {
				case 1:
					return llm.Response{ToolCalls: []llm.ToolCall{{ID: "answer-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[` +
						`{"id":"s1","kind":"summary","surface_role":"principal","trace_causal_claim_caliber":"bounded_window_candidate","text":"主线程在所问窗口内的时间分布与链上原因见下文；这是模型自己的结论。"},` +
						`{"id":"s2","kind":"section","surface_role":"principal","title":"主线程时间去向","text":"按采集记录的线程状态与唤醒关系说明主线程在该窗口内的等待与运行。","facet_ids":["observed_artifact_fact"],"claim_uses":[{"claim_form":"external_observation"}]}]}`)}}}, nil
				case 2:
					accepted = ctx.Mutable.AnswerDocumentV2()
					for _, schema := range llm.CompactToolSchemas(base.buildToolSchemas(sk, ctx)) {
						if schema.Name != hmc376Full {
							broad = append(broad, schema)
						}
					}
					first = tracefinding.SelectableRootCauseCandidates(ctx.Mutable.TraceFindingContract())[0].Decision.CandidateID
					return llm.Response{ToolCalls: []llm.ToolCall{{ID: "follow-up-1", Name: hmc376Patch, Params: json.RawMessage(
						`{"unchanged_block_ids":["s1","s2"],"replace_trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"` + first + `"}]}}`)}}}, nil
				}
				return llm.Response{}, hmc377Stop
			}
			base = NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: 4,
				AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce, ContextPressureSoftRatio: 0.7, ContextPressureHardRatio: 0.9}}).(*BaseAgent)
			if _, err := base.Execute(ctx, sk); err != nil && !errors.Is(err, hmc377Stop) {
				t.Fatalf("finalizer: %v", err)
			}
			if len(adapter.messages) != 2 || accepted == nil {
				t.Fatalf("the first answer must be accepted and the follow-up sent: calls=%d results=%+v", len(adapter.messages), ctx.Mutable.DispatchToolResults())
			}
			if got := hmc377Properties(t, adapter.tools[1][0]); len(adapter.tools[1]) != 1 || !reflect.DeepEqual(got, []string{"replace_trace_root_causes", "unchanged_block_ids"}) {
				t.Fatalf("follow-up surface: %v", got)
			}
			report := ctx.Mutable.TraceRootCauseReport()
			if report == nil || len(report.RootCauses) != 1 {
				t.Fatalf("the model's selection did not reach the report: %+v", report)
			}
			if hmc377Snapshot(t, accepted) != hmc377Snapshot(t, ctx.Mutable.AnswerDocumentV2()) {
				t.Fatal("the follow-up altered the accepted answer")
			}
			receipt := ctx.Mutable.AnswerFollowUpDelivery()
			if receipt == nil || !receipt.Sent || receipt.RequestBytes != adapter.size(1) {
				t.Fatalf("receipt: %+v", receipt)
			}
			whole := llm.EstimateRequestBytes(adapter.messages[1], broad)
			t.Logf("%s: first request %d B; follow-up %d B with %d B of tools (whole compatibility catalog %d B -> request %d B); allocation %d B; selected %s",
				c.name, adapter.size(0), adapter.size(1), llm.EstimateToolSchemasBytes(adapter.tools[1]), llm.EstimateToolSchemasBytes(broad), whole, receipt.AllocationBytes, first)
		})
	}
}

// When even the planned follow-up cannot fit — here a provider reasoning
// channel that every rung keeps with its tool call — nothing is sent, the
// accepted answer ships unchanged, and a typed receipt records that the offer
// was not delivered and why, for both refusal points of the loop.
func TestHMC377FollowUpThatCannotFitLeavesATypedReceipt(t *testing.T) {
	reasoning := strings.Repeat("provider reasoning channel ", 4000)
	script := hmc377Script(reasoning)
	wide := hmc377Run{window: 1_000_000, script: script}.execute(t)
	if len(wide.adapter.messages) != 2 {
		t.Fatalf("fixture: calls=%d err=%v", len(wide.adapter.messages), wide.err)
	}
	sent := wide.live.ctx.Mutable.AnswerFollowUpDelivery()
	if sent == nil || !sent.Sent || !sent.RootCauseSelectionOffered || sent.NotSentCause != "" || sent.RequestBytes != wide.adapter.size(1) ||
		!reflect.DeepEqual(sent.Operations, []string{"replace_trace_root_causes", "unchanged_block_ids"}) {
		t.Fatalf("a sent follow-up must leave its typed receipt: %+v", sent)
	}
	allocation := wide.adapter.size(0) + 512
	if wide.adapter.size(1)-len(hmc376Notes("DRAFT", 1250)) <= allocation {
		t.Fatal("fixture: the reasoning channel must keep the follow-up over the allocation after every rung")
	}
	window := hmc376Window(allocation)
	for _, c := range []struct {
		name     string
		settings types.AgentSettings
		cause    types.AnswerFollowUpNotSentCause
	}{
		{"request allocation", types.AgentSettings{}, types.AnswerFollowUpNotSentRequestAllocation},
		{"context pressure", types.AgentSettings{ContextPressureSoftRatio: 0.7, ContextPressureHardRatio: 0.9}, types.AnswerFollowUpNotSentContextPressure},
	} {
		t.Run(c.name, func(t *testing.T) {
			tight := hmc377Run{window: window, settings: c.settings, script: script}.execute(t)
			if tight.err != nil || len(tight.adapter.messages) != 1 {
				t.Fatalf("an over-allocation follow-up must not be sent and must not fail the stage: calls=%d err=%v", len(tight.adapter.messages), tight.err)
			}
			mutable := tight.live.ctx.Mutable
			receipt := mutable.AnswerFollowUpDelivery()
			if !receipt.RootCauseSelectionNotSent() || receipt.NotSentCause != c.cause || receipt.RequestBytes <= hmc376Allocation(window) || receipt.AllocationBytes != hmc376Allocation(window) {
				t.Fatalf("missing or wrong typed receipt: %+v", receipt)
			}
			if mutable.TraceRootCauseReport() != nil || mutable.PendingTraceRootCauseReport() != nil || mutable.TraceRootCauseSelectorRejected() {
				t.Fatal("an undelivered offer must not select, stage or reject anything")
			}
			doc := mutable.AnswerDocumentV2()
			if hmc377Snapshot(t, doc) != hmc377Snapshot(t, wide.accepted) || !strings.Contains(hmc377Snapshot(t, doc), hmc377Answer) {
				t.Fatal("the accepted answer changed")
			}
		})
	}
}

// The lane table names only operations the dispatch patch schema publishes,
// and the union rule keeps the whole surface whenever one delivered row needs
// it. A row without a declaration stays on the whole surface.
func TestHMC377FollowUpOperationsAreDeclaredPerLane(t *testing.T) {
	ctx := hmc377Context(false)(t)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "a", Kind: types.BlockSummary, Text: "first"}, {ID: "b", Kind: types.BlockSection, Title: "Second", Text: "second"},
	}})
	var whole struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx), &whole); err != nil {
		t.Fatal(err)
	}
	declared := 0
	for _, lane := range postEmitAdvisoryLanes {
		for _, operation := range lane.operations {
			if whole.Properties[operation] == nil {
				t.Errorf("lane %s offers %q, which the dispatch patch schema does not publish", lane.kind, operation)
			}
		}
		if lane.operations != nil {
			declared++
		}
	}
	if declared != 2 {
		t.Fatalf("expected the order and root-cause selection rows to declare narrow operations, got %d", declared)
	}
	for _, c := range []struct {
		kinds []postEmitAdvisoryLaneKind
		want  []string
	}{
		{nil, nil},
		{[]postEmitAdvisoryLaneKind{postEmitAdvisoryTraceRootCauseSelection}, []string{"replace_trace_root_causes", "unchanged_block_ids"}},
		{[]postEmitAdvisoryLaneKind{postEmitAdvisoryRequestedDimensionOrder, postEmitAdvisoryTraceRootCauseSelection}, []string{"model_block_order", "unchanged_block_ids", "replace_trace_root_causes"}},
		{[]postEmitAdvisoryLaneKind{postEmitAdvisoryRequestedDimensions, postEmitAdvisoryTraceRootCauseSelection}, nil},
		{[]postEmitAdvisoryLaneKind{postEmitAdvisoryTracePrimaryCauseEntity}, nil},
	} {
		if got := postEmitAdvisoryPatchOperations(c.kinds); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %v want %v", c.kinds, got, c.want)
		}
	}
	params, ok := tool.ProjectAnswerDocumentPatchFollowUpParameters((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx), []string{"replace_trace_root_causes", "unchanged_block_ids"})
	if !ok || !json.Valid(params) {
		t.Fatal("the selection surface must project from the dispatch schema")
	}
	if _, ok := tool.ProjectAnswerDocumentPatchFollowUpParameters(params, []string{"replace_blocks"}); ok {
		t.Fatal("a projection must never advertise an operation the schema does not publish")
	}
}

// One source per rule: the follow-up representation shares the operation,
// graph and fact items with the necessary-repair representation, never calls
// the accepted answer a draft, and is never longer, so the planned repair
// frame (whose dividend is measured on the repair representation) also covers
// the follow-up request.
func TestHMC377FollowUpTeachingSharesTheRepairItems(t *testing.T) {
	sk := traceTeachingSkill(t, "answer-document-skill")
	repair, follow := sk.AnswerDocumentPatchTeaching(), sk.AnswerDocumentFollowUpTeaching()
	if repair == nil || follow == nil || len(repair.Workflow) != len(follow.Workflow) {
		t.Fatal("the built-in skill must have both representations")
	}
	for i := 1; i < len(repair.Workflow); i++ {
		if i != 2 && repair.Workflow[i] != follow.Workflow[i] {
			t.Errorf("workflow item %d is not shared", i)
		}
	}
	size := func(c *skill.Config) int {
		n := len(c.Goal) + len(c.OutputFormat)
		for _, item := range c.Workflow {
			n += len(item)
		}
		return n
	}
	if size(follow) > size(repair) {
		t.Fatalf("follow-up teaching (%d B) is longer than the repair teaching (%d B)", size(follow), size(repair))
	}
	for _, text := range []string{follow.Goal, follow.Workflow[0], follow.Workflow[2], follow.OutputFormat} {
		if strings.Contains(text, "draft") || strings.Contains(text, "defect") {
			t.Errorf("the accepted-answer follow-up is taught as a repair: %q", text)
		}
	}
	custom := *sk
	custom.Goal += " customer wording"
	if custom.AnswerDocumentFollowUpTeaching() != nil {
		t.Fatal("a customer skill must never be replaced")
	}
}
