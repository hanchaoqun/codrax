package repl

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestShellBang_CapturesOutputForContext locks the user-requested
// behaviour: a `!<cmd>` invocation streams to the terminal AND
// records a memory turn whose Response carries the captured stdout
// /stderr. The next pipeline / chat turn's BuildContext picks the
// capture up via the recent-turn buffer so the operator can ask
// follow-up questions ("explain this output", "fix this error")
// against the actual command output.
func TestShellBang_CapturesOutputForContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-bang tests rely on POSIX echo; skipping on Windows")
	}
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	in := strings.NewReader("")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)

	r.handleShellBangCmd("echo hello-from-shellbang")

	// The terminal output must include the literal stdout — `!` is
	// supposed to behave like a real shell (streams in real time).
	if !strings.Contains(out.String(), "hello-from-shellbang") {
		t.Errorf("terminal output missing echoed string; got %q", out.String())
	}

	// And the memory turn must record both the command (Request)
	// and the captured output (Response) so future BuildContext
	// calls surface it.
	turns := store.Recent()
	if len(turns) == 0 {
		t.Fatalf("expected a recorded turn; store empty")
	}
	last := turns[len(turns)-1]
	if last.Request != "!echo hello-from-shellbang" {
		t.Errorf("Request = %q, want %q", last.Request, "!echo hello-from-shellbang")
	}
	if !strings.Contains(last.Response, "hello-from-shellbang") {
		t.Errorf("Response should contain captured stdout; got %q", last.Response)
	}
}

// TestShellBang_TruncatesLargeOutput locks the bounded-buffer
// behaviour. A command producing more than shellBangCaptureCap
// bytes streams the full output to the terminal but the captured
// snippet stops at the cap with a "[output truncated…]" marker.
// This protects REPL memory from runaway commands like `!find /`
// or `!cat huge_log_file`.
func TestShellBang_TruncatesLargeOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only")
	}
	dir := t.TempDir()
	store, err := memory.NewStore(dir, stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	in := strings.NewReader("")
	out := &bytes.Buffer{}
	r := newTestREPL(store, in, out)

	// Write a stream larger than the cap. `yes x | head -c <bytes>`
	// is portable across BusyBox / GNU coreutils.
	r.handleShellBangCmd("yes x | head -c 100000")

	turns := store.Recent()
	if len(turns) == 0 {
		t.Fatalf("expected a recorded turn")
	}
	resp := turns[len(turns)-1].Response
	// Captured slice must be bounded AND carry a truncation marker.
	// The memory store layer adds its own "…[truncated]" footer
	// after sanitizeTurnText hits MaxRecentBytes; the REPL layer
	// adds "[output truncated…]" when the boundedBuffer's cap fires.
	// Either marker is acceptable evidence the bound worked.
	hasMarker := strings.Contains(resp, "[output truncated") ||
		strings.Contains(resp, "…[truncated]")
	if !hasMarker {
		t.Errorf("Response missing truncation marker; got tail %q", tail(resp, 200))
	}
	if len(resp) > shellBangCaptureCap+1024 {
		t.Errorf("captured response %d bytes — should be bounded near cap %d",
			len(resp), shellBangCaptureCap)
	}
}

// TestBoundedBuffer_StopsAtCap covers the helper directly: writes
// past cap silently drop bytes but the writer keeps reporting
// "all bytes accepted" so the upstream MultiWriter doesn't error.
func TestBoundedBuffer_StopsAtCap(t *testing.T) {
	b := newBoundedBuffer(10)
	n, err := b.Write([]byte("0123456789ABCDEF"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 16 {
		t.Errorf("Write returned %d bytes accepted; want 16 (lying-by-design so MultiWriter does not abort)", n)
	}
	if b.String() != "0123456789" {
		t.Errorf("captured = %q, want first 10 bytes", b.String())
	}
	if !b.Truncated() {
		t.Error("Truncated should be true after over-cap write")
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
