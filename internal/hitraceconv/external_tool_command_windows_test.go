//go:build windows

package hitraceconv

import (
	"testing"
	"unsafe"
)

func TestExternalToolWindowsJobAccountingLayoutPinned(t *testing.T) {
	if size := unsafe.Sizeof(externalToolWindowsJobAccounting{}); size != 48 {
		t.Fatalf("Windows Job accounting ABI size=%d, want 48", size)
	}
}
