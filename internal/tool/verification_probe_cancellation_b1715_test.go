package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Drive the public tool with real runtimes. With cancellation lost, the
// current probe passes and its baseline fails, minting differential proof
// even though neither process was authorized to start after cancellation.
func TestB1715PreCanceledProbePublicHasNoExecutionOrProof(t *testing.T) {
	for _, language := range []string{"python", "javascript"} {
		t.Run(language, func(t *testing.T) {
			root, baseline := t.TempDir(), t.TempDir()
			if !VerificationProbeRuntimeAvailable(language, root) {
				t.Skip("real probe runtime unavailable")
			}
			path, current, previous, code := "widget.py", "VALUE = 2\n", "VALUE = 1\n", "import widget\nfrom pathlib import Path\nPath('probe-executed').write_text('executed')\nassert widget.VALUE == 2\n"
			if language == "javascript" {
				path, current, previous = "widget.cjs", "module.exports.VALUE = 2;\n", "module.exports.VALUE = 1;\n"
				code = "require('fs').writeFileSync('probe-executed', 'executed'); require('assert/strict').equal(require('./widget.cjs').VALUE, 2);"
			}
			for dir, source := range map[string]string{root: current, baseline: previous} {
				if err := os.WriteFile(filepath.Join(dir, path), []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			parent, cancel := context.WithCancel(context.Background())
			cancel()
			probe := types.VerificationProbe{
				ID: "canceled", Language: language, Code: code,
				ChangedSymbolRefs: []string{"path:" + path}, ContractRefs: []string{"value"},
				ExpectsBaselineFailure: true,
			}
			ctx := b1715ProbeContext(t, root, baseline, parent, probe)
			plan := ctx.Mutable.ChangePlan()
			plan.TargetPaths, plan.AppliedPaths = []string{path}, []string{path}
			plan.BehaviorContracts = []types.WriteBehaviorContract{{
				ID: "value", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
				Operator: types.WriteBehaviorOpEquals, Expected: "2", Required: true, Source: "write_analyzer",
			}}
			ctx.Mutable.SetChangePlan(plan)
			result, err := (&RunTests{}).Execute(ctx, b1715RunnerParams(language))
			if err != nil {
				t.Fatalf("public Execute: %v", err)
			}
			for _, dir := range []string{root, baseline} {
				if _, err := os.Stat(filepath.Join(dir, "probe-executed")); !os.IsNotExist(err) {
					t.Errorf("pre-canceled probe executed a side effect in %s: %v", dir, err)
				}
			}
			if result.Success {
				t.Error("pre-canceled public probe reported success")
			}
			report := ctx.Mutable.ChangeReport()
			b1715AssertCanceledReport(t, report)
			baselineFound := false
			for _, command := range report.ExecutedCommands {
				if command.Source == verificationProbeBaselineSource {
					baselineFound = true
					if command.Outcome != types.ExecutedCommandOutcomeBaselineUnavailable {
						t.Errorf("cancellation minted a baseline verdict: %+v", command)
					}
				}
				if command.ProbeExecution != nil {
					t.Errorf("pre-canceled command minted an execution receipt: %+v", command)
				}
			}
			if !baselineFound {
				t.Error("requested baseline must retain its unavailable evidence row")
			}
		})
	}
}

// The local socket is a readiness handshake, not a sleep-based estimate of
// child startup. Keep it open until Execute returns so cancellation, rather
// than EOF or a deliberately failing comparator, terminates the child.
func TestB1715RunningProbePublicCancellationIsUnavailable(t *testing.T) {
	for _, language := range []string{"python", "javascript"} {
		t.Run(language, func(t *testing.T) {
			root := t.TempDir()
			if !VerificationProbeRuntimeAvailable(language, root) {
				t.Skip("real probe runtime unavailable")
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			port := listener.Addr().(*net.TCPAddr).Port
			code := fmt.Sprintf("import socket\ns = socket.create_connection(('127.0.0.1', %d))\ns.sendall(b'ready\\n')\nassert s.recv(1) == b'x'\n", port)
			if language == "javascript" {
				code = fmt.Sprintf("const socket = require('net').createConnection({host:'127.0.0.1', port:%d}, () => socket.write('ready\\n')); socket.on('data', () => { throw new Error('unexpected release'); });", port)
			}
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := b1715ProbeContext(t, root, root, parent, types.VerificationProbe{ID: "in-flight", Language: language, Code: code, TimeoutSeconds: 5})
			type execution struct {
				result types.ToolResult
				err    error
			}
			done := make(chan execution, 1)
			go func() {
				result, err := (&RunTests{}).Execute(ctx, b1715RunnerParams(language))
				done <- execution{result, err}
			}()
			if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatal(err)
			}
			conn, err := listener.Accept()
			if err != nil {
				t.Fatalf("probe did not reach readiness handshake: %v", err)
			}
			defer conn.Close()
			if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatal(err)
			}
			ready, err := bufio.NewReader(conn).ReadString('\n')
			if err != nil || ready != "ready\n" {
				t.Fatalf("readiness handshake = %q, %v", ready, err)
			}
			cancel()
			select {
			case got := <-done:
				if got.err != nil {
					t.Fatalf("public Execute: %v", got.err)
				}
				if got.result.Success {
					t.Error("canceled running probe reported success")
				}
				b1715AssertCanceledReport(t, ctx.Mutable.ChangeReport())
			case <-time.After(10 * time.Second):
				t.Fatal("public probe did not return after cancellation or its bounded timeout")
			}
		})
	}
}

func TestB1715ProbeCommandInheritsParentAndPreservesBudget(t *testing.T) {
	root := t.TempDir()
	parent, cancelParent := context.WithCancel(context.Background())
	bus := &types.BusContext{RepoRoot: root, MainRepoRoot: root, Ctx: parent}
	child, _, budget, cancel := newVerificationProbeCommand("unused-probe-runtime", nil, root, "node", bus, types.VerificationProbe{}, "")
	defer cancel()
	if budget != 10*time.Second {
		t.Errorf("default budget = %v, want 10s", budget)
	}
	cancelParent()
	if child.Err() != context.Canceled {
		t.Errorf("child lost parent cancellation: %v", child.Err())
	}

	deadline := time.Now().Add(4 * time.Second)
	parent, cancelParent = context.WithDeadline(context.Background(), deadline)
	defer cancelParent()
	bus.Ctx = parent
	child, _, budget, cancel = newVerificationProbeCommand("unused-probe-runtime", nil, root, "node", bus, types.VerificationProbe{TimeoutSeconds: 99}, "")
	defer cancel()
	if got, ok := child.Deadline(); !ok || !got.Equal(deadline) {
		t.Errorf("child deadline = %v (%t), want exact parent deadline %v", got, ok, deadline)
	}
	if budget != 30*time.Second {
		t.Errorf("probe budget changed with parent deadline: %v, want unchanged 30s cap", budget)
	}

	bus.Ctx = nil
	for _, seconds := range []int{0, 2, 99} {
		probe := types.VerificationProbe{TimeoutSeconds: seconds}
		start := time.Now()
		child, _, budget, cancel = newVerificationProbeCommand("unused-probe-runtime", nil, root, "node", bus, probe, "")
		want := verificationProbeTimeout(probe)
		got, ok := child.Deadline()
		if child.Err() != nil || budget != want || !ok || got.Before(start.Add(want)) || got.After(time.Now().Add(want)) {
			t.Errorf("nil parent compatibility changed: seconds=%d budget=%v deadline=%v err=%v", seconds, budget, got, child.Err())
		}
		cancel()
	}
}

func TestB1715NilParentProbePublicStillExecutes(t *testing.T) {
	for _, language := range []string{"python", "javascript"} {
		t.Run(language, func(t *testing.T) {
			root := t.TempDir()
			if !VerificationProbeRuntimeAvailable(language, root) {
				t.Skip("real probe runtime unavailable")
			}
			code := "from pathlib import Path\nPath('probe-executed').write_text('executed')\nassert True\n"
			if language == "javascript" {
				code = "require('fs').writeFileSync('probe-executed', 'executed'); require('assert/strict').ok(true);"
			}
			ctx := b1715ProbeContext(t, root, root, nil, types.VerificationProbe{ID: "uncanceled", Language: language, Code: code})
			result, err := (&RunTests{}).Execute(ctx, b1715RunnerParams(language))
			if err != nil || !result.Success || ctx.Mutable.ChangeReport() == nil || ctx.Mutable.ChangeReport().NormalizeVerificationStatus() != types.VerificationStatusPassed {
				t.Fatalf("nil parent public compatibility failed: result=%+v err=%v report=%+v", result, err, ctx.Mutable.ChangeReport())
			}
			if _, err := os.Stat(filepath.Join(root, "probe-executed")); err != nil {
				t.Errorf("uncanceled probe did not execute: %v", err)
			}
		})
	}
}

func b1715ProbeContext(t *testing.T, root, mainRoot string, parent context.Context, probe types.VerificationProbe) *types.BusContext {
	t.Helper()
	mu := types.NewMutableState("B1715 probe cancellation")
	mu.SetChangePlan(&types.ChangePlan{ID: "b1715-" + probe.ID, Status: types.PlanStatusApplied, VerificationProbes: []types.VerificationProbe{probe}})
	return &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: mainRoot, WorkDir: t.TempDir(), Ctx: parent}
}

