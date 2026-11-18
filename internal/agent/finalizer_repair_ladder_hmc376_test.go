package agent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc376Exchange(id, name, freeText string) []llm.Message {
	return []llm.Message{
		{Role: "assistant", Content: freeText, ToolCalls: []llm.ToolCall{{ID: id, Name: name, Params: json.RawMessage(`{"submitted":"` + id + `"}`)}}},
		{Role: "tool", ToolCallID: id, Content: id + " validator result"},
		{Role: "user", Content: id + " correction"},
	}
}

func hmc376Rejected(name string) []types.ToolResult {
	return []types.ToolResult{{ToolName: name, Repair: &types.ToolRepair{}}}
}

// hmc376History records three answer attempts the way the loop does: each
// exchange is observed right after its own correction was appended.
func hmc376History(t *testing.T, change func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult)) (*finalizerRepairHistory, []llm.Message) {
	t.Helper()
	mutable := types.NewMutableState("question stays byte-exact")
	history := &finalizerRepairHistory{}
	messages := []llm.Message{{Role: "system", Content: "teaching"}, {Role: "user", Content: "request"}, {Role: "user", Content: "evidence"}}
	for n, attempt := range []struct{ id, name, text string }{
		{"one", hmc376Full, "FIRST free text"}, {"two", hmc376Patch, "SECOND free text"}, {"three", hmc376Patch, "THIRD free text"},
	} {
		exchange, results := hmc376Exchange(attempt.id, attempt.name, attempt.text), hmc376Rejected(attempt.name)
		if change != nil {
			exchange, results = change(n, exchange, results)
		}
		start := len(messages)
		messages = append(messages, exchange...)
		if results != nil {
			history.observe(mutable, messages, start, start+2, results)
		}
	}
	return history, append(messages, llm.Message{Role: "system", Content: "current tool surface"})
}

func hmc376Remove(messages []llm.Message, id string) []llm.Message {
	var out []llm.Message
	for _, message := range messages {
		owned := message.ToolCallID == id || strings.HasPrefix(message.Content, id+" ")
		for _, call := range message.ToolCalls {
			owned = owned || call.ID == id
		}
		if !owned {
			out = append(out, message)
		}
	}
	return out
}

