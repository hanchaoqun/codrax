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
	outputParent *publishedConversionFilePlatformState,
	leaf, bindingPath, authorityPath string,
	kind sealedConversionPublicationKind,
) (publication *retainedTraceDBPublication, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || dir == nil || outputParent == nil || outputParent.parentFD < 0 {
		return nil, fmt.Errorf("Linux %s source authority is incomplete", kind.diagnosticName())
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("validate Linux %s source before snapshot: %w", kind.diagnosticName(), err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dir.mu.Lock()
	if dir.terminal || dir.platform.guardFD < 0 {
		dir.mu.Unlock()
		return nil, fmt.Errorf("Linux %s staging authority is closed", kind.diagnosticName())
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		dir.mu.Unlock()
		return nil, err
	}
	dir.mu.Unlock()
	// O_TMPFILE remains the preferred zero-visibility publication path.
	// Filesystems presented through WSL DrvFS can reject it even though they
	// support ordinary exclusive creation and atomic no-replace publication.
	// Only that precise capability failure activates one randomized, held,
	// parent-relative compatibility generation instead.
	tempLeaf := ""
	tempFD, err := unix.Openat(outputParent.parentFD, ".", unix.O_TMPFILE|unix.O_RDWR|unix.O_CLOEXEC, 0o600)
	if err != nil && linuxUnnamedPublicationFallbackAllowed(err) {
		tempFD, tempLeaf, err = openLinuxNamedPublicationTemp(outputParent.parentFD, kind)
	}
	if err != nil {
		return nil, fmt.Errorf("create %s publication inode: %w", kind.diagnosticName(), err)
	}
	handleName := kind.privateHandleName()
	if tempLeaf != "" {
		handleName = tempLeaf
	}
	temp := os.NewFile(uintptr(tempFD), handleName)
	if temp == nil {
		closeErr := unix.Close(tempFD)
		if tempLeaf != "" {
			closeErr = traceDBJoinPreservingSingle(closeErr, unix.Unlinkat(outputParent.parentFD, tempLeaf, 0))
		}
		return nil, traceDBJoinPreservingSingle(fmt.Errorf("wrap %s publication inode", kind.diagnosticName()), closeErr)
	}
	closeTemp := true
	namedTempPresent := tempLeaf != ""
	defer func() {
		if namedTempPresent {
			resultErr = traceDBJoinPreservingSingle(resultErr, removeLinuxNamedPublicationTemp(outputParent.parentFD, tempLeaf, temp, kind))
		}
		if closeTemp {
			resultErr = traceDBJoinPreservingSingle(resultErr, temp.Close())
		}
	}()

	var sourcePerm os.FileMode
	copyErr := source.withOpenFile(func(sourceFile *os.File) error {
		sourceInfo, err := sourceFile.Stat()
		if err != nil {
			return fmt.Errorf("stat %s before publication snapshot: %w", kind.sealedSourceName(), err)
		}
		if !sourceInfo.Mode().IsRegular() {
			return fmt.Errorf("%s is not regular before publication snapshot", kind.sealedSourceName())
		}
		sourcePerm = sourceInfo.Mode().Perm()
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
	// Both O_TMPFILE and the named compatibility generation start at 0600,
	// while callers deliberately choose the staging file's customer-visible
	// permissions. Preserve that sealed source mode instead of silently
	// narrowing every generic publication on Linux.
	if err := temp.Chmod(sourcePerm); err != nil {
		return nil, fmt.Errorf("preserve %s publication permissions: %w", kind.diagnosticName(), err)
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
	if err != nil || !info.Mode().IsRegular() || info.Size() != source.Size() || info.Mode().Perm() != sourcePerm {
		if err == nil {
			err = fmt.Errorf("mode=%s perm=%#o want_perm=%#o size=%d want=%d", info.Mode(), info.Mode().Perm(), sourcePerm, info.Size(), source.Size())
		}
		return nil, fmt.Errorf("validate %s publication inode: %w", kind.diagnosticName(), err)
	}

	if outputParent.parentFD < 0 {
		return nil, fmt.Errorf("Linux %s parent authority closed before publication", kind.diagnosticName())
	}
	linked := false
	var linkErr error
	if tempLeaf == "" {
		linkErr = unix.Linkat(int(temp.Fd()), "", outputParent.parentFD, leaf, unix.AT_EMPTY_PATH)
		if linkErr == nil {
			linked = true
		}
		if linkErr != nil && linuxRetainedTraceDBProcLinkFallbackAllowed(linkErr) {
			linked, linkErr = linkLinuxRetainedTraceDBThroughHeldProcFD(int(temp.Fd()), outputParent.parentFD, leaf, kind)
		}
	} else {
		linkErr = publishLinuxNamedPublicationTemp(outputParent.parentFD, tempLeaf, leaf, temp, kind)
		if linkErr == nil {
			linked = true
			namedTempPresent = false
		}
	}
	if linkErr != nil && linked {
		borrowed := publishedConversionFilePlatformState{parentFD: outputParent.parentFD}
		linkErr = traceDBJoinPreservingSingle(linkErr, removePublishedConversionFilePlatform(&borrowed, leaf, temp, kind))
	}
	if linkErr != nil {
		return nil, fmt.Errorf("atomically publish %s generation: %w", kind.diagnosticName(), linkErr)
	}
	platform, err := duplicatePublishedConversionParentPlatform(outputParent, kind)
	if err != nil {
		borrowed := publishedConversionFilePlatformState{parentFD: outputParent.parentFD}
		removeErr := removePublishedConversionFilePlatform(&borrowed, leaf, temp, kind)
		return nil, traceDBJoinPreservingSingle(err, removeErr)
	}
	publication, err = newRetainedTraceDBPublication(temp, platform, kind, leaf, bindingPath, authorityPath, source.Size())
	if err != nil {
		return nil, abortRetainedTraceDBPublication(temp, &platform, leaf, kind, err)
	}
	closeTemp = false
	return publication, nil
}

func openLinuxNamedPublicationTemp(parentFD int, kind sealedConversionPublicationKind) (fd int, leaf string, resultErr error) {
	for attempt := 0; attempt < privateConversionDirCreateAttempts; attempt++ {
		leaf, resultErr = nextPrivateConversionDirLeaf(".codrax-publish-*.tmp")
		if resultErr != nil {
			return -1, "", resultErr
		}
		fd, resultErr = unix.Openat(parentFD, leaf, unix.O_CREAT|unix.O_EXCL|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
		if resultErr == nil {
			return fd, leaf, nil
		}
		if !errors.Is(resultErr, unix.EEXIST) {
			return -1, "", fmt.Errorf("create named %s publication compatibility inode: %w", kind.diagnosticName(), resultErr)
		}
	}
	return -1, "", fmt.Errorf("create named %s publication compatibility inode: exhausted collision retries", kind.diagnosticName())
}

func linuxUnnamedPublicationFallbackAllowed(err error) bool {
	return errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOTSUP) ||
		errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EISDIR)
}

func validateLinuxNamedPublicationTemp(parentFD int, leaf string, file *os.File, kind sealedConversionPublicationKind) error {
	if parentFD < 0 || leaf == "" || file == nil {
		return fmt.Errorf("named %s publication authority is incomplete", kind.diagnosticName())
	}
	var relative unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &relative, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	var descriptor unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &descriptor); err != nil {
		return err
	}
	runtime.KeepAlive(file)
	if relative.Mode&unix.S_IFMT != unix.S_IFREG || relative.Dev != descriptor.Dev || relative.Ino != descriptor.Ino {
		return fmt.Errorf("parent-relative named %s publication identity mismatch", kind.diagnosticName())
	}
	return nil
}

func removeLinuxNamedPublicationTemp(parentFD int, leaf string, file *os.File, kind sealedConversionPublicationKind) error {
	if err := validateLinuxNamedPublicationTemp(parentFD, leaf, file, kind); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	if err := unix.Unlinkat(parentFD, leaf, 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

func publishLinuxNamedPublicationTemp(parentFD int, tempLeaf, finalLeaf string, file *os.File, kind sealedConversionPublicationKind) error {
	if err := validateLinuxNamedPublicationTemp(parentFD, tempLeaf, file, kind); err != nil {
		return fmt.Errorf("validate named %s publication generation: %w", kind.diagnosticName(), err)
	}
	renameErr := unix.Renameat2(parentFD, tempLeaf, parentFD, finalLeaf, unix.RENAME_NOREPLACE)
	if renameErr == nil {
		return nil
	}
	if !linuxNamedPublicationLinkFallbackAllowed(renameErr) {
		return fmt.Errorf("atomically rename named %s publication generation: %w", kind.diagnosticName(), renameErr)
	}
	if err := unix.Linkat(parentFD, tempLeaf, parentFD, finalLeaf, 0); err != nil {
		return traceDBJoinPreservingSingle(
			fmt.Errorf("atomically link named %s publication generation: %w", kind.diagnosticName(), err),
			renameErr,
		)
	}
	borrowed := publishedConversionFilePlatformState{parentFD: parentFD}
	if err := unix.Unlinkat(parentFD, tempLeaf, 0); err != nil {
		return traceDBJoinPreservingSingle(
			fmt.Errorf("detach named %s publication compatibility binding: %w", kind.diagnosticName(), err),
			removePublishedConversionFilePlatform(&borrowed, finalLeaf, file, kind),
		)
	}
	return nil
}

func linuxNamedPublicationLinkFallbackAllowed(err error) bool {
	return errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP)
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
