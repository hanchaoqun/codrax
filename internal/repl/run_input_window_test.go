package repl

import (
	"bufio"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TTY-2 pins (design ledger docs/design/tty_single_stdin_owner_20260729.md
// §3): the run-phase input window queues typed lines, routes Ctrl+C to
// the cancel path, aborts only on a BARE esc, and hands every byte
// back at drain time — nothing typed during a Run is ever lost.

// windowFromScript builds a run window over a scripted byte source.
// The injected readByte reads the shared buffer directly; end-of-script
// behaves like a quiet terminal (timeout ticks) until drain.
func windowFromScript(t *testing.T, script string, cb runInputWindowCallbacks) (*runInputWindow, *ttyStdinOwner) {
	t.Helper()
	o, _ := testOwner(script)
	exhausted := atomic.Bool{}
	// Inject readByte: tests drive the shared buffer directly (the
	// nil-readByte production path needs a real tty fd).
	reader := o.reader
	readByte := func() (byte, bool, error) {
		if exhausted.Load() {
			time.Sleep(2 * time.Millisecond) // quiet terminal tick
			return 0, false, nil
		}
		b, err := reader.ReadByte()
		if err == io.EOF {
			exhausted.Store(true)
			return 0, false, nil
		}
		return b, err == nil, err
	}
	w, err := o.borrowRunInput(cb, readByte)
	if err != nil {
		t.Fatalf("borrowRunInput: %v", err)
	}
	return w, o
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunInputWindow_QueuesLinesAndEchoes(t *testing.T) {
	var echoed []string
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}
	cb := runInputWindowCallbacks{onLine: func(line string) {
		<-mu
		echoed = append(echoed, line)
		mu <- struct{}{}
	}}
	w, _ := windowFromScript(t, "再看下水位段\r第二条 note\n\r\n", cb)
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.queue) == 2
	})
	queued, partial, esc, dropped := w.drain()
	if len(queued) != 2 || queued[0] != "再看下水位段" || queued[1] != "第二条 note" {
		t.Fatalf("queued = %v", queued)
	}
	if partial != "" || esc || dropped {
		t.Fatalf("clean run: partial=%q esc=%v dropped=%v", partial, esc, dropped)
	}
	<-mu
	if len(echoed) != 2 {
		t.Fatalf("echo callback count = %d, want 2 (empty line never echoes)", len(echoed))
	}
}

func TestRunInputWindow_BareEscAbortsButSequencesDoNot(t *testing.T) {
	var escCount atomic.Int64
	cb := runInputWindowCallbacks{onEsc: func() { escCount.Add(1) }}

	// Arrow key (CSI A), then a queued line, then a BARE esc at end of
	// script (no follow-up byte → timeout → bare).
	w, _ := windowFromScript(t, "\x1b[Akeep going\r\x1b", cb)
	waitFor(t, func() bool { return escCount.Load() == 1 })
	queued, _, escAborted, _ := w.drain()
	if len(queued) != 1 || queued[0] != "keep going" {
		t.Fatalf("CSI sequence must be swallowed, line must queue; got %v", queued)
	}
	if !escAborted {
		t.Fatal("bare ESC must mark the window esc-aborted")
	}
	if escCount.Load() != 1 {
		t.Fatalf("onEsc fired %d times, want exactly 1 (arrow key must NOT abort)", escCount.Load())
	}
}

func TestRunInputWindow_CtrlCRoutesToCancel(t *testing.T) {
	var taps atomic.Int64
	cb := runInputWindowCallbacks{onCtrlC: func() { taps.Add(1) }}
	w, _ := windowFromScript(t, "half-typed\x03", cb)
	waitFor(t, func() bool { return taps.Load() == 1 })
	queued, partial, _, _ := w.drain()
	if len(queued) != 0 {
		t.Fatalf("unsubmitted text must not queue; got %v", queued)
	}
	if partial != "half-typed" {
		t.Fatalf("partial = %q, want the half-typed prefix handed back for prefill", partial)
	}
}

