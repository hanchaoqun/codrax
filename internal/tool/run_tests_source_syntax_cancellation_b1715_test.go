package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These are real source-parser processes reached through public RunTests,
// not the separate inline verification-probe syntax-validation entry point.
func TestB1715SourceSyntaxFallbackCancellationPublic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled parser fixture uses POSIX shell")
	}
	for _, tc := range []struct {
		name      string
		cancel    bool
		deadline  bool
		fail      bool
		priorFail bool
	}{
		{name: "caller_cancel", cancel: true, fail: true},
		{name: "caller_deadline", deadline: true, fail: true},
		{name: "cancel_preserves_completed_diagnostic", cancel: true, fail: true, priorFail: true},
		{name: "ordinary_parser_diagnostic", fail: true},
		{name: "ordinary_parser_success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, bin := t.TempDir(), t.TempDir()
			paths := []string{"a-completed.js", "b-blocked.js", "z-later.js"}
			for _, path := range paths {
				b1715WriteProjectSuiteFile(t, root, path, "const value = 1;\n")
			}
			b1715WriteProjectSuiteFile(t, root, "package.json", "{}\n")
			started, finished, later := filepath.Join(root, "parser-ready"), filepath.Join(root, "parser-finished"), filepath.Join(root, "later-parser-started")
			pause := ""
			if tc.cancel || tc.deadline {
				pause = "/bin/sleep 5\n"
			}
			verdict := "exit 0\n"
			if tc.fail {
				verdict = "printf 'SyntaxError: B1715 exact parser diagnostic\\n' >&2\nexit 1\n"
			}
			firstVerdict := "printf 'B1715 earlier parser completed\\n'; exit 0"
			if tc.priorFail {
				firstVerdict = "printf 'SyntaxError: B1715 earlier completed diagnostic\\n' >&2; exit 1"
			}
			parser := fmt.Sprintf("#!/bin/sh\ncase \"$2\" in\n  */a-completed.js) %s;;\n  */b-blocked.js) printf 'B1715 current parser entered\\n'; printf ready > %s\n%sprintf finished > %s\n%s;;\n  */z-later.js) printf started > %s; exit 0;;\n  *) exit 17;;\nesac\n", firstVerdict, shellQuoteWord(started), pause, shellQuoteWord(finished), verdict, shellQuoteWord(later))
			parserPath := filepath.Join(bin, "node")
			if err := os.WriteFile(parserPath, []byte(parser), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			var canceled <-chan b1715CancellationReceipt
			if tc.deadline {
				deadline := &b1715ControlledProjectDeadline{Context: context.Background(), done: make(chan struct{}), at: time.Now().Add(time.Hour)}
				defer deadline.expire()
				parent = deadline
				canceled = b1715DriftCancelAfterFile(started, deadline.expire)
			} else if tc.cancel {
				canceled = b1715DriftCancelAfterFile(started, cancel)
			}
			mu := types.NewMutableState("B1715 source syntax cancellation")
			mu.SetChangePlan(&types.ChangePlan{ID: "b1715-source-syntax-" + tc.name, Status: types.PlanStatusApplied, TargetPaths: paths})
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(), Ctx: parent}
			result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"runner":"node"}`))
			returnedAt := time.Now()
			if err != nil {
				t.Fatalf("public Execute: %v", err)
			}
			report := mu.ChangeReport()
			if report == nil {
				t.Fatal("public source fallback did not install a report")
			}
			if canceled != nil {
				receipt := <-canceled
				if receipt.Err != nil {
					t.Fatal(receipt.Err)
				}
				if receipt.CanceledAt.IsZero() || returnedAt.Before(receipt.CanceledAt) || returnedAt.Sub(receipt.CanceledAt) >= 4*time.Second {
					t.Errorf("source parser did not return promptly after actual cancellation: cancel_to_return=%v", returnedAt.Sub(receipt.CanceledAt))
				}
				wantKind, wantStatus := types.FailureKindVerificationIncomplete, types.VerificationStatusUnavailable
				if tc.deadline {
					wantKind, wantStatus = types.FailureKindTimeout, types.VerificationStatusFailed
				}
				if result.Success || report.Passed || report.FailureKind != wantKind || report.NormalizeVerificationStatus() != wantStatus {
					t.Errorf("interrupted parser became a product verdict: success=%v report=%+v", result.Success, report)
				}
				if tc.priorFail {
					if !report.BuildFailed || len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "a-completed.js" || !strings.Contains(report.TestResults[0].FailureDetail, "B1715 earlier completed diagnostic") {
						t.Errorf("earlier completed source diagnostic was lost or replaced: %+v", report.TestResults)
					}
				} else if report.BuildFailed || len(report.TestResults) != 0 {
					t.Errorf("interrupted parser minted source diagnostics/assertions: %+v", report)
				}
				for _, marker := range []string{finished, later} {
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Errorf("post-cancellation source work ran: %s: %v", filepath.Base(marker), err)
					}
				}
				for _, confidence := range report.VerificationConfidence {
					if confidence.Category == "source_compile" && confidence.Status == "satisfied" {
						if tc.priorFail || len(confidence.ChangedSymbolRefs) != 1 || confidence.ChangedSymbolRefs[0] != "path:a-completed.js" {
							t.Errorf("interrupted source fallback minted compile proof outside prior completed file: %+v", confidence)
						}
					}
				}
				if got := verificationConfidenceContains(report.VerificationConfidence, "source_compile", "satisfied", "source_compile_ok"); got == tc.priorFail {
					t.Errorf("earlier independently completed parser observation not preserved: %+v", report.VerificationConfidence)
				}
				for _, coverage := range report.ChangedPathCoverage {
					if coverage.Caliber == types.ChangedPathVerificationSourceCheck {
						if tc.priorFail || coverage.Path != "a-completed.js" || coverage.Capability != types.VerificationCapabilitySyntaxOnly {
							t.Errorf("interrupted source fallback minted coverage outside prior completed file: %+v", coverage)
						}
					} else if coverage.Path == "a-completed.js" && !tc.priorFail {
						t.Errorf("earlier independently completed parser path was erased: %+v", coverage)
					}
				}
			} else {
				if _, err := os.Stat(later); err != nil {
					t.Errorf("normal per-file traversal no longer checks later source: %v", err)
				}
				if tc.fail {
					if result.Success || !report.BuildFailed || report.FailureKind != types.FailureKindBuildFailure || len(report.TestResults) != 1 || !strings.Contains(report.TestResults[0].FailureDetail, "SyntaxError: B1715 exact parser diagnostic") {
						t.Errorf("normal exact parser diagnosis changed: result=%+v report=%+v", result, report)
					}
				} else if !verificationConfidenceContains(report.VerificationConfidence, "source_compile", "satisfied", "source_compile_ok") {
					t.Errorf("completed source parser lost bounded compile confidence: %+v", report.VerificationConfidence)
				}
			}
			ref := report.FailureSummaryBlobRef
			if ref == "" {
				ref = result.RawRef
			}
			if ref == "" && canceled == nil && !tc.fail {
				return // A successful small result need not spill a raw artifact.
			}
			output, err := os.ReadFile(ref)
			if err != nil {
				t.Fatalf("source parser output missing: %q: %v", ref, err)
			}
			priorMarker := "ok    a-completed.js"
			if tc.priorFail {
				priorMarker = "B1715 earlier completed diagnostic"
			}
			if !strings.Contains(string(output), priorMarker) {
				t.Errorf("completed earlier parser observation was lost: %q", output)
			}
			if canceled != nil && !strings.Contains(string(output), "B1715 current parser entered") {
				t.Errorf("interrupted parser's actual output was lost: %q", output)
			}
		})
	}
}

// Fake parser names test dispatch/lifecycle only, not language acceptance.
// Direct provider calls ensure the common helper is not protected only by
// RunTests' outer project queue cancellation check.
func TestB1715SourceSyntaxSiblingEntriesPreCanceled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled parser fixture uses POSIX shell")
	}
	root, bin := t.TempDir(), t.TempDir()
	marker := filepath.Join(root, "parser-started")
	for _, binary := range []string{"node", "tsc", "ruby", "go", "mvn", "gradle", "javac", "kotlinc", "swift", "python", "python3"} {
		if err := os.WriteFile(filepath.Join(bin, binary), []byte("#!/bin/sh\nprintf started > "+shellQuoteWord(marker)+"\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	files := []string{"source.py", "source.js", "source.ts", "source.rb", "source.go", "Source.java", "source.kt", "source.swift"}
	for i, file := range files {
		b1715WriteProjectSuiteFile(t, root, file, "source\n")
		files[i] = filepath.Join(root, file)
	}
	b1715WriteProjectSuiteFile(t, root, "pom.xml", "<project/>\n")
	b1715WriteProjectSuiteFile(t, root, "Package.swift", "// fixture manifest\n")
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := &types.BusContext{Ctx: parent, RepoRoot: root, MainRepoRoot: root}
	type entry struct {
		name string
		run  func(*types.BusContext, string, string, []string) (*types.ChangeReport, string)
	}
	var entries []entry
	for _, provider := range sourceCheckProviderRegistry {
		entries = append(entries, entry{provider.Runner, provider.Run})
	}
	entries = append(entries, entry{"typescript_leaf", runTypeScriptCompileFallback}, entry{"java_leaf", runJavaProjectCompileFallback}, entry{"kotlin_leaf", runKotlinFileCompileFallback})
	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			report, _ := entry.run(ctx, "canceled-fixture", root, files)
			if report == nil || report.Passed || report.FailureKind != types.FailureKindVerificationIncomplete || len(report.TestResults) != 0 {
				t.Errorf("pre-canceled source entry minted a parser verdict: %+v", report)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Errorf("pre-canceled source entry started a parser: %v", err)
			}
		})
	}
	t.Run("python_static_leaf", func(t *testing.T) {
		_, ok := runPythonStaticNameCheck(ctx, filepath.Join(bin, "python3"), nil, root, files[0])
		if ok {
			t.Error("pre-canceled static name check claimed success")
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Errorf("pre-canceled static name check started a parser: %v", err)
		}
	})
}

func TestB1715SourceCompileConfidenceRequiresCompletedCleanCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		exit int
		want bool
	}{
		{"completed_before_later_cancellation", 0, true},
		{"interrupted_or_failed_command", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := &types.ChangeReport{FailureKind: types.FailureKindVerificationIncomplete, FailureReasonCode: "verification_canceled", ExecutedCommands: []types.ExecutedCommand{{Runner: "node", Outcome: types.ExecutedCommandOutcomeSyntaxCheckFallback, ExitCode: tc.exit,
				CoveredPaths: []string{"completed.js"}, SourceCheckExecution: &types.SourceCheckExecutionReceipt{Version: types.SourceCheckExecutionReceiptVersion, Started: true, Completed: true, ExitCodeKnown: true, ExitCode: tc.exit, CheckedPaths: []string{"completed.js"}},
			}}}
			got := verificationConfidenceContains(verificationConfidenceRecordsFromReport(nil, report), "source_compile", "satisfied", "source_compile_ok")
			if got != tc.want {
				t.Errorf("source compile confidence=%v, want %v for exit %d", got, tc.want, tc.exit)
			}
		})
	}
}

func TestB1715SourceSyntaxPythonPreparationCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled Python candidate fixture uses POSIX executable aliases")
	}
	captureControlledProcessFailureDiagnostics(t)
	root := t.TempDir()
	started, finished, later := filepath.Join(root, "python-preparation-ready"), filepath.Join(root, "python-preparation-finished"), filepath.Join(root, "later-python-candidate")
	bin := installControlledProcessFixture(t, "python_preparation", root, "python3", "python")
	t.Setenv("PATH", bin)
	path := filepath.Join(root, "source.py")
	if err := os.WriteFile(path, []byte("value = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	canceled := b1715DriftCancelAfterFile(started, cancel)
	ctx := &types.BusContext{Ctx: parent, RepoRoot: root, MainRepoRoot: root}
	executionStartedAt := time.Now()
	report, output := runPyCompileFallback(ctx, "python-preparation", root, []string{path})
	returnedAt := time.Now()
	receipt := <-canceled
	if receipt.Err != nil {
		encodedReport, _ := json.Marshal(report)
		_, startedErr := os.Stat(started)
		_, finishedErr := os.Stat(finished)
		_, laterErr := os.Stat(later)
		t.Fatalf("%v; provider_returned_after=%v returned_before_cancel=%v markers(started=%v finished=%v later=%v) output=%q report=%s", receipt.Err,
			returnedAt.Sub(executionStartedAt), returnedAt.Before(receipt.CanceledAt), startedErr, finishedErr, laterErr, output, encodedReport)
	}
	if receipt.CanceledAt.IsZero() || returnedAt.Before(receipt.CanceledAt) || returnedAt.Sub(receipt.CanceledAt) >= 4*time.Second {
		t.Errorf("Python preparation ignored caller cancellation: cancel_to_return=%v", returnedAt.Sub(receipt.CanceledAt))
	}
	if report == nil || report.Passed || report.BuildFailed || len(report.TestResults) != 0 || report.FailureKind != types.FailureKindVerificationIncomplete {
		t.Errorf("canceled Python preparation became syntax evidence: %+v", report)
	}
	for _, marker := range []string{finished, later} {
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Errorf("canceled preparation executed later work: %s: %v", filepath.Base(marker), err)
		}
	}
}

func TestB1715SourceSyntaxPythonPreparationLegacyAndPreCanceled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled Python candidate fixture uses POSIX shell")
	}
	for _, tc := range []struct {
		name     string
		canceled bool
		legacy   bool
	}{
		{name: "pre_canceled", canceled: true},
		{name: "nil_parent_keeps_candidate_fallback"},
		{name: "legacy_signature_keeps_candidate_fallback", legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, bin := t.TempDir(), t.TempDir()
			first, second := filepath.Join(root, "first-candidate"), filepath.Join(root, "second-candidate")
			for binary, body := range map[string]string{
				"python3": "#!/bin/sh\nprintf first > " + shellQuoteWord(first) + "\nexit 1\n",
				"python":  "#!/bin/sh\nprintf second > " + shellQuoteWord(second) + "\nexit 0\n",
			} {
				if err := os.WriteFile(filepath.Join(bin, binary), []byte(body), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)
			var parent context.Context
			if tc.canceled {
				var cancel context.CancelFunc
				parent, cancel = context.WithCancel(context.Background())
				cancel()
			}
			var runner pythonDryBuildRunner
			var ok bool
			if tc.legacy {
				runner, ok = resolvePythonDryBuildRunner()
			} else {
				runner, ok = resolvePythonDryBuildRunnerWithContext(parent)
			}
			if tc.canceled {
				if ok {
					t.Errorf("pre-canceled resolver accepted candidate: %+v", runner)
				}
				for _, marker := range []string{first, second} {
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Errorf("pre-canceled resolver started candidate: %s: %v", marker, err)
					}
				}
			} else {
				if !ok || runner.ExePath != filepath.Join(bin, "python") {
					t.Errorf("normal failed-candidate fallback changed: %+v, ok=%v", runner, ok)
				}
				for _, marker := range []string{first, second} {
					if _, err := os.Stat(marker); err != nil {
						t.Errorf("normal candidate was not attempted: %s: %v", marker, err)
					}
				}
			}
		})
	}
}
