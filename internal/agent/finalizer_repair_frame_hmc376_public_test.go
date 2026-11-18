package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	hmc376Full  = "emit_answer_document"
	hmc376Patch = "emit_answer_document_patch"
)

type hmc376Adapter struct {
	traceTeachingCaptureLLM
	window   int
	script   func(n int) (llm.Response, error)
	messages [][]llm.Message
	tools    [][]llm.ToolSchema
}

func (a *hmc376Adapter) MaxContextTokens() int { return a.window }
func (*hmc376Adapter) MaxOutputTokens() int    { return 0 }
func (a *hmc376Adapter) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	a.messages = append(a.messages, append([]llm.Message(nil), messages...))
	a.tools = append(a.tools, append([]llm.ToolSchema(nil), schemas...))
	return a.script(len(a.messages))
}

func (a *hmc376Adapter) size(i int) int {
	return llm.EstimateRequestBytes(a.messages[i], a.tools[i])
}

func (a *hmc376Adapter) names(i int) []string {
	out := make([]string, 0, len(a.tools[i]))
	for _, schema := range a.tools[i] {
		out = append(out, schema.Name)
	}
	return out
}

// A retained model-authored draft with one checkable call-chain item.
func hmc376Draft() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2", Citations: []types.Citation{{File: "x.go", Line: 10}},
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "RETAINED-DRAFT-PREMISE model-authored premise", FacetIDs: []string{string(types.FacetCurrentCodePath)}},
			{ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal, FacetIDs: []string{string(types.FacetCurrentCodePath)},
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}}, Items: []types.AnswerBlockItem{{ID: "hop", Label: "original rejected hop", CitationRef: 0}}},
		},
	}
}

type hmc376Run struct {
	window     int
	iterations int
	policy     types.FinalizerRequestBytePolicy
	fullOnly   bool
	context    func(*testing.T) *types.AgentContext
	skill      func(*skill.Config) *skill.Config
	before     func(*types.MutableState)
	script     func(n int, mutable *types.MutableState) (llm.Response, error)
}

type hmc376Result struct {
	adapter *hmc376Adapter
	state   *types.MutableState
	budget  *finalizerPromptBudget
	eval    *answerDocumentEvaluator
	err     error
}

func hmc376CallChainContext(*testing.T) *types.AgentContext {
	mutable := types.NewMutableState("Describe the verified call chain from A to B")
	return &types.AgentContext{Objective: mutable.Objective(), Stage: types.StageFinalize, AgentName: types.AgentFinalizer, Language: "en", Mutable: mutable,
		Constraints: []string{"Preserve the independent evidence constraint."},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}}}}
}

func (r hmc376Run) execute(t *testing.T) hmc376Result {
	t.Helper()
	build := r.context
	if build == nil {
		build = hmc376CallChainContext
	}
	ctx := build(t)
	if r.before != nil {
		r.before(ctx.Mutable)
	}
	policy := r.policy
	if policy == "" {
		policy = types.FinalizerRequestByteEnforce
	}
	iterations := r.iterations
	if iterations == 0 {
		iterations = 8
	}
	adapter := &hmc376Adapter{window: r.window}
	adapter.script = func(n int) (llm.Response, error) { return r.script(n, ctx.Mutable) }
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	if !r.fullOnly {
		registry.Register(&tool.EmitAnswerDocumentPatch{})
	}
	sk := traceTeachingSkill(t, "answer-document-skill")
	if r.skill != nil {
		sk = r.skill(sk)
	}
	base := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: iterations,
		AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: policy}}).(*BaseAgent)
	_, err := base.Execute(ctx, sk)
	eval := base.eval.(*answerDocumentEvaluator)
	return hmc376Result{adapter: adapter, state: ctx.Mutable, budget: eval.promptBudget, eval: eval, err: err}
}

// The window is in tokens; the allocation is seven eighths of four bytes per token.
func hmc376Window(allocation int) int { return allocation * 2 / 7 }
func hmc376Allocation(window int) int { return window*4 - window*4/8 }

func hmc376Notes(tag string, repeats int) string {
	return strings.Repeat(tag+" scratch reasoning ", repeats)
}

func hmc376CallMessage(messages []llm.Message, id string) (assistant, result llm.Message, ok bool) {
	for i, message := range messages {
		if message.Role == "assistant" && len(message.ToolCalls) == 1 && message.ToolCalls[0].ID == id && i+1 < len(messages) {
			return message, messages[i+1], true
		}
	}
	return llm.Message{}, llm.Message{}, false
}

