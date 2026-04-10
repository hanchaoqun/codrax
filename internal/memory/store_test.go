package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// Simulated restart: drop s, open a fresh store on the same dir.
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
