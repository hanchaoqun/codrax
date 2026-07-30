package repl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"

	"golang.org/x/term"
)

// ttyStdinOwner is the single owner of os.Stdin for the TTY
// interactive lane (TTY-1, design ledger
// docs/design/tty_single_stdin_owner_20260729.md). It holds THE one
// bufio.Reader over stdin for the REPL's lifetime and is the only
// component that switches the terminal between raw and cooked mode.
//
// Why: the pre-owner input layer created a fresh buffered reader per
// prompt window and threw it away on return — any type-ahead bytes the
// buffer had already consumed were silently lost (census defect D1),
// and every new window was a potential competing reader. With one
// shared buffer, bytes left over from window N are simply the first
// bytes of window N+1; with one mode manager, raw/cooked transitions
// and reads can never race each other.
//
// Scope: the script lane (r.in != nil) and the bubbletea fallback
// (non-tty interactive) deliberately stay outside the owner — see the
// design ledger's narrowing rulings.
type ttyStdinOwner struct {
	fd     int
	reader *bufio.Reader

	mu    sync.Mutex
	inUse bool
	// activeRestore is non-nil exactly while a raw borrow is live.
	// restoreForExit uses it on the hard-kill path (double Ctrl+C →
	// os.Exit skips defers — census defect D2).
	activeRestore func()

	// makeRaw is injectable for tests; production wraps term.MakeRaw.
	makeRaw func(fd int) (restore func(), err error)
	// makeRunMode is the RUN-window terminal mode (cbreak: ECHO|ICANON
	// off, OPOST and ISIG KEPT — full raw mode during a run corrupts
	// the dock repaint arithmetic; see run_input_mode_bsd.go).
	makeRunMode func(fd int) (restore func(), err error)
}

func newTTYStdinOwner() *ttyStdinOwner {
	return &ttyStdinOwner{
		fd:     int(os.Stdin.Fd()),
		reader: bufio.NewReader(os.Stdin),
		makeRaw: func(fd int) (func(), error) {
			state, err := term.MakeRaw(fd)
			if err != nil {
				return nil, err
			}
			return func() { _ = term.Restore(fd, state) }, nil
		},
		makeRunMode: makeRunInputMode,
	}
}

var errStdinOwnerBusy = errors.New("stdin owner: window already borrowed")

// borrowRaw opens an exclusive raw-mode window over the shared reader.
// The returned release func restores cooked mode and frees the window;
// buffered bytes stay in the shared reader for the next window.
func (o *ttyStdinOwner) borrowRaw() (*bufio.Reader, func(), error) {
	return o.borrowWithMode(o.makeRaw)
}

// borrowWithMode is the shared exclusive-window core: one mode
// switch, one reader, release restores and frees.
func (o *ttyStdinOwner) borrowWithMode(mode func(fd int) (func(), error)) (*bufio.Reader, func(), error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.inUse {
		return nil, nil, errStdinOwnerBusy
	}
	if mode == nil {
		mode = o.makeRaw
	}
	restore, err := mode(o.fd)
	if err != nil {
		return nil, nil, err
	}
	o.inUse = true
	o.activeRestore = restore
	var once sync.Once
	release := func() {
		once.Do(func() {
			o.mu.Lock()
			defer o.mu.Unlock()
			if o.activeRestore != nil {
				o.activeRestore()
				o.activeRestore = nil
			}
			o.inUse = false
		})
	}
	return o.reader, release, nil
}

// borrowCookedLines opens an exclusive cooked-mode line window (the
// terminal is already cooked between prompts; no mode change). Used by
// the interactive capture lanes (/log paste, /paste).
func (o *ttyStdinOwner) borrowCookedLines(maxLineBytes int) (*ownerLineScanner, func(), error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.inUse {
		return nil, nil, errStdinOwnerBusy
	}
	o.inUse = true
	var once sync.Once
	release := func() {
		once.Do(func() {
			o.mu.Lock()
			defer o.mu.Unlock()
			o.inUse = false
		})
	}
	return &ownerLineScanner{reader: o.reader, maxLineBytes: maxLineBytes}, release, nil
}

