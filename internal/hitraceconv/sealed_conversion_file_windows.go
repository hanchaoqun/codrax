//go:build windows

package hitraceconv

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func adoptPrivateConversionRegularChildPlatform(state *privateConversionDirPlatformState, name string) (*os.File, os.FileInfo, error) {
	if state == nil || state.guard == nil {
		return nil, nil, fmt.Errorf("Windows private directory authority is incomplete")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, nil, err
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
		windows.FILE_READ_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		oa,
		&iosb,
		&allocationSize,
		0,
		windows.FILE_SHARE_READ,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	runtime.KeepAlive(objectName)
	runtime.KeepAlive(state.guard)
	if err != nil {
		if status, ok := err.(windows.NTStatus); ok &&
			(status == windows.STATUS_NO_SUCH_FILE || status == windows.STATUS_OBJECT_NAME_NOT_FOUND) {
			return nil, nil, fmt.Errorf("%w: %v", os.ErrNotExist, err)
		}
		return nil, nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		return nil, nil, traceDBJoinPreservingSingle(
			fmt.Errorf("wrap private conversion child handle"), windows.CloseHandle(handle),
		)
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
		return nil, nil, traceDBJoinPreservingSingle(
			fmt.Errorf("%w: %s", errSealedConversionFileNotRegular, name), file.Close(),
		)
	}
	return file, info, nil
}

func validatePrivateConversionRegularChildPlatform(state *privateConversionDirPlatformState, name string, file *os.File, held os.FileInfo) error {
	if state == nil || state.guard == nil || file == nil || held == nil {
		return fmt.Errorf("Windows sealed child authority is incomplete")
	}
	current, err := file.Stat()
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(held, current) {
		return fmt.Errorf("held child identity mismatch")
	}
	reopened, reopenedInfo, err := adoptPrivateConversionRegularChildPlatform(state, name)
	if err != nil {
		return err
	}
	same := os.SameFile(current, reopenedInfo)
	closeErr := reopened.Close()
	if !same {
		return traceDBJoinPreservingSingle(fmt.Errorf("parent-relative child identity mismatch"), closeErr)
	}
	return closeErr
}
