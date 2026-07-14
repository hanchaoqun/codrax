//go:build windows

package hitraceconv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	privateConversionDirFullControl        = windows.ACCESS_MASK(0x001f01ff)
	privateConversionDirWindowsDirBuffer   = 64 * 1024
	privateConversionDirWindowsNameOffset  = 68
	privateConversionDirWindowsMaxDepth    = 4096
	privateConversionDirWindowsObjectAttrs = windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE
)

type privateConversionDirPlatformState struct {
	guard         *os.File
	parent        windows.Handle
	leaf          string
	volumeSerial  uint32
	fileIndexHigh uint32
	fileIndexLow  uint32
}

func validatePrivateConversionDirChildNamePlatform(base string) error {
	if strings.TrimRight(base, " .") != base || strings.ContainsAny(base, `<>:"/\|?*`) {
		return fmt.Errorf("name is not a plain Windows file name")
	}
	for _, char := range base {
		if char < 0x20 {
			return fmt.Errorf("name contains a control character")
		}
	}
	stem := strings.ToUpper(strings.SplitN(base, ".", 2)[0])
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$":
		return fmt.Errorf("name uses a reserved Windows device stem")
	}
	if len(stem) == 4 && stem[3] >= '1' && stem[3] <= '9' && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) {
		return fmt.Errorf("name uses a reserved Windows device stem")
	}
	return nil
}

func createPrivateConversionDirPlatform(parent, pattern string) (string, os.FileInfo, privateConversionDirPlatformState, error) {
	if parent == "" {
		parent = os.TempDir()
	}
	absParent, err := filepath.Abs(filepath.Clean(parent))
	if err != nil {
		return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("resolve private conversion parent: %w", err)
	}
	canonicalParent, err := filepath.EvalSymlinks(absParent)
	if err != nil {
		return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("resolve private conversion parent symlinks: %w", err)
	}
	if _, _, err := splitPrivateConversionDirPattern(pattern); err != nil {
		return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("create private conversion directory: %w", err)
	}
	parentHandle, err := openPrivateConversionDirWindowsParent(canonicalParent)
	if err != nil {
		return "", nil, privateConversionDirPlatformState{}, err
	}
	state := privateConversionDirPlatformState{parent: parentHandle}
	sd, err := privateConversionDirSecurityDescriptor()
	if err != nil {
		_ = closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("build private conversion directory security descriptor: %w", err)
	}
	for attempt := 0; attempt < privateConversionDirCreateAttempts; attempt++ {
		leaf, err := nextPrivateConversionDirLeaf(pattern)
		if err != nil {
			_ = closePrivateConversionDirPlatform(&state)
			return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("create private conversion directory: %w", err)
		}
		guard, info, handleInfo, err := createPrivateConversionDirWindowsRelative(parentHandle, leaf, sd)
		runtime.KeepAlive(sd)
		if err != nil {
			if errors.Is(err, windows.STATUS_OBJECT_NAME_COLLISION) || errors.Is(err, windows.STATUS_OBJECT_NAME_EXISTS) {
				continue
			}
			_ = closePrivateConversionDirPlatform(&state)
			return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("create private conversion directory relative to held parent: %w", err)
		}
		state.guard = guard
		state.leaf = leaf
		state.volumeSerial = handleInfo.VolumeSerialNumber
		state.fileIndexHigh = handleInfo.FileIndexHigh
		state.fileIndexLow = handleInfo.FileIndexLow
		return filepath.Join(canonicalParent, leaf), info, state, nil
	}
	_ = closePrivateConversionDirPlatform(&state)
	return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("create private conversion directory: exhausted %d collision attempts", privateConversionDirCreateAttempts)
}

