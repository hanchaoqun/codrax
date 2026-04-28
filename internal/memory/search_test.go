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

// TestSearch_IncludesRecentTurns is the headline regression fix:
// pre-fix Store.Search only consulted s.index, leaving recent
// (uncompacted) turns invisible to recall_memory. The user trigger
// was a REPL session with many turns where compaction had not yet
// fired — recall returned 0 entries even though /history clearly
// listed the same turns under "recent turns". Post-fix scoreRecent
// surfaces matching recent turns alongside compacted ones.
func TestSearch_IncludesRecentTurns(t *testing.T) {
	recent := []Turn{
		{
			ID:        "turn-1",
			Request:   "how does the OAuth migration work?",
			Response:  "It rewrites token storage and bumps the refresh window.",
			Timestamp: time.Now().Add(-2 * time.Minute),
		},
		{
			ID:        "turn-2",
			Request:   "show me the planner skill prompt",
			Response:  "The planner emits ChangePlan via emit_change_plan.",
			Timestamp: time.Now().Add(-1 * time.Minute),
		},
	}
	s := newSearchStubStore(t, nil, recent)
	out := s.Search("oauth migration", SearchOpts{Limit: 5})
	if len(out) == 0 {
		t.Fatalf("recent-only Search must surface matching turn; got 0")
	}
	if out[0].ID != "turn-1" {
		t.Errorf("top match = %q, want 'turn-1' (oauth/migration tokens in body)", out[0].ID)
	}
}

// TestSearch_RecentTurnsRankedByRecency pins the timestamp tie-
// breaker: when recent turns share the same hit count, the more
// recent one surfaces first. Each turn is constructed so the
// query "oauth" matches exactly once in the body — equal score so
// recency is the only differentiator.
func TestSearch_RecentTurnsRankedByRecency(t *testing.T) {
	t0 := time.Now().Add(-10 * time.Minute)
	t1 := time.Now().Add(-5 * time.Minute)
	t2 := time.Now()
	recent := []Turn{
		{ID: "old", Request: "what about oauth", Response: "see prior", Timestamp: t0},
		{ID: "mid", Request: "remind me of oauth", Response: "see prior", Timestamp: t1},
		{ID: "new", Request: "tell me about oauth", Response: "see prior", Timestamp: t2},
	}
	s := newSearchStubStore(t, nil, recent)
	out := s.Search("oauth", SearchOpts{Limit: 5})
	if len(out) < 3 {
		t.Fatalf("expected 3 matches, got %d", len(out))
	}
	if out[0].ID != "new" {
		t.Errorf("expected most recent 'new' first, got %q", out[0].ID)
	}
}

// TestSearch_ChineseQuerySubstringFallback covers the Unicode
// blind-spot fix: pre-fix tokenize() returned 0 tokens for a
// Chinese query, so scoreIndex's keyword-hit loop scored every
// entry at zero and recall returned nothing — even when an entry's
// Topic / Summary / Keywords contained the query verbatim.
// Post-fix the substring fallback fires when token-set is empty
// AND the trimmed query is ≥3 runes, surfacing entries that name
// the query in any narrative surface.
func TestSearch_ChineseQuerySubstringFallback(t *testing.T) {
	idx := []IndexEntry{
		{
			ID:       "compacted-zh",
			Topic:    "记忆系统的实现细节",
			Summary:  "讨论了 IndexEntry 的字段以及压缩流程",
			Keywords: []string{"memory", "compaction"},
		},
		{
			ID:       "compacted-other",
			Topic:    "OAuth refresh token",
			Summary:  "rotate refresh tokens every 12h",
			Keywords: []string{"oauth"},
		},
	}
	s := newSearchStubStore(t, idx, nil)
	out := s.Search("记忆系统", SearchOpts{Limit: 5})
	if len(out) == 0 {
		t.Fatalf("Chinese substring query must surface matching entry; got 0")
	}
	if out[0].ID != "compacted-zh" {
		t.Errorf("top match = %q, want 'compacted-zh' (Chinese substring in Topic)", out[0].ID)
	}
}

// TestSearch_RecentChineseSubstringFallback combines both fixes:
// recent turns + Chinese query. Pre-fix this scenario returned 0
// entries on TWO independent grounds (recent ignored AND Chinese
// untokenisable). Post-fix it returns the matching turn.
func TestSearch_RecentChineseSubstringFallback(t *testing.T) {
	recent := []Turn{
		{
			ID:        "turn-zh",
			Request:   "记忆里有什么内容",
			Response:  "目前 recall_memory 工具会搜索 IndexEntry 的 Topic 和 Keywords。",
			Timestamp: time.Now(),
		},
	}
	s := newSearchStubStore(t, nil, recent)
	out := s.Search("记忆里有什么", SearchOpts{Limit: 5})
	if len(out) == 0 {
		t.Fatalf("Chinese query against recent turn must match; got 0")
	}
	if out[0].ID != "turn-zh" {
		t.Errorf("top match = %q, want 'turn-zh'", out[0].ID)
	}
}

// TestSearch_RecentDoesNotShadowCompacted pins the merge order:
// when an index entry AND a recent turn both match, the index
// entry takes precedence (higher curation — has LLM-generated
// Topic / Summary / Keywords). recent fills in only what's not
// already represented.
func TestSearch_RecentDoesNotShadowCompacted(t *testing.T) {
	idx := []IndexEntry{
		{ID: "compacted-1", Topic: "OAuth design", Keywords: []string{"oauth", "session"}, Summary: "..."},
	}
	recent := []Turn{
		{ID: "turn-other", Request: "oauth flow", Response: "see earlier discussion", Timestamp: time.Now()},
	}
	s := newSearchStubStore(t, idx, recent)
	out := s.Search("oauth", SearchOpts{Limit: 5})
	if len(out) < 2 {
		t.Fatalf("expected both entries, got %d", len(out))
	}
	if out[0].ID != "compacted-1" {
		t.Errorf("compacted entry must rank first; got %q first", out[0].ID)
	}
}

// TestSearch_RecentKindFilterApplied locks the Kind filter on the
// recent path so a `kind=plan` query doesn't surface a chitchat
// recent turn just because Search now consults recent.
func TestSearch_RecentKindFilterApplied(t *testing.T) {
	recent := []Turn{
		{ID: "chat", Kind: KindChitchat, Request: "oauth question", Response: "x", Timestamp: time.Now()},
		{ID: "plan", Kind: KindPlan, Request: "oauth plan", Response: "y", Timestamp: time.Now()},
	}
	s := newSearchStubStore(t, nil, recent)
	out := s.Search("oauth", SearchOpts{Limit: 5, Kind: string(KindPlan)})
	if len(out) != 1 || out[0].ID != "plan" {
		t.Errorf("Kind=plan filter must drop chitchat turn; got %+v", idsOf(out))
	}
}

// idsOf is a small test helper to render []IndexEntry by ID.
func idsOf(es []IndexEntry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.ID)
	}
	return out
}