// restoreModeEarly performs the pending terminal-mode restore NOW and
// clears it, WITHOUT freeing the borrow (the wedged reader still owns
// the shared bufio.Reader until it exits). SWEEPFIX S2 (sweep
// 2026-07-30): the fuse's release-on-done watcher used to run the
// restore at whatever arbitrary later moment the reader unwedged —
// snapping the terminal to cooked mode underneath whichever input lane
// (e.g. the bubbletea fallback editor) owned it by then. The mode goes
// back at ABANDON time, when this drain still owns the terminal; the
// watcher's release() then finds activeRestore nil and only frees.
func (o *ttyStdinOwner) restoreModeEarly() {
	if o == nil {
		return
	}
	o.mu.Lock()
	restore := o.activeRestore
	o.activeRestore = nil
	o.mu.Unlock()
	if restore != nil {
		restore()
	}
}

// restoreForExit restores the terminal if a raw window is live — the
// hard-kill path (os.Exit) never runs window defers. Idempotent.
func (o *ttyStdinOwner) restoreForExit() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.activeRestore != nil {
		o.activeRestore()
		o.activeRestore = nil
	}
}

// ownerLineScanner adapts the shared bufio.Reader to the minimal
// Scan/Text/Err surface the capture loops use, so type-ahead bytes
// buffered during a prompt window flow seamlessly into the capture
// window (bufio.Scanner over a fresh reader would start past them).
type ownerLineScanner struct {
	reader       *bufio.Reader
	maxLineBytes int
	line         string
	err          error
}

func (s *ownerLineScanner) Scan() bool {
	if s.err != nil {
		return false
	}
	var b strings.Builder
	for {
		chunk, err := s.reader.ReadString('\n')
		b.WriteString(chunk)
		if s.maxLineBytes > 0 && b.Len() > s.maxLineBytes {
			s.err = bufio.ErrTooLong
			return false
		}
		if err != nil {
			if err == io.EOF && b.Len() > 0 {
				// Final unterminated line: deliver it; next Scan
				// reports EOF-driven false with a nil Err (scanner
				// semantics).
				s.line = strings.TrimRight(b.String(), "\r\n")
				s.err = io.EOF
				return true
			}
			if err != io.EOF {
				s.err = err
			} else {
				s.err = io.EOF
			}
			return false
		}
		s.line = strings.TrimRight(b.String(), "\r\n")
		return true
	}
}

func (s *ownerLineScanner) Text() string { return s.line }

// Err mirrors bufio.Scanner: EOF is a clean stop, not an error.
func (s *ownerLineScanner) Err() error {
	if s.err == io.EOF {
		return nil
	}
	return s.err
}

// captureLineScanner is the minimal read surface shared by the
// scripted bufio.Scanner and the owner's line window.
type captureLineScanner interface {
	Scan() bool
	Text() string
	Err() error
}

// ttyStdinOwnerInstance lazily creates the REPL's owner. Only the TTY
// interactive lane calls this; script-lane REPLs never construct one.
func (r *REPL) ttyStdinOwnerInstance() *ttyStdinOwner {
	r.stdinOwnerOnce.Do(func() {
		r.stdinOwnerInst = newTTYStdinOwner()
	})
	return r.stdinOwnerInst
}

// restoreTTYForExit is the hard-kill hook: safe on nil/never-built
// owners, safe to call from the SIGINT goroutine.
func (r *REPL) restoreTTYForExit() {
	if r == nil {
		return
	}
	// Round-4 R0: the hard-kill paths (double Ctrl+C -> os.Exit skips
	// every defer) must not leave DECSET 2004 on the user's terminal —
	// writing the disable when the mode was never enabled is harmless.
	if r.out != nil {
		fmt.Fprint(r.out, ansiDisableBracketed)
	}
	if r.stdinOwnerInst == nil {
		return
	}
	r.stdinOwnerInst.restoreForExit()
}