func openPrivateConversionDirWindowsParent(path string) (windows.Handle, error) {
	pathPtr, err := privateConversionDirWindowsAPIPath(path)
	if err != nil {
		return 0, fmt.Errorf("encode private conversion parent path: %w", err)
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return 0, fmt.Errorf("open private conversion parent authority: %w", err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, fmt.Errorf("inspect private conversion parent authority: %w", err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return 0, fmt.Errorf("private conversion parent is not a plain directory: attributes=0x%x", info.FileAttributes)
	}
	return handle, nil
}

func createPrivateConversionDirWindowsRelative(parent windows.Handle, leaf string, sd *windows.SECURITY_DESCRIPTOR) (*os.File, os.FileInfo, windows.ByHandleFileInformation, error) {
	objectName, err := windows.NewNTUnicodeString(leaf)
	if err != nil {
		return nil, nil, windows.ByHandleFileInformation{}, err
	}
	oa := &windows.OBJECT_ATTRIBUTES{
		Length:             uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory:      parent,
		ObjectName:         objectName,
		Attributes:         privateConversionDirWindowsObjectAttrs,
		SecurityDescriptor: sd,
	}
	var handle windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	allocationSize := int64(0)
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES|windows.FILE_WRITE_ATTRIBUTES|windows.READ_CONTROL|windows.WRITE_DAC|windows.DELETE|windows.SYNCHRONIZE,
		oa,
		&iosb,
		&allocationSize,
		windows.FILE_ATTRIBUTE_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_CREATE,
		windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT,
		0,
		0,
	)
	runtime.KeepAlive(objectName)
	runtime.KeepAlive(sd)
	if err != nil {
		return nil, nil, windows.ByHandleFileInformation{}, err
	}
	file := os.NewFile(uintptr(handle), leaf)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, nil, windows.ByHandleFileInformation{}, fmt.Errorf("wrap private directory authority handle")
	}
	info, statErr := file.Stat()
	var handleInfo windows.ByHandleFileInformation
	infoErr := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &handleInfo)
	if statErr != nil || infoErr != nil || !info.IsDir() || handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 ||
		handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		deleteErr := markPrivateConversionDirWindowsHandleForDeletion(windows.Handle(file.Fd()))
		closeErr := file.Close()
		return nil, nil, windows.ByHandleFileInformation{}, traceDBJoinPreservingSingle(
			fmt.Errorf("inspect created private directory: stat=%v info=%v attributes=0x%x", statErr, infoErr, handleInfo.FileAttributes),
			deleteErr,
			closeErr,
		)
	}
	return file, info, handleInfo, nil
}