// Every precondition of the free-text rungs is structural. Each row changes
// one of them; cleared lists the call identities whose free text is dropped
// (nil means no projection at all).
func TestHMC376RepairFreeTextNeedsPairedRecordedAttempts(t *testing.T) {
	cases := []struct {
		name      string
		record    func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult)
		request   func(messages []llm.Message) []llm.Message
		recorded  []string
		supersede []string
		all       []string
	}{
		{name: "recorded attempts", recorded: []string{"one", "two", "three"}, supersede: []string{"one", "two"}, all: []string{"one", "two", "three"}},
		{name: "single attempt", record: func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult) {
			if n > 0 {
				return nil, nil
			}
			return exchange, results
		}, recorded: []string{"one"}, supersede: nil, all: []string{"one"}},
		{name: "duplicate call identity", record: func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult) {
			if n == 2 {
				exchange[0].ToolCalls[0].ID, exchange[1].ToolCallID = "one", "one"
			}
			return exchange, results
		}, recorded: []string{"one", "two", "one"}},
		{name: "result no longer follows its call", request: func(messages []llm.Message) []llm.Message {
			out := append([]llm.Message(nil), messages[:4]...)
			out = append(out, llm.Message{Role: "user", Content: "inserted between a call and its result"})
			return append(out, messages[4:]...)
		}, recorded: []string{"one", "two", "three"}},
		{name: "identity answered twice", request: func(messages []llm.Message) []llm.Message {
			return append(append([]llm.Message(nil), messages...), llm.Message{Role: "tool", ToolCallID: "two", Content: "second answer"})
		}, recorded: []string{"one", "two", "three"}},
		{name: "content parts", request: func(messages []llm.Message) []llm.Message {
			out := append([]llm.Message(nil), messages...)
			out[3].ContentParts = []llm.ContentPart{{Type: "text", Text: "part"}}
			return out
		}, recorded: []string{"one", "two", "three"}},
		{name: "renamed call", request: func(messages []llm.Message) []llm.Message {
			out := append([]llm.Message(nil), messages...)
			out[3].ToolCalls = []llm.ToolCall{{ID: "one", Name: "read_file", Params: messages[3].ToolCalls[0].Params}}
			return out
		}, recorded: []string{"one", "two", "three"}},
		{name: "latest attempt missing", request: func(messages []llm.Message) []llm.Message { return hmc376Remove(messages, "three") },
			recorded: []string{"one", "two", "three"}},
		{name: "attempt already replaced by the staged-draft checkpoint", request: func(messages []llm.Message) []llm.Message { return hmc376Remove(messages, "one") },
			recorded: []string{"one", "two", "three"}, supersede: []string{"two"}, all: []string{"two", "three"}},
		{name: "batch with two results is not recorded", record: func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult) {
			if n == 1 {
				results = append(results, types.ToolResult{ToolName: "read_file"})
			}
			return exchange, results
		}, recorded: []string{"one", "three"}, supersede: []string{"one"}, all: []string{"one", "three"}},
		{name: "another tool is not recorded", record: func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult) {
			if n == 1 {
				exchange[0].ToolCalls[0].Name, results[0].ToolName = "read_file", "read_file"
			}
			return exchange, results
		}, recorded: []string{"one", "three"}, supersede: []string{"one"}, all: []string{"one", "three"}},
		{name: "accepted and optional outcomes are still recorded", record: func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult) {
			if n == 0 {
				results[0].Repair, results[0].OptionalCarrierOutcomes = nil, []types.OptionalCarrierOutcome{{}}
			}
			return exchange, results
		}, recorded: []string{"one", "two", "three"}, supersede: []string{"one", "two"}, all: []string{"one", "two", "three"}},
		{name: "reasoning on the provider's own channel stays with its call", record: func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult) {
			exchange[0].ReasoningContent = exchange[0].ToolCalls[0].ID + " native reasoning"
			return exchange, results
		}, recorded: []string{"one", "two", "three"}, supersede: []string{"one", "two"}, all: []string{"one", "two", "three"}},
		{name: "nothing to give up", record: func(n int, exchange []llm.Message, results []types.ToolResult) ([]llm.Message, []types.ToolResult) {
			exchange[0].Content = ""
			return exchange, results
		}, recorded: []string{"one", "two", "three"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			history, messages := hmc376History(t, c.record)
			var recorded []string
			for _, call := range history.calls {
				recorded = append(recorded, call.id)
			}
			if !reflect.DeepEqual(recorded, c.recorded) {
				t.Fatalf("recorded identities: %v", recorded)
			}
			if c.request != nil {
				messages = c.request(messages)
			}
			before, _ := json.Marshal(messages)
			for _, mode := range []struct {
				latest bool
				want   []string
			}{{false, c.supersede}, {true, c.all}} {
				got, removed := history.projectFreeText(messages, mode.latest)
				if after, _ := json.Marshal(messages); string(after) != string(before) {
					t.Fatal("projection changed the original history")
				}
				if mode.want == nil {
					if got != nil || removed != 0 {
						t.Fatalf("latest=%t: an unproven shape was projected", mode.latest)
					}
					continue
				}
				if len(got) != len(messages) {
					t.Fatalf("latest=%t: projection missing or changed the message count", mode.latest)
				}
				var cleared []string
				bytes := 0
				for i := range messages {
					want := messages[i]
					if reflect.DeepEqual(got[i], want) {
						continue
					}
					bytes += len(want.Content)
					want.Content = ""
					if got[i].Role != "assistant" || len(got[i].ToolCalls) != 1 || !reflect.DeepEqual(got[i], want) {
						t.Fatalf("latest=%t: message %d changed beyond its free text", mode.latest, i)
					}
					cleared = append(cleared, got[i].ToolCalls[0].ID)
				}
				if !reflect.DeepEqual(cleared, mode.want) || removed != bytes {
					t.Fatalf("latest=%t: cleared %v (%d bytes reported, %d observed), want %v", mode.latest, cleared, removed, bytes, mode.want)
				}
			}
		})
	}
}

