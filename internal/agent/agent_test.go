package agent

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestStructuredToolDetailTraceQueryShowsFlavor(t *testing.T) {
	got := structuredToolDetail("trace_query", json.RawMessage(`{
		"view":"wakeup_chain",
		"path":"record.systrace",
		"trace_flavor":"harmony_hitrace",
		"thread":"com.tencent.mm-36379",
		"span_name":"Choreographer#doFrame",
		"interaction_direction":"incoming",
		"recipe_name":"jank",
		"time_start":2942.124416,
		"time_end":2942.260210,
		"max_depth":6
	}`))
	for _, want := range []string{"view=wakeup_chain", "path=record.systrace", "trace_flavor=harmony_hitrace", "thread=com.tencent.mm-36379", "span=Choreographer#doFrame", "direction=incoming", "recipe=jank", "window=2942.124416..2942.26021"} {
		if !strings.Contains(got, want) {
			t.Fatalf("trace_query detail missing %q in %q", want, got)
		}
	}

	got = structuredToolDetail("trace_query", json.RawMessage(`{
		"view":"window_stats",
		"path":"record.systrace",
		"platform":"android_atrace",
		"pid":36379
	}`))
	if !strings.Contains(got, "platform=android_atrace") || !strings.Contains(got, "pid=36379") {
		t.Fatalf("trace_query detail should surface platform alias and pid: %q", got)
	}

	got = structuredToolDetail("trace_query", json.RawMessage(`{
		"view":"recipe",
		"path":"record.systrace",
		"traceFlavor":"harmony_hitrace",
		"thread":"com.tencent.mm-36379",
		"spanName":"Choreographer#doFrame",
		"interactionDirection":"incoming",
		"recipeName":"jank",
		"pid":"36379",
		"timeStart":"2942.124416s",
		"timeEnd":"2942.260210s",
		"maxDepth":6
	}`))
	for _, want := range []string{"view=recipe", "trace_flavor=harmony_hitrace", "span=Choreographer#doFrame", "direction=incoming", "recipe=jank", "pid=36379", "window=2942.124416s..2942.260210s", "max_depth=6"} {
		if !strings.Contains(got, want) {
			t.Fatalf("trace_query detail should surface camelCase alias %q in %q", want, got)
		}
	}
}

// TestPruneToolHistoryKeepsRecentAndStubsOlder locks the ReAct
// history-pruning contract that protects long explorer runs from
// blowing the model's context window. The 2026-04-12 incident: 15
// explorer iterations × multiple read_file calls per iter accumulated
// ~450 KB of tool output in the `messages` slice, and the next LLM
// call 400'd with context_length_exceeded. Fix: stub every tool
// message older than the newest N that fit in types.DefaultAgentSettings().MaxToolHistoryBytes,
// preserving ToolCallID so OpenAI's tool-call pairing stays valid.
func TestPruneToolHistoryKeepsRecentAndStubsOlder(t *testing.T) {
	// Build a conversation with 20 tool results at 20 KB each = 400 KB,
	// well over the 150 KB budget. Assistant messages are tiny and
	// interleaved so the pruner has to walk past them.
	const (
		numTools     = 20
		perToolBytes = 20 * 1024
	)
	payload := strings.Repeat("X", perToolBytes)

	var messages []llm.Message
	messages = append(messages, llm.Message{Role: "system", Content: "system prompt"})
	messages = append(messages, llm.Message{Role: "user", Content: "initial request"})
	for i := 0; i < numTools; i++ {
		messages = append(messages, llm.Message{
			Role:    "assistant",
			Content: "thinking step",
			ToolCalls: []llm.ToolCall{
				{ID: toolID(i), Name: "read_file"},
			},
		})
		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    payload,
			ToolCallID: toolID(i),
		})
	}

	pruned := pruneToolHistory(messages, types.DefaultAgentSettings().MaxToolHistoryBytes)
	if !pruned {
		t.Fatalf("pruneToolHistory returned false, expected pruning to occur (400 KB > 150 KB)")
	}

	// Sum surviving verbatim "tool" role bytes. Must stay under budget.
	liveBytes := 0
	stubbed := 0
	intactToolIDs := map[string]bool{}
	for _, m := range messages {
		if m.Role != "tool" {
			continue
		}
		if strings.HasPrefix(m.Content, "[earlier tool result elided") {
			stubbed++
			// Even stubbed messages must keep their ToolCallID so the
			// assistant tool_call above still has a matching response.
			if m.ToolCallID == "" {
				t.Errorf("stubbed tool message lost ToolCallID")
			}
			continue
		}
		liveBytes += len(m.Content)
		intactToolIDs[m.ToolCallID] = true
	}

	if liveBytes > types.DefaultAgentSettings().MaxToolHistoryBytes {
		t.Errorf("surviving tool bytes %d exceed budget %d", liveBytes, types.DefaultAgentSettings().MaxToolHistoryBytes)
	}
	if stubbed == 0 {
		t.Errorf("expected at least one stubbed message")
	}
	// The most recent tool result must be intact (it's the one the LLM
	// is about to reason over).
	lastID := toolID(numTools - 1)
	if !intactToolIDs[lastID] {
		t.Errorf("most recent tool message %s was stubbed — should be preserved", lastID)
	}
	// ToolCall → tool response pairing: every assistant tool_call ID
	// must still appear as a ToolCallID on some "tool" message.
	pairedIDs := map[string]bool{}
	for _, m := range messages {
		if m.Role == "tool" && m.ToolCallID != "" {
			pairedIDs[m.ToolCallID] = true
		}
	}
	for i := 0; i < numTools; i++ {
		if !pairedIDs[toolID(i)] {
			t.Errorf("tool_call ID %s lost its response after pruning — breaks OpenAI API pairing", toolID(i))
		}
	}
}

