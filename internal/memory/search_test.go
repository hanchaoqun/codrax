package memory

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// newSearchStubStore builds a *Store backed by an in-memory index
// (no filesystem) so Search tests don't need disk I/O. Mirrors the
// minimal pattern used elsewhere in the package's tests; copies the
// MemorySettings hydration path so policy lookups behave identically.
func newSearchStubStore(t *testing.T, idx []IndexEntry, recent []Turn) *Store {
	t.Helper()
	ms := types.ResolvedMemorySettings(types.MemorySettings{})
	return &Store{
		settings:  ms,
		maxRecent: ms.MaxRecentTurns,
		maxBytes:  ms.MaxRecentBytes,
		bcLimits: buildContextLimits{
			maxTurnBodyBytes:       ms.MaxTurnBodyBytes,
			maxBuildContextMatches: ms.MaxBuildContextMatches,
		},
		index:  append([]IndexEntry(nil), idx...),
		recent: append([]Turn(nil), recent...),
	}
}

// TestSearch_RanksByKeywordOverlap locks the primary scoring path:
// an entry whose Keywords overlap with the query ranks above one
// that doesn't, regardless of insertion order. Verifies Search
// reuses the existing scoreIndex (no second algorithm to maintain).
func TestSearch_RanksByKeywordOverlap(t *testing.T) {
	idx := []IndexEntry{
		{ID: "noise", Topic: "off", Keywords: []string{"foo", "bar"}, Summary: "x"},
		{ID: "hit", Topic: "on", Keywords: []string{"oauth", "session"}, Summary: "y"},
	}
	s := newSearchStubStore(t, idx, nil)
	out := s.Search("oauth migration", SearchOpts{Limit: 5})
	if len(out) == 0 {
		t.Fatal("expected at least one match")
	}
	if out[0].ID != "hit" {
		t.Errorf("top hit = %q, want 'hit' (oauth keyword overlap)", out[0].ID)
	}
}

// TestSearch_HardCapsLimit verifies the 20-entry hard ceiling
// regardless of the caller's request, so a misbehaving LLM cannot
// blow the context window with limit=999.
func TestSearch_HardCapsLimit(t *testing.T) {
	var idx []IndexEntry
	for i := 0; i < 50; i++ {
		idx = append(idx, IndexEntry{
			ID:       string(rune('a'+i)) + "-id",
			Topic:    "topic",
			Keywords: []string{"oauth", "common"},
			Summary:  "s",
		})
	}
	s := newSearchStubStore(t, idx, nil)
	out := s.Search("oauth", SearchOpts{Limit: 999})
	if len(out) > 20 {
		t.Errorf("Search returned %d entries; hard cap is 20", len(out))
	}
}

// TestSearch_KindFilter narrows the scan to one Kind. Entries of
// other Kinds are excluded BEFORE scoring so the cap math reflects
// the requested slice.
func TestSearch_KindFilter(t *testing.T) {
	idx := []IndexEntry{
		{ID: "p1", Kind: KindPipeline, Keywords: []string{"oauth"}, Summary: "p"},
		{ID: "c1", Kind: KindChitchat, Keywords: []string{"oauth"}, Summary: "c"},
		{ID: "p2", Kind: KindPipeline, Keywords: []string{"oauth"}, Summary: "p2"},
	}
	s := newSearchStubStore(t, idx, nil)
	out := s.Search("oauth", SearchOpts{Kind: string(KindPipeline)})
	for _, e := range out {
		if e.Kind != KindPipeline {
			t.Errorf("Kind filter leak: got Kind=%q in pipeline-only result", e.Kind)
		}
	}
}

// TestSearch_EmptyQueryReturnsNil drops the trivial / abusive case:
// an empty / whitespace-only query never matches. Surfaces in the
// tool layer so the LLM gets a clean "rejected: empty query"
// rather than scanning the whole index.
func TestSearch_EmptyQueryReturnsNil(t *testing.T) {
	idx := []IndexEntry{{ID: "x", Keywords: []string{"foo"}, Summary: "y"}}
	s := newSearchStubStore(t, idx, nil)
	if got := s.Search("", SearchOpts{}); got != nil {
		t.Errorf("empty query should return nil; got %d entries", len(got))
	}
	if got := s.Search("   ", SearchOpts{}); got != nil {
		t.Errorf("whitespace-only query should return nil; got %d entries", len(got))
	}
}

