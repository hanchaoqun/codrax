package agent

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

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

	if got := estimateMessagesBytes(msgs); got != want {
		t.Errorf("estimateMessagesBytes = %d, want %d", got, want)
	}
}

// TestEstimateMessagesBytes_EmptySliceIsZero is the degenerate guard:
// a never-dispatched agent's messages slice starts empty, the
// watchdog must see zero (not a nil-pointer panic or a negative
// value that would trip the ratio check).
func TestEstimateMessagesBytes_EmptySliceIsZero(t *testing.T) {
	if got := estimateMessagesBytes(nil); got != 0 {
		t.Errorf("nil slice: got %d, want 0", got)
	}
	if got := estimateMessagesBytes([]llm.Message{}); got != 0 {
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
		t.Fatalf("original tool call params must stay unchanged for tool execution, got %q", got)
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
