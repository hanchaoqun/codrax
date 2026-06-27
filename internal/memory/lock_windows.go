//go:build windows

package memory

import (
	"syscall"
	"unsafe"
)

// Windows file locking goes through kernel32.dll!LockFileEx and
// !UnlockFileEx, which the stdlib syscall package does not export
// in Go 1.22. Using golang.org/x/sys/windows would solve this in one
// line but adds a non-stdlib dependency, which violates this
// project's "only gopkg.in/yaml.v3" rule. The LazyDLL approach is
// pure stdlib and equivalent at the syscall layer.
var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

// Windows file-locking constants. Defined locally because the stdlib
// syscall package does not export them either.
const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
	errorLockViolation      = 33 // ERROR_LOCK_VIOLATION
)

// callLockFileEx is a thin wrapper around the kernel32 LockFileEx
// stdcall. The argument order matches the Windows API:
//
//	BOOL LockFileEx(
//	  HANDLE       hFile,
//	  DWORD        dwFlags,
//	  DWORD        dwReserved,
//	  DWORD        nNumberOfBytesToLockLow,
//	  DWORD        nNumberOfBytesToLockHigh,
//	  LPOVERLAPPED lpOverlapped
//	);
//
// A non-zero return means success; zero means the call failed and
// the syscall.Errno from the OS is in e1.
func callLockFileEx(handle syscall.Handle, flags, reserved, lockLow, lockHigh uint32, ol *syscall.Overlapped) error {
	r1, _, e1 := procLockFileEx.Call(
		uintptr(handle),
		uintptr(flags),
		uintptr(reserved),
		uintptr(lockLow),
		uintptr(lockHigh),
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		if e1 != nil {
			return e1
		}
		return syscall.EINVAL
	}
	return nil
}

// callUnlockFileEx mirrors callLockFileEx for the unlock side.
func callUnlockFileEx(handle syscall.Handle, reserved, lockLow, lockHigh uint32, ol *syscall.Overlapped) error {
	r1, _, e1 := procUnlockFileEx.Call(
		uintptr(handle),
		uintptr(reserved),
		uintptr(lockLow),
		uintptr(lockHigh),
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		if e1 != nil {
			return e1
		}
		return syscall.EINVAL
	}
	return nil
}

// lockWhole locks the entire file. Passing all-ones for both halves
// of the length yields the maximum 64-bit length, which is the
// documented way to "lock the rest of the file".
func (l *fileLock) lockWhole(flags uint32) error {
	var ol syscall.Overlapped
	return callLockFileEx(syscall.Handle(l.f.Fd()), flags, 0, 0xFFFFFFFF, 0xFFFFFFFF, &ol)
}

// lock acquires an exclusive (writer) lock, blocking until it is held.
func (l *fileLock) lock() error {
	return l.lockWhole(lockfileExclusiveLock)
}

// rlock acquires a shared (reader) lock, blocking until it is held.
// Passing 0 flags asks for a shared lock per LockFileEx documentation.
func (l *fileLock) rlock() error {
	return l.lockWhole(0)
}

// tryRLock attempts to acquire a shared lock without blocking. It mirrors
// tryLock but without LOCKFILE_EXCLUSIVE_LOCK.
func (l *fileLock) tryRLock() (bool, error) {
	err := l.lockWhole(lockfileFailImmediately)
	if err == nil {
		return true, nil
	}
	if errno, ok := err.(syscall.Errno); ok && errno == errorLockViolation {
		return false, nil
	}
	return false, err
}

// unlock releases whatever lock the fd currently holds. UnlockFileEx
// takes the same range we passed to LockFileEx.
func (l *fileLock) unlock() error {
	var ol syscall.Overlapped
	return callUnlockFileEx(syscall.Handle(l.f.Fd()), 0, 0xFFFFFFFF, 0xFFFFFFFF, &ol)
}

// tryLock attempts to acquire an exclusive lock without blocking.
// Returns (true, nil) on success, (false, nil) if another holder
// already has an incompatible lock, or (false, err) on a real I/O
// error. The Windows distinction between "would block" and "real
// error" comes from ERROR_LOCK_VIOLATION (33), which LockFileEx
// reports when LOCKFILE_FAIL_IMMEDIATELY is set and the file is
// already locked.
func (l *fileLock) tryLock() (bool, error) {
	err := l.lockWhole(lockfileExclusiveLock | lockfileFailImmediately)
	if err == nil {
		return true, nil
	}
	if errno, ok := err.(syscall.Errno); ok && errno == errorLockViolation {
		return false, nil
	}
	return false, err
}

// downgradeToShared transitions an exclusive lock to a shared lock.
// Windows LockFileEx has no atomic downgrade, so we unlock then
// re-acquire as shared. There is a brief window where the file is
// unlocked. NewStore tolerates this window because it only occurs
// after orphan recovery has finished and before any Append has
// started, so a peer that grabs the lock during the window observes
// the same post-recovery state and produces the same orphan-empty
// behavior.
func (l *fileLock) downgradeToShared() error {
	if err := l.unlock(); err != nil {
		return err
	}
	return l.lockWhole(0)
}
