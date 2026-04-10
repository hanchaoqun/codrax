package repl

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stubRunner is a no-op Runner so the REPL can be exercised without
// pulling in the full orchestrator. /clear and /exit are slash
// commands and never call Run, so the runner is unused for these
// tests.
type stubRunner struct{}

func (stubRunner) Run(_, _, _ string) (*types.BusContext, error) {
	return &types.BusContext{}, nil
}

// stubSummarizer mirrors the one in internal/memory's tests but is
// duplicated here so the repl package's tests do not have to import
// internal test helpers from a sibling package (Go forbids that).
type stubSummarizer struct{}

func (stubSummarizer) Summarize(_ context.Context, t memory.Turn) (memory.IndexEntry, error) {
	return memory.IndexEntry{
		ID:      t.ID,
		Topic:   "topic-" + t.ID,
		Summary: "summary",
	}, nil
}

func renderNothing(*types.BusContext) string { return "" }

func newTestREPL(store *memory.Store, in *strings.Reader, out *bytes.Buffer) *REPL {
	return New(Config{
		Runner:     stubRunner{},
		Store:      store,
		Render:     renderNothing,
		RepoRoot:   ".",
		Branch:     "main",
		In:         in,
		Out:        out,
		Prompt:     ">",
		PromptCont: ".",
		Banner:     "test-banner",
	})
}

// TestClearPromptDeclined verifies that /clear's confirmation step
// actually blocks the wipe when the user does not type 'y'. We
// scripted "/clear\nn\n/exit\n" through stdin and check that the
// store still has its turn afterwards.
func TestClearPromptDeclined(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.Append(memory.Turn{ID: "t1", Request: "hello", Response: "hi"}); err != nil {
		t.Fatalf("seed append: %v", err)
	}

	in := strings.NewReader("/clear\nn\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !strings.Contains(out.String(), "clear cancelled") {
		t.Errorf("expected 'clear cancelled' in output, got:\n%s", out.String())
	}
	if got := len(store.Recent()); got != 1 {
		t.Errorf("turn was wiped despite declining; recent=%d, want 1", got)
	}
}

// TestClearPromptAccepted verifies the wipe happens when the user
// types 'y'. The complementary test to TestClearPromptDeclined.
func TestClearPromptAccepted(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.Append(memory.Turn{ID: "t1", Request: "hello", Response: "hi"}); err != nil {
		t.Fatalf("seed append: %v", err)
	}

	in := strings.NewReader("/clear\ny\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !strings.Contains(out.String(), "conversation memory cleared") {
		t.Errorf("expected confirmation in output, got:\n%s", out.String())
	}
	if got := len(store.Recent()); got != 0 {
		t.Errorf("turn was not wiped after accepting; recent=%d, want 0", got)
	}
}

// TestClearPromptShowsPeerCount verifies that the warning text
// reflects the number of live peer Stores. With one peer alive in
// the same dir, the prompt must mention "1 other live codrax
// instance".
func TestClearPromptShowsPeerCount(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Spin up a peer Store on the same dir; its sidecar makes it
	// visible to LivePeerCount.
	peer, err := memory.NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("peer NewStore: %v", err)
	}
	defer peer.Close()

	in := strings.NewReader("/clear\nn\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !strings.Contains(out.String(), "1 other live codrax instance") {
		t.Errorf("expected peer-count warning, got:\n%s", out.String())
	}
}

// TestMultilineInput verifies that lines ending with \ are joined
// into a single multi-line request.
func TestMultilineInput(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("hello \\\nworld\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	if len(recent) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(recent))
	}
	if !strings.Contains(recent[0].Request, "hello") || !strings.Contains(recent[0].Request, "world") {
		t.Errorf("expected multiline request with both parts, got: %q", recent[0].Request)
	}
	if !strings.Contains(recent[0].Request, "\n") {
		t.Errorf("expected newline in multiline request, got: %q", recent[0].Request)
	}
	// Continuation prompt should appear in output.
	if !strings.Contains(out.String(), ".") {
		t.Errorf("expected continuation prompt in output")
	}
}

// TestMultilineThreeLines verifies continuation across three lines.
func TestMultilineThreeLines(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("aaa \\\nbbb \\\nccc\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	if len(recent) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(recent))
	}
	req := recent[0].Request
	if !strings.Contains(req, "aaa") || !strings.Contains(req, "bbb") || !strings.Contains(req, "ccc") {
		t.Errorf("expected all three parts in request, got: %q", req)
	}
	// Should have exactly 2 newlines (3 lines joined).
	if strings.Count(req, "\n") != 2 {
		t.Errorf("expected 2 newlines in request, got %d in: %q", strings.Count(req, "\n"), req)
	}
}