func hmc376UnstagedPatch(id, ref string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: hmc376Patch, Params: json.RawMessage(`{"diagram_participant_edits":[{"action":"ensure_visible","participant_ref":"` + ref + `","node_id":"n1","visible_label":"reader label"}]}`)}
}

// A rejected complete draft followed by patches that are rejected before any
// staging: no later call restates the draft, so every round trip must stay.
func TestHMC376FinalizerPublicUnstagedRepairLaneStaysSendable(t *testing.T) {
	stop := errors.New("fourth request reached the adapter")
	script := func(n int, mutable *types.MutableState) (llm.Response, error) {
		switch n {
		case 1:
			mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
			return llm.Response{Content: hmc376Notes("ROUND1", 1200), ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[]}`)}}}, nil
		case 2:
			return llm.Response{Content: hmc376Notes("ROUND2", 1200), ToolCalls: []llm.ToolCall{hmc376UnstagedPatch("patch-2", "guess-two")}}, nil
		case 3:
			return llm.Response{Content: hmc376Notes("ROUND3", 300), ToolCalls: []llm.ToolCall{hmc376UnstagedPatch("patch-3", "guess-three")}}, nil
		default:
			return llm.Response{}, stop
		}
	}
	wide := hmc376Run{window: 1_000_000, script: script}.execute(t)
	if !errors.Is(wide.err, stop) || len(wide.adapter.messages) != 4 {
		t.Fatalf("fixture: calls=%d err=%v", len(wide.adapter.messages), wide.err)
	}
	var outcomes []string
	for _, result := range wide.state.DispatchToolResults() {
		if result.Repair != nil {
			outcomes = append(outcomes, result.ToolName+":"+result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome])
		}
	}
	if want := []string{hmc376Full + ":", hmc376Patch + ":" + types.AnswerDocumentPatchOutcomeNotStaged, hmc376Patch + ":" + types.AnswerDocumentPatchOutcomeNotStaged}; !reflect.DeepEqual(outcomes, want) {
		t.Fatalf("fixture must reject both patches before staging: %v", outcomes)
	}
	// Fits the first three requests once the static teaching is lane-matched,
	// and not the fourth as long as every earlier free text is re-sent.
	window := hmc376Window(wide.adapter.size(0) + 9000)
	allocation := hmc376Allocation(window)
	tight := hmc376Run{window: window, script: script}.execute(t)
	var refused *AnswerContextByteBudgetError
	if errors.As(tight.err, &refused) {
		t.Fatalf("repair request was not sent: calls=%d actual=%d allocation=%d", len(tight.adapter.messages), refused.ActualBytes, refused.AllocatedBytes)
	}
	if !errors.Is(tight.err, stop) || len(tight.adapter.messages) != 4 {
		t.Fatalf("unexpected: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	last, reference := tight.adapter.messages[3], wide.adapter.messages[3]
	if len(last) != len(reference) {
		t.Fatalf("message count changed: %d vs %d", len(last), len(reference))
	}
	identical, cleared := 0, 0
	for i := 1; i < len(reference); i++ {
		got, want := last[i], reference[i]
		if reflect.DeepEqual(got, want) {
			identical++
			continue
		}
		want.Content = ""
		if got.Role != "assistant" || len(got.ToolCalls) != 1 || !reflect.DeepEqual(got, want) {
			t.Fatalf("message %d changed beyond the free text of a superseded attempt", i)
		}
		cleared++
	}
	if cleared != 2 {
		t.Fatalf("expected exactly the two superseded attempts to lose their free text, got %d (identical=%d)", cleared, identical)
	}
	raw, _ := json.Marshal(last)
	for _, kept := range []string{"full-1", "patch-2", "patch-3", "guess-two", "guess-three", "ROUND3 scratch reasoning", "Preserve the independent evidence constraint."} {
		if !strings.Contains(string(raw), kept) {
			t.Errorf("repair request lost %q", kept)
		}
	}
	for _, stale := range []string{"ROUND1 scratch reasoning", "ROUND2 scratch reasoning"} {
		if strings.Contains(string(raw), stale) {
			t.Errorf("repair request still carries superseded free text %q", stale)
		}
	}
	if tight.adapter.size(3) > allocation || tight.state.AnswerDocumentV2() != nil || !reflect.DeepEqual(tight.adapter.tools, wide.adapter.tools) ||
		!reflect.DeepEqual(tight.state.DispatchToolResults()[0].Summary, wide.state.DispatchToolResults()[0].Summary) {
		t.Fatal("allocation bypassed, answer published, tool authority or validator output changed")
	}
}

// A dispatch that starts with a retained rejected draft shows neither that
// draft nor its feedback, so it offers the complete answer tool alone.
func TestHMC376FinalizerPublicInheritedDraftDispatchPublishesOneCatalog(t *testing.T) {
	stop := errors.New("adapter reached")
	script := func(int, *types.MutableState) (llm.Response, error) { return llm.Response{}, stop }
	inherit := func(mutable *types.MutableState) { mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft()) }
	clean := hmc376Run{window: 1_000_000, iterations: 1, script: script}.execute(t)
	wide := hmc376Run{window: 1_000_000, iterations: 1, before: inherit, script: script}.execute(t)
	if len(clean.adapter.messages) != 1 || len(wide.adapter.messages) != 1 {
		t.Fatalf("fixture: %d / %d", len(clean.adapter.messages), len(wide.adapter.messages))
	}
	if got := wide.adapter.names(0); !reflect.DeepEqual(got, []string{hmc376Full}) {
		t.Fatalf("a dispatch without an answer attempt of its own advertised %v", got)
	}
	if !reflect.DeepEqual(wide.adapter.messages[0], clean.adapter.messages[0]) || !reflect.DeepEqual(wide.adapter.tools[0], clean.adapter.tools[0]) {
		t.Fatal("the inherited draft changed the first request")
	}
	if raw, _ := json.Marshal(wide.adapter.messages[0]); strings.Contains(string(raw), "RETAINED-DRAFT-PREMISE") {
		t.Fatal("fixture premise: the inherited draft must not be visible in the request")
	}
	if wide.budget == nil || wide.budget.SchemaBytes != llm.EstimateToolSchemasBytes(wide.adapter.tools[0]) {
		t.Fatalf("the plan and the request disagree on the schema: %+v", wide.budget)
	}
	// Required units fit beside one catalog and not beside two.
	window := hmc376Window(clean.adapter.size(0) + 2048)
	tight := hmc376Run{window: window, iterations: 1, before: inherit, script: script}.execute(t)
	var refused *AnswerContextByteBudgetError
	if errors.As(tight.err, &refused) {
		t.Fatalf("no request sent: actual=%d allocation=%d", refused.ActualBytes, refused.AllocatedBytes)
	}
	if !errors.Is(tight.err, stop) || len(tight.adapter.messages) != 1 || !reflect.DeepEqual(tight.adapter.names(0), []string{hmc376Full}) ||
		tight.adapter.size(0) > hmc376Allocation(window) {
		t.Fatalf("unexpected: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	if !reflect.DeepEqual(tight.adapter.messages[0][:2], wide.adapter.messages[0][:2]) || !reflect.DeepEqual(tight.adapter.tools[0], clean.adapter.tools[0]) ||
		!reflect.DeepEqual(tight.state.LastRejectedAnswerDocumentV2(), hmc376Draft()) {
		t.Fatal("teaching, request context, the complete answer schema or the retained draft changed")
	}
}

// Without any patch tool the only lane is another complete rewrite.
func TestHMC376FinalizerPublicFullEmitOnlyLaneStaysSendable(t *testing.T) {
	stop := errors.New("third request reached the adapter")
	script := func(n int, _ *types.MutableState) (llm.Response, error) {
		switch n {
		case 1:
			return llm.Response{Content: hmc376Notes("FIRST", 1500), ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[],"caveats":["first submitted premise"]}`)}}}, nil
		case 2:
			return llm.Response{Content: hmc376Notes("SECOND", 200), ToolCalls: []llm.ToolCall{{ID: "full-2", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[],"caveats":["second submitted premise"]}`)}}}, nil
		default:
			return llm.Response{}, stop
		}
	}
	wide := hmc376Run{window: 1_000_000, fullOnly: true, script: script}.execute(t)
	if !errors.Is(wide.err, stop) || len(wide.adapter.messages) != 3 {
		t.Fatalf("fixture must produce two rejected complete rewrites: calls=%d err=%v", len(wide.adapter.messages), wide.err)
	}
	for i := range wide.adapter.messages {
		if !reflect.DeepEqual(wide.adapter.names(i), []string{hmc376Full}) {
			t.Fatalf("fixture lane changed: request %d tools=%v", i+1, wide.adapter.names(i))
		}
	}
	window := hmc376Window(wide.adapter.size(1) + 3000)
	tight := hmc376Run{window: window, fullOnly: true, script: script}.execute(t)
	var refused *AnswerContextByteBudgetError
	if errors.As(tight.err, &refused) {
		t.Fatalf("repair request was not sent: calls=%d actual=%d allocation=%d", len(tight.adapter.messages), refused.ActualBytes, refused.AllocatedBytes)
	}
	if !errors.Is(tight.err, stop) || len(tight.adapter.messages) != 3 || tight.adapter.size(2) > hmc376Allocation(window) {
		t.Fatalf("unexpected: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	if !reflect.DeepEqual(tight.adapter.messages[2][0], wide.adapter.messages[2][0]) {
		t.Fatal("a lane without a patch tool replaced its teaching")
	}
	raw, _ := json.Marshal(tight.adapter.messages[2])
	for _, kept := range []string{"full-1", "first submitted premise", "full-2", "second submitted premise", "SECOND scratch reasoning"} {
		if !strings.Contains(string(raw), kept) {
			t.Errorf("repair request lost %q", kept)
		}
	}
	if strings.Contains(string(raw), "FIRST scratch reasoning") {
		t.Error("superseded free text is still sent")
	}
	for _, id := range []string{"full-1", "full-2"} {
		_, got, _ := hmc376CallMessage(tight.adapter.messages[2], id)
		_, want, ok := hmc376CallMessage(wide.adapter.messages[2], id)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("validator result of %s changed", id)
		}
	}
}

// After a staged patch, later patches that were rolled back leave no state:
// the staged-draft checkpoint cannot be proven, the round trips stay.
func TestHMC376FinalizerPublicRepairAfterRolledBackPatch(t *testing.T) {
	stop := errors.New("fifth request reached the adapter")
	script := func(n int, mutable *types.MutableState) (llm.Response, error) {
		switch n {
		case 1:
			mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
			return llm.Response{Content: hmc376Notes("DRAFT", 1100), ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[]}`)}}}, nil
		case 2:
			return llm.Response{Content: hmc376Notes("STAGED", 900), ToolCalls: []llm.ToolCall{{ID: "staged-2", Name: hmc376Patch,
				Params: json.RawMessage(`{"unchanged_block_ids":["s1"],"replace_blocks":[{"id":"path","kind":"ordered_list","surface_role":"principal","facet_ids":["current_code_path"],"claim_uses":[{"claim_form":"call_edge"}],"items":[{"id":"hop","label":"staged hop requiring a real edge","citation_ref":0}]}]}`)}}}, nil
		case 3:
			return llm.Response{Content: hmc376Notes("ROLLED-A", 900), ToolCalls: []llm.ToolCall{hmc376UnstagedPatch("rolled-3", "guess-a")}}, nil
		case 4:
			return llm.Response{Content: hmc376Notes("ROLLED-B", 200), ToolCalls: []llm.ToolCall{hmc376UnstagedPatch("rolled-4", "guess-b")}}, nil
		default:
			return llm.Response{}, stop
		}
	}
	wide := hmc376Run{window: 1_000_000, script: script}.execute(t)
	if !errors.Is(wide.err, stop) || len(wide.adapter.messages) != 5 || wide.state.PendingAnswerDocumentPatchBase() == nil {
		t.Fatalf("fixture: calls=%d staged=%t err=%v", len(wide.adapter.messages), wide.state.PendingAnswerDocumentPatchBase() != nil, wide.err)
	}
	var outcomes []string
	for _, result := range wide.state.DispatchToolResults() {
		if result.Repair != nil {
			outcomes = append(outcomes, result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome]+"/"+result.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchFullyRevalidated])
		}
	}
	if want := []string{"/", types.AnswerDocumentPatchOutcomeStagedForRetry + "/true", types.AnswerDocumentPatchOutcomeNotStaged + "/", types.AnswerDocumentPatchOutcomeNotStaged + "/"}; !reflect.DeepEqual(outcomes, want) {
		t.Fatalf("fixture must stage once and then roll back twice: %v", outcomes)
	}
	window := hmc376Window(wide.adapter.size(0) + wide.adapter.size(0)/50)
	allocation := hmc376Allocation(window)
	tight := hmc376Run{window: window, script: script}.execute(t)
	var refused *AnswerContextByteBudgetError
	if errors.As(tight.err, &refused) {
		t.Fatalf("repair request after a rolled-back patch was not sent: calls=%d actual=%d allocation=%d", len(tight.adapter.messages), refused.ActualBytes, refused.AllocatedBytes)
	}
	if !errors.Is(tight.err, stop) || len(tight.adapter.messages) != 5 {
		t.Fatalf("unexpected: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	for request := 3; request <= 4; request++ {
		messages := tight.adapter.messages[request]
		if tight.adapter.size(request) > allocation {
			t.Fatalf("request %d bypassed the allocation", request+1)
		}
		if !reflect.DeepEqual(messages[1:3], tight.adapter.messages[0][1:3]) {
			t.Fatalf("request %d changed the request context or the evidence message", request+1)
		}
		raw, _ := json.Marshal(messages)
		for _, kept := range []string{"staged hop requiring a real edge", "staged-2", "rolled-3", "guess-a", "Preserve the independent evidence constraint."} {
			if !strings.Contains(string(raw), kept) {
				t.Errorf("request %d lost %q", request+1, kept)
			}
		}
		for _, stale := range []string{"DRAFT scratch reasoning", "STAGED scratch reasoning"} {
			if strings.Contains(string(raw), stale) {
				t.Errorf("request %d still carries superseded free text %q", request+1, stale)
			}
		}
		for _, id := range []string{"staged-2", "rolled-3"} {
			_, got, _ := hmc376CallMessage(messages, id)
			_, want, ok := hmc376CallMessage(wide.adapter.messages[request], id)
			if !ok || !reflect.DeepEqual(got, want) {
				t.Errorf("request %d changed the validator result of %s", request+1, id)
			}
		}
	}
	if raw, _ := json.Marshal(tight.adapter.messages[4]); !strings.Contains(string(raw), "rolled-4") || !strings.Contains(string(raw), "guess-b") {
		t.Error("the latest call is missing from the request")
	}
	if tight.state.AnswerDocumentV2() != nil || !reflect.DeepEqual(tight.state.LastRejectedAnswerDocumentV2(), wide.state.LastRejectedAnswerDocumentV2()) ||
		!reflect.DeepEqual(tight.state.PendingAnswerDocumentPatchBase(), wide.state.PendingAnswerDocumentPatchBase()) || !reflect.DeepEqual(tight.adapter.tools, wide.adapter.tools) {
		t.Fatal("request recovery published an answer or changed the retained draft, the staged base or the tool authority")
	}
}

