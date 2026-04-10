package memory

import (
	"fmt"
	"os"
)

// fileLock is an OS-level advisory file lock used to serialize
// MEMORY.md mutations across multiple codrax processes that share one
// memory directory (the multi-instance scenario unlocked by per-repo
// path slugging in main.go).
//
// On Unix the underlying primitive is flock(2); on Windows it is
// LockFileEx. Both are advisory and per-open-file-description, which
// means a single process holding two independent fds on the same lock
// file will serialize against itself just as cleanly as two processes
// would. That property is what makes the in-process unit tests in
// store_concurrency_test.go faithful to the cross-process case.
//
// The lock file lives at <dir>/MEMORY.md.lock and is opened once for
// the lifetime of the Store. Clear() does not touch it, so the fd
// stays valid across the entire process. Closing the Store (or letting
// the process exit) releases everything.
type fileLock struct {
	f *os.File
}

// newFileLock opens (creating if missing) the lock file at path. The
// returned lock starts unheld; callers must call lock or rlock before
// the critical section and unlock after.
func newFileLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}
	return &fileLock{f: f}, nil
}

// close releases any held lock and closes the underlying fd. Safe to
// call on a nil receiver.
func (l *fileLock) close() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = l.unlock() // best-effort; OS releases on close anyway
	err := l.f.Close()
	l.f = nil
	return err
}
