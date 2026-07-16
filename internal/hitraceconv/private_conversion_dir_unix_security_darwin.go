//go:build darwin

package hitraceconv

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	privateConversionDirDarwinACLExtended  = 0x00000100
	privateConversionDirDarwinACLNoInherit = 1 << 17
	privateConversionDirDarwinFileSecMode  = 4
	privateConversionDirDarwinFileSecACL   = 5
)

var (
	privateConversionDirDarwinACLInitTrampolineAddr     uintptr
	privateConversionDirDarwinACLSetFDTrampolineAddr    uintptr
	privateConversionDirDarwinACLGetFDTrampolineAddr    uintptr
	privateConversionDirDarwinACLFreeTrampolineAddr     uintptr
	privateConversionDirDarwinACLGetFlagsTrampolineAddr uintptr
	privateConversionDirDarwinACLAddFlagTrampolineAddr  uintptr
	privateConversionDirDarwinFileSecInitTrampolineAddr uintptr
	privateConversionDirDarwinFileSecSetTrampolineAddr  uintptr
	privateConversionDirDarwinFileSecFreeTrampolineAddr uintptr
	privateConversionDirDarwinMkdirXTrampolineAddr      uintptr
)

// Darwin routes libc calls through syscall.syscall; exported syscall.Syscall
// is the kernel-trap ABI and must never receive a libSystem function pointer.
// This is the same bridge used by x/sys/unix's Darwin generated wrappers.
//
//go:linkname privateConversionDirDarwinLibcCall syscall.syscall
func privateConversionDirDarwinLibcCall(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, errno syscall.Errno)

//go:linkname privateConversionDirDarwinLibcCallPtr syscall.syscallPtr
func privateConversionDirDarwinLibcCallPtr(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, errno syscall.Errno)

// These imports use the same non-cgo libSystem trampoline mechanism as
// x/sys/unix on Darwin, so normal CGO_ENABLED=0 builds retain the ACL gate.
//
//go:cgo_import_dynamic libc_codrax_acl_init acl_init "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_acl_set_fd_np acl_set_fd_np "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_acl_get_fd_np acl_get_fd_np "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_acl_free acl_free "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_acl_get_flagset_np acl_get_flagset_np "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_acl_add_flag_np acl_add_flag_np "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_filesec_init filesec_init "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_filesec_set_property filesec_set_property "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_filesec_free filesec_free "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_codrax_mkdirx_np mkdirx_np "/usr/lib/libSystem.B.dylib"

// createPrivateConversionDirUnixPlatform creates the Darwin staging root with
// an explicit no-inherit ACL in the creation syscall. chmod/acl_set_fd after a
// plain mkdir is too late: an inheritable parent ACL can grant access during
// that gap. mkdirx_np is public but path-based, so the common caller must bind
// the result back to its already-held parent with openat/fstatat before any
// child data is written. An ancestor race can therefore only fail closed and
// leave an empty 0700 directory; it cannot redirect provider input or output.
func createPrivateConversionDirUnixPlatform(_ int, canonicalParent, leaf string) error {
	pathBytes, err := syscall.BytePtrFromString(filepath.Join(canonicalParent, leaf))
	if err != nil {
		return fmt.Errorf("encode private conversion directory path: %w", err)
	}
	acl, err := privateConversionDirDarwinNoInheritACL()
	if err != nil {
		return err
	}
	defer privateConversionDirDarwinACLFree(acl)
	fileSec, _, errno := privateConversionDirDarwinLibcCallPtr(
		privateConversionDirDarwinFileSecInitTrampolineAddr, 0, 0, 0,
	)
	if fileSec == 0 {
		return privateConversionDirDarwinPointerError("initialize Darwin filesec", errno)
	}
	defer privateConversionDirDarwinFileSecFree(fileSec)

	mode := uint16(0o700) // Darwin mode_t is uint16.
	if err := privateConversionDirDarwinIntCall("set Darwin filesec mode",
		privateConversionDirDarwinFileSecSetTrampolineAddr,
		fileSec,
		privateConversionDirDarwinFileSecMode,
		uintptr(unsafe.Pointer(&mode))); err != nil {
		return err
	}
	aclProperty := acl
	if err := privateConversionDirDarwinIntCall("set Darwin filesec no-inherit ACL",
		privateConversionDirDarwinFileSecSetTrampolineAddr,
		fileSec,
		privateConversionDirDarwinFileSecACL,
		uintptr(unsafe.Pointer(&aclProperty))); err != nil {
		return err
	}
	createErr := privateConversionDirDarwinIntCall("mkdirx_np private conversion directory",
		privateConversionDirDarwinMkdirXTrampolineAddr,
		uintptr(unsafe.Pointer(pathBytes)),
		fileSec,
		0)
	// filesec_set_property copies both scalar and ACL payload synchronously;
	// retain the Go values through mkdirx_np for the dynamic-call boundary.
	runtime.KeepAlive(&mode)
	runtime.KeepAlive(&aclProperty)
	runtime.KeepAlive(pathBytes)
	return createErr
}

