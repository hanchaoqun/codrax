//go:build linux

package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseExternalToolInputLeaseLinuxInheritedFD(t *testing.T) {
	parent := t.TempDir()
	inputPath := filepath.Join(parent, "input.trace")
	want := []byte("linux-inherited-fd\n")
	if err := os.WriteFile(inputPath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(inputPath)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	staging, err := newPrivateConversionDir(parent, ".external-tool-linux-*")
	if err != nil {
		t.Fatal(err)
	}
	defer staging.FinalizeCleanup()
	lease, err := newExternalToolInputLease(
		context.Background(), authority, staging, "input.snapshot", externalToolInputVerifiedLinuxFD,
	)
	if err != nil {
		t.Fatal(err)
	}
	if lease.transport != externalToolInputTransportLinuxFD {
		t.Skip("host does not expose a verified /proc/self/fd authority; snapshot fallback is expected")
	}
	cmd, err := lease.Command(context.Background(), "/bin/cat", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "/proc/self/fd/3" || len(cmd.ExtraFiles) != 1 {
		t.Fatalf("Linux inherited-FD command binding drifted: args=%#v extra=%d", cmd.Args, len(cmd.ExtraFiles))
	}
	var got bytes.Buffer
	cmd.Stdout = &got
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("Linux inherited FD read=%q want=%q", got.Bytes(), want)
	}
	if err := finishExternalToolCommand(context.Background(), lease, staging, nil); err != nil {
		t.Fatal(err)
	}
}
