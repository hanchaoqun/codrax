//go:build darwin

package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestB1715DarwinGuardianPreservesNormalCommand(t *testing.T) {
	for _, exitCode := range []int{0, 7} {
		t.Run(strconv.Itoa(exitCode), func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("printf 'exact stdout\\n'; printf 'exact stderr\\n' >&2; exit %d", exitCode))
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			result := SupervisedRun(context.Background(), cmd, SupervisedRunOptions{})
			if result.ExitKind != SupervisedExitNormal || cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != exitCode {
				t.Fatalf("guardian altered target exit: result=%+v state=%v", result, cmd.ProcessState)
			}
			if (result.Err == nil) != (exitCode == 0) || stdout.String() != "exact stdout\n" || stderr.String() != "exact stderr\n" {
				t.Fatalf("guardian altered output/error: stdout=%q stderr=%q result=%+v", stdout.String(), stderr.String(), result)
			}
		})
	}
}

// The leaf's descriptors are redirected before it starts. Thus target Wait
// cannot establish that this already-running descendant has stopped.
func TestB1715DarwinGuardianStopsPublishedClosedStdioLeaf(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	readyPath := filepath.Join(root, "ready")
	finishedPath := filepath.Join(root, "finished")
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	script := shellQuoteWord(binary) + " -test.run='^TestB1715DarwinGuardianClosedStdioLeafHelper$' >/dev/null 2>&1 & wait"
	cmd := exec.CommandContext(parent, "/bin/sh", "-c", script)
	cmd.Env = append(os.Environ(), "CODRAX_B1715_CLOSED_STDIO_LEAF=1", "CODRAX_B1715_CLOSED_STDIO_ROOT="+root)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	done := make(chan SupervisedResult, 1)
	go func() { done <- SupervisedRun(parent, cmd, SupervisedRunOptions{}) }()
	var leafPID int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(readyPath); err == nil {
			if pid, err := strconv.Atoi(string(data)); err == nil && pid > 0 {
				leafPID = pid
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if leafPID == 0 {
		cancel()
		t.Fatal("closed-stdio leaf never became ready")
	}
	canceledAt := time.Now()
	cancel()
	select {
	case result := <-done:
		if result.ExitKind != SupervisedExitNormal || !errors.Is(result.Err, context.Canceled) || !errors.Is(result.Err, errDarwinTreeCleanupUnproven) {
			t.Errorf("cancellation/unknown cleanup identity was lost: %+v", result)
		}
		if time.Since(canceledAt) >= 4*time.Second {
			t.Errorf("published closed-stdio leaf delayed cancellation: %v", time.Since(canceledAt))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("guardian did not return before the leaf's post-wait action")
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err = syscall.Kill(leafPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !errors.Is(err, syscall.ESRCH) {
		t.Errorf("published closed-stdio leaf %d remains: %v", leafPID, err)
	}
	if _, err := os.Stat(finishedPath); !os.IsNotExist(err) {
		t.Errorf("closed-stdio descendant reached its post-wait side effect: %v", err)
	}
}

func TestB1715DarwinGuardianClosedStdioLeafHelper(t *testing.T) {
	if os.Getenv("CODRAX_B1715_CLOSED_STDIO_LEAF") != "1" {
		return
	}
	root := os.Getenv("CODRAX_B1715_CLOSED_STDIO_ROOT")
	if err := os.WriteFile(filepath.Join(root, "ready"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		os.Exit(2)
	}
	time.Sleep(5 * time.Second)
	_ = os.WriteFile(filepath.Join(root, "finished"), []byte("escaped cleanup"), 0o600)
	os.Exit(0)
}

func TestB1715DarwinGuardianReleaseEndsSignalAuthority(t *testing.T) {
	target := exec.Command("/bin/sh", "-c", "exit 0")
	target.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := target.Start(); err != nil {
		t.Fatal(err)
	}
	lease, err := startDarwinProcessGroupLease(target.Process.Pid)
	if err != nil {
		_ = target.Wait()
		t.Fatal(err)
	}
	defer lease.release(time.Now().Add(time.Second))
	if err := target.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := lease.signal(); err != nil {
		t.Fatalf("guardian did not retain the reaped target's group identity: %v", err)
	}
	if err := lease.release(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := lease.signal(); err == nil || !strings.Contains(err.Error(), "lease is unavailable") {
		t.Errorf("released lease allowed another group signal: %v", err)
	}
	if _, err := startDarwinProcessGroupLease(0); err == nil {
		t.Error("invalid PGID acquired guardian authority")
	}
}