// One attempt whose own free text is larger than the remaining room: the
// submitted call, its validator result and its correction are the repair.
func TestHMC376FinalizerPublicLatestAttemptFreeTextIsTheLastRung(t *testing.T) {
	stop := errors.New("second request reached the adapter")
	run := func(window int, inArguments bool) hmc376Result {
		return hmc376Run{window: window, iterations: 2, script: func(n int, mutable *types.MutableState) (llm.Response, error) {
			if n != 1 {
				return llm.Response{}, stop
			}
			mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
			bulk := hmc376Notes("ONLY", 5200)
			if inArguments {
				params, _ := json.Marshal(map[string]any{"blocks": []any{}, "caveats": []string{bulk}})
				return llm.Response{ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: params}}}, nil
			}
			return llm.Response{Content: bulk, ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[],"caveats":["the submitted premise"]}`)}}}, nil
		}}.execute(t)
	}
	wide := run(1_000_000, false)
	if !errors.Is(wide.err, stop) || len(wide.adapter.messages) != 2 || !reflect.DeepEqual(wide.adapter.names(1), []string{hmc376Patch}) {
		t.Fatalf("fixture must reach a patch-first repair request: calls=%d err=%v", len(wide.adapter.messages), wide.err)
	}
	teaching := len(wide.adapter.messages[1][0].Content)
	window := hmc376Window(wide.adapter.size(0) + 9000)
	allocation := hmc376Allocation(window)
	tight := run(window, false)
	var refused *AnswerContextByteBudgetError
	if errors.As(tight.err, &refused) {
		t.Fatalf("repair request was not sent: actual=%d allocation=%d", refused.ActualBytes, refused.AllocatedBytes)
	}
	if !errors.Is(tight.err, stop) || len(tight.adapter.messages) != 2 || tight.adapter.size(1) > allocation {
		t.Fatalf("unexpected: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	if teaching-len(tight.adapter.messages[1][0].Content) <= 0 {
		t.Fatal("fixture premise: the lane-matched teaching must already be in use before any free text is given up")
	}
	got, want := tight.adapter.messages[1], wide.adapter.messages[1]
	if len(got) != len(want) {
		t.Fatalf("message count changed: %d vs %d", len(got), len(want))
	}
	for i := 1; i < len(want); i++ {
		expected := want[i]
		if expected.Role == "assistant" {
			expected.Content = ""
		}
		if !reflect.DeepEqual(got[i], expected) {
			t.Fatalf("message %d changed beyond the free text of the attempt", i)
		}
	}
	if raw, _ := json.Marshal(got); strings.Contains(string(raw), "ONLY scratch reasoning") || !strings.Contains(string(raw), "the submitted premise") || !strings.Contains(string(raw), "full-1") {
		t.Fatal("the attempt's free text is still sent, or its submitted call was lost")
	}
	// Control: the same bulk inside the submitted arguments may not be dropped.
	blocked := run(window, true)
	if len(blocked.adapter.messages) != 1 || !errors.As(blocked.err, &refused) || refused.ActualBytes <= refused.AllocatedBytes || blocked.state.AnswerDocumentV2() != nil {
		t.Fatalf("a request that is over the allocation because of submitted arguments was sent or published: calls=%d err=%v", len(blocked.adapter.messages), blocked.err)
	}
}

// The retry lane that restates the previous emit keeps both answer tools on
// its first request, exactly as before.
func TestHMC376FinalizerPublicRetryLaneWithRenderedDraftKeepsPatchTool(t *testing.T) {
	stop := errors.New("adapter reached")
	previous, _ := json.Marshal(hmc376Draft())
	got := hmc376Run{window: 1_000_000, iterations: 1, before: func(mutable *types.MutableState) {
		mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
		mutable.SetRetryState(&types.RetryState{Attempt: 1, PrevEmitJSON: previous, PrevEmitSummary: types.RetryStateSummary{
			BlockSummaries: []types.RetryBlockSummary{{ID: "s1", Kind: types.BlockSummary, TextPreview: hmc376Draft().Blocks[0].Text}}, CitationsCount: 1,
		}})
	}, script: func(int, *types.MutableState) (llm.Response, error) { return llm.Response{}, stop }}.execute(t)
	if len(got.adapter.messages) != 1 || !reflect.DeepEqual(got.adapter.names(0), []string{hmc376Full, hmc376Patch}) {
		t.Fatalf("a dispatch that renders its previous emit must still offer the patch tool: %v", got.adapter.tools)
	}
	var instruction string
	for _, message := range got.adapter.messages[0] {
		instruction += message.Content
	}
	if !strings.Contains(instruction, "## Previous Emit") || !strings.Contains(instruction, "RETAINED-DRAFT-PREMISE") {
		t.Fatal("fixture premise: the previous emit must be visible in this request")
	}
}

// One agent serves every dispatch of a run. The attempt count belongs to the
// dispatch: after a dispatch with attempts, the next one starts again with the
// complete answer tool alone, and the patch tool returns with its own attempt.
func TestHMC376FinalizerPublicAttemptCountIsPerDispatch(t *testing.T) {
	stop := errors.New("dispatch stopped by the fixture")
	ctx := hmc376CallChainContext(t)
	adapter := &hmc376Adapter{window: 1_000_000}
	adapter.script = func(n int) (llm.Response, error) {
		switch n {
		case 1, 3:
			ctx.Mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "full-" + string(rune('0'+n)), Name: hmc376Full, Params: json.RawMessage(`{"blocks":[]}`)}}}, nil
		default:
			return llm.Response{}, stop
		}
	}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewFinalizerAgent(&Dependencies{LLM: adapter, Tools: registry, MaxIterations: 4,
		AgentSettings: types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce}}).(*BaseAgent)
	sk := traceTeachingSkill(t, "answer-document-skill")
	for dispatch := 0; dispatch < 2; dispatch++ {
		if _, err := base.Execute(ctx, sk); !errors.Is(err, stop) {
			t.Fatalf("dispatch %d: %v", dispatch+1, err)
		}
		ctx.Mutable.ResetActiveAnswerDocumentV2ForFinalizeDispatch()
	}
	if len(adapter.messages) != 4 {
		t.Fatalf("fixture: %d requests", len(adapter.messages))
	}
	surfaces := [][]string{adapter.names(0), adapter.names(1), adapter.names(2), adapter.names(3)}
	want := [][]string{{hmc376Full}, {hmc376Patch}, {hmc376Full}, {hmc376Patch}}
	if !reflect.DeepEqual(surfaces, want) {
		t.Fatalf("tool surfaces per request: %v", surfaces)
	}
	if !reflect.DeepEqual(adapter.messages[2], adapter.messages[0]) || !reflect.DeepEqual(adapter.tools[2], adapter.tools[0]) {
		t.Fatal("the second dispatch did not start from the same first request")
	}
	if ctx.Mutable.LastRejectedAnswerDocumentV2() == nil {
		t.Fatal("the retained draft must stay available as the patch base")
	}
}

// A call to a tool the request did not offer submitted nothing: the dispatch
// still has no attempt of its own and keeps its single answer tool.
func TestHMC376FinalizerPublicUnofferedPatchCallIsNotAnAttempt(t *testing.T) {
	stop := errors.New("second request reached the adapter")
	got := hmc376Run{window: 1_000_000, iterations: 2, before: func(mutable *types.MutableState) {
		mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
	}, script: func(n int, _ *types.MutableState) (llm.Response, error) {
		if n != 1 {
			return llm.Response{}, stop
		}
		return llm.Response{ToolCalls: []llm.ToolCall{hmc376UnstagedPatch("unoffered-1", "guess")}}, nil
	}}.execute(t)
	if !errors.Is(got.err, stop) || len(got.adapter.messages) != 2 {
		t.Fatalf("fixture: calls=%d err=%v", len(got.adapter.messages), got.err)
	}
	if got.eval.dispatchAnswerAttempts != 0 || !reflect.DeepEqual(got.adapter.names(1), []string{hmc376Full}) {
		t.Fatalf("an unoffered call changed the dispatch: attempts=%d tools=%v", got.eval.dispatchAnswerAttempts, got.adapter.names(1))
	}
}

// Why no in-dispatch catalog choice exists: with a patch base, the rejection of
// a single answer call always leaves the patch tool alone on the next request,
// whether that call was a complete draft, a staged patch or a rolled-back one.
func TestHMC376FinalizerPublicRecordedAttemptLeavesOneCatalog(t *testing.T) {
	stop := errors.New("fifth request reached the adapter")
	got := hmc376Run{window: 1_000_000, script: func(n int, mutable *types.MutableState) (llm.Response, error) {
		switch n {
		case 1:
			mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[]}`)}}}, nil
		case 2:
			return llm.Response{ToolCalls: []llm.ToolCall{{ID: "staged-2", Name: hmc376Patch,
				Params: json.RawMessage(`{"unchanged_block_ids":["s1"],"replace_blocks":[{"id":"path","kind":"ordered_list","surface_role":"principal","facet_ids":["current_code_path"],"claim_uses":[{"claim_form":"call_edge"}],"items":[{"id":"hop","label":"staged hop requiring a real edge","citation_ref":0}]}]}`)}}}, nil
		case 3, 4:
			return llm.Response{ToolCalls: []llm.ToolCall{hmc376UnstagedPatch("rolled-"+string(rune('0'+n)), "guess")}}, nil
		default:
			return llm.Response{}, stop
		}
	}}.execute(t)
	if !errors.Is(got.err, stop) || len(got.adapter.messages) != 5 || got.eval.dispatchAnswerAttempts != 4 {
		t.Fatalf("fixture: calls=%d attempts=%d err=%v", len(got.adapter.messages), got.eval.dispatchAnswerAttempts, got.err)
	}
	for request := 1; request < 5; request++ {
		if !reflect.DeepEqual(got.adapter.names(request), []string{hmc376Patch}) {
			t.Fatalf("request %d published %v beside a recorded attempt", request+1, got.adapter.names(request))
		}
	}
}

