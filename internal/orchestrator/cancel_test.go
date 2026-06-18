package orchestrator

import (
	"errors"
	"sync"
	"testing"
)

// TestCancelToken_FirstReasonWins locks the idempotent contract:
// multiple Cancel() calls keep the FIRST recorded reason so
// Ctrl+C cannot be overwritten by a follow-on /cancel. Tests / logs
// that read Reason() get a stable value regardless of caller order.
func TestCancelToken_FirstReasonWins(t *testing.T) {
	tok := NewCancelToken()
	if tok.IsCanceled() {
		t.Error("fresh token should not be canceled")
	}
	tok.Cancel("Ctrl+C")
	tok.Cancel("/cancel") // second caller — must NOT overwrite
	if !tok.IsCanceled() {
		t.Error("Cancel should mark token canceled")
	}
	if got := tok.Reason(); got != "Ctrl+C" {
		t.Errorf("Reason = %q, want first reason 'Ctrl+C'", got)
	}
	if got := tok.Source(); got != CancelSourceUser {
		t.Errorf("Source = %q, want %q", got, CancelSourceUser)
	}
}

func TestCancelToken_FirstSourceWins(t *testing.T) {
	tok := NewCancelToken()
	tok.CancelWithSource("write mode wall-time exceeded", CancelSourceWriteDeadline)
	tok.Cancel("Ctrl+C")
	if got := tok.Reason(); got != "write mode wall-time exceeded" {
		t.Errorf("Reason = %q, want first deadline reason", got)
	}
	if got := tok.Source(); got != CancelSourceWriteDeadline {
		t.Errorf("Source = %q, want %q", got, CancelSourceWriteDeadline)
	}
}

// TestCancelToken_EmptyReasonFallback verifies a missing reason
// degrades to the canonical default rather than ""; downstream
// rendering relies on having SOME text in the canceled message.
func TestCancelToken_EmptyReasonFallback(t *testing.T) {
	tok := NewCancelToken()
	tok.Cancel("")
	if got := tok.Reason(); got != "user interrupt" {
		t.Errorf("empty reason should fall back to 'user interrupt'; got %q", got)
	}
}

// TestCancelToken_NilSafe — defensive: a nil receiver must not
// panic. The Orchestrator returns nil from CancelChecker() when
// uninstantiated, and tests with stub orchestrators rely on
// IsCanceled / Reason behaving as no-op on nil.
func TestCancelToken_NilSafe(t *testing.T) {
	var tok *CancelToken
	tok.Cancel("anything")
	if tok.IsCanceled() {
		t.Error("nil token IsCanceled must be false")
	}
	if got := tok.Reason(); got != "" {
		t.Errorf("nil token Reason should be empty; got %q", got)
	}
}

// TestCancelToken_ConcurrentCancel covers the multi-goroutine race:
// signal handler and `/cancel` slash command both fire Cancel() —
// the test verifies no data race + idempotent end state.
func TestCancelToken_ConcurrentCancel(t *testing.T) {
	tok := NewCancelToken()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				tok.Cancel("Ctrl+C")
			} else {
				tok.Cancel("/cancel")
			}
		}(i)
	}
	wg.Wait()
	if !tok.IsCanceled() {
		t.Error("after concurrent cancels, IsCanceled must be true")
	}
	r := tok.Reason()
	if r != "Ctrl+C" && r != "/cancel" {
		t.Errorf("Reason should be one of the supplied; got %q", r)
	}
}

// TestCanceledError_Format covers the three formatting branches
// (stage+iter / stage only / bare). Stage label populated by the
// most-specific checkpoint; iter populated by the agent loop only.
func TestCanceledError_Format(t *testing.T) {
	cases := []struct {
		e    *CanceledError
		want string
	}{
		{&CanceledError{Reason: "Ctrl+C", AtStage: "explore", Iter: 5}, "canceled at stage=explore iter=5: Ctrl+C"},
		{&CanceledError{Reason: "Ctrl+C", AtStage: "explore"}, "canceled at stage=explore: Ctrl+C"},
		{&CanceledError{Reason: "Ctrl+C"}, "canceled: Ctrl+C"},
	}
	for _, c := range cases {
		if got := c.e.Error(); got != c.want {
			t.Errorf("Error() = %q, want %q", got, c.want)
		}
	}
}

// TestCanceledError_IsErrCanceled locks the sentinel match:
// errors.Is(err, ErrCanceled) succeeds even on a nested wrapper.
// Without this, REPL's friendlyRunError can't recognise the typed
// shape and falls through to the generic "context canceled" branch.
func TestCanceledError_IsErrCanceled(t *testing.T) {
	err := &CanceledError{Reason: "Ctrl+C", AtStage: "apply"}
	if !errors.Is(err, ErrCanceled) {
		t.Error("errors.Is(err, ErrCanceled) must succeed for direct wrap")
	}
	wrapped := &wrapperErr{inner: err}
	if !errors.Is(wrapped, ErrCanceled) {
		t.Error("errors.Is must traverse Unwrap chain")
	}
}

type wrapperErr struct{ inner error }

func (w *wrapperErr) Error() string { return w.inner.Error() }
func (w *wrapperErr) Unwrap() error { return w.inner }

// TestCanceledError_StageMethod covers the user-visible Stage()
// accessor (the messages package reads it via a small interface to
// avoid an import cycle). nil receiver returns "" — callers that
// receive nil errors should not crash.
func TestCanceledError_StageMethod(t *testing.T) {
	var nilCe *CanceledError
	if got := nilCe.Stage(); got != "" {
		t.Errorf("nil Stage() = %q, want empty", got)
	}
	ce := &CanceledError{AtStage: "verify"}
	if got := ce.Stage(); got != "verify" {
		t.Errorf("Stage() = %q, want verify", got)
	}
}
