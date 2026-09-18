package repl

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"sync"
)

// scriptInputOwner is the only reader of a non-TTY REPL input. The pump may
// prefetch one line, but ownership is assigned when a consumer takes the line,
// not when Read returns. Prompt, capture and runtime never compete for Read.
// A borrowed io.Reader cannot be interrupted safely: close revokes delivery,
// without closing the reader or waiting for a blocked Read to return.
type scriptInputOwner struct {
	mu      sync.Mutex
	changed *sync.Cond
	scanner *bufio.Scanner
	line    string
	ready   bool
	ended   bool
	err     error
	closed  bool
	active  *scriptInputLease
	byteCap int
}

type scriptInputLease struct {
	owner   *scriptInputOwner
	revoked bool
	queue   []string
	bytes   int
	dropped int
	warn    func(string, ...any)
}

type scriptInputReceipt struct {
	lines   []string
	dropped int
	byteCap int
}

const scriptInputQueueBytes = 8 * 1024 * 1024

func newScriptInputOwner(in io.Reader, maxLineBytes int) *scriptInputOwner {
	if maxLineBytes <= 0 {
		maxLineBytes = 1024 * 1024
	}
	o := &scriptInputOwner{scanner: bufio.NewScanner(in), byteCap: max(scriptInputQueueBytes, maxLineBytes)}
	o.scanner.Buffer(make([]byte, min(64*1024, maxLineBytes)), maxLineBytes)
	o.changed = sync.NewCond(&o.mu)
	go o.pump()
	return o
}

func (o *scriptInputOwner) pump() {
	for {
		o.mu.Lock()
		for o.ready && !o.closed {
			o.changed.Wait()
		}
		if o.closed {
			o.mu.Unlock()
			return
		}
		o.mu.Unlock()
		// Exactly this goroutine owns Scan, including its read-ahead buffer.
		ok := o.scanner.Scan()
		o.mu.Lock()
		if o.closed {
			o.mu.Unlock()
			return
		}
		if !ok {
			o.ended, o.err = true, o.scanner.Err()
			o.changed.Broadcast()
			o.mu.Unlock()
			return
		}
		o.line, o.ready = o.scanner.Text(), true
		o.changed.Broadcast()
		o.mu.Unlock()
	}
}

func (o *scriptInputOwner) borrow() (*scriptInputLease, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, errors.New("script input is closed")
	}
	if o.active != nil {
		return nil, errors.New("script input already has a consumer")
	}
	l := &scriptInputLease{owner: o}
	o.active = l // The lease pointer is the generation identity; never reused.
	o.changed.Broadcast()
	return l, nil
}

func (o *scriptInputOwner) borrowLines() (captureLineScanner, func()) {
	l, err := o.borrow()
	return &scriptLineScanner{lease: l, err: err}, func() { l.stopAndDrain() }
}

func (o *scriptInputOwner) beginRuntime(cancel func(string), warn func(string, ...any)) (*scriptInputLease, error) {
	l, err := o.borrow()
	if err != nil {
		return nil, err
	}
	l.warn = warn
	go l.consumeRuntime(cancel)
	return l, nil
}

func (l *scriptInputLease) consumeRuntime(cancel func(string)) {
	o := l.owner
	o.mu.Lock()
	defer o.mu.Unlock()
	for {
		for !o.ready && !o.ended && !o.closed && o.active == l {
			o.changed.Wait()
		}
		if o.closed || o.active != l || !o.ready {
			return
		}
		raw := strings.TrimSpace(o.line)
		o.line, o.ready = "", false
		o.changed.Broadcast()
		if raw == "" {
			continue
		}
		if isScriptCancelCommand(raw) {
			// Callback admission and stop share the mutex. Stop cannot return
			// while an admitted callback could still target a later operation.
			// Cancellation callbacks must be short and not reenter this owner.
			cancel("/cancel")
			return // Leave all subsequent lines for the next consumer.
		}
		if len(l.queue) >= cancelListenerQueueCap || len(raw) > o.byteCap-l.bytes {
			l.dropped++
			if l.dropped == 1 && l.warn != nil {
				l.warn("[repl] follow-up queue full (%d lines / %d bytes); further input is dropped, /cancel remains available\n", cancelListenerQueueCap, o.byteCap)
			}
			continue
		}
		l.queue = append(l.queue, raw)
		l.bytes += len(raw)
		if len(l.queue) == 1 && l.warn != nil {
			l.warn("[repl] input received during Run queued as follow-up (runs after this turn): %q\n", truncateForWarn(raw, 80))
		}
	}
}

func isScriptCancelCommand(line string) bool {
	command := strings.ToLower(strings.TrimSpace(line))
	return command == "/cancel" || command == "\\cancel" ||
		strings.HasPrefix(command, "/cancel ") || strings.HasPrefix(command, "\\cancel ")
}

// stopAndDrain atomically revokes this lease and transfers its admitted lines.
// An old/repeated stop cannot revoke a newer consumer or replay a line twice.
func (l *scriptInputLease) stopAndDrain() scriptInputReceipt {
	if l == nil {
		return scriptInputReceipt{}
	}
	o := l.owner
	o.mu.Lock()
	defer o.mu.Unlock()
	if l.revoked {
		return scriptInputReceipt{}
	}
	l.revoked = true
	if o.active == l {
		o.active = nil
	}
	receipt := scriptInputReceipt{lines: l.queue, dropped: l.dropped, byteCap: o.byteCap}
	l.queue = nil
	o.changed.Broadcast()
	return receipt
}

func (o *scriptInputOwner) close() {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.closed = true
	o.line, o.ready = "", false
	o.changed.Broadcast()
	o.mu.Unlock()
}

type scriptLineScanner struct {
	lease *scriptInputLease
	text  string
	err   error
}

func (s *scriptLineScanner) Scan() bool {
	if s.err != nil || s.lease == nil {
		return false
	}
	o := s.lease.owner
	o.mu.Lock()
	defer o.mu.Unlock()
	for !o.ready && !o.ended && !o.closed && o.active == s.lease {
		o.changed.Wait()
	}
	if o.closed || o.active != s.lease {
		s.err = errors.New("script input consumer is no longer active")
		return false
	}
	if !o.ready {
		s.err = o.err
		return false
	}
	s.text = o.line
	o.line, o.ready = "", false
	o.changed.Broadcast()
	return true
}

func (s *scriptLineScanner) Text() string { return s.text }
func (s *scriptLineScanner) Err() error   { return s.err }

func (r *REPL) scriptedInputOwner() *scriptInputOwner {
	if r.scriptInput == nil {
		r.scriptInput = newScriptInputOwner(r.in, r.attachedLogMaxBytes+1)
	}
	return r.scriptInput
}