func TestToolDetailForCallStructuredEmitSummaries(t *testing.T) {
	cases := []struct {
		name string
		call llm.ToolCall
		want string
	}{
		{
			name: "evidence item count and first subject",
			call: llm.ToolCall{
				Name:   "emit_evidence",
				Params: json.RawMessage(`{"items":[{"subject":"SubExplorer"},{"subject":"BaseAgent"}]}`),
			},
			want: "items=2 SubExplorer",
		},
		{
			name: "analysis compact classification",
			call: llm.ToolCall{
				Name:   "emit_analysis",
				Params: json.RawMessage(`{"intent":"mechanism","question_kind":"code_architecture","entities":["Explorer"],"keywords":["agent"]}`),
			},
			want: "intent=mechanism question_kind=code_architecture entities=1 keywords=1",
		},
		{
			name: "answer document block counts",
			call: llm.ToolCall{
				Name:   "emit_answer_document",
				Params: json.RawMessage(`{"blocks":[{"id":"s"}],"citations":[{"file":"a.go","line":1}]}`),
			},
			want: "blocks=1 citations=1",
		},
		{
			name: "repo map overview shows default view and path",
			call: llm.ToolCall{
				Name:   "repo_map",
				Params: json.RawMessage(`{"path":"."}`),
			},
			want: "view=overview path=.",
		},
		{
			name: "repo map relation call shows compact navigation fields",
			call: llm.ToolCall{
				Name:   "repo_map",
				Params: json.RawMessage(`{"path":"kernel/bpf","view":"relation_map","query":"map update elem dispatch","sources":["kernel/bpf/syscall.c","kernel/bpf/hashtab.c"],"relation_kinds":["call","reference"],"top_n":20}`),
			},
			want: "view=relation_map path=kernel/bpf query=map update elem dispatch sources=2:kernel/bpf/syscall.c relation_kinds=2:call top_n=20",
		},
		{
			name: "trace query shows compact runtime navigation fields",
			call: llm.ToolCall{
				Name:   "trace_query",
				Params: json.RawMessage(`{"path":"trace.systrace","view":"wakeup_chain","pid":36379,"time_start":2942.124416,"time_end":2942.260210,"max_depth":6,"max_branches":8}`),
			},
			want: "view=wakeup_chain path=trace.systrace pid=36379 window=2942.124416..2942.260210 max_depth=6 max_branches=8",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolDetailForCall(tc.call); got != tc.want {
				t.Fatalf("toolDetailForCall() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCurrentTurnToolSurfaceDirectiveOnlyNamesCallableTools(t *testing.T) {
	base := []llm.ToolSchema{
		{Name: "read_file"},
		{Name: "grep"},
		{Name: "emit_evidence"},
		{Name: "emit_investigation_complete"},
	}
	effective := []llm.ToolSchema{
		{Name: "emit_evidence"},
		{Name: "emit_investigation_complete"},
	}

	got := currentTurnToolSurfaceDirective(base, effective)
	if got == "" {
		t.Fatal("expected directive for narrowed tool surface")
	}
	for _, want := range []string{"`emit_evidence`", "`emit_investigation_complete`", "current tool schema is authoritative"} {
		if !strings.Contains(got, want) {
			t.Fatalf("directive missing %q: %s", want, got)
		}
	}
	for _, unavailable := range []string{"`read_file`", "`grep`"} {
		if strings.Contains(got, unavailable) {
			t.Fatalf("directive must not restate unavailable tool %s: %s", unavailable, got)
		}
	}
}

func TestCurrentTurnToolSurfaceDirectiveSkipsUnchangedSurface(t *testing.T) {
	base := []llm.ToolSchema{{Name: "emit_evidence"}, {Name: "emit_investigation_complete"}}
	if got := currentTurnToolSurfaceDirective(base, base); got != "" {
		t.Fatalf("unchanged tool surface should not inject directive, got %q", got)
	}
}

// TestContextPressureDirective_AgentSpecific locks the per-agent
// terminal-tool mapping. Each agent's AllowedSet MUST name ONLY
// tools it actually has access to — suggesting "emit_change_plan"
// to a verifier drives the LLM into tool-not-available dead-ends
// and wastes the last iteration of a pressure-bounded dispatch.
// The directive is rendered via internal/analysis/hint.Composer so
// the format matches every other retry hint the orchestrator emits
// (contract-check / CGEC / DAG window retries).
func TestContextPressureDirective_AgentSpecific(t *testing.T) {
	cases := []struct {
		agent       types.AgentName
		mustInclude []string
		mustExclude []string
	}{
		{
			agent:       types.AgentAnalyzer,
			mustInclude: []string{"emit_analysis"},
			mustExclude: []string{"emit_change_plan", "emit_answer_document", "run_tests"},
		},
		{
			agent:       types.AgentExplorer,
			mustInclude: []string{"emit_investigation_complete"},
			mustExclude: []string{"emit_change_plan", "run_tests", "emit_analysis"},
		},
		{
			agent:       types.AgentExtractor,
			mustInclude: []string{"emit_answer_symbol", "emit_hypothesis_verdict"},
			mustExclude: []string{"emit_change_plan", "run_tests", "emit_analysis"},
		},
		{
			agent:       types.AgentFinalizer,
			mustInclude: []string{"emit_answer_document"},
			mustExclude: []string{"emit_change_plan", "run_tests", "emit_investigation_complete"},
		},
		{
			agent:       types.AgentLogTriager,
			mustInclude: []string{"emit_log_triage"},
			mustExclude: []string{"emit_change_plan", "run_tests"},
		},
		{
			agent:       types.AgentPlanner,
			mustInclude: []string{"emit_change_plan"},
			mustExclude: []string{"run_tests", "emit_investigation_complete", "emit_answer_symbol"},
		},
		{
			agent:       types.AgentCoder,
			mustInclude: []string{"apply_patch"},
			mustExclude: []string{"emit_change_plan", "run_tests", "emit_investigation_complete"},
		},
		{
			agent:       types.AgentVerifier,
			mustInclude: []string{"run_tests"},
			mustExclude: []string{"emit_change_plan", "apply_patch", "emit_answer_symbol"},
		},
	}
	const (
		testPromptBytes = 700_000
		testByteBudget  = 800_000
		testHardRatio   = 0.9
	)
	for _, c := range cases {
		t.Run(string(c.agent), func(t *testing.T) {
			got := contextPressureDirective(c.agent, testPromptBytes, testByteBudget, testHardRatio)
			if got == "" {
				t.Fatal("directive empty")
			}
			// Hint Composer sections:
			for _, w := range []string{
				"**What failed**", "**Why it failed**",
				"**What I already did**", "**How to fix now**",
				"**Allowed**", "**Do NOT**",
			} {
				if !strings.Contains(got, w) {
					t.Errorf("%s directive missing hint section %q; got:\n%s", c.agent, w, got)
				}
			}
			// Numeric threshold embedded in WhyItFailed.
			if !strings.Contains(got, "700000 of 800000") && !strings.Contains(got, "700,000") {
				// fmt prints %d without separators; loose check for 700000.
				if !strings.Contains(got, "700000") {
					t.Errorf("%s directive missing numeric prompt-byte context; got:\n%s", c.agent, got)
				}
			}
			for _, w := range c.mustInclude {
				if !strings.Contains(got, w) {
					t.Errorf("%s directive missing expected tool %q; got:\n%s", c.agent, w, got)
				}
			}
			for _, w := range c.mustExclude {
				if strings.Contains(got, w) {
					t.Errorf("%s directive mentions sibling-stage tool %q (cross-stage leak); got:\n%s", c.agent, w, got)
				}
			}
		})
	}
}

// TestContextPressureDirective_UnknownAgentFallback covers the
// fallthrough path — an experimental / unregistered agent name still
// receives a sensible force-stop message rather than an empty string
// (which would disable the hard ratio for that agent silently).
func TestContextPressureDirective_UnknownAgentFallback(t *testing.T) {
	got := contextPressureDirective(types.AgentName("experimental_custom"), 1000, 2000, 0.5)
	if got == "" {
		t.Fatal("fallback directive empty")
	}
	if !strings.Contains(got, "**What failed**") {
		t.Errorf("fallback missing hint-format section; got:\n%s", got)
	}
	if !strings.Contains(got, "terminal tool call") {
		t.Errorf("fallback should prompt for generic terminal tool call; got:\n%s", got)
	}
}

func TestRequiredNoToolAnswerDraftCompactedInHistoryOutsideFinalizer(t *testing.T) {
	content := strings.Join([]string{
		"## Draft Answer",
		"",
		strings.Repeat("This is an answer paragraph. ", 80),
		"",
		"```mermaid",
		"sequenceDiagram",
		"    A->>B: call",
		"```",
	}, "\n")
	resp := llm.Response{Content: content}
	ctx := &types.AgentContext{Stage: types.StageExplore}

	if !shouldCompactNoToolAnswerDraftHistory(ctx, resp) {
		t.Fatal("explore-stage answer-like prose without tool calls should be compacted before history replay")
	}
	history := contentForNoToolHistory(ctx, resp)
	if history == content || strings.Contains(history, "sequenceDiagram") {
		t.Fatalf("history should keep a compact protocol placeholder, got:\n%s", history)
	}
	if !strings.Contains(history, "protocol-only text omitted") {
		t.Fatalf("history placeholder should explain protocol omission, got:\n%s", history)
	}
}

func TestRequiredNoToolAnswerDraftPreservedForFinalizer(t *testing.T) {
	content := "## Draft Answer\n\n" + strings.Repeat("finalizer prose ", 100)
	resp := llm.Response{Content: content}
	ctx := &types.AgentContext{Stage: types.StageFinalize}

	if shouldCompactNoToolAnswerDraftHistory(ctx, resp) {
		t.Fatal("finalizer retries need the prior prose draft for emit_answer_document correction")
	}
	if got := contentForNoToolHistory(ctx, resp); got != content {
		t.Fatalf("finalizer draft should be preserved, got:\n%s", got)
	}
}

func TestFinalizerNoToolHistoryPreservesOnlyFirstRichDraft(t *testing.T) {
	content := "## Draft Answer\n\n" + strings.Repeat("finalizer prose ", 100)
	resp := llm.Response{Content: content}
	ctx := &types.AgentContext{Stage: types.StageFinalize}

	first, preserved := contentForNoToolHistoryWithFinalizerDraftBudget(ctx, resp, 0)
	if first != content || preserved != 1 {
		t.Fatalf("first finalizer draft should be preserved, preserved=%d content=%q", preserved, first)
	}
	second, preserved := contentForNoToolHistoryWithFinalizerDraftBudget(ctx, resp, preserved)
	if second == content || !strings.Contains(second, "protocol-only text omitted") || preserved != 1 {
		t.Fatalf("repeated finalizer draft should be compacted without increment, preserved=%d content=%q", preserved, second)
	}
}

func TestRecordFinalizerNoToolAnswerDraftCapturesRecoverablePayloads(t *testing.T) {
	ctx := &types.AgentContext{
		Stage:   types.StageFinalize,
		Mutable: types.NewMutableState(""),
	}
	if recordFinalizerNoToolAnswerDraft(ctx, "plain finalizer prose") {
		t.Fatal("plain prose must not be recorded as a recoverable finalizer draft")
	}
	if got := ctx.Mutable.FinalizerNoToolAnswerDrafts(); len(got) != 0 {
		t.Fatalf("unexpected draft capture for prose: %+v", got)
	}
	payload := `{"blocks":[{"id":"summary","kind":"summary","text":"visible"}]}`
	if !recordFinalizerNoToolAnswerDraft(ctx, payload) {
		t.Fatal("answer_document-shaped payload should be recorded")
	}
	if recordFinalizerNoToolAnswerDraft(ctx, payload) {
		t.Fatal("duplicate payload should be deduplicated")
	}
	drafts := ctx.Mutable.FinalizerNoToolAnswerDrafts()
	if len(drafts) != 1 || drafts[0].Content != payload {
		t.Fatalf("captured drafts = %+v, want one exact payload", drafts)
	}
	surface := "| 项 | 值 |\n| --- | --- |\n| timeout | stream |\n| validation | content |"
	if !recordFinalizerNoToolAnswerDraft(ctx, surface) {
		t.Fatal("model-authored table/diagram/list surface draft should be recorded")
	}
	drafts = ctx.Mutable.FinalizerNoToolAnswerDrafts()
	if len(drafts) != 2 || drafts[1].Content != surface {
		t.Fatalf("surface draft was not captured exactly: %+v", drafts)
	}
	nonFinalizer := &types.AgentContext{Stage: types.StageExplore, Mutable: types.NewMutableState("")}
	if recordFinalizerNoToolAnswerDraft(nonFinalizer, payload) {
		t.Fatal("non-finalizer stages must not capture finalizer drafts")
	}
}

// TestContextPressureForbiddenPatterns_PerAgentExtension pins the
// shared-core + per-agent-extension shape of the Do-NOT list. The
// shared core applies to every agent; the extensions call out
// stage-specific temptations (extractor reaching for `complete`,
// coder re-reading files, etc.).
func TestContextPressureForbiddenPatterns_PerAgentExtension(t *testing.T) {
	// Shared core must appear for every agent.
	for _, a := range []types.AgentName{
		types.AgentAnalyzer, types.AgentExplorer, types.AgentExtractor,
		types.AgentFinalizer, types.AgentLogTriager, types.AgentPerfTriager,
		types.AgentPlanner, types.AgentCoder, types.AgentVerifier,
	} {
		p := contextPressureForbiddenPatterns(a)
		if len(p) < 2 {
			t.Errorf("%s forbidden patterns: got %d, want ≥ 2 (shared core)", a, len(p))
		}
		joined := strings.Join(p, "|")
		if !strings.Contains(joined, "investigative") {
			t.Errorf("%s forbidden patterns missing shared-core 'investigative'; got %v", a, p)
		}
	}

	// Per-agent extensions: specific phrases must appear only for
	// their target agent.
	extCases := []struct {
		agent types.AgentName
		want  string
	}{
		{types.AgentExtractor, "complete"},
		{types.AgentCoder, "re-read"},
		{types.AgentExplorer, "emit_evidence"},
		{types.AgentPlanner, "multi-kind"},
	}
	for _, c := range extCases {
		joined := strings.Join(contextPressureForbiddenPatterns(c.agent), "|")
		if !strings.Contains(joined, c.want) {
			t.Errorf("%s forbidden patterns missing stage-specific %q; got %v",
				c.agent, c.want, contextPressureForbiddenPatterns(c.agent))
		}
	}
}

// TestEstimateMessagesBytes_CountsRoleContentToolCallsAndParams pins
// the contract the BaseAgent context-pressure watchdog relies on:
// the estimate covers every byte that will flow on the wire. Drift
// here silently makes the watchdog under-report pressure (the real
// wire payload grows beyond what the estimator sees → context_length_
// exceeded at the adapter).
func TestEstimateMessagesBytes_CountsRoleContentToolCallsAndParams(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys-body"},
		{Role: "user", Content: "user-body", ToolCallID: ""},
		{Role: "assistant", Content: "assistant-body", ToolCalls: []llm.ToolCall{
			{ID: "call-1", Name: "grep", Params: json.RawMessage(`{"pattern":"x"}`)},
			{ID: "call-2", Name: "read_file", Params: json.RawMessage(`{"path":"a.go"}`)},
		}},
		{Role: "tool", Content: "tool-body", ToolCallID: "call-1"},
	}
	want := 0
	// Manual roll-up mirrors the function so "what counts" stays
	// explicit — the test is a ledger, not a re-implementation.
	want += len("system") + len("sys-body")
	want += len("user") + len("user-body")
	want += len("assistant") + len("assistant-body")
	want += len("call-1") + len("grep") + len(`{"pattern":"x"}`)
	want += len("call-2") + len("read_file") + len(`{"path":"a.go"}`)
	want += len("tool") + len("tool-body") + len("call-1")

	if got := llm.EstimateMessagesBytes(msgs); got != want {
		t.Errorf("EstimateMessagesBytes = %d, want %d", got, want)
	}
}

// TestEstimateMessagesBytes_EmptySliceIsZero is the degenerate guard:
// a never-dispatched agent's messages slice starts empty, the
// watchdog must see zero (not a nil-pointer panic or a negative
// value that would trip the ratio check).
func TestEstimateMessagesBytes_EmptySliceIsZero(t *testing.T) {
	if got := llm.EstimateMessagesBytes(nil); got != 0 {
		t.Errorf("nil slice: got %d, want 0", got)
	}
	if got := llm.EstimateMessagesBytes([]llm.Message{}); got != 0 {
		t.Errorf("empty slice: got %d, want 0", got)
	}
}

// TestPruneToolHistoryIdempotent verifies that running the pruner
// twice doesn't keep shrinking already-stubbed placeholders. The loop
// calls it every iteration, so a non-idempotent implementation would
// keep prepending "[earlier tool result elided" wrappers.
func TestPruneToolHistoryIdempotent(t *testing.T) {
	payload := strings.Repeat("Y", 20*1024)
	var messages []llm.Message
	for i := 0; i < 15; i++ {
		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    payload,
			ToolCallID: toolID(i),
		})
	}
	_ = pruneToolHistory(messages, types.DefaultAgentSettings().MaxToolHistoryBytes)
	snapshot := make([]string, len(messages))
	for i, m := range messages {
		snapshot[i] = m.Content
	}
	_ = pruneToolHistory(messages, types.DefaultAgentSettings().MaxToolHistoryBytes)
	for i, m := range messages {
		if m.Content != snapshot[i] {
			t.Errorf("message %d content changed on second prune: %q → %q", i, snapshot[i], m.Content)
		}
	}
}

// TestPruneToolHistoryUnderBudgetNoop verifies the common fast path:
// when the conversation is still under budget, nothing is touched.
func TestPruneToolHistoryUnderBudgetNoop(t *testing.T) {
	messages := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "question"},
		{Role: "assistant", Content: "plan", ToolCalls: []llm.ToolCall{{ID: "a", Name: "grep"}}},
		{Role: "tool", Content: strings.Repeat("Z", 10*1024), ToolCallID: "a"},
	}
	if pruneToolHistory(messages, types.DefaultAgentSettings().MaxToolHistoryBytes) {
		t.Errorf("pruneToolHistory modified messages under budget")
	}
	if len(messages[3].Content) != 10*1024 {
		t.Errorf("tool content mutated while under budget")
	}
}

