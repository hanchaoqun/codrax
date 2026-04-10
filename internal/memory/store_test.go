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