func privateConversionDirWindowsAPIPath(path string) (*uint16, error) {
	clean := filepath.Clean(path)
	switch {
	case strings.HasPrefix(clean, `\\?\`):
		// Already in the Win32 extended-length namespace.
	case strings.HasPrefix(clean, `\\.\`):
		return nil, fmt.Errorf("device namespace paths are not accepted")
	case strings.HasPrefix(clean, `\\`):
		clean = `\\?\UNC\` + strings.TrimPrefix(clean, `\\`)
	default:
		clean = `\\?\` + clean
	}
	return windows.UTF16PtrFromString(clean)
}

func privateConversionDirSecurityDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	user, system, err := privateConversionDirPrincipals()
	if err != nil {
		return nil, err
	}
	sddl := "O:" + user.String() + "D:P(A;OICI;FA;;;" + user.String() + ")"
	if !user.Equals(system) {
		sddl += "(A;OICI;FA;;;" + system.String() + ")"
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, err
	}
	if sd == nil || !sd.IsValid() {
		return nil, fmt.Errorf("constructed security descriptor is invalid")
	}
	return sd, nil
}

func privateConversionDirPrincipals() (user, system *windows.SID, err error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, fmt.Errorf("read current process token user: %w", err)
	}
	if tokenUser == nil || tokenUser.User.Sid == nil || !tokenUser.User.Sid.IsValid() {
		return nil, nil, fmt.Errorf("current process token user SID is invalid")
	}
	system, err = windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, nil, fmt.Errorf("create LocalSystem SID: %w", err)
	}
	return tokenUser.User.Sid, system, nil
}

func privateConversionDirWindowsHandleInfo(state *privateConversionDirPlatformState) (windows.ByHandleFileInformation, error) {
	if state == nil || state.guard == nil {
		return windows.ByHandleFileInformation{}, fmt.Errorf("Windows private directory authority is incomplete")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(state.guard.Fd()), &info); err != nil {
		return windows.ByHandleFileInformation{}, err
	}
	return info, nil
}

func privateConversionDirWindowsSameIdentity(state *privateConversionDirPlatformState, info windows.ByHandleFileInformation) bool {
	return state != nil && info.VolumeSerialNumber == state.volumeSerial && info.FileIndexHigh == state.fileIndexHigh && info.FileIndexLow == state.fileIndexLow
}

func validatePrivateConversionDirPublicBindingPlatform(path string, identity os.FileInfo, state *privateConversionDirPlatformState) error {
	if identity == nil || state == nil || state.guard == nil {
		return fmt.Errorf("Windows private directory authority is incomplete")
	}
	pathPtr, err := privateConversionDirWindowsAPIPath(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if !privateConversionDirWindowsSameIdentity(state, info) || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("public directory identity mismatch")
	}
	return nil
}

func openPrivateConversionDirRootPlatform(string, os.FileInfo, *privateConversionDirPlatformState) (*os.Root, error) {
	// Go's Windows os.Root handle does not share DELETE access, so it cannot
	// coexist with the DELETE-capable, no-delete-share authority. Windows child
	// cleanup is therefore implemented entirely relative to the held NT handle.
	return nil, nil
}

func validatePrivateConversionDirIdentityPlatform(_ string, identity os.FileInfo, state *privateConversionDirPlatformState) error {
	if state == nil || state.guard == nil || identity == nil {
		return fmt.Errorf("Windows private directory authority is incomplete")
	}
	openedInfo, err := state.guard.Stat()
	if err != nil {
		return fmt.Errorf("stat held directory authority: %w", err)
	}
	handleInfo, err := privateConversionDirWindowsHandleInfo(state)
	if err != nil {
		return fmt.Errorf("read held directory authority attributes: %w", err)
	}
	if !openedInfo.IsDir() || !privateConversionDirWindowsSameIdentity(state, handleInfo) ||
		handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("held path is not the original plain directory: attributes=0x%x", handleInfo.FileAttributes)
	}
	return nil
}

func validatePrivateConversionDirSecurityPlatform(path string, identity os.FileInfo, state *privateConversionDirPlatformState) error {
	if err := validatePrivateConversionDirIdentityPlatform(path, identity, state); err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(
		windows.Handle(state.guard.Fd()),
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("read security descriptor: %w", err)
	}
	if sd == nil || !sd.IsValid() {
		return fmt.Errorf("security descriptor is missing or invalid")
	}
	control, _, err := sd.Control()
	if err != nil {
		return fmt.Errorf("read security descriptor control: %w", err)
	}
	if control&windows.SE_DACL_PRESENT == 0 || control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("DACL is not present and protected: control=0x%x", uint16(control))
	}
	owner, ownerDefaulted, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("read security descriptor owner: %w", err)
	}
	user, system, err := privateConversionDirPrincipals()
	if err != nil {
		return err
	}
	if owner == nil || ownerDefaulted || !owner.Equals(user) {
		return fmt.Errorf("directory owner is not the current process user")
	}
	dacl, daclDefaulted, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read directory DACL: %w", err)
	}
	if dacl == nil || daclDefaulted {
		return fmt.Errorf("directory DACL is nil or defaulted")
	}
	wantACECount := uint16(2)
	if user.Equals(system) {
		wantACECount = 1
	}
	if dacl.AceCount != wantACECount {
		return fmt.Errorf("directory DACL ACE count=%d, want %d", dacl.AceCount, wantACECount)
	}
	seenUser := false
	seenSystem := false
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return fmt.Errorf("read directory DACL ACE %d: %w", index, err)
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("directory DACL ACE %d is not access-allowed", index)
		}
		wantFlags := uint8(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
		if ace.Header.AceFlags != wantFlags || ace.Mask != privateConversionDirFullControl {
			return fmt.Errorf("directory DACL ACE %d flags=0x%x mask=0x%x", index, ace.Header.AceFlags, uint32(ace.Mask))
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.IsValid() {
			return fmt.Errorf("directory DACL ACE %d SID is invalid", index)
		}
		switch {
		case sid.Equals(user):
			if seenUser {
				return fmt.Errorf("directory DACL repeats current user ACE")
			}
			seenUser = true
		case sid.Equals(system):
			if seenSystem {
				return fmt.Errorf("directory DACL repeats LocalSystem ACE")
			}
			seenSystem = true
		default:
			return fmt.Errorf("directory DACL grants an unexpected SID %s", sid.String())
		}
	}
	if !seenUser || (!user.Equals(system) && !seenSystem) {
		return fmt.Errorf("directory DACL is missing the required user or LocalSystem ACE")
	}
	return nil
}

func preparePrivateConversionDirCleanupPlatform(path string, identity os.FileInfo, _ *os.Root, state *privateConversionDirPlatformState) error {
	if err := validatePrivateConversionDirIdentityPlatform(path, identity, state); err != nil {
		return fmt.Errorf("%w: %v", errPrivateConversionDirIdentityChanged, err)
	}
	sd, err := privateConversionDirSecurityDescriptor()
	if err != nil {
		return fmt.Errorf("rebuild private directory cleanup DACL: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read private directory cleanup DACL: %w", err)
	}
	if dacl == nil {
		return fmt.Errorf("private directory cleanup DACL is nil")
	}
	err = windows.SetSecurityInfo(
		windows.Handle(state.guard.Fd()),
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
	runtime.KeepAlive(sd)
	if err != nil {
		return fmt.Errorf("restore private directory cleanup DACL through held handle: %w", err)
	}
	return nil
}

func removePrivateConversionDirChildrenPlatform(_ string, identity os.FileInfo, state *privateConversionDirPlatformState) error {
	if err := validatePrivateConversionDirIdentityPlatform("", identity, state); err != nil {
		return err
	}
	removed := 0
	return removePrivateConversionDirWindowsChildren(windows.Handle(state.guard.Fd()), 0, &removed)
}

func removePrivateConversionDirWindowsChildren(parent windows.Handle, depth int, removed *int) error {
	if depth > privateConversionDirWindowsMaxDepth {
		return fmt.Errorf("private conversion directory cleanup depth exceeded: %d", privateConversionDirWindowsMaxDepth)
	}
	for {
		names, err := privateConversionDirWindowsDirectoryNames(parent)
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return nil
		}
		if *removed > privateConversionDirCleanupEntryLimit-len(names) {
			return fmt.Errorf("private conversion directory cleanup entry limit exceeded: %d", privateConversionDirCleanupEntryLimit)
		}
		for _, name := range names {
			child, attributes, err := openPrivateConversionDirWindowsChild(parent, name)
			if err != nil {
				return fmt.Errorf("open private conversion directory child %q: %w", name, err)
			}
			if attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
				err = removePrivateConversionDirWindowsChildren(windows.Handle(child.Fd()), depth+1, removed)
			}
			if err == nil {
				err = markPrivateConversionDirWindowsHandleForDeletion(windows.Handle(child.Fd()))
			}
			closeErr := child.Close()
			if err != nil || closeErr != nil {
				var removeErr error
				if err != nil {
					removeErr = fmt.Errorf("remove private conversion directory child %q: %w", name, err)
				}
				return traceDBJoinPreservingSingle(removeErr, closeErr)
			}
			*removed++
		}
	}
}

func privateConversionDirWindowsDirectoryNames(handle windows.Handle) ([]string, error) {
	buffer := make([]byte, privateConversionDirWindowsDirBuffer)
	err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileFullDirectoryRestartInfo,
		&buffer[0],
		uint32(len(buffer)),
	)
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("enumerate private conversion directory handle: %w", err)
	}
	var names []string
	for offset := 0; ; {
		if offset < 0 || offset+privateConversionDirWindowsNameOffset > len(buffer) {
			return nil, fmt.Errorf("private conversion directory enumeration record is truncated")
		}
		next := int(binary.LittleEndian.Uint32(buffer[offset : offset+4]))
		nameBytes := int(binary.LittleEndian.Uint32(buffer[offset+60 : offset+64]))
		if nameBytes < 0 || nameBytes%2 != 0 || offset+privateConversionDirWindowsNameOffset+nameBytes > len(buffer) {
			return nil, fmt.Errorf("private conversion directory enumeration name is invalid")
		}
		encoded := make([]uint16, nameBytes/2)
		for index := range encoded {
			start := offset + privateConversionDirWindowsNameOffset + index*2
			encoded[index] = binary.LittleEndian.Uint16(buffer[start : start+2])
		}
		name := string(utf16.Decode(encoded))
		if name != "." && name != ".." {
			names = append(names, name)
		}
		if next == 0 {
			break
		}
		if next < privateConversionDirWindowsNameOffset || offset+next > len(buffer) {
			return nil, fmt.Errorf("private conversion directory enumeration next offset is invalid")
		}
		offset += next
	}
	return names, nil
}

func openPrivateConversionDirWindowsChild(parent windows.Handle, name string) (*os.File, uint32, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, 0, err
	}
	oa := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE,
	}
	var handle windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	allocationSize := int64(0)
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES|windows.FILE_WRITE_ATTRIBUTES|windows.DELETE|windows.SYNCHRONIZE,
		oa,
		&iosb,
		&allocationSize,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT,
		0,
		0,
	)
	runtime.KeepAlive(objectName)
	if err != nil {
		return nil, 0, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, 0, fmt.Errorf("wrap private directory child handle")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	return file, info.FileAttributes, nil
}

func markPrivateConversionDirWindowsHandleForDeletion(handle windows.Handle) error {
	if err := clearPrivateConversionDirWindowsReadOnly(handle); err != nil {
		return err
	}
	// FILE_DISPOSITION_INFO uses the one-byte Windows BOOLEAN type.
	deleteFile := byte(1)
	return windows.SetFileInformationByHandle(handle, windows.FileDispositionInfo, &deleteFile, 1)
}

type privateConversionDirWindowsFileBasicInfo struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangeTime     int64
	FileAttributes uint32
	_              uint32
}

func clearPrivateConversionDirWindowsReadOnly(handle windows.Handle) error {
	var info privateConversionDirWindowsFileBasicInfo
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileBasicInfo,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return fmt.Errorf("read private conversion entry attributes before deletion: %w", err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_READONLY == 0 {
		return nil
	}
	info.FileAttributes &^= windows.FILE_ATTRIBUTE_READONLY
	if info.FileAttributes == 0 {
		info.FileAttributes = windows.FILE_ATTRIBUTE_NORMAL
	}
	if err := windows.SetFileInformationByHandle(
		handle,
		windows.FileBasicInfo,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return fmt.Errorf("clear read-only private conversion entry attribute: %w", err)
	}
	return nil
}

func removePrivateConversionDirRootPlatform(path string, identity os.FileInfo, state *privateConversionDirPlatformState) error {
	if err := validatePrivateConversionDirIdentityPlatform(path, identity, state); err != nil {
		return err
	}
	return markPrivateConversionDirWindowsHandleForDeletion(windows.Handle(state.guard.Fd()))
}

func closePrivateConversionDirPlatform(state *privateConversionDirPlatformState) error {
	if state == nil {
		return nil
	}
	var result error
	if state.guard != nil {
		guard := state.guard
		state.guard = nil
		result = traceDBJoinPreservingSingle(result, guard.Close())
	}
	if state.parent != 0 && state.parent != windows.InvalidHandle {
		parent := state.parent
		state.parent = 0
		result = traceDBJoinPreservingSingle(result, windows.CloseHandle(parent))
	}
	return result
}