func TestBuildToolHistoryPruneCheckpointCarriesAcceptedStructuredState(t *testing.T) {
	mut := types.NewMutableState("question")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "A",
			Predicate:       "calls",
			Object:          "B",
			Source:          "a.go",
			LineStart:       12,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "A calls B from the current source line",
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "stale",
			Source:          "stale.go",
			LineStart:       99,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingRecovered,
			Summary:         "should stay out of the accepted checkpoint",
		},
	})
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "agents",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"explorer"},
	}})
	mut.SetInvestigationResultKind("resolved")
	mut.SetInvestigationComplete("accepted closure")
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mut}

	got := buildToolHistoryPruneCheckpoint(ctx)
	if !strings.HasPrefix(got, toolHistoryPruneCheckpointPrefix) {
		t.Fatalf("checkpoint prefix missing:\n%s", got)
	}
	for _, want := range []string{
		"Accepted citable evidence: 1",
		"direct A calls B @ a.go:12",
		"A calls B from the current source line",
		"recovered/ungrounded/non-citable rows intentionally omitted",
		"agents",
		"explorer",
		"result_kind: `resolved`",
		"accepted closure",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "should stay out of the accepted checkpoint") {
		t.Fatalf("non-citable recovered evidence leaked into checkpoint:\n%s", got)
	}
}

func TestBuildToolHistoryPruneCheckpointEmptyWithoutAcceptedState(t *testing.T) {
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: types.NewMutableState("question")}
	if got := buildToolHistoryPruneCheckpoint(ctx); got != "" {
		t.Fatalf("empty mutable state should not emit a checkpoint, got:\n%s", got)
	}
}

func TestBuildToolHistoryPruneCheckpointCarriesDispatchCommandMeasurement(t *testing.T) {
	mut := types.NewMutableState("count files")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Success:  true,
		Summary: "[exec_command: $ find internal/tool -name '*.go' | wc -l]\n" +
			"[exec_command: evidence_origin=command_measurement measurement=count]\n" +
			"140\n",
		RawRef: "/tmp/codrax/blob/exec_command-measurement.txt",
	})
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mut}

	got := buildToolHistoryPruneCheckpoint(ctx)
	if !strings.HasPrefix(got, toolHistoryPruneCheckpointPrefix) {
		t.Fatalf("checkpoint prefix missing:\n%s", got)
	}
	for _, want := range []string{
		"Typed observation ledger snapshot",
		"origin=`command_measurement`",
		"find internal/tool -name '*.go' | wc -l",
		"/tmp/codrax/blob/exec_command-measurement.txt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint missing %q:\n%s", want, got)
		}
	}
}

func TestBuildToolHistoryPruneCheckpointCarriesRepairDebt(t *testing.T) {
	mut := types.NewMutableState("repair debt before prune")
	closure := mut.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{
		File:       "internal/agent/explorer.go",
		Origin:     "pre_complete.multi_path_anchor",
		Rationale:  "exact line still needs local verification",
		LineRanges: []types.LineRange{{Start: 42, End: 42}},
	})
	closure.AddRepair(types.RepairDirective{
		Kind:     types.RepairExpandSearch,
		Keywords: []string{"optional"},
		Origin:   "support.telemetry",
		Advisory: true,
	})
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mut}

	got := buildToolHistoryPruneCheckpoint(ctx)
	if !strings.HasPrefix(got, toolHistoryPruneCheckpointPrefix) {
		t.Fatalf("checkpoint prefix missing:\n%s", got)
	}
	for _, want := range []string{
		"Active repair debt snapshot",
		"principal_blocking=0 surgical_grounding=1 advisory=1",
		"class=`surgical_grounding` pending_read kind=`read_file`",
		"internal/agent/explorer.go",
		"line:42",
		"class=`advisory` repair kind=`expand_search`",
		"not new evidence",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint missing %q:\n%s", want, got)
		}
	}
}

func TestSanitizeToolCallsForHistory_ReplacesInvalidParamsOnlyInHistory(t *testing.T) {
	calls := []llm.ToolCall{
		{ID: "call-good", Name: "read_file", Params: json.RawMessage(`{"path":"a.go"}`)},
		{ID: "call-bad", Name: "emit_evidence", Params: json.RawMessage(`{"items":`)},
		{ID: "call-empty", Name: "repo_map", Params: json.RawMessage(`   `)},
	}

	sanitized := sanitizeToolCallsForHistory(calls)

	if got := string(sanitized[0].Params); got != `{"path":"a.go"}` {
		t.Fatalf("valid params mutated: %q", got)
	}
	if got := string(sanitized[1].Params); got != `{}` {
		t.Fatalf("invalid params should be replaced with {}, got %q", got)
	}
	if got := string(sanitized[2].Params); got != `{}` {
		t.Fatalf("empty params should be replaced with {}, got %q", got)
	}
	if got := string(calls[1].Params); got != `{"items":` {
		t.Fatalf("original tool call params must stay unchanged for execution-side repair/reporting, got %q", got)
	}
	if sanitized[1].ID != "call-bad" || sanitized[1].Name != "emit_evidence" {
		t.Fatalf("tool-call identity must be preserved for pairing, got %+v", sanitized[1])
	}
}

