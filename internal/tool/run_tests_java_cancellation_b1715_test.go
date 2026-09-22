package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A completed main is an observed fact, unlike the next main whose process
// is interrupted. Exercise ordinary surface discovery and the public tool,
// not a fabricated ChangeReport or a direct parser invocation.
func TestB1715ManifestlessJavaCancellationPreservesCompletedMainPublic(t *testing.T) {
	root := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	b1715JavaCancellationFixture(t, root)
	b1715JavaCancellationRuntime(t, root, listener.Addr().String())
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := b1715JavaCancellationContext(t, root, parent)
	type execution struct {
		result types.ToolResult
		err    error
	}
	done := make(chan execution, 1)
	go func() {
		result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{}`))
		done <- execution{result, err}
	}()
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	connection, err := listener.Accept()
	if err != nil {
		select {
		case got := <-done:
			t.Fatalf("Java pipeline ended before its second main handshake: result=%+v err=%v", got.result, got.err)
		default:
			t.Fatalf("Java second main did not reach readiness: %v", err)
		}
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	ready, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil || ready != "second-main-running\n" {
		t.Fatalf("Java readiness = %q, %v", ready, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "first-main-completed")); err != nil || string(data) != "completed" {
		t.Fatalf("second main started without the first main's real side effect: %q, %v", data, err)
	}
	cancel()
	var got execution
	select {
	case got = <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("manifestless Java did not stop after caller cancellation")
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	report := ctx.Mutable.ChangeReport()
	if report == nil || got.result.Success || report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable || report.FailureKind != types.FailureKindVerificationIncomplete || report.FailureReasonCode != "verification_canceled" {
		t.Fatalf("canceled Java became a terminal verdict: result=%+v report=%+v", got.result, report)
	}
	if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "FirstTest" || !report.TestResults[0].Passed || report.TestResults[0].Suite != "manifestless-java-main" {
		t.Errorf("must retain only the completed main assertion, not an interrupted failed row: %+v", report.TestResults)
	}
	var commands []types.ExecutedCommand
	for _, command := range report.ExecutedCommands {
		if command.Runner == "java" && command.Framework == javaFrameworkDirectMain {
			commands = append(commands, command)
		}
	}
	if len(commands) != 3 {
		t.Fatalf("expected completed compiler, completed first main, interrupted second main receipts: %+v", commands)
	}
	assertNativeInvocationRowsOwned(t, report)
	ids := map[string]bool{}
	for _, command := range commands {
		if command.InvocationID == "" || ids[command.InvocationID] {
			t.Fatalf("compiler and separate main processes need distinct invocation identities: %+v", commands)
		}
		ids[command.InvocationID] = true
	}
	if len(report.TestResults) == 1 && report.TestResults[0].InvocationID != commands[1].InvocationID {
		t.Fatal("completed main result must not borrow the compiler or interrupted main identity")
	}
	if !strings.HasPrefix(commands[0].Command, "javac ") || commands[0].ExitCode != 0 || commands[0].Outcome != types.ExecutedCommandOutcomeSyntaxPreflight {
		t.Errorf("completed compiler receipt was altered: %+v", commands[0])
	}
	if commands[1].Command != "java -ea FirstTest" || commands[1].ExitCode != 0 || commands[1].Outcome != types.ExecutedCommandOutcomeExecuted {
		t.Errorf("completed first main receipt was altered: %+v", commands[1])
	}
	if commands[2].Command != "java -ea SecondTest" || commands[2].ExitCode == 0 {
		t.Errorf("interrupted process receipt was lost or relabeled successful: %+v", commands[2])
	}
	if _, err := os.Stat(filepath.Join(root, "third-main-started")); !os.IsNotExist(err) {
		t.Errorf("a later main executed after cancellation: %v", err)
	}
	ref := got.result.RawRef
	if ref == "" {
		ref = report.FailureSummaryBlobRef
	}
	output, err := os.ReadFile(ref)
	if err != nil {
		t.Fatalf("real subprocess output was not retained: %q: %v", ref, err)
	}
	for _, marker := range []string{"B1715 first main completed", "B1715 second main entered"} {
		if !strings.Contains(string(output), marker) {
			t.Errorf("retained output lost %q", marker)
		}
	}
}

func TestB1715ManifestlessJavaPreCanceledPublicNoExecution(t *testing.T) {
	root := t.TempDir()
	b1715JavaCancellationFixture(t, root)
	// Always use observable subprocess tools for this negative control: even
	// a compiler launch with no source side effect must be detectable.
	b1715JavaCancellationRuntime(t, root, "127.0.0.1:1")
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := b1715JavaCancellationContext(t, root, parent)
	result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	report := ctx.Mutable.ChangeReport()
	if report == nil || result.Success || report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable || report.FailureReasonCode != "verification_canceled" {
		t.Fatalf("pre-canceled Java was not unavailable: result=%+v report=%+v", result, report)
	}
	if len(report.ExecutedCommands) != 0 || len(report.TestResults) != 0 {
		t.Errorf("pre-canceled Java invented execution or assertions: commands=%+v tests=%+v", report.ExecutedCommands, report.TestResults)
	}
	for _, marker := range []string{"java-tool-started", "first-main-completed", "third-main-started"} {
		if _, err := os.Stat(filepath.Join(root, marker)); !os.IsNotExist(err) {
			t.Errorf("pre-canceled Java started %s: %v", marker, err)
		}
	}
}

func b1715JavaCancellationContext(t *testing.T, root string, parent context.Context) *types.BusContext {
	t.Helper()
	mu := types.NewMutableState("B1715 manifestless Java cancellation")
	mu.SetChangePlan(&types.ChangePlan{ID: "b1715-java-cancel", Status: types.PlanStatusApplied})
	return &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(), Ctx: parent}
}

func b1715JavaCancellationFixture(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"FirstTest", "SecondTest", "ThirdTest"} {
		source := "public class " + name + " { public static void main(String[] args) {} }\n"
		if err := os.WriteFile(filepath.Join(root, name+".java"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if surface := discoverManifestlessJavaMainSurface(root); strings.Join(surface.TestMainClasses, ",") != "FirstTest,SecondTest,ThirdTest" {
		t.Fatalf("fixture did not discover all ordered real main declarations: %+v", surface)
	}
}

func b1715JavaCancellationRuntime(t *testing.T, root, address string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("deterministic subprocess launchers use POSIX shell")
	}
	t.Log("using deterministic subprocess compiler/runtime stand-ins; this tests cancellation, not Java semantics")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	bin := t.TempDir()
	for _, name := range []string{"javac", "java"} {
		script := "#!/bin/sh\nexport CODRAX_B1715_JAVA_HELPER=1\nexec " + quote(binary) + " -test.run='^TestB1715ManifestlessJavaProcessHelper$' -- " + quote(name) + " \"$@\"\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CODRAX_B1715_JAVA_ROOT", root)
	t.Setenv("CODRAX_B1715_JAVA_ADDRESS", address)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestB1715ManifestlessJavaProcessHelper(t *testing.T) {
	if os.Getenv("CODRAX_B1715_JAVA_HELPER") != "1" {
		return
	}
	fail := func(err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	root := os.Getenv("CODRAX_B1715_JAVA_ROOT")
	fail(os.WriteFile(filepath.Join(root, "java-tool-started"), []byte("started"), 0o600))
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) < 2 {
		fail(fmt.Errorf("missing fake Java command arguments"))
	}
	if args[0] == "javac" {
		fmt.Println("B1715 compiler completed")
		os.Exit(0)
	}
	switch args[len(args)-1] {
	case "FirstTest":
		fail(os.WriteFile(filepath.Join(root, "first-main-completed"), []byte("completed"), 0o600))
		fmt.Println("B1715 first main completed")
	case "SecondTest":
		data, err := os.ReadFile(filepath.Join(root, "first-main-completed"))
		fail(err)
		if string(data) != "completed" {
			fail(fmt.Errorf("first main did not complete"))
		}
		fmt.Println("B1715 second main entered")
		connection, err := net.DialTimeout("tcp", os.Getenv("CODRAX_B1715_JAVA_ADDRESS"), 5*time.Second)
		fail(err)
		defer connection.Close()
		_, err = connection.Write([]byte("second-main-running\n"))
		fail(err)
		var release [1]byte
		_, err = io.ReadFull(connection, release[:])
		fail(err)
	case "ThirdTest":
		fail(os.WriteFile(filepath.Join(root, "third-main-started"), []byte("started"), 0o600))
	default:
		fail(fmt.Errorf("unexpected main arguments: %v", args))
	}
	os.Exit(0)
}
