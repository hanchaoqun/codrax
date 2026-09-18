package repl

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC-17.4 input ownership: these tests deliberately enter New/Loop, not
// private listener helpers. No input owner instance is injected.
//
// The fake runner latches early cancellation for its first operation. Thus the
// pre-read case tests input routing, not Orchestrator.Run's separate token-ready
// startup gap. No model, converter, usage instrumentation or SIGINT is exercised.

const hmc17ScriptWait = 3 * time.Second

type hmc17ScriptReadProbe struct {
	pipe    *io.PipeReader
	active  atomic.Int32
	max     atomic.Int32
	reads   atomic.Int32
	closes  atomic.Int32
	changed chan struct{}
}

func (r *hmc17ScriptReadProbe) Read(p []byte) (int, error) {
	n := r.active.Add(1)
	defer r.active.Add(-1)
	for old := r.max.Load(); n > old; old = r.max.Load() {
		if r.max.CompareAndSwap(old, n) {
			break
		}
	}
	r.reads.Add(1)
	select {
	case r.changed <- struct{}{}:
	default:
	}
	return r.pipe.Read(p)
}

func (r *hmc17ScriptReadProbe) Close() error {
	r.closes.Add(1)
	return r.pipe.Close()
}

type hmc17ScriptOutput struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	changed chan struct{}
}

func (w *hmc17ScriptOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	select {
	case w.changed <- struct{}{}:
	default:
	}
	return n, err
}

func (w *hmc17ScriptOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

type hmc17ScriptRunner struct {
	mu          sync.Mutex
	current     string
	requests    []string
	reasons     []string
	entered     chan string
	firstFinish chan struct{}
	firstCancel chan struct{}
	abort       chan struct{}
	finishOnce  sync.Once
	cancelOnce  sync.Once
	abortOnce   sync.Once
	canceled    atomic.Bool
	blockFirst  bool
}

func (r *hmc17ScriptRunner) SetOutputTranscriptRequest(s string) {
	r.mu.Lock()
	r.current = s
	r.mu.Unlock()
}

func (r *hmc17ScriptRunner) Run(request, _, _ string) (*types.BusContext, error) {
	r.mu.Lock()
	current := r.current
	if current == "" {
		current = request
	}
	r.requests = append(r.requests, current)
	n := len(r.requests)
	r.mu.Unlock()
	r.entered <- current
	if n == 1 && r.blockFirst {
		select {
		case <-r.firstFinish:
		case <-r.abort:
			return nil, context.Canceled
		}
		select {
		case <-r.firstCancel:
			return nil, context.Canceled
		default:
		}
	}
	r.canceled.Store(false)
	return &types.BusContext{Mutable: types.NewMutableState(current)}, nil
}

func (r *hmc17ScriptRunner) Cancel(reason string) {
	r.mu.Lock()
	r.reasons = append(r.reasons, reason)
	r.mu.Unlock()
	r.canceled.Store(true)
	r.cancelOnce.Do(func() { close(r.firstCancel) })
}

func (r *hmc17ScriptRunner) IsCanceled() bool { return r.canceled.Load() }

func (r *hmc17ScriptRunner) finish() {
	r.finishOnce.Do(func() { close(r.firstFinish) })
}

type hmc17ScriptLoop struct {
	repl   *REPL
	runner *hmc17ScriptRunner
	reader *hmc17ScriptReadProbe
	writer *io.PipeWriter
	out    *hmc17ScriptOutput
	done   chan struct{}
	err    error // read only after done is closed
}

func hmc17NewScriptLoop(t *testing.T, blockFirst bool, lineCap int) *hmc17ScriptLoop {
	t.Helper()
	pr, pw := io.Pipe()
	h := &hmc17ScriptLoop{
		reader: &hmc17ScriptReadProbe{pipe: pr, changed: make(chan struct{}, 1)},
		writer: pw,
		out:    &hmc17ScriptOutput{changed: make(chan struct{}, 1)},
		done:   make(chan struct{}),
		runner: &hmc17ScriptRunner{
			entered: make(chan string, 128), firstFinish: make(chan struct{}),
			firstCancel: make(chan struct{}), abort: make(chan struct{}),
			blockFirst: blockFirst,
		},
	}
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{
		MaxRecentTurns: 64, MaxRecentBytes: 4 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	h.repl = New(Config{
		Runner: h.runner, Store: store, In: h.reader, Out: h.out,
		Render: renderNothing, RepoRoot: t.TempDir(), Branch: "main",
		Language: "en", Banner: "script-owner-test", AttachedLogMaxBytes: lineCap,
	})
	// Avoid installing a process-global SIGINT handler. This does not replace
	// any input code; all runtime listener/owner setup still happens in Loop.
	h.repl.cancelSigOnce.Do(func() {})
	t.Cleanup(func() {
		h.runner.abortOnce.Do(func() { close(h.runner.abort) })
		// The CALLER closes its pipe only during cleanup. In particular, do
		// not call h.reader.Close: that counter belongs to the REPL contract.
		_ = pw.Close()
		_ = pr.Close()
		select {
		case <-h.done:
		case <-time.After(hmc17ScriptWait):
			t.Error("Loop did not exit after caller-owned pipe cleanup")
		}
	})
	go func() {
		h.err = h.repl.Loop()
		close(h.done)
	}()
	return h
}

func (h *hmc17ScriptLoop) write(s string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(h.writer, s)
		done <- err
	}()
	return done
}

func (h *hmc17ScriptLoop) writeAll(t *testing.T, s string) {
	t.Helper()
	select {
	case err := <-h.write(s):
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(hmc17ScriptWait):
		t.Fatal("pipe write blocked: no input consumer or control hidden behind a full queue")
	}
}

func (h *hmc17ScriptLoop) wantRun(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-h.runner.entered:
		if got != want {
			t.Fatalf("next dispatched input = %q, want %q", got, want)
		}
	case <-time.After(hmc17ScriptWait):
		t.Fatalf("input %q was not dispatched; output=%s", want, h.out.String())
	}
}

