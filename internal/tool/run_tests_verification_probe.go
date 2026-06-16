package tool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

type verificationProbeRunResult struct {
	Report   *types.ChangeReport
	Output   string
	Commands []types.ExecutedCommand
}

type pythonVerificationProbeStatus struct {
	Outcome   string `json:"outcome,omitempty"`
	Exception string `json:"exception,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
}

const pythonVerificationProbeWrapper = `
import base64
import json
import os
import sys
import traceback

result_path = os.environ.get("CODRAX_VERIFICATION_PROBE_RESULT", "")
encoded = os.environ.get("CODRAX_VERIFICATION_PROBE_CODE", "")

def write_result(outcome, exception="", exit_code=0):
    if not result_path:
        return
    with open(result_path, "w", encoding="utf-8") as handle:
        json.dump({"outcome": outcome, "exception": exception, "exit_code": int(exit_code or 0)}, handle, sort_keys=True)

try:
    source = base64.b64decode(encoded.encode("ascii")).decode("utf-8")
    ns = {"__name__": "__main__", "__file__": "<codrax_verification_probe>"}
    exec(compile(source, "<codrax_verification_probe>", "exec"), ns, ns)
except AssertionError as exc:
    traceback.print_exc()
    write_result("assertion_failed", exc.__class__.__name__, 1)
    sys.exit(1)
except SystemExit as exc:
    code = exc.code
    exit_code = code if isinstance(code, int) else (0 if code is None else 1)
    write_result("passed" if exit_code == 0 else "system_exit", exc.__class__.__name__, exit_code)
    raise
except (ImportError, ModuleNotFoundError) as exc:
    traceback.print_exc()
    write_result("import_error", exc.__class__.__name__, 1)
    sys.exit(1)
except SyntaxError as exc:
    traceback.print_exc()
    write_result("syntax_error", exc.__class__.__name__, 1)
    sys.exit(1)
except BaseException as exc:
    traceback.print_exc()
    write_result("exception", exc.__class__.__name__, 1)
    sys.exit(1)
else:
    write_result("passed", "", 0)
`

func runPlanVerificationProbes(ctx *types.BusContext, source string) (*verificationProbeRunResult, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, false
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) == 0 {
		return nil, false
	}
	var (
		results  []types.TestResult
		outputs  []string
		commands []types.ExecutedCommand
	)
	for _, probe := range plan.VerificationProbes {
		res := runSingleVerificationProbe(ctx, probe, source)
		results = append(results, res.Report.TestResults...)
		if strings.TrimSpace(res.Output) != "" {
			outputs = append(outputs, res.Output)
		} else if strings.TrimSpace(res.Report.FailureSummary) != "" && !res.Report.Passed {
			outputs = append(outputs, res.Report.FailureSummary)
		} else {
			outputs = append(outputs, "")
		}
		commands = append(commands, res.Commands...)
	}
	passed := true
	var failureKind types.FailureKind
	failureReasonCode := ""
	for _, result := range results {
		if !result.Passed {
			passed = false
			break
		}
	}
	if !passed {
		failureKind = types.FailureKindTestsFailed
		for _, cmd := range commands {
			switch cmd.Outcome {
			case "runner_missing":
				failureKind = types.FailureKindRunnerMissing
			case "parser_error":
				failureKind = types.FailureKindParserError
			case "timeout":
				failureKind = types.FailureKindTimeout
			case "oom":
				failureKind = types.FailureKindOOM
			case "cpu_limit":
				failureKind = types.FailureKindCPULimit
			}
			if failureReasonCode == "" && strings.TrimSpace(cmd.ReasonCode) != "" {
				failureReasonCode = strings.TrimSpace(cmd.ReasonCode)
			}
			if failureKind != types.FailureKindTestsFailed {
				break
			}
		}
	}
	summary := ""
	if !passed {
		summary = strings.TrimSpace(strings.Join(outputs, "\n\n"))
		if summary == "" {
			summary = "verification probe failed"
		}
	}
	report := &types.ChangeReport{
		PlanID:            plan.ID,
		TestResults:       results,
		Passed:            passed,
		FailureKind:       failureKind,
		FailureReasonCode: failureReasonCode,
		FailureSummary:    summary,
	}
	return &verificationProbeRunResult{
		Report:   report,
		Output:   renderVerificationProbeOutput(plan.VerificationProbes, outputs),
		Commands: commands,
	}, true
}

func runSingleVerificationProbe(ctx *types.BusContext, probe types.VerificationProbe, source string) verificationProbeRunResult {
	id := strings.TrimSpace(probe.ID)
	if id == "" {
		id = "probe"
	}
	lang := strings.ToLower(strings.TrimSpace(probe.Language))
	if lang == "" {
		lang = "python"
	}
	wd, rel, dirErr := resolveVerificationProbeWorkingDir(ctx.RepoRoot, probe.WorkingDir, lang)
	if dirErr != nil {
		detail := dirErr.Error()
		return verificationProbeRunResult{
			Report: &types.ChangeReport{
				TestResults: []types.TestResult{{
					Kind:          types.TestResultKindUnit,
					AssertionID:   id,
					Suite:         "verification_probe/" + lang,
					Passed:        false,
					FailureDetail: detail,
				}},
				Passed:         false,
				FailureKind:    types.FailureKindTestsFailed,
				FailureSummary: detail,
			},
			Output: detail,
			Commands: []types.ExecutedCommand{{
				Runner:     "verification_probe",
				Framework:  lang,
				WorkingDir: rel,
				Source:     source,
				Outcome:    "probe_config_error",
			}},
		}
	}
	switch lang {
	case "python":
		return runPythonVerificationProbe(ctx, probe, id, wd, rel, source)
	default:
		detail := fmt.Sprintf("verification probe %q has unsupported language %q", id, probe.Language)
		return verificationProbeRunResult{
			Report: &types.ChangeReport{
				TestResults: []types.TestResult{{
					Kind:          types.TestResultKindUnit,
					AssertionID:   id,
					Suite:         "verification_probe/" + lang,
					Passed:        false,
					FailureDetail: detail,
				}},
				Passed:         false,
				FailureKind:    types.FailureKindTestsFailed,
				FailureSummary: detail,
			},
			Output: detail,
			Commands: []types.ExecutedCommand{{
				Runner:     "verification_probe",
				Framework:  lang,
				WorkingDir: rel,
				Source:     source,
				Outcome:    "probe_config_error",
			}},
		}
	}
}

func runPythonVerificationProbe(ctx *types.BusContext, probe types.VerificationProbe, id, wd, rel, source string) verificationProbeRunResult {
	timeout := time.Duration(probe.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	interp := pythonRuntimeInterpreter(ctx.RepoRoot, ctx.MainRepoRoot, wd)
	statusPath := ""
	if f, err := os.CreateTemp("", "codrax-verification-probe-*.json"); err == nil {
		statusPath = f.Name()
		_ = f.Close()
		defer os.Remove(statusPath)
	}
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(execCtx, interp, "-c", pythonVerificationProbeWrapper)
	cmd.Dir = wd
	cmd.Env = append(runnerExecutionEnv("python", ctx.RepoRoot, wd, ctx.MainRepoRoot),
		"CODRAX_VERIFICATION_PROBE_CODE="+base64.StdEncoding.EncodeToString([]byte(probe.Code)),
		"CODRAX_VERIFICATION_PROBE_RESULT="+statusPath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	caps := verifyResourceCaps()
	if timeoutSeconds := uint64(timeout.Seconds()); timeoutSeconds > 0 && caps.CPULimitSeconds > timeoutSeconds {
		caps.CPULimitSeconds = timeoutSeconds
	}
	supRes := SupervisedRun(execCtx, cmd, caps)
	duration := time.Since(start)
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}
	exitCode := extractExitCode(supRes.Err)
	probeStatus := readPythonVerificationProbeStatus(statusPath)
	outcome := "executed"
	failureKind := types.FailureKindTestsFailed
	switch supRes.ExitKind {
	case SupervisedExitTimeout:
		outcome = "timeout"
		failureKind = types.FailureKindTimeout
	case SupervisedExitOOM:
		outcome = "oom"
		failureKind = types.FailureKindOOM
	case SupervisedExitCPULimit:
		outcome = "cpu_limit"
		failureKind = types.FailureKindCPULimit
	default:
		if missing, _, _ := detectRunnerMissing("python", supRes.Err, output); missing {
			outcome = "runner_missing"
			failureKind = types.FailureKindRunnerMissing
		}
	}
	passed := supRes.Err == nil
	reasonCode := ""
	if outcome == "executed" && probeStatus.Outcome != "" {
		switch probeStatus.Outcome {
		case "passed":
			passed = passed && probeStatus.ExitCode == 0
		case "assertion_failed", "system_exit":
			passed = false
			failureKind = types.FailureKindTestsFailed
		// Unhandled probe exceptions are probe/runtime failures, not an
		// authoritative product-code assertion. Keep them out of the
		// verify->replan hard-failure lane; the project runner can still
		// provide a stronger verdict after this pre-suite probe degrades.
		case "import_error", "syntax_error", "exception":
			passed = false
			outcome = "parser_error"
			failureKind = types.FailureKindParserError
			reasonCode = pythonVerificationProbeReasonCode(probeStatus)
		default:
			passed = false
			outcome = "parser_error"
			failureKind = types.FailureKindParserError
			reasonCode = pythonVerificationProbeReasonCode(probeStatus)
		}
	}
	var missingExpected []string
	if passed && len(probe.ExpectedStdout) > 0 {
		for _, fragment := range probe.ExpectedStdout {
			if !strings.Contains(stdout.String(), fragment) {
				missingExpected = append(missingExpected, fragment)
			}
		}
		if len(missingExpected) > 0 {
			passed = false
			outcome = "expected_stdout_missing"
		}
	}
	if passed {
		failureKind = ""
	}
	detail := ""
	if !passed {
		detail = stdoutHead(output, 4000)
		if probeStatus.Outcome != "" && probeStatus.Outcome != "passed" {
			if detail != "" {
				detail += "\n"
			}
			detail += fmt.Sprintf("verification probe structured outcome: %s", probeStatus.Outcome)
			if probeStatus.Exception != "" {
				detail += " (" + probeStatus.Exception + ")"
			}
		}
		if len(missingExpected) > 0 {
			if detail != "" {
				detail += "\n"
			}
			detail += "missing expected stdout fragments: " + strings.Join(missingExpected, ", ")
		}
		if detail == "" {
			detail = fmt.Sprintf("verification probe %q exited with code %d", id, exitCode)
		}
	}
	logging.Info("[run_tests] verification_probe id=%s lang=python cwd=%s outcome=%s exit=%d duration=%v",
		id, rel, outcome, exitCode, duration)
	return verificationProbeRunResult{
		Report: &types.ChangeReport{
			TestResults: []types.TestResult{{
				Kind:          types.TestResultKindUnit,
				AssertionID:   id,
				Suite:         "verification_probe/python",
				Passed:        passed,
				Duration:      duration,
				FailureDetail: detail,
			}},
			Passed:            passed,
			FailureKind:       failureKind,
			FailureReasonCode: reasonCode,
			FailureSummary:    detail,
		},
		Output: output,
		Commands: []types.ExecutedCommand{{
			Runner:     "verification_probe",
			Framework:  "python",
			WorkingDir: rel,
			Command:    "python -c <verification_probe:" + id + ">",
			ExitCode:   exitCode,
			DurationMS: duration.Milliseconds(),
			Source:     source,
			Outcome:    outcome,
			ReasonCode: reasonCode,
		}},
	}
}

func pythonVerificationProbeReasonCode(status pythonVerificationProbeStatus) string {
	outcome := strings.TrimSpace(status.Outcome)
	exception := strings.TrimSpace(status.Exception)
	switch outcome {
	case "import_error":
		switch exception {
		case "ModuleNotFoundError":
			return "verification_probe_module_not_found"
		case "ImportError":
			return "verification_probe_import_error"
		default:
			return "verification_probe_import_error"
		}
	case "syntax_error":
		return "verification_probe_syntax_error"
	case "exception":
		if exception != "" {
			return "verification_probe_exception"
		}
		return "verification_probe_runtime_exception"
	default:
		if outcome != "" {
			return "verification_probe_unclassified_" + strings.ReplaceAll(outcome, "-", "_")
		}
		return "verification_probe_unclassified"
	}
}

func resolveVerificationProbeWorkingDir(repoRoot, workingDir, language string) (string, string, error) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return "", "", fmt.Errorf("verification probe requires repo root")
	}
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		rootAbs = repoRoot
	}
	rel := strings.TrimSpace(workingDir)
	if rel == "" {
		rel = "."
	}
	cleaned := filepath.Clean(rel)
	if filepath.IsAbs(cleaned) {
		return "", filepath.ToSlash(cleaned), fmt.Errorf("verification probe working_dir %q must be repo-relative", workingDir)
	}
	for _, seg := range strings.Split(filepath.ToSlash(cleaned), "/") {
		if seg == ".." {
			return "", filepath.ToSlash(cleaned), fmt.Errorf("verification probe working_dir %q escapes the repository", workingDir)
		}
	}
	target := filepath.Join(rootAbs, cleaned)
	backRel, err := filepath.Rel(rootAbs, target)
	if err != nil || strings.HasPrefix(filepath.ToSlash(backRel), "../") || backRel == ".." {
		return "", filepath.ToSlash(cleaned), fmt.Errorf("verification probe working_dir %q resolves outside the repository", workingDir)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return "", filepath.ToSlash(cleaned), fmt.Errorf("verification probe working_dir %q does not exist or is not a directory", workingDir)
	}
	if backRel == "" {
		backRel = "."
	}
	if strings.EqualFold(strings.TrimSpace(language), "python") {
		if root, ok := nearestPythonProbeProjectRoot(rootAbs, target); ok {
			rootRel, err := filepath.Rel(rootAbs, root)
			if err == nil {
				if rootRel == "" {
					rootRel = "."
				}
				rootRel = filepath.ToSlash(rootRel)
				if rootRel != filepath.ToSlash(backRel) {
					logging.Info("[run_tests] verification_probe python working_dir %q executes at project root %q to avoid package-internal cwd import shadowing",
						filepath.ToSlash(backRel), rootRel)
				}
				return root, rootRel, nil
			}
		}
	}
	return target, filepath.ToSlash(backRel), nil
}

func nearestPythonProbeProjectRoot(repoRoot, target string) (string, bool) {
	repoRoot = filepath.Clean(repoRoot)
	target = filepath.Clean(target)
	if !pathWithinRoot(repoRoot, target) {
		return "", false
	}
	for dir := target; ; dir = filepath.Dir(dir) {
		if pythonProbeProjectRootMarker(dir) {
			return dir, true
		}
		if samePath(dir, repoRoot) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", false
}

func pythonProbeProjectRootMarker(dir string) bool {
	for _, name := range []string{
		"pyproject.toml",
		"setup.py",
		"setup.cfg",
		"pytest.ini",
		"tox.ini",
		"noxfile.py",
		"manage.py",
	} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func readPythonVerificationProbeStatus(path string) pythonVerificationProbeStatus {
	path = strings.TrimSpace(path)
	if path == "" {
		return pythonVerificationProbeStatus{}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return pythonVerificationProbeStatus{}
	}
	var status pythonVerificationProbeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return pythonVerificationProbeStatus{}
	}
	status.Outcome = strings.TrimSpace(status.Outcome)
	status.Exception = strings.TrimSpace(status.Exception)
	return status
}

func renderVerificationProbeOutput(probes []types.VerificationProbe, outputs []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### verification_probes (%d)\n", len(probes))
	for i, probe := range probes {
		id := strings.TrimSpace(probe.ID)
		if id == "" {
			id = fmt.Sprintf("probe-%d", i+1)
		}
		lang := strings.ToLower(strings.TrimSpace(probe.Language))
		if lang == "" {
			lang = "python"
		}
		workingDir := strings.TrimSpace(probe.WorkingDir)
		if workingDir == "" {
			workingDir = "."
		}
		timeout := probe.TimeoutSeconds
		if timeout <= 0 {
			timeout = 10
		}
		if timeout > 30 {
			timeout = 30
		}
		fmt.Fprintf(&b, "#### %s\n", id)
		fmt.Fprintf(&b, "language=%s working_dir=%s timeout_seconds=%d\n", lang, workingDir, timeout)
		if len(probe.ExpectedStdout) > 0 {
			fmt.Fprintf(&b, "expected_stdout=%q\n", probe.ExpectedStdout)
		}
		if len(probe.ContractRefs) > 0 {
			fmt.Fprintf(&b, "contract_refs=%q\n", probe.ContractRefs)
		}
		if len(probe.ChangedSymbolRefs) > 0 {
			fmt.Fprintf(&b, "changed_symbol_refs=%q\n", probe.ChangedSymbolRefs)
		}
		if probe.ExpectsBaselineFailure {
			b.WriteString("expects_baseline_failure=true\n")
		}
		code := strings.TrimRight(probe.Code, "\n")
		if code != "" {
			b.WriteString("source:\n")
			fmt.Fprintf(&b, "```%s\n", lang)
			b.WriteString(stdoutHead(code, 2000))
			if !strings.HasSuffix(code, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("```\n")
		}
		b.WriteString("output:\n")
		if i < len(outputs) && strings.TrimSpace(outputs[i]) != "" {
			b.WriteString(outputs[i])
			if !strings.HasSuffix(outputs[i], "\n") {
				b.WriteString("\n")
			}
		} else {
			b.WriteString("(no output)\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