type hmc376Ladder struct {
	policy   types.FinalizerRequestBytePolicy
	accepted bool
	unbound  bool
	custom   bool
	prefer   bool
	surface  []string
}

// project is exercised with exact byte totals: a draft teaching of 400 bytes,
// a lane-matched teaching of 100, and three attempts of 200 bytes of free text.
func (l hmc376Ladder) run(t *testing.T, allocation int) (before, after []llm.Message, size int) {
	t.Helper()
	teaching := &finalizerRepairTeaching{draftSystem: strings.Repeat("d", 400), patchSystem: strings.Repeat("p", 100)}
	if l.custom {
		teaching.patchSystem = ""
	}
	mutable := types.NewMutableState("question stays byte-exact")
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mutable}
	policy := l.policy
	if policy == "" {
		policy = types.FinalizerRequestByteEnforce
	}
	e := &answerDocumentEvaluator{requestBytePolicy: policy, promptBudget: &finalizerPromptBudget{RequestAllocationBytes: allocation}, mu: mutable, preferPatchNext: l.prefer}
	if l.unbound {
		e.mu = types.NewMutableState("another dispatch")
	}
	messages := []llm.Message{{Role: "system", Content: teaching.draftSystem}, {Role: "user", Content: "request"}, {Role: "user", Content: "evidence"}}
	for _, id := range []string{"one", "two", "three"} {
		start := len(messages)
		messages = append(messages, hmc376Exchange(id, hmc376Patch, strings.Repeat(id[:1], 200))...)
		teaching.history.observe(mutable, messages, start, start+2, hmc376Rejected(hmc376Patch))
	}
	if l.accepted {
		mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "accepted", Kind: types.BlockSummary, Text: "accepted answer"}}})
	}
	surface := l.surface
	if surface == nil {
		surface = []string{hmc376Patch}
	}
	var tools []llm.ToolSchema
	for _, name := range surface {
		tools = append(tools, llm.ToolSchema{Name: name, Parameters: json.RawMessage(`{"type":"object"}`)})
	}
	snapshot, _ := json.Marshal(messages)
	projected := teaching.project(ctx, e, messages, tools)
	if now, _ := json.Marshal(messages); string(now) != string(snapshot) {
		t.Fatal("projection changed the durable history")
	}
	return messages, projected, llm.EstimateRequestBytes(projected, tools)
}

func hmc376FreeTextKept(messages []llm.Message) (kept []string) {
	for _, message := range messages {
		if message.Role == "assistant" && message.Content != "" {
			kept = append(kept, message.ToolCalls[0].ID)
		}
	}
	return kept
}

