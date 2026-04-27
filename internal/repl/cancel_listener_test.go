package repl

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubCanceller is a runnerCanceller test double. Records Cancel
// calls so the test can assert reason + count.
type stubCanceller struct {
	mu       sync.Mutex
	calls    []string
	canceled atomic.Bool
}

func (s *stubCanceller) Cancel(reason string) {
	s.mu.Lock()
	s.calls = append(s.calls, reason)
	s.mu.Unlock()
	s.canceled.Store(true)
}

func (s *stubCanceller) IsCanceled() bool { return s.canceled.Load() }

func (s *stubCanceller) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *stubCanceller) lastReason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.calls) == 0 {
		return ""
	}
	return s.calls[len(s.calls)-1]
}

// waitForCancel polls the stub until canceled with a short timeout.
// Avoids time.Sleep in tests by polling the atomic flag.
func waitForCancel(s *stubCanceller, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.IsCanceled() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// TestCancelListener_TriggersOnSlashCancelLine locks the primary
// path: a `/cancel\n` line on stdin drives runner.Cancel exactly
// once with reason="/cancel".
func TestCancelListener_TriggersOnSlashCancelLine(t *testing.T) {
	stdin := strings.NewReader("/cancel\n")
	stub := &stubCanceller{}
	cl := startCancelListener(stdin, stub, nil)
	if cl == nil {
		t.Fatal("listener should start for non-TTY io.Reader")
	}
	defer cl.stop()
	if !waitForCancel(stub, 200*time.Millisecond) {
		t.Fatal("Cancel was not called within 200ms")
	}
	if stub.lastReason() != "/cancel" {
		t.Errorf("Cancel reason = %q, want /cancel", stub.lastReason())
	}
	if stub.callCount() != 1 {
		t.Errorf("Cancel called %d times, want 1", stub.callCount())
	}
}

// TestCancelListener_AcceptsBackslashAlias verifies `\cancel` line
// is treated identically — operators with muscle memory for
// PostgreSQL psql / certain custom REPLs reach for `\` first.
func TestCancelListener_AcceptsBackslashAlias(t *testing.T) {
	stdin := strings.NewReader("\\cancel\n")
	stub := &stubCanceller{}
	cl := startCancelListener(stdin, stub, nil)
	if cl == nil {
		t.Fatal("listener should start")
	}
	defer cl.stop()
	if !waitForCancel(stub, 200*time.Millisecond) {
		t.Fatal("backslash-cancel not honoured")
	}
}

// TestCancelListener_IgnoresOtherLines drops non-cancel input with
// a one-shot warn so accidental paste / wrong commands during Run
// do not silently leak into the next turn or flood the terminal.
func TestCancelListener_IgnoresOtherLines(t *testing.T) {
	// Two stray lines, then /cancel. Only the cancel should fire
	// runner.Cancel; the warn should fire exactly once across the
	// stray lines (rate-limited).
	stdin := strings.NewReader("hello\nworld\n/cancel\n")
	stub := &stubCanceller{}
	var warnCalls atomic.Int32
	warn := func(format string, args ...any) { warnCalls.Add(1) }
	cl := startCancelListener(stdin, stub, warn)
	if cl == nil {
		t.Fatal("listener should start")
	}
	defer cl.stop()
	if !waitForCancel(stub, 200*time.Millisecond) {
		t.Fatal("Cancel was not called for the trailing /cancel line")
	}
	// Wait one more poll cycle so any spurious extra warn surfaces.
	time.Sleep(20 * time.Millisecond)
	if got := warnCalls.Load(); got != 1 {
		t.Errorf("warn fired %d times, want exactly 1 (one-shot rate-limit)", got)
	}
}

// TestCancelListener_LeadingWhitespaceTolerated covers the operator
// who hits space before the slash. TrimSpace makes the listener
// forgiving without admitting genuinely different commands like
// `/cancel-foo`.
func TestCancelListener_LeadingWhitespaceTolerated(t *testing.T) {
	stdin := strings.NewReader("   /cancel   \n")
	stub := &stubCanceller{}
	cl := startCancelListener(stdin, stub, nil)
	if cl == nil {
		t.Fatal("listener should start")
	}
	defer cl.stop()
	if !waitForCancel(stub, 200*time.Millisecond) {
		t.Fatal("whitespace-padded /cancel not recognised")
	}
}

// TestCancelListener_RejectsCancelPrefixOnly defends against
// false-positives like `/cancellation` — the listener uses an exact
// match or the longer `/cancel ARGS` form, never a HasPrefix on the
// raw token.
func TestCancelListener_RejectsCancelPrefixOnly(t *testing.T) {
	stdin := strings.NewReader("/cancellation\n/canceler\n")
	stub := &stubCanceller{}
	cl := startCancelListener(stdin, stub, nil)
	if cl == nil {
		t.Fatal("listener should start")
	}
	defer cl.stop()
	time.Sleep(80 * time.Millisecond)
	if stub.IsCanceled() {
		t.Errorf("/cancellation / /canceler must NOT trigger Cancel; got reason=%q", stub.lastReason())
	}
}

// TestCancelListener_NilCancellerSkips covers the test-runner case:
// no orchestrator → no listener. Returns nil, defer .stop is a
// no-op.
func TestCancelListener_NilCancellerSkips(t *testing.T) {
	cl := startCancelListener(strings.NewReader("/cancel\n"), nil, nil)
	if cl != nil {
		t.Errorf("listener must not start when canceller is nil")
	}
	cl.stop() // nil-safe
}

// TestCancelListener_StopUnblocksOnEOF covers the lifecycle: stop()
// flips the flag, the goroutine exits naturally when the underlying
// reader hits EOF (which strings.NewReader does after consumption).
func TestCancelListener_StopUnblocksOnEOF(t *testing.T) {
	stdin := strings.NewReader("not-cancel\n")
	stub := &stubCanceller{}
	cl := startCancelListener(stdin, stub, nil)
	if cl == nil {
		t.Fatal("listener should start")
	}
	cl.stop()
	// done channel should be closed.
	select {
	case <-cl.done:
		// ok
	case <-time.After(200 * time.Millisecond):
		t.Fatal("done channel not closed after stop")
	}
}
