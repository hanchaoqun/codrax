package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	externalToolSupervisorTestMode      = "CODRAX_EXTERNAL_TOOL_SUPERVISOR_TEST_MODE"
	externalToolSupervisorTestReady     = "CODRAX_EXTERNAL_TOOL_SUPERVISOR_TEST_READY"
	externalToolSupervisorTestHoldReady = "CODRAX_EXTERNAL_TOOL_SUPERVISOR_TEST_HOLD_READY"
)

func TestExternalToolCommandSupervisorProcessTree(t *testing.T) {
	switch os.Getenv(externalToolSupervisorTestMode) {
	case "root_cancel", "root_exit", "root_fail", "root_cancel_lease":
		runExternalToolSupervisorRootFixture()
		return
	case "grandchild", "grandchild_hold_input":
		externalToolSupervisorTestIgnoreGracefulSignal()
		if os.Getenv(externalToolSupervisorTestMode) == "grandchild_hold_input" {
			inputPath := os.Args[len(os.Args)-1]
			file, err := os.Open(inputPath)
			if err != nil {
				panic(err)
			}
			defer file.Close()
			if err := os.WriteFile(os.Getenv(externalToolSupervisorTestHoldReady), []byte("held\n"), 0o600); err != nil {
				panic(err)
			}
		}
		for {
			time.Sleep(time.Second)
		}
	}

	for _, test := range []struct {
		name   string
		mode   string
		cancel bool
		fail   bool
	}{
		{name: "cancel_kills_child_and_grandchild", mode: "root_cancel", cancel: true},
		{name: "normal_root_exit_cleans_residual_grandchild", mode: "root_exit"},
		{name: "nonzero_root_exit_cleans_residual_grandchild", mode: "root_fail", fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ready := t.TempDir() + string(os.PathSeparator) + "processes.ready"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			command, err := newExternalToolCommand(ctx, os.Args[0], "-test.run=^TestExternalToolCommandSupervisorProcessTree$")
			if err != nil {
				t.Fatal(err)
			}
			command.setEnvironment(append(os.Environ(),
				externalToolSupervisorTestMode+"="+test.mode,
				externalToolSupervisorTestReady+"="+ready,
			))
			command.setOutput(io.Discard, io.Discard)
			done := make(chan error, 1)
			go func() {
				runErr, _ := runExternalToolCommandUntilExit(command, nil)
				done <- runErr
			}()
			rootPID, grandchildPID := waitForExternalToolSupervisorFixture(t, ready)
			if test.cancel {
				cancel()
			}
			select {
			case runErr := <-done:
				if test.cancel {
					if !errors.Is(runErr, context.Canceled) {
						t.Fatalf("canceled process tree lost context identity: %T %v", runErr, runErr)
					}
				} else if test.fail {
					var exitErr *exec.ExitError
					if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 23 {
						t.Fatalf("nonzero root exit identity drifted: %T %v", runErr, runErr)
					}
				} else if runErr != nil {
					t.Fatalf("successful root exit became a child failure: %T %v", runErr, runErr)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("supervisor did not finish after root terminal state")
			}
			waitForExternalToolSupervisorProcessExit(t, rootPID)
			waitForExternalToolSupervisorProcessExit(t, grandchildPID)
		})
	}
	t.Run("cancellation_releases_private_snapshot_and_staging", testExternalToolSupervisorLeaseCleanup)
}

func runExternalToolSupervisorRootFixture() {
	externalToolSupervisorTestIgnoreGracefulSignal()
	grandchildArgs := []string{"-test.run=^TestExternalToolCommandSupervisorProcessTree$"}
	grandchildMode := "grandchild"
	if os.Getenv(externalToolSupervisorTestMode) == "root_cancel_lease" {
		grandchildMode = "grandchild_hold_input"
		grandchildArgs = append(grandchildArgs, os.Args[len(os.Args)-1])
	}
	grandchild := exec.Command(os.Args[0], grandchildArgs...)
	grandchild.Env = append(os.Environ(), externalToolSupervisorTestMode+"="+grandchildMode)
	// Retain the supervisor-owned copy pipes deliberately. A direct-child-only
	// implementation hangs after the root exits and leaks this grandchild.
	grandchild.Stdout = os.Stdout
	grandchild.Stderr = os.Stderr
	if err := grandchild.Start(); err != nil {
		panic(err)
	}
	if grandchildMode == "grandchild_hold_input" {
		waitForExternalToolSupervisorChildPath(os.Getenv(externalToolSupervisorTestHoldReady))
	}
	ready := os.Getenv(externalToolSupervisorTestReady)
	body := fmt.Sprintf("%d %d\n", os.Getpid(), grandchild.Process.Pid)
	if err := os.WriteFile(ready, []byte(body), 0o600); err != nil {
		panic(err)
	}
	if os.Getenv(externalToolSupervisorTestMode) == "root_exit" {
		return
	}
	if os.Getenv(externalToolSupervisorTestMode) == "root_fail" {
		os.Exit(23)
	}
	for {
		time.Sleep(time.Second)
	}
}

