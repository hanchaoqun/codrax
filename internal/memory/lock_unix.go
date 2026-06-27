//go:build !windows

package memory

import "syscall"

// lock acquires an exclusive (writer) lock, blocking until it is held.
// Re-entrant calls on the same fd upgrade/downgrade per flock(2)
// semantics, which is good enough for our usage: we never nest
// rlock inside lock or vice versa.
func (l *fileLock) lock() error {
	return syscall.Flock(int(l.f.Fd()), syscall.LOCK_EX)
}

// rlock acquires a shared (reader) lock, blocking until it is held.
func (l *fileLock) rlock() error {
	return syscall.Flock(int(l.f.Fd()), syscall.LOCK_SH)
}

// tryRLock attempts to acquire a shared lock without blocking. It is used by
// foreground prompt-context reads so a slow peer/background compaction cannot
// freeze an interactive REPL turn.
func (l *fileLock) tryRLock() (bool, error) {
	err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if err == syscall.EWOULDBLOCK {
		return false, nil
	}
	return false, err
}

// unlock releases whatever lock the fd currently holds. A no-op when
// nothing is held.
func (l *fileLock) unlock() error {
	return syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
}

// tryLock attempts to acquire an exclusive lock without blocking.
// Returns (true, nil) on success, (false, nil) if another holder
// already has an incompatible lock, or (false, err) on a real I/O
// error. Used by NewStore to decide whether this process is the
// only Store on the directory and may therefore safely recover
// orphan turn files from a previous crashed session.
func (l *fileLock) tryLock() (bool, error) {
	err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if err == syscall.EWOULDBLOCK {
		return false, nil
	}
	return false, err
}

// downgradeToShared atomically transitions an exclusive lock to a
// shared lock. On Unix flock(2) treats a subsequent LOCK_SH on an
// already-EX-held fd as a downgrade with no intervening unlock
// window, which is exactly the property NewStore needs to prevent
// a peer from sneaking in between "I finished orphan recovery" and
// "I am now a shared holder". The Windows implementation has a tiny
// window because LockFileEx has no downgrade primitive.
func (l *fileLock) downgradeToShared() error {
	return syscall.Flock(int(l.f.Fd()), syscall.LOCK_SH)
}
