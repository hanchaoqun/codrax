package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A real Go TestMain can emit green test events before its process completes.
// Cancel only after m.Run and an explicit readiness handshake. The tool must
// retain those bytes without interpreting the incomplete command as a pass.
func TestB1715ProjectSuiteCancellationAfterGreenOutputPublic(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("real Go toolchain unavailable")
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOPROXY", "off")
	for _, tc := range []struct {
		name     string
		cancel   bool
		deadline bool
		queued   bool
	}{
		{name: "canceled_single_suite", cancel: true},
		{name: "canceled_with_queued_candidate", cancel: true, queued: true},
		{name: "deadline_with_queued_candidate", deadline: true, queued: true},
		{name: "uncanceled_completes_both_candidates", queued: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			b1715WriteProjectSuiteFile(t, root, "a-ready/go.mod", "module example.com/b1715/ready\n\ngo 1.22\n")
			b1715WriteProjectSuiteFile(t, root, "a-ready/ready_test.go", fmt.Sprintf(`package ready
import (
    "fmt"
    "net"
    "os"
    "strings"
    "testing"
)
func TestGreenBeforeCancellation(t *testing.T) { t.Log("B1715 assertion completed") }
func TestMain(m *testing.M) {
    code := m.Run()
    if code != 0 { os.Exit(code) }
    fmt.Println("B1715 m.Run completed successfully")
    // Exceed ordinary pipe buffers, so the outer go-test JSON producer must
    // already have consumed the earlier real PASS event before readiness.
    fmt.Println(strings.Repeat("B1715 retained output\n", %d))
    connection, err := net.Dial("tcp", %q)
    if err != nil { panic(err) }
    defer connection.Close()
    if _, err := connection.Write([]byte("tests-passed\n")); err != nil { panic(err) }
    var release [1]byte
    if _, err := connection.Read(release[:]); err != nil { panic(err) }
    if release[0] != 'x' { panic("unexpected release") }
    os.Exit(code)
}
`, max(8192, MaxInlineBytes/20+1), listener.Addr().String()))
			if tc.queued {
				b1715WriteProjectSuiteFile(t, root, "z-later/go.mod", "module example.com/b1715/later\n\ngo 1.22\n")
				b1715WriteProjectSuiteFile(t, root, "z-later/later_test.go", `package later
import ("os"; "testing")
func TestLaterCandidate(t *testing.T) {
    if err := os.WriteFile("later-candidate-started", []byte("executed"), 0600); err != nil { t.Fatal(err) }
}
`)
			}
			// Prove this fixture really queues the later candidate by ordinary
			// filesystem discovery, rather than fabricating a candidate receipt.
			plans := defaultRunnerPlansFromTestSurface(root, BuildTestSurface(root, root), "")
			wantPlans := 1
			if tc.queued {
				wantPlans = 2
			}
			if len(plans) != wantPlans || runnerPlanRel(root, plans[0]) != "a-ready" || (tc.queued && runnerPlanRel(root, plans[1]) != "z-later") {
				t.Fatalf("fixture did not discover the ordered native Go candidates: %+v", plans)
			}

			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			var deadline *b1715ControlledProjectDeadline
			if tc.deadline {
				deadline = &b1715ControlledProjectDeadline{Context: context.Background(), done: make(chan struct{}), at: time.Now().Add(time.Hour)}
				parent = deadline
				defer deadline.expire()
			}
			mu := types.NewMutableState("B1715 project suite cancellation")
			mu.SetChangePlan(&types.ChangePlan{ID: "b1715-project-" + tc.name, Status: types.PlanStatusApplied})
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(), Ctx: parent}
			type execution struct {
				result types.ToolResult
				err    error
			}
			done := make(chan execution, 1)
			go func() {
				result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"timeout_seconds":45}`))
				done <- execution{result, err}
			}()
			if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(40 * time.Second)); err != nil {
				t.Fatal(err)
			}
			connection, err := listener.Accept()
			if err != nil {
				select {
				case got := <-done:
					t.Fatalf("real Go suite stopped before readiness: result=%+v err=%v", got.result, got.err)
				default:
					t.Fatalf("real Go suite did not reach readiness: %v", err)
				}
			}
			defer connection.Close()
			if err := connection.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatal(err)
			}
			ready, err := bufio.NewReader(connection).ReadString('\n')
			if err != nil || ready != "tests-passed\n" {
				t.Fatalf("Go suite readiness = %q, %v", ready, err)
			}
			if tc.cancel {
				cancel()
			} else if tc.deadline {
				deadline.expire()
			} else if _, err := connection.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
			var got execution
			select {
			case got = <-done:
			case <-time.After(20 * time.Second):
				t.Fatal("public project-suite execution did not terminate after cancel/release")
			}
			if got.err != nil {
				t.Fatalf("public Execute: %v", got.err)
			}
			report := mu.ChangeReport()
			if report == nil {
				t.Fatal("public project execution did not install a report")
			}
			if tc.cancel || tc.deadline {
				wantStatus, wantKind := types.VerificationStatusUnavailable, types.FailureKindVerificationIncomplete
				if tc.deadline {
					wantStatus, wantKind = types.VerificationStatusFailed, types.FailureKindTimeout
				}
				if got.result.Success || report.Passed || report.NormalizeVerificationStatus() != wantStatus || report.FailureKind != wantKind {
					t.Errorf("green output from canceled project command became a terminal verdict: success=%t passed=%t status=%s kind=%s", got.result.Success, report.Passed, report.NormalizeVerificationStatus(), report.FailureKind)
				}
				for _, command := range report.ExecutedCommands {
					if command.WorkingDir == "z-later" && (command.Outcome == types.ExecutedCommandOutcomeExecuted || command.Outcome == types.ExecutedCommandOutcomeParserError || command.ExitCode != 0) {
						t.Errorf("canceled run dispatched the later candidate: %+v", command)
					}
				}
				if _, err := os.Stat(filepath.Join(root, "z-later", "later-candidate-started")); !os.IsNotExist(err) {
					t.Errorf("later candidate executed after cancellation: %v", err)
				}
			} else {
				if !got.result.Success || !report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
					t.Errorf("uncanceled native suites failed: result=%+v report=%+v", got.result, report)
				}
				if _, err := os.Stat(filepath.Join(root, "z-later", "later-candidate-started")); err != nil {
					t.Errorf("uncanceled later candidate did not execute: %v", err)
				}
			}
			ref := report.FailureSummaryBlobRef
			if ref == "" {
				ref = got.result.RawRef
			}
			output, err := os.ReadFile(ref)
			if err != nil {
				t.Fatalf("actual project output was not retained: ref=%q err=%v", ref, err)
			}
			for _, marker := range []string{"B1715 m.Run completed successfully", `"Action":"pass"`, "TestGreenBeforeCancellation", "B1715 retained output"} {
				if !strings.Contains(string(output), marker) {
					t.Errorf("retained project output lost observed marker %q", marker)
				}
			}
		})
	}
}

// This context's test-owned clock advances only after the real suite's ready
// handshake. No short wall-clock timer races compilation or process startup.
type b1715ControlledProjectDeadline struct {
	context.Context
	mu      sync.Mutex
	done    chan struct{}
	at      time.Time
	expired bool
}

func (c *b1715ControlledProjectDeadline) Deadline() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at, true
}

func (c *b1715ControlledProjectDeadline) Done() <-chan struct{} { return c.done }

func (c *b1715ControlledProjectDeadline) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expired {
		return context.DeadlineExceeded
	}
	return nil
}

func (c *b1715ControlledProjectDeadline) expire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.expired {
		c.expired, c.at = true, time.Now()
		close(c.done)
	}
}

func b1715WriteProjectSuiteFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