func validatePrivateConversionDirUnixBirthSecurityPlatform(fd int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("stat Darwin private directory birth security: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o777 != 0o700 {
		return fmt.Errorf("Darwin private directory birth mode=%#o, want directory 0700", stat.Mode)
	}
	if uint64(stat.Uid) != uint64(os.Geteuid()) {
		return fmt.Errorf("Darwin private directory birth owner uid=%d, want effective uid=%d", stat.Uid, os.Geteuid())
	}
	return validatePrivateConversionDirUnixSecurityPlatform(fd)
}

func removePrivateConversionDirUnixCreationPlatform(parentFD int, leaf string, creatorBound bool) error {
	if !creatorBound {
		// mkdirx_np is path-based. Until held/public identity and exact birth
		// security are both proven, unlinking either namespace could delete an
		// unrelated directory after an ancestor race. The only safe residue is
		// the empty, random, 0700/no-ACL directory created by mkdirx_np.
		return nil
	}
	return unix.Unlinkat(parentFD, leaf, unix.AT_REMOVEDIR)
}

func privateConversionDirDarwinNoInheritACL() (uintptr, error) {
	acl, _, errno := privateConversionDirDarwinLibcCallPtr(
		privateConversionDirDarwinACLInitTrampolineAddr, 1, 0, 0,
	)
	if acl == 0 {
		return 0, privateConversionDirDarwinPointerError("initialize Darwin no-inherit ACL", errno)
	}
	var flags uintptr
	if err := privateConversionDirDarwinIntCall("get Darwin ACL flagset",
		privateConversionDirDarwinACLGetFlagsTrampolineAddr,
		acl,
		uintptr(unsafe.Pointer(&flags)),
		0); err != nil {
		privateConversionDirDarwinACLFree(acl)
		return 0, err
	}
	if flags == 0 {
		privateConversionDirDarwinACLFree(acl)
		return 0, fmt.Errorf("get Darwin ACL flagset: acl_get_flagset_np returned NULL")
	}
	if err := privateConversionDirDarwinIntCall("add Darwin ACL no-inherit flag",
		privateConversionDirDarwinACLAddFlagTrampolineAddr,
		flags,
		privateConversionDirDarwinACLNoInherit,
		0); err != nil {
		privateConversionDirDarwinACLFree(acl)
		return 0, err
	}
	runtime.KeepAlive(&flags)
	return acl, nil
}

func securePrivateConversionDirUnixPlatform(fd int) error {
	acl, _, errno := privateConversionDirDarwinLibcCallPtr(privateConversionDirDarwinACLInitTrampolineAddr, 1, 0, 0)
	if acl == 0 {
		return privateConversionDirDarwinPointerError("initialize empty Darwin ACL", errno)
	}
	defer privateConversionDirDarwinACLFree(acl)
	return privateConversionDirDarwinIntCall(
		"set empty Darwin ACL on held directory",
		privateConversionDirDarwinACLSetFDTrampolineAddr,
		uintptr(fd),
		acl,
		privateConversionDirDarwinACLExtended,
	)
}

func validatePrivateConversionDirUnixSecurityPlatform(fd int) error {
	acl, _, errno := privateConversionDirDarwinLibcCallPtr(
		privateConversionDirDarwinACLGetFDTrampolineAddr,
		uintptr(fd),
		privateConversionDirDarwinACLExtended,
		0,
	)
	if acl == 0 {
		if errno == syscall.ENOENT {
			return nil
		}
		if errno == 0 {
			return fmt.Errorf("read Darwin ACL from held directory: acl_get_fd_np returned NULL without ENOENT")
		}
		return fmt.Errorf("read Darwin ACL from held directory: %w", errno)
	}
	defer privateConversionDirDarwinACLFree(acl)
	return fmt.Errorf("held private directory has an extended Darwin ACL object")
}

func privateConversionDirDarwinACLFree(acl uintptr) {
	if acl == 0 {
		return
	}
	_, _, _ = privateConversionDirDarwinLibcCall(privateConversionDirDarwinACLFreeTrampolineAddr, acl, 0, 0)
}

func privateConversionDirDarwinFileSecFree(fileSec uintptr) {
	if fileSec == 0 {
		return
	}
	_, _, _ = privateConversionDirDarwinLibcCall(privateConversionDirDarwinFileSecFreeTrampolineAddr, fileSec, 0, 0)
}

// privateConversionDirDarwinIntCall is the single compiler-visible escape
// boundary for Go pointers passed to integer-returning Darwin libc calls.
// Pointer-to-uintptr conversions must remain direct arguments to this helper.
//
//go:uintptrescapes
func privateConversionDirDarwinIntCall(op string, fn, a1, a2, a3 uintptr) error {
	result, _, errno := privateConversionDirDarwinLibcCall(fn, a1, a2, a3)
	// Darwin's C int result is returned in the low 32 bits; arm64 may leave
	// -1 zero-extended as 0x00000000ffffffff rather than uintptr max.
	if int32(result) != -1 {
		return nil
	}
	if errno == 0 {
		errno = syscall.EIO
	}
	return fmt.Errorf("%s: %w", op, errno)
}

func privateConversionDirDarwinPointerError(op string, errno syscall.Errno) error {
	if errno == 0 {
		errno = syscall.ENOMEM
	}
	return fmt.Errorf("%s: %w", op, errno)
}
