//go:build unix

package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReleasePrivateConversionDirUnixAmbiguousCreationNeverUnlinksHeldEntry(t *testing.T) {
	parent := t.TempDir()
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parentFD)
	const leaf = "unrelated-held-entry"
	path := filepath.Join(parent, leaf)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removePrivateConversionDirUnixCreationPlatform(parentFD, leaf, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("ambiguous POSIX cleanup removed held entry: %v", err)
	}
	if err := removePrivateConversionDirUnixCreationPlatform(parentFD, leaf, true); err != nil {
		t.Fatal(err)
	}
}

func TestReleasePrivateConversionDirCommandBoundaryPreservesAllFailures(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-private-boundary-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir.Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runErr := errors.New("adapter exit 23")
	err = privateConversionDirCommandBoundaryError(ctx, runErr, dir)
	for _, want := range []error{context.Canceled, runErr, errPrivateConversionDirSecurityInvalid} {
		if !errors.Is(err, want) {
			t.Fatalf("command boundary error %v lost %v", err, want)
		}
	}
	if err := dir.FinalizeCleanup(); err != nil {
		t.Fatalf("terminal cleanup after mixed command failure: %v", err)
	}
}

func TestReleasePrivateConversionDirFinalizeClosesRetryAuthority(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-private-finalize-*")
	if err != nil {
		t.Fatal(err)
	}
	dir.mu.Lock()
	heldRoot := dir.root
	dir.root = nil // deterministically force the retryable missing-Root branch
	dir.mu.Unlock()

	firstErr := dir.FinalizeCleanup()
	if firstErr == nil {
		t.Fatal("terminal cleanup unexpectedly accepted a missing Root authority")
	}
	dir.mu.Lock()
	terminal := dir.terminal
	parentFD := dir.platform.parentFD
	guardFD := dir.platform.guardFD
	dir.mu.Unlock()
	if !terminal || parentFD >= 0 || guardFD >= 0 {
		t.Fatalf("provider finalization retained authority: terminal=%t parent_fd=%d guard_fd=%d", terminal, parentFD, guardFD)
	}
	if retryErr := dir.FinalizeCleanup(); retryErr == nil || retryErr.Error() != firstErr.Error() {
		t.Fatalf("terminal cleanup result drifted: first=%v retry=%v", firstErr, retryErr)
	}
	if heldRoot != nil {
		if err := heldRoot.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(dir.Path()); err != nil {
		t.Fatal(err)
	}
}

func TestReleasePrivateConversionDirUnixLifecycleAndOutsideSymlink(t *testing.T) {
	parent := t.TempDir()
	dir, err := newPrivateConversionDir(parent, "codrax-private-*-stage")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(dir.Path()), "codrax-private-") || !strings.HasSuffix(filepath.Base(dir.Path()), "-stage") {
		t.Fatalf("private directory pattern was not preserved: %s", dir.Path())
	}
	info, err := os.Lstat(dir.Path())
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("private directory mode mismatch: info=%v err=%v", info, err)
	}
	child, err := dir.ChildPath("nested")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(child, "deeper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "deeper", "payload"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideSentinel := filepath.Join(outside, "sentinel")
	wantSentinel := []byte("must-survive")
	if err := os.WriteFile(outsideSentinel, wantSentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir.Path(), "outside-link")); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	for _, invalid := range []string{"", ".", "..", "a/b", filepath.Join("a", "b")} {
		if _, err := dir.ChildPath(invalid); err == nil {
			t.Fatalf("invalid child name %q was accepted", invalid)
		}
	}
	if err := dir.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatalf("private directory cleanup is not idempotent: %v", err)
	}
	if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
		t.Fatalf("private directory survived cleanup: %v", err)
	}
	gotSentinel, err := os.ReadFile(outsideSentinel)
	if err != nil || !bytes.Equal(gotSentinel, wantSentinel) {
		t.Fatalf("cleanup followed outside symlink: got=%q err=%v", gotSentinel, err)
	}
}

func TestReleasePrivateConversionDirUnixSecurityDriftStillCleansOwnedIdentity(t *testing.T) {
	for _, mode := range []os.FileMode{0o755, 0o000} {
		t.Run(mode.String(), func(t *testing.T) {
			dir, err := newPrivateConversionDir(t.TempDir(), "codrax-private-mode-*")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir.Path(), mode); err != nil {
				t.Fatal(err)
			}
			if err := dir.Validate(); !errors.Is(err, errPrivateConversionDirSecurityInvalid) {
				t.Fatalf("permission drift error=%v, want security sentinel", err)
			}
			if err := dir.Cleanup(); err != nil {
				t.Fatalf("security drift prevented identity-safe cleanup: %v", err)
			}
			if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
				t.Fatalf("security-drift directory survived cleanup: %v", err)
			}
		})
	}
}

