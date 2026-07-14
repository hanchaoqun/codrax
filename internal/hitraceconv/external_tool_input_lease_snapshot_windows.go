//go:build windows

package hitraceconv

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var externalToolReOpenFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

func validateExternalToolInputSourcePlatform(source conversionInputView) error {
	fileSource, ok := source.(externalToolInputFileSource)
	if !ok {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			source.DisplayPath(),
			fmt.Errorf("Windows external tool snapshot requires a held file source"),
		)
	}
	if err := fileSource.withOpenFile(func(file *os.File) error {
		if file == nil {
			return fmt.Errorf("Windows source handle is missing")
		}
		return validateWindowsExactGenerationFileSystem(windows.Handle(file.Fd()), "external tool source")
	}); err != nil {
		return err
	}
	return nil
}

// The caller holds privateConversionDir.mu while lending this platform state.
func validateExternalToolInputSnapshotDirPlatform(state *privateConversionDirPlatformState) error {
	if state == nil || state.guard == nil {
		return fmt.Errorf("Windows private snapshot directory authority is incomplete")
	}
	return validateWindowsExactGenerationFileSystem(windows.Handle(state.guard.Fd()), "external tool snapshot")
}

func createExternalToolInputSnapshotFilePlatform(state *privateConversionDirPlatformState, name string) (*os.File, error) {
	if state == nil || state.guard == nil {
		return nil, fmt.Errorf("Windows private snapshot directory authority is incomplete")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: windows.Handle(state.guard.Fd()),
		ObjectName:    objectName,
		Attributes:    privateConversionDirWindowsObjectAttrs,
	}
	var handle windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	allocationSize := int64(0)
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_READ_DATA|windows.FILE_WRITE_DATA|windows.FILE_READ_ATTRIBUTES|windows.FILE_WRITE_ATTRIBUTES|windows.SYNCHRONIZE,
		oa,
		&iosb,
		&allocationSize,
		windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_CREATE,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	runtime.KeepAlive(objectName)
	runtime.KeepAlive(state.guard)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		return nil, traceDBJoinPreservingSingle(
			fmt.Errorf("wrap Windows external tool snapshot creator handle"),
			windows.CloseHandle(handle),
		)
	}
	var handleInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &handleInfo); err != nil ||
		handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 ||
		handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		if err == nil {
			err = fmt.Errorf("created Windows external tool snapshot is not a plain file: attributes=0x%x", handleInfo.FileAttributes)
		}
		return nil, traceDBJoinPreservingSingle(err, file.Close())
	}
	return file, nil
}

func freezeExternalToolInputSnapshotFilePlatform(
	state *privateConversionDirPlatformState,
	name string,
	writer *os.File,
	created os.FileInfo,
) (*os.File, os.FileInfo, error) {
	if state == nil || state.guard == nil || writer == nil || created == nil {
		if writer != nil {
			_ = writer.Close()
		}
		return nil, nil, fmt.Errorf("Windows external tool snapshot freeze authority is incomplete")
	}
	current, err := writer.Stat()
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(created, current) {
		if err == nil {
			err = fmt.Errorf("Windows external tool snapshot creator identity mismatch")
		}
		return nil, nil, traceDBJoinPreservingSingle(err, writer.Close())
	}
	if err := validatePrivateConversionRegularChildPlatform(state, name, writer, current); err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, writer.Close())
	}

	// The creator must share WRITE while it still owns write access. First make
	// a read-only bridge on the same kernel file object, then close the creator
	// and reopen once more with FILE_SHARE_READ only. This leaves the child a
	// conventional read-only input handle while blocking rename, delete and
	// mutation for the entire command lifetime, without a pathname reopen gap.
	bridgeHandle, err := reOpenExternalToolSnapshotWindows(
		windows.Handle(writer.Fd()),
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
	)
	if err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, writer.Close())
	}
	bridge := os.NewFile(uintptr(bridgeHandle), name+"-bridge")
	if bridge == nil {
		return nil, nil, traceDBJoinPreservingSingle(
			fmt.Errorf("wrap Windows external tool snapshot bridge handle"),
			windows.CloseHandle(bridgeHandle),
			writer.Close(),
		)
	}
	if err := writer.Close(); err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, bridge.Close())
	}

	finalHandle, err := reOpenExternalToolSnapshotWindows(
		windows.Handle(bridge.Fd()),
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
	)
	if err != nil {
		return nil, nil, traceDBJoinPreservingSingle(err, bridge.Close())
	}
	final := os.NewFile(uintptr(finalHandle), name)
	if final == nil {
		return nil, nil, traceDBJoinPreservingSingle(
			fmt.Errorf("wrap Windows external tool snapshot final handle"),
			windows.CloseHandle(finalHandle),
			bridge.Close(),
		)
	}
	bridgeInfo, bridgeStatErr := bridge.Stat()
	finalInfo, finalStatErr := final.Stat()
	bridgeCloseErr := bridge.Close()
	if bridgeStatErr != nil || finalStatErr != nil || bridgeInfo == nil || finalInfo == nil ||
		!finalInfo.Mode().IsRegular() || !os.SameFile(created, bridgeInfo) || !os.SameFile(created, finalInfo) {
		if bridgeStatErr == nil && finalStatErr == nil {
			finalStatErr = fmt.Errorf("Windows external tool snapshot changed during access downgrade")
		}
		return nil, nil, traceDBJoinPreservingSingle(
			bridgeStatErr,
			finalStatErr,
			bridgeCloseErr,
			final.Close(),
		)
	}
	if bridgeCloseErr != nil {
		return nil, nil, traceDBJoinPreservingSingle(bridgeCloseErr, final.Close())
	}
	return final, finalInfo, nil
}

func reOpenExternalToolSnapshotWindows(
	handle windows.Handle,
	desiredAccess uint32,
	shareMode uint32,
) (windows.Handle, error) {
	reopened, _, callErr := externalToolReOpenFile.Call(
		uintptr(handle),
		uintptr(desiredAccess),
		uintptr(shareMode),
		uintptr(windows.FILE_FLAG_OPEN_REPARSE_POINT),
	)
	runtime.KeepAlive(handle)
	if windows.Handle(reopened) == windows.InvalidHandle {
		return windows.InvalidHandle, fmt.Errorf("reopen Windows external tool snapshot handle: %w", callErr)
	}
	return windows.Handle(reopened), nil
}
