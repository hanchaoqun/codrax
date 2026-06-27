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

	"github.com/hanchaoqun/codrax/internal/types"
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
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

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
	// Compaction is async — wait for the background goroutine to
	// drain before asserting on Recent/Index state.
	s.compactWG.Wait()

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

// TestSummarizerSeesExpandedRequest locks C4: when a Turn carries a
// paste-expanded RequestForSummary, the summarizer must be called with
// that expanded form, not the display-folded Request. Otherwise the
// resulting IndexEntry Summary/Keywords would be derived from a
// "[Pasted text #N]" placeholder and future keyword-matched retrieval
// would never surface the turn by paste-content keywords.
func TestSummarizerSeesExpandedRequest(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Turn 0 has a display-folded Request and an expanded form that
	// begins with a unique token the stub summarizer will elevate to
	// the IndexEntry keyword.
	expanded := "REALCONTENTKW0 followed by a long paste body"
	_ = s.Append(Turn{
		ID:                "t0",
		Request:           "[Pasted text #0 +42 lines +500 chars]",
		RequestForSummary: expanded,
		Response:          "answer 0",
	})
	// Pad until t0 rolls out of the recent buffer and compacts.
	for i := 1; i < 8; i++ {
		_ = s.Append(Turn{
			ID:       fmt.Sprintf("t%d", i),
			Request:  fmt.Sprintf("kw%d question", i),
			Response: fmt.Sprintf("answer %d", i),
		})
	}
	s.compactWG.Wait()

	idx := s.Index()
	if len(idx) < 1 {
		t.Fatalf("expected at least one compacted entry; got %d", len(idx))
	}
	var t0Entry *IndexEntry
	for i := range idx {
		if idx[i].ID == "t0" {
			t0Entry = &idx[i]
			break
		}
	}
	if t0Entry == nil {
		t.Fatalf("t0 not compacted; idx=%v", idx)
	}
	if !strings.Contains(t0Entry.Summary, "REALCONTENTKW0") {
		t.Errorf("Summary should derive from expanded request; got %q", t0Entry.Summary)
	}
	if strings.Contains(t0Entry.Summary, "Pasted text") {
		t.Errorf("Summary should NOT contain display placeholder; got %q", t0Entry.Summary)
	}
	if len(t0Entry.Keywords) == 0 || t0Entry.Keywords[0] != "REALCONTENTKW0" {
		t.Errorf("Keywords should surface expanded-form token; got %v", t0Entry.Keywords)
	}

	// BuildContext, given a query that matches the paste keyword, must
	// still find t0 — proves the summarizer swap plumbed end to end.
	out := s.BuildContext("what was REALCONTENTKW0 about?")
	if !strings.Contains(out, "topic-t0") {
		t.Errorf("BuildContext should surface t0 by expanded-form keyword; got %q", out)
	}
}

// TestBuildContextSurfacesMatchingEntrySummary locks the post-C1
// behavior: when a compacted IndexEntry's keywords match the
// current request, BuildContext renders ONLY its Topic + Summary
// bullet. Full turn text is never inlined, no matter how large the
// on-disk turn file is.
func TestBuildContextSurfacesMatchingEntrySummary(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()
	for i := 0; i < 8; i++ {
		_ = s.Append(Turn{
			ID:       fmt.Sprintf("t%d", i),
			Request:  fmt.Sprintf("kw%d question", i),
			Response: fmt.Sprintf("answer %d", i),
		})
	}
	s.compactWG.Wait()
	// Should match the first compacted entry whose keyword is "kw0".
	out := s.BuildContext("how about kw0 again?")
	if !strings.Contains(out, "topic-t0") {
		t.Errorf("BuildContext should surface matching index entry topic; got %q", out)
	}
	if !strings.Contains(out, "summary of kw0 question") {
		t.Errorf("BuildContext should render index entry summary; got %q", out)
	}
	// Full turn text must NEVER be inlined — only Summary.
	if strings.Contains(out, "answer 0") {
		t.Errorf("BuildContext must not inline full turn text; got %q", out)
	}
	if strings.Contains(out, "Full turn:") {
		t.Errorf("BuildContext must not emit Full turn: block; got %q", out)
	}
	// Recent turns are always included.
	if !strings.Contains(out, "kw7") {
		t.Errorf("BuildContext should include recent turns; got %q", out)
	}
}

