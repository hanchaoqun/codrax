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

// logAwareRunner captures every SetAttachedLog propagation plus the
// attachedLog value visible to Run, so tests can prove that REPL's
// one-shot auto-route semantics fire on the dispatch after a paste
// and only on that dispatch.
type logAwareRunner struct {
	setCalls []string // each SetAttachedLog argument, in call order
	seenLogs []string // attachedLog observed at Run() entry per dispatch
	curLog   string
}

func (r *logAwareRunner) Run(_, _, _ string) (*types.BusContext, error) {
	r.seenLogs = append(r.seenLogs, r.curLog)
	return &types.BusContext{}, nil
}
func (r *logAwareRunner) SetAttachedLog(s string) {
	r.setCalls = append(r.setCalls, s)
	r.curLog = s
}

// errorRunner simulates a pipeline that failed with a LastError
// — used to verify the REPL sanitizes what it persists to memory.
type errorRunner struct{ errText string }

func (r errorRunner) Run(_, _, _ string) (*types.BusContext, error) {
	bc := &types.BusContext{}
	bc.TaskState.LastError = r.errText
	bc.Mutable = types.NewMutableState("probe")
	return bc, nil
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
		// Pin language to English so test assertions on literal
		// English strings stay stable. Production zh-default
		// behaviour is exercised by the bilingual messages.go
		// helper tests.
		Language: "en",
	})
}

