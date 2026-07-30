package repl

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
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
	if r == nil || r.stdinOwnerInst == nil {
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
	onLine func(line string)
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
	stop    chan struct{}
	done    chan struct{}

	mu      sync.Mutex
	queue   []string
	partial []byte
	escSeen bool
	overCap bool
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
	if readByte == nil {
		readByte = func() (byte, bool, error) {
			return readByteFDWithTimeout(reader, o.fd, runInputWindowTick)
		}
	}
	w := &runInputWindow{
		release: release,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
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
		switch b {
		case runInputCtrlC:
			if cb.onCtrlC != nil {
				cb.onCtrlC()
			}
		case runInputEsc:
			w.handleEsc(cb, readByte)
		case '\r', '\n':
			w.commitLine(cb)
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
	if err != nil || !ok {
		w.mu.Lock()
		w.escSeen = true
		w.mu.Unlock()
		if cb.onEsc != nil {
			cb.onEsc()
		}
		return
	}
	if b == '[' || b == 'O' {
		for i := 0; i < 16; i++ {
			c, ok2, err2 := readByte()
			if err2 != nil || !ok2 {
				return
			}
			if c >= '@' && c <= '~' {
				return
			}
		}
	}
}

func (w *runInputWindow) commitLine(cb runInputWindowCallbacks) {
	w.mu.Lock()
	line := strings.TrimSpace(string(w.partial))
	w.partial = w.partial[:0]
	w.mu.Unlock()
	if line != "" && cb.trySteer != nil && cb.trySteer(line) {
		if cb.onSteered != nil {
			cb.onSteered(line)
		}
		return
	}
	w.mu.Lock()
	tooMany := len(w.queue) >= runInputWindowMaxQueue
	if line != "" && !tooMany {
		w.queue = append(w.queue, line)
	}
	if tooMany {
		w.overCap = true
	}
	w.mu.Unlock()
	if line != "" && !tooMany && cb.onLine != nil {
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
	close(w.stop)
	// Belt: the unix tick loop can never park in Read for more than
	// one tick, but if a platform read ever wedges, the REPL must NOT
	// hang here (customer report 2026-07-29: frozen 整理上下文中).
	// On timeout we abandon the goroutine WITHOUT releasing the
	// borrow — the shared reader stays owned so no second consumer
	// can race the parked one; subsequent prompts fall back to the
	// bubbletea lane. Loud ERROR so the degradation is diagnosable.
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
		logging.Error("[repl] run input window reader did not stop; abandoning window (input degraded to fallback editor for this session)")
		w.mu.Lock()
		defer w.mu.Unlock()
		return append([]string(nil), w.queue...), strings.TrimSpace(string(w.partial)), w.escSeen, w.overCap
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
			return steerSink != nil && steerSink.PushSteeringNote(line)
		},
		onSteered: func(line string) {
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
	queued, partial, escAborted, dropped := w.drain()
	// TTY-3: steering notes accepted by the run but never consumed
	// (run ended first, or a lane without explore boundaries) come
	// back here — steered or replayed, never lost, never run twice.
	if taker, ok := r.runner.(steeringUnconsumed); ok {
		if rem := taker.TakeUnconsumedSteeringNotes(); len(rem) > 0 {
			queued = append(rem, queued...)
		}
	}
	if dropped {
		r.warn("%s\n", runInputDroppedMsg(r.language))
	}
	if escAborted {
		restored := append(append([]string(nil), queued...), splitNonEmpty(partial)...)
		switch len(restored) {
		case 0:
		case 1:
			r.pendingInputPrefill = restored[0]
		default:
			// Multi-line restore rides the existing paste-placeholder
			// lane: folded in the editor, expanded on submit.
			r.pendingPaste = strings.Join(restored, "\n")
		}
		if len(restored) > 0 {
			r.info(runInputRestoredMsg(r.language, len(restored)))
		}
		return
	}
	if len(queued) > 0 {
		r.pendingFollowUps = append(r.pendingFollowUps, queued...)
	}
	if partial != "" {
		r.pendingInputPrefill = partial
	}
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
