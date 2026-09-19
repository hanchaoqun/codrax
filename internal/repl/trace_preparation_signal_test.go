package repl

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

const hmc17SignalHelperEnv = "CODRAX_HMC17_SIGNAL_HELPER"
const hmc17SignalDirEnv = "CODRAX_HMC17_SIGNAL_DIR"

// The helper holds a real, unpublished RMQ preparation at a deterministic
// boundary. This tests the actual process signal handler (including the
// non-canceller runner lane), not a direct call to its cancellation callback.
// A pipe handshake holds rollback open; no sleeps race against conversion.
func TestTracePreparationProcessSignalsWaitForRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process SIGINT/SIGTERM acceptance is Unix-only")
	}
	if mode := os.Getenv(hmc17SignalHelperEnv); mode != "" {
		hmc17RunSignalHelper(t, mode, os.Getenv(hmc17SignalDirEnv))
		return
	}
	for _, test := range []struct {
		name string
		sig  os.Signal
		exit int
	}{
		{"interrupt", syscall.SIGINT, 0},
		{"terminate", syscall.SIGTERM, 143},
		{"double-interrupt", syscall.SIGINT, 130},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			original := hmc17BinaryTraceTailFixture()
			path := filepath.Join(dir, "capture.sys")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			// This is only a hung-helper safety bound, never a test assertion
			// about how long cancellation or conversion should take.
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestTracePreparationProcessSignalsWaitForRollback$", "-test.count=1", "-test.timeout=18s")
			command.Env = append(os.Environ(), hmc17SignalHelperEnv+"="+test.name, hmc17SignalDirEnv+"="+dir,
				"CODRAX_TRACE_STREAMER="+filepath.Join(dir, "missing-trace-streamer"))
			stdin, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			stdout, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			command.Stderr = &stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = command.Process.Kill() })
			scanner := bufio.NewScanner(stdout)
			var transcript strings.Builder
			await := func(want string) {
				t.Helper()
				for scanner.Scan() {
					line := scanner.Text()
					fmt.Fprintln(&transcript, line)
					if line == "HMC17_SIGNAL "+want {
						return
					}
				}
				waitErr := command.Wait()
				t.Fatalf("helper stopped before %s: read=%v process=%v\nstdout:\n%s\nstderr:\n%s", want, scanner.Err(), waitErr, transcript.String(), stderr.String())
			}
			await("READY")
			if err := command.Process.Signal(test.sig); err != nil {
				t.Fatal(err)
			}
			await("CANCELED_ROLLBACK_HELD")
			if test.name == "double-interrupt" {
				if err := command.Process.Signal(syscall.SIGINT); err != nil {
					t.Fatal(err)
				}
			}
			if test.exit != 0 {
				// Sending a signal is not an acknowledgement that its handler
				// ran. Wait for the actual shutdown request before rollback.
				await("SHUTDOWN_ADMITTED")
			}
			owned, err := filepath.Glob(filepath.Join(dir, "runtime", "trace-input-*"))
			if err != nil || len(owned) != 1 {
				t.Fatalf("signal did not leave unpublished material held for rollback: owned=%v err=%v", owned, err)
			}
			if _, err := fmt.Fprintln(stdin, "release-rollback"); err != nil {
				t.Fatalf("signal exited while rollback was still held: %v", err)
			}
			await("ROLLBACK_COMPLETE")
			if test.exit == 0 {
				await("DONE_WITHOUT_EXIT")
			}
			for scanner.Scan() {
				fmt.Fprintln(&transcript, scanner.Text())
			}
			err = command.Wait()
			if strings.Contains(transcript.String(), "UNEXPECTED_FINISH_RETURN") {
				t.Fatalf("shutdown released the preparation owner before process exit: %v\n%s\n%s", err, transcript.String(), stderr.String())
			}
			if test.exit == 0 {
				if err != nil {
					t.Fatalf("single interrupt exited the process: %v\n%s\n%s", err, transcript.String(), stderr.String())
				}
			} else {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != test.exit {
					t.Fatalf("shutdown did not use its expected exit code %d: %v\n%s\n%s", test.exit, err, transcript.String(), stderr.String())
				}
			}
			if scanner.Err() != nil || ctx.Err() != nil {
				t.Fatalf("helper transport failed or timed out: read=%v context=%v", scanner.Err(), ctx.Err())
			}
			if owned, err := filepath.Glob(filepath.Join(dir, "runtime", "trace-input-*")); err != nil || len(owned) != 0 {
				t.Fatalf("signal escaped before owned rollback completed: owned=%v err=%v", owned, err)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("signal cleanup changed the original RMQ capture: %v", err)
			}
		})
	}
}

