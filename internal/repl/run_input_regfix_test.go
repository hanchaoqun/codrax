package repl

import (
	"errors"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
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

// TestRunInputWindow_PastedBangLinesNeverEnterCommandFunnel pins
// SWEEPFIX S5: a bracketed paste containing "!"-leading lines is DATA —
// captured as one verbatim blob, never split into per-line commands
// that the replay funnel would shell-execute.
func TestRunInputWindow_PastedBangLinesNeverEnterCommandFunnel(t *testing.T) {
	r, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	script := "\x1b[200~!pip install foo\r![diagram](url)\r!git clean -fdx\x1b[201~"
	w, _ := windowFromScript(t, script, runInputWindowCallbacks{})
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.pastes) == 1
	})
	r.drainRunInputWindow(w)
	if len(r.pendingFollowUps) != 1 {
		t.Fatalf("paste must stay ONE blob; got %d entries: %+v", len(r.pendingFollowUps), r.pendingFollowUps)
	}
	entry := r.pendingFollowUps[0]
	if !entry.Verbatim {
		t.Fatal("pasted content must replay verbatim — the funnel would shell-execute the ! lines")
	}
	if !strings.Contains(entry.Text, "!git clean -fdx") || !strings.Contains(entry.Text, "!pip install foo") {
		t.Fatalf("paste content mangled: %q", entry.Text)
	}
}

// TestSteeringIntake_PathLeadingProseIsAccepted pins SWEEPFIX S4 (repl
// side): the window's trySteer wrapper refuses TEMPLATE commands but
// hands path-leading prose to the sink — only funnel-precise command
// shapes queue for replay.
func TestRunInputWindow_PathProseSteersButCommandsQueue(t *testing.T) {
	sink := &fakeSteeringRunner{acceptUntil: 8}
	// Mirror the production composition: the orchestrator intake
	// refuses funnel-precise command shapes (pinned on its own side by
	// TestSteeringIntake_PreciseCommandRefusal); the window sees that
	// refusal as trySteer=false and queues the line.
	cb := runInputWindowCallbacks{trySteer: func(line string) bool {
		if strings.HasPrefix(line, "!") || types.NormalizeREPLCommandAlias(line) != "" {
			return false
		}
		return sink.PushSteeringNote(line)
	}}
	w, _ := windowFromScript(t, "/var/log/app.log has a panic at 10:32\r/approve\r!echo hi\r", cb)
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.queue) == 2
	})
	queued, _, _, _ := w.drain()
	if len(sink.accepted) != 1 || !strings.HasPrefix(sink.accepted[0], "/var/log/") {
		t.Fatalf("path-leading prose must steer; accepted=%v", sink.accepted)
	}
	if len(queued) != 2 || queued[0] != "/approve" || queued[1] != "!echo hi" {
		t.Fatalf("real commands must queue for funnel replay; queued=%v", queued)
	}
}

// TestDrainRunInputWindow_DisablesBracketedPaste pins round-3 R0's
// drain half: the window's exit always emits ESC[?2004l so the
// terminal stops sending paste markers once no reader is armed. (The
// arm half emits ESC[?2004h; armRunInputWindow needs a real TTY, so
// the enable is pinned by the paired constant use at the arm site and
// this drain-side observable.)
func TestDrainRunInputWindow_DisablesBracketedPaste(t *testing.T) {
	r, _, out := newApprovalREPL(t, "", &writeCapableRunner{})
	w, _ := windowFromScript(t, "", runInputWindowCallbacks{})
	r.drainRunInputWindow(w)
	if !strings.Contains(out.String(), "\x1b[?2004l") {
		t.Fatal("drain must disable bracketed-paste mode (paired with arm's enable)")
	}
}

// TestRunInputWindow_SplitPasteTerminatorSurvivesTicks pins round-3
// R3: a 201~ terminator split across quiet ticks (laggy link) must
// still close the paste lane instead of leaving the window swallowing
// all input in paste mode.
func TestRunInputWindow_SplitPasteTerminatorSurvivesTicks(t *testing.T) {
	script := []byte("\x1b[200~data line\x1b[201~")
	var pos atomic.Int64
	var stalls atomic.Int64
	readByte := func() (byte, bool, error) {
		i := int(pos.Load())
		if i >= len(script) {
			time.Sleep(2 * time.Millisecond)
			return 0, false, nil
		}
		// Stall (quiet tick) right in the middle of the 201~ marker.
		if i == len(script)-3 && stalls.Load() < 3 {
			stalls.Add(1)
			return 0, false, nil
		}
		pos.Add(1)
		return script[i], true, nil
	}
	o, _ := testOwner("")
	w, err := o.borrowRunInput(runInputWindowCallbacks{}, readByte)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return !w.pasting.Load() && len(w.pastes) == 1
	})
	pastes := w.takePastes()
	if len(pastes) != 1 || pastes[0] != "data line" {
		t.Fatalf("pastes = %q, want the closed blob", pastes)
	}
	w.drain()
}