// The rungs are ordered and each one runs only while the request is still
// over the allocation; every gate is a typed state or a byte total.
func TestHMC376RepairLadderOrderAndStructuralControls(t *testing.T) {
	// over is how many bytes the unprojected request exceeds its allocation by.
	for _, c := range []struct {
		name      string
		ladder    hmc376Ladder
		over      int
		system    int
		kept      []string
		untouched bool
	}{
		{name: "fits", ladder: hmc376Ladder{prefer: true}, over: 0, system: 400, kept: []string{"one", "two", "three"}, untouched: true},
		{name: "lane-matched teaching alone", ladder: hmc376Ladder{prefer: true}, over: 300, system: 100, kept: []string{"one", "two", "three"}},
		{name: "then superseded free text", ladder: hmc376Ladder{prefer: true}, over: 301, system: 100, kept: []string{"three"}},
		{name: "superseded free text is enough", ladder: hmc376Ladder{prefer: true}, over: 700, system: 100, kept: []string{"three"}},
		{name: "then the latest free text", ladder: hmc376Ladder{prefer: true}, over: 701, system: 100, kept: nil},
		{name: "still over after every rung", ladder: hmc376Ladder{prefer: true}, over: 901, system: 100, kept: nil},
		{name: "lane without replaceable teaching", ladder: hmc376Ladder{custom: true, prefer: true}, over: 1, system: 400, kept: []string{"three"}},
		{name: "lane without replaceable teaching, still over", ladder: hmc376Ladder{custom: true, prefer: true}, over: 601, system: 400, kept: nil},
		{name: "patch lane not selected by the evaluator", ladder: hmc376Ladder{}, over: 1, system: 400, kept: []string{"three"}},
		{name: "complete rewrite surface", ladder: hmc376Ladder{prefer: true, surface: []string{hmc376Full}}, over: 1, system: 400, kept: []string{"three"}},
		{name: "two answer tools on the surface", ladder: hmc376Ladder{prefer: true, surface: []string{hmc376Full, hmc376Patch}}, over: 401, system: 400, kept: nil},
		{name: "accepted answer", ladder: hmc376Ladder{prefer: true, accepted: true}, over: 901, system: 400, kept: []string{"one", "two", "three"}, untouched: true},
		{name: "advisory byte policy", ladder: hmc376Ladder{prefer: true, policy: types.FinalizerRequestByteLegacyAdvisory}, over: 901, system: 400, kept: []string{"one", "two", "three"}, untouched: true},
		{name: "evaluator bound to another dispatch", ladder: hmc376Ladder{prefer: true, unbound: true}, over: 901, system: 400, kept: []string{"one", "two", "three"}, untouched: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			roomy, _, raw := c.ladder.run(t, 1<<30)
			if len(hmc376FreeTextKept(roomy)) != 3 {
				t.Fatal("fixture")
			}
			before, after, _ := c.ladder.run(t, raw-c.over)
			if c.untouched != (len(after) > 0 && &after[0] == &before[0]) {
				t.Fatalf("request identity: untouched=%t", !c.untouched)
			}
			if len(after) != len(before) || len(after[0].Content) != c.system || !reflect.DeepEqual(hmc376FreeTextKept(after), c.kept) {
				t.Fatalf("system=%d kept=%v", len(after[0].Content), hmc376FreeTextKept(after))
			}
			for i := 1; i < len(before); i++ {
				want := before[i]
				want.Content = after[i].Content
				if !reflect.DeepEqual(after[i], want) || (before[i].Role != "assistant" && after[i].Content != before[i].Content) {
					t.Fatalf("message %d lost a submitted call, a validator result or a correction", i)
				}
			}
		})
	}
}

