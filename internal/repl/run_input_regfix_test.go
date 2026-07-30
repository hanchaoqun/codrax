package repl

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// REGFIX-A pins (regression sweep 2026-07-30, findings #0/#1/#3/#4/#5;
// postmortem docs/design/tty_single_stdin_owner_20260729.md §6): the
// containment's own degraded lane must never act on a future run.

// TestRunInputWindow_AbandonedReaderCannotFireCallbacks pins finding #0:
// a reader that unwedges AFTER drain timed out must touch nothing — no
// cancel of the next run, no stale steering note, no swallowed byte
// echo. Decisive: the byte is delivered late and the callbacks stay
// silent.
func TestRunInputWindow_AbandonedReaderCannotFireCallbacks(t *testing.T) {
	var esc, ctrlC, lines atomic.Int64
	release := make(chan struct{})
	unwedge := make(chan struct{})
	parked := make(chan struct{})
	first := true
	readByte := func() (byte, bool, error) {
		if first {
			first = false
			close(parked)
			<-unwedge              // park like a wedged platform read
			return 0x1b, true, nil // bare ESC arrives late
		}
		<-release
		return 0, false, nil
	}
	o, _ := testOwner("")
	w, err := o.borrowRunInput(runInputWindowCallbacks{
		onEsc:     func() { esc.Add(1) },
		onCtrlC:   func() { ctrlC.Add(1) },
		trySteer:  func(string) bool { lines.Add(1); return true },
		onSteered: func(string) {},
	}, readByte)
	if err != nil {
		t.Fatal(err)
	}

	<-parked
	done := make(chan struct{})
	go func() { w.drain(); close(done) }()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("drain must return via the 3s fuse")
	}

	close(unwedge) // reader finally delivers the ESC byte
	close(release)
	time.Sleep(150 * time.Millisecond)
	if esc.Load() != 0 || ctrlC.Load() != 0 || lines.Load() != 0 {
		t.Fatalf("abandoned reader fired callbacks: esc=%d ctrlC=%d lines=%d",
			esc.Load(), ctrlC.Load(), lines.Load())
	}
}

// TestRunInputWindow_FuseReleasesBorrowWhenReaderExits pins finding #1:
// one stall must not brick the owner for the session — the borrow is
// released as soon as the goroutine exits, so later windows can arm.
func TestRunInputWindow_FuseReleasesBorrowWhenReaderExits(t *testing.T) {
	unwedge := make(chan struct{})
	parked := make(chan struct{})
	var parkedOnce atomic.Bool
	readByte := func() (byte, bool, error) {
		if parkedOnce.CompareAndSwap(false, true) {
			close(parked)
		}
		<-unwedge
		return 0, false, nil
	}
	o, restores := testOwner("")
	w, err := o.borrowRunInput(runInputWindowCallbacks{}, readByte)
	if err != nil {
		t.Fatal(err)
	}
	<-parked  // the reader is genuinely wedged before we drain
	w.drain() // fuse fires (reader still parked)

	if _, _, err := o.borrowRaw(); err != errStdinOwnerBusy {
		t.Fatalf("borrow during an abandoned window should be busy; got %v", err)
	}
	close(unwedge) // reader unwedges → deferred release must run
	deadline := time.Now().Add(3 * time.Second)
	for {
		reader, rel, err := o.borrowRaw()
		if err == nil {
			_ = reader
			rel()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owner stayed bricked after the reader exited")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if *restores == 0 {
		t.Fatal("terminal mode must be restored when the abandoned window releases")
	}
}

// TestRunInputWindow_ReadErrorNeverCancelsRun pins finding #4: an ESC
// followed by a poll ERROR (EINTR-shaped) must NOT be treated as a bare
// ESC — cancelling the user's run on a signal artifact is unacceptable.
func TestRunInputWindow_ReadErrorNeverCancelsRun(t *testing.T) {
	var esc atomic.Int64
	step := 0
	readByte := func() (byte, bool, error) {
		step++
		switch step {
		case 1:
			return 0x1b, true, nil // ESC
		case 2:
			return 0, false, errors.New("interrupted system call") // probe errors
		}
		time.Sleep(5 * time.Millisecond)
		return 0, false, nil
	}
	o, _ := testOwner("")
	w, err := o.borrowRunInput(runInputWindowCallbacks{onEsc: func() { esc.Add(1) }}, readByte)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	_, _, escAborted, _ := w.drain()
	if esc.Load() != 0 || escAborted {
		t.Fatalf("read error must not become a bare ESC: onEsc=%d escAborted=%v", esc.Load(), escAborted)
	}
}

// TestRunInputWindow_BackspaceEditsPendingLine pins finding #5: DEL
// must edit the pending line, not land as a raw byte inside the queued
// text (and it must trim whole runes).
func TestRunInputWindow_BackspaceEditsPendingLine(t *testing.T) {
	w, _ := windowFromScript(t, "ab\x7f中\x7fc\r", runInputWindowCallbacks{})
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.queue) == 1
	})
	queued, _, _, _ := w.drain()
	if len(queued) != 1 || queued[0] != "ac" {
		t.Fatalf("queued = %q, want \"ac\" (backspace trims one byte then one whole rune)", queued)
	}
}

