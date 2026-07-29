package repl

import (
	"bufio"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/term"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// cancelListener watches stdin for `/cancel` lines while a Run is in
// flight and drives runner.Cancel("/cancel") when one arrives. It is
// the typed-command counterpart to the SIGINT handler — same exit
// path (CancelToken → checkpoint unwind), same user-facing rendering.
//
// Scope: started ONLY when stdin is non-TTY (pipe / redirect /
// scripted tests). In an interactive TTY, bubbletea owns the input
// box during prompts and closes it during Run; a concurrent reader
// would race with bubbletea's next prompt and eat the user's keys.
// TTY operators get the SIGINT path; non-TTY callers (CI scripts,
// expect-style automation, future IPC pumps that pipe lines into
// stdin) get the typed-command path.
//
// Lifecycle: each Run() invocation is wrapped by runInFlightWrap,
// which calls Start() before fn and Stop() after. Stop is idempotent
// + non-blocking — the goroutine exits when the next Scan() call
// returns false (typically EOF on a piped producer that finished
// sending data). For long-lived pipes that stay open across Runs the
// goroutine stays parked on Read. (An earlier draft mentioned a
// listenerActive guard; the actual reuse protection is the per-Run
// Start/Stop pairing in runInFlightWrap.)
type cancelListener struct {
	scanner   *bufio.Scanner
	canceller runnerCanceller
	warn      func(format string, args ...any)
	stopped   atomic.Bool
	done      chan struct{}
	once      sync.Once

	// queued collects non-/cancel lines typed (or piped) while the
	// Run is in flight (PIB-2 v1, ledger
	// docs/design/pi_borrow_analysis_20260729.md §7.6): instead of
	// warn-and-drop, they become follow-up turns the REPL replays in
	// order after the Run completes — pi's follow-up queue semantics
	// on the non-TTY lane. Capped so a runaway pipe cannot balloon.
	queueMu sync.Mutex
	queued  []string
}

// cancelListenerQueueCap bounds the follow-up queue; beyond it lines
// are dropped with the one-shot warn (precise, disclosed).
const cancelListenerQueueCap = 32

// queuedLines returns the collected follow-up lines in arrival order.
func (cl *cancelListener) queuedLines() []string {
	if cl == nil {
		return nil
	}
	cl.queueMu.Lock()
	defer cl.queueMu.Unlock()
	return append([]string(nil), cl.queued...)
}

// startCancelListener returns a non-nil listener when stdin matches
// the non-TTY criterion AND the runner exposes the cancellation
// surface AND no listener is already active. nil return means the
// caller should fall through to the SIGINT-only path. The optional
// stdin parameter overrides os.Stdin — used by unit tests to feed a
// strings.Reader / *os.File pipe deterministically.
func startCancelListener(stdin io.Reader, canceller runnerCanceller, warn func(format string, args ...any)) *cancelListener {
	if canceller == nil {
		return nil
	}
	src := stdin
	if src == nil {
		src = os.Stdin
	}
	// TTY check: only the production *os.File path can ask the
	// kernel; injected io.Readers (tests) bypass and always start
	// the listener since the test harness owns input ordering.
	if f, ok := src.(*os.File); ok {
		if term.IsTerminal(int(f.Fd())) {
			return nil
		}
	}
	cl := &cancelListener{
		scanner:   bufio.NewScanner(src),
		canceller: canceller,
		warn:      warn,
		done:      make(chan struct{}),
	}
	// Larger buffer than default 64 KiB to tolerate scripted
	// callers that paste long lines before /cancel (jest fixture
	// outputs etc.). Buffer cap matters only for over-long single
	// lines; typical /cancel arrives well under 1 KiB.
	cl.scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	go cl.loop()
	return cl
}

// startCancelListenerForREPL is the REPL-facing policy gate over the low-level
// listener. In interactive mode (r.in == nil), stdin belongs to the prompt
// implementation and must never have a background reader, even if the host
// terminal/ConPTY reports as non-TTY. Ctrl+C covers interactive cancellation.
func startCancelListenerForREPL(stdin io.Reader, interactive bool, canceller runnerCanceller, warn func(format string, args ...any)) *cancelListener {
	if interactive {
		return nil
	}
	return startCancelListener(stdin, canceller, warn)
}

// stop signals the listener to wind down. Idempotent. The goroutine
// exits at the next Scan() return — for piped producers that are
// done, this is immediate (EOF); for long-lived producers, the
// stopped flag causes the next /cancel line to be ignored AND the
// goroutine exits as soon as Scan returns false. Production callers
// always defer this in runInFlightWrap; closing channel done lets
// tests await graceful shutdown via Wait().
func (cl *cancelListener) stop() {
	if cl == nil {
		return
	}
	cl.stopped.Store(true)
	cl.once.Do(func() {
		// Close done so any waiter unblocks. The Read in loop()
		// stays blocked until stdin produces more data or EOF;
		// that's a cooperative limit accepted by design (TTY
		// operators use the SIGINT path).
		close(cl.done)
	})
}

// loop is the goroutine body. Reads stdin line-by-line, matches the
// `/cancel` or `\cancel` shape (leading/trailing whitespace tolerated)
// and fires runner.Cancel. Anything else is dropped with a one-shot
// warn so a misdirected paste doesn't flood the terminal.
func (cl *cancelListener) loop() {
	warned := false
	for cl.scanner.Scan() {
		if cl.stopped.Load() {
			return
		}
		raw := strings.TrimSpace(cl.scanner.Text())
		if raw == "" {
			continue
		}
		lower := strings.ToLower(raw)
		if lower == "/cancel" || lower == "\\cancel" ||
			strings.HasPrefix(lower, "/cancel ") ||
			strings.HasPrefix(lower, "\\cancel ") {
			cl.canceller.Cancel("/cancel")
			logging.Info("[repl] /cancel via stdin listener — cancel signal sent")
			return
		}
		// Non-cancel input during Run: queue it as a follow-up turn
		// (PIB-2 v1). A one-shot info line discloses the queueing;
		// beyond the cap lines are dropped with the same one-shot
		// warn so a runaway pipe cannot balloon memory.
		cl.queueMu.Lock()
		underCap := len(cl.queued) < cancelListenerQueueCap
		if underCap {
			cl.queued = append(cl.queued, raw)
		}
		cl.queueMu.Unlock()
		if !warned && cl.warn != nil {
			if underCap {
				cl.warn("[repl] input received during Run queued as follow-up (runs after this turn): %q\n", truncateForWarn(raw, 80))
			} else {
				cl.warn("[repl] follow-up queue full (%d); further input during this Run is dropped\n", cancelListenerQueueCap)
			}
			warned = true
		}
	}
	if err := cl.scanner.Err(); err != nil && !cl.stopped.Load() {
		logging.Debug("[repl] cancel listener scanner err: %v", err)
	}
}

// truncateForWarn keeps the dropped-input warn line tidy. Anything
// longer than max gets a trailing ellipsis; UTF-8 safe (back off to
// a rune boundary). HYG-2 G18: the former hand-rolled backoff loop
// moved to the shared primitive — behavior-identical for max > 0.
func truncateForWarn(s string, max int) string {
	return types.TruncateBytesEllipsis(s, max)
}
