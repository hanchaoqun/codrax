//go:build windows

package memlimit

import (
	"syscall"
	"unsafe"
)

// memoryStatusEx mirrors the Win32 MEMORYSTATUSEX struct passed to
// GlobalMemoryStatusEx.
type memoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

// systemTotalMemory reads total host RAM via the kernel32
// GlobalMemoryStatusEx API.
func systemTotalMemory() (uint64, bool) {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	if err := proc.Find(); err != nil {
		return 0, false
	}
	var status memoryStatusEx
	status.length = uint32(unsafe.Sizeof(status))
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&status)))
	if r == 0 || status.totalPhys == 0 {
		return 0, false
	}
	return status.totalPhys, true
}
