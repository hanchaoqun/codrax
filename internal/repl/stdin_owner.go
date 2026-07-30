package repl

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"sync"

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
	}
}

var errStdinOwnerBusy = errors.New("stdin owner: window already borrowed")

// borrowRaw opens an exclusive raw-mode window over the shared reader.
// The returned release func restores cooked mode and frees the window;
// buffered bytes stay in the shared reader for the next window.
func (o *ttyStdinOwner) borrowRaw() (*bufio.Reader, func(), error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.inUse {
		return nil, nil, errStdinOwnerBusy
	}
	restore, err := o.makeRaw(o.fd)
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