func (h *hmc17ScriptLoop) wantCancel(t *testing.T) {
	t.Helper()
	select {
	case <-h.runner.firstCancel:
	case <-time.After(hmc17ScriptWait):
		t.Fatal("runtime /cancel was not delivered while the pipe remained open")
	}
	// The fake operation has not completed its cleanup yet. Input cancellation
	// must not cause Loop to dispatch the following question before finish.
	select {
	case got := <-h.runner.entered:
		t.Fatalf("input dispatched before canceled operation cleanup completed: %q", got)
	default:
	}
}

func (h *hmc17ScriptLoop) wantDone(t *testing.T, wantErr error) {
	t.Helper()
	select {
	case <-h.done:
	case <-time.After(hmc17ScriptWait):
		t.Fatalf("Loop waited for EOF or lost /exit; output=%s", h.out.String())
	}
	if !errors.Is(h.err, wantErr) {
		t.Fatalf("Loop error = %v, want %v", h.err, wantErr)
	}
	if got := h.reader.max.Load(); got != 1 {
		t.Errorf("maximum concurrent underlying Read calls = %d, want 1", got)
	}
	if got := h.reader.closes.Load(); got != 0 {
		t.Errorf("REPL closed the caller-owned Reader %d times", got)
	}
}

func (h *hmc17ScriptLoop) wantCalls(t *testing.T, requests []string, cancels int) {
	t.Helper()
	h.runner.mu.Lock()
	defer h.runner.mu.Unlock()
	if !reflect.DeepEqual(h.runner.requests, requests) {
		t.Fatalf("requests = %#v, want %#v", h.runner.requests, requests)
	}
	if len(h.runner.reasons) != cancels {
		t.Fatalf("cancel calls = %#v, want %d", h.runner.reasons, cancels)
	}
	for _, reason := range h.runner.reasons {
		if reason != "/cancel" {
			t.Fatalf("cancel reason = %q", reason)
		}
	}
}

func hmc17Await(t *testing.T, changed <-chan struct{}, ready func() bool, why string) {
	t.Helper()
	timer := time.NewTimer(hmc17ScriptWait)
	defer timer.Stop()
	for !ready() {
		select {
		case <-changed:
		case <-timer.C:
			t.Fatal(why)
		}
	}
}

func TestHMC17ScriptLoopPrefetchedCancelAndNextCommand(t *testing.T) {
	h := hmc17NewScriptLoop(t, true, 128*1024)
	// Fits in the first scanner buffer. The prompt may pre-read everything;
	// no further pipe write or EOF is available to rescue a second scanner.
	h.writeAll(t, "first\n/cancel\nsecond\n/exit\n")
	h.wantRun(t, "first")
	h.wantCancel(t)
	h.runner.finish()
	h.wantRun(t, "second")
	h.wantDone(t, nil)
	h.wantCalls(t, []string{"first", "second"}, 1)
}

func TestHMC17ScriptLoopCancelKeepsSameReadTail(t *testing.T) {
	h := hmc17NewScriptLoop(t, true, 128*1024)
	h.writeAll(t, "first\n")
	h.wantRun(t, "first")
	// Runtime scanner must not abandon same-read bytes after /cancel.
	h.writeAll(t, "before-cancel\n/cancel\nafter-cancel\n/exit\n")
	h.wantCancel(t)
	h.runner.finish()
	h.wantRun(t, "before-cancel")
	h.wantRun(t, "after-cancel")
	h.wantDone(t, nil)
	h.wantCalls(t, []string{"first", "before-cancel", "after-cancel"}, 1)
}