// TestClearPromptDeclined verifies that /clear's confirmation step
// actually blocks the wipe when the user does not type 'y'. We
// scripted "/clear\nn\n/exit\n" through stdin and check that the
// store still has its turn afterwards.
func TestClearPromptDeclined(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Spin up a peer Store on the same dir; its sidecar makes it
	// visible to LivePeerCount.
	peer, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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

// TestErrorResponseNotPersistedToMemory is the write-side lock for
// the 2026-04-12 REPL memory-pollution fix. When the pipeline ends
// with a non-empty TaskState.LastError, the REPL must NOT persist
// the full error-laden render into memory — it must persist a
// clean placeholder so subsequent turns' prior-conversation block
// doesn't leak historical failure text into the LLM's context.
func TestErrorResponseNotPersistedToMemory(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("some broken question\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:     errorRunner{errText: "task deadbeef stuck at stage explore after 5 visits"},
		Store:      store,
		Render:     renderErrorFromBus,
		RepoRoot:   ".",
		Branch:     "main",
		In:         in,
		Out:        out,
		Prompt:     ">",
		PromptCont: ".",
		Banner:     "test-banner",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	recent := store.Recent()
	if len(recent) != 1 {
		t.Fatalf("expected 1 turn persisted, got %d", len(recent))
	}
	stored := recent[0].Response
	// The raw error details must not leak into the stored response.
	if strings.Contains(stored, "deadbeef") {
		t.Errorf("task UUID leaked into memory: %q", stored)
	}
	if strings.Contains(stored, "stuck at stage") {
		t.Errorf("raw error phrase leaked into memory: %q", stored)
	}
	// The clean placeholder should be what got stored.
	if !strings.Contains(stored, "previous attempt ended in error") {
		t.Errorf("expected placeholder in stored response, got: %q", stored)
	}
}

// renderErrorFromBus mimics the real renderer's behavior for an
// errored BusContext: prepends "error: <LastError>" to whatever
// answer text exists. Kept separate from renderNothing so the
// error-response test can exercise the render→recordTurn path.
func renderErrorFromBus(bc *types.BusContext) string {
	if bc == nil {
		return ""
	}
	if bc.TaskState.LastError != "" {
		return "\n  error: " + bc.TaskState.LastError + "\n"
	}
	return ""
}

// TestPasteSlashCapturesToPending verifies the /paste command
// collects lines via bufio until `/end`, and stashes the capture in
// r.pendingPaste ready for the next interactive readInput to
// consume. Scripted mode skips the bubbletea seeding step but the
// capture path is identical.
func TestPasteSlashCapturesToPending(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("/paste\nfunc foo() {\n    return 42\n}\n/end\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	want := "func foo() {\n    return 42\n}"
	if r.pendingPaste != want {
		t.Errorf("pendingPaste = %q, want %q", r.pendingPaste, want)
	}
	if !strings.Contains(out.String(), "captured 3 lines") {
		t.Errorf("expected 'captured 3 lines' in output, got:\n%s", out.String())
	}
}

// TestPasteSlashEmptyCaptureNoOp verifies that pressing /end
// immediately without content leaves pendingPaste empty.
func TestPasteSlashEmptyCaptureNoOp(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("/paste\n/end\n/exit\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if r.pendingPaste != "" {
		t.Errorf("pendingPaste should be empty on no-input capture, got %q", r.pendingPaste)
	}
	if !strings.Contains(out.String(), "no input captured") {
		t.Errorf("expected 'no input captured' in output, got:\n%s", out.String())
	}
}

// TestAutoRoutedLogIsOneShot verifies C5: a log body auto-detected
// inside a request is attached for that single dispatch and then
// cleared, so subsequent turns do not re-run log_triage against the
// same bytes. Explicit /log path stays sticky (covered separately).
func TestAutoRoutedLogIsOneShot(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Turn 1 embeds a 3-line timestamped log block that splitPastedLog
	// detects. Turn 2 is an unrelated follow-up. Line continuations
	// ("\\") let a single logical request span multiple input lines.
	input := "analyze this log \\\n" +
		"2026-04-21T14:54:37.362 INFO [x] dispatching agent=a skill=s \\\n" +
		"2026-04-21T14:54:56.671 INFO [x] dispatching agent=b skill=t \\\n" +
		"2026-04-21T14:56:00.822 INFO [x] dispatching agent=c skill=u\n" +
		"follow-up question\n" +
		"/exit\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}
	runner := &logAwareRunner{}
	r := New(Config{
		Runner:     runner,
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
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	// Two dispatches happened.
	if got := len(runner.seenLogs); got != 2 {
		t.Fatalf("expected 2 dispatches, got %d: %v", got, runner.seenLogs)
	}
	// Turn 1 saw the log at Run() entry.
	if runner.seenLogs[0] == "" {
		t.Errorf("turn 1 should have seen auto-routed log; got empty")
	}
	if !strings.Contains(runner.seenLogs[0], "14:54:37") {
		t.Errorf("turn 1 log missing expected timestamp; got %q", runner.seenLogs[0])
	}
	// Turn 2 saw an empty log — one-shot clear fired.
	if runner.seenLogs[1] != "" {
		t.Errorf("turn 2 should see no attached log after one-shot clear; got %q", runner.seenLogs[1])
	}
	// REPL internal state after exit: both flag and value cleared.
	if r.attachedLog != "" {
		t.Errorf("attachedLog not cleared after one-shot turn; got %q", r.attachedLog)
	}
	if r.attachedLogAutoRouted {
		t.Errorf("attachedLogAutoRouted should be false after clear")
	}
}

// TestExplicitLogStaysStickyAcrossTurns is the positive counterpart
// to TestAutoRoutedLogIsOneShot: an explicit /log paste-capture
// attachment must survive into subsequent dispatches so users can
// keep asking about the same panic from different angles.
func TestExplicitLogStaysStickyAcrossTurns(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// /log (paste mode) captures three log lines, then two questions
	// run back to back. Both dispatches must see the same attachedLog.
	input := "/log\n" +
		"2026-04-21T14:54:37.362 INFO [x] dispatching\n" +
		"2026-04-21T14:54:56.671 INFO [x] dispatching\n" +
		"2026-04-21T14:56:00.822 INFO [x] dispatching\n" +
		"/end\n" +
		"first question about the log\n" +
		"second question about the log\n" +
		"/exit\n"
	in := strings.NewReader(input)
	out := &bytes.Buffer{}
	runner := &logAwareRunner{}
	r := New(Config{
		Runner:     runner,
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
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if got := len(runner.seenLogs); got != 2 {
		t.Fatalf("expected 2 dispatches, got %d: %v", got, runner.seenLogs)
	}
	for i, got := range runner.seenLogs {
		if !strings.Contains(got, "14:54:37") {
			t.Errorf("turn %d should see sticky log with timestamp; got %q", i+1, got)
		}
	}
	if r.attachedLogAutoRouted {
		t.Errorf("explicit /log should never set attachedLogAutoRouted")
	}
	if r.attachedLog == "" {
		t.Errorf("attachedLog should still be set after /exit")
	}
}

// TestMultilineInput verifies that lines ending with \ are joined
// into a single multi-line request.
func TestMultilineInput(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
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

func TestBackslashQuitAliasHandledLocally(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("\\q\n")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if got := len(store.Recent()); got != 0 {
		t.Fatalf("\\q should exit before dispatch, persisted turns=%d", got)
	}
	if !strings.Contains(out.String(), "Goodbye!") {
		t.Errorf("expected goodbye output for \\q, got:\n%s", out.String())
	}
}

// TestVersionSlashCommand verifies the /version REPL command prints
// the build identifier passed in via Config and continues the loop.
// Also covers the /v alias and the legacy backslash form.
func TestVersionSlashCommand(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("/version\n/v\n\\version\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
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
		Version:    "1.2.3-test",
		BuildTime:  "2026-04-22T00:00:00Z",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	got := out.String()
	want := "codrax 1.2.3-test (built 2026-04-22T00:00:00Z)"
	if c := strings.Count(got, want); c != 3 {
		t.Errorf("expected /version output to appear 3 times (one per alias), got %d:\n%s", c, got)
	}
	if n := len(store.Recent()); n != 0 {
		t.Errorf("/version must not record a turn, recent=%d", n)
	}
}

// TestStickyTag_PropagatesToPasteAndLogPasteModes locks that capture-
// mode prompts (`/paste`, `/log paste`) prepend the sticky tag so a
// user typing into paste mode while in plan mode does not lose the
// [mode:plan] context. Pre-fix path printed bare "paste> "/"log> ".
func TestStickyTag_PropagatesToPasteAndLogPasteModes(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// /mode plan flips currentMode; then /paste enters capture and
	// expects "[mode:plan] paste> " in its prompt.
	in := strings.NewReader("/mode plan\n/paste\n/end\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner: stubRunner{}, Store: store, Render: renderNothing,
		RepoRoot: ".", Branch: "main", In: in, Out: out,
		Banner: "test", WriteEnabled: true,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "[mode:plan]") {
		t.Errorf("scripted mode echo lost [mode:plan]; out:\n%s", got)
	}
	// /paste prompt was wired to scripted scanner so the prompt itself
	// is only printed when r.interactive(); the substring assertion is
	// thus on the [mode:plan] tag in OTHER places (echo, banner) — the
	// scripted-mode path tests the wiring; the interactive prompt path
	// is exercised by inspecting r.currentStickyTag at handler time.
	r.currentMode = types.ModePlan
	r.attachedLog = "x"
	if tag := r.currentStickyTag(); !strings.Contains(tag, "[mode:plan]") || !strings.Contains(tag, "[log]") {
		t.Errorf("currentStickyTag must compose mode + attachments; got %q", tag)
	}
}

// TestStickyTag_RendersInScriptedMode pins the contract that the
// per-turn sticky-state tag (mode/log/trace/plan/mem!) reaches the
// scripted-mode output stream, not just the Bubble Tea path. Real
// bug: readInputLines used to ignore its caller's prompt and fall
// back to r.prompt, so `/mode plan` switched the mode but the next
// turn's prompt still rendered as bare `>` with no `[mode:plan]`
// indicator visible to anyone tailing the session.
func TestStickyTag_RendersInScriptedMode(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// /mode plan flips currentMode; the next turn's prompt must
	// carry [mode:plan]. /exit ends the loop after one dispatch.
	in := strings.NewReader("/mode plan\nsome request\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner: stubRunner{}, Store: store, Render: renderNothing,
		RepoRoot: ".", Branch: "main", In: in, Out: out,
		Prompt: ">", PromptCont: ".", Banner: "test-banner",
		WriteEnabled: true,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}

	if !strings.Contains(out.String(), "[mode:plan]") {
		t.Errorf("expected [mode:plan] sticky tag in prompt; out:\n%s", out.String())
	}
}

// TestVersionConfigDefaults verifies an empty Version/BuildTime in
// Config falls back to "dev" / "unknown" so a `go run` build still
// produces a coherent /version line.
func TestVersionConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	in := strings.NewReader("/version\n/exit\n")
	out := &bytes.Buffer{}
	r := New(Config{
		Runner: stubRunner{}, Store: store, Render: renderNothing,
		RepoRoot: ".", Branch: "main", In: in, Out: out,
		Prompt: ">", PromptCont: ".", Banner: "test-banner",
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if !strings.Contains(out.String(), "codrax dev (built unknown)") {
		t.Errorf("expected dev/unknown fallback, got:\n%s", out.String())
	}
}

// TestHumanByteSize pins the banner digest's unit selection so a
// tiny memory (a few bytes) does not show up as "0.0 KB" and a
// real multi-MB directory does not overflow the single-line format.
func TestHumanByteSize(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0 B"},
		{42, "42 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048575, "1024.0 KB"}, // boundary: below MB threshold
		{1048576, "1.0 MB"},
		{5 * 1024 * 1024, "5.0 MB"},
	}
	for _, c := range cases {
		if got := humanByteSize(c.in); got != c.want {
			t.Errorf("humanByteSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