// TestCommitLine_DeadPreservesPartialForDrain pins SWEEPFIX S0: an
// Enter racing drain() must not lose the typed line — a dead window
// leaves the bytes in partial so drain returns them as prefill.
func TestCommitLine_DeadPreservesPartialForDrain(t *testing.T) {
	o, _ := testOwner("")
	w, err := o.borrowRunInput(runInputWindowCallbacks{
		trySteer: func(string) bool { t.Fatal("dead window must not steer"); return false },
	}, func() (byte, bool, error) { time.Sleep(2 * time.Millisecond); return 0, false, nil })
	if err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	w.partial = append(w.partial, []byte("deploy the fix")...)
	w.mu.Unlock()
	w.dead.Store(true)
	w.commitLine(runInputWindowCallbacks{
		trySteer: func(string) bool { t.Fatal("dead window must not steer"); return false },
	})
	_, partial, _, _ := w.drain()
	if partial != "deploy the fix" {
		t.Fatalf("partial = %q, want the typed line preserved for prefill", partial)
	}
}

// TestRunInputWindow_DeadPathHandsConsumedByteBack pins SWEEPFIX S1: a
// byte consumed out of the shared reader just as drain invalidates the
// window must be unread (next window's type-ahead), not dropped.
func TestRunInputWindow_DeadPathHandsConsumedByteBack(t *testing.T) {
	var unreads atomic.Int64
	parked := make(chan struct{})
	unwedge := make(chan struct{})
	first := true
	readByte := func() (byte, bool, error) {
		if first {
			first = false
			close(parked)
			<-unwedge
			return 'x', true, nil // keystroke lands after drain began
		}
		return 0, false, nil
	}
	o, _ := testOwner("")
	w, err := o.borrowRunInput(runInputWindowCallbacks{}, readByte)
	if err != nil {
		t.Fatal(err)
	}
	w.unreadByte = func() error { unreads.Add(1); return nil }
	<-parked
	done := make(chan struct{})
	go func() { w.drain(); close(done) }()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("drain must return")
	}
	close(unwedge)
	waitFor(t, func() bool { return unreads.Load() == 1 })
}

// TestRunInputWindow_FuseRestoresModeAtAbandonTime pins SWEEPFIX S2:
// the terminal mode goes back when the fuse ABANDONS the window (while
// this drain still owns the lane), not at the arbitrary later moment
// the wedged reader unwedges underneath another input lane.
func TestRunInputWindow_FuseRestoresModeAtAbandonTime(t *testing.T) {
	unwedge := make(chan struct{})
	parked := make(chan struct{})
	var parkedOnce atomic.Bool
	readByte := func() (byte, bool, error) {
		if parkedOnce.CompareAndSwap(false, true) {
			close(parked)
		}
		<-unwedge
		return 0, false, nil
	}
	o, restores := testOwner("")
	w, err := o.borrowRunInput(runInputWindowCallbacks{}, readByte)
	if err != nil {
		t.Fatal(err)
	}
	<-parked
	w.drain() // fuse fires
	if *restores != 1 {
		t.Fatalf("mode must be restored AT abandon time; restores=%d", *restores)
	}
	close(unwedge)
	// The unwedge frees the borrow but the WATCHER must not restore a
	// second time. The successful probe borrow below restores once on
	// its own release, so the final count is exactly 2 — a watcher
	// restore would make it 3.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, rel, err := o.borrowRaw(); err == nil {
			rel()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("borrow must free after unwedge")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if *restores != 2 {
		t.Fatalf("watcher must not re-restore (1 fuse + 1 probe borrow expected); restores=%d", *restores)
	}
}