func TestHMC17ScriptLoopNormalCompletionDoesNotWaitForOpenPipe(t *testing.T) {
	h := hmc17NewScriptLoop(t, true, 128*1024)
	h.writeAll(t, "first\n")
	h.wantRun(t, "first")
	// Prove an input Read is already parked before the operation finishes.
	hmc17Await(t, h.reader.changed, func() bool { return h.reader.reads.Load() >= 2 }, "runtime input never began reading")
	h.runner.finish()
	hmc17Await(t, h.out.changed, func() bool {
		return strings.Count(h.out.String(), "❯❯") >= 2
	}, "operation stop waited for an open pipe instead of returning to the prompt")
	h.writeAll(t, "second\n/exit\n")
	h.wantRun(t, "second")
	h.wantDone(t, nil)
	h.wantCalls(t, []string{"first", "second"}, 0)
}

func TestHMC17ScriptLoopPartialLineCompletesAfterPreviousRun(t *testing.T) {
	h := hmc17NewScriptLoop(t, true, 128*1024)
	h.writeAll(t, "first\n")
	h.wantRun(t, "first")
	// A physical Read starts under the old operation, but this is not a
	// submitted line yet. Finishing that operation must retain every byte.
	h.writeAll(t, "late-")
	h.runner.finish()
	hmc17Await(t, h.out.changed, func() bool {
		return strings.Count(h.out.String(), "❯❯") >= 2
	}, "partial input prevented return to the next prompt")
	h.writeAll(t, "question\n/exit\n")
	h.wantRun(t, "late-question")
	h.wantDone(t, nil)
	h.wantCalls(t, []string{"first", "late-question"}, 0)
}

func TestHMC17ScriptLoopQueueOverflowStillReachesCancelAndDisclosesLoss(t *testing.T) {
	h := hmc17NewScriptLoop(t, true, 128*1024)
	h.writeAll(t, "first\n")
	h.wantRun(t, "first")
	const capacity, overflow = 32, 5 // existing follow-up admission contract
	var batch strings.Builder
	want := []string{"first"}
	for i := 0; i < capacity+overflow; i++ {
		line := fmt.Sprintf("queued-%02d", i)
		fmt.Fprintln(&batch, line)
		if i < capacity {
			want = append(want, line)
		}
	}
	batch.WriteString("/cancel\n")
	h.writeAll(t, batch.String())
	h.wantCancel(t)
	h.runner.finish()
	for _, line := range want[1:] {
		h.wantRun(t, line)
	}
	h.writeAll(t, "/exit\n")
	h.wantDone(t, nil)
	h.wantCalls(t, want, 1)
	printed := strings.ToLower(h.out.String())
	if !strings.Contains(printed, "queue full") || !strings.Contains(printed, "drop") {
		t.Fatalf("queueing notice must not suppress overflow loss disclosure: %s", printed)
	}
}

func TestHMC17ScriptLoopCaptureKeepsCommandsAsDataAndContinuation(t *testing.T) {
	h := hmc17NewScriptLoop(t, false, 128*1024)
	h.writeAll(t, "/log\r\n/cancel\r\n/exit\r\n  literal  \r\n/end\r\nquestion\\\r\ncontinued\r\n/exit\r\n")
	h.wantRun(t, "question\ncontinued")
	h.wantDone(t, nil)
	h.wantCalls(t, []string{"question\ncontinued"}, 0)
	if got, want := h.repl.attachedLog, "/cancel\n/exit\n  literal  \n"; got != want {
		t.Fatalf("capture changed command-shaped text: %q, want %q", got, want)
	}
}

func TestHMC17ScriptLoopEOFFinalUnterminatedLine(t *testing.T) {
	h := hmc17NewScriptLoop(t, false, 128*1024)
	h.writeAll(t, "last-question")
	if err := h.writer.Close(); err != nil {
		t.Fatal(err)
	}
	h.wantRun(t, "last-question")
	h.wantDone(t, nil)
	h.wantCalls(t, []string{"last-question"}, 0)
}

func TestHMC17ScriptLoopLongRuntimeLineUsesSharedConfiguredLimit(t *testing.T) {
	const limit = 2 * 1024 * 1024
	h := hmc17NewScriptLoop(t, true, limit)
	h.writeAll(t, "first\n")
	h.wantRun(t, "first")
	// Above the old runtime listener's 1 MiB but within the prompt/capture
	// configured ceiling. No line fragment may be dispatched as a question.
	long := strings.Repeat("x", 1024*1024+257)
	h.writeAll(t, long+"\n/cancel\n")
	h.wantCancel(t)
	h.runner.finish()
	h.wantRun(t, long)
	h.writeAll(t, "/exit\n")
	h.wantDone(t, nil)
	h.wantCalls(t, []string{"first", long}, 1)
}

func TestHMC17ScriptLoopOversizeIsAnExplicitErrorNotSuccessfulEOF(t *testing.T) {
	const limit = 128 * 1024
	h := hmc17NewScriptLoop(t, false, limit)
	// An over-limit writer may remain blocked after Scanner terminates. Do
	// not close it to force the result; only test cleanup owns that Close.
	h.write(strings.Repeat("x", limit+1024) + "\n")
	h.wantDone(t, bufio.ErrTooLong)
	h.wantCalls(t, nil, 0)
}
