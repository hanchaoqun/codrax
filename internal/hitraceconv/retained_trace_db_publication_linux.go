//go:build linux

package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"
)

func publishSealedConversionFilePlatform(
	ctx context.Context,
	source *sealedConversionFile,
	dir *privateConversionDir,
	leaf, bindingPath, authorityPath string,
	kind sealedConversionPublicationKind,
) (publication *retainedTraceDBPublication, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || dir == nil {
		return nil, fmt.Errorf("Linux %s source authority is incomplete", kind.diagnosticName())
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("validate Linux %s source before snapshot: %w", kind.diagnosticName(), err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dir.mu.Lock()
	if dir.terminal || dir.platform.parentFD < 0 {
		dir.mu.Unlock()
		return nil, fmt.Errorf("Linux %s parent authority is closed", kind.diagnosticName())
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		dir.mu.Unlock()
		return nil, err
	}
	tempFD, err := unix.Openat(dir.platform.parentFD, ".", unix.O_TMPFILE|unix.O_RDWR|unix.O_CLOEXEC, 0o600)
	dir.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("create unnamed %s publication inode: %w", kind.diagnosticName(), err)
	}
	temp := os.NewFile(uintptr(tempFD), kind.privateHandleName())
	if temp == nil {
		return nil, traceDBJoinPreservingSingle(fmt.Errorf("wrap unnamed %s publication inode", kind.diagnosticName()), unix.Close(tempFD))
	}
	closeTemp := true
	defer func() {
		if closeTemp {
			resultErr = traceDBJoinPreservingSingle(resultErr, temp.Close())
		}
	}()

	copyErr := source.withOpenFile(func(sourceFile *os.File) error {
		cloneErr := unix.IoctlFileClone(int(temp.Fd()), int(sourceFile.Fd()))
		runtime.KeepAlive(sourceFile)
		if cloneErr == nil {
			return nil
		}
		if !linuxRetainedTraceDBCloneFallbackAllowed(cloneErr) {
			return fmt.Errorf("clone %s into publication inode: %w", kind.sealedSourceName(), cloneErr)
		}
		if err := unix.Ftruncate(int(temp.Fd()), 0); err != nil {
			return traceDBJoinPreservingSingle(fmt.Errorf("reset %s publication inode after clone fallback: %w", kind.diagnosticName(), err), cloneErr)
		}
		if _, err := temp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		written, err := copyStandaloneRange(ctx, temp, io.NewSectionReader(sourceFile, 0, source.Size()))
		if err != nil {
			return err
		}
		if written != source.Size() {
			return fmt.Errorf("%s snapshot size mismatch: wrote=%d want=%d", kind.diagnosticName(), written, source.Size())
		}
		return nil
	})
	if copyErr != nil {
		return nil, copyErr
	}
	if err := temp.Sync(); err != nil {
		return nil, fmt.Errorf("sync %s publication inode: %w", kind.diagnosticName(), err)
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("validate Linux %s source after snapshot: %w", kind.diagnosticName(), err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := temp.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != source.Size() {
		if err == nil {
			err = fmt.Errorf("mode=%s size=%d want=%d", info.Mode(), info.Size(), source.Size())
		}
		return nil, fmt.Errorf("validate unnamed %s publication inode: %w", kind.diagnosticName(), err)
	}

	dir.mu.Lock()
	if dir.terminal || dir.platform.parentFD < 0 {
		dir.mu.Unlock()
		return nil, fmt.Errorf("Linux %s parent authority closed before publication", kind.diagnosticName())
	}
	linked := false
	linkErr := unix.Linkat(int(temp.Fd()), "", dir.platform.parentFD, leaf, unix.AT_EMPTY_PATH)
	if linkErr == nil {
		linked = true
	}
	if linkErr != nil && linuxRetainedTraceDBProcLinkFallbackAllowed(linkErr) {
		linked, linkErr = linkLinuxRetainedTraceDBThroughHeldProcFD(int(temp.Fd()), dir.platform.parentFD, leaf, kind)
	}
	if linkErr != nil && linked {
		borrowed := publishedConversionFilePlatformState{parentFD: dir.platform.parentFD}
		linkErr = traceDBJoinPreservingSingle(linkErr, removePublishedConversionFilePlatform(&borrowed, leaf, temp, kind))
	}
	dir.mu.Unlock()
	if linkErr != nil {
		return nil, fmt.Errorf("atomically publish %s generation: %w", kind.diagnosticName(), linkErr)
	}
	platform, err := duplicatePublishedConversionParentPlatform(dir, kind)
	if err != nil {
		dir.mu.Lock()
		borrowed := publishedConversionFilePlatformState{parentFD: dir.platform.parentFD}
		removeErr := removePublishedConversionFilePlatform(&borrowed, leaf, temp, kind)
		dir.mu.Unlock()
		return nil, traceDBJoinPreservingSingle(err, removeErr)
	}
	publication, err = newRetainedTraceDBPublication(temp, platform, kind, leaf, bindingPath, authorityPath, source.Size())
	if err != nil {
		return nil, abortRetainedTraceDBPublication(temp, &platform, leaf, kind, err)
	}
	closeTemp = false
	return publication, nil
}

func linuxRetainedTraceDBCloneFallbackAllowed(err error) bool {
	return errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOTTY) ||
		errors.Is(err, unix.EXDEV) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS)
}

func linuxRetainedTraceDBProcLinkFallbackAllowed(err error) bool {
	return errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) ||
		errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOENT)
}

func linkLinuxRetainedTraceDBThroughHeldProcFD(tempFD, parentFD int, finalLeaf string, kind sealedConversionPublicationKind) (linked bool, resultErr error) {
	procFD, err := unix.Open("/proc/self/fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, fmt.Errorf("open held proc fd directory for %s publication: %w", kind.diagnosticName(), err)
	}
	defer func() {
		resultErr = traceDBJoinPreservingSingle(resultErr, unix.Close(procFD))
	}()
	var fs unix.Statfs_t
	if err := unix.Fstatfs(procFD, &fs); err != nil {
		return false, fmt.Errorf("identify held proc fd directory: %w", err)
	}
	if uint64(fs.Type) != uint64(unix.PROC_SUPER_MAGIC) {
		return false, fmt.Errorf("held /proc/self/fd is not procfs: type=%#x", fs.Type)
	}
	fdLeaf := strconv.Itoa(tempFD)
	var procEntry unix.Stat_t
	if err := unix.Fstatat(procFD, fdLeaf, &procEntry, 0); err != nil {
		return false, fmt.Errorf("bind proc fd entry for %s publication: %w", kind.diagnosticName(), err)
	}
	var temp unix.Stat_t
	if err := unix.Fstat(tempFD, &temp); err != nil {
		return false, fmt.Errorf("stat unnamed %s publication inode: %w", kind.diagnosticName(), err)
	}
	if procEntry.Mode&unix.S_IFMT != unix.S_IFREG || procEntry.Dev != temp.Dev || procEntry.Ino != temp.Ino {
		return false, fmt.Errorf("proc fd entry does not name the held %s publication inode", kind.diagnosticName())
	}
	if err := unix.Linkat(procFD, fdLeaf, parentFD, finalLeaf, unix.AT_SYMLINK_FOLLOW); err != nil {
		return false, err
	}
	return true, nil
}
