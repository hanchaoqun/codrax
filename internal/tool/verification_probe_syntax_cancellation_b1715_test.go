package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestVerificationProjectRunnerPreCanceledB1715(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX make recipe")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte(".PHONY: check\ncheck:\n\t@touch project-started\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := &types.BusContext{Ctx: parent, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(), Mode: types.ModeApply, PipelineStage: types.StageVerify, Mutable: types.NewMutableState("canceled project check")}
	ctx.Mutable.SetChangePlan(&types.ChangePlan{ID: "canceled-project", Status: types.PlanStatusApplied})
	result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"runner":"make","suite":"check"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "project-started")); !os.IsNotExist(err) {
		t.Fatalf("canceled project runner started: %v", err)
	}
	report := ctx.Mutable.ChangeReport()
	if result.Success || report == nil || report.Passed {
		t.Fatalf("canceled project verification passed: result=%+v report=%+v", result, report)
	}
}

func TestVerificationProbeSyntaxCancellationB1715(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake parser uses a POSIX executable; runtime cancellation is covered separately")
	}
	for _, tc := range []struct{ language, executable, diagnostic string }{
		{"python", "python3", "SyntaxError: fake parser was launched"},
		{"javascript", "node", "SyntaxError: fake parser was launched"},
		{"ruby", "ruby", "syntax error: fake parser was launched"},
		{"java", "javac", "compiler.err.expected: fake parser was launched"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			root := t.TempDir()
			marker := filepath.Join(root, "parser-started")
			script := "#!/bin/sh\n: > '" + marker + "'\nprintf '%s\\n' '" + tc.diagnostic + "' >&2\nexit 1\n"
			if err := os.WriteFile(filepath.Join(root, tc.executable), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", root)
			parent, cancel := context.WithCancel(context.Background())
			cancel()
			ctx := &types.BusContext{Ctx: parent, RepoRoot: root}
			if got := verificationProbeSyntaxError(ctx, tc.language, "valid source is irrelevant to a canceled invocation"); got != "" {
				t.Errorf("cancellation became an authoring rejection: %s", got)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("canceled syntax parser was started: %v", err)
			}
		})
	}
}

// Runtime launchers can leave descendants holding the parser's output pipes.
// Exercise the shared parser entry and the separate Java entry with a real
// child tree: caller cancellation must stop both the launcher and its child,
// without reclassifying an interrupted parser as invalid model source.
func TestVerificationProbeSyntaxLauncherTreeCancellationB1715(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled launcher fixture uses POSIX shell")
	}
	for _, tc := range []struct{ language, executable string }{
		{"javascript", "node"},
		{"java", "javac"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			root := t.TempDir()
			started := filepath.Join(root, "child-started")
			finished := filepath.Join(root, "child-finished")
			child := "printf ready > " + shellQuoteWord(started) + "; /bin/sleep 5; printf finished > " + shellQuoteWord(finished)
			launcher := "#!/bin/sh\n/bin/sh -c " + shellQuoteWord(child) + " &\nwait\n"
			if err := os.WriteFile(filepath.Join(root, tc.executable), []byte(launcher), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", root)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready := b1715DriftCancelAfterFile(started, cancel)
			ctx := &types.BusContext{Ctx: parent, RepoRoot: root}
			got := verificationProbeSyntaxError(ctx, tc.language, "source is not executed by the controlled launcher")
			returnedAt := time.Now()
			handshake := <-ready
			if handshake.Err != nil {
				t.Fatal(handshake.Err)
			}
			if handshake.CanceledAt.IsZero() || returnedAt.Before(handshake.CanceledAt) {
				t.Fatalf("syntax invocation returned before confirmed caller cancellation: canceled=%v returned=%v", handshake.CanceledAt, returnedAt)
			}
			if got != "" {
				t.Errorf("interrupted parser minted a syntax rejection: %s", got)
			}
			if elapsed := returnedAt.Sub(handshake.CanceledAt); elapsed >= 4*time.Second {
				t.Errorf("caller cancellation waited for launcher descendants: cancel_to_return=%v", elapsed)
			}
			if _, err := os.Stat(finished); !os.IsNotExist(err) {
				t.Errorf("parser child continued after caller cancellation: %v", err)
			}
		})
	}
}
