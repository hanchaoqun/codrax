package tool

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_worktree_drift.go — V5-2 (colleague_merge_audit §40.11): the
// tracked-drift gate's typed owner classification and its escape lanes.
//
// Inputs are typed facts only: the executed-runner roster (ExecutedCommand
// rows with Outcome=executed), the closed per-runner side-effect manifest,
// the plan's output_path behavior contracts, and byte-exact witnesses (git
// blob of the pre-run content, formatter output, locked re-run snapshots).
// No path prose, runner output text or model rationale participates.

// verificationWorktreeDriftInput is what the classifier may read.
type verificationWorktreeDriftInput struct {
	plan     *types.ChangePlan
	executed []types.ExecutedCommand
	repoRoot string
	mainRoot string
	caps     SupervisedRunOptions
	timeout  time.Duration
}

type verificationLockedReverifyRequest struct {
	Runner, Framework, WorkingDir, Command string
	Env                                    []string
	Timeout                                time.Duration
	Caps                                   SupervisedRunOptions
	AuditRoot                              string
}

type verificationLockedReverifyResult struct {
	ExitCode     int
	DriftedPaths []string
	Unavailable  bool
}

// Test seams: nil ⇒ real runner / real formatter.
var (
	verificationLockedReverifyHook func(req verificationLockedReverifyRequest) verificationLockedReverifyResult
	verificationFormatterHook      func(argv []string, input []byte) ([]byte, bool)
)

type verificationWorktreeDriftDecision struct {
	class       types.VerificationWorktreeDriftClass
	ownerRunner string
	workingDir  string
	framework   string
	suite       string
}

type verificationDriftRosterEntry struct {
	runner, framework, suite string
	dirRel                   string // relative to the audit root, "." for root
	// suiteInfraOutcome is the launched-outcome label (timeout | oom |
	// cpu_limit) when the primary suite of this (runner, workdir) was cut
	// short by an infrastructure cap; "" otherwise. The locked re-verify is
	// not attempted for such an owner (it would die under the same caps) and
	// its lockfile rows are disclosed with the fixed point marked UNPROVEN.
	suiteInfraOutcome string
	// suiteExitFailed is the seat's own typed failure fact: a launched,
	// non-infra command row of this (runner, workdir) exited non-zero. The
	// locked re-verify decision keys on the OWNER SEAT (infra outcome first,
	// then this flag), never on the report-level verdict — a report that is
	// not Passed for coverage reasons (verification_incomplete) still gets
	// its cheap locked witness when the seat itself exited 0.
	suiteExitFailed bool
}

// key identifies the (runner, workdir) owner seat.
func (e verificationDriftRosterEntry) key() string { return e.runner + "\x00" + e.dirRel }

// classifyVerificationWorktreeDrift decides the class of one tracked effect.
func classifyVerificationWorktreeDrift(effect types.VerificationWorktreeEffect, in verificationWorktreeDriftInput, baseline verificationWorktreeSnapshot, roster []verificationDriftRosterEntry, declared map[string]bool) verificationWorktreeDriftDecision {
	unclassified := verificationWorktreeDriftDecision{class: types.VerificationWorktreeDriftUnclassified}
	rel := normalizeVerificationWorktreePath(effect.Path)
	if rel == "" || effect.Kind != types.VerificationWorktreeEffectTrackedChanged {
		return unclassified
	}
	manifests := runnerSideEffectManifests()
	base := path.Base(rel)
	dir := path.Dir(rel)
	// (a) dependency lockfile owned by an executed runner: the toolchain reads
	// the NEAREST lockfile of that basename at or above its working dir, so
	// only that directory's file is owned — a same-named lockfile further up
	// belongs to another project and stays unclassified.
	for _, entry := range roster {
		manifest := manifests[entry.runner]
		if !manifest.hasLockedLane() || !verificationDriftListContains(manifest.LockfileBasenames, base) {
			continue
		}
		if owner, ok := verificationDriftNearestLockfileDir(baseline.root, base, entry.dirRel); ok && owner == strings.Trim(path.Clean("/"+dir), "/") {
			return verificationWorktreeDriftDecision{class: types.VerificationWorktreeDriftDependencyLockfileRefresh,
				ownerRunner: entry.runner, workingDir: entry.dirRel, framework: entry.framework, suite: entry.suite}
		}
	}
	// (b) formatter fixed point: the path was clean at baseline, an executed
	// runner's language formatter owns its extension, and formatting the
	// pre-run blob reproduces the post-run bytes exactly.
	if _, dirtyAtBaseline := baseline.tracked[rel]; !dirtyAtBaseline {
		ext := strings.ToLower(path.Ext(rel))
		for _, entry := range roster {
			manifest := manifests[entry.runner]
			if len(manifest.Formatter) == 0 || !verificationDriftListContains(manifest.FormatterExts, ext) || !verificationDriftDirOwns(entry.dirRel, dir) {
				continue
			}
			if verificationDriftFormatterFixedPoint(in, baseline.root, rel, entry, manifest.Formatter) {
				return verificationWorktreeDriftDecision{class: types.VerificationWorktreeDriftFormatterNoSemanticDiff, ownerRunner: entry.runner, workingDir: entry.dirRel}
			}
		}
	}
	// (c) plan-declared generated output (typed output_path contract).
	if declared[verificationDriftRepoRelative(baseline.root, in.repoRoot, rel)] {
		return verificationWorktreeDriftDecision{class: types.VerificationWorktreeDriftDeclaredGeneratedOutput}
	}
	return unclassified
}

