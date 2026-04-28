package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// stubLister implements both MemoryReader and MemoryLister so tests
// can exercise the list_memory tool through the same capability gate
// the production *memory.Adapter satisfies.
type stubLister struct {
	rows []types.MemoryIndexEntry
	last types.MemoryListOpts
}

func (s *stubLister) Search(query string, _ types.MemorySearchOpts) []types.MemoryIndexEntry {
	return nil
}

func (s *stubLister) List(opts types.MemoryListOpts) []types.MemoryIndexEntry {
	s.last = opts
	return s.rows
}

// stubReaderOnly implements ONLY MemoryReader (no MemoryLister) so
// the capability-gate negative path is exercised.
type stubReaderOnly struct{}

func (stubReaderOnly) Search(_ string, _ types.MemorySearchOpts) []types.MemoryIndexEntry {
	return nil
}

// TestListMemory_HappyPath_RendersEntries exercises the typical
// browse-mode call: lister returns 2 entries, the tool renders them
// with the [list_memory] banner, kind/timestamp/topic preserved.
func TestListMemory_HappyPath_RendersEntries(t *testing.T) {
	rows := []types.MemoryIndexEntry{
		{ID: "turn-200-1", Topic: "newer", Kind: "chitchat", Timestamp: time.Now()},
		{ID: "turn-100-1", Topic: "older", Kind: "pipeline", Timestamp: time.Now().Add(-1 * time.Hour)},
	}
	lister := &stubLister{rows: rows}
	ctx := &types.BusContext{Memory: lister}

	tool := &ListMemory{}
	res, err := tool.Execute(ctx, json.RawMessage(`{"limit": 10}`))
	if err != nil || !res.Success {
		t.Fatalf("Execute failed: err=%v summary=%q", err, res.Summary)
	}
	if !strings.Contains(res.Summary, "[list_memory]") {
		t.Errorf("missing banner; got %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "newer") || !strings.Contains(res.Summary, "older") {
		t.Errorf("entries not rendered: %q", res.Summary)
	}
	if lister.last.Limit != 10 {
		t.Errorf("limit not threaded: got %d", lister.last.Limit)
	}
}

// TestListMemory_DefaultLimit covers the "0 → 10" default path.
func TestListMemory_DefaultLimit(t *testing.T) {
	lister := &stubLister{}
	ctx := &types.BusContext{Memory: lister}
	tool := &ListMemory{}
	_, _ = tool.Execute(ctx, json.RawMessage(`{}`))
	// list_memory.go does NOT pre-clamp limit (it threads to the
	// MemoryListOpts unchanged; Store.List does the clamping). So
	// the lister sees limit=0 here, and the production Store.List
	// applies the default. We assert the params reach the lister.
	if lister.last.Limit != 0 {
		t.Errorf("expected raw limit=0 to lister; got %d", lister.last.Limit)
	}
}

// TestListMemory_HardCap is exercised at the Store layer test; for
// the tool layer we just assert the caller-supplied limit reaches
// the lister untouched (the boundary keeps clamping in one place).
func TestListMemory_HardCap(t *testing.T) {
	lister := &stubLister{}
	ctx := &types.BusContext{Memory: lister}
	tool := &ListMemory{}
	_, _ = tool.Execute(ctx, json.RawMessage(`{"limit": 999}`))
	if lister.last.Limit != 999 {
		t.Errorf("limit should reach lister verbatim; got %d", lister.last.Limit)
	}
}

// TestListMemory_NilMemory_TypedReply pins the unavailable shape:
// when ctx.Memory is nil, the tool returns Success=false with a
// typed message matching the recall_memory pattern. No panic.
func TestListMemory_NilMemory_TypedReply(t *testing.T) {
	tool := &ListMemory{}
	res, err := tool.Execute(&types.BusContext{}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Success {
		t.Errorf("nil Memory must surface Success=false")
	}
	if !strings.Contains(res.Summary, "list_memory unavailable") {
		t.Errorf("unavailable message missing: %q", res.Summary)
	}
}

// TestListMemory_NoListerCapability_TypedReply pins the soft gate:
// when the wired MemoryReader does not implement MemoryLister
// (test stub from another package, future alternative impl), the
// tool surfaces a typed unavailable reply rather than panicking.
func TestListMemory_NoListerCapability_TypedReply(t *testing.T) {
	tool := &ListMemory{}
	ctx := &types.BusContext{Memory: stubReaderOnly{}}
	res, _ := tool.Execute(ctx, json.RawMessage(`{}`))
	if res.Success {
		t.Errorf("no-Lister-capability must surface Success=false")
	}
	if !strings.Contains(res.Summary, "MemoryLister") {
		t.Errorf("unavailable reason should name MemoryLister: %q", res.Summary)
	}
}

// TestListMemory_InvalidSinceRejected pins the time-parsing gate.
func TestListMemory_InvalidSinceRejected(t *testing.T) {
	lister := &stubLister{}
	ctx := &types.BusContext{Memory: lister}
	tool := &ListMemory{}
	res, _ := tool.Execute(ctx, json.RawMessage(`{"since": "not-a-timestamp"}`))
	if res.Success {
		t.Errorf("invalid since must surface Success=false")
	}
	if !strings.Contains(res.Summary, "RFC3339") {
		t.Errorf("rejection reason should mention RFC3339; got %q", res.Summary)
	}
}

// TestListMemory_EmptyResultGuidance covers the 0-entries render
// path: the tool tells the LLM the memory is genuinely empty
// (rather than letting the LLM guess at a "nothing matched" reason).
func TestListMemory_EmptyResultGuidance(t *testing.T) {
	lister := &stubLister{rows: nil}
	ctx := &types.BusContext{Memory: lister}
	tool := &ListMemory{}
	res, _ := tool.Execute(ctx, json.RawMessage(`{"limit": 5}`))
	if !res.Success {
		t.Errorf("empty result is still a successful call; got Success=false")
	}
	if !strings.Contains(res.Summary, "0 entries") {
		t.Errorf("empty banner missing: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "genuinely empty") {
		t.Errorf("empty guidance missing: %q", res.Summary)
	}
}
