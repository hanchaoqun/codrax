//go:build windows

package cpulimit

import "syscall"

// Win32 process priority classes. Windows has no fine-grained nice
// scale, so the [0, 19] nice value is bucketed: 0 maps to NORMAL (no
// politeness) and any positive value to BELOW_NORMAL — the standard
// "be polite to interactive work" class.
const (
	normalPriorityClass      = 0x00000020
	belowNormalPriorityClass = 0x00004000
)

// setThreadNice lowers the calling process's scheduling priority class.
//
// Unlike Unix niceness this is process-wide (SetPriorityClass), so
// every thread — including ones the Go runtime created before Apply
// ran — is covered; the per-worker NiceCurrentThread calls then become
// idempotent. Lowering the priority class never requires privilege.
func setThreadNice(nice int) error {
	class := uintptr(belowNormalPriorityClass)
	if nice <= 0 {
		class = normalPriorityClass
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getCurrentProcess := kernel32.NewProc("GetCurrentProcess")
	setPriorityClass := kernel32.NewProc("SetPriorityClass")
	if err := setPriorityClass.Find(); err != nil {
		return err
	}
	handle, _, _ := getCurrentProcess.Call()
	if r, _, err := setPriorityClass.Call(handle, class); r == 0 {
		return err
	}
	return nil
}
