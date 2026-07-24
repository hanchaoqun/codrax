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

func openPublishedConversionParentPlatform(path string, kind sealedConversionPublicationKind) (*publishedConversionFilePlatformState, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s output parent authority: %w", kind.diagnosticName(), err)
	}
	return &publishedConversionFilePlatformState{parentFD: fd}, nil
}

func duplicatePublishedConversionParentPlatform(parent *publishedConversionFilePlatformState, kind sealedConversionPublicationKind) (publishedConversionFilePlatformState, error) {
	if parent == nil {
		return publishedConversionFilePlatformState{parentFD: -1}, fmt.Errorf("%s parent authority is missing", kind.diagnosticName())
	}
	if parent.parentFD < 0 {
		return publishedConversionFilePlatformState{parentFD: -1}, fmt.Errorf("%s parent authority is closed", kind.diagnosticName())
	}
	fd, err := unix.Dup(parent.parentFD)
	if err != nil {
		return publishedConversionFilePlatformState{parentFD: -1}, fmt.Errorf("duplicate %s parent authority: %w", kind.diagnosticName(), err)
	}
	unix.CloseOnExec(fd)
	return publishedConversionFilePlatformState{parentFD: fd}, nil
}

func validatePublishedConversionFilePlatform(
	state *publishedConversionFilePlatformState,
	leaf string,
	file *os.File,
	held os.FileInfo,
	kind sealedConversionPublicationKind,
) error {
	if state == nil || state.parentFD < 0 || file == nil || held == nil {
		return fmt.Errorf("POSIX %s publication authority is incomplete", kind.diagnosticName())
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
		return fmt.Errorf("parent-relative %s identity mismatch", kind.diagnosticName())
	}
	return nil
}

func removePublishedConversionFilePlatform(
	state *publishedConversionFilePlatformState,
	leaf string,
	file *os.File,
	kind sealedConversionPublicationKind,
) error {
	if file == nil {
		return nil
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if err := validatePublishedConversionFilePlatform(state, leaf, file, info, kind); err != nil {
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
