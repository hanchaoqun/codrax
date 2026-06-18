package tool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	Outcome       string `json:"outcome,omitempty"`
	Exception     string `json:"exception,omitempty"`
	ExitCode      int    `json:"exit_code,omitempty"`
	ProbeTopLevel bool   `json:"probe_top_level,omitempty"`
}

type inlineVerificationProbeStatus struct {
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

def probe_top_level_exception(exc):
    frames = traceback.extract_tb(exc.__traceback__)
    if not frames:
        return False
    has_probe_frame = any(frame.filename == "<codrax_verification_probe>" for frame in frames)
    has_product_frame = any(frame.filename not in ("<string>", "<codrax_verification_probe>") for frame in frames)
    return bool(has_probe_frame and not has_product_frame)

def write_result(outcome, exception="", exit_code=0, probe_top_level=False):
    if not result_path:
        return
    with open(result_path, "w", encoding="utf-8") as handle:
        json.dump({"outcome": outcome, "exception": exception, "exit_code": int(exit_code or 0), "probe_top_level": bool(probe_top_level)}, handle, sort_keys=True)

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
    write_result("exception", exc.__class__.__name__, 1, probe_top_level_exception(exc))
    sys.exit(1)
else:
    write_result("passed", "", 0)
`

const javascriptVerificationProbeWrapper = `
const fs = require("fs");
const vm = require("vm");

const resultPath = process.env.CODRAX_VERIFICATION_PROBE_RESULT || "";
const encoded = process.env.CODRAX_VERIFICATION_PROBE_CODE || "";

function writeResult(outcome, exception, exitCode) {
  if (!resultPath) return;
  fs.writeFileSync(resultPath, JSON.stringify({
    outcome: outcome || "",
    exception: exception || "",
    exit_code: Number(exitCode || 0)
  }));
}

try {
  const source = Buffer.from(encoded, "base64").toString("utf8");
  vm.runInThisContext(source, {filename: "<codrax_verification_probe>"});
  writeResult("passed", "", 0);
} catch (err) {
  if (err && err.stack) {
    console.error(err.stack);
  } else {
    console.error(String(err));
  }
  const name = err && err.name ? String(err.name) : "Error";
  if (name === "AssertionError") {
    writeResult("assertion_failed", name, 1);
  } else if (name === "SyntaxError") {
    writeResult("syntax_error", name, 1);
  } else if (name === "ReferenceError") {
    writeResult("reference_error", name, 1);
  } else if (err && err.code === "MODULE_NOT_FOUND") {
    writeResult("import_error", name, 1);
  } else {
    writeResult("exception", name, 1);
  }
  process.exit(1);
}
`

const rubyVerificationProbeWrapper = `
require "base64"
require "json"

result_path = ENV.fetch("CODRAX_VERIFICATION_PROBE_RESULT", "")
encoded = ENV.fetch("CODRAX_VERIFICATION_PROBE_CODE", "")

def write_result(path, outcome, exception="", exit_code=0)
  return if path.empty?
  File.write(path, JSON.generate({
    "outcome" => outcome.to_s,
    "exception" => exception.to_s,
    "exit_code" => exit_code.to_i,
  }))
end

begin
  source = Base64.decode64(encoded)
  eval(source, TOPLEVEL_BINDING, "<codrax_verification_probe>")
rescue SystemExit => e
  write_result(result_path, e.status.to_i == 0 ? "passed" : "system_exit", e.class.name, e.status.to_i)
  raise
rescue SyntaxError => e
  warn e.full_message
  write_result(result_path, "syntax_error", e.class.name, 1)
  exit 1
rescue LoadError => e
  warn e.full_message
  write_result(result_path, "import_error", e.class.name, 1)
  exit 1
rescue Exception => e
  warn e.full_message
  outcome = e.class.name == "AssertionError" ? "assertion_failed" : "exception"
  write_result(result_path, outcome, e.class.name, 1)
  exit 1
else
  write_result(result_path, "passed", "", 0)
end
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
		ExecutedCommands:  append([]types.ExecutedCommand(nil), commands...),
	}
	report.EnsureVerificationStatus()
	report.VerificationConfidence = mergeVerificationConfidenceRecords(
		report.VerificationConfidence,
		verificationConfidenceRecordsFromReport(plan, report),
	)
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
	lang, ok := normalizeVerificationProbeLanguage(probe.Language)
	if !ok {
		lang = strings.ToLower(strings.TrimSpace(probe.Language))
		if lang == "" {
			lang = defaultVerificationProbeLanguage
		}
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
	case "javascript":
		return runJavaScriptVerificationProbe(ctx, probe, id, wd, rel, source)
	case "ruby":
		return runRubyVerificationProbe(ctx, probe, id, wd, rel, source)
	case "java":
		return runJavaVerificationProbe(ctx, probe, id, wd, rel, source)
	case "go":
		return runGoVerificationProbe(ctx, probe, id, wd, rel, source)
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

func runJavaScriptVerificationProbe(ctx *types.BusContext, probe types.VerificationProbe, id, wd, rel, source string) verificationProbeRunResult {
	statusPath := newVerificationProbeStatusPath()
	if statusPath != "" {
		defer os.Remove(statusPath)
	}
	execCtx, cmd, timeout, cancel := newVerificationProbeCommand("node", []string{"-e", javascriptVerificationProbeWrapper}, wd, "node", ctx, probe, statusPath)
	defer cancel()
	return runExternalVerificationProbe(ctx, probe, externalVerificationProbeInput{
		ID:          id,
		Language:    "javascript",
		Command:     cmd,
		ExecCtx:     execCtx,
		Timeout:     timeout,
		StatusPath:  statusPath,
		WorkingDir:  rel,
		Source:      source,
		CommandText: "node -e <verification_probe:" + id + ">",
	})
}

func runRubyVerificationProbe(ctx *types.BusContext, probe types.VerificationProbe, id, wd, rel, source string) verificationProbeRunResult {
	statusPath := newVerificationProbeStatusPath()
	if statusPath != "" {
		defer os.Remove(statusPath)
	}
	execCtx, cmd, timeout, cancel := newVerificationProbeCommand("ruby", []string{"-e", rubyVerificationProbeWrapper}, wd, "ruby", ctx, probe, statusPath)
	defer cancel()
	return runExternalVerificationProbe(ctx, probe, externalVerificationProbeInput{
		ID:          id,
		Language:    "ruby",
		Command:     cmd,
		ExecCtx:     execCtx,
		Timeout:     timeout,
		StatusPath:  statusPath,
		WorkingDir:  rel,
		Source:      source,
		CommandText: "ruby -e <verification_probe:" + id + ">",
	})
}

func runJavaVerificationProbe(ctx *types.BusContext, probe types.VerificationProbe, id, wd, rel, source string) verificationProbeRunResult {
	tmpDir, err := createVerificationProbeTempDir(ctx.RepoRoot, "java")
	if err != nil {
		detail := fmt.Sprintf("verification probe %q could not create temp Java source directory: %v", id, err)
		return verificationProbeConfigError(id, "java", rel, source, detail)
	}
	defer os.RemoveAll(tmpDir)
	sourcePath := filepath.Join(tmpDir, "CodraxVerificationProbe.java")
	sourceCode := javaVerificationProbeSource(probe.Code)
	mainClass := javaVerificationProbeMainClass(sourceCode)
	if err := os.WriteFile(sourcePath, []byte(sourceCode), 0o600); err != nil {
		detail := fmt.Sprintf("verification probe %q could not write temp Java source: %v", id, err)
		return verificationProbeConfigError(id, "java", rel, source, detail)
	}

	classPath := javaVerificationProbeClassPath(ctx.RepoRoot, wd, tmpDir)
	sourcePathArg := javaVerificationProbeSourcePath(ctx.RepoRoot, wd)
	javacArgs := []string{"-encoding", "UTF-8", "-cp", classPath, "-sourcepath", sourcePathArg, "-d", tmpDir, sourcePath}
	compileCtx, compileCmd, compileTimeout, compileCancel := newVerificationProbeCommand("javac", javacArgs, wd, "java", ctx, probe, "")
	compileOutput, compileExit, compileDuration, compileExitKind, compileErr := runVerificationProbeCommand(compileCtx, compileCmd, compileTimeout)
	compileCancel()
	if compileErr != nil {
		outcome := "parser_error"
		failureKind := types.FailureKindParserError
		reasonCode := "verification_probe_java_compile_error"
		if verificationProbeRunnerMissing("javac", compileErr, compileOutput) || javaRuntimeMissingOutput(compileOutput) {
			outcome = "runner_missing"
			failureKind = types.FailureKindRunnerMissing
			reasonCode = "verification_probe_runner_missing"
		}
		switch compileExitKind {
		case SupervisedExitTimeout:
			outcome = "timeout"
			failureKind = types.FailureKindTimeout
			reasonCode = ""
		case SupervisedExitOOM:
			outcome = "oom"
			failureKind = types.FailureKindOOM
			reasonCode = ""
		case SupervisedExitCPULimit:
			outcome = "cpu_limit"
			failureKind = types.FailureKindCPULimit
			reasonCode = ""
		}
		detail := strings.TrimSpace(compileOutput)
		if detail == "" {
			detail = fmt.Sprintf("verification probe %q javac exited with code %d", id, compileExit)
		}
		detail = stdoutHead(detail, 4000)
		logging.Info("[run_tests] verification_probe id=%s lang=java cwd=%s compile_outcome=%s exit=%d duration=%v",
			id, rel, outcome, compileExit, compileDuration)
		return verificationProbeRunResult{
			Report: &types.ChangeReport{
				TestResults: []types.TestResult{{
					Kind:          types.TestResultKindUnit,
					AssertionID:   id,
					Suite:         "verification_probe/java",
					Passed:        false,
					Duration:      compileDuration,
					FailureDetail: detail,
				}},
				Passed:            false,
				FailureKind:       failureKind,
				FailureReasonCode: reasonCode,
				FailureSummary:    detail,
			},
			Output: detail,
			Commands: []types.ExecutedCommand{{
				Runner:     "verification_probe",
				Framework:  "java",
				WorkingDir: rel,
				Command:    "javac <verification_probe:" + id + ">",
				ExitCode:   compileExit,
				DurationMS: compileDuration.Milliseconds(),
				Source:     source,
				Outcome:    outcome,
				ReasonCode: reasonCode,
			}},
		}
	}

	runCtx, runCmd, timeout, runCancel := newVerificationProbeCommand("java", []string{"-ea", "-cp", classPath, mainClass}, wd, "java", ctx, probe, "")
	defer runCancel()
	res := runExternalVerificationProbe(ctx, probe, externalVerificationProbeInput{
		ID:          id,
		Language:    "java",
		Command:     runCmd,
		ExecCtx:     runCtx,
		Timeout:     timeout,
		WorkingDir:  rel,
		Source:      source,
		CommandText: "java -ea <verification_probe:" + id + ">",
	})
	res.Commands = append([]types.ExecutedCommand{{
		Runner:     "verification_probe",
		Framework:  "java",
		WorkingDir: rel,
		Command:    "javac <verification_probe:" + id + ">",
		ExitCode:   compileExit,
		DurationMS: compileDuration.Milliseconds(),
		Source:     source,
		Outcome:    "executed",
	}}, res.Commands...)
	return res
}

func runGoVerificationProbe(ctx *types.BusContext, probe types.VerificationProbe, id, wd, rel, source string) verificationProbeRunResult {
	tmp, cleanup, err := createGoVerificationProbeTemp(ctx.RepoRoot)
	if err != nil {
		detail := fmt.Sprintf("verification probe %q could not create temp Go source: %v", id, err)
		return verificationProbeConfigError(id, "go", rel, source, detail)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(probe.Code); err != nil {
		_ = tmp.Close()
		cleanup()
		detail := fmt.Sprintf("verification probe %q could not write temp Go source: %v", id, err)
		return verificationProbeConfigError(id, "go", rel, source, detail)
	}
	_ = tmp.Close()
	defer cleanup()
	execCtx, cmd, timeout, cancel := newVerificationProbeCommand("go", []string{"run", tmpPath}, wd, "go", ctx, probe, "")
	defer cancel()
	return runExternalVerificationProbe(ctx, probe, externalVerificationProbeInput{
		ID:          id,
		Language:    "go",
		Command:     cmd,
		ExecCtx:     execCtx,
		Timeout:     timeout,
		WorkingDir:  rel,
		Source:      source,
		CommandText: "go run <verification_probe:" + id + ">",
	})
}

func createGoVerificationProbeTemp(repoRoot string) (*os.File, func(), error) {
	base := ""
	if strings.TrimSpace(repoRoot) != "" {
		base = filepath.Join(repoRoot, ".codrax", "tmp", "verification-probes")
		if err := os.MkdirAll(base, 0o700); err != nil {
			base = ""
		}
	}
	if base == "" {
		f, err := os.CreateTemp("", "codrax-verification-probe-*.go")
		if err != nil {
			return nil, func() {}, err
		}
		return f, func() { _ = os.Remove(f.Name()) }, nil
	}
	f, err := os.CreateTemp(base, "probe-*.go")
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { _ = os.Remove(f.Name()) }, nil
}

func createVerificationProbeTempDir(repoRoot, language string) (string, error) {
	language = strings.TrimSpace(language)
	if language == "" {
		language = "probe"
	}
	base := ""
	if strings.TrimSpace(repoRoot) != "" {
		base = filepath.Join(repoRoot, ".codrax", "tmp", "verification-probes")
		if err := os.MkdirAll(base, 0o700); err != nil {
			base = ""
		}
	}
	if base == "" {
		return os.MkdirTemp("", "codrax-verification-probe-"+language+"-*")
	}
	return os.MkdirTemp(base, language+"-*")
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
		case "import_error", "syntax_error":
			passed = false
			outcome = "parser_error"
			failureKind = types.FailureKindParserError
			reasonCode = pythonVerificationProbeReasonCode(probeStatus)
		case "exception":
			passed = false
			reasonCode = pythonVerificationProbeReasonCode(probeStatus)
			if pythonVerificationProbeExceptionIsInfrastructure(probeStatus) {
				outcome = "parser_error"
				failureKind = types.FailureKindParserError
			} else {
				failureKind = types.FailureKindTestsFailed
			}
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

type externalVerificationProbeInput struct {
	ID          string
	Language    string
	Command     *exec.Cmd
	ExecCtx     context.Context
	Timeout     time.Duration
	StatusPath  string
	WorkingDir  string
	Source      string
	CommandText string
}

func newVerificationProbeStatusPath() string {
	f, err := os.CreateTemp("", "codrax-verification-probe-*.json")
	if err != nil {
		return ""
	}
	path := f.Name()
	_ = f.Close()
	return path
}

func newVerificationProbeCommand(binary string, args []string, wd, runner string, ctx *types.BusContext, probe types.VerificationProbe, statusPath string) (context.Context, *exec.Cmd, time.Duration, context.CancelFunc) {
	timeout := verificationProbeTimeout(probe)
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	cmd := exec.CommandContext(execCtx, binary, args...)
	cmd.Dir = wd
	env := runnerExecutionEnv(runner, ctx.RepoRoot, wd, ctx.MainRepoRoot)
	if statusPath != "" {
		env = append(env,
			"CODRAX_VERIFICATION_PROBE_CODE="+base64.StdEncoding.EncodeToString([]byte(probe.Code)),
			"CODRAX_VERIFICATION_PROBE_RESULT="+statusPath,
		)
	}
	cmd.Env = env
	return execCtx, cmd, timeout, cancel
}

func runVerificationProbeCommand(execCtx context.Context, cmd *exec.Cmd, timeout time.Duration) (string, int, time.Duration, SupervisedExitKind, error) {
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
	return output, extractExitCode(supRes.Err), duration, supRes.ExitKind, supRes.Err
}

func verificationProbeTimeout(probe types.VerificationProbe) time.Duration {
	timeout := time.Duration(probe.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	return timeout
}

func runExternalVerificationProbe(ctx *types.BusContext, probe types.VerificationProbe, in externalVerificationProbeInput) verificationProbeRunResult {
	var stdout, stderr bytes.Buffer
	in.Command.Stdout = &stdout
	in.Command.Stderr = &stderr
	start := time.Now()
	caps := verifyResourceCaps()
	if timeoutSeconds := uint64(in.Timeout.Seconds()); timeoutSeconds > 0 && caps.CPULimitSeconds > timeoutSeconds {
		caps.CPULimitSeconds = timeoutSeconds
	}
	supRes := SupervisedRun(in.ExecCtx, in.Command, caps)
	duration := time.Since(start)
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}
	exitCode := extractExitCode(supRes.Err)
	status := readInlineVerificationProbeStatus(in.StatusPath)
	outcome := "executed"
	failureKind := types.FailureKindTestsFailed
	reasonCode := ""
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
		if verificationProbeRunnerMissing(in.Command.Path, supRes.Err, output) {
			outcome = "runner_missing"
			failureKind = types.FailureKindRunnerMissing
			reasonCode = "verification_probe_runner_missing"
		}
	}
	passed := supRes.Err == nil
	if outcome == "executed" && status.Outcome != "" {
		switch status.Outcome {
		case "passed":
			passed = passed && status.ExitCode == 0
		case "assertion_failed", "system_exit":
			passed = false
			failureKind = types.FailureKindTestsFailed
		case "import_error", "syntax_error", "reference_error":
			passed = false
			outcome = "parser_error"
			failureKind = types.FailureKindParserError
			reasonCode = inlineVerificationProbeReasonCode(in.Language, status)
		case "exception":
			passed = false
			failureKind = types.FailureKindTestsFailed
			reasonCode = inlineVerificationProbeReasonCode(in.Language, status)
		default:
			passed = false
			outcome = "parser_error"
			failureKind = types.FailureKindParserError
			reasonCode = inlineVerificationProbeReasonCode(in.Language, status)
		}
	}
	if in.Language == "go" && outcome == "executed" && supRes.Err != nil && looksLikeGoProbeCompileError(output) {
		outcome = "parser_error"
		failureKind = types.FailureKindParserError
		reasonCode = "verification_probe_go_compile_error"
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
			failureKind = types.FailureKindTestsFailed
		}
	}
	if passed {
		failureKind = ""
		reasonCode = ""
	}
	detail := ""
	if !passed {
		detail = stdoutHead(output, 4000)
		if status.Outcome != "" && status.Outcome != "passed" {
			if detail != "" {
				detail += "\n"
			}
			detail += fmt.Sprintf("verification probe structured outcome: %s", status.Outcome)
			if status.Exception != "" {
				detail += " (" + status.Exception + ")"
			}
		}
		if len(missingExpected) > 0 {
			if detail != "" {
				detail += "\n"
			}
			detail += "missing expected stdout fragments: " + strings.Join(missingExpected, ", ")
		}
		if detail == "" {
			detail = fmt.Sprintf("verification probe %q exited with code %d", in.ID, exitCode)
		}
	}
	logging.Info("[run_tests] verification_probe id=%s lang=%s cwd=%s outcome=%s exit=%d duration=%v",
		in.ID, in.Language, in.WorkingDir, outcome, exitCode, duration)
	return verificationProbeRunResult{
		Report: &types.ChangeReport{
			TestResults: []types.TestResult{{
				Kind:          types.TestResultKindUnit,
				AssertionID:   in.ID,
				Suite:         "verification_probe/" + in.Language,
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
			Framework:  in.Language,
			WorkingDir: in.WorkingDir,
			Command:    in.CommandText,
			ExitCode:   exitCode,
			DurationMS: duration.Milliseconds(),
			Source:     in.Source,
			Outcome:    outcome,
			ReasonCode: reasonCode,
		}},
	}
}

func verificationProbeConfigError(id, language, rel, source, detail string) verificationProbeRunResult {
	return verificationProbeRunResult{
		Report: &types.ChangeReport{
			TestResults: []types.TestResult{{
				Kind:          types.TestResultKindUnit,
				AssertionID:   id,
				Suite:         "verification_probe/" + language,
				Passed:        false,
				FailureDetail: detail,
			}},
			Passed:         false,
			FailureKind:    types.FailureKindParserError,
			FailureSummary: detail,
		},
		Output: detail,
		Commands: []types.ExecutedCommand{{
			Runner:     "verification_probe",
			Framework:  language,
			WorkingDir: rel,
			Source:     source,
			Outcome:    "probe_config_error",
		}},
	}
}

func verificationProbeRunnerMissing(binary string, runErr error, output string) bool {
	binary = strings.TrimSpace(filepath.Base(binary))
	if binary == "" {
		return false
	}
	if runErr != nil {
		if errors.Is(runErr, exec.ErrNotFound) {
			return true
		}
		if exitCode := extractExitCode(runErr); exitCode == 127 || exitCode == 126 {
			return true
		}
	}
	return outputIndicatesMissingBinary(output, binary) || ((binary == "java" || binary == "javac") && javaRuntimeMissingOutput(output))
}

func javaVerificationProbeSource(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return "public final class CodraxVerificationProbe { public static void main(String[] args) {} }\n"
	}
	if strings.Contains(code, "class CodraxVerificationProbe") {
		return code + "\n"
	}
	var imports []string
	var body []string
	seenBody := false
	for _, line := range strings.Split(code, "\n") {
		trimmed := strings.TrimSpace(line)
		if !seenBody && (trimmed == "" || strings.HasPrefix(trimmed, "import ")) {
			if trimmed != "" {
				imports = append(imports, line)
			}
			continue
		}
		seenBody = true
		body = append(body, line)
	}
	var b strings.Builder
	for _, line := range imports {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("public final class CodraxVerificationProbe {\n")
	b.WriteString("  public static void main(String[] args) throws Exception {\n")
	for _, line := range body {
		if strings.TrimSpace(line) == "" {
			b.WriteByte('\n')
			continue
		}
		b.WriteString("    ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}

func javaVerificationProbeMainClass(source string) string {
	if pkg := javaPackageDeclaration(source); pkg != "" {
		return pkg + ".CodraxVerificationProbe"
	}
	return "CodraxVerificationProbe"
}

func javaVerificationProbeClassPath(repoRoot, wd, tmpDir string) string {
	var entries []string
	seen := map[string]struct{}{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		key := cleanAbsPathKey(path)
		if key == "" {
			key = path
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		entries = append(entries, path)
	}
	add(tmpDir)
	for _, root := range []string{wd, repoRoot} {
		add(root)
		for _, rel := range []string{
			"src/main/java",
			"src/test/java",
			"target/classes",
			"target/test-classes",
			"build/classes/java/main",
			"build/classes/java/test",
			"build/classes/kotlin/main",
			"build/classes/kotlin/test",
			"out/production/classes",
			"out/test/classes",
		} {
			add(filepath.Join(root, rel))
		}
	}
	return strings.Join(entries, string(os.PathListSeparator))
}

func javaVerificationProbeSourcePath(repoRoot, wd string) string {
	var entries []string
	seen := map[string]struct{}{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		key := cleanAbsPathKey(path)
		if key == "" {
			key = path
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		entries = append(entries, path)
	}
	for _, root := range []string{wd, repoRoot} {
		add(root)
		for _, rel := range []string{"src/main/java", "src/test/java"} {
			add(filepath.Join(root, rel))
		}
	}
	return strings.Join(entries, string(os.PathListSeparator))
}

func javaRuntimeMissingOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "unable to locate a java runtime") ||
		strings.Contains(lower, "could not find java") ||
		strings.Contains(lower, "no java runtime present")
}

func readInlineVerificationProbeStatus(path string) inlineVerificationProbeStatus {
	path = strings.TrimSpace(path)
	if path == "" {
		return inlineVerificationProbeStatus{}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return inlineVerificationProbeStatus{}
	}
	var status inlineVerificationProbeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return inlineVerificationProbeStatus{}
	}
	status.Outcome = strings.TrimSpace(status.Outcome)
	status.Exception = strings.TrimSpace(status.Exception)
	return status
}

func inlineVerificationProbeReasonCode(language string, status inlineVerificationProbeStatus) string {
	outcome := strings.TrimSpace(status.Outcome)
	exception := strings.TrimSpace(status.Exception)
	if outcome == "" {
		return "verification_probe_unclassified"
	}
	switch outcome {
	case "import_error":
		return "verification_probe_import_error"
	case "syntax_error":
		return "verification_probe_syntax_error"
	case "reference_error":
		return "verification_probe_reference_error"
	case "exception":
		if exception != "" {
			return "verification_probe_exception"
		}
		return "verification_probe_runtime_exception"
	default:
		return "verification_probe_" + language + "_" + strings.ReplaceAll(outcome, "-", "_")
	}
}

func looksLikeGoProbeCompileError(output string) bool {
	for _, marker := range []string{
		"# command-line-arguments",
		"syntax error:",
		"undefined:",
		"expected '",
		"expected operand",
		"missing import path",
	} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
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
		if status.ProbeTopLevel {
			switch exception {
			case "NameError":
				return "verification_probe_name_error"
			default:
				return "verification_probe_top_level_exception"
			}
		}
		switch exception {
		case "NameError":
			return "verification_probe_name_error"
		}
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

func pythonVerificationProbeExceptionIsInfrastructure(status pythonVerificationProbeStatus) bool {
	if !status.ProbeTopLevel {
		return false
	}
	return true
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
		if normalized, ok := normalizeVerificationProbeLanguage(lang); ok {
			lang = normalized
		}
		if lang == "" {
			lang = defaultVerificationProbeLanguage
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
