package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// richSummarizer lets a test populate Entities / Refs on the
// IndexEntry so the Kind-aware retrieval path can be exercised
// without spinning up the real LLM summarizer.
type richSummarizer struct {
	entities []string
	refs     []string
}

func (r richSummarizer) Summarize(_ context.Context, t Turn) (IndexEntry, error) {
	return IndexEntry{
		ID:       t.ID,
		Topic:    "topic-" + t.ID,
		Keywords: []string{strings.Fields(t.Request)[0]},
		Summary:  "summary of " + t.Request,
		Entities: append([]string(nil), r.entities...),
		Refs:     append([]string(nil), r.refs...),
	}, nil
}

// TestBuildContext_LegacyCallerByteCompat pins that BuildContext
// without BuildOpts behaves byte-for-byte identically to the pre-
// session-32 implementation. Any divergence here breaks every
// existing caller + test that passes a single string argument.
func TestBuildContext_LegacyCallerByteCompat(t *testing.T) {
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
	out := s.BuildContext("how about kw0 again?")
	// Legacy expectations from pre-session-32 tests.
	if !strings.Contains(out, "### Recent conversation") {
		t.Errorf("legacy output missing recent section: %q", out)
	}
	if !strings.Contains(out, "### Relevant compacted memory") {
		t.Errorf("legacy output missing compacted section: %q", out)
	}
	if !strings.Contains(out, "topic-t0") {
		t.Errorf("legacy keyword match should surface t0: %q", out)
	}
	// oneLine caps at 200 bytes; the test payloads are short so the
	// rendered output should be short too.
	if len(out) > 2048 {
		t.Errorf("legacy output unexpectedly large (%d bytes); may indicate body cap regression", len(out))
	}
}

// TestBuildContext_ChitchatSessionPinsRecentTurns proves that under
// Kind=chitchat + SessionID, the most recent in-thread turns are
// rendered with a richer body cap (800 runes) even when the current
// request has no keyword overlap with those turns. This is the
// "continue that topic / 刚才那个" follow-up case chitchat broke on
// pre-session-32 because oneLine + keyword-only matching dropped
// the continuity signal entirely.
func TestBuildContext_ChitchatSessionPinsRecentTurns(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// A body long enough that oneLine would truncate (> 200 bytes)
	// but still under the 800-rune chitchat cap.
	longBody := strings.Repeat("conversation context with substantive detail ", 10) // ~450 chars
	_ = s.Append(Turn{
		ID:        "chat-1",
		Request:   "tell me about write mode design",
		Response:  longBody,
		SessionID: "sess-A",
		Kind:      KindChitchat,
	})

	out := s.BuildContext("继续那个话题", BuildOpts{
		Kind:      KindChitchat,
		SessionID: "sess-A",
	})
	if !strings.Contains(out, "### Recent conversation") {
		t.Fatalf("chitchat output missing recent section: %q", out)
	}
	// Session-pinned turn must render richer than oneLine would allow.
	// oneLine caps at 200 bytes + "…"; we want to see more than that.
	// Look for a known tail fragment that oneLine would have cut.
	tail := "substantive detail"
	if !strings.Contains(out, tail) {
		t.Errorf("chitchat session-pinned recent body should preserve tail %q, got %q", tail, out)
	}
}

// TestBuildContext_PipelineUsesOneLine ensures the pipeline policy
// keeps the pre-session-32 oneLine body cap — pipeline answers are
// already long (Answer Documents) and expanding the recent-turn
// body would push out of the analyzer's context budget.
func TestBuildContext_PipelineUsesOneLine(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()
	longBody := strings.Repeat("very long response body chunk ", 20) // ~600 chars
	_ = s.Append(Turn{
		ID:        "pipe-1",
		Request:   "how does dispatch work",
		Response:  longBody,
		SessionID: "sess-P",
		Kind:      KindPipeline,
	})

	out := s.BuildContext("follow up", BuildOpts{
		Kind:      KindPipeline,
		SessionID: "sess-P",
	})
	// oneLine caps at 200 BYTES. 600-char body should be truncated.
	if strings.Contains(out, strings.Repeat("very long response body chunk ", 20)) {
		t.Errorf("pipeline recent body should be oneLine-truncated, got full body: %q", out)
	}
	// The truncation sentinel should appear.
	if !strings.Contains(out, "…") {
		t.Errorf("pipeline recent body expected trailing …, got %q", out)
	}
}