// TestSanitizeErrorResponse covers the structural detector that
// replaces error-laden turn responses with a clean placeholder
// before they leak into the LLM's prior-conversation context.
//
// Locks the 2026-04-12 REPL memory-pollution fix: historical
// pipeline errors ("error: task X stuck at stage explore after
// 5 visits", OpenAI 400 context_length_exceeded payloads, etc.)
// used to be persisted verbatim by `repl.recordTurn` and then
// re-rendered into every subsequent analyzer prompt. Structural
// detection means even pre-fix legacy turn files on disk get
// sanitized at read time.
func TestSanitizeErrorResponse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "normal answer passes through",
			in:   "The agent that can invoke a subagent is explorer.",
			want: "The agent that can invoke a subagent is explorer.",
		},
		{
			name: "empty passes through",
			in:   "",
			want: "",
		},
		{
			name: "already-sanitized placeholder is idempotent",
			in:   errorResponsePlaceholder,
			want: errorResponsePlaceholder,
		},
		{
			name: "leading error: prefix replaced",
			in:   "error: task abc123 stuck at stage explore after 5 visits",
			want: errorResponsePlaceholder,
		},
		{
			name: "error: prefix with leading whitespace (oneLine collapse)",
			in:   "  error: analyze: something failed",
			want: errorResponsePlaceholder,
		},
		{
			name: "oscillation phrase in body",
			in:   "Codrax ran into trouble; stuck at stage explore while trying to satisfy ERM.",
			want: errorResponsePlaceholder,
		},
		{
			name: "oscillation guard log phrase",
			in:   "explorer retry limit: oscillation guard tripped at visit 5",
			want: errorResponsePlaceholder,
		},
		{
			name: "OpenAI context_length_exceeded payload",
			in:   "error: analyze: OpenAI API error: {\"error\":{\"code\":\"context_length_exceeded\",\"message\":\"...\"}}",
			want: errorResponsePlaceholder,
		},
		{
			name: "legitimate answer mentioning error handling NOT flagged",
			in:   "Error handling lives in internal/orchestrator/orchestrator.go at line 587.",
			want: "Error handling lives in internal/orchestrator/orchestrator.go at line 587.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeErrorResponse(c.in); got != c.want {
				t.Errorf("sanitizeErrorResponse(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestBuildContextSanitizesLegacyErrorTurn verifies the read-side
// integration: a legacy error-laden turn file already on disk must
// appear as the clean placeholder in the assembled BuildContext
// output, without requiring re-writing the turn file.
func TestBuildContextSanitizesLegacyErrorTurn(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Simulate a pre-fix turn file: the REPL persisted the full
	// rendered error response verbatim.
	if err := s.Append(Turn{
		ID:       "legacy-err",
		Request:  "how does some broken thing work",
		Response: "error: task 3e5d94c4 stuck at stage explore after 5 visits. The pipeline includes the ListFiles tool twice.",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	out := s.BuildContext("unrelated next question")
	if !strings.Contains(out, "### Recent conversation") {
		t.Fatalf("expected recent conversation section, got %q", out)
	}
	if !strings.Contains(out, errorResponsePlaceholder) {
		t.Errorf("legacy error turn should be replaced with placeholder; got %q", out)
	}
	if strings.Contains(out, "stuck at stage explore") {
		t.Errorf("raw error text should not leak into BuildContext output; got %q", out)
	}
	if strings.Contains(out, "3e5d94c4") {
		t.Errorf("historical task UUID should not leak into BuildContext output; got %q", out)
	}
}

// TestSanitizeStripsANSIAndCapsSize locks the write-side contract:
// glamour-style ANSI escapes must not survive to disk, and a
// pathologically large Response must be truncated at types.DefaultMemorySettings().MaxTurnBodyBytes.
// Regression guard for the 2026-04-12 incident where a 2.8 MB turn file
// blew the analyzer's context window.
func TestSanitizeStripsANSIAndCapsSize(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	huge := strings.Repeat("\x1b[38;5;252mA\x1b[0m", 200*1024) // ~3.4 MB pre-strip
	if err := s.Append(Turn{
		ID:       "big",
		Request:  "q",
		Response: huge,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// On-disk file must be clean and bounded.
	raw, err := os.ReadFile(filepath.Join(dir, "turns", "big.md"))
	if err != nil {
		t.Fatalf("read turn file: %v", err)
	}
	if strings.Contains(string(raw), "\x1b[") {
		t.Errorf("ANSI escape leaked to disk")
	}
	// Header adds ~100 bytes; cap is MaxTurnBodyBytes body + truncation marker.
	if len(raw) > types.DefaultMemorySettings().MaxTurnBodyBytes+512 {
		t.Errorf("turn file size %d exceeds cap %d", len(raw), types.DefaultMemorySettings().MaxTurnBodyBytes)
	}
	if !strings.Contains(string(raw), "[truncated]") {
		t.Errorf("expected truncation marker in oversized turn")
	}

	// In-memory recent buffer must mirror the sanitized form.
	recent := s.Recent()
	if len(recent) != 1 {
		t.Fatalf("recent len = %d, want 1", len(recent))
	}
	if strings.Contains(recent[0].Response, "\x1b[") {
		t.Errorf("ANSI escape leaked into recent buffer")
	}
	if len(recent[0].Response) > types.DefaultMemorySettings().MaxTurnBodyBytes+32 {
		t.Errorf("recent[0].Response size %d exceeds cap", len(recent[0].Response))
	}
}

// TestReadTurnFileSanitizesLegacyFile locks the read-side contract:
// a pre-existing oversized turn file (written by a version of codrax
// that didn't sanitize on write) must be truncated when loadOrphanRecent
// resurrects it, so it cannot be re-fed to the summarizer at full size.
func TestReadTurnFileSanitizesLegacyFile(t *testing.T) {
	dir := t.TempDir()
	turnsDir := filepath.Join(dir, "turns")
	if err := os.MkdirAll(turnsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Hand-craft a legacy oversized file bypassing Append's sanitizer.
	// Exercises the path loadOrphanRecent → readTurnFile takes for an
	// orphan from a crashed previous session.
	body := strings.Repeat("\x1b[38;5;252mX\x1b[0m", 200*1024)
	content := "# legacy\n\n_2026-04-12T00:00:00Z_\n\n## Request\n\nq\n\n## Response\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(turnsDir, "legacy.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	recent := s.Recent()
	if len(recent) != 1 {
		t.Fatalf("recent len = %d, want 1", len(recent))
	}
	if strings.Contains(recent[0].Response, "\x1b[") {
		t.Errorf("legacy ANSI escape survived readTurnFile")
	}
	if len(recent[0].Response) > types.DefaultMemorySettings().MaxTurnBodyBytes+32 {
		t.Errorf("legacy Response size %d exceeds cap %d", len(recent[0].Response), types.DefaultMemorySettings().MaxTurnBodyBytes)
	}
}

// TestBuildContextSummaryOnly locks the BuildContext contract: even
// when many compacted entries match the same keyword and each on-disk
// turn file is 1 MB, BuildContext emits ONLY the LLM-generated Summary
// line (one bullet per match, capped by MaxBuildContextMatches). Full
// turn text stays on disk — never in the prompt.
func TestBuildContextSummaryOnly(t *testing.T) {
	dir := t.TempDir()
	turnsDir := filepath.Join(dir, "turns")
	if err := os.MkdirAll(turnsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Hand-craft a MEMORY.md with 10 entries all keyed on "pipeline".
	var idx strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&idx, "---\nid: legacy-%d\ntopic: %q\nkeywords: [pipeline]\nfull_ref: turns/legacy-%d.md\nsummary: s\n---\n%s\n\n",
			i, fmt.Sprintf("topic %d", i), i, "short summary")
		// Hand-write a 1 MB turn file, bypassing sanitizer. If
		// BuildContext ever regresses to inlining full turns, the
		// assembled context will explode past the size assertion.
		big := strings.Repeat("Z", 1024*1024)
		content := fmt.Sprintf("# legacy-%d\n\n_2026-04-12T00:00:00Z_\n\n## Request\n\nq\n\n## Response\n\n%s\n", i, big)
		if err := os.WriteFile(filepath.Join(turnsDir, fmt.Sprintf("legacy-%d.md", i)), []byte(content), 0o644); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(idx.String()), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()
	if got := len(s.Index()); got != 10 {
		t.Fatalf("index len = %d, want 10", got)
	}

	ctx := s.BuildContext("how many pipeline stages are there")
	// Summary-only means the assembled block is tiny regardless of
	// how many 1 MB turn files sit on disk. 2 KB is generous headroom
	// for the 3-bullet header + topic lines.
	if len(ctx) > 2*1024 {
		t.Errorf("BuildContext output size %d exceeds summary-only budget", len(ctx))
	}
	// Must include exactly MaxBuildContextMatches=3 bullets.
	if n := strings.Count(ctx, "**topic "); n != 3 {
		t.Errorf("emitted %d topic bullets, want 3", n)
	}
	// No full turn inlining ever.
	if strings.Contains(ctx, "Full turn:") {
		t.Errorf("BuildContext must not emit Full turn: block; got %q", ctx)
	}
	if strings.Contains(ctx, "ZZZZZZ") {
		t.Errorf("BuildContext leaked raw turn body; got %q", ctx)
	}
}

func TestBuildContextDoesNotBlockWhenMemoryIndexLockBusy(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()
	if err := s.Append(Turn{
		ID:       "t0",
		Request:  "kw0 architecture question",
		Response: "answer from prior turn",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	blocker, err := newFileLock(filepath.Join(dir, "MEMORY.md.lock"))
	if err != nil {
		t.Fatalf("newFileLock: %v", err)
	}
	if err := blocker.lock(); err != nil {
		_ = blocker.close()
		t.Fatalf("lock blocker: %v", err)
	}
	defer blocker.close()

	done := make(chan string, 1)
	go func() {
		done <- s.BuildContext("kw0 follow-up")
	}()

	select {
	case got := <-done:
		if !strings.Contains(got, "answer from prior turn") {
			t.Fatalf("BuildContext should use cached recent turns while index lock is busy, got %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("BuildContext blocked on a busy MEMORY.md lock; REPL foreground turns must remain responsive")
	}
}

func TestCachedCountsDoesNotBlockWhenMemoryIndexLockBusy(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()
	if err := s.Append(Turn{
		ID:       "t0",
		Request:  "kw0 architecture question",
		Response: "answer from prior turn",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	blocker, err := newFileLock(filepath.Join(dir, "MEMORY.md.lock"))
	if err != nil {
		t.Fatalf("newFileLock: %v", err)
	}
	if err := blocker.lock(); err != nil {
		_ = blocker.close()
		t.Fatalf("lock blocker: %v", err)
	}
	defer blocker.close()

	done := make(chan struct {
		recent int
		index  int
	}, 1)
	go func() {
		recent, index := s.CachedCounts()
		done <- struct {
			recent int
			index  int
		}{recent: recent, index: index}
	}()

	select {
	case got := <-done:
		if got.recent != 1 || got.index != 0 {
			t.Fatalf("CachedCounts = (%d,%d), want (1,0)", got.recent, got.index)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("CachedCounts blocked on a busy MEMORY.md lock; REPL foreground hints must remain responsive")
	}
}

func TestClearRemovesEverything(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()
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
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()
	for i := 0; i < 8; i++ {
		_ = s.Append(Turn{ID: fmt.Sprintf("t%d", i), Request: fmt.Sprintf("kw%d q", i)})
	}
	s.compactWG.Wait()
	// Re-open and ensure the index reloads.
	s2, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
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
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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
	s2, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

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
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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
	s2, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
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

	a, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore a: %v", err)
	}
	defer a.Close()

	// Just one Store → no peers.
	if got, err := a.LivePeerCount(); err != nil || got != 0 {
		t.Errorf("alone count: got (%d, %v), want (0, nil)", got, err)
	}

	b, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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
	c, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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

			s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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
	fresh, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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

// blockingSummarizer lets a test pause the summarizer inside
// Summarize, snapshot concurrent-call count, and release it later.
// Used to prove Append is non-blocking (compaction runs async) and
// that at most one goroutine is in flight at a time (single-flight
// latch).
type blockingSummarizer struct {
	entered chan struct{} // sends once per Summarize entry
	release chan struct{} // closed by the test to unblock all entries

	mu            sync.Mutex
	concurrent    int // current in-flight count
	maxConcurrent int // peak in-flight count across the run
	seen          []string
}

func (b *blockingSummarizer) Summarize(_ context.Context, t Turn) (IndexEntry, error) {
	b.mu.Lock()
	b.concurrent++
	if b.concurrent > b.maxConcurrent {
		b.maxConcurrent = b.concurrent
	}
	b.seen = append(b.seen, t.ID)
	b.mu.Unlock()

	b.entered <- struct{}{}
	<-b.release

	b.mu.Lock()
	b.concurrent--
	b.mu.Unlock()
	return IndexEntry{
		ID:       t.ID,
		Topic:    "topic-" + t.ID,
		Keywords: []string{strings.Fields(t.Request)[0]},
		Summary:  "summary of " + t.Request,
	}, nil
}

// TestAppendIsNonBlocking proves C3 invariant 1: Append returns
// before the backing Summarize completes. A synchronous
// implementation would deadlock on this test because Append would
// block waiting on b.release, which is only closed after the test
// reads past the blocked Append.
func TestAppendIsNonBlocking(t *testing.T) {
	dir := t.TempDir()
	b := &blockingSummarizer{
		entered: make(chan struct{}, 16),
		release: make(chan struct{}),
	}
	s, err := NewStore(dir, b, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer func() {
		// Close waits on WG; unblock the pending summarize first
		// so Close doesn't hang.
		close(b.release)
		s.Close()
	}()

	// Fill past maxRecent=6 to trigger compaction.
	for i := 0; i < 7; i++ {
		if err := s.Append(Turn{
			ID:       fmt.Sprintf("t%d", i),
			Request:  fmt.Sprintf("kw%d question", i),
			Response: fmt.Sprintf("answer %d", i),
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// The background goroutine must have entered Summarize — proves
	// scheduleCompact actually spawned a worker and that Append
	// returned without waiting for it.
	select {
	case <-b.entered:
	case <-time.After(2 * time.Second):
		t.Fatalf("Summarize was not entered asynchronously within 2s; Append likely ran synchronously")
	}

	// At this point Summarize is still blocked inside release, but
	// all 7 Appends already returned. The recent buffer still carries
	// the oldest turn (pop happens only after Summarize returns).
	if got := len(s.Recent()); got != 7 {
		t.Errorf("pre-release recent len = %d, want 7 (compaction still blocked)", got)
	}
	if got := len(s.Index()); got != 0 {
		t.Errorf("pre-release index len = %d, want 0 (compaction still blocked)", got)
	}
}

// TestCompactionIsSingleFlight proves C3 invariant 2: no matter how
// many Appends push the buffer over budget, at most one summarizer
// call is in flight at a time (the compactInFlight latch prevents
// scheduleCompact from firing a second goroutine while one is
// already running; the worker's internal re-check loop still picks
// up Appends that arrived mid-run).
func TestCompactionIsSingleFlight(t *testing.T) {
	dir := t.TempDir()
	b := &blockingSummarizer{
		entered: make(chan struct{}, 64),
		release: make(chan struct{}), // unbuffered: one send = one wake
	}
	s, err := NewStore(dir, b, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Push enough turns to require multiple compactions. Default
	// maxRecent=6 so 10 appends => 4 expected compactions.
	const N = 10
	expectCompactions := N - s.maxRecent
	for i := 0; i < N; i++ {
		if err := s.Append(Turn{
			ID:       fmt.Sprintf("t%d", i),
			Request:  fmt.Sprintf("kw%d question", i),
			Response: fmt.Sprintf("answer %d", i),
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// Drain each expected compaction: wait for entry, assert only one
	// is in-flight, release it, loop. An unbuffered release channel
	// means b.release <- struct{}{} blocks until Summarize's <-release
	// is ready, which is the correct pairing.
	for i := 0; i < expectCompactions; i++ {
		select {
		case <-b.entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for compaction %d of %d", i+1, expectCompactions)
		}
		b.mu.Lock()
		cur := b.concurrent
		b.mu.Unlock()
		if cur != 1 {
			t.Errorf("expected 1 in-flight summarize at entry %d, got %d", i, cur)
		}
		select {
		case b.release <- struct{}{}:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out releasing compaction %d", i+1)
		}
	}
	// Belt-and-suspenders for the case where the worker makes one
	// more loop iteration than expected: close so any lingering
	// <-b.release returns immediately.
	close(b.release)
	s.Close()

	b.mu.Lock()
	peak := b.maxConcurrent
	seen := len(b.seen)
	b.mu.Unlock()
	if peak > 1 {
		t.Errorf("maxConcurrent = %d, want 1 (single-flight latch broken)", peak)
	}
	if seen < expectCompactions {
		t.Errorf("seen=%d summarizer invocations, want ≥ %d", seen, expectCompactions)
	}
}

// recordingSummarizer remembers every turn it was asked to summarize
// so the WasError short-circuit can be proven by absence.
type recordingSummarizer struct {
	mu   sync.Mutex
	seen []string // Turn IDs the summarizer was invoked on
}

func (r *recordingSummarizer) Summarize(_ context.Context, t Turn) (IndexEntry, error) {
	r.mu.Lock()
	r.seen = append(r.seen, t.ID)
	r.mu.Unlock()
	return IndexEntry{
		ID:       t.ID,
		Topic:    "topic-" + t.ID,
		Keywords: []string{strings.Fields(t.Request)[0]},
		Summary:  "LLM summary of " + t.Request,
	}, nil
}

// TestCompactOldestBypassesSummarizerForErrorTurn locks the #4 fix
// from the REPL-equivalence follow-up list. An error-laden turn must
// never reach the LLM summarizer, otherwise the summarizer produces
// natural-language descriptions ("hit a 5-visit oscillation") that
// evade the read-side sanitizeErrorResponse phrase list. The
// compactOldest short-circuit synthesizes a deterministic WasError
// entry instead.
func TestCompactOldestBypassesSummarizerForErrorTurn(t *testing.T) {
	dir := t.TempDir()
	rec := &recordingSummarizer{}
	s, err := NewStore(dir, rec, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Append 7 turns — turn 0 is error-laden, the others are clean.
	// With maxRecent=6, turn 0 is compacted first.
	_ = s.Append(Turn{
		ID:       "t0",
		Request:  "how does config loader work",
		Response: "error: task 3e5d94c4 stuck at stage explore after 5 visits",
	})
	for i := 1; i < 7; i++ {
		_ = s.Append(Turn{
			ID:       fmt.Sprintf("t%d", i),
			Request:  fmt.Sprintf("kw%d clean question", i),
			Response: fmt.Sprintf("clean answer %d", i),
		})
	}
	s.compactWG.Wait()

	idx := s.Index()
	if len(idx) != 1 {
		t.Fatalf("index len = %d, want 1 (only t0 should be compacted)", len(idx))
	}
	entry := idx[0]
	if entry.ID != "t0" {
		t.Errorf("compacted entry ID = %q, want t0", entry.ID)
	}
	if !entry.WasError {
		t.Errorf("WasError should be true for error-laden turn; entry=%+v", entry)
	}
	if entry.Summary != errorResponsePlaceholder {
		t.Errorf("Summary = %q, want placeholder", entry.Summary)
	}
	if len(entry.Keywords) != 0 {
		t.Errorf("Keywords should be empty (so matchIndex never surfaces the entry); got %v", entry.Keywords)
	}

	// The summarizer must NOT have been called for t0.
	rec.mu.Lock()
	seen := append([]string(nil), rec.seen...)
	rec.mu.Unlock()
	for _, id := range seen {
		if id == "t0" {
			t.Errorf("summarizer was called on error turn t0 (bypass failed); seen=%v", seen)
		}
	}
}

// TestIndexEntryWasErrorRoundTrip locks the on-disk schema: when an
// entry with WasError=true is appended to MEMORY.md, a fresh Store
// parsing MEMORY.md back must recover the flag. Pre-schema entries
// (no was_error line) must default to false.
func TestIndexEntryWasErrorRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rec := &recordingSummarizer{}
	s, err := NewStore(dir, rec, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Force a WasError compaction via an error-laden oldest turn.
	_ = s.Append(Turn{
		ID:       "err-1",
		Request:  "broken question",
		Response: "error: OpenAI 400 context_length_exceeded",
	})
	// Pad the buffer to trigger compaction.
	for i := 0; i < 7; i++ {
		_ = s.Append(Turn{
			ID:       fmt.Sprintf("clean-%d", i),
			Request:  fmt.Sprintf("word%d clean", i),
			Response: fmt.Sprintf("answer %d", i),
		})
	}
	s.compactWG.Wait()

	// Sanity: MEMORY.md has the was_error line.
	data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if !strings.Contains(string(data), "was_error: true") {
		t.Errorf("MEMORY.md missing was_error marker; got %q", string(data))
	}
	s.Close()

	// Fresh Store parses it back.
	s2, err := NewStore(dir, rec, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore reload: %v", err)
	}
	defer s2.Close()
	idx := s2.Index()
	var found *IndexEntry
	for i := range idx {
		if idx[i].ID == "err-1" {
			found = &idx[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("err-1 entry not found after reload; idx=%+v", idx)
	}
	if !found.WasError {
		t.Errorf("WasError not recovered from disk; got %+v", *found)
	}

	// Pre-schema entry (no was_error line) → WasError stays false.
	for _, e := range idx {
		if e.ID != "err-1" && e.WasError {
			t.Errorf("clean entry should have WasError=false; got %+v", e)
		}
	}
}

// TestBuildContextSkipsWasErrorEntry is the defense-in-depth guard:
// even if a WasError entry somehow had non-empty keywords (hand-edited
// MEMORY.md, future match-rule change), BuildContext must NOT emit it
// into the prior-conversation prompt block.
func TestBuildContextSkipsWasErrorEntry(t *testing.T) {
	dir := t.TempDir()
	// Hand-craft a MEMORY.md with a WasError entry that has keywords
	// matching a query token. The in-process parser + BuildContext
	// must still refuse to surface it.
	memPath := filepath.Join(dir, "MEMORY.md")
	entry := "" +
		"---\n" +
		"id: tainted\n" +
		`topic: "bad"` + "\n" +
		"keywords: [foo, bar]\n" +
		"full_ref: turns/tainted.md\n" +
		"was_error: true\n" +
		"---\n" +
		"LLM-written error description about hit a 5-visit oscillation\n\n"
	if err := os.WriteFile(memPath, []byte(entry), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}
	// Provide a full_ref target so BuildContext's ReadFile would
	// succeed if the guard ever failed.
	if err := os.MkdirAll(filepath.Join(dir, "turns"), 0o755); err != nil {
		t.Fatalf("mkdir turns: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "turns", "tainted.md"),
		[]byte("# tainted\n\npolluting raw turn body\n"), 0o644); err != nil {
		t.Fatalf("write tainted turn: %v", err)
	}

	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	out := s.BuildContext("tell me about foo")
	if strings.Contains(out, "LLM-written error description") {
		t.Errorf("WasError entry Summary leaked into BuildContext: %q", out)
	}
	if strings.Contains(out, "polluting raw turn body") {
		t.Errorf("WasError entry full_ref body leaked into BuildContext: %q", out)
	}
	if strings.Contains(out, "tainted") {
		t.Errorf("WasError entry id/topic leaked into BuildContext: %q", out)
	}
}