func TestReleasePrivateConversionDirUnixRootDeleteFailureRetainsAuthorityForRetry(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-private-retry-*")
	if err != nil {
		t.Fatal(err)
	}
	dir.mu.Lock()
	if err := dir.removeChildrenLocked(); err != nil {
		dir.mu.Unlock()
		t.Fatal(err)
	}
	lateChild, err := dir.ChildPath("late-child")
	if err != nil {
		dir.mu.Unlock()
		t.Fatal(err)
	}
	if err := os.WriteFile(lateChild, []byte("late"), 0o600); err != nil {
		dir.mu.Unlock()
		t.Fatal(err)
	}
	removeErr := removePrivateConversionDirRootPlatform(dir.path, dir.identity, &dir.platform)
	rootRetained := dir.root != nil
	dir.mu.Unlock()
	if removeErr == nil {
		t.Fatal("non-empty root removal unexpectedly succeeded")
	}
	if !rootRetained {
		t.Fatal("failed root removal consumed the held Root authority")
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatalf("cleanup retry did not remove the late child: %v", err)
	}
	if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
		t.Fatalf("retried private directory survived cleanup: %v", err)
	}
}

func TestReleasePrivateConversionDirUnixReplacementIsTerminalAndSentinelSafe(t *testing.T) {
	parent := t.TempDir()
	dir, err := newPrivateConversionDir(parent, "codrax-private-replace-*")
	if err != nil {
		t.Fatal(err)
	}
	originalPath := dir.Path()
	movedOriginal := originalPath + ".original"
	if err := os.Rename(originalPath, movedOriginal); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(originalPath, 0o700); err != nil {
		t.Fatal(err)
	}
	replacementSentinel := filepath.Join(originalPath, "replacement-sentinel")
	want := []byte("external-owner")
	if err := os.WriteFile(replacementSentinel, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dir.Validate(); !errors.Is(err, errPrivateConversionDirIdentityChanged) {
		t.Fatalf("replacement validation error=%v, want identity sentinel", err)
	}
	firstErr := dir.Cleanup()
	if !errors.Is(firstErr, errPrivateConversionDirIdentityChanged) {
		t.Fatalf("replacement cleanup error=%v, want identity sentinel", firstErr)
	}
	got, err := os.ReadFile(replacementSentinel)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("replacement sentinel changed: got=%q err=%v", got, err)
	}
	if _, err := os.Stat(movedOriginal); err != nil {
		t.Fatalf("original directory was removed through its stale name: %v", err)
	}

	replacementSaved := originalPath + ".replacement"
	if err := os.Rename(originalPath, replacementSaved); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(movedOriginal, originalPath); err != nil {
		t.Fatal(err)
	}
	if retryErr := dir.Cleanup(); retryErr == nil || retryErr.Error() != firstErr.Error() {
		t.Fatalf("terminal cleanup retried another generation: first=%v retry=%v", firstErr, retryErr)
	}
	if _, err := os.Stat(originalPath); err != nil {
		t.Fatalf("terminal retry deleted restored original generation: %v", err)
	}
	if err := os.RemoveAll(originalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(replacementSaved); err != nil {
		t.Fatal(err)
	}
}

func TestReleasePrivateConversionDirUnixSymlinkReplacementDoesNotFollowTarget(t *testing.T) {
	parent := t.TempDir()
	dir, err := newPrivateConversionDir(parent, "codrax-private-link-*")
	if err != nil {
		t.Fatal(err)
	}
	originalPath := dir.Path()
	movedOriginal := originalPath + ".original"
	if err := os.Rename(originalPath, movedOriginal); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "external-target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "sentinel")
	if err := os.WriteFile(sentinel, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, originalPath); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	if err := dir.Cleanup(); !errors.Is(err, errPrivateConversionDirIdentityChanged) {
		t.Fatalf("symlink replacement cleanup error=%v, want identity sentinel", err)
	}
	if body, err := os.ReadFile(sentinel); err != nil || string(body) != "external" {
		t.Fatalf("cleanup followed replacement symlink: body=%q err=%v", body, err)
	}
	if info, err := os.Lstat(originalPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("replacement symlink was removed: info=%v err=%v", info, err)
	}
	if err := os.Remove(originalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(movedOriginal); err != nil {
		t.Fatal(err)
	}
}

func TestReleasePrivateConversionDirUnixAncestorRetargetCleansHeldParentOnly(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := newPrivateConversionDir(parent, "codrax-private-parent-*")
	if err != nil {
		t.Fatal(err)
	}
	originalLeaf := filepath.Base(dir.Path())
	movedParent := filepath.Join(root, "parent-original")
	if err := os.Rename(parent, movedParent); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(parent, originalLeaf)
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	replacementSentinel := filepath.Join(replacement, "sentinel")
	if err := os.WriteFile(replacementSentinel, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dir.Validate(); !errors.Is(err, errPrivateConversionDirIdentityChanged) {
		t.Fatalf("ancestor retarget validation error=%v, want identity sentinel", err)
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatalf("held parent cleanup failed after ancestor retarget: %v", err)
	}
	if _, err := os.Stat(filepath.Join(movedParent, originalLeaf)); !os.IsNotExist(err) {
		t.Fatalf("held-parent original directory survived cleanup: %v", err)
	}
	if body, err := os.ReadFile(replacementSentinel); err != nil || string(body) != "replacement" {
		t.Fatalf("ancestor retarget replacement changed: body=%q err=%v", body, err)
	}
}
