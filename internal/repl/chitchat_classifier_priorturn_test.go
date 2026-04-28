package repl

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestBuildPriorTurnHint_EmptyOnNoStore covers the byte-identical-
// degradation contract: nil store / empty Recent → empty hint, and
// the classifier must treat empty hint identically to the pre-fix
// shape.
func TestBuildPriorTurnHint_EmptyOnNoStore(t *testing.T) {
	r := &REPL{store: nil}
	if got := r.buildPriorTurnHint(); got != "" {
		t.Errorf("nil store should produce empty hint; got %q", got)
	}
}

// TestBuildPriorTurnHint_EmptyOnEmptyRecent guards the first-turn
// case: store wired but no turns yet.
func TestBuildPriorTurnHint_EmptyOnEmptyRecent(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	r := &REPL{store: store}
	if got := r.buildPriorTurnHint(); got != "" {
		t.Errorf("empty Recent should produce empty hint; got %q", got)
	}
}

// TestBuildPriorTurnHint_FormatIncludesKindAndTopic locks the wire
// format the classifier prompt expects: 'kind=<x> topic=<y>'.
// Future refactors that reorder the fields or change separators
// must update the test deliberately.
func TestBuildPriorTurnHint_FormatIncludesKindAndTopic(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	store.Append(memory.Turn{
		ID:        "turn-1",
		Request:   "看一下记忆力都有哪些内容？",
		Response:  "[list_memory] 10 entries...",
		Timestamp: time.Now(),
		Kind:      memory.KindChitchat,
	})

	r := &REPL{store: store}
	hint := r.buildPriorTurnHint()
	if !strings.Contains(hint, "kind=chitchat") {
		t.Errorf("hint missing kind: %q", hint)
	}
	if !strings.Contains(hint, "topic=") {
		t.Errorf("hint missing topic: %q", hint)
	}
	if !strings.Contains(hint, "看一下记忆力") {
		t.Errorf("hint missing original topic text: %q", hint)
	}
}

// TestBuildPriorTurnHint_TopicTruncation pins the rune-aware cap:
// topics longer than 100 runes get truncated with ellipsis. Uses
// repeated CJK chars to verify rune (not byte) counting.
func TestBuildPriorTurnHint_TopicTruncation(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	store.Append(memory.Turn{
		ID:        "turn-1",
		Request:   strings.Repeat("中", 200),
		Timestamp: time.Now(),
		Kind:      memory.KindChitchat,
	})

	r := &REPL{store: store}
	hint := r.buildPriorTurnHint()
	if !strings.HasSuffix(hint, "…") {
		t.Errorf("oversized topic should end with ellipsis: %q", hint)
	}
	if rc := len([]rune(hint)); rc > 210 {
		t.Errorf("hint runes %d exceeds reasonable cap (~200 + suffix)", rc)
	}
}

// classifierResp builds a single canned response with the
// classification tool call shape the classifier expects.
func classifierResp(decision string) llm.Response {
	return llm.Response{
		ToolCalls: []llm.ToolCall{
			{
				ID:     "call-1",
				Name:   "emit_chitchat_classification",
				Params: []byte(`{"decision":"` + decision + `","reason":"test"}`),
			},
		},
	}
}

// lastClassifierUserContent extracts the user-role message from the
// most recent scripted Chat call. Used by the priorTurn tests below
// to assert what the classifier rendered into the prompt.
func lastClassifierUserContent(adapter *scriptedChatAdapter) string {
	if len(adapter.calls) == 0 {
		return ""
	}
	msgs := adapter.calls[len(adapter.calls)-1].messages
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

// TestClassifyAcceptsHintParameter exercises the new signature with
// a non-nil hint, asserting the hint shows up in the user content
// alongside the current message.
func TestClassifyAcceptsHintParameter(t *testing.T) {
	hint := "kind=chitchat topic=记忆里目前有 10 条记录"
	current := "展开一下 10"

	adapter := &scriptedChatAdapter{
		responses: []llm.Response{classifierResp("chitchat")},
	}
	c := &llmChitchatClassifier{adapter: adapter}

	got, err := c.Classify(current, hint)
	if err != nil {
		t.Fatalf("Classify with hint failed: %v", err)
	}
	if !got {
		t.Errorf("expected chitchat decision; got %v", got)
	}
	user := lastClassifierUserContent(adapter)
	if !strings.Contains(user, "## priorTurn:") {
		t.Errorf("user content should contain priorTurn section; got %q", user)
	}
	if !strings.Contains(user, hint) {
		t.Errorf("user content should embed the hint verbatim; got %q", user)
	}
	if !strings.Contains(user, "## current: "+current) {
		t.Errorf("user content should embed the current message section; got %q", user)
	}
}

// TestClassify_EmptyHintIsByteIdenticalToNoHint locks the regression
// guard: when priorTurnHint is "", the user content sent to the LLM
// must be JUST the userLine — not a wrapped section, no prefix, no
// suffix. Pre-fix shape preserved byte-for-byte.
func TestClassify_EmptyHintIsByteIdenticalToNoHint(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{classifierResp("repo_question")},
	}
	c := &llmChitchatClassifier{adapter: adapter}
	if _, err := c.Classify("how does X work?", ""); err != nil {
		t.Fatalf("Classify err: %v", err)
	}
	if got := lastClassifierUserContent(adapter); got != "how does X work?" {
		t.Errorf("empty hint should yield raw userLine; got %q", got)
	}
}

// TestClassify_WhitespaceHintTreatedAsEmpty pins the trim semantics:
// a hint that's only whitespace is treated as absent so the wrapped
// section never appears (would otherwise confuse the LLM with an
// empty priorTurn frame).
func TestClassify_WhitespaceHintTreatedAsEmpty(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{classifierResp("repo_question")},
	}
	c := &llmChitchatClassifier{adapter: adapter}
	if _, err := c.Classify("test", "   \n\t  "); err != nil {
		t.Fatalf("Classify err: %v", err)
	}
	if user := lastClassifierUserContent(adapter); strings.Contains(user, "priorTurn:") {
		t.Errorf("whitespace hint should NOT produce priorTurn frame; got %q", user)
	}
}

// TestChitchatClassifierSystemPrompt_TeachesPriorTurnRouting is the
// prompt-content guard for the multi-turn refinement fix. Future
// edits that drop the priorTurn routing rules regress this test
// instead of silently regressing UX. Specific keywords pinned
// because the classifier MUST teach the LLM what the hint shape
// is and how to use it for continuation references.
func TestChitchatClassifierSystemPrompt_TeachesPriorTurnRouting(t *testing.T) {
	mustContain := []string{
		"priorTurn",        // hint section name
		"continuation",     // routing concept
		"multi-turn refinement", // pattern label
		"recall_memory",    // chained next step on chitchat side
		"Do NOT route to chitchat just because", // false-positive guard
		"Pipeline-to-pipeline",                  // pipeline-prior path stays
		"absent",           // empty hint behaviour
	}
	for _, sub := range mustContain {
		if !strings.Contains(chitchatClassifierSystemPrompt, sub) {
			t.Errorf("classifier prompt missing priorTurn rule keyword %q\nfull prompt:\n%s",
				sub, chitchatClassifierSystemPrompt)
		}
	}
}
