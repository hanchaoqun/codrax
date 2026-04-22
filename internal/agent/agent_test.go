package agent

import (
	"encoding/json"
	"strings"
	"testing"

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
}

func (s *stubEvaluator) BuildInitialInstruction(_ *types.AgentContext, _ *skill.Config) string {
	return ""
}
func (s *stubEvaluator) ShouldStop(_ llm.Response, _ int) bool { return false }
func (s *stubEvaluator) ParseOutput(
	_ *types.AgentContext, _ []llm.Message, _ []types.ToolResult, _ []types.MCPResponse,
) (*StageOutput, error) {
	s.parseCalls++
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
		{types.StageExplore, ""},
	}
	for _, c := range cases {
		if got := toolChoiceForStage(c.stage); got != c.want {
			t.Errorf("toolChoiceForStage(%q) = %q, want %q", c.stage, got, c.want)
		}
	}
}

