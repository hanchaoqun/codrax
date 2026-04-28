package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// stubMemory is a types.MemoryReader test double that returns a
// canned slice. Lets the recall_memory tool be exercised without
// touching internal/memory.
type stubMemory struct {
	results []types.MemoryIndexEntry
	calls   int
	lastQ   string
	lastOpt types.MemorySearchOpts
}

func (s *stubMemory) Search(query string, opts types.MemorySearchOpts) []types.MemoryIndexEntry {
	s.calls++
	s.lastQ = query
	s.lastOpt = opts
	return s.results
}

// TestRecallMemory_HappyPath drives a query end-to-end: the tool
// passes params through to the store, formats the result block, and
// returns Success=true. Locks the rendered output's structural
// markers so the LLM can rely on them for downstream parsing.
func TestRecallMemory_HappyPath(t *testing.T) {
	stub := &stubMemory{
		results: []types.MemoryIndexEntry{
			{
				ID: "turn-1", Topic: "oauth migration",
				Summary: "moved tokens to JWT", Keywords: []string{"oauth", "jwt"},
				Kind: "plan", SessionID: "sess-A",
			},
		},
	}
	ctx := &types.BusContext{Memory: stub}
	tool := &RecallMemory{}
	params := json.RawMessage(`{"query": "oauth refactor", "limit": 5}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !res.Success {
		t.Errorf("Success = false; want true")
	}
	if stub.calls != 1 {
		t.Errorf("Search called %d times; want 1", stub.calls)
	}
	if stub.lastQ != "oauth refactor" {
		t.Errorf("query forwarded as %q; want 'oauth refactor'", stub.lastQ)
	}
	if stub.lastOpt.Limit != 5 {
		t.Errorf("limit forwarded as %d; want 5", stub.lastOpt.Limit)
	}
	for _, want := range []string{
		"matched 1 entries", "turn-1", "oauth migration",
		"moved tokens to JWT", "kind=plan",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("Summary missing %q; got %q", want, res.Summary)
		}
	}
}

// TestRecallMemory_EmptyQueryRejected: an empty / whitespace query
// shortcircuits with a typed message — no Search call, no rendering
// pass. Defends against the LLM sending {} by mistake.
func TestRecallMemory_EmptyQueryRejected(t *testing.T) {
	stub := &stubMemory{}
	ctx := &types.BusContext{Memory: stub}
	tool := &RecallMemory{}
	for _, q := range []string{"", "   ", "\n\t"} {
		params := json.RawMessage(`{"query": ` + jsonQuote(q) + `}`)
		res, _ := tool.Execute(ctx, params)
		if res.Success {
			t.Errorf("query=%q should be rejected; got Success=true", q)
		}
		if !strings.Contains(res.Summary, "empty `query`") {
			t.Errorf("rejection message missing 'empty `query`'; got %q", res.Summary)
		}
	}
	if stub.calls != 0 {
		t.Errorf("Search was called %d times on empty query; should be 0", stub.calls)
	}
}

// TestRecallMemory_NoMemoryWired covers the non-REPL / single-shot
// CLI path: BusContext.Memory is nil. Tool returns a typed message
// telling the LLM to fall through to repo_map / grep, no panic.
func TestRecallMemory_NoMemoryWired(t *testing.T) {
	ctx := &types.BusContext{} // Memory: nil
	tool := &RecallMemory{}
	params := json.RawMessage(`{"query": "oauth"}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Errorf("Success should be false when Memory is nil")
	}
	if !strings.Contains(res.Summary, "prior-conversation memory is not available") {
		t.Errorf("expected nil-memory message; got %q", res.Summary)
	}
}