// ── T-2: run-phase input window ──────────────────────────────────────
//
// While a pipeline Run is in flight the TTY previously had NO stdin
// reader at all — typing was invisible and lost. The run window
// borrows the owner (raw mode) on a goroutine that tick-polls the
// shared reader via readByteFDWithTimeout: the goroutine can never
// park indefinitely in Read, so release() returns within one tick and
// the next prompt window starts with zero competing readers (the
// census's "hanging Read across the boundary" hazard is structurally
// avoided). Assembled lines queue for post-Run replay; control bytes
// route to the same cancel semantics the SIGINT path uses.

// runInputWindowCallbacks are invoked FROM the window goroutine.
// onLine echoes an enqueued line (must go through the renderer lock,
// never a raw write — dock repaint discipline); onCtrlC/onEsc must be
// non-blocking.
type runInputWindowCallbacks struct {
	onLine  func(line string)
	onPaste func(lineCount int)
	// trySteer offers the line to the running pipeline FIRST (TTY-3):
	// true = consumed as a mid-run steering note (not queued);
	// false/nil = the line queues for post-Run replay.
	trySteer  func(line string) bool
	onSteered func(line string)
	onCtrlC   func()
	onEsc     func()
}

type runInputWindow struct {
	release func()
	owner   *ttyStdinOwner
	// unreadByte pushes the last consumed byte back into the shared
	// reader (SWEEPFIX S1): when the dead check discards a freshly
	// consumed byte it must return it to the reader as the next
	// window's type-ahead — dropping it loses a keystroke, and if the
	// byte was a UTF-8 lead byte the orphaned continuation bytes would
	// surface as mojibake in the next prompt. Nil in injected-reader
	// tests that manage their own byte source.
	unreadByte func() error
	stop       chan struct{}
	done       chan struct{}
	// dead is set the instant drain() begins (REGFIX-A #0): the loop
	// checks it AFTER every readByte so a byte that arrives while the
	// goroutine was parked can never reach run-N's callbacks — the
	// abandoned-reader path could otherwise cancel a FUTURE run, inject
	// a stale steering note into it, or swallow the user's keystrokes.
	dead atomic.Bool

	mu      sync.Mutex
	queue   []string
	partial []byte
	escSeen bool
	overCap bool

	// Bracketed-paste lane (SWEEPFIX S5): pasted content is DATA, never
	// commands. Without this lane every pasted line streamed through
	// commitLine, and a pasted "!git clean -fdx" (a chat transcript, a
	// jupyter cell, a markdown image "![...](url)") was refused as a
	// command by the steering intake, queued, and shell-EXECUTED by the
	// replay funnel after the run. pasteBuf/pastes are mu-guarded;
	// `pasting` is written only by the loop goroutine but READ by drain
	// and by tests, so it is atomic (round-3 self-check: the ESC arm
	// wrote it unguarded while readers held mu — a real race the
	// detector caught before this batch shipped).
	pasting  atomic.Bool
	pasteBuf []byte
	pastes   []string
}

const (
	runInputWindowTick     = 100 * time.Millisecond
	runInputWindowMaxLineB = 1 << 20 // same cap family as the non-TTY listener
	runInputWindowMaxQueue = 32      // same cap as PIB-2 v1
	runInputEsc            = 0x1b
	runInputCtrlC          = 0x03
)

