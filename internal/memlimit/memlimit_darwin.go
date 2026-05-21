//go:build darwin

package memlimit

import (
	"encoding/binary"
	"syscall"
)

// systemTotalMemory reads total host RAM from the hw.memsize sysctl.
//
// syscall.Sysctl returns the value as raw bytes and strips trailing NUL
// bytes — which, for a little-endian 64-bit byte count, are exactly the
// high-order zeros of any realistic RAM size. Padding the bytes back to
// a full 8-byte word before decoding is therefore correct regardless of
// how many trailing zeros were stripped. Both darwin architectures
// (amd64, arm64) are little-endian.
func systemTotalMemory() (uint64, bool) {
	raw, err := syscall.Sysctl("hw.memsize")
	if err != nil || raw == "" {
		return 0, false
	}
	b := []byte(raw)
	if len(b) > 8 {
		b = b[:8]
	}
	var word [8]byte
	copy(word[:], b)
	total := binary.LittleEndian.Uint64(word[:])
	if total == 0 {
		return 0, false
	}
	return total, true
}
