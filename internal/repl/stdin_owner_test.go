package repl

import (
	"bufio"
	"strings"
	"testing"
)

// TTY-1 pins (design ledger docs/design/tty_single_stdin_owner_20260729.md
// §3): one shared buffer, one mode manager, no byte ever lost between
// windows, terminal restored even on the hard-kill path.

func testOwner(src string) (*ttyStdinOwner, *int) {
	restores := 0
	o := &ttyStdinOwner{
		fd:     -1,
		reader: bufio.NewReader(strings.NewReader(src)),
		makeRaw: func(int) (func(), error) {
			return func() { restores++ }, nil
		},
	}
	o.makeRunMode = o.makeRaw
	return o, &restores
}

// TestStdinOwner_TypeAheadSurvivesWindowBoundary pins the D1 fix: bytes
// buffered but unconsumed in window N are the FIRST bytes of window
// N+1 — the pre-owner per-prompt readers silently dropped them.
func TestStdinOwner_TypeAheadSurvivesWindowBoundary(t *testing.T) {
	o, restores := testOwner("ab/end of prompt\nqueued line\n")

	raw, release, err := o.borrowRaw()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []byte{'a', 'b'} {
		got, err := raw.ReadByte()
		if err != nil || got != want {
			t.Fatalf("raw read = %q err=%v, want %q", got, err, want)
		}
	}
	release()
	if *restores != 1 {
		t.Fatalf("release must restore cooked mode exactly once; restores=%d", *restores)
	}

	// The cooked window continues at the exact next byte: the rest of
	// the type-ahead line, then the queued line.
	lines, release2, err := o.borrowCookedLines(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	defer release2()
	if !lines.Scan() || lines.Text() != "/end of prompt" {
		t.Fatalf("cooked line 1 = %q err=%v, want the type-ahead remainder", lines.Text(), lines.Err())
	}
	if !lines.Scan() || lines.Text() != "queued line" {
		t.Fatalf("cooked line 2 = %q, want the queued line", lines.Text())
	}
	if lines.Scan() {
		t.Fatal("EOF expected after final line")
	}
	if lines.Err() != nil {
		t.Fatalf("clean EOF must report nil Err; got %v", lines.Err())
	}
}

// TestStdinOwner_ExclusiveWindows pins the no-competing-readers
// contract: overlapping borrows fail loudly instead of racing.
func TestStdinOwner_ExclusiveWindows(t *testing.T) {
	o, _ := testOwner("x")
	_, release, err := o.borrowRaw()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.borrowRaw(); err != errStdinOwnerBusy {
		t.Fatalf("second raw borrow = %v, want errStdinOwnerBusy", err)
	}
	if _, _, err := o.borrowCookedLines(0); err != errStdinOwnerBusy {
		t.Fatalf("cooked borrow during raw window = %v, want errStdinOwnerBusy", err)
	}
	release()
	release() // idempotent
	if _, release2, err := o.borrowRaw(); err != nil {
		t.Fatalf("borrow after release must succeed: %v", err)
	} else {
		release2()
	}
}

// TestStdinOwner_RestoreForExit pins the D2 fix: the hard-kill path
// (os.Exit skips defers) restores the terminal iff a raw window is
// live, idempotently.
func TestStdinOwner_RestoreForExit(t *testing.T) {
	o, restores := testOwner("x")
	o.restoreForExit() // no raw window: no-op
	if *restores != 0 {
		t.Fatal("restoreForExit without a raw window must be a no-op")
	}
	_, release, err := o.borrowRaw()
	if err != nil {
		t.Fatal(err)
	}
	o.restoreForExit()
	if *restores != 1 {
		t.Fatalf("restoreForExit with a live raw window must restore; restores=%d", *restores)
	}
	o.restoreForExit()
	release()
	if *restores != 1 {
		t.Fatalf("restore must be exactly-once across exit hook + release; restores=%d", *restores)
	}

	// Nil-safety of the REPL-level hook.
	var nilREPL *REPL
	nilREPL.restoreTTYForExit()
	(&REPL{}).restoreTTYForExit()
}

// TestOwnerLineScanner_UnterminatedFinalLineAndCap pins scanner-parity
// semantics: a final line without newline is delivered, clean EOF
// reports nil Err, and the byte cap fails loudly like bufio.ErrTooLong.
func TestOwnerLineScanner_UnterminatedFinalLineAndCap(t *testing.T) {
	s := &ownerLineScanner{reader: bufio.NewReader(strings.NewReader("tail without newline")), maxLineBytes: 1 << 20}
	if !s.Scan() || s.Text() != "tail without newline" {
		t.Fatalf("unterminated final line must be delivered; got %q", s.Text())
	}
	if s.Scan() || s.Err() != nil {
		t.Fatalf("after final line: Scan=false Err=nil expected; err=%v", s.Err())
	}

	long := &ownerLineScanner{reader: bufio.NewReader(strings.NewReader(strings.Repeat("a", 100) + "\n")), maxLineBytes: 10}
	if long.Scan() {
		t.Fatal("over-cap line must not be delivered")
	}
	if long.Err() != bufio.ErrTooLong {
		t.Fatalf("over-cap must surface bufio.ErrTooLong; got %v", long.Err())
	}
}