// A customer-authored workflow is never replaced to make room. Its attempt
// history is served like any other lane: only the free text that accompanied
// the attempt is given up.
func TestHMC376FinalizerPublicCustomWorkflowIsNeverReplaced(t *testing.T) {
	stop := errors.New("second request reached the adapter")
	custom := func(sk *skill.Config) *skill.Config {
		changed := *sk
		changed.Workflow = append(append([]string(nil), sk.Workflow...), "Custom mandatory workflow. Keep this exact.")
		return &changed
	}
	script := func(n int, mutable *types.MutableState) (llm.Response, error) {
		if n != 1 {
			return llm.Response{}, stop
		}
		mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
		return llm.Response{Content: hmc376Notes("CUSTOM", 1700), ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[],"caveats":["the submitted premise"]}`)}}}, nil
	}
	wide := hmc376Run{window: 1_000_000, iterations: 2, skill: custom, script: script}.execute(t)
	if !errors.Is(wide.err, stop) || len(wide.adapter.messages) != 2 {
		t.Fatalf("fixture: calls=%d err=%v", len(wide.adapter.messages), wide.err)
	}
	window := hmc376Window(wide.adapter.size(0) + 7000)
	tight := hmc376Run{window: window, iterations: 2, skill: custom, script: script}.execute(t)
	var refused *AnswerContextByteBudgetError
	if errors.As(tight.err, &refused) {
		t.Fatalf("repair request was not sent: actual=%d allocation=%d", refused.ActualBytes, refused.AllocatedBytes)
	}
	if !errors.Is(tight.err, stop) || len(tight.adapter.messages) != 2 || tight.adapter.size(1) > hmc376Allocation(window) {
		t.Fatalf("unexpected: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	system := tight.adapter.messages[1][0]
	if system.Role != "system" || !strings.Contains(system.Content, "Custom mandatory workflow. Keep this exact.") || !reflect.DeepEqual(system, tight.adapter.messages[0][0]) {
		t.Fatal("customer-authored workflow text was replaced")
	}
	got, want := tight.adapter.messages[1], wide.adapter.messages[1]
	if len(got) != len(want) {
		t.Fatalf("message count changed: %d vs %d", len(got), len(want))
	}
	for i := range want {
		expected := want[i]
		if expected.Role == "assistant" {
			expected.Content = ""
		}
		if !reflect.DeepEqual(got[i], expected) {
			t.Fatalf("message %d changed beyond the free text of the attempt", i)
		}
	}
	if tight.budget == nil || tight.budget.RepairTeachingDividendBytes != 0 || tight.budget.RepairFrameReserveBytes != tight.budget.RepairFrameEnvelopeBytes {
		t.Fatalf("a non-replaceable teaching has no dividend to plan with: %+v", tight.budget)
	}
}

// Required units are byte-identical in a projected repair request: the request
// context, the evidence message with its causal roster, and the constraints.
func TestHMC376FinalizerPublicRepairKeepsRequiredUnits(t *testing.T) {
	stop := errors.New("third request reached the adapter")
	run := func(window int) hmc376Result {
		return hmc376Run{window: window, iterations: 3, context: func(t *testing.T) *types.AgentContext {
			ctx := hmc362RosterContext(t)
			ctx.Constraints = []string{"Unrelated global constraint remains active in repair."}
			ctx.Preferences = []string{"Keep the exact target window."}
			return ctx
		}, script: func(n int, mutable *types.MutableState) (llm.Response, error) {
			switch n {
			case 1:
				mutable.SetLastRejectedAnswerDocumentV2(hmc376Draft())
				return llm.Response{Content: hmc376Notes("FIRST", 2600), ToolCalls: []llm.ToolCall{{ID: "full-1", Name: hmc376Full, Params: json.RawMessage(`{"blocks":[]}`)}}}, nil
			case 2:
				return llm.Response{Content: hmc376Notes("SECOND", 5000), ToolCalls: []llm.ToolCall{hmc376UnstagedPatch("patch-2", "guess-two")}}, nil
			default:
				return llm.Response{}, stop
			}
		}}.execute(t)
	}
	wide := run(2_000_000)
	if !errors.Is(wide.err, stop) || len(wide.adapter.messages) != 3 {
		t.Fatalf("fixture: calls=%d err=%v", len(wide.adapter.messages), wide.err)
	}
	// Room for the first request only: the last request needs every rung.
	window := hmc376Window(wide.adapter.size(0) + 2000)
	tight := run(window)
	var refused *AnswerContextByteBudgetError
	if errors.As(tight.err, &refused) {
		t.Fatalf("repair request was not sent: calls=%d actual=%d allocation=%d", len(tight.adapter.messages), refused.ActualBytes, refused.AllocatedBytes)
	}
	if !errors.Is(tight.err, stop) || len(tight.adapter.messages) != 3 || tight.adapter.size(2) > hmc376Allocation(window) {
		t.Fatalf("unexpected: calls=%d err=%v", len(tight.adapter.messages), tight.err)
	}
	last := tight.adapter.messages[2]
	if raw, _ := json.Marshal(last); strings.Contains(string(raw), "FIRST scratch reasoning") || strings.Contains(string(raw), "SECOND scratch reasoning") {
		t.Fatal("fixture premise: this request must have given up the free text of both attempts")
	}
	// The planner may publish an equivalent compact roster under pressure; the
	// repair projection itself never touches a required message.
	if !reflect.DeepEqual(last[1:3], tight.adapter.messages[0][1:3]) {
		t.Fatal("repair projection changed the request context or the evidence message")
	}
	wideRows, _ := hmc362RosterRows(t, wide.adapter.messages[2][2].Content, 2)
	tightRows, _ := hmc362RosterRows(t, last[2].Content, 2)
	if len(tightRows) != 2 || len(tightRows[0])+len(tightRows[1]) != 18 || !reflect.DeepEqual(wideRows, tightRows) {
		t.Fatal("causal roster rows were lost or changed")
	}
	if system := last[0].Content; !strings.Contains(system, "Unrelated global constraint remains active in repair.") || !strings.Contains(system, "Keep the exact target window.") {
		t.Fatal("repair teaching lost a global constraint or the explicit window preference")
	}
	for _, id := range []string{"full-1", "patch-2"} {
		gotCall, gotResult, _ := hmc376CallMessage(last, id)
		wantCall, wantResult, ok := hmc376CallMessage(wide.adapter.messages[2], id)
		wantCall.Content = ""
		if !ok || !reflect.DeepEqual(gotCall, wantCall) || !reflect.DeepEqual(gotResult, wantResult) {
			t.Errorf("submitted call or validator result of %s changed", id)
		}
	}
	if tight.state.AnswerDocumentV2() != nil || tight.state.TraceRootCauseReport() != nil {
		t.Fatal("request recovery published an answer or a root-cause report")
	}
}