// TestSearch_IncludeBodyPullsRecentResponse covers the
// IncludeBody=true path: when an IndexEntry's ID matches a still-
// uncompacted Turn in the recent buffer, body is populated.
func TestSearch_IncludeBodyPullsRecentResponse(t *testing.T) {
	idx := []IndexEntry{{ID: "turn-1", Keywords: []string{"oauth"}, Summary: "y"}}
	recent := []Turn{
		{ID: "turn-1", Request: "?", Response: "FULL TEXT BODY", Timestamp: time.Now()},
	}
	s := newSearchStubStore(t, idx, recent)
	out := s.Search("oauth", SearchOpts{IncludeBody: true})
	if len(out) != 1 {
		t.Fatalf("expected 1 match, got %d", len(out))
	}
	if got := bodyOf(out[0]); got != "FULL TEXT BODY" {
		t.Errorf("body=%q, want FULL TEXT BODY", got)
	}
}

// TestSearch_IncludeBodyDefaultOff verifies bodies are NOT inlined
// unless the caller asks. Default false saves tokens.
func TestSearch_IncludeBodyDefaultOff(t *testing.T) {
	idx := []IndexEntry{{ID: "turn-1", Keywords: []string{"oauth"}, Summary: "y"}}
	recent := []Turn{
		{ID: "turn-1", Request: "?", Response: "FULL TEXT", Timestamp: time.Now()},
	}
	s := newSearchStubStore(t, idx, recent)
	out := s.Search("oauth", SearchOpts{}) // IncludeBody defaults false
	if len(out) != 1 {
		t.Fatalf("expected 1 match")
	}
	if got := bodyOf(out[0]); got != "" {
		t.Errorf("body should be empty when IncludeBody=false; got %q", got)
	}
}

// TestSearch_NilStoreSafe defends against accidental nil receiver:
// the orchestrator's CancelChecker pattern returns nil from the
// closure when uninstantiated, mirror that defensiveness here.
func TestSearch_NilStoreSafe(t *testing.T) {
	var s *Store
	if got := s.Search("oauth", SearchOpts{}); got != nil {
		t.Errorf("nil receiver Search should return nil; got %v", got)
	}
}

// TestAdapter_TranslatesEntries covers the boundary type adapter:
// every IndexEntry field maps onto the types.MemoryIndexEntry mirror
// without loss, and the Body payload is read via bodyOf.
func TestAdapter_TranslatesEntries(t *testing.T) {
	idx := []IndexEntry{{
		ID: "turn-1", Topic: "oauth migration", Summary: "moved to JWT",
		Keywords: []string{"oauth", "jwt"}, Entities: []string{"AuthHandler"},
		Refs: []string{"turn-0"}, Kind: KindPipeline, SessionID: "sess-A",
		FullRef: "turns/turn-1.md", body: "RESP",
	}}
	s := newSearchStubStore(t, idx, nil)
	a := NewAdapter(s)
	out := a.Search("oauth", types.MemorySearchOpts{IncludeBody: true})
	if len(out) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(out))
	}
	e := out[0]
	if e.ID != "turn-1" || e.Topic != "oauth migration" || e.Summary != "moved to JWT" {
		t.Errorf("scalar fields lost in translation: %+v", e)
	}
	if len(e.Keywords) != 2 || e.Keywords[0] != "oauth" {
		t.Errorf("Keywords not copied: %v", e.Keywords)
	}
	if e.Kind != "pipeline" {
		t.Errorf("Kind = %q, want 'pipeline'", e.Kind)
	}
	if e.Body != "RESP" {
		t.Errorf("Body = %q, want RESP", e.Body)
	}
}

// TestAdapter_NilStoreReturnsNil covers the test-fixture path:
// NewAdapter(nil) produces nil; calling Search on nil receiver is
// safe and returns nil. BusContext.Memory handles nil interface
// values without a panic via the recall_memory tool's nil-check.
func TestAdapter_NilStoreReturnsNil(t *testing.T) {
	if a := NewAdapter(nil); a != nil {
		t.Errorf("NewAdapter(nil) should return nil")
	}
	var a *Adapter
	if got := a.Search("x", types.MemorySearchOpts{}); got != nil {
		t.Errorf("nil Adapter Search should return nil; got %v", got)
	}
}
