//go:build unix

package hitraceconv

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func validateExternalToolInputSourcePlatform(conversionInputView) error { return nil }

func validateExternalToolInputSnapshotDirPlatform(*privateConversionDirPlatformState) error {
	return nil
}

func createExternalToolInputSnapshotFilePlatform(state *privateConversionDirPlatformState, name string) (*os.File, error) {
	if state == nil || state.guardFD < 0 {
		return nil, fmt.Errorf("POSIX private snapshot directory authority is incomplete")
	}
	fd, err := unix.Openat(
		state.guardFD,
		name,
		unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC,
		0o600,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		return nil, traceDBJoinPreservingSingle(fmt.Errorf("wrap POSIX external tool snapshot handle"), unix.Close(fd))
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return nil, traceDBJoinPreservingSingle(fmt.Errorf("secure POSIX external tool snapshot: %w", err), file.Close())
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || uint64(stat.Uid) != uint64(os.Geteuid()) {
		if err == nil {
			err = fmt.Errorf("owner/type mismatch: uid=%d mode=%#o effective_uid=%d", stat.Uid, stat.Mode, os.Geteuid())
		}
		return nil, traceDBJoinPreservingSingle(fmt.Errorf("validate POSIX external tool snapshot birth: %w", err), file.Close())
	}
	runtime.KeepAlive(state)
	return file, nil
}

func freezeExternalToolInputSnapshotFilePlatform(
	state *privateConversionDirPlatformState,
	name string,
	writer *os.File,
	created os.FileInfo,
) (*os.File, os.FileInfo, error) {
	if state == nil || state.guardFD < 0 || writer == nil || created == nil {
		if writer != nil {
			_ = writer.Close()
		}
		return nil, nil, fmt.Errorf("POSIX external tool snapshot freeze authority is incomplete")
	}
	current, err := writer.Stat()
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(created, current) {
		if err == nil {
			err = fmt.Errorf("creator handle identity mismatch")
		}
		return nil, nil, traceDBJoinPreservingSingle(err, writer.Close())
	}
	if err := validatePrivateConversionRegularChildPlatform(state, name, writer, current); err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, writer.Close())
	}
	// POSIX has no Windows-style access/share conflict. Keeping the original
	// O_RDWR creator handle removes the close/reopen namespace window; the
	// private 0700 parent and the post-child strong-generation gate remain the
	// mutation authority.
	return writer, current, nil
}

func prepareExternalToolInputSnapshotForSealedTransfer(file *os.File, _ string) (*os.File, error) {
	if file == nil {
		return nil, fmt.Errorf("POSIX external tool snapshot transfer handle is missing")
	}
	return file, nil
}
