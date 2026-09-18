package repl

import (
	"io"
	"sync"
)

// Keep the historical command/queue tests, but exercise the shared owner.
// Production has no per-Run Scanner or second input-reader entry point.
type cancelListener struct {
	owner   *scriptInputOwner
	lease   *scriptInputLease
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	stopped bool
	queued  []string
}

func startCancelListener(in io.Reader, canceller runnerCanceller, warn func(string, ...any)) *cancelListener {
	if canceller == nil {
		return nil
	}
	o := newScriptInputOwner(in, 1024*1024)
	l, err := o.beginRuntime(canceller.Cancel, warn)
	if err != nil {
		panic(err)
	}
	return &cancelListener{owner: o, lease: l, done: make(chan struct{})}
}

func startCancelListenerForREPL(in io.Reader, interactive bool, canceller runnerCanceller, warn func(string, ...any)) *cancelListener {
	if interactive {
		return nil
	}
	return startCancelListener(in, canceller, warn)
}

func (cl *cancelListener) stop() {
	if cl == nil {
		return
	}
	cl.once.Do(func() {
		cl.mu.Lock()
		defer cl.mu.Unlock()
		cl.queued = cl.lease.stopAndDrain().lines
		cl.stopped = true
		cl.owner.close()
		close(cl.done)
	})
}

func (cl *cancelListener) queuedLines() []string {
	if cl == nil {
		return nil
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.stopped {
		return append([]string(nil), cl.queued...)
	}
	cl.owner.mu.Lock()
	defer cl.owner.mu.Unlock()
	return append([]string(nil), cl.lease.queue...)
}
