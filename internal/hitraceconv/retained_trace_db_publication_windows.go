//go:build windows

package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

type publishedConversionFilePlatformState struct {
	parent windows.Handle
}

type retainedTraceDBFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func duplicatePublishedConversionParentPlatform(dir *privateConversionDir) (publishedConversionFilePlatformState, error) {
	if dir == nil {
		return publishedConversionFilePlatformState{}, fmt.Errorf("Windows retained trace DB parent authority is missing")
	}
	dir.mu.Lock()
	defer dir.mu.Unlock()
	if dir.terminal || dir.platform.parent == 0 || dir.platform.parent == windows.InvalidHandle {
		return publishedConversionFilePlatformState{}, fmt.Errorf("Windows retained trace DB parent authority is closed")
	}
	if err := dir.validateIdentityLocked(true); err != nil {
		return publishedConversionFilePlatformState{}, err
	}
	if err := validateRetainedTraceDBWindowsFileSystem(dir.platform.parent); err != nil {
		return publishedConversionFilePlatformState{}, err
	}
	process := windows.CurrentProcess()
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(process, dir.platform.parent, process, &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return publishedConversionFilePlatformState{}, fmt.Errorf("duplicate Windows retained trace DB parent authority: %w", err)
	}
	return publishedConversionFilePlatformState{parent: duplicate}, nil
}

func validateRetainedTraceDBWindowsFileSystem(parent windows.Handle) error {
	return validateWindowsExactGenerationFileSystem(parent, "retained trace DB destination")
}

func validatePublishedConversionFilePlatform(
	state *publishedConversionFilePlatformState,
	leaf string,
	file *os.File,
	held os.FileInfo,
) error {
	if state == nil || state.parent == 0 || state.parent == windows.InvalidHandle || file == nil || held == nil {
		return fmt.Errorf("Windows retained trace DB publication authority is incomplete")
	}
	reopened, info, err := openPublishedConversionRegularChildWindows(state.parent, leaf)
	if err != nil {
		return err
	}
	same := os.SameFile(held, info)
	closeErr := reopened.Close()
	if !same {
		return traceDBJoinPreservingSingle(fmt.Errorf("parent-relative retained trace DB identity mismatch"), closeErr)
	}
	return closeErr
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
	return markPrivateConversionDirWindowsHandleForDeletion(windows.Handle(file.Fd()))
}

func closePublishedConversionFilePlatform(state *publishedConversionFilePlatformState) error {
	if state == nil || state.parent == 0 || state.parent == windows.InvalidHandle {
		return nil
	}
	parent := state.parent
	state.parent = 0
	return windows.CloseHandle(parent)
}

func publishSealedConversionFilePlatform(
	ctx context.Context,
	source *sealedConversionFile,
	dir *privateConversionDir,
	leaf, bindingPath, authorityPath string,
) (*retainedTraceDBPublication, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if source == nil || dir == nil {
		return nil, fmt.Errorf("Windows retained trace DB source authority is incomplete")
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("validate Windows retained trace DB source before publication: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	platform, err := duplicatePublishedConversionParentPlatform(dir)
	if err != nil {
		return nil, err
	}
	file, renameErr := source.publishAndDetachOpenFile(func(file *os.File) error {
		return renameRetainedTraceDBWindows(windows.Handle(file.Fd()), platform.parent, leaf)
	})
	if renameErr != nil {
		return nil, traceDBJoinPreservingSingle(
			fmt.Errorf("atomically publish retained trace DB generation: %w", renameErr),
			closePublishedConversionFilePlatform(&platform),
		)
	}
	publication, err := newRetainedTraceDBPublication(file, platform, leaf, bindingPath, authorityPath, source.Size())
	if err != nil {
		return nil, abortRetainedTraceDBPublication(file, &platform, leaf, err)
	}
	return publication, nil
}

func renameRetainedTraceDBWindows(file, parent windows.Handle, leaf string) error {
	name, err := windows.UTF16FromString(leaf)
	if err != nil {
		return err
	}
	name = name[:len(name)-1]
	nameBytes := len(name) * 2
	var layout retainedTraceDBFileRenameInformation
	bufferSize := int(unsafe.Sizeof(layout)) + nameBytes
	buffer := make([]byte, bufferSize)
	info := (*retainedTraceDBFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = 0
	info.RootDirectory = parent
	info.FileNameLength = uint32(nameBytes)
	nameStart := unsafe.Pointer(uintptr(unsafe.Pointer(&buffer[0])) + unsafe.Offsetof(layout.FileName))
	copy(unsafe.Slice((*uint16)(nameStart), len(name)), name)
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtSetInformationFile(file, &iosb, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
	runtime.KeepAlive(buffer)
	return err
}

func openPublishedConversionRegularChildWindows(parent windows.Handle, name string) (*os.File, os.FileInfo, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, nil, err
	}
	oa := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    privateConversionDirWindowsObjectAttrs,
	}
	var handle windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	allocationSize := int64(0)
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		oa,
		&iosb,
		&allocationSize,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	runtime.KeepAlive(objectName)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		return nil, nil, traceDBJoinPreservingSingle(fmt.Errorf("wrap Windows retained trace DB final handle"), windows.CloseHandle(handle))
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, file.Close())
	}
	var handleInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &handleInfo); err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, file.Close())
	}
	if !info.Mode().IsRegular() || handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 ||
		handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, nil, traceDBJoinPreservingSingle(fmt.Errorf("Windows retained trace DB final is not a plain regular file"), file.Close())
	}
	return file, info, nil
}
