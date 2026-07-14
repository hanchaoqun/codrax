//go:build unix

package hitraceconv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type privateConversionDirPlatformState struct {
	parentFD int
	guardFD  int
	leaf     string
}

func validatePrivateConversionDirChildNamePlatform(string) error { return nil }

func validatePrivateConversionDirPublicBindingPlatform(path string, identity os.FileInfo, _ *privateConversionDirPlatformState) error {
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if identity == nil || !current.IsDir() || !os.SameFile(identity, current) {
		return fmt.Errorf("public directory identity mismatch")
	}
	return nil
}

func openPrivateConversionDirRootPlatform(path string, _ os.FileInfo, _ *privateConversionDirPlatformState) (*os.Root, error) {
	return os.OpenRoot(path)
}

func removePrivateConversionDirChildrenPlatform(string, os.FileInfo, *privateConversionDirPlatformState) error {
	return fmt.Errorf("POSIX private directory Root authority is missing")
}

func createPrivateConversionDirPlatform(parent, pattern string) (string, os.FileInfo, privateConversionDirPlatformState, error) {
	if parent == "" {
		parent = os.TempDir()
	}
	abs, err := filepath.Abs(filepath.Clean(parent))
	if err != nil {
		return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("resolve private conversion parent: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("resolve private conversion parent symlinks: %w", err)
	}
	parentFD, err := unix.Open(canonical, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("open private conversion parent: %w", err)
	}
	state := privateConversionDirPlatformState{parentFD: parentFD, guardFD: -1}
	var path string
	for attempt := 0; attempt < privateConversionDirCreateAttempts; attempt++ {
		leaf, leafErr := nextPrivateConversionDirLeaf(pattern)
		if leafErr != nil {
			_ = closePrivateConversionDirPlatform(&state)
			return "", nil, privateConversionDirPlatformState{}, leafErr
		}
		if err := createPrivateConversionDirUnixPlatform(state.parentFD, canonical, leaf); err != nil {
			if errors.Is(err, unix.EEXIST) {
				continue
			}
			_ = closePrivateConversionDirPlatform(&state)
			return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("create private conversion directory with platform security authority: %w", err)
		}
		state.leaf = leaf
		path = filepath.Join(canonical, leaf)
		break
	}
	if state.leaf == "" {
		_ = closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, fmt.Errorf("create private conversion directory: exhausted %d collision attempts", privateConversionDirCreateAttempts)
	}
	state.guardFD, err = unix.Openat(state.parentFD, state.leaf, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		removeErr := removePrivateConversionDirUnixCreationPlatform(state.parentFD, state.leaf, false)
		closeErr := closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, traceDBJoinPreservingSingle(
			fmt.Errorf("open private conversion directory guard: %w", err), removeErr, closeErr,
		)
	}
	heldInfo, err := privateConversionDirUnixEntryInfo(&state)
	if err != nil {
		removeErr := removePrivateConversionDirUnixCreationPlatform(state.parentFD, state.leaf, false)
		closeErr := closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, traceDBJoinPreservingSingle(err, removeErr, closeErr)
	}
	publicInfo, err := os.Lstat(path)
	if err != nil || !publicInfo.IsDir() || !os.SameFile(heldInfo, publicInfo) {
		if err == nil {
			err = fmt.Errorf("public path does not name the held private directory")
		}
		removeErr := removePrivateConversionDirUnixCreationPlatform(state.parentFD, state.leaf, false)
		closeErr := closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, traceDBJoinPreservingSingle(
			fmt.Errorf("bind newly-created private conversion directory: %w", err), removeErr, closeErr,
		)
	}
	if err := validatePrivateConversionDirUnixBirthSecurityPlatform(state.guardFD); err != nil {
		removeErr := removePrivateConversionDirUnixCreationPlatform(state.parentFD, state.leaf, false)
		closeErr := closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, traceDBJoinPreservingSingle(
			fmt.Errorf("validate newly-created private conversion directory security: %w", err), removeErr, closeErr,
		)
	}
	var birthStat unix.Stat_t
	if err := unix.Fstat(state.guardFD, &birthStat); err != nil || birthStat.Mode&unix.S_IFMT != unix.S_IFDIR || uint64(birthStat.Uid) != uint64(os.Geteuid()) {
		if err == nil {
			err = fmt.Errorf("birth owner/type mismatch: uid=%d mode=%#o effective_uid=%d", birthStat.Uid, birthStat.Mode, os.Geteuid())
		}
		removeErr := removePrivateConversionDirUnixCreationPlatform(state.parentFD, state.leaf, false)
		closeErr := closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, traceDBJoinPreservingSingle(
			fmt.Errorf("validate newly-created private conversion directory identity: %w", err), removeErr, closeErr,
		)
	}
	creatorBound := true
	if err := unix.Fchmod(state.guardFD, 0o700); err != nil {
		removeErr := removePrivateConversionDirUnixCreationPlatform(state.parentFD, state.leaf, creatorBound)
		closeErr := closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, traceDBJoinPreservingSingle(
			fmt.Errorf("secure private conversion directory guard: %w", err), removeErr, closeErr,
		)
	}
	if err := securePrivateConversionDirUnixPlatform(state.guardFD); err != nil {
		removeErr := removePrivateConversionDirUnixCreationPlatform(state.parentFD, state.leaf, creatorBound)
		closeErr := closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, traceDBJoinPreservingSingle(
			fmt.Errorf("clear inherited private directory security metadata: %w", err), removeErr, closeErr,
		)
	}
	identity, err := privateConversionDirUnixEntryInfo(&state)
	if err != nil {
		removeErr := removePrivateConversionDirUnixCreationPlatform(state.parentFD, state.leaf, creatorBound)
		closeErr := closePrivateConversionDirPlatform(&state)
		return "", nil, privateConversionDirPlatformState{}, traceDBJoinPreservingSingle(err, removeErr, closeErr)
	}
	return path, identity, state, nil
}

func privateConversionDirUnixEntryInfo(state *privateConversionDirPlatformState) (os.FileInfo, error) {
	if state == nil || state.guardFD < 0 {
		return nil, fmt.Errorf("POSIX private directory authority is incomplete")
	}
	fd, err := unix.Dup(state.guardFD)
	if err != nil {
		return nil, fmt.Errorf("duplicate private directory guard: %w", err)
	}
	unix.CloseOnExec(fd)
	file := os.NewFile(uintptr(fd), state.leaf)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("wrap private directory parent-relative handle")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat private directory parent-relative handle: %w", err)
	}
	return info, nil
}