// TestRecallMemory_NoResultsRendersExplicitMiss verifies the empty-
// result rendering: when Search returns 0 entries, the LLM gets a
// clear "0 entries" message rather than an empty string. Empty
// strings would let the LLM hallucinate a recall.
func TestRecallMemory_NoResultsRendersExplicitMiss(t *testing.T) {
	stub := &stubMemory{results: nil}
	ctx := &types.BusContext{Memory: stub}
	tool := &RecallMemory{}
	params := json.RawMessage(`{"query": "no-such-topic"}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Errorf("Success = false on empty result; want true (an empty result IS a successful query)")
	}
	if !strings.Contains(res.Summary, "matched 0 entries") {
		t.Errorf("expected explicit '0 entries' miss line; got %q", res.Summary)
	}
}

// TestRecallMemory_ParameterSchema_HasRequiredQuery locks the
// JSON schema's `required` declaration. Without query in `required`,
// the LLM might skip it, which then triggers the empty-query
// rejection path — better to fail at schema validation upstream.
func TestRecallMemory_ParameterSchema_HasRequiredQuery(t *testing.T) {
	tool := &RecallMemory{}
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters not valid JSON: %v", err)
	}
	required, _ := schema["required"].([]any)
	found := false
	for _, r := range required {
		if r == "query" {
			found = true
		}
	}
	if !found {
		t.Errorf("query must be declared as required; got required=%v", required)
	}
}

// TestRecallMemory_NameAndDescriptionStable acts as a drift guard:
// the tool name appears in skill prose verbatim, and the description
// carries the trigger markers (`记忆 / previously / earlier ...`)
// the LLM keys on. Renaming or weakening them breaks the explorer
// skill's instruction without compile error.
func TestRecallMemory_NameAndDescriptionStable(t *testing.T) {
	tool := &RecallMemory{}
	if tool.Name() != "recall_memory" {
		t.Errorf("Name() = %q, want recall_memory", tool.Name())
	}
	desc := tool.Description()
	for _, marker := range []string{"记忆", "previously", "earlier", "Prior-conversation"} {
		if !strings.Contains(desc, marker) {
			t.Errorf("Description missing trigger marker %q; LLM may not call this tool", marker)
		}
	}
}

// jsonQuote quotes s as a JSON string literal. Used in the
// EmptyQueryRejected test to avoid pulling fmt.Sprintf in patterns
// that would clash with json.Unmarshal whitespace handling.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Compile-time assertion the Tool interface is satisfied. If a
// future Tool method is added, this file fails to compile rather
// than the runtime registration panicking on a missing method.
var _ Tool = &RecallMemory{}

// Compile-time assertion the stub satisfies MemoryReader. Catches
// a future MemoryReader interface change at test time.
var _ types.MemoryReader = &stubMemory{}

// Compile-time check that context import is used (some package
// configurations strip imports lazily; this prevents the linter
// from removing the dependency this test indirectly uses).
var _ = context.Background

// TestRecallMemory_AppendsListMemoryHintOnSelfReferentialMatch
// pins the cross-tool guidance feature: when recall_memory returns
// ≤2 entries AND every match's Topic looks like a meta-reference
// to memory itself, the tool result appends a "consider
// list_memory" suggestion. This trains the LLM's next-turn
// decision without changing schema.
func TestRecallMemory_AppendsListMemoryHintOnSelfReferentialMatch(t *testing.T) {
	mem := &stubMemory{results: []types.MemoryIndexEntry{
		{ID: "a", Topic: "看一下记忆里都有哪些"},
		{ID: "b", Topic: "what's in memory today"},
	}}
	ctx := &types.BusContext{Memory: mem}
	tool := &RecallMemory{}
	res, _ := tool.Execute(ctx, json.RawMessage(`{"query": "memory recall history content"}`))
	if !res.Success {
		t.Fatalf("Execute failed: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "list_memory") {
		t.Errorf("self-referential hint missing — should suggest list_memory: %q", res.Summary)
	}
}

// TestRecallMemory_NoHintOnGenuineMatches guards the false-positive:
// when recall returns ≥1 substantive entry (not all self-referential),
// the hint must NOT fire. Otherwise we'd dilute legitimate searches
// with off-topic suggestions.
func TestRecallMemory_NoHintOnGenuineMatches(t *testing.T) {
	mem := &stubMemory{results: []types.MemoryIndexEntry{
		{ID: "a", Topic: "OAuth refresh token rotation"},
		{ID: "b", Topic: "看一下记忆里都有哪些"},
	}}
	ctx := &types.BusContext{Memory: mem}
	tool := &RecallMemory{}
	res, _ := tool.Execute(ctx, json.RawMessage(`{"query": "oauth"}`))
	if !res.Success {
		t.Fatalf("Execute failed: %q", res.Summary)
	}
	if strings.Contains(res.Summary, "self-referential") {
		t.Errorf("hint mis-fired on a genuine match: %q", res.Summary)
	}
}

// TestRecallMemory_HintFiresAtThreeSelfReferential covers the
// 2026-04-28 user-trace bump: 3 entries all self-referential must
// fire the list_memory hint. Pre-bump threshold (≤2) missed this
// realistic pattern.
func TestRecallMemory_HintFiresAtThreeSelfReferential(t *testing.T) {
	mem := &stubMemory{results: []types.MemoryIndexEntry{
		{ID: "a", Topic: "看一下记忆力都有哪些内容"},
		{ID: "b", Topic: "找一下之前的记忆里"},
		{ID: "c", Topic: "看一下记忆里有多少"},
	}}
	ctx := &types.BusContext{Memory: mem}
	tool := &RecallMemory{}
	res, _ := tool.Execute(ctx, json.RawMessage(`{"query": "memory recall history content"}`))
	if !res.Success {
		t.Fatalf("Execute failed: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "list_memory") {
		t.Errorf("3 self-referential entries must fire list_memory hint: %q", res.Summary)
	}
}

// TestRecallMemory_NoHintAboveLowRecallThreshold pins the size
// gate at ≤5: 6 self-referential entries no longer fire the hint
// (genuinely large self-ref result set means the user really IS
// asking about memory-meta and is getting useful results).
func TestRecallMemory_NoHintAboveLowRecallThreshold(t *testing.T) {
	mem := &stubMemory{results: []types.MemoryIndexEntry{
		{ID: "a", Topic: "记忆里有哪些"},
		{ID: "b", Topic: "what's in memory"},
		{ID: "c", Topic: "history of our chat"},
		{ID: "d", Topic: "previously discussed memory"},
		{ID: "e", Topic: "look at my memory"},
		{ID: "f", Topic: "the memory contents I asked"},
	}}
	ctx := &types.BusContext{Memory: mem}
	tool := &RecallMemory{}
	res, _ := tool.Execute(ctx, json.RawMessage(`{"query": "history"}`))
	if !res.Success {
		t.Fatalf("Execute failed: %q", res.Summary)
	}
	if strings.Contains(res.Summary, "INVENTORY of memory") {
		t.Errorf("hint should NOT fire above ≤5 threshold: %q", res.Summary)
	}
}

// TestRecallMemory_HintInZeroResultsPath also surfaces in the zero-
// matches branch: the empty-result message names list_memory as one
// of the alternatives.
func TestRecallMemory_HintInZeroResultsPath(t *testing.T) {
	mem := &stubMemory{results: nil}
	ctx := &types.BusContext{Memory: mem}
	tool := &RecallMemory{}
	res, _ := tool.Execute(ctx, json.RawMessage(`{"query": "anything"}`))
	if !res.Success {
		t.Fatalf("Execute failed: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "list_memory") {
		t.Errorf("zero-result message should mention list_memory option: %q", res.Summary)
	}
}
