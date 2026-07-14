//go:build unix

package hitraceconv

import (
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

type publishedConversionFilePlatformState struct {
	parentFD int
}

func duplicatePublishedConversionParentPlatform(dir *privateConversionDir) (publishedConversionFilePlatformState, error) {
	if dir == nil {
		return publishedConversionFilePlatformState{parentFD: -1}, fmt.Errorf("retained trace DB parent authority is missing")
	}
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal || dir.platform.parentFD < 0 {
		return publishedConversionFilePlatformState{parentFD: -1}, fmt.Errorf("retained trace DB parent authority is closed")
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		return publishedConversionFilePlatformState{parentFD: -1}, err
	}
	fd, err := unix.Dup(dir.platform.parentFD)
	if err != nil {
		return publishedConversionFilePlatformState{parentFD: -1}, fmt.Errorf("duplicate retained trace DB parent authority: %w", err)
	}
	unix.CloseOnExec(fd)
	return publishedConversionFilePlatformState{parentFD: fd}, nil
}

func validatePublishedConversionFilePlatform(
	state *publishedConversionFilePlatformState,
	leaf string,
	file *os.File,
	held os.FileInfo,
) error {
	if state == nil || state.parentFD < 0 || file == nil || held == nil {
		return fmt.Errorf("POSIX retained trace DB publication authority is incomplete")
	}
	var relative unix.Stat_t
	if err := unix.Fstatat(state.parentFD, leaf, &relative, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	var descriptor unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &descriptor); err != nil {
		return err
	}
	runtime.KeepAlive(file)
	if relative.Mode&unix.S_IFMT != unix.S_IFREG || relative.Dev != descriptor.Dev || relative.Ino != descriptor.Ino {
		return fmt.Errorf("parent-relative retained trace DB identity mismatch")
	}
	return nil
}

func removePublishedConversionFilePlatform(
	state *publishedConversionFilePlatformState,
	leaf string,
	file *os.File,
) error {
	if file == nil {
		return nil
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if err := validatePublishedConversionFilePlatform(state, leaf, file, info); err != nil {
		return err
	}
	if err := unix.Unlinkat(state.parentFD, leaf, 0); err != nil && err != unix.ENOENT {
		return err
	}
	return nil
}

func closePublishedConversionFilePlatform(state *publishedConversionFilePlatformState) error {
	if state == nil || state.parentFD < 0 {
		return nil
	}
	fd := state.parentFD
	state.parentFD = -1
	return unix.Close(fd)
}