func b1715RunnerParams(language string) json.RawMessage {
	if language == "python" {
		return json.RawMessage(`{"runner":"python","framework":"unittest"}`)
	}
	return json.RawMessage(`{"runner":"node"}`)
}

func b1715AssertCanceledReport(t *testing.T, report *types.ChangeReport) {
	t.Helper()
	if report == nil {
		t.Fatal("public tool did not install cancellation report")
	}
	if report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable || report.FailureKind != types.FailureKindVerificationIncomplete {
		t.Errorf("cancellation must be unavailable/incomplete, not pass, product failure, or timeout: passed=%t status=%s kind=%s", report.Passed, report.NormalizeVerificationStatus(), report.FailureKind)
	}
	for _, result := range report.TestResults {
		if result.Passed {
			t.Errorf("canceled probe retained a passed test row: %+v", result)
		}
	}
	for _, row := range report.ChangedPathCoverage {
		if row.Status == types.ChangedPathVerificationCovered && (row.Capability == types.VerificationCapabilityTargetExecution || row.Capability == types.VerificationCapabilityTargetBehavior) {
			t.Errorf("canceled probe minted target authority: %+v", row)
		}
	}
	for _, confidence := range report.VerificationConfidence {
		if confidence.Status == "satisfied" && (confidence.Category == "probe_baseline" || len(confidence.ContractRefs) > 0 || len(confidence.ChangedSymbolRefs) > 0) {
			t.Errorf("canceled probe minted proof confidence: %+v", confidence)
		}
	}
	for _, command := range report.ExecutedCommands {
		if command.Outcome == types.ExecutedCommandOutcomeExpectedFailureObserved {
			t.Errorf("cancellation minted expected_failure_observed: %+v", command)
		}
	}
}