// verificationDriftNearestLockfileDir walks from the executed working dir
// (audit-root relative) upward and returns the first directory holding a
// file named `base` — the one the toolchain actually reads. Directories are
// returned trimmed ("" = audit root).
func verificationDriftNearestLockfileDir(auditRoot, base, execDir string) (string, bool) {
	dir := strings.Trim(path.Clean("/"+execDir), "/")
	for {
		if _, err := os.Stat(filepath.Join(auditRoot, filepath.FromSlash(dir), base)); err == nil {
			return dir, true
		}
		if dir == "" {
			return "", false
		}
		parent := path.Dir(dir)
		if parent == "." || parent == "/" {
			parent = ""
		}
		dir = parent
	}
}

// verificationDriftDirOwns reports whether `ancestor` (audit-root relative,
// "." = root) is `dir` itself or one of its ancestors.
func verificationDriftDirOwns(ancestor, dir string) bool {
	ancestor = strings.Trim(path.Clean("/"+ancestor), "/")
	dir = strings.Trim(path.Clean("/"+dir), "/")
	if ancestor == "" {
		return true
	}
	return dir == ancestor || strings.HasPrefix(dir, ancestor+"/")
}

func verificationDriftRepoRelative(auditRoot, repoRoot, rel string) string {
	abs := filepath.Join(auditRoot, filepath.FromSlash(rel))
	repoAbs, err := filepath.Abs(strings.TrimSpace(repoRoot))
	if err != nil {
		return rel
	}
	out, err := filepath.Rel(repoAbs, abs)
	if err != nil {
		return rel
	}
	return filepath.ToSlash(out)
}

// verificationDriftDeclaredOutputs collects the plan's typed output_path
// contract paths (Subject / Expected that parse as repo-relative paths).
func verificationDriftDeclaredOutputs(plan *types.ChangePlan) map[string]bool {
	out := map[string]bool{}
	if plan == nil {
		return out
	}
	for _, contract := range types.ChangePlanVerificationBehaviorContracts(plan) {
		// Only a contract the verifier itself treats as an authoritative,
		// expected-polarity declaration that the path exists declares a
		// generated output; forbidden/observed/planning-only contracts and
		// content operands never grant ownership.
		if contract.Kind != types.WriteBehaviorOutputPath || !types.IsHardRequiredWriteBehaviorContract(contract) ||
			contract.Polarity == types.WriteBehaviorPolarityObserved {
			continue
		}
		switch contract.Operator {
		case types.WriteBehaviorOpExists, types.WriteBehaviorOpEquals, types.WriteBehaviorOpContains:
		default:
			continue
		}
		if rel, ok := postApplySourceContractRepoPath(contract.Subject); ok {
			out[rel] = true
		}
	}
	return out
}

