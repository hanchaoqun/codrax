package repl

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

// stubResponder records every call and returns a canned reply or error.
// Lets chit-chat tests assert on what the REPL forwarded to the
// responder without needing an actual LLM adapter.
type stubResponder struct {
	calls   []stubResponderCall
	reply   string
	err     error
}

type stubResponderCall struct {
	userLine string
	prior    string
}

func (s *stubResponder) Respond(userLine, prior string) (string, error) {
	s.calls = append(s.calls, stubResponderCall{userLine: userLine, prior: prior})
	return s.reply, s.err
}

func newChitchatTestREPL(t *testing.T, responder ChitchatResponder, input string) (*REPL, *memory.Store, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	out := &bytes.Buffer{}
	r := New(Config{
		Runner:            stubRunner{},
		Store:             store,
		Render:            renderNothing,
		RepoRoot:          ".",
		Branch:            "main",
		In:                strings.NewReader(input),
		Out:               out,
		Prompt:            ">",
		PromptCont:        ".",
		Banner:            "test-banner",
		ChitchatResponder: responder,
	})
	return r, store, out
}

// TestChitchat_DispatchesResponderAndPersistsMemory verifies that
// `/chat hi there` calls the responder with the trimmed user line,
// renders the reply, and appends a Turn to memory.
func TestChitchat_DispatchesResponderAndPersistsMemory(t *testing.T) {
	resp := &stubResponder{reply: "hello — how can I help?"}
	r, store, out := newChitchatTestREPL(t, resp, "/chat hi there\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(resp.calls); n != 1 {
		t.Fatalf("responder calls: got %d, want 1", n)
	}
	if got := resp.calls[0].userLine; got != "hi there" {
		t.Errorf("responder userLine: got %q, want %q", got, "hi there")
	}
	if !strings.Contains(out.String(), "hello — how can I help?") {
		t.Errorf("reply not rendered; out=%q", out.String())
	}
	if n := len(store.Recent()); n != 1 {
		t.Fatalf("memory turns: got %d, want 1", n)
	}
	turn := store.Recent()[0]
	if turn.Request != "hi there" {
		t.Errorf("memory Request: got %q, want %q", turn.Request, "hi there")
	}
	if turn.Response != "hello — how can I help?" {
		t.Errorf("memory Response: got %q, want the reply", turn.Response)
	}
}

// TestChitchat_EmptyArgsPrintsUsageWithoutResponder verifies that
// bare `/chat` (no message) prints the usage hint, does NOT invoke
// the responder, and does NOT write to memory — mirroring the
// "invalid subcommand" semantics of the other slash commands.
func TestChitchat_EmptyArgsPrintsUsageWithoutResponder(t *testing.T) {
	resp := &stubResponder{reply: "should-not-appear"}
	r, store, out := newChitchatTestREPL(t, resp, "/chat\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(resp.calls); n != 0 {
		t.Errorf("responder must not be called on empty /chat; calls=%d", n)
	}
	if n := len(store.Recent()); n != 0 {
		t.Errorf("memory must be untouched on empty /chat; turns=%d", n)
	}
	if !strings.Contains(out.String(), "/chat <message>") {
		t.Errorf("usage hint not printed; out=%q", out.String())
	}
}

// TestChitchat_ResponderErrorDoesNotWriteMemory verifies the fail-
// safe: a responder error prints a visible warning but does NOT
// persist a Turn. Polluting prior-conversation context with
// placeholder failure text would encourage future responders to
// hallucinate around phantom turns; the cleaner contract is
// "this chat turn failed silently from memory's point of view".
func TestChitchat_ResponderErrorDoesNotWriteMemory(t *testing.T) {
	resp := &stubResponder{err: errors.New("upstream timeout")}
	r, store, out := newChitchatTestREPL(t, resp, "/chat hi\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(resp.calls); n != 1 {
		t.Fatalf("responder calls: got %d, want 1", n)
	}
	if n := len(store.Recent()); n != 0 {
		t.Errorf("memory must stay empty after responder error; turns=%d", n)
	}
	if !strings.Contains(out.String(), "chat failed") {
		t.Errorf("expected visible failure notice; out=%q", out.String())
	}
}

// TestChitchat_NilResponderPrintsNotConfigured verifies the disabled
// path: when ChitchatResponder is nil (operator set chitchat_enabled
// false), /chat prints a warning and no memory or runner activity
// happens.
func TestChitchat_NilResponderPrintsNotConfigured(t *testing.T) {
	r, store, out := newChitchatTestREPL(t, nil, "/chat hi\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(store.Recent()); n != 0 {
		t.Errorf("memory must stay empty when responder nil; turns=%d", n)
	}
	if !strings.Contains(out.String(), "chit-chat is disabled") {
		t.Errorf("expected disabled warning; out=%q", out.String())
	}
}

// TestChitchat_DoesNotTouchAttachedLogRunner verifies the sticky
// attached-log invariant: /chat does not call the Runner and does
// not clear / re-propagate the attachedLog. Uses logAwareRunner so
// each runner interaction is visible.
func TestChitchat_DoesNotTouchAttachedLogRunner(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	runner := &logAwareRunner{}
	runner.SetAttachedLog("panic: boom")

	resp := &stubResponder{reply: "noted"}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:            runner,
		Store:             store,
		Render:            renderNothing,
		RepoRoot:          ".",
		Branch:            "main",
		In:                strings.NewReader("/chat hi\n/exit\n"),
		Out:               out,
		Prompt:            ">",
		PromptCont:        ".",
		Banner:            "test-banner",
		ChitchatResponder: resp,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(runner.seenLogs); n != 0 {
		t.Errorf("/chat must not call Run(); seenLogs=%d", n)
	}
	// The one and only SetAttachedLog call is the test-setup seed; no
	// subsequent propagation on /chat.
	if n := len(runner.setCalls); n != 1 {
		t.Errorf("/chat must not re-set attached log; setCalls=%d", n)
	}
}
