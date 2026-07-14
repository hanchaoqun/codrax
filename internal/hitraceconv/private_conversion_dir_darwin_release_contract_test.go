//go:build darwin

package hitraceconv

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReleasePrivateConversionDirDarwinIsPrivateAtBirth(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "birth-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	setPrivateConversionDirDarwinACL(t, parent,
		"everyone allow list,search,add_file,add_subdirectory,file_inherit,directory_inherit")
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parentFD)
	const leaf = "birth-private"
	if err := createPrivateConversionDirUnixPlatform(parentFD, parent, leaf); err != nil {
		t.Fatal(err)
	}
	defer unix.Unlinkat(parentFD, leaf, unix.AT_REMOVEDIR)

	path := filepath.Join(parent, leaf)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("Darwin private directory birth mode=%s, want directory 0700", info.Mode())
	}
	childFD, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePrivateConversionDirUnixSecurityPlatform(childFD); err != nil {
		_ = unix.Close(childFD)
		t.Fatalf("Darwin private directory was not exact 0700/no-ACL at birth: %v", err)
	}
	if err := unix.Close(childFD); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("/bin/ls", "-lde", path).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect birth ACL: %v: %s", err, output)
	}
	if bytes.Contains(output, []byte("group:everyone")) || bytes.Contains(output, []byte(" inherited ")) {
		t.Fatalf("Darwin private directory inherited an ACL at birth:\n%s", output)
	}
	if err := createPrivateConversionDirUnixPlatform(parentFD, parent, leaf); !errors.Is(err, unix.EEXIST) {
		t.Fatalf("duplicate Darwin mkdirx result=%v, want EEXIST", err)
	}
}

func TestReleasePrivateConversionDirDarwinClearsInheritedACLAndRejectsDrift(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "acl-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	setPrivateConversionDirDarwinACL(t, parent,
		"everyone allow list,search,add_file,add_subdirectory,file_inherit,directory_inherit")

	// Pin the fixture: a plain mkdir below this parent really does inherit an
	// effective extended ACL even though its POSIX mode is 0700.
	control := filepath.Join(parent, "control")
	if err := os.Mkdir(control, 0o700); err != nil {
		t.Fatal(err)
	}
	controlFD, err := unix.Open(control, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePrivateConversionDirUnixSecurityPlatform(controlFD); err == nil {
		_ = unix.Close(controlFD)
		t.Fatal("Darwin inherited-ACL fixture has no detectable ACL entry")
	}
	if err := unix.Close(controlFD); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(control); err != nil {
		t.Fatal(err)
	}

	dir, err := newPrivateConversionDir(parent, "codrax-private-darwin-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := dir.Validate(); err != nil {
		t.Fatalf("new private directory retained its inherited Darwin ACL: %v", err)
	}

	setPrivateConversionDirDarwinACL(t, dir.Path(), "everyone allow list,search")
	if err := dir.Validate(); !errors.Is(err, errPrivateConversionDirSecurityInvalid) {
		t.Fatalf("Darwin ACL drift error=%v, want security sentinel", err)
	}
	if err := dir.FinalizeCleanup(); err != nil {
		t.Fatalf("held-FD cleanup failed after Darwin ACL drift: %v", err)
	}
	if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
		t.Fatalf("Darwin ACL-drift directory survived cleanup: %v", err)
	}
}

func TestReleasePrivateConversionDirDarwinACLPointerFailuresFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed-fd")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	if err := validatePrivateConversionDirUnixSecurityPlatform(fd); err == nil || !errors.Is(err, unix.EBADF) {
		t.Fatalf("closed-FD Darwin ACL validation did not fail closed with EBADF: %v", err)
	}
	if err := securePrivateConversionDirUnixPlatform(fd); err == nil || !errors.Is(err, unix.EBADF) {
		t.Fatalf("closed-FD Darwin ACL clearing did not fail closed with EBADF: %v", err)
	}
}

func setPrivateConversionDirDarwinACL(t *testing.T, path, entry string) {
	t.Helper()
	output, err := exec.Command("/bin/chmod", "+a", entry, path).CombinedOutput()
	if err != nil {
		t.Fatalf("install Darwin ACL fixture: %v: %s", err, output)
	}
}
