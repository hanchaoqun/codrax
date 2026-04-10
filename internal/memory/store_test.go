package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubSummarizer produces a deterministic IndexEntry from a Turn so the
// tests do not need an LLM.
type stubSummarizer struct{}

func (stubSummarizer) Summarize(_ context.Context, t Turn) (IndexEntry, error) {
	return IndexEntry{
		ID:       t.ID,
		Topic:    "topic-" + t.ID,
		Keywords: []string{strings.Fields(t.Request)[0]},
		Summary:  "summary of " + t.Request,
	}, nil
}

func TestAppendCompactsOldest(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Append 8 turns; with maxRecent=6, the oldest 2 must compact.
	for i := 0; i < 8; i++ {
		turn := Turn{
			ID:        fmt.Sprintf("t%d", i),
			Request:   fmt.Sprintf("kw%d question text", i),
			Response:  fmt.Sprintf("answer %d", i),
			Timestamp: time.Now(),
		}
		if err := s.Append(turn); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	if got := len(s.Recent()); got != 6 {
		t.Errorf("recent len = %d, want 6", got)
	}
	idx := s.Index()
	if len(idx) != 2 {
		t.Fatalf("index len = %d, want 2", len(idx))
	}
	if idx[0].ID != "t0" || idx[1].ID != "t1" {
		t.Errorf("expected oldest 2 to be compacted; got %v %v", idx[0].ID, idx[1].ID)
	}

	// Both compacted turn files must still exist on disk.
	for _, id := range []string{"t0", "t1"} {
		if _, err := os.Stat(filepath.Join(dir, "turns", id+".md")); err != nil {
			t.Errorf("turn file %s missing: %v", id, err)
		}
	}

	// MEMORY.md should contain both summaries.
	data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if !strings.Contains(string(data), "id: t0") || !strings.Contains(string(data), "id: t1") {
		t.Errorf("MEMORY.md missing expected entries; got %q", string(data))
	}
}

func TestBuildContextInlinesMatchingTurn(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for i := 0; i < 8; i++ {
		_ = s.Append(Turn{
			ID:       fmt.Sprintf("t%d", i),
			Request:  fmt.Sprintf("kw%d question", i),
			Response: fmt.Sprintf("answer %d", i),
		})
	}
	// Should match the first compacted entry whose keyword is "kw0".
	out := s.BuildContext("how about kw0 again?")
	if !strings.Contains(out, "topic-t0") {
		t.Errorf("BuildContext should surface matching index entry; got %q", out)
	}
	if !strings.Contains(out, "answer 0") {
		t.Errorf("BuildContext should inline full turn for matching entry; got %q", out)
	}
	// Recent turns are always included.
	if !strings.Contains(out, "kw7") {
		t.Errorf("BuildContext should include recent turns; got %q", out)
	}
}

func TestClearRemovesEverything(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for i := 0; i < 8; i++ {
		_ = s.Append(Turn{ID: fmt.Sprintf("t%d", i), Request: fmt.Sprintf("kw%d q", i)})
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if len(s.Recent()) != 0 || len(s.Index()) != 0 {
		t.Errorf("Clear should empty recent and index")
	}
	if _, err := os.Stat(filepath.Join(dir, "MEMORY.md")); !os.IsNotExist(err) {
		t.Errorf("MEMORY.md should be removed after Clear")
	}
}

func TestParseIndexRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for i := 0; i < 8; i++ {
		_ = s.Append(Turn{ID: fmt.Sprintf("t%d", i), Request: fmt.Sprintf("kw%d q", i)})
	}
	// Re-open and ensure the index reloads.
	s2, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := len(s2.Index()); got != 2 {
		t.Errorf("reopened index len = %d, want 2", got)
	}
}

// TestRecentSurvivesRestart locks the orphan-recovery contract: after
// 8 appends and a simulated restart, the new store must show 6 recent
// turns (the uncompacted tail of the previous session) plus the 2
// compacted index entries — i.e. nothing is lost across restart.
func TestRecentSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for i := 0; i < 8; i++ {
		ts := time.Date(2026, 1, 1, 12, 0, i, 0, time.UTC)
		if err := s.Append(Turn{
			ID:        fmt.Sprintf("turn-%02d", i),
			Request:   fmt.Sprintf("kw%d question", i),
			Response:  fmt.Sprintf("answer %d", i),
			Timestamp: ts,
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// Simulated restart: cleanly close s and open a fresh store on
	// the same dir. The Close releases the instance lock so the new
	// Store's "am I alone?" check passes and orphan recovery runs.
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	s2, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	if got := len(s2.Index()); got != 2 {
		t.Errorf("reopened index len = %d, want 2", got)
	}
	recent := s2.Recent()
	if len(recent) != 6 {
		t.Fatalf("reopened recent len = %d, want 6", len(recent))
	}
	// Recent must be the LAST 6 turns (turn-02 .. turn-07), in order.
	for i, want := 0, 2; i < 6; i, want = i+1, want+1 {
		if recent[i].ID != fmt.Sprintf("turn-%02d", want) {
			t.Errorf("recent[%d].ID = %s, want turn-%02d", i, recent[i].ID, want)
		}
		if recent[i].Request != fmt.Sprintf("kw%d question", want) {
			t.Errorf("recent[%d] request not preserved: %q", i, recent[i].Request)
		}
		if recent[i].Response != fmt.Sprintf("answer %d", want) {
			t.Errorf("recent[%d] response not preserved: %q", i, recent[i].Response)
		}
		if recent[i].Timestamp.IsZero() {
			t.Errorf("recent[%d] timestamp not recovered", i)
		}
	}

	// Crucially: BuildContext on the reopened store must include the
	// recovered recent turns, so the next conversation does not start
	// blind to what just happened in the previous session.
	ctx := s2.BuildContext("kw5 follow up")
	if !strings.Contains(ctx, "kw5 question") {
		t.Errorf("BuildContext should surface recovered recent turn; got %q", ctx)
	}
}

// TestOrphanRecoveryRespectsCap ensures we never resurrect more than
// maxRecent turns, even when turns/ holds many uncompacted files
// (simulating a legacy directory or a manually-seeded scenario).
func TestOrphanRecoveryRespectsCap(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	// Disable compaction by raising the cap, then dump 12 turns.
	s.maxRecent = 100
	for i := 0; i < 12; i++ {
		_ = s.Append(Turn{
			ID:       fmt.Sprintf("turn-%02d", i),
			Request:  fmt.Sprintf("kw%d", i),
			Response: "ok",
		})
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen with the default cap of 6 — only the newest 6 must come back.
	s2, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := len(s2.Recent()); got != 6 {
		t.Errorf("reopened recent len = %d, want 6", got)
	}
	if got := s2.Recent()[0].ID; got != "turn-06" {
		t.Errorf("oldest recovered turn = %s, want turn-06", got)
	}
	if got := s2.Recent()[5].ID; got != "turn-11" {
		t.Errorf("newest recovered turn = %s, want turn-11", got)
	}
}

// TestLivePeerCount verifies that LivePeerCount reflects the
// presence and absence of peer Stores via sidecar marker files.
// REPL /clear formats its warning from this number, so getting it
// right matters more than the number itself ever feeding into a
// safety decision.
func TestLivePeerCount(t *testing.T) {
	dir := t.TempDir()

	a, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore a: %v", err)
	}
	defer a.Close()

	// Just one Store → no peers.
	if got, err := a.LivePeerCount(); err != nil || got != 0 {
		t.Errorf("alone count: got (%d, %v), want (0, nil)", got, err)
	}

	b, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore b: %v", err)
	}

	// Both Stores see exactly one peer (the other one).
	if got, err := a.LivePeerCount(); err != nil || got != 1 {
		t.Errorf("a sees: got (%d, %v), want (1, nil)", got, err)
	}
	if got, err := b.LivePeerCount(); err != nil || got != 1 {
		t.Errorf("b sees: got (%d, %v), want (1, nil)", got, err)
	}

	// Add a third.
	c, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore c: %v", err)
	}
	if got, _ := a.LivePeerCount(); got != 2 {
		t.Errorf("a sees with c added: got %d, want 2", got)
	}

	// Close c → its sidecar is removed → a and b each see 1 peer.
	if err := c.Close(); err != nil {
		t.Fatalf("close c: %v", err)
	}
	if got, _ := a.LivePeerCount(); got != 1 {
		t.Errorf("a sees after c closed: got %d, want 1", got)
	}
	if got, _ := b.LivePeerCount(); got != 1 {
		t.Errorf("b sees after c closed: got %d, want 1", got)
	}

	// Drop a stale sidecar from a known-dead PID. Must be ignored.
	stale := filepath.Join(dir, fmt.Sprintf("%s%d-%d", sidecarPrefix, 9999999, time.Now().UnixNano()))
	if err := os.WriteFile(stale, nil, 0o644); err != nil {
		t.Fatalf("seed stale sidecar: %v", err)
	}
	if got, _ := a.LivePeerCount(); got != 1 {
		t.Errorf("a sees with stale sidecar: got %d, want 1 (stale should be ignored)", got)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("close b: %v", err)
	}
	if got, _ := a.LivePeerCount(); got != 0 {
		t.Errorf("a sees after b closed: got %d, want 0", got)
	}
}

// TestConcurrentStoresShareMemoryMd is the contract test for the
// cross-process flock around MEMORY.md. It simulates the multi-
// instance scenario by spawning N goroutines that each open their
// own Store on the same directory and run a series of Appends in
// parallel. Each Store has its own *fileLock fd, which on Linux
// flock(2) and Windows LockFileEx is exactly the same situation as
// N processes — locks are per-open-file-description, not per-process.
//
// A barrier holds every goroutine at the gate until all of them are
// ready, then releases them simultaneously. Without the barrier the
// Go scheduler tends to run goroutines mostly serially on small
// machines, which would let one Store complete (and Close, releasing
// its instance lock) before the next even calls NewStore — at which
// point the next is "alone" and legitimately recovers the previous
// store's leftovers as its own. That is correct behavior for crash
// recovery, but it is not what this test is trying to exercise.
//
// Properties verified:
//
//  1. No errors from any goroutine (no deadlock, no I/O races
//     bubbling up).
//  2. Every Append produces a turn file on disk — explicit per-
//     goroutine IDs make this deterministic.
//  3. MEMORY.md parses cleanly: every entry's ID starts with the
//     expected goroutine prefix, which would not be true if a torn
//     write spliced two entries together.
//  4. NO turn ID appears more than once in MEMORY.md. This is the
//     core safety property — the orphan-recovery filter against the
//     index plus the instance lock together must prevent the same
//     turn from being compacted by two different stores.
//  5. The total compacted count is bounded above by N * (turnsPerStore
//     - 1) and below by N * (turnsPerStore - maxRecent). The lower
//     bound is the "perfectly concurrent" case (each store compacts
//     only its own oldest); the upper bound is "serialized so each
//     store inherits the previous one's tail and compacts a lot
//     more". The window allows scheduling slack without losing the
//     teeth of the test.
//
// Explicit turn IDs are used (not the auto-generated nano+pid ones)
// because all goroutines share this test process's PID. Real codrax
// instances run in different processes with different PIDs, so the
// production path does not hit that ambiguity.
func TestConcurrentStoresShareMemoryMd(t *testing.T) {
	dir := t.TempDir()

	const numStores = 4
	const turnsPerStore = 12 // > maxRecent (6) so each store compacts

	// Barrier: every goroutine signals "ready" then waits on
	// "release". Once all numStores have signaled, the test closes
	// release and they all charge into NewStore at once.
	var ready sync.WaitGroup
	ready.Add(numStores)
	release := make(chan struct{})

	var wg sync.WaitGroup
	errs := make(chan error, numStores*turnsPerStore)
	for i := 0; i < numStores; i++ {
		wg.Add(1)
		go func(storeIdx int) {
			defer wg.Done()
			ready.Done()
			<-release

			s, err := NewStore(dir, stubSummarizer{})
			if err != nil {
				errs <- fmt.Errorf("store %d open: %w", storeIdx, err)
				return
			}
			defer s.Close()
			for j := 0; j < turnsPerStore; j++ {
				turn := Turn{
					ID:        fmt.Sprintf("g%d-t%02d", storeIdx, j),
					Request:   fmt.Sprintf("kw%d-%d question text", storeIdx, j),
					Response:  fmt.Sprintf("answer %d-%d", storeIdx, j),
					Timestamp: time.Now(),
				}
				if err := s.Append(turn); err != nil {
					errs <- fmt.Errorf("store %d turn %d: %w", storeIdx, j, err)
					return
				}
			}
		}(i)
	}
	ready.Wait()
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("%v", err)
	}

	// Every Append produced a turn file.
	turnFiles, err := os.ReadDir(filepath.Join(dir, "turns"))
	if err != nil {
		t.Fatalf("read turns dir: %v", err)
	}
	if got, want := len(turnFiles), numStores*turnsPerStore; got != want {
		t.Errorf("turn files: got %d, want %d", got, want)
	}

	// Re-open from a fresh Store and check MEMORY.md is intact.
	fresh, err := NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("fresh NewStore: %v", err)
	}
	defer fresh.Close()

	idx := fresh.Index()

	// Property 4: no turn ID appears twice. This is the safety
	// invariant the lock + orphan filter exists to enforce.
	seen := map[string]int{}
	for _, e := range idx {
		seen[e.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("turn ID %s appears %d times in MEMORY.md (double compaction)", id, n)
		}
	}

	// Property 3: every entry has a recognizable ID shape. A torn
	// write would produce garbage IDs that fail this check.
	for _, e := range idx {
		if !strings.HasPrefix(e.ID, "g") {
			t.Errorf("entry with malformed ID: %+v", e)
		}
	}

	// Property 5: count is in the expected window. Lower bound:
	// every store compacted only its own oldest few. Upper bound:
	// every store eventually inherited and compacted everyone's
	// tails (the perfectly-serialized degenerate case).
	minCompacted := numStores * (turnsPerStore - 6) // 24
	maxCompacted := numStores * (turnsPerStore - 1) // 44
	if len(idx) < minCompacted || len(idx) > maxCompacted {
		t.Errorf("compacted count %d outside [%d, %d]", len(idx), minCompacted, maxCompacted)
	}
}