func toolID(i int) string {
	return "call-" + string(rune('a'+i%26)) + string(rune('0'+i/10))
}

// TestValidateAnalyzerPrescanToolCall pins the evidence-lite
// boundary enforcement: in the analyze stage, grep MUST be called
// with files_only=true. The validator is a pre-execution gate that
// synthesizes a failed ToolResult instead of dispatching to the
// real grep tool; the LLM sees the failure as a normal tool-error
// message and can retry within the same dispatch.
func TestValidateAnalyzerPrescanToolCall(t *testing.T) {
	t.Run("analyze stage rejects grep without files_only", func(t *testing.T) {
		ctx := &types.AgentContext{Stage: types.StageAnalyze}
		tc := llm.ToolCall{
			Name:   "grep",
			Params: json.RawMessage(`{"pattern":"analyzer"}`),
		}
		result := validateAnalyzerPrescanToolCall(ctx, tc)
		if result == nil {
			t.Fatal("expected violation for grep without files_only, got nil")
		}
		if result.Success {
			t.Errorf("violation result should have Success=false")
		}
		if !strings.Contains(result.Summary, "files_only=true") {
			t.Errorf("violation summary should mention files_only=true, got %q", result.Summary)
		}
	})

	t.Run("analyze stage accepts grep with files_only", func(t *testing.T) {
		ctx := &types.AgentContext{Stage: types.StageAnalyze}
		tc := llm.ToolCall{
			Name:   "grep",
			Params: json.RawMessage(`{"pattern":"analyzer","files_only":true}`),
		}
		if got := validateAnalyzerPrescanToolCall(ctx, tc); got != nil {
			t.Errorf("files_only=true should pass, got violation: %+v", got)
		}
	})

	t.Run("non-analyze stage has no files_only constraint", func(t *testing.T) {
		// The explorer is the full-power consumer of grep and routinely
		// calls it without files_only to get line-level matches.
		for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
			ctx := &types.AgentContext{Stage: stage}
			tc := llm.ToolCall{
				Name:   "grep",
				Params: json.RawMessage(`{"pattern":"analyzer"}`),
			}
			if got := validateAnalyzerPrescanToolCall(ctx, tc); got != nil {
				t.Errorf("stage=%s: grep without files_only should be allowed, got violation", stage)
			}
		}
	})

	t.Run("analyze stage ignores non-grep tools", func(t *testing.T) {
		ctx := &types.AgentContext{Stage: types.StageAnalyze}
		for _, name := range []string{"repo_map", "list_files", "emit_analysis"} {
			tc := llm.ToolCall{Name: name, Params: json.RawMessage(`{}`)}
			if got := validateAnalyzerPrescanToolCall(ctx, tc); got != nil {
				t.Errorf("tool=%s should be unaffected, got violation", name)
			}
		}
	})

	t.Run("analyze stage rejects source inventory row expansion", func(t *testing.T) {
		ctx := &types.AgentContext{Stage: types.StageAnalyze}
		tc := llm.ToolCall{
			Name:   "repo_map",
			Params: json.RawMessage(`{"path":"internal/analysis","view":"source_inventory","roles":["function"],"scopes":["aggregator"]}`),
		}
		got := validateAnalyzerPrescanToolCall(ctx, tc)
		if got == nil {
			t.Fatal("expected source_inventory repo_map to be rejected during analyze")
		}
		if got.Success {
			t.Fatalf("source_inventory boundary result should fail, got %+v", got)
		}
		if got.Repair == nil || got.Repair.Code != analyzerSourceInventoryAnalyzeBoundaryCode {
			t.Fatalf("repair code = %+v, want %q", got.Repair, analyzerSourceInventoryAnalyzeBoundaryCode)
		}
		for _, want := range []string{"not available in this classification step", "source_inventory_profile", `view="overview"`, `view="task_map"`, `view="file_map"`, `view="relation_map"`, "relation_kinds"} {
			if !strings.Contains(got.Summary, want) {
				t.Fatalf("summary should give positive source_inventory guidance; missing %q in %q", want, got.Summary)
			}
		}
		for _, forbidden := range []string{"belongs to explore", "not analyze", "Analyze is"} {
			if strings.Contains(got.Summary, forbidden) {
				t.Fatalf("summary should not leak internal stage wording %q: %q", forbidden, got.Summary)
			}
		}
	})

	t.Run("terminal emit retry blocks all prescan tools", func(t *testing.T) {
		ctx := &types.AgentContext{
			Stage:                 types.StageAnalyze,
			EmitStageRetryAttempt: 1,
			Mutable:               types.NewMutableState(""),
		}
		for _, name := range []string{"repo_map", "grep", "list_files"} {
			tc := llm.ToolCall{Name: name, Params: json.RawMessage(`{"files_only":true}`)}
			got := validateAnalyzerPrescanToolCall(ctx, tc)
			if got == nil {
				t.Fatalf("tool=%s should be blocked during terminal emit retry", name)
			}
			if !strings.Contains(got.Summary, "Call emit_analysis now") {
				t.Fatalf("tool=%s summary should redirect to emit_analysis, got %q", name, got.Summary)
			}
		}
	})

	t.Run("must-emit wall blocks further prescan tools", func(t *testing.T) {
		mu := types.NewMutableState("")
		mu.SetPrescanRoundLimit(2)
		mu.AppendPrescanSummary("round 1")
		mu.AppendPrescanSummary("round 2")
		ctx := &types.AgentContext{
			Stage:   types.StageAnalyze,
			Mutable: mu,
		}
		for _, name := range []string{"repo_map", "grep", "list_files"} {
			tc := llm.ToolCall{Name: name, Params: json.RawMessage(`{"files_only":true}`)}
			got := validateAnalyzerPrescanToolCall(ctx, tc)
			if got == nil {
				t.Fatalf("tool=%s should be blocked after prescan budget is reached", name)
			}
			if !strings.Contains(got.Summary, "pre-scan budget already reached") {
				t.Fatalf("tool=%s summary should name must-emit wall, got %q", name, got.Summary)
			}
		}
	})

	t.Run("malformed params fall through to the tool", func(t *testing.T) {
		// Defensive: a tool call with unparseable params is NOT
		// rejected by the pre-check so the real grep tool produces
		// its canonical error message. This keeps the LLM's error
		// experience consistent.
		ctx := &types.AgentContext{Stage: types.StageAnalyze}
		tc := llm.ToolCall{
			Name:   "grep",
			Params: json.RawMessage(`{not json`),
		}
		if got := validateAnalyzerPrescanToolCall(ctx, tc); got != nil {
			t.Errorf("malformed params should fall through, got violation: %+v", got)
		}
	})
}

func TestAnalyzerSameBatchPrescanBoundary(t *testing.T) {
	t.Run("analyze prescan batches execute sequentially", func(t *testing.T) {
		ctx := &types.AgentContext{Stage: types.StageAnalyze}
		calls := []llm.ToolCall{
			{Name: "list_files", Params: json.RawMessage(`{"path":"internal/analysis"}`)},
			{Name: "list_files", Params: json.RawMessage(`{"path":"internal/analysis/gate"}`)},
		}
		if canExecuteToolBatchInParallel(ctx, calls) {
			t.Fatal("analyze-stage prescan batch must not run in parallel")
		}
		if !canExecuteToolBatchInParallel(&types.AgentContext{Stage: types.StageExplore}, calls) {
			t.Fatal("non-analyze read-only batches should keep parallel execution")
		}
	})

	t.Run("strong directory listing closes same batch", func(t *testing.T) {
		mu := types.NewMutableState("mixed source inventory")
		mu.SetPrescanRoundLimit(4)
		ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
		result := &types.ToolResult{
			ToolName: "list_files",
			Success:  true,
			Summary: strings.Join([]string{
				"[list_files: path=internal/analysis recursive=false]",
				"internal/analysis/aggregator",
				"internal/analysis/amplifier",
				"internal/analysis/binder",
				"internal/analysis/gate",
			}, "\n"),
		}

		applyAnalyzerSameBatchPrescanBoundary(ctx, result)
		if !mu.PrescanReady() {
			t.Fatal("strong same-batch list_files result should close analyzer prescan")
		}
		got := validateAnalyzerToolBoundary(ctx, llm.ToolCall{
			Name:   "list_files",
			Params: json.RawMessage(`{"path":"internal/analysis/gate"}`),
		})
		if got == nil || got.Success {
			t.Fatalf("subsequent same-batch prescan call should be rejected, got %+v", got)
		}
		if got.Repair == nil || got.Repair.Code != analyzerPrescanTerminalEmitModeCode {
			t.Fatalf("repair code = %+v, want %q", got.Repair, analyzerPrescanTerminalEmitModeCode)
		}
		if rejected := validateAnalyzerToolBoundary(ctx, llm.ToolCall{Name: "emit_analysis", Params: json.RawMessage(`{}`)}); rejected != nil {
			t.Fatalf("emit_analysis must remain available after same-batch close, got %+v", rejected)
		}
	})

	t.Run("default budget closes after scoped grep", func(t *testing.T) {
		mu := types.NewMutableState("scoped import lookup")
		mu.SetPrescanRoundLimit(2)
		ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
		result := &types.ToolResult{
			ToolName: "grep",
			Success:  true,
			Summary:  "[grep: 1 matching files]\n[grep params: pattern=import path=internal/tool files_only=true]\ninternal/tool/emit_analysis.go\n",
		}

		applyAnalyzerSameBatchPrescanBoundary(ctx, result)
		if !mu.PrescanReady() {
			t.Fatal("default-budget scoped grep should close same-batch analyzer prescan")
		}
	})
}