// The single-catalog rule and its attempt counter read typed dispatch state.
func TestHMC376AnswerPatchOperandAndAttemptCountControls(t *testing.T) {
	mutable := types.NewMutableState("question")
	ctx := &types.AgentContext{Stage: types.StageFinalize, Mutable: mutable}
	previous, _ := json.Marshal(hmc376Draft())
	for _, c := range []struct {
		name   string
		setup  func(e *answerDocumentEvaluator, state *types.MutableState, ctx *types.AgentContext)
		hidden bool
	}{
		{name: "new dispatch", hidden: true},
		{name: "retained draft of an earlier dispatch", setup: func(_ *answerDocumentEvaluator, state *types.MutableState, _ *types.AgentContext) {
			state.SetLastRejectedAnswerDocumentV2(hmc376Draft())
			state.SetPendingAnswerDocumentPatchBase(hmc376Draft())
		}, hidden: true},
		{name: "own attempt observed", setup: func(e *answerDocumentEvaluator, _ *types.MutableState, _ *types.AgentContext) {
			e.dispatchAnswerAttempts = 1
		}},
		{name: "accepted document", setup: func(_ *answerDocumentEvaluator, state *types.MutableState, _ *types.AgentContext) {
			state.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, hmc376Draft())
		}},
		{name: "accepted-answer advisory round", setup: func(e *answerDocumentEvaluator, _ *types.MutableState, _ *types.AgentContext) {
			e.postEmitAdvisoryDelivered = true
		}},
		{name: "retry lane renders the previous emit", setup: func(_ *answerDocumentEvaluator, state *types.MutableState, _ *types.AgentContext) {
			state.SetRetryState(&types.RetryState{Attempt: 1, PrevEmitJSON: previous})
		}},
		{name: "retry state without a previous emit", setup: func(_ *answerDocumentEvaluator, state *types.MutableState, _ *types.AgentContext) {
			state.SetRetryState(&types.RetryState{Attempt: 1, ActiveViolations: []types.ScoredViolation{{}}})
		}, hidden: true},
		{name: "retry state that is not rendered", setup: func(_ *answerDocumentEvaluator, state *types.MutableState, _ *types.AgentContext) {
			state.SetRetryState(&types.RetryState{PrevEmitJSON: previous})
		}, hidden: true},
		{name: "evaluator bound to another dispatch", setup: func(e *answerDocumentEvaluator, _ *types.MutableState, _ *types.AgentContext) {
			e.mu = types.NewMutableState("another dispatch")
		}},
		{name: "evaluator not bound yet", setup: func(e *answerDocumentEvaluator, _ *types.MutableState, _ *types.AgentContext) { e.mu = nil }},
		{name: "another stage", setup: func(_ *answerDocumentEvaluator, _ *types.MutableState, ctx *types.AgentContext) {
			ctx.Stage = types.StageExtract
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			state := types.NewMutableState("question")
			local := &types.AgentContext{Stage: types.StageFinalize, Mutable: state}
			e := &answerDocumentEvaluator{mu: state}
			if c.setup != nil {
				c.setup(e, state, local)
			}
			if got := e.answerPatchOperandHidden(local); got != c.hidden {
				t.Fatalf("hidden=%t", got)
			}
		})
	}
	var absent *answerDocumentEvaluator
	if absent.answerPatchOperandHidden(ctx) || (&answerDocumentEvaluator{mu: mutable}).answerPatchOperandHidden(nil) {
		t.Fatal("missing evaluator or context must not hide a tool")
	}

	rejected := func(name string) types.ToolResult {
		return types.ToolResult{ToolName: name, Repair: &types.ToolRepair{}}
	}
	offered := map[string]bool{hmc376Full: true}
	last := rejected(hmc376Patch)
	for _, c := range []struct {
		name string
		obs  LoopObservation
		want int
	}{
		{"complete draft", LoopObservation{CurrentToolResults: []types.ToolResult{rejected(hmc376Full)}, ToolSurfaceKnown: true, AvailableToolNames: offered}, 1},
		{"tool the request did not offer", LoopObservation{CurrentToolResults: []types.ToolResult{rejected(hmc376Patch)}, ToolSurfaceKnown: true, AvailableToolNames: offered}, 0},
		{"another tool", LoopObservation{CurrentToolResults: []types.ToolResult{rejected("read_file")}}, 0},
		{"two answer results in one batch", LoopObservation{CurrentToolResults: []types.ToolResult{rejected(hmc376Full), rejected(hmc376Patch)}}, 2},
		{"only the last result reported", LoopObservation{LastToolResult: &last}, 1},
		{"nothing observed", LoopObservation{}, 0},
	} {
		if got := observedAnswerAttempts(c.obs); got != c.want {
			t.Errorf("%s: counted %d", c.name, got)
		}
	}
	e := &answerDocumentEvaluator{mu: mutable, dispatchAnswerAttempts: 3}
	e.Observe(ctx, LoopObservation{Phase: PhaseSoftStop, CurrentToolResults: []types.ToolResult{rejected(hmc376Full)}})
	if e.dispatchAnswerAttempts != 3 {
		t.Fatal("a round without a tool batch was counted as an attempt")
	}
	e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, CurrentToolResults: []types.ToolResult{rejected(hmc376Patch)}})
	if e.dispatchAnswerAttempts != 4 {
		t.Fatal("an observed answer result was not counted")
	}
	e.BuildInitialInstruction(ctx, nil)
	if e.dispatchAnswerAttempts != 0 {
		t.Fatal("the dispatch reset kept the previous dispatch's attempts")
	}
}