func hmc17RunSignalHelper(t *testing.T, mode, dir string) {
	t.Helper()
	if mode != "interrupt" && mode != "terminate" && mode != "double-interrupt" || dir == "" {
		t.Fatalf("invalid isolated signal helper mode=%q dir=%q", mode, dir)
	}
	op := newTracePreparationOperation()
	runner := &materialAwareRunner{} // Deliberately does not implement Cancel.
	r := New(Config{Runner: runner, In: strings.NewReader(""), Out: os.Stdout, Language: "en"})
	r.tracePreparation.Store(op)
	worktree.InstallSignalHandler()
	r.installCancelSignalHandler()
	pending, err := traceinput.Begin(op.ctx, traceinput.Options{
		InputPath: filepath.Join(dir, "capture.sys"), RuntimeAnchor: filepath.Join(dir, "runtime"), PreviewBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Discard()
	fmt.Fprintln(os.Stdout, "HMC17_SIGNAL READY")
	<-op.ctx.Done()
	if !errors.Is(op.ctx.Err(), context.Canceled) {
		t.Fatalf("signal lost typed cancellation: %v", op.ctx.Err())
	}
	select {
	case <-op.done:
		t.Fatal("operation done closed before rollback began")
	default:
	}
	fmt.Fprintln(os.Stdout, "HMC17_SIGNAL CANCELED_ROLLBACK_HELD")
	if mode != "interrupt" {
		// Observe only; the real SIGTERM/second SIGINT handler must request
		// shutdown. Yielding while inspecting the precise locked state avoids
		// sleeping to guess whether the second signal has been processed.
		for {
			op.mu.Lock()
			shutdown := op.shutdownRequested
			op.mu.Unlock()
			if shutdown {
				break
			}
			runtime.Gosched()
		}
		fmt.Fprintln(os.Stdout, "HMC17_SIGNAL SHUTDOWN_ADMITTED")
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() || scanner.Text() != "release-rollback" {
		t.Fatalf("rollback hold was not released by the parent: %q err=%v", scanner.Text(), scanner.Err())
	}
	if err := pending.Discard(); err != nil {
		t.Fatalf("canceled operation rollback: %v", err)
	}
	if owned, err := filepath.Glob(filepath.Join(dir, "runtime", "trace-input-*")); err != nil || len(owned) != 0 {
		t.Fatalf("discard returned while owned outputs survived: %v err=%v", owned, err)
	}
	if runner.material != nil || runner.curTrace != "" || len(runner.seenMaterials) != 0 {
		t.Fatal("canceled local operation published or dispatched material")
	}
	select {
	case <-op.done:
		t.Fatal("signal handler closed done instead of waiting for the operation owner")
	default:
	}
	fmt.Fprintln(os.Stdout, "HMC17_SIGNAL ROLLBACK_COMPLETE")
	op.finish()
	if mode != "interrupt" {
		// The owner must be held inside finish until the real signal handler
		// exits. Returning would let Loop consume a queued /exit or new input
		// and race shutdown, even though its rollback had already completed.
		fmt.Fprintln(os.Stdout, "HMC17_SIGNAL UNEXPECTED_FINISH_RETURN")
		os.Exit(97)
	}
	<-op.done
	r.tracePreparation.CompareAndSwap(op, nil)
	fmt.Fprintln(os.Stdout, "HMC17_SIGNAL DONE_WITHOUT_EXIT")
}