func TestValidateObservationOnlyRuntimeToolCall(t *testing.T) {
	analyzeCtx := &types.AgentContext{
		Stage: types.StageAnalyze,
		LogTriage: &types.LogBundle{
			Observations: []types.LogObservation{{
				Kind:       types.LogObservationRuntimeEvent,
				Subject:    "WARN 日志行号",
				Summary:    "WARN 出现在附件日志第 3 行",
				Diagnostic: true,
				Confidence: 1,
			}},
		},
	}
	for _, name := range []string{"grep", "repo_map", "list_files"} {
		t.Run("analyze allows "+name, func(t *testing.T) {
			got := validateObservationOnlyRuntimeToolCall(analyzeCtx, llm.ToolCall{Name: name, Params: json.RawMessage(`{"files_only":true}`)})
			if got != nil {
				t.Fatalf("analyzer pre-scan tool %q must not be hard-blocked for mixed runtime+current-source classification, got %+v", name, got)
			}
		})
	}
	if got := validateObservationOnlyRuntimeToolCall(analyzeCtx, llm.ToolCall{Name: "emit_analysis", Params: json.RawMessage(`{}`)}); got != nil {
		t.Fatalf("emit_analysis must remain available, got %+v", got)
	}

	observationOnlyCtx := &types.AgentContext{
		Stage: types.StageExplore,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "RuntimeError"}},
				},
			},
		},
	}
	for _, name := range []string{"grep", "read_file", "repo_map", "list_files", "exec_command", "emit_evidence", "propose_sub_agents"} {
		t.Run("blocks "+name, func(t *testing.T) {
			got := validateObservationOnlyRuntimeToolCall(observationOnlyCtx, llm.ToolCall{Name: name, Params: json.RawMessage(`{}`)})
			if got == nil {
				t.Fatalf("tool %q should be blocked for observation-only runtime artifact", name)
			}
			if got.Success {
				t.Fatalf("tool %q block must be a failed ToolResult", name)
			}
			if got.Repair == nil || got.Repair.Code != "observation_only_runtime_repo_tool" {
				t.Fatalf("tool %q repair metadata missing: %+v", name, got.Repair)
			}
		})
	}

	if got := validateObservationOnlyRuntimeToolCall(observationOnlyCtx, llm.ToolCall{Name: "emit_investigation_complete", Params: json.RawMessage(`{}`)}); got != nil {
		t.Fatalf("emit_investigation_complete must remain available, got %+v", got)
	}

	currentStatusCtx := &types.AgentContext{
		Stage: types.StageExplore,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					ExactTargets: []string{"RuntimeError"},
				},
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "RuntimeError"}},
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					CurrentRisk: true,
				},
			},
		},
	}
	if got := validateObservationOnlyRuntimeToolCall(currentStatusCtx, llm.ToolCall{Name: "grep", Params: json.RawMessage(`{}`)}); got != nil {
		t.Fatalf("current-status diagnostics must keep repo tools available, got %+v", got)
	}
}

func TestBuildToolSchemas_ObservationOnlyRuntimeHidesRepoTools(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&tool.GrepTool{})
	registry.Register(&tool.ReadFile{})
	registry.Register(&tool.ListFiles{})
	registry.Register(&tool.ExecCommand{})
	registry.Register(&tool.EmitEvidence{})
	registry.Register(&tool.EmitInvestigationComplete{})

	agent := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: registry}, &stubEvaluator{})
	ctx := &types.AgentContext{
		Stage: types.StageExplore,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "RuntimeError"}},
				},
			},
		},
	}
	schemas := agent.buildToolSchemas(&skill.Config{
		ToolSuggestions: []string{"grep", "read_file", "list_files", "exec_command", "emit_evidence", "emit_investigation_complete"},
	}, ctx)
	got := map[string]bool{}
	for _, s := range schemas {
		got[s.Name] = true
	}
	for _, blocked := range []string{"grep", "read_file", "list_files", "exec_command", "emit_evidence"} {
		if got[blocked] {
			t.Fatalf("observation-only runtime schema exposed repo tool %q: %+v", blocked, schemas)
		}
	}
	if !got["emit_investigation_complete"] {
		t.Fatalf("emit_investigation_complete must stay exposed for artifact-only closure: %+v", schemas)
	}
}

func TestBuildToolSchemas_ObservationOnlyRuntimeKeepsAnalyzerPrescanTools(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&tool.GrepTool{})
	registry.Register(&tool.ListFiles{})
	registry.Register(&tool.EmitAnalysis{})

	agent := NewBaseAgent(types.AgentAnalyzer, &Dependencies{Tools: registry}, &stubEvaluator{})
	ctx := &types.AgentContext{
		Stage: types.StageAnalyze,
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{Type: "RuntimeError"}},
		},
	}
	schemas := agent.buildToolSchemas(&skill.Config{
		ToolSuggestions: []string{"grep", "list_files", "emit_analysis"},
	}, ctx)
	got := map[string]bool{}
	for _, s := range schemas {
		got[s.Name] = true
	}
	for _, allowed := range []string{"grep", "list_files"} {
		if !got[allowed] {
			t.Fatalf("runtime analyzer schema should keep pre-scan tool %q available for mixed artifact+current-source requests: %+v", allowed, schemas)
		}
	}
	if !got["emit_analysis"] {
		t.Fatalf("emit_analysis must stay exposed for direct classification: %+v", schemas)
	}
}

// TestIsReadFilePathMiss covers the Summary-substring classifier that
// decides whether a failed read_file call should refund its budget
// slot. Path-resolution failures refund (LLM self-corrects on the
// next iter); other failure modes (size, permission) stay charged
// because they aren't trivially retryable by a different path.
func TestIsReadFilePathMiss(t *testing.T) {
	cases := []struct {
		name    string
		toolRaw string
		success bool
		summary string
		want    bool
	}{
		{"ENOENT lowercase", "read_file", false, "read failed: open internal/foo.go: no such file or directory", true},
		{"ENOENT mixed case", "read_file", false, "read failed: OPEN x: No Such File Or Directory", true},
		{"is a directory", "read_file", false, "read failed: open dir: is a directory", true},
		{"explicit does not exist", "read_file", false, "file does not exist: internal/foo.go", true},
		{"alias read", "read", false, "read failed: no such file", true},
		{"success is never a miss", "read_file", true, "some success summary", false},
		{"permission denied stays charged", "read_file", false, "read failed: permission denied", false},
		{"IO error stays charged", "read_file", false, "read failed: input/output error", false},
		{"empty summary stays charged", "read_file", false, "", false},
		{"other tool name", "grep", false, "read failed: no such file or directory", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &types.ToolResult{
				ToolName: c.toolRaw,
				Success:  c.success,
				Summary:  c.summary,
			}
			got := isReadFilePathMiss(c.toolRaw, r)
			if got != c.want {
				t.Errorf("isReadFilePathMiss(%q, %+v) = %v, want %v", c.toolRaw, *r, got, c.want)
			}
		})
	}
	// Nil result must not panic.
	if isReadFilePathMiss("read_file", nil) {
		t.Errorf("nil result must return false, not true")
	}
}

// stubEvaluator is a trivial Evaluator that records ParseOutput
// invocations and can be configured to panic. Kept local to this
// test so salvagePartialDispatch's contract (run ParseOutput for
// side-effects, recover panics) is verifiable without a full
// BaseAgent.Execute integration.
type stubEvaluator struct {
	parseCalls   int
	panicMessage string
	messages     []llm.Message
	output       *StageOutput
}

func (s *stubEvaluator) BuildInitialInstruction(_ *types.AgentContext, _ *skill.Config) string {
	return ""
}
func (s *stubEvaluator) ShouldStop(_ llm.Response, _ int) bool { return false }
func (s *stubEvaluator) ParseOutput(
	_ *types.AgentContext, messages []llm.Message, _ []types.ToolResult, _ []types.MCPResponse,
) (*StageOutput, error) {
	s.parseCalls++
	s.messages = append([]llm.Message(nil), messages...)
	if s.panicMessage != "" {
		panic(s.panicMessage)
	}
	if s.output != nil {
		return s.output, nil
	}
	return &StageOutput{}, nil
}
func (s *stubEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}

func TestSalvagePartialDispatch_RunsParseOutput(t *testing.T) {
	eval := &stubEvaluator{}
	b := &BaseAgent{name: types.AgentExplorer, deps: &Dependencies{}, eval: eval}
	b.salvagePartialDispatch(nil, nil, nil, nil, 3, errFake("upstream 429"))
	if eval.parseCalls != 1 {
		t.Errorf("ParseOutput should have been invoked once, got %d", eval.parseCalls)
	}
}

func TestSalvagePartialDispatch_ReturnsParsedOutput(t *testing.T) {
	want := &StageOutput{
		StageReport: "partial stage report",
		EvidenceItems: []types.EvidenceItem{{
			Kind:      types.EvidenceMechanism,
			Subject:   "buildAnalysisIR",
			Source:    "internal/agent/analyzer.go",
			LineStart: 1505,
		}},
	}
	eval := &stubEvaluator{output: want}
	b := &BaseAgent{name: types.AgentExplorer, deps: &Dependencies{}, eval: eval}
	got := b.salvagePartialDispatch(nil, nil, nil, nil, 3, errFake("upstream 429"))
	if got == nil {
		t.Fatal("salvage should return the parsed StageOutput")
	}
	if got.StageReport != want.StageReport || len(got.EvidenceItems) != 1 {
		t.Fatalf("salvage output lost parsed fields: %+v", got)
	}
}

