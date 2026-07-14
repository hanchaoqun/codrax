//go:build windows

package filegeneration

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const fileBasicInfoClass = 0

type windowsFileBasicInfo struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangeTime     int64
	FileAttributes uint32
	_              uint32
}

var getFileInformationByHandleEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFileInformationByHandleEx")

func enhanceIdentityFromFile(file *os.File, _ os.FileInfo, id Identity) Identity {
	if file == nil {
		return id
	}
	defer runtime.KeepAlive(file)
	handle := syscall.Handle(file.Fd())
	var fileInfo syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &fileInfo); err != nil {
		return id
	}
	var basic windowsFileBasicInfo
	ok, _, _ := getFileInformationByHandleEx.Call(
		uintptr(handle),
		uintptr(fileBasicInfoClass),
		uintptr(unsafe.Pointer(&basic)),
		unsafe.Sizeof(basic),
	)
	if ok == 0 {
		return id
	}
	volume := uint64(fileInfo.VolumeSerialNumber)
	fileIndex := uint64(fileInfo.FileIndexHigh)<<32 | uint64(fileInfo.FileIndexLow)
	if !validWindowsStrongIdentity(volume, fileIndex, basic.ChangeTime) {
		return id
	}
	id.device = volume
	id.inode = fileIndex
	id.changePrimary = basic.ChangeTime
	id.changeSecondary = basic.CreationTime
	id.strong = true
	return id
}