// borrowRunInput opens the run-phase window. readByte is injectable
// for tests; production passes nil to use the platform fd primitive.
func (o *ttyStdinOwner) borrowRunInput(cb runInputWindowCallbacks, readByte func() (byte, bool, error)) (*runInputWindow, error) {
	// REGRESSION FIX (customer report 2026-07-29: dock rows leaking
	// into permanent scrollback, frozen stale status frames): the run
	// window must NOT use full raw mode — term.MakeRaw clears OPOST,
	// which staircases every renderer newline while the dock repaints
	// with cursor-up arithmetic. cbreak keeps OPOST (renderer output
	// sane)
	// and ISIG (Ctrl+C stays a real SIGINT, the pre-window path).
	reader, release, err := o.borrowWithMode(o.makeRunMode)
	if err != nil {
		return nil, err
	}
	w := &runInputWindow{
		release: release,
		owner:   o,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if readByte == nil {
		readByte = func() (byte, bool, error) {
			return readByteFDWithTimeout(reader, o.fd, runInputWindowTick)
		}
		w.unreadByte = reader.UnreadByte
	}
	go w.loop(cb, readByte)
	return w, nil
}

func (w *runInputWindow) loop(cb runInputWindowCallbacks, readByte func() (byte, bool, error)) {
	defer close(w.done)
	for {
		select {
		case <-w.stop:
			return
		default:
		}
		b, ok, err := readByte()
		if w.dead.Load() {
			// Drain started while we were parked: touch nothing — but a
			// byte we already consumed is the user's keystroke, not
			// ours to drop (SWEEPFIX S1): hand it back to the shared
			// reader as the next window's type-ahead.
			if ok && w.unreadByte != nil {
				_ = w.unreadByte()
			}
			return
		}
		if err != nil {
			if err == io.EOF {
				return
			}
			// Transient poll error (or a bad fd in degraded
			// environments): back off one tick rather than spinning.
			select {
			case <-w.stop:
				return
			case <-time.After(runInputWindowTick):
			}
			continue
		}
		if !ok {
			continue // tick expired, no data
		}
		if w.pasting.Load() && b != runInputEsc {
			// Inside a bracketed paste every byte is literal content
			// (only ESC can open the 201~ terminator).
			w.mu.Lock()
			if len(w.pasteBuf) < runInputWindowMaxLineB {
				w.pasteBuf = append(w.pasteBuf, b)
			} else {
				w.overCap = true
			}
			w.mu.Unlock()
			continue
		}
		switch b {
		case runInputCtrlC:
			if cb.onCtrlC != nil && !w.dead.Load() {
				cb.onCtrlC()
			}
		case runInputEsc:
			w.handleEsc(cb, readByte)
		case '\r', '\n':
			w.commitLine(cb)
		case 0x7f, 0x08:
			// REGFIX-A #5: backspace edits the pending line instead of
			// landing as a raw byte inside the queued/steering text.
			w.mu.Lock()
			if n := len(w.partial); n > 0 {
				// Trim one whole rune (UTF-8 continuation bytes).
				n--
				for n > 0 && w.partial[n]&0xC0 == 0x80 {
					n--
				}
				w.partial = w.partial[:n]
			}
			w.mu.Unlock()
		default:
			if b < 0x20 && b != '\t' {
				continue // other control bytes: ignore in run mode
			}
			w.mu.Lock()
			if len(w.partial) < runInputWindowMaxLineB {
				w.partial = append(w.partial, b)
			} else {
				w.overCap = true
			}
			w.mu.Unlock()
		}
	}
}

// handleEsc disambiguates a bare ESC (abort gesture) from escape
// SEQUENCES (arrow keys, alt+key): aborting a Run must require an
// unambiguous bare ESC — a stray arrow key press during a run must
// never cancel it. Bare = no follow-up byte within one tick; CSI/SS3
// follow-ups are swallowed to their terminator; anything else is
// swallowed conservatively without aborting.
func (w *runInputWindow) handleEsc(cb runInputWindowCallbacks, readByte func() (byte, bool, error)) {
	b, ok, err := readByte()
	if w.dead.Load() {
		if ok && w.unreadByte != nil {
			_ = w.unreadByte() // SWEEPFIX S1: same hand-back as the loop
		}
		return
	}
	if err != nil {
		// REGFIX-A #4: a poll error (EINTR from a resize storm, a
		// degraded fd) is NOT evidence of a bare ESC. Cancelling the
		// user's in-flight run on a signal artifact is unacceptable;
		// swallow the ESC instead.
		return
	}
	if !ok {
		if w.pasting.Load() {
			// A lone ESC inside pasted content is literal data.
			w.appendPasteLocked(runInputEsc)
			return
		}
		w.mu.Lock()
		w.escSeen = true
		w.mu.Unlock()
		if cb.onEsc != nil {
			cb.onEsc()
		}
		return
	}
	if b == '[' || b == 'O' {
		seq := make([]byte, 0, 16)
		retries := 0
		for len(seq) < 16 {
			c, ok2, err2 := readByte()
			if err2 != nil {
				return
			}
			if !ok2 {
				// ROUND-3 R3: a paste MARKER split across a tick
				// boundary (laggy link) must not be dropped — a lost
				// 201~ terminator would leave the window stuck in
				// paste mode swallowing all input for the rest of the
				// run. Retry a bounded budget when the bytes so far
				// could still become a marker; give up otherwise
				// (non-marker sequences keep the old prompt-abort
				// cadence).
				if retries < 20 && b == '[' && pasteMarkerPrefix(seq) {
					retries++
					continue
				}
				return
			}
			seq = append(seq, c)
			if c >= '@' && c <= '~' {
				break
			}
		}
		// SWEEPFIX S5: bracketed-paste markers open/close the paste
		// lane; every other sequence inside a paste is literal content.
		if b == '[' && string(seq) == "200~" {
			w.pasting.Store(true)
			return
		}
		if b == '[' && string(seq) == "201~" {
			w.finishPaste(cb)
			return
		}
		if w.pasting.Load() {
			w.appendPasteLocked(append([]byte{runInputEsc, b}, seq...)...)
		}
		return
	}
	if w.pasting.Load() {
		w.appendPasteLocked(runInputEsc, b)
	}
}

// pasteMarkerPrefix reports whether the CSI bytes collected so far are
// a proper prefix of a bracketed-paste marker body ("200~" / "201~").
func pasteMarkerPrefix(seq []byte) bool {
	if len(seq) >= 4 {
		return false
	}
	s := string(seq)
	return strings.HasPrefix("200~", s) || strings.HasPrefix("201~", s)
}

// appendPasteLocked appends literal paste bytes under the same 1MB cap
// the plain-byte arm enforces (ROUND-3 R2: the ESC-sequence literal
// arms bypassed the cap, so escape-dense pastes grew pasteBuf without
// bound). Caller holds no lock; this takes mu.
func (w *runInputWindow) appendPasteLocked(b ...byte) {
	w.mu.Lock()
	if len(w.pasteBuf)+len(b) <= runInputWindowMaxLineB {
		w.pasteBuf = append(w.pasteBuf, b...)
	} else {
		w.overCap = true
	}
	w.mu.Unlock()
}

// finishPaste closes the bracketed-paste lane: the whole blob is
// steered as ONE note when the sink accepts prose, else stored for
// VERBATIM replay (pasted content never enters the command funnel).
// Same linearization contract as commitLine: everything under mu, dead
// first (a dead window keeps the buffer for drain's snapshot).
func (w *runInputWindow) finishPaste(cb runInputWindowCallbacks) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pasting.Store(false)
	if w.dead.Load() {
		return
	}
	blob := strings.Trim(string(w.pasteBuf), "\r\n")
	w.pasteBuf = w.pasteBuf[:0]
	if blob == "" {
		return
	}
	if cb.trySteer != nil && cb.trySteer(blob) {
		if cb.onSteered != nil {
			cb.onSteered(blob)
		}
		return
	}
	if len(w.pastes) >= 8 {
		w.overCap = true
		return
	}
	w.pastes = append(w.pastes, blob)
	if cb.onPaste != nil {
		cb.onPaste(strings.Count(blob, "\n") + 1)
	}
}