func privateConversionDirUnixParentEntryMatches(state *privateConversionDirPlatformState) error {
	if state == nil || state.parentFD < 0 || state.guardFD < 0 || state.leaf == "" {
		return fmt.Errorf("POSIX private directory authority is incomplete")
	}
	var held unix.Stat_t
	if err := unix.Fstat(state.guardFD, &held); err != nil {
		return fmt.Errorf("stat held private directory guard: %w", err)
	}
	var current unix.Stat_t
	if err := unix.Fstatat(state.parentFD, state.leaf, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("stat private directory relative to held parent: %w", err)
	}
	if held.Dev != current.Dev || held.Ino != current.Ino || current.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("parent-relative directory identity mismatch")
	}
	return nil
}

func validatePrivateConversionDirIdentityPlatform(_ string, identity os.FileInfo, state *privateConversionDirPlatformState) error {
	current, err := privateConversionDirUnixEntryInfo(state)
	if err != nil {
		return err
	}
	if identity == nil || !current.IsDir() || !os.SameFile(identity, current) {
		return fmt.Errorf("held directory identity mismatch")
	}
	return privateConversionDirUnixParentEntryMatches(state)
}

func validatePrivateConversionDirSecurityPlatform(path string, identity os.FileInfo, state *privateConversionDirPlatformState) error {
	if err := validatePrivateConversionDirIdentityPlatform(path, identity, state); err != nil {
		return err
	}
	current, err := privateConversionDirUnixEntryInfo(state)
	if err != nil {
		return err
	}
	if current.Mode().Perm() != 0o700 {
		return fmt.Errorf("mode=%s, want 0700", current.Mode())
	}
	var held unix.Stat_t
	if err := unix.Fstat(state.guardFD, &held); err != nil {
		return fmt.Errorf("stat private directory security owner: %w", err)
	}
	if uint64(held.Uid) != uint64(os.Geteuid()) {
		return fmt.Errorf("owner uid=%d, want effective uid=%d", held.Uid, os.Geteuid())
	}
	if err := validatePrivateConversionDirUnixSecurityPlatform(state.guardFD); err != nil {
		return err
	}
	return nil
}

func preparePrivateConversionDirCleanupPlatform(_ string, identity os.FileInfo, root *os.Root, state *privateConversionDirPlatformState) error {
	if root == nil {
		return fmt.Errorf("POSIX private directory root authority is missing")
	}
	rootInfo, err := privateConversionDirUnixEntryInfo(state)
	if err != nil {
		return fmt.Errorf("%w: stat held private directory guard during cleanup: %v", errPrivateConversionDirIdentityChanged, err)
	}
	if identity == nil || !rootInfo.IsDir() || !os.SameFile(identity, rootInfo) {
		return fmt.Errorf("%w: held private directory guard mismatch during cleanup", errPrivateConversionDirIdentityChanged)
	}
	if state == nil || state.guardFD < 0 {
		return fmt.Errorf("%w: POSIX private directory guard is missing", errPrivateConversionDirIdentityChanged)
	}
	if err := unix.Fchmod(state.guardFD, 0o700); err != nil {
		return fmt.Errorf("restore private directory cleanup mode: %w", err)
	}
	if err := securePrivateConversionDirUnixPlatform(state.guardFD); err != nil {
		return fmt.Errorf("restore private directory cleanup security metadata: %w", err)
	}
	if err := validatePrivateConversionDirIdentityPlatform("", identity, state); err != nil {
		return fmt.Errorf("%w: %v", errPrivateConversionDirIdentityChanged, err)
	}
	return nil
}

func removePrivateConversionDirRootPlatform(_ string, identity os.FileInfo, state *privateConversionDirPlatformState) error {
	if err := validatePrivateConversionDirIdentityPlatform("", identity, state); err != nil {
		return err
	}
	if err := unix.Unlinkat(state.parentFD, state.leaf, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

func closePrivateConversionDirPlatform(state *privateConversionDirPlatformState) error {
	if state == nil {
		return nil
	}
	var result error
	if state.guardFD >= 0 {
		fd := state.guardFD
		state.guardFD = -1
		result = traceDBJoinPreservingSingle(result, unix.Close(fd))
	}
	if state.parentFD >= 0 {
		fd := state.parentFD
		state.parentFD = -1
		result = traceDBJoinPreservingSingle(result, unix.Close(fd))
	}
	return result
}