func TestDrainRunInputWindow_ReplayAndRestoreLanes(t *testing.T) {
	// Normal completion: queued lines converge on pendingFollowUps,
	// partial becomes the prompt prefill.
	r, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	w, _ := windowFromScript(t, "q1\rq2\rhalf", runInputWindowCallbacks{})
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.queue) == 2 && len(w.partial) > 0
	})
	r.drainRunInputWindow(w)
	if len(r.pendingFollowUps) != 2 || r.pendingFollowUps[0].Text != "q1" {
		t.Fatalf("pendingFollowUps = %v", r.pendingFollowUps)
	}
	if r.pendingInputPrefill != "half" {
		t.Fatalf("prefill = %q", r.pendingInputPrefill)
	}

	// Esc abort: everything is RESTORED, not replayed — multi-line
	// rides the paste-placeholder lane.
	r2, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	w2, _ := windowFromScript(t, "a1\ra2\r\x1b", runInputWindowCallbacks{})
	waitFor(t, func() bool {
		w2.mu.Lock()
		defer w2.mu.Unlock()
		return w2.escSeen
	})
	r2.drainRunInputWindow(w2)
	if len(r2.pendingFollowUps) != 0 {
		t.Fatalf("esc abort must not replay; followUps=%v", r2.pendingFollowUps)
	}
	if r2.pendingPaste != "a1\na2" {
		t.Fatalf("multi-line restore must ride the paste lane; got %q", r2.pendingPaste)
	}

	// Single-line esc restore goes to plain prefill.
	r3, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	w3, _ := windowFromScript(t, "only one\r\x1b", runInputWindowCallbacks{})
	waitFor(t, func() bool {
		w3.mu.Lock()
		defer w3.mu.Unlock()
		return w3.escSeen
	})
	r3.drainRunInputWindow(w3)
	if r3.pendingInputPrefill != "only one" || r3.pendingPaste != "" {
		t.Fatalf("single-line restore: prefill=%q paste=%q", r3.pendingInputPrefill, r3.pendingPaste)
	}
}

// TestRunInputWindow_DrainNeverWedgesOnPollError pins the shutdown
// contract on a degraded fd: the loop backs off on poll errors instead
// of spinning, and drain() returns promptly regardless.
func TestRunInputWindow_DrainNeverWedgesOnPollError(t *testing.T) {
	o := &ttyStdinOwner{fd: -1, reader: bufio.NewReader(strings.NewReader("x")),
		makeRaw: func(int) (func(), error) { return func() {}, nil }}
	w, err := o.borrowRunInput(runInputWindowCallbacks{}, nil)
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	done := make(chan struct{})
	go func() { w.drain(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("drain wedged on a degraded fd")
	}
}

// fakeSteeringRunner accepts up to acceptUntil notes and hands back a
// canned unconsumed remainder at drain time.
type fakeSteeringRunner struct {
	writeCapableRunner
	acceptUntil int
	accepted    []string
	unconsumed  []string
}

func (f *fakeSteeringRunner) PushSteeringNote(note string) bool {
	if len(f.accepted) >= f.acceptUntil {
		return false
	}
	f.accepted = append(f.accepted, note)
	return true
}

func (f *fakeSteeringRunner) TakeUnconsumedSteeringNotes() []string {
	out := f.unconsumed
	f.unconsumed = nil
	return out
}

// TestRunInputWindow_SteeredLinesBypassQueue pins the TTY-3 window
// branch: a line the sink accepts is steered (echoed as steering,
// never queued); once the sink refuses, lines queue as before.
func TestRunInputWindow_SteeredLinesBypassQueue(t *testing.T) {
	var steered []string
	sink := &fakeSteeringRunner{acceptUntil: 1}
	cb := runInputWindowCallbacks{
		trySteer:  sink.PushSteeringNote,
		onSteered: func(line string) { steered = append(steered, line) },
	}
	w, _ := windowFromScript(t, "steer me\rqueue me\r", cb)
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.queue) == 1
	})
	queued, _, _, _ := w.drain()
	if len(sink.accepted) != 1 || sink.accepted[0] != "steer me" {
		t.Fatalf("accepted = %v", sink.accepted)
	}
	if len(steered) != 1 || steered[0] != "steer me" {
		t.Fatalf("steered echo = %v", steered)
	}
	if len(queued) != 1 || queued[0] != "queue me" {
		t.Fatalf("queued = %v", queued)
	}
}

// TestDrainRunInputWindow_UnconsumedNotesReplayFirst pins the handback
// contract: notes the run accepted but never consumed re-enter the
// replay queue AFTER the window's queued lines (SWEEPFIX S6: commands
// typically change the mode/context the subsequent prose assumes, so
// queued lines — the command lane — replay first; the handed-back
// prose replays VERBATIM, never through the command funnel).
func TestDrainRunInputWindow_QueuedLinesReplayBeforeUnconsumedNotes(t *testing.T) {
	r, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	r.runner = &fakeSteeringRunner{unconsumed: []string{"steered but not consumed"}, acceptUntil: 0}
	w, _ := windowFromScript(t, "typed later\r", runInputWindowCallbacks{})
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.queue) == 1
	})
	r.drainRunInputWindow(w)
	if len(r.pendingFollowUps) != 2 ||
		r.pendingFollowUps[0].Text != "typed later" ||
		r.pendingFollowUps[1].Text != "steered but not consumed" {
		t.Fatalf("pendingFollowUps = %v", r.pendingFollowUps)
	}
	if r.pendingFollowUps[0].Verbatim || !r.pendingFollowUps[1].Verbatim {
		t.Fatalf("queued line replays through the funnel, handed-back prose replays verbatim; got %+v", r.pendingFollowUps)
	}
}