// takePastes hands back completed paste blobs plus any half-received
// paste (drain-time). Exactly-once, mu-safe.
func (w *runInputWindow) takePastes() []string {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.pastes
	w.pastes = nil
	if blob := strings.Trim(string(w.pasteBuf), "\r\n"); blob != "" {
		out = append(out, blob)
	}
	w.pasteBuf = nil
	return out
}

func (w *runInputWindow) commitLine(cb runInputWindowCallbacks) {
	// SWEEPFIX S0 (sweep 2026-07-30): the whole commit runs under mu
	// with the dead check FIRST. The previous shape snapshotted and
	// cleared partial, THEN checked dead — an Enter racing drain()
	// passed the loop-level check, cleared partial, and the dead check
	// discarded the snapshot: the typed line was neither steered, nor
	// queued, nor returned by drain as prefill. Now a dead window
	// leaves partial intact for drain's snapshot (prefill lane), and a
	// live commit completes atomically before drain's snapshot can run
	// (drain takes mu only after the goroutine exits or under the fuse)
	// — either way the line survives. Callbacks under mu are safe:
	// steering intake / renderer never call back into the window.
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.dead.Load() {
		return
	}
	line := strings.TrimSpace(string(w.partial))
	w.partial = w.partial[:0]
	if line == "" {
		return
	}
	if cb.trySteer != nil && cb.trySteer(line) {
		if cb.onSteered != nil {
			cb.onSteered(line)
		}
		return
	}
	if len(w.queue) >= runInputWindowMaxQueue {
		w.overCap = true
		return
	}
	w.queue = append(w.queue, line)
	if cb.onLine != nil {
		cb.onLine(line)
	}
}

