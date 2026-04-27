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

// streamingStubResponder implements streamingChitchatResponder so
// tests can exercise the streaming dispatch branch. Delta callbacks
// fire with the canned chunks in order, mimicking an SSE-driven
// adapter. The final reply returned is the concatenation of chunks
// so the test can assert both the streamed pieces and the final
// persisted reply without divergence.
type streamingStubResponder struct {
	chunks []string
	err    error

	gotLine  string
	gotPrior string
	deltas   []string
}

func (s *streamingStubResponder) Respond(userLine, prior string) (string, error) {
	// Kept for interface completeness — the REPL should prefer the
	// streaming branch, so any Respond call here is a bug.
	s.gotLine = userLine
	s.gotPrior = prior
	return strings.Join(s.chunks, ""), s.err
}

func (s *streamingStubResponder) RespondStream(userLine, prior string, onDelta func(string)) (string, error) {
	s.gotLine = userLine
	s.gotPrior = prior
	for _, c := range s.chunks {
		s.deltas = append(s.deltas, c)
		if onDelta != nil {
			onDelta(c)
		}
	}
	if s.err != nil {
		return "", s.err
	}
	return strings.Join(s.chunks, ""), nil
}

// TestChitchat_StreamingResponderUsesStreamBranch verifies that a
// responder implementing streamingChitchatResponder still takes the
// sync fallback when no renderer is wired (tests run with renderer
// nil to keep stdout deterministic). The stream branch only lights
// up in interactive TTY mode; this test locks the nil-renderer
// fallback so scripted output stays byte-stable.
//
// This test also guards against an accidental regression where the
// streaming branch leaks through to non-TTY callers and writes
// pterm escape codes into stdout buffers captured by CI.
func TestChitchat_StreamingResponderUsesStreamBranch(t *testing.T) {
	resp := &streamingStubResponder{chunks: []string{"hello", ", ", "world"}}
	r, store, out := newChitchatTestREPL(t, resp, "/chat hi there\n/exit\n")
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if resp.gotLine != "hi there" {
		t.Errorf("responder userLine: got %q, want %q", resp.gotLine, "hi there")
	}
	// Renderer is nil in tests → the dispatch path falls back to the
	// sync Respond, so deltas are NOT exercised here. The responder
	// still returns the joined reply via Respond, and the REPL
	// persists it to memory.
	if len(resp.deltas) != 0 {
		t.Errorf("deltas must stay empty without TTY renderer; got %v", resp.deltas)
	}
	if !strings.Contains(out.String(), "hello, world") {
		t.Errorf("final reply missing; out=%q", out.String())
	}
	if n := len(store.Recent()); n != 1 {
		t.Fatalf("memory turns: got %d, want 1", n)
	}
	if got := store.Recent()[0].Response; got != "hello, world" {
		t.Errorf("memory Response: got %q, want %q", got, "hello, world")
	}
}

// stubClassifier records every call and returns a canned decision
// or error. Lets tests drive the classifier gate without an LLM.
type stubClassifier struct {
	calls       []string
	isChitchat  bool
	err         error
}

func (c *stubClassifier) Classify(line string) (bool, error) {
	c.calls = append(c.calls, line)
	return c.isChitchat, c.err
}