// verificationDriftRoster derives the executed-runner roster from typed
// ExecutedCommand rows (Outcome=executed), with working dirs expressed
// relative to the audit root.
func verificationDriftRoster(in verificationWorktreeDriftInput, auditRoot string) []verificationDriftRosterEntry {
	var out []verificationDriftRosterEntry
	seen := map[string]int{}
	for _, cmd := range in.executed {
		if !verificationDriftCommandLaunched(cmd) {
			continue
		}
		runner := strings.TrimSpace(cmd.Runner)
		if runner == "" {
			continue
		}
		abs := filepath.Join(strings.TrimSpace(in.repoRoot), filepath.FromSlash(normalizeRunTestsWorkingDir(cmd.WorkingDir)))
		rel, err := filepath.Rel(auditRoot, abs)
		if err != nil {
			continue
		}
		dirRel := filepath.ToSlash(rel)
		if dirRel == "" || dirRel == ".." || strings.HasPrefix(dirRel, "../") {
			continue
		}
		key := runner + "\x00" + dirRel
		infra := verificationDriftCommandSuiteInfraOutcome(cmd)
		exitFailed := infra == "" && cmd.ExitCode != 0
		if idx, ok := seen[key]; ok {
			// Same owner seat launched more than once (syntax preflight +
			// suite, escalations, the pre-suite continuation preview row):
			// an infra-downgraded launch marks the seat, and any non-zero
			// launched exit marks it failed — the seat's facts aggregate over
			// its rows in precedence order, so a preview row (exit 0) never
			// hides the real suite's failure.
			if infra != "" && out[idx].suiteInfraOutcome == "" {
				out[idx].suiteInfraOutcome = infra
			}
			if exitFailed {
				out[idx].suiteExitFailed = true
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, verificationDriftRosterEntry{runner: runner, framework: strings.TrimSpace(cmd.Framework), suite: strings.TrimSpace(cmd.Suite), dirRel: dirRel, suiteInfraOutcome: infra, suiteExitFailed: exitFailed})
	}
	return out
}

// verificationDriftLaunchedOutcomes is the closed "process was started in
// the audited worktree" roster; verificationDriftNotLaunchedOutcomes is its
// exact complement (nothing ran in the audit root: synthetic / skipped /
// refused / preflight-only rows, and the main-snapshot baseline evidence
// rows, which execute against the immutable MAIN snapshot outside the
// audited worktree); verificationDriftSuiteInfraOutcomes is the launched
// roster's infrastructure-downgrade subset (the supervisor killed the suite:
// wall timeout / memory cap / CPU cap — the same three kinds
// makeResourceExhaustionReport types). All tables are built from the typed
// types.ExecutedCommandOutcome* labels the producers write — never from
// parallel literals — and are census-pinned (run_tests_outcome_census_test.go
// reads the writers through go/ast and pins launched ∪ not-launched ==
// AllExecutedCommandOutcomes, disjoint, infra ⊆ launched) so a renamed
// label, a member classified in neither table, or an outcome added to one
// table without the other goes red.
var (
	verificationDriftLaunchedOutcomes = []string{
		types.ExecutedCommandOutcomeExecuted, types.ExecutedCommandOutcomeSuiteContinued, types.ExecutedCommandOutcomeParserError,
		types.ExecutedCommandOutcomeExpectedStdoutMissing, types.ExecutedCommandOutcomeZeroTests,
		types.ExecutedCommandOutcomeTimeout, types.ExecutedCommandOutcomeOOM, types.ExecutedCommandOutcomeCPULimit,
	}
	verificationDriftNotLaunchedOutcomes = []string{
		types.ExecutedCommandOutcomeSyntheticNoTests, types.ExecutedCommandOutcomeSyntaxCheckFallback,
		types.ExecutedCommandOutcomeSyntaxPreflight, types.ExecutedCommandOutcomeSuiteSkipped,
		types.ExecutedCommandOutcomeRunnerMissing, types.ExecutedCommandOutcomeNotConfigured,
		types.ExecutedCommandOutcomeProbeConfigError, types.ExecutedCommandOutcomeExpectedFailureObserved,
		types.ExecutedCommandOutcomeExpectedFailureNotObserved, types.ExecutedCommandOutcomeBaselineUnavailable,
	}
	verificationDriftSuiteInfraOutcomes = []string{types.ExecutedCommandOutcomeTimeout, types.ExecutedCommandOutcomeOOM, types.ExecutedCommandOutcomeCPULimit}
)

// verificationDriftCommandLaunched reports whether the runner process was
// actually started in its working dir — the typed fact the roster needs. The
// post-hoc verdict label (parser_error, zero_tests, timeout, …) does not
// undo the launch: a process that ran may have refreshed its lockfile.
func verificationDriftCommandLaunched(cmd types.ExecutedCommand) bool {
	return verificationDriftListContains(verificationDriftLaunchedOutcomes, strings.TrimSpace(cmd.Outcome))
}

// verificationDriftCommandSuiteInfraOutcome returns the launched outcome when
// the supervisor cut the suite short (timeout | oom | cpu_limit), "" otherwise.
func verificationDriftCommandSuiteInfraOutcome(cmd types.ExecutedCommand) string {
	outcome := strings.TrimSpace(cmd.Outcome)
	if verificationDriftListContains(verificationDriftSuiteInfraOutcomes, outcome) {
		return outcome
	}
	return ""
}

// verificationDriftFormatterFixedPoint proves formatter_no_semantic_diff:
// format(git HEAD blob) == current bytes. Any unavailability ⇒ false. The
// formatter runs in the file's own directory with the runner's execution
// environment, so project formatter configuration and venv-local binaries
// apply exactly as they do for the runner.
func verificationDriftFormatterFixedPoint(in verificationWorktreeDriftInput, auditRoot, rel string, entry verificationDriftRosterEntry, formatter []string) bool {
	if len(formatter) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	show := exec.CommandContext(ctx, "git", "show", "HEAD:"+rel)
	show.Dir = auditRoot
	pre, err := show.Output()
	if err != nil {
		return false
	}
	current, err := os.ReadFile(filepath.Join(auditRoot, filepath.FromSlash(rel)))
	if err != nil {
		return false
	}
	format := verificationFormatterHook
	if format == nil {
		workDir := filepath.Join(auditRoot, filepath.FromSlash(entry.dirRel))
		fileDir := filepath.Join(auditRoot, filepath.FromSlash(path.Dir(rel)))
		env := runnerExecutionEnv(entry.runner, in.repoRoot, workDir, in.mainRoot)
		format = func(argv []string, input []byte) ([]byte, bool) {
			return verificationDriftRunFormatter(argv, input, fileDir, env)
		}
	}
	formatted, ok := format(formatter, pre)
	return ok && bytes.Equal(formatted, current)
}

func verificationDriftRunFormatter(argv []string, input []byte, dir string, env []string) ([]byte, bool) {
	if len(argv) == 0 {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Resolve the binary BEFORE constructing the command: exec.CommandContext
	// stamps cmd.Err=ErrNotFound when the bare name is absent from the codrax
	// process PATH, and assigning cmd.Path afterwards does not clear it — the
	// formatter would never start even though the runner env can find it.
	var binary string
	if len(env) > 0 {
		resolved, err := lookPathInEnv(argv[0], env)
		if err != nil {
			return nil, false
		}
		binary = resolved
	} else {
		resolved, err := exec.LookPath(argv[0])
		if err != nil {
			return nil, false
		}
		binary = resolved
	}
	cmd := exec.CommandContext(ctx, binary, argv[1:]...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = env
	}
	cmd.Stdin = bytes.NewReader(input)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}

// runVerificationLockedReverify re-runs one executed runner in locked mode
// after a lockfile refresh and reports whether the refreshed lockfile is a
// fixed point (exit 0 and no tracked drift).
func runVerificationLockedReverify(in verificationWorktreeDriftInput, auditRoot string, entry verificationDriftRosterEntry) types.VerificationLockedReverify {
	record := types.VerificationLockedReverify{Runner: entry.runner, Framework: entry.framework, WorkingDir: entry.dirRel}
	cmd, env, ok := buildLockedRunCommand(entry.runner, entry.framework, entry.suite, in.repoRoot, in.mainRoot)
	if !ok {
		record.Outcome = types.VerificationLockedReverifyUnavailable
		record.ReasonCode = types.VerificationLockedReverifyFailedReason
		return record
	}
	record.Command = cmd
	req := verificationLockedReverifyRequest{Runner: entry.runner, Framework: entry.framework, WorkingDir: entry.dirRel,
		Command: cmd, Env: env, Timeout: in.timeout, Caps: in.caps, AuditRoot: auditRoot}
	run := verificationLockedReverifyHook
	if run == nil {
		run = verificationDriftExecuteLocked(in)
	}
	result := run(req)
	record.ExitCode = result.ExitCode
	record.DriftedPaths = result.DriftedPaths
	switch {
	case result.Unavailable:
		record.Outcome = types.VerificationLockedReverifyUnavailable
		record.ReasonCode = types.VerificationLockedReverifyFailedReason
	case result.ExitCode != 0:
		record.Outcome = types.VerificationLockedReverifyFailed
		record.ReasonCode = types.VerificationLockedReverifyFailedReason
	case len(result.DriftedPaths) > 0:
		record.Outcome = types.VerificationLockedReverifyDriftRecurred
		record.ReasonCode = types.VerificationLockedReverifyFailedReason
	default:
		record.Outcome = types.VerificationLockedReverifyPassed
	}
	return record
}

// verificationLockedReverifyRecordForOwner decides, from the OWNER SEAT's
// typed facts only (F-run-tests round three, finding B), whether the locked
// re-run is executed for one lockfile owner and returns its record plus the
// typed fixed-point state its rows carry, in this precedence:
//   - owner suite infra-downgraded (timeout | oom | cpu_limit)
//     ⇒ skipped_suite_infra_downgraded / unproven_suite_infra_downgraded
//     (never refused, never re-run under the caps that just killed it;
//     evaluated BEFORE any other check so a cut-short suite is never
//     called "failed")
//   - owner seat exited non-zero ⇒ skipped_report_failed (published wire
//     bytes) / unproven_suite_failed ("the owner suite exited non-zero")
//   - owner seat exited 0 ⇒ the locked re-run executes — also when the
//     report-level verdict is not Passed for reasons that are not the
//     seat's (changed-path coverage → verification_incomplete, another
//     seat's failure) and for a Passed zero-test report (cheap
//     lockfile-only witness).
//
// The report-level verdict never participates: keying on report.Passed told
// a passing suite whose changed path was merely uncovered that "the test
// suite failed", and evaluated before the infra outcome it mislabelled a
// timed-out suite the same way.
func verificationLockedReverifyRecordForOwner(in verificationWorktreeDriftInput, auditRoot string, owner verificationDriftRosterEntry) (types.VerificationLockedReverify, types.VerificationLockfileFixedPoint) {
	record := types.VerificationLockedReverify{Runner: owner.runner, Framework: owner.framework, WorkingDir: owner.dirRel}
	switch {
	case owner.suiteInfraOutcome != "":
		record.Outcome = types.VerificationLockedReverifySkippedSuiteInfraDowngraded
		record.SuiteOutcome = owner.suiteInfraOutcome
		return record, types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded
	case owner.suiteExitFailed:
		record.Outcome = types.VerificationLockedReverifySkippedReportFailed
		return record, types.VerificationLockfileFixedPointUnprovenSuiteFailed
	}
	record = runVerificationLockedReverify(in, auditRoot, owner)
	if record.Outcome == types.VerificationLockedReverifyPassed {
		return record, types.VerificationLockfileFixedPointProven
	}
	return record, types.VerificationLockfileFixedPointDisproven
}

func verificationDriftExecuteLocked(in verificationWorktreeDriftInput) func(req verificationLockedReverifyRequest) verificationLockedReverifyResult {
	return func(req verificationLockedReverifyRequest) verificationLockedReverifyResult {
		workDir := filepath.Join(req.AuditRoot, filepath.FromSlash(req.WorkingDir))
		before := captureVerificationWorktreeSnapshot(context.Background(), req.AuditRoot)
		if !before.applicable || before.unavailable {
			return verificationLockedReverifyResult{Unavailable: true}
		}
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = runTestsDefaultTimeout()
		}
		execCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := NewShellCommandContext(execCtx, wrapShellCommandWithCaps(req.Command, req.Caps))
		cmd.Dir = workDir
		cmd.Env = append(runnerExecutionEnv(req.Runner, in.repoRoot, workDir, in.mainRoot), req.Env...)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		logging.Info("[run_tests] worktree_audit locked re-verify exec: %s (cwd=%s timeout=%v)", req.Command, workDir, timeout)
		supRes := SupervisedRun(execCtx, cmd, req.Caps)
		exitCode := extractExitCode(supRes.Err)
		logging.Info("[run_tests] worktree_audit locked re-verify exit=%d output_bytes=%d excerpt=%q", exitCode, buf.Len(), truncateForLog(buf.String(), 300))
		after := captureVerificationWorktreeSnapshot(context.Background(), req.AuditRoot)
		if !after.applicable || after.unavailable {
			return verificationLockedReverifyResult{ExitCode: exitCode, Unavailable: true}
		}
		var drifted []string
		for _, effect := range verificationWorktreeEffects(before, after) {
			if effect.Kind == types.VerificationWorktreeEffectTrackedChanged {
				drifted = append(drifted, effect.Path)
			}
		}
		sort.Strings(drifted)
		return verificationLockedReverifyResult{ExitCode: exitCode, DriftedPaths: drifted}
	}
}

func verificationDriftListContains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// lookPathInEnv resolves a formatter binary against the PATH of the runner
// environment rather than the codrax process environment.
func lookPathInEnv(name string, env []string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		return name, nil
	}
	pathValue := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			pathValue = strings.TrimPrefix(kv, "PATH=")
		}
	}
	if pathValue == "" {
		return exec.LookPath(name)
	}
	for _, dir := range filepath.SplitList(pathValue) {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}