// drain stops the goroutine, restores cooked mode, and hands back the
// window state: queued lines, any half-typed partial line (to become
// the next prompt's prefill — never silently dropped), whether esc
// aborted, and whether any input was dropped by the caps.
func (w *runInputWindow) drain() (queued []string, partial string, escAborted, dropped bool) {
	if w == nil {
		return nil, "", false, false
	}
	// REGFIX-A #0: invalidate BEFORE signalling so a goroutine parked
	// in readByte can never act on the byte it eventually receives.
	w.dead.Store(true)
	close(w.stop)
	// Belt: the unix tick loop can never park in Read for more than
	// one tick, but if a platform read ever wedges, the REPL must NOT
	// hang here (customer report 2026-07-29: frozen 整理上下文中).
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
		logging.Error("[repl] run input window reader did not stop within 3s; window abandoned (callbacks disarmed; borrow released when the reader unwedges)")
		// SWEEPFIX S2: return the terminal mode NOW, while this drain
		// still owns the lane — deferring the restore to unwedge time
		// let it fire underneath whatever input lane (e.g. the
		// bubbletea fallback editor) owned the terminal by then,
		// snapping it to cooked mode mid-edit.
		if w.owner != nil {
			w.owner.restoreModeEarly()
		}
		// REGFIX-A #1: free the borrow as soon as the goroutine DOES
		// exit, so one transient stall cannot brick the owner for the
		// session (every later borrow would fail busy and type-ahead
		// would be stranded). Safe because dead=true already disarmed
		// every callback, and the mode was restored above — release()
		// finds activeRestore nil and only frees.
		go func() {
			<-w.done
			w.release()
		}()
		w.mu.Lock()
		defer w.mu.Unlock()
		queued = append([]string(nil), w.queue...)
		partial = strings.TrimSpace(string(w.partial))
		// Hand the partial back exactly once: clear it so a late
		// commitLine cannot double-deliver the same text.
		w.partial = w.partial[:0]
		return queued, partial, w.escSeen, w.overCap
	}
	w.release()
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.queue...), strings.TrimSpace(string(w.partial)), w.escSeen, w.overCap
}