func testExternalToolSupervisorLeaseCleanup(t *testing.T) {
	parent := t.TempDir()
	inputPath := parent + string(os.PathSeparator) + "input.trace"
	if err := os.WriteFile(inputPath, []byte("sealed external tool input\n"), 0o600); err != nil {
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
	staging, err := newPrivateConversionDir(parent, ".external-tool-supervisor-*")
	if err != nil {
		t.Fatal(err)
	}
	stagingPath := staging.Path()
	lease, err := newExternalToolInputLease(context.Background(), authority, staging, "input.snapshot", externalToolInputSnapshotOnly)
	if err != nil {
		staging.FinalizeCleanup()
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command, err := lease.Command(ctx, os.Args[0], []string{"-test.run=^TestExternalToolCommandSupervisorProcessTree$"}, nil)
	if err != nil {
		lease.Close()
		staging.FinalizeCleanup()
		t.Fatal(err)
	}
	args := command.arguments()
	privateInput := args[len(args)-1]
	ready := parent + string(os.PathSeparator) + "lease-processes.ready"
	holdReady := parent + string(os.PathSeparator) + "lease-held.ready"
	command.setEnvironment(append(os.Environ(),
		externalToolSupervisorTestMode+"=root_cancel_lease",
		externalToolSupervisorTestReady+"="+ready,
		externalToolSupervisorTestHoldReady+"="+holdReady,
	))
	command.setOutput(io.Discard, io.Discard)
	done := make(chan error, 1)
	go func() {
		runErr, _ := runExternalToolCommandUntilExit(command, nil)
		done <- runErr
	}()
	rootPID, grandchildPID := waitForExternalToolSupervisorFixture(t, ready)
	cancel()
	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("supervisor did not finish canceled lease command")
	}
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("lease cancellation lost context identity: %v", runErr)
	}
	boundaryErr := finishExternalToolCommand(ctx, lease, staging, runErr)
	if !errors.Is(boundaryErr, context.Canceled) {
		t.Fatalf("lease boundary lost cancellation: %v", boundaryErr)
	}
	waitForExternalToolSupervisorProcessExit(t, rootPID)
	waitForExternalToolSupervisorProcessExit(t, grandchildPID)
	if err := staging.FinalizeCleanup(); err != nil {
		t.Fatalf("supervised process tree retained private staging authority: %v", err)
	}
	for _, path := range []string{privateInput, stagingPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("supervised cancellation leaked private path %q: %v", path, err)
		}
	}
}

func waitForExternalToolSupervisorChildPath(path string) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	panic("timed out waiting for grandchild input hold")
}

func waitForExternalToolSupervisorFixture(t *testing.T, ready string) (int, int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(ready)
		if err == nil {
			fields := strings.Fields(string(body))
			if len(fields) != 2 {
				t.Fatalf("malformed process-tree fixture receipt %q", body)
			}
			root, rootErr := strconv.Atoi(fields[0])
			grandchild, grandchildErr := strconv.Atoi(fields[1])
			if rootErr != nil || grandchildErr != nil || root <= 0 || grandchild <= 0 {
				t.Fatalf("invalid process-tree fixture receipt %q: root=%v grandchild=%v", body, rootErr, grandchildErr)
			}
			return root, grandchild
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read process-tree fixture receipt: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for process-tree fixture")
	return 0, 0
}

func waitForExternalToolSupervisorProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !externalToolSupervisorTestProcessAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("supervised process %d remained alive", pid)
}