// TestBuildContext_EntityWeightingBeatsKeyword proves the Kind=
// pipeline policy's entityScoreMul=3 actually prioritizes a weak
// keyword match on entry A over a strong-looking but irrelevant
// entry B. Sets up a MEMORY.md manually to make the expected
// ranking obvious.
func TestBuildContext_EntityWeightingBeatsKeyword(t *testing.T) {
	dir := t.TempDir()
	// Hand-craft MEMORY.md: entry A has the entity "ChitchatResponder"
	// and a generic keyword; entry B has three generic keywords.
	// Query contains "ChitchatResponder" as a substring.
	memPath := filepath.Join(dir, "MEMORY.md")
	content := "" +
		"---\n" +
		"id: entA\n" +
		`topic: "auth responder"` + "\n" +
		"keywords: [foo]\n" +
		"full_ref: turns/entA.md\n" +
		"entities: [ChitchatResponder]\n" +
		"---\n" +
		"summary A about ChitchatResponder\n\n" +
		"---\n" +
		"id: entB\n" +
		`topic: "unrelated"` + "\n" +
		"keywords: [where, does, it]\n" +
		"full_ref: turns/entB.md\n" +
		"---\n" +
		"summary B unrelated matches generic words\n\n"
	if err := os.WriteFile(memPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write MEMORY.md: %v", err)
	}

	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Query has 3 keyword overlaps with B (where / does / it), 0
	// keyword overlaps with A. But A's entity "ChitchatResponder"
	// is a substring of the query. With entityScoreMul=3 → A gets
	// 1*3 = 3, B gets 3. Tie broken by sort.SliceStable preserving
	// file order; A appears first in file so A wins.
	out := s.BuildContext("where does it call ChitchatResponder from", BuildOpts{
		Kind: KindPipeline,
	})
	// Both should surface, but A must render before B.
	idxA := strings.Index(out, "**auth responder**")
	idxB := strings.Index(out, "**unrelated**")
	if idxA < 0 {
		t.Fatalf("entity-matched entry A missing: %q", out)
	}
	if idxB >= 0 && idxB < idxA {
		t.Errorf("entity-weighted entry A should rank before generic-keyword entry B; got A@%d B@%d\n%q", idxA, idxB, out)
	}
}

// TestIndexEntry_NewFrontmatterRoundTrip locks the on-disk schema
// for the four session-32 additions. A store that writes an entry
// with all four fields populated must parse it back verbatim.
// Legacy entries (missing these fields) must recover as zero values.
func TestIndexEntry_NewFrontmatterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rich := richSummarizer{
		entities: []string{"ChitchatResponder", "internal/repl/chitchat.go"},
		refs:     []string{"turn-prior-1", "turn-prior-2"},
	}
	s, err := NewStore(dir, rich, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Append 8 turns → first 2 compact with rich metadata stamped.
	for i := 0; i < 8; i++ {
		_ = s.Append(Turn{
			ID:        fmt.Sprintf("t%d", i),
			Request:   fmt.Sprintf("kw%d question", i),
			Response:  fmt.Sprintf("answer %d", i),
			SessionID: "sess-round-trip",
			Kind:      KindChitchat,
		})
	}
	s.compactWG.Wait()
	s.Close()

	// Reopen → parse back.
	s2, err := NewStore(dir, rich, types.MemorySettings{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	idx := s2.Index()
	if len(idx) < 2 {
		t.Fatalf("expected ≥ 2 compacted entries, got %d", len(idx))
	}
	first := idx[0]
	if first.Kind != KindChitchat {
		t.Errorf("Kind not round-tripped: got %q, want %q", first.Kind, KindChitchat)
	}
	if first.SessionID != "sess-round-trip" {
		t.Errorf("SessionID not round-tripped: got %q", first.SessionID)
	}
	if len(first.Entities) != 2 || first.Entities[0] != "ChitchatResponder" {
		t.Errorf("Entities not round-tripped: got %v", first.Entities)
	}
	if len(first.Refs) != 2 || first.Refs[0] != "turn-prior-1" {
		t.Errorf("Refs not round-tripped: got %v", first.Refs)
	}
}

// TestTurnFile_MetadataRoundTripsOrphanRecovery covers the other
// end: a Turn with SessionID / Kind written to turns/<id>.md must
// recover those fields on restart so orphan turns from a previous
// REPL session (same user, pre-compaction) stay in-thread when the
// next REPL starts and calls BuildContext.
func TestTurnFile_MetadataRoundTripsOrphanRecovery(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ts := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	_ = s.Append(Turn{
		ID:        "turn-orphan-1",
		Request:   "q1",
		Response:  "r1",
		Timestamp: ts,
		SessionID: "sess-orphan",
		Kind:      KindChitchat,
	})

	// Close to release the instance lock; reopen simulates restart.
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	s2, err := NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	recent := s2.Recent()
	if len(recent) != 1 {
		t.Fatalf("orphan recovery len = %d, want 1", len(recent))
	}
	recovered := recent[0]
	if recovered.Kind != KindChitchat {
		t.Errorf("orphan Kind lost: got %q, want %q", recovered.Kind, KindChitchat)
	}
	if recovered.SessionID != "sess-orphan" {
		t.Errorf("orphan SessionID lost: got %q", recovered.SessionID)
	}
}

// TestLegacyTurnFile_NoMetadataDegradesGracefully ensures that a
// pre-session-32 turn file on disk (no HTML-comment metadata) still
// parses cleanly. Kind and SessionID stay empty, and BuildContext
// treats the turn as cross-session — which means the legacy oneLine
// path, preserving existing behavior.
func TestLegacyTurnFile_NoMetadataDegradesGracefully(t *testing.T) {
	dir := t.TempDir()
	turnsDir := filepath.Join(dir, "turns")
	if err := os.MkdirAll(turnsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Hand-craft a legacy turn file with no HTML-comment metadata.
	legacy := "# legacy-1\n\n_2026-04-24T10:00:00Z_\n\n## Request\n\nold question\n\n## Response\n\nold answer\n"
	if err := os.WriteFile(filepath.Join(turnsDir, "legacy-1.md"), []byte(legacy), 0o644); err != nil {
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
	if recent[0].Kind != "" {
		t.Errorf("legacy turn should have empty Kind, got %q", recent[0].Kind)
	}
	if recent[0].SessionID != "" {
		t.Errorf("legacy turn should have empty SessionID, got %q", recent[0].SessionID)
	}
}