// armRunInputWindow opens the run-phase window on the real TTY lane.
// Nil (skip) when scripted, stdin is not a terminal, or the owner
// window cannot be borrowed — the Run proceeds exactly as before.
func (r *REPL) armRunInputWindow(canceller runnerCanceller) *runInputWindow {
	if !r.interactive() || canceller == nil || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	// Fuse: operators can disable the run-phase input window outright
	// (regression insurance after the 2026-07-29 dock-corruption
	// report; pre-window behavior = no reader during runs).
	if os.Getenv("CODRAX_DISABLE_RUN_INPUT") != "" {
		return nil
	}
	steerSink, _ := r.runner.(steeringSink)
	cb := runInputWindowCallbacks{
		onLine: func(line string) {
			if r.renderer != nil {
				r.renderer.CommitUserInputLine(line)
			}
		},
		trySteer: func(line string) bool {
			// SWEEPFIX S4: prompt-TEMPLATE commands are refused here —
			// the template registry lives on the REPL, not in the
			// intake — so "/mytemplate args" queues for replay
			// expansion instead of being steered as prose.
			if _, _, isTemplate := r.expandTemplateCommand(line); isTemplate {
				return false
			}
			return steerSink != nil && steerSink.PushSteeringNote(line)
		},
		onPaste: func(lineCount int) {
			if r.renderer != nil {
				r.renderer.CommitUserInputLine(runInputPasteQueuedLabel(r.language, lineCount))
			}
		},
		onSteered: func(line string) {
			// REGFIX-B #13 (sweep): the intake ACCEPTING a note is not
			// the same as an explore boundary CONSUMING it — the intake
			// stays open through extract/finalize and write-mode runs
			// have no explore boundary at all. Claiming "injected into
			// this exploration" at accept time was an over-promise; the
			// honest echo says accepted-for-this-run, and the drain path
			// replays anything that was never consumed.
			if r.renderer != nil {
				r.renderer.CommitUserSteeredLine(line)
			}
		},
		onCtrlC: func() {
			// Byte → signal: the one installed SIGINT handler owns
			// single-tap/double-tap semantics. Windows (no self-kill)
			// falls back to direct single-tap cancel.
			if !raiseSelfSIGINT() {
				canceller.Cancel("Ctrl+C")
				r.cancelTurn()
			}
		},
		onEsc: func() {
			canceller.Cancel("esc")
			r.cancelTurn()
		},
	}
	w, err := r.ttyStdinOwnerInstance().borrowRunInput(cb, nil)
	if err == nil {
		// ROUND-3 SWEEP R0 (high): the S5 paste lane was UNREACHABLE in
		// production — bracketed-paste mode (DECSET 2004) was enabled
		// only inside the prompt editor and disabled on prompt exit, so
		// no terminal ever sent ESC[200~/201~ during a run. The window
		// owns the mode for the run's lifetime: enabled here AFTER the
		// borrow succeeds (round-4 R1: enabling before the borrow
		// leaked the mode on every borrow-failure platform — Windows
		// always fails makeRunInputMode — with no reader armed and no
		// disable), disabled at drain (the single exit, fuse-abandon
		// included) and by restoreTTYForExit as the hard-kill belt.
		fmt.Fprint(r.out, ansiEnableBracketed)
	}
	if err != nil {
		logging.Debug("[repl] run input window unavailable: %v", err)
		return nil
	}
	return w
}

