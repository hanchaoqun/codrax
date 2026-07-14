//go:build unix

package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

const externalToolSnapshotFileLimitFixture = "CODRAX_EXTERNAL_TOOL_SNAPSHOT_FILE_LIMIT_FIXTURE"

func TestReleaseExternalToolInputLeaseFileLimitCleanup(t *testing.T) {
	if os.Getenv(externalToolSnapshotFileLimitFixture) == "1" {
		runExternalToolSnapshotFileLimitFixture()
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestReleaseExternalToolInputLeaseFileLimitCleanup$")
	command.Env = append(os.Environ(), externalToolSnapshotFileLimitFixture+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("file-limit fixture failed: %v\n%s", err, output)
	}
}

func runExternalToolSnapshotFileLimitFixture() {
	parent, err := os.MkdirTemp("", "codrax-external-tool-file-limit-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(parent)
	inputPath := filepath.Join(parent, "input.trace")
	if err := os.WriteFile(inputPath, make([]byte, 128*1024), 0o600); err != nil {
		panic(err)
	}
	authority, err := openConversionInputAuthority(inputPath)
	if err != nil {
		panic(err)
	}
	defer authority.Close()
	staging, err := newPrivateConversionDir(parent, ".external-tool-file-limit-*")
	if err != nil {
		panic(err)
	}

	// Keep SIGXFSZ from terminating the helper so Write returns EFBIG and the
	// normal lease error/cleanup path is exercised deterministically.
	signal.Ignore(syscall.SIGXFSZ)
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
		panic(err)
	}
	limit.Cur = 4096
	if limit.Max < limit.Cur {
		limit.Cur = limit.Max
	}
	if limit.Cur == 0 {
		panic("RLIMIT_FSIZE cannot admit the fixture prefix")
	}
	if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
		panic(err)
	}
	lease, leaseErr := newExternalToolInputLease(
		context.Background(), authority, staging, "input.snapshot", externalToolInputSnapshotOnly,
	)
	if lease != nil || leaseErr == nil {
		panic(fmt.Sprintf("file limit returned lease=%v error=%v", lease, leaseErr))
	}
	if cleanupErr := staging.FinalizeCleanup(); cleanupErr != nil {
		panic(fmt.Sprintf("file-limit cleanup retained open child: %v (lease error: %v)", cleanupErr, leaseErr))
	}
}
