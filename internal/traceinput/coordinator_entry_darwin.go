//go:build darwin

package traceinput

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// getattrlist(2) ATTR_CMN_NAME reports the native spelling without reading the
// containing directory. Darwin may choose a vnode's other hardlink name, so
// only directories (which cannot be hardlinked by callers) and single-link
// regular files qualify. Listable hardlinks continue through the exact-entry
// enumeration path; unknown/multiple-link search-only inputs fail closed.
func preparationNativeEntryName(path string, info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (!info.IsDir() && (!info.Mode().IsRegular() || stat.Nlink != 1)) {
		return "", fmt.Errorf("native entry identity requires a directory or a single-link regular file")
	}
	name, err := syscall.BytePtrFromString(path)
	if err != nil {
		return "", err
	}
	attrs := struct {
		BitmapCount uint16
		Reserved    uint16
		Common      uint32
		Volume      uint32
		Directory   uint32
		File        uint32
		Fork        uint32
	}{BitmapCount: 5, Common: 1} // ATTR_CMN_NAME, sys/attr.h
	var buf [4096]byte
	_, _, errno := syscall.Syscall6(syscall.SYS_GETATTRLIST, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 1, 0) // FSOPT_NOFOLLOW
	if errno != 0 {
		return "", errno
	}
	length := int(binary.NativeEndian.Uint32(buf[:4]))
	offset := int(int32(binary.NativeEndian.Uint32(buf[4:8]))) + 4
	size := int(binary.NativeEndian.Uint32(buf[8:12]))
	if length < 12 || length > len(buf) || offset < 12 || size < 2 || size > length-offset || buf[offset+size-1] != 0 {
		return "", fmt.Errorf("native entry-name response is incomplete")
	}
	return string(buf[offset : offset+size-1]), nil
}
