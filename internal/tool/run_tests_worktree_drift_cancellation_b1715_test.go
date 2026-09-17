package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1715DriftCanceledMultiOwnerAuditDoesNotReverify(t *testing.T) {
	for _, mode := range []string{"before_audit", "during_first_reverify", "nil_legacy"} {
		t.Run(mode, func(t *testing.T) {
			root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n", "gomod/go.mod": "module x\n", "gomod/go.sum": "h1\n"})
			baseline := captureVerificationWorktreeSnapshot(context.Background(), root)
			writeVerificationWorktreeFile(t, root, "Cargo.lock", "v2\n")
			writeVerificationWorktreeFile(t, root, "gomod/go.sum", "h2\n")
			writeVerificationWorktreeFile(t, root, "retained-output.txt", "still disclosed\n")
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			in := driftInput(root, nil, executedRunner("rust", "."), executedRunner("go", "gomod"))
			if mode != "nil_legacy" {
				in.executionContext = parent
			}
			if mode == "before_audit" {
				cancel()
			}
			calls := 0
			old := verificationLockedReverifyHook
			verificationLockedReverifyHook = func(req verificationLockedReverifyRequest) verificationLockedReverifyResult {
				calls++
				if mode == "during_first_reverify" {
					cancel()
				}
				return verificationLockedReverifyResult{ExitCode: 0}
			}
			t.Cleanup(func() { verificationLockedReverifyHook = old })
			report := passingVerificationWorktreeReport()
			attachVerificationWorktreeAudit(context.Background(), report, baseline, root, in)
			wantCalls := map[string]int{"before_audit": 0, "during_first_reverify": 1, "nil_legacy": 2}[mode]
			if calls != wantCalls {
				t.Errorf("new execution calls=%d, want %d after caller cancellation boundary", calls, wantCalls)
			}
			audit := report.WorktreeAudit
			if audit == nil || audit.TrackedEffectCount != 2 || audit.UntrackedEffectCount != 1 || len(audit.LockedReverify) != 2 {
				t.Fatalf("independent cleanup audit must retain all effects/owners: %+v", audit)
			}
			for _, record := range audit.LockedReverify {
				wantOutcome := types.VerificationLockedReverifyUnavailable
				if mode == "nil_legacy" {
					wantOutcome = types.VerificationLockedReverifyPassed
				}
				if record.Outcome != wantOutcome {
					t.Errorf("owner %s/%s outcome=%s, want %s", record.Runner, record.WorkingDir, record.Outcome, wantOutcome)
				}
			}
			if report.Passed != (mode == "nil_legacy") {
				t.Errorf("canceled fixed point cannot sign passing audit: %+v", report)
			}
			for rel, want := range map[string]string{"Cargo.lock": "v2\n", "gomod/go.sum": "h2\n", "retained-output.txt": "still disclosed\n"} {
				got, err := os.ReadFile(filepath.Join(root, rel))
				if err != nil || string(got) != want {
					t.Fatalf("audit changed retained bytes %s: %q %v", rel, got, err)
				}
			}
		})
	}
}

func TestB1715DriftFormatterCancellationDoesNotCallOrAcceptHook(t *testing.T) {
	for _, mode := range []string{"before_formatter", "during_formatter", "nil_legacy"} {
		t.Run(mode, func(t *testing.T) {
			root := driftRepoWithFiles(t, map[string]string{"main.go": "package main\nfunc main() {}\n"})
			current := "package main\n\nfunc main() {}\n"
			writeVerificationWorktreeFile(t, root, "main.go", current)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			in := driftInput(root, nil, executedRunner("go", "."))
			if mode != "nil_legacy" {
				in.executionContext = parent
			}
			if mode == "before_formatter" {
				cancel()
			}
			calls := 0
			old := verificationFormatterHook
			verificationFormatterHook = func(argv []string, input []byte) ([]byte, bool) {
				calls++
				if mode == "during_formatter" {
					cancel()
				}
				return []byte(current), true
			}
			t.Cleanup(func() { verificationFormatterHook = old })
			ok := verificationDriftFormatterFixedPoint(in, root, "main.go", verificationDriftRosterEntry{runner: "go", dirRel: "."}, []string{"gofmt"})
			wantCalls := 1
			if mode == "before_formatter" {
				wantCalls = 0
			}
			if calls != wantCalls || ok != (mode == "nil_legacy") {
				t.Fatalf("formatter calls=%d fixed_point=%v, want calls=%d fixed_point=%v", calls, ok, wantCalls, mode == "nil_legacy")
			}
		})
	}
}

func TestB1715DriftLockedExecutionInheritsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled shell fixture uses POSIX shell")
	}
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := driftInput(root, nil, executedRunner("rust", "."))
	in.executionContext = parent
	started := filepath.Join(root, "started")
	finished := filepath.Join(root, "finished")
	ready := b1715DriftCancelAfterFile(started, cancel)
	got := verificationDriftExecuteLocked(in)(verificationLockedReverifyRequest{
		Runner: "rust", WorkingDir: ".", AuditRoot: root, Timeout: 8 * time.Second,
		Command: fmt.Sprintf("printf ready > %q; sleep 5; printf finished > %q", started, finished),
	})
	returnedAt := time.Now()
	handshake := <-ready
	if handshake.Err != nil {
		t.Fatal(handshake.Err)
	}
	if handshake.CanceledAt.IsZero() || returnedAt.Before(handshake.CanceledAt) {
		t.Fatalf("locked execution returned before confirmed caller cancellation: canceled=%v returned=%v", handshake.CanceledAt, returnedAt)
	}
	elapsed := returnedAt.Sub(handshake.CanceledAt)
	if !got.Unavailable || elapsed >= 4*time.Second {
		t.Errorf("locked command ignored caller cancellation: result=%+v cancel_to_return=%v", got, elapsed)
	}
	if _, err := os.Stat(finished); !os.IsNotExist(err) {
		t.Errorf("canceled descendant reached post-wait side effect: %v", err)
	}
}

func TestB1715DriftFormatterExecutionInheritsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled shell fixture uses POSIX shell")
	}
	root := driftRepoWithFiles(t, map[string]string{"main.go": "before\n"})
	writeVerificationWorktreeFile(t, root, "main.go", "after\n")
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := driftInput(root, nil, executedRunner("go", "."))
	in.executionContext = parent
	started := filepath.Join(root, "started")
	finished := filepath.Join(root, "finished")
	ready := b1715DriftCancelAfterFile(started, cancel)
	ok := verificationDriftFormatterFixedPoint(in, root, "main.go", verificationDriftRosterEntry{runner: "go", dirRel: "."}, []string{
		"/bin/sh", "-c", fmt.Sprintf("printf ready > %q; sleep 5; printf finished > %q; printf 'after\\n'", started, finished),
	})
	returnedAt := time.Now()
	handshake := <-ready
	if handshake.Err != nil {
		t.Fatal(handshake.Err)
	}
	if handshake.CanceledAt.IsZero() || returnedAt.Before(handshake.CanceledAt) {
		t.Fatalf("formatter returned before confirmed caller cancellation: canceled=%v returned=%v", handshake.CanceledAt, returnedAt)
	}
	elapsed := returnedAt.Sub(handshake.CanceledAt)
	if ok || elapsed >= 4*time.Second {
		t.Errorf("formatter ignored caller cancellation: fixed_point=%v cancel_to_return=%v", ok, elapsed)
	}
	if _, err := os.Stat(finished); !os.IsNotExist(err) {
		t.Errorf("canceled formatter descendant reached post-wait side effect: %v", err)
	}
}

func TestB1715DriftNilContextRealLockedExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled shell fixture uses POSIX shell")
	}
	root := driftRepoWithFiles(t, map[string]string{"Cargo.lock": "v1\n"})
	got := verificationDriftExecuteLocked(driftInput(root, nil))(verificationLockedReverifyRequest{
		Runner: "rust", WorkingDir: ".", AuditRoot: root, Timeout: time.Second, Command: "true",
	})
	if got.Unavailable || got.ExitCode != 0 || len(got.DriftedPaths) != 0 {
		t.Fatalf("nil execution context must preserve normal legacy execution: %+v", got)
	}
}

type b1715CancellationReceipt struct {
	CanceledAt time.Time
	Err        error
}

// Setup may need scheduler time before the child is ready. Measure the
// cancellation response separately, retaining time.Time's monotonic clock.
func b1715DriftCancelAfterFile(path string, cancel context.CancelFunc) <-chan b1715CancellationReceipt {
	done := make(chan b1715CancellationReceipt, 1)
	go func() {
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			if _, err := os.Stat(path); err == nil {
				canceledAt := time.Now()
				cancel()
				done <- b1715CancellationReceipt{CanceledAt: canceledAt}
				return
			}
			select {
			case <-deadline.C:
				canceledAt := time.Now()
				cancel()
				done <- b1715CancellationReceipt{CanceledAt: canceledAt, Err: fmt.Errorf("execution never reached start marker %s", path)}
				return
			case <-tick.C:
			}
		}
	}()
	return done
}