// TestChitchatClassifier_ChitchatDecisionRoutesToResponder verifies
// the gate's primary job: when the classifier says chitchat, the
// turn goes to the responder and never to the runner.
func TestChitchatClassifier_ChitchatDecisionRoutesToResponder(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	runner := &logAwareRunner{}
	resp := &stubResponder{reply: "hello friend"}
	classifier := &stubClassifier{isChitchat: true}

	out := &bytes.Buffer{}
	r := New(Config{
		Runner:             runner,
		Store:              store,
		Render:             renderNothing,
		RepoRoot:           ".",
		Branch:             "main",
		In:                 strings.NewReader("hi there\n/exit\n"),
		Out:                out,
		Prompt:             ">",
		PromptCont:         ".",
		Banner:             "test-banner",
		ChitchatResponder:  resp,
		ChitchatClassifier: classifier,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(classifier.calls); n != 1 {
		t.Fatalf("classifier calls: got %d, want 1", n)
	}
	if got := classifier.calls[0]; got != "hi there" {
		t.Errorf("classifier line: got %q, want %q", got, "hi there")
	}
	if n := len(resp.calls); n != 1 {
		t.Fatalf("responder calls: got %d, want 1 (classifier routed to chitchat)", n)
	}
	if n := len(runner.seenLogs); n != 0 {
		t.Errorf("runner must not fire when classifier routes to chitchat; seenLogs=%d", n)
	}
}

// TestChitchatClassifier_RepoQuestionFallsThroughToRunner verifies
// the other primary branch: classifier says repo_question, the turn
// takes the normal pipeline path and the responder is untouched.
func TestChitchatClassifier_RepoQuestionFallsThroughToRunner(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	runner := &logAwareRunner{}
	resp := &stubResponder{reply: "should-not-appear"}
	classifier := &stubClassifier{isChitchat: false}

	out := &bytes.Buffer{}
	r := New(Config{
		Runner:             runner,
		Store:              store,
		Render:             renderNothing,
		RepoRoot:           ".",
		Branch:             "main",
		In:                 strings.NewReader("how does emit_evidence work\n/exit\n"),
		Out:                out,
		Prompt:             ">",
		PromptCont:         ".",
		Banner:             "test-banner",
		ChitchatResponder:  resp,
		ChitchatClassifier: classifier,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(classifier.calls); n != 1 {
		t.Fatalf("classifier calls: got %d, want 1", n)
	}
	if n := len(resp.calls); n != 0 {
		t.Errorf("responder must not fire on repo_question; calls=%d", n)
	}
	if n := len(runner.seenLogs); n != 1 {
		t.Errorf("runner must fire on repo_question; seenLogs=%d, want 1", n)
	}
}

// TestChitchatClassifier_ErrorFallsThroughToRunner verifies the
// fail-safe: any classifier error routes the turn to the pipeline,
// not to the chit-chat responder. A broken classifier must not
// silently misroute real code questions.
func TestChitchatClassifier_ErrorFallsThroughToRunner(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	runner := &logAwareRunner{}
	resp := &stubResponder{reply: "should-not-appear"}
	classifier := &stubClassifier{err: errors.New("upstream timeout")}

	out := &bytes.Buffer{}
	r := New(Config{
		Runner:             runner,
		Store:              store,
		Render:             renderNothing,
		RepoRoot:           ".",
		Branch:             "main",
		In:                 strings.NewReader("something\n/exit\n"),
		Out:                out,
		Prompt:             ">",
		PromptCont:         ".",
		Banner:             "test-banner",
		ChitchatResponder:  resp,
		ChitchatClassifier: classifier,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(classifier.calls); n != 1 {
		t.Fatalf("classifier calls: got %d, want 1", n)
	}
	if n := len(resp.calls); n != 0 {
		t.Errorf("responder must not fire on classifier error; calls=%d", n)
	}
	if n := len(runner.seenLogs); n != 1 {
		t.Errorf("runner must fire on classifier error (fail-safe); seenLogs=%d", n)
	}
}

// TestChitchatClassifier_SkipsWhenAttachedLogPresent verifies the
// explicit log skip rule: when the user has attached a runtime log
// via /log, classifier is bypassed because a log is strong evidence
// of a code question. Saves an LLM call and avoids classifying a
// turn where the user's intent is already unambiguous.
func TestChitchatClassifier_SkipsWhenAttachedLogPresent(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	runner := &logAwareRunner{}
	runner.SetAttachedLog("panic: boom")

	resp := &stubResponder{reply: "should-not-appear"}
	classifier := &stubClassifier{isChitchat: true}

	out := &bytes.Buffer{}
	r := New(Config{
		Runner:             runner,
		Store:              store,
		Render:             renderNothing,
		RepoRoot:           ".",
		Branch:             "main",
		In:                 strings.NewReader("what happened\n/exit\n"),
		Out:                out,
		Prompt:             ">",
		PromptCont:         ".",
		Banner:             "test-banner",
		ChitchatResponder:  resp,
		ChitchatClassifier: classifier,
	})
	// Seed the REPL's attachedLog state to mirror /log or --log CLI.
	r.attachedLog = "panic: boom"
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(classifier.calls); n != 0 {
		t.Errorf("classifier must be skipped when attached log present; calls=%d", n)
	}
	if n := len(runner.seenLogs); n != 1 {
		t.Errorf("runner must fire (pipeline path); seenLogs=%d, want 1", n)
	}
}

// TestChitchatClassifier_SkipsWhenInPlanMode verifies that an
// explicit `/mode plan|apply|verify` selection bypasses the
// classifier. Plan mode is the user telling the REPL "drive the
// write pipeline"; the classifier's "no repo context" verdict for a
// fresh-code request like "用Python写一个猜数字游戏" must not
// reroute that turn to chit-chat. Regression for a real bug observed
// in session 36 where four consecutive plan-mode turns were silently
// answered by the chit-chat responder.
func TestChitchatClassifier_SkipsWhenInPlanMode(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	runner := &logAwareRunner{}
	resp := &stubResponder{reply: "should-not-appear"}
	classifier := &stubClassifier{isChitchat: true}

	out := &bytes.Buffer{}
	r := New(Config{
		Runner:             runner,
		Store:              store,
		Render:             renderNothing,
		RepoRoot:           ".",
		Branch:             "main",
		In:                 strings.NewReader("/mode plan\nwrite a guessing game in python\n/exit\n"),
		Out:                out,
		Prompt:             ">",
		PromptCont:         ".",
		Banner:             "test-banner",
		ChitchatResponder:  resp,
		ChitchatClassifier: classifier,
		WriteEnabled:       true,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if n := len(classifier.calls); n != 0 {
		t.Errorf("classifier must be skipped in plan mode; calls=%d", n)
	}
	if n := len(resp.calls); n != 0 {
		t.Errorf("responder must not fire in plan mode; calls=%d", n)
	}
	if n := len(runner.seenLogs); n != 1 {
		t.Errorf("runner must fire on plan-mode turn; seenLogs=%d, want 1", n)
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
