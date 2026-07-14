//go:build unix

package hitraceconv

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func adoptPrivateConversionRegularChildPlatform(state *privateConversionDirPlatformState, name string) (*os.File, os.FileInfo, error) {
	if state == nil || state.guardFD < 0 {
		return nil, nil, fmt.Errorf("POSIX private directory authority is incomplete")
	}
	fd, err := unix.Openat(state.guardFD, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		return nil, nil, traceDBJoinPreservingSingle(
			fmt.Errorf("wrap private conversion child handle"), unix.Close(fd),
		)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, file.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, nil, traceDBJoinPreservingSingle(
			fmt.Errorf("%w: %s", errSealedConversionFileNotRegular, name), file.Close(),
		)
	}
	return file, info, nil
}

func validatePrivateConversionRegularChildPlatform(state *privateConversionDirPlatformState, name string, file *os.File, held os.FileInfo) error {
	if state == nil || state.guardFD < 0 || file == nil || held == nil {
		return fmt.Errorf("POSIX sealed child authority is incomplete")
	}
	current, err := file.Stat()
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(held, current) {
		return fmt.Errorf("held child identity mismatch")
	}
	var relative unix.Stat_t
	if err := unix.Fstatat(state.guardFD, name, &relative, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	var descriptor unix.Stat_t
	err = unix.Fstat(int(file.Fd()), &descriptor)
	runtime.KeepAlive(file)
	if err != nil {
		return err
	}
	if relative.Mode&unix.S_IFMT != unix.S_IFREG || relative.Dev != descriptor.Dev || relative.Ino != descriptor.Ino {
		return fmt.Errorf("parent-relative child identity mismatch")
	}
	return nil
}