func TestSalvagePartialDispatch_RecoversPanic(t *testing.T) {
	eval := &stubEvaluator{panicMessage: "partial-data panic"}
	b := &BaseAgent{name: types.AgentExplorer, deps: &Dependencies{}, eval: eval}
	// Must not propagate the panic; the caller still returns the
	// original LLM error through the normal path.
	b.salvagePartialDispatch(nil, nil, nil, nil, 3, errFake("upstream 429"))
	if eval.parseCalls != 1 {
		t.Errorf("ParseOutput should have been invoked once even when it panics, got %d", eval.parseCalls)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

type mixedContentToolLLM struct{}

func (mixedContentToolLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	return llm.Response{
		Content: "Let me record the evidence now.",
		ToolCalls: []llm.ToolCall{{
			ID:     "call-trace",
			Name:   "trace_tool",
			Params: json.RawMessage(`{"path":"internal/example.go"}`),
		}},
	}, nil
}

func (mixedContentToolLLM) ModelID() string               { return "mixed-content-tool" }
func (mixedContentToolLLM) MaxContextTokens() int         { return 128000 }
func (mixedContentToolLLM) MaxOutputTokens() int          { return 4096 }
func (mixedContentToolLLM) RequestTimeout() time.Duration { return 0 }
func (mixedContentToolLLM) RetryMaxAttempts() int         { return 0 }

type traceTool struct {
	tool.ReadOnly
	tool.NonEvidenceTool
}

func (traceTool) Name() string        { return "trace_tool" }
func (traceTool) Description() string { return "records that it was called" }
func (traceTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
}
func (traceTool) Execute(_ *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	return types.ToolResult{
		ToolName:  "trace_tool",
		Summary:   "trace tool executed",
		Success:   true,
		Timestamp: time.Now(),
	}, nil
}

func TestBaseAgent_EmitsToolBatchBeforeExecutingMixedContentToolResponse(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(traceTool{})

	var events []render.Event
	b := NewBaseAgent(types.AgentExplorer, &Dependencies{
		LLM:           mixedContentToolLLM{},
		Tools:         registry,
		MaxIterations: 1,
		Emit: func(ev render.Event) {
			events = append(events, ev)
		},
	}, &stubEvaluator{})

	out, err := b.Execute(&types.AgentContext{
		Stage:   types.StageExplore,
		Mutable: types.NewMutableState(""),
	}, &skill.Config{ToolSuggestions: []string{"trace_tool"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil || len(out.ToolResults) != 1 || out.ToolResults[0].ToolName != "trace_tool" {
		t.Fatalf("tool did not execute cleanly, output=%+v", out)
	}

	reasoningIdx, batchIdx, startIdx := -1, -1, -1
	for i, ev := range events {
		switch ev.Kind {
		case render.EventAgentReasoning:
			reasoningIdx = i
		case render.EventAgentToolCallBatch:
			batchIdx = i
			if ev.ToolName != "trace_tool" || ev.ToolCallCount != 1 {
				t.Fatalf("tool batch event lost call metadata: %+v", ev)
			}
		case render.EventToolCallStart:
			startIdx = i
		}
	}
	if reasoningIdx < 0 || batchIdx < 0 || startIdx < 0 {
		t.Fatalf("expected reasoning, tool batch, and tool start events, got %+v", events)
	}
	if !(reasoningIdx < batchIdx && batchIdx < startIdx) {
		t.Fatalf("tool batch must be visible after prose and before execution start; reasoning=%d batch=%d start=%d events=%+v",
			reasoningIdx, batchIdx, startIdx, events)
	}
}

type unavailableToolLLM struct{}

func (unavailableToolLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	return llm.Response{
		ToolCalls: []llm.ToolCall{{
			ID:     "call-unavailable",
			Name:   "registered_but_hidden",
			Params: json.RawMessage(`{}`),
		}},
	}, nil
}

func (unavailableToolLLM) ModelID() string               { return "unavailable-tool" }
func (unavailableToolLLM) MaxContextTokens() int         { return 128000 }
func (unavailableToolLLM) MaxOutputTokens() int          { return 4096 }
func (unavailableToolLLM) RequestTimeout() time.Duration { return 0 }
func (unavailableToolLLM) RetryMaxAttempts() int         { return 0 }

type registeredButHiddenTool struct {
	tool.ReadOnly
	tool.NonEvidenceTool
	called *bool
}

func (t registeredButHiddenTool) Name() string { return "registered_but_hidden" }
func (t registeredButHiddenTool) Description() string {
	return "must not execute unless exposed in current schema"
}
func (t registeredButHiddenTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t registeredButHiddenTool) Execute(_ *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	if t.called != nil {
		*t.called = true
	}
	return types.ToolResult{ToolName: t.Name(), Summary: "executed", Success: true, Timestamp: time.Now()}, nil
}

func TestBaseAgent_DoesNotExecuteRegisteredToolOutsideCurrentSchema(t *testing.T) {
	registry := tool.NewRegistry()
	called := false
	registry.Register(registeredButHiddenTool{called: &called})

	var events []render.Event
	b := NewBaseAgent(types.AgentFinalizer, &Dependencies{
		LLM:           unavailableToolLLM{},
		Tools:         registry,
		MaxIterations: 1,
		Emit: func(ev render.Event) {
			events = append(events, ev)
		},
	}, &stubEvaluator{})

	out, err := b.Execute(&types.AgentContext{
		Stage:   types.StageFinalize,
		Mutable: types.NewMutableState(""),
	}, &skill.Config{ToolSuggestions: []string{"emit_answer_document"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if called {
		t.Fatal("registered tool outside current schema was executed")
	}
	if out == nil || len(out.ToolResults) != 1 || out.ToolResults[0].Success {
		t.Fatalf("unavailable tool should produce one failed ToolResult, output=%+v", out)
	}
	sawUnavailableBatch := false
	for _, ev := range events {
		if ev.Kind == render.EventAgentToolCallBatch {
			sawUnavailableBatch = ev.ToolUnavailable &&
				len(ev.UnavailableToolNames) == 1 &&
				ev.UnavailableToolNames[0] == "registered_but_hidden"
		}
	}
	if !sawUnavailableBatch {
		t.Fatalf("tool batch event did not mark unavailable tool attempt: %+v", events)
	}
}

type transientPartialLLM struct{}

func (transientPartialLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	if opts.OnToolCallDelta != nil {
		opts.OnToolCallDelta(0, "emit_answer_document", `{"summary":"partial structured summary from interrupted stream`)
	}
	return llm.Response{}, io.ErrUnexpectedEOF
}

func (transientPartialLLM) ModelID() string               { return "transient-partial" }
func (transientPartialLLM) MaxContextTokens() int         { return 128000 }
func (transientPartialLLM) MaxOutputTokens() int          { return 4096 }
func (transientPartialLLM) RequestTimeout() time.Duration { return 0 }
func (transientPartialLLM) RetryMaxAttempts() int         { return 0 }

type protocolSoftStopLLM struct {
	calls int
}

func (l *protocolSoftStopLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{Content: "## Premature answer\n\n" + strings.Repeat("This stage is trying to answer in prose. ", 40)}, nil
	}
	return llm.Response{}, nil
}

func (*protocolSoftStopLLM) ModelID() string               { return "protocol-softstop" }
func (*protocolSoftStopLLM) MaxContextTokens() int         { return 128000 }
func (*protocolSoftStopLLM) MaxOutputTokens() int          { return 4096 }
func (*protocolSoftStopLLM) RequestTimeout() time.Duration { return 0 }
func (*protocolSoftStopLLM) RetryMaxAttempts() int         { return 0 }

type richExtractorSoftStopLLM struct {
	content string
	calls   int
}

func (l *richExtractorSoftStopLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	return llm.Response{Content: l.content}, nil
}

func (*richExtractorSoftStopLLM) ModelID() string               { return "rich-extractor-softstop" }
func (*richExtractorSoftStopLLM) MaxContextTokens() int         { return 128000 }
func (*richExtractorSoftStopLLM) MaxOutputTokens() int          { return 4096 }
func (*richExtractorSoftStopLLM) RequestTimeout() time.Duration { return 0 }
func (*richExtractorSoftStopLLM) RetryMaxAttempts() int         { return 0 }

type protocolEmptyFirstLLM struct {
	calls int
}

func (l *protocolEmptyFirstLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{}, nil
	}
	return llm.Response{Content: "structured draft after empty response"}, nil
}

func (*protocolEmptyFirstLLM) ModelID() string               { return "protocol-empty-first" }
func (*protocolEmptyFirstLLM) MaxContextTokens() int         { return 128000 }
func (*protocolEmptyFirstLLM) MaxOutputTokens() int          { return 4096 }
func (*protocolEmptyFirstLLM) RequestTimeout() time.Duration { return 0 }
func (*protocolEmptyFirstLLM) RetryMaxAttempts() int         { return 0 }

type isolatedPromptLLM struct {
	calls       int
	toolCounts  []int
	toolChoices []string
	messages    [][]llm.Message
}

func (l *isolatedPromptLLM) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	l.calls++
	l.toolCounts = append(l.toolCounts, len(tools))
	l.toolChoices = append(l.toolChoices, opts.ToolChoice)
	l.messages = append(l.messages, append([]llm.Message(nil), messages...))
	if l.calls == 1 {
		return llm.Response{}, nil
	}
	return llm.Response{Content: "isolated prose fallback answer"}, nil
}

func (*isolatedPromptLLM) ModelID() string               { return "isolated-prompt" }
func (*isolatedPromptLLM) MaxContextTokens() int         { return 128000 }
func (*isolatedPromptLLM) MaxOutputTokens() int          { return 4096 }
func (*isolatedPromptLLM) RequestTimeout() time.Duration { return 0 }
func (*isolatedPromptLLM) RetryMaxAttempts() int         { return 0 }

type protocolSoftStopEvaluator struct {
	observeCalls   int
	shouldStopSeen int
}

func (e *protocolSoftStopEvaluator) BuildInitialInstruction(_ *types.AgentContext, _ *skill.Config) string {
	return ""
}

func (e *protocolSoftStopEvaluator) ShouldStop(_ llm.Response, _ int) bool {
	e.shouldStopSeen++
	return true
}

func (e *protocolSoftStopEvaluator) ParseOutput(_ *types.AgentContext, _ []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	return &StageOutput{}, nil
}

func (e *protocolSoftStopEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}

func (e *protocolSoftStopEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseSoftStop {
		return LoopSignal{}
	}
	e.observeCalls++
	return LoopSignal{
		HintRequested: true,
		HintKey:       "test.protocol_softstop",
		Hint:          "Use the stage's structured tool call instead of prose.",
	}
}

type isolatedPromptEvaluator struct{}

func (e *isolatedPromptEvaluator) BuildInitialInstruction(_ *types.AgentContext, _ *skill.Config) string {
	return ""
}

func (e *isolatedPromptEvaluator) ShouldStop(_ llm.Response, _ int) bool { return false }

func (e *isolatedPromptEvaluator) ParseOutput(_ *types.AgentContext, _ []llm.Message, _ []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	return &StageOutput{}, nil
}

func (e *isolatedPromptEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingNone
}

func (e *isolatedPromptEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseSoftStop {
		return LoopSignal{}
	}
	if strings.TrimSpace(obs.Response.Content) == "" {
		return LoopSignal{
			HintRequested:        true,
			HintKey:              "test.isolated",
			Hint:                 "isolated fallback prompt",
			DisableToolsNextTurn: true,
			IsolateNextPrompt:    true,
			BypassBudget:         true,
			BypassThrottle:       true,
		}
	}
	return LoopSignal{StopRequested: true, StopReason: "accepted isolated prose"}
}

func TestProtocolStagesRouteNoToolProseThroughLoopControllerBeforeShouldStop(t *testing.T) {
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract} {
		t.Run(string(stage), func(t *testing.T) {
			llmStub := &protocolSoftStopLLM{}
			eval := &protocolSoftStopEvaluator{}
			b := NewBaseAgent(types.AgentExplorer, &Dependencies{
				LLM:           llmStub,
				MaxIterations: 2,
			}, eval)
			out, err := b.Execute(&types.AgentContext{
				Stage:   stage,
				Mutable: types.NewMutableState(""),
			}, &skill.Config{})
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if out == nil {
				t.Fatal("Execute returned nil output")
			}
			wantObserveCalls := 1
			if stage == types.StageExtract {
				wantObserveCalls = 2
			}
			if eval.observeCalls != wantObserveCalls {
				t.Fatalf("no-tool prose in %s must reach soft-stop controller before ShouldStop, observeCalls=%d want=%d", stage, eval.observeCalls, wantObserveCalls)
			}
			if llmStub.calls != 2 {
				t.Fatalf("controller hint should trigger one continuation, calls=%d", llmStub.calls)
			}
		})
	}
}

func TestRequiredToolStageRoutesEmptyNoToolThroughLoopController(t *testing.T) {
	llmStub := &protocolEmptyFirstLLM{}
	eval := &protocolSoftStopEvaluator{}
	var events []render.Event
	b := NewBaseAgent(types.AgentFinalizer, &Dependencies{
		LLM:           llmStub,
		MaxIterations: 2,
		Emit: func(ev render.Event) {
			events = append(events, ev)
		},
	}, eval)
	out, err := b.Execute(&types.AgentContext{
		Stage:   types.StageFinalize,
		Mutable: types.NewMutableState(""),
	}, &skill.Config{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil {
		t.Fatal("Execute returned nil output")
	}
	if eval.observeCalls != 1 {
		t.Fatalf("empty required-tool response must reach soft-stop controller, observeCalls=%d", eval.observeCalls)
	}
	if llmStub.calls != 2 {
		t.Fatalf("controller hint should trigger one continuation after empty response, calls=%d", llmStub.calls)
	}
	if countNoticeKind(events, render.NoticeNoToolCall) != 1 {
		t.Fatalf("empty required-tool repair should surface one retry notice, events=%+v", events)
	}
}

func TestLoopSignalCanIsolateNextPromptAndDisableTools(t *testing.T) {
	llmStub := &isolatedPromptLLM{}
	b := NewBaseAgent(types.AgentFinalizer, &Dependencies{
		LLM:           llmStub,
		MaxIterations: 3,
	}, &isolatedPromptEvaluator{})
	out, err := b.Execute(&types.AgentContext{
		Stage:   types.StageFinalize,
		Mutable: types.NewMutableState(""),
	}, &skill.Config{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil {
		t.Fatal("Execute returned nil output")
	}
	if llmStub.calls != 2 {
		t.Fatalf("isolated fallback should make exactly two calls, got %d", llmStub.calls)
	}
	if len(llmStub.messages) != 2 || len(llmStub.messages[1]) != 1 ||
		llmStub.messages[1][0].Role != "user" || llmStub.messages[1][0].Content != "isolated fallback prompt" {
		t.Fatalf("second turn must use only the isolated fallback prompt, got %#v", llmStub.messages)
	}
	if llmStub.toolCounts[1] != 0 || llmStub.toolChoices[1] != "" {
		t.Fatalf("isolated fallback turn must disable tools and tool_choice, got tools=%d choice=%q",
			llmStub.toolCounts[1], llmStub.toolChoices[1])
	}
}

func TestLooksLikeEmbeddedToolCallRecognizesTaggedToolCalls(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{
			name: "tagged grep call",
			in:   `<tool_call>{"name":"grep","arguments":{"pattern":"Agent","files_only":true}}</tool_call>`,
			want: true,
		},
		{
			name: "tagged read file call with parameters alias",
			in:   `<tool_call>{"name":"read_file","parameters":{"path":"a.go"}}</tool_call>`,
			want: true,
		},
		{
			name: "existing emit json path",
			in:   "```json\n{\"name\":\"emit_evidence\",\"arguments\":{\"items\":[]}}\n```",
			want: true,
		},
		{
			name: "prose mention only",
			in:   "The model should call <tool_call> in some providers, but no concrete arguments are present.",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeEmbeddedToolCall(tc.in); got != tc.want {
				t.Fatalf("looksLikeEmbeddedToolCall()=%v want=%v for %q", got, tc.want, tc.in)
			}
		})
	}
}

type embeddedToolTextLLM struct {
	calls int
}

func (l *embeddedToolTextLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{
			Content: `<tool_call>{"name":"trace_tool","arguments":{"path":"internal/example.go"}}</tool_call>`,
		}, nil
	}
	return llm.Response{
		ToolCalls: []llm.ToolCall{{
			ID:     "call-trace",
			Name:   "trace_tool",
			Params: json.RawMessage(`{"path":"internal/example.go"}`),
		}},
	}, nil
}

func (*embeddedToolTextLLM) ModelID() string               { return "embedded-tool-text" }
func (*embeddedToolTextLLM) MaxContextTokens() int         { return 128000 }
func (*embeddedToolTextLLM) MaxOutputTokens() int          { return 4096 }
func (*embeddedToolTextLLM) RequestTimeout() time.Duration { return 0 }
func (*embeddedToolTextLLM) RetryMaxAttempts() int         { return 0 }

func TestBaseAgent_RendersRawEmbeddedToolCallTextWhenNotRecovered(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(traceTool{})

	var events []render.Event
	b := NewBaseAgent(types.AgentExplorer, &Dependencies{
		LLM:           &embeddedToolTextLLM{},
		Tools:         registry,
		MaxIterations: 2,
		Emit: func(ev render.Event) {
			events = append(events, ev)
		},
	}, &stubEvaluator{})

	out, err := b.Execute(&types.AgentContext{
		Stage:    types.StageExplore,
		Mutable:  types.NewMutableState(""),
		Language: "zh",
	}, &skill.Config{ToolSuggestions: []string{"trace_tool"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil || len(out.ToolResults) != 1 || out.ToolResults[0].ToolName != "trace_tool" {
		t.Fatalf("tool did not execute after protocol correction, output=%+v", out)
	}
	var sawRawToolCall bool
	for _, ev := range events {
		if ev.Kind != render.EventAgentReasoning {
			continue
		}
		if strings.Contains(ev.Reasoning, "<tool_call>") || strings.Contains(ev.Reasoning, `"trace_tool"`) {
			sawRawToolCall = true
		}
	}
	if !sawRawToolCall {
		t.Fatalf("unrecovered embedded tool-call text should stay visible for diagnostics, events=%+v", events)
	}
}

func TestToolResultSummarySurfacesAnswerDocumentRejectsOnly(t *testing.T) {
	answerReject := &types.ToolResult{
		ToolName: "emit_answer_document",
		Success:  false,
		Summary:  "Field: `citations[]`\nAction: preserve citations",
	}
	if got := toolResultSummary(answerReject); !strings.Contains(got, "citations[]") {
		t.Fatalf("answer-document rejection should remain visible to the renderer, got %q", got)
	}

	readReject := &types.ToolResult{
		ToolName: "read_file",
		Success:  false,
		Summary:  "path not found",
	}
	if got := toolResultSummary(readReject); got != "" {
		t.Fatalf("generic failed tools should stay quiet in the dock, got %q", got)
	}

	success := &types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "3 matches",
	}
	if got := toolResultSummary(success); got != "3 matches" {
		t.Fatalf("successful tool summary = %q, want %q", got, "3 matches")
	}
}

type protocolSoftStopAcceptEvaluator struct {
	protocolSoftStopEvaluator
}

func (e *protocolSoftStopAcceptEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase == PhaseSoftStop {
		e.observeCalls++
	}
	return LoopSignal{}
}

func TestProtocolStagesAcceptedNoToolSoftStopDoesNotShowRetryNotice(t *testing.T) {
	llmStub := &protocolSoftStopLLM{}
	eval := &protocolSoftStopAcceptEvaluator{}
	var events []render.Event
	b := NewBaseAgent(types.AgentExtractor, &Dependencies{
		LLM:           llmStub,
		MaxIterations: 2,
		Emit: func(ev render.Event) {
			events = append(events, ev)
		},
	}, eval)
	out, err := b.Execute(&types.AgentContext{
		Stage:   types.StageExtract,
		Mutable: types.NewMutableState(""),
	}, &skill.Config{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil {
		t.Fatal("Execute returned nil output")
	}
	if llmStub.calls != 1 {
		t.Fatalf("accepted soft-stop should not request continuation, calls=%d", llmStub.calls)
	}
	if eval.observeCalls != 1 {
		t.Fatalf("soft-stop controller should observe once, got %d", eval.observeCalls)
	}
	if countNoticeKind(events, render.NoticeNoToolCall) != 0 {
		t.Fatalf("accepted soft-stop must not claim retry, events=%+v", events)
	}
}

func TestExtractorAcceptedNoToolSoftStopPreservesVisibleSurfaceDraft(t *testing.T) {
	content := strings.Join([]string{
		"### Draft",
		"",
		"```mermaid",
		"flowchart TD",
		"    A[Analyze] --> B[Explore]",
		"```",
		"",
		"| stage | role |",
		"| --- | --- |",
		"| analyze | classify |",
	}, "\n")
	llmStub := &richExtractorSoftStopLLM{content: content}
	eval := &protocolSoftStopAcceptEvaluator{}
	b := NewBaseAgent(types.AgentExtractor, &Dependencies{
		LLM:           llmStub,
		MaxIterations: 2,
	}, eval)
	out, err := b.Execute(&types.AgentContext{
		Stage:   types.StageExtract,
		Mutable: types.NewMutableState(""),
	}, &skill.Config{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out == nil {
		t.Fatal("Execute returned nil output")
	}
	if llmStub.calls != 1 {
		t.Fatalf("accepted soft-stop should not request continuation, calls=%d", llmStub.calls)
	}
	if !strings.Contains(out.StageReport, "### Draft") ||
		!strings.Contains(out.StageReport, "```mermaid") ||
		!strings.Contains(out.StageReport, "| analyze | classify |") {
		t.Fatalf("extractor visible surface draft was not preserved as StageReport:\n%s", out.StageReport)
	}
}

func TestAcceptedNoToolSoftStopKeepsExploreProtocolCompaction(t *testing.T) {
	content := "```mermaid\nflowchart TD\nA --> B\n```"
	resp := llm.Response{Content: content}
	ctx := &types.AgentContext{Stage: types.StageExplore}
	fallback := "[protocol-only text omitted]"

	if got := contentForAcceptedNoToolSoftStopHistory(ctx, resp, fallback); got != fallback {
		t.Fatalf("explore no-tool answer draft should stay compacted, got:\n%s", got)
	}
}

func TestUnavailableToolResultListsExactCurrentSurface(t *testing.T) {
	got := unavailableToolResult(
		&types.AgentContext{Stage: types.StageExplore},
		llm.ToolCall{Name: "read_file"},
		map[string]bool{"emit_evidence": true, "emit_investigation_complete": true},
	)
	if got == nil || got.Success {
		t.Fatalf("unavailable tool result should fail, got %+v", got)
	}
	for _, want := range []string{"read_file", "available tools now: emit_evidence, emit_investigation_complete"} {
		if !strings.Contains(got.Summary, want) {
			t.Fatalf("summary missing %q: %q", want, got.Summary)
		}
	}
	for _, forbidden := range []string{"read/search tools", "git tools"} {
		if strings.Contains(got.Summary, forbidden) {
			t.Fatalf("summary must not use broad stage-level surface %q: %q", forbidden, got.Summary)
		}
	}
}

func countNoticeKind(events []render.Event, kind render.OrchestratorNoticeKind) int {
	count := 0
	for _, ev := range events {
		if ev.Kind == render.EventOrchestratorNotice && ev.NoticeKind == kind {
			count++
		}
	}
	return count
}

func TestBaseAgent_FinalizeTransientErrorDoesNotSynthesizePartialSummary(t *testing.T) {
	eval := &stubEvaluator{}
	b := NewBaseAgent(types.AgentFinalizer, &Dependencies{
		LLM:           transientPartialLLM{},
		MaxIterations: 1,
	}, eval)
	ctx := &types.AgentContext{
		Stage:   types.StageFinalize,
		Mutable: types.NewMutableState(""),
	}

	out, err := b.Execute(ctx, &skill.Config{})
	if err == nil {
		t.Fatal("Execute should return the transient stream error")
	}
	if out == nil || !strings.Contains(out.Error, "LLM call failed") {
		t.Fatalf("Execute should return structured failure output, got out=%+v err=%v", out, err)
	}
	if eval.parseCalls != 1 {
		t.Fatalf("transient failure should still salvage deterministic artifacts via ParseOutput, got %d calls", eval.parseCalls)
	}
	for _, msg := range eval.messages {
		if msg.Role == "assistant" && strings.Contains(msg.Content, "partial structured summary") {
			t.Fatalf("partial finalizer stream must stay UI-only, not become assistant fallback content: %+v", eval.messages)
		}
	}
}

// TestToolChoiceForStage pins the per-stage tool_choice mapping. The
// stages whose evaluator treats "no tool call this turn" as a retry
// trigger (analyze / extract / finalize / log_triage) must emit
// "required" so the protocol layer rejects the failure mode instead
// of burning the continuation retry budget on a chatty model.
// Explore stays "" (auto) because its ReAct loop legitimately
// intermixes reasoning turns with tool-calling turns.
func TestToolChoiceForStage(t *testing.T) {
	cases := []struct {
		stage types.PipelineStage
		want  string
	}{
		{types.StageAnalyze, "required"},
		{types.StageExtract, "required"},
		{types.StageFinalize, "required"},
		{types.StageLogTriage, "required"},
		{types.StagePerfTriage, "required"},
		{types.StageExplore, ""},
	}
	for _, c := range cases {
		if got := toolChoiceForStage(c.stage); got != c.want {
			t.Errorf("toolChoiceForStage(%q) = %q, want %q", c.stage, got, c.want)
		}
	}
}

// TestResolveToolChoice_TerminalForcing pins the retry-attempt
// escalation. On attempt 0 (happy path), every emit-required stage
// returns the bare "required" keyword. On attempt > 0, single-emit
// stages (analyze / finalize / log_triage / perf_triage) escalate to
// the named-function JSON object form so the model is constrained to
// the specific tool. Multi-emit stages (extract emits answer_symbol
// AND hypothesis_verdict) keep "required" because constraining to one
// tool would block the other half of the contract. Explore stays "".
func TestResolveToolChoice_TerminalForcing(t *testing.T) {
	cases := []struct {
		name    string
		stage   types.PipelineStage
		attempt int
		want    string
	}{
		// Happy path: bare "required" or "" as before.
		{"analyze_attempt0", types.StageAnalyze, 0, "required"},
		{"finalize_attempt0", types.StageFinalize, 0, "required"},
		{"extract_attempt0", types.StageExtract, 0, "required"},
		{"log_triage_attempt0", types.StageLogTriage, 0, "required"},
		{"perf_triage_attempt0", types.StagePerfTriage, 0, "required"},
		{"explore_attempt0", types.StageExplore, 0, ""},

		// Terminal forcing: named-function form for single-emit stages.
		{"analyze_attempt1", types.StageAnalyze, 1, `{"type":"function","function":{"name":"emit_analysis"}}`},
		{"analyze_attempt2", types.StageAnalyze, 2, `{"type":"function","function":{"name":"emit_analysis"}}`},
		{"finalize_attempt1", types.StageFinalize, 1, `{"type":"function","function":{"name":"emit_answer_document"}}`},
		{"log_triage_attempt1", types.StageLogTriage, 1, `{"type":"function","function":{"name":"emit_log_triage"}}`},
		{"perf_triage_attempt1", types.StagePerfTriage, 1, `{"type":"function","function":{"name":"emit_perf_trace"}}`},

		// Multi-emit stage stays bare "required" even on retry.
		{"extract_attempt1_keeps_required", types.StageExtract, 1, "required"},
		// Explore stays "" even on retry — never an emit-required stage.
		{"explore_attempt1_stays_empty", types.StageExplore, 1, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := &types.AgentContext{Stage: c.stage, EmitStageRetryAttempt: c.attempt}
			if got := resolveToolChoice(ctx); got != c.want {
				t.Errorf("resolveToolChoice(stage=%q attempt=%d) = %q, want %q",
					c.stage, c.attempt, got, c.want)
			}
		})
	}
}