// drainRunInputWindow hands the window state to the replay surfaces:
// esc-aborted input is RESTORED (single line → next prompt prefill;
// multiple lines → the paste placeholder lane) instead of replayed;
// normal completion queues lines into pendingFollowUps (the same
// replay path the non-TTY listener feeds) and a half-typed partial
// line becomes the next prompt's prefill. Cap overflow is disclosed.
func (r *REPL) drainRunInputWindow(w *runInputWindow) {
	if w == nil {
		return
	}
	fmt.Fprint(r.out, ansiDisableBracketed) // R0: paired with armRunInputWindow's enable
	queued, partial, escAborted, dropped := w.drain()
	pastes := w.takePastes()
	// TTY-3: steering notes accepted by the run but never consumed
	// (run ended first, or a lane without explore boundaries) come
	// back here — steered or replayed, never lost, never run twice.
	// SWEEPFIX S6: replay order is window-queued lines FIRST, then the
	// unconsumed steering prose. The previous prepend inverted a typed
	// "command then prose" sequence; commands typically change the
	// mode/context the subsequent prose assumes, so commands-first is
	// the honest default (recorded in the design ledger — true typed
	// interleaving is not reconstructible from two unordered lanes).
	var rem []string
	if taker, ok := r.runner.(steeringUnconsumed); ok {
		rem = taker.TakeUnconsumedSteeringNotes()
	}
	if dropped {
		r.warn("%s\n", runInputDroppedMsg(r.language))
	}
	if escAborted {
		restored := append(append([]string(nil), queued...), rem...)
		restored = append(restored, pastes...)
		restored = append(restored, splitNonEmpty(partial)...)
		switch {
		case len(restored) == 0:
		case len(restored) == 1 && !strings.Contains(restored[0], "\n"):
			r.pendingInputPrefill = restored[0]
		default:
			// Multi-line restore rides the existing paste-placeholder
			// lane: folded in the editor, expanded on submit. ROUND-3
			// R1: a SINGLE restored entry can itself be a multiline
			// paste blob now — raw \n runes corrupt the single-line
			// raw-mode editor, so anything multiline folds here too.
			r.pendingPaste = strings.Join(restored, "\n")
		}
		if len(restored) > 0 {
			r.info(runInputRestoredMsg(r.language, len(restored)))
		}
		return
	}
	for _, line := range queued {
		r.pendingFollowUps = append(r.pendingFollowUps, pendingFollowUp{Text: line})
	}
	for _, note := range rem {
		// Accepted-as-prose steering replays as prose — never through
		// the command funnel (a template name colliding with the first
		// word must not fire on replay).
		r.pendingFollowUps = append(r.pendingFollowUps, pendingFollowUp{Text: note, Verbatim: true})
	}
	for _, blob := range pastes {
		r.pendingFollowUps = append(r.pendingFollowUps, pendingFollowUp{Text: blob, Verbatim: true})
	}
	if partial != "" {
		r.pendingInputPrefill = partial
	}
}

// pendingFollowUp is one queued replay unit. Verbatim entries (pasted
// blobs, unconsumed steering prose) bypass the command funnel on
// replay — they are data, not typed commands (SWEEPFIX S5/S6).
type pendingFollowUp struct {
	Text     string
	Verbatim bool
}

// steeringSink / steeringUnconsumed are the optional runner surfaces
// for mid-run steering (TTY-3); test stubs without them fall back to
// pure queueing.
type steeringSink interface {
	PushSteeringNote(note string) bool
}

type steeringUnconsumed interface {
	TakeUnconsumedSteeringNotes() []string
}

// runInputPasteQueuedLabel is the folded echo for a paste captured
// mid-run and queued for verbatim replay.
func runInputPasteQueuedLabel(lang string, lines int) string {
	if isZh(lang) {
		return fmt.Sprintf("[粘贴 %d 行，已排队待运行结束后处理]", lines)
	}
	return fmt.Sprintf("[pasted %d line(s), queued until the run completes]", lines)
}

func splitNonEmpty(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []string{s}
}

func runInputDroppedMsg(lang string) string {
	if isZh(lang) {
		return "运行期输入超出队列上限,超出部分已丢弃"
	}
	return "Run-phase input exceeded the queue cap; overflow was dropped"
}

func runInputRestoredMsg(lang string, n int) string {
	if isZh(lang) {
		return "已取消本轮;运行期输入已还原到编辑器(" + itoa(n) + " 条)"
	}
	return "Run cancelled; run-phase input restored to the editor (" + itoa(n) + " line(s))"
}
