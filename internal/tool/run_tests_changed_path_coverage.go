package tool

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const changedPathVerificationUncoveredReasonCode = "changed_path_verification_uncovered"

type changedPathCoverageEvidence struct {
	caliber types.ChangedPathVerificationCaliber
	runner  string
	source  string
}

// applyChangedPathVerificationCoverage closes the report-level authority gap
// where a successful runner for one language used to authorize unrelated
// changed source paths. It consumes only typed plan paths, runner identities,
// working directories, exact CoveredPaths and probe metadata.
func applyChangedPathVerificationCoverage(ctx *types.BusContext, report *types.ChangeReport) {
	if ctx == nil || ctx.Mutable == nil || report == nil {
		return
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return
	}
	targetPaths, _ := types.ActiveChangePlanApplyTargetPaths(plan, ctx.Mutable.WriteWorkflowRun())
	targets, targetFamilies := recognizedChangedSourcePaths(targetPaths)
	if len(targets) == 0 {
		report.ChangedPathCoverage = nil
		return
	}

	report.ExecutedCommands = enrichSuccessfulCommandCoveredPaths(
		report.ExecutedCommands, targets, targetFamilies, report.TestSurface,
	)
	evidence := changedPathCoverageFromCommands(
		report.ExecutedCommands, targets, targetFamilies, report.TestSurface,
	)
	for path, probeEvidence := range changedPathCoverageFromPassedProbes(plan, report, targets, targetFamilies) {
		if existing, ok := evidence[path]; !ok || changedPathCoverageCaliberRank(probeEvidence.caliber) > changedPathCoverageCaliberRank(existing.caliber) {
			evidence[path] = probeEvidence
		}
	}

	rows := make([]types.ChangedPathVerificationCoverage, 0, len(targets))
	var uncovered []string
	for _, path := range targets {
		row := types.ChangedPathVerificationCoverage{
			Path:             path,
			LanguageFamilies: append([]types.VerificationLanguageFamily(nil), targetFamilies[path]...),
		}
		if proof, ok := evidence[path]; ok {
			row.Status = types.ChangedPathVerificationCovered
			row.Caliber = proof.caliber
			row.Runner = proof.runner
			row.Source = proof.source
		} else {
			row.Status = types.ChangedPathVerificationUncovered
			row.ReasonCode = changedPathVerificationUncoveredReasonCode
			uncovered = append(uncovered, path)
		}
		rows = append(rows, row)
	}
	report.ChangedPathCoverage = rows

	// Preserve real red tests/builds and already-unavailable runs. This gate
	// only prevents a nominally verified pass from being signed by evidence
	// that does not cover every changed source path.
	if len(uncovered) == 0 || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		return
	}
	report.Passed = false
	report.BuildFailed = false
	report.FailureKind = types.FailureKindVerificationIncomplete
	report.FailureReasonCode = changedPathVerificationUncoveredReasonCode
	report.FailureSummary = changedPathVerificationFailureSummary(uncovered)
	pathRefs := make([]string, 0, len(uncovered))
	for _, path := range uncovered {
		pathRefs = append(pathRefs, "path:"+path)
	}
	report.VerificationConfidence = mergeVerificationConfidenceRecords(
		report.VerificationConfidence,
		[]types.VerificationConfidenceRecord{{
			Source:            "changed_path_coverage",
			Category:          "changed_path_coverage",
			Status:            "unavailable",
			Severity:          "error",
			ReasonCode:        changedPathVerificationUncoveredReasonCode,
			ChangedSymbolRefs: pathRefs,
			Detail:            "successful verification evidence did not cover every recognized changed source path",
		}},
	)
}

func recognizedChangedSourcePaths(paths []string) ([]string, map[string][]types.VerificationLanguageFamily) {
	seen := map[string]bool{}
	familiesByPath := map[string][]types.VerificationLanguageFamily{}
	var out []string
	for _, raw := range paths {
		path := cleanRepoRelPath(raw)
		if path == "" {
			continue
		}
		families := sourceVerificationLanguageFamilies(types.VerificationLanguageFamiliesFromPath(path))
		if len(families) == 0 {
			continue
		}
		key := strings.ToLower(path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, path)
		familiesByPath[path] = families
	}
	return out, familiesByPath
}

func sourceVerificationLanguageFamilies(in []types.VerificationLanguageFamily) []types.VerificationLanguageFamily {
	var out []types.VerificationLanguageFamily
	for _, family := range types.NormalizeVerificationLanguageFamilies(in) {
		if family == types.VerificationLanguageConfigWorkflow {
			continue
		}
		out = append(out, family)
	}
	return out
}

func enrichSuccessfulCommandCoveredPaths(
	commands []types.ExecutedCommand,
	targets []string,
	targetFamilies map[string][]types.VerificationLanguageFamily,
	surface *types.TestSurface,
) []types.ExecutedCommand {
	out := append([]types.ExecutedCommand(nil), commands...)
	for i := range out {
		cmd := &out[i]
		cmd.CoveredPaths = normalizeCoveredTargetPaths(cmd.CoveredPaths, targets)
		outcome := strings.TrimSpace(cmd.Outcome)
		if outcome != "executed" || cmd.ExitCode != 0 || strings.TrimSpace(cmd.Runner) == "verification_probe" {
			continue
		}
		runnerFamilies := sourceVerificationLanguageFamilies(
			types.VerificationLanguageFamiliesFromRunner(cmd.Runner, cmd.Framework),
		)
		if len(runnerFamilies) == 0 {
			continue
		}
		for _, path := range targets {
			if !repoPathWithinWorkingDir(path, cmd.WorkingDir) ||
				(!verificationLanguageFamiliesIntersect(targetFamilies[path], runnerFamilies) &&
					!typedPolyglotProjectRunnerCommand(cmd, surface)) {
				continue
			}
			cmd.CoveredPaths = appendUniqueRepoPath(cmd.CoveredPaths, path)
		}
	}
	return out
}

func changedPathCoverageFromCommands(
	commands []types.ExecutedCommand,
	targets []string,
	targetFamilies map[string][]types.VerificationLanguageFamily,
	surface *types.TestSurface,
) map[string]changedPathCoverageEvidence {
	targetSet := repoPathKeySet(targets)
	out := map[string]changedPathCoverageEvidence{}
	for _, cmd := range commands {
		outcome := strings.TrimSpace(cmd.Outcome)
		var caliber types.ChangedPathVerificationCaliber
		switch {
		case outcome == "executed" && cmd.ExitCode == 0 && strings.TrimSpace(cmd.Runner) != "verification_probe":
			caliber = types.ChangedPathVerificationProjectRunner
		case (outcome == "syntax_check_fallback" || outcome == "syntax_preflight") && cmd.ExitCode == 0:
			caliber = types.ChangedPathVerificationSourceCheck
		default:
			continue
		}
		runnerFamilies := sourceVerificationLanguageFamilies(
			types.VerificationLanguageFamiliesFromRunner(cmd.Runner, cmd.Framework),
		)
		for _, raw := range cmd.CoveredPaths {
			path := cleanRepoRelPath(raw)
			canonical, ok := targetSet[strings.ToLower(path)]
			if !ok ||
				(!verificationLanguageFamiliesIntersect(targetFamilies[canonical], runnerFamilies) &&
					!typedPolyglotProjectRunnerCommand(&cmd, surface)) {
				continue
			}
			next := changedPathCoverageEvidence{
				caliber: caliber,
				runner:  strings.TrimSpace(cmd.Runner),
				source:  strings.TrimSpace(cmd.Source),
			}
			if existing, ok := out[canonical]; !ok ||
				changedPathCoverageCaliberRank(next.caliber) > changedPathCoverageCaliberRank(existing.caliber) {
				out[canonical] = next
			}
		}
	}
	return out
}

// typedPolyglotProjectRunnerCommand admits a cross-language project-runner
// proof only when the successful command is byte-for-byte bound to a
// filesystem-derived runnable test surface. Make is a meta runner: a root
// `make check` can intentionally execute a Python behavioral oracle over Rust
// sources (or any other polyglot arrangement), so assigning Make to C/C++ and
// rejecting every other family loses real verification. Conversely, a bare
// model-selected `make <target>` is not enough: without the exact typed
// candidate, cross-language coverage remains fail-closed.
func typedPolyglotProjectRunnerCommand(cmd *types.ExecutedCommand, surface *types.TestSurface) bool {
	if cmd == nil || surface == nil ||
		!strings.EqualFold(strings.TrimSpace(cmd.Runner), "make") ||
		strings.TrimSpace(cmd.Outcome) != "executed" || cmd.ExitCode != 0 {
		return false
	}
	workingDir := cleanRepoRelPath(cmd.WorkingDir)
	if workingDir == "" {
		workingDir = "."
	}
	for _, candidate := range surface.Candidates {
		candidateDir := cleanRepoRelPath(candidate.WorkingDir)
		if candidateDir == "" {
			candidateDir = "."
		}
		if !candidate.HasTestSignal ||
			!strings.EqualFold(strings.TrimSpace(candidate.Runner), "make") ||
			!strings.EqualFold(strings.TrimSpace(candidate.Framework), strings.TrimSpace(cmd.Framework)) ||
			!strings.EqualFold(candidateDir, workingDir) ||
			strings.TrimSpace(candidate.MakeTarget) == "" ||
			strings.TrimSpace(candidate.MakeTarget) != strings.TrimSpace(cmd.Suite) ||
			strings.TrimSpace(candidate.Command) == "" ||
			strings.TrimSpace(candidate.Command) != strings.TrimSpace(cmd.Command) {
			continue
		}
		return true
	}
	return false
}

func changedPathCoverageFromPassedProbes(
	plan *types.ChangePlan,
	report *types.ChangeReport,
	targets []string,
	targetFamilies map[string][]types.VerificationLanguageFamily,
) map[string]changedPathCoverageEvidence {
	passedIDs := map[string]bool{}
	for _, result := range report.TestResults {
		if !result.Passed || !strings.HasPrefix(strings.TrimSpace(result.Suite), "verification_probe/") {
			continue
		}
		if id := strings.TrimSpace(result.AssertionID); id != "" {
			passedIDs[id] = true
		}
	}
	if len(passedIDs) == 0 {
		return nil
	}
	targetSet := repoPathKeySet(targets)
	out := map[string]changedPathCoverageEvidence{}
	for _, probe := range plan.VerificationProbes {
		if !passedIDs[strings.TrimSpace(probe.ID)] {
			continue
		}
		probeFamilies := sourceVerificationLanguageFamilies(
			types.VerificationLanguageFamiliesFromVerificationProbeSuite("verification_probe/" + strings.TrimSpace(probe.Language)),
		)
		var exact []string
		for _, ref := range probe.ChangedSymbolRefs {
			ref = strings.TrimSpace(ref)
			if !strings.HasPrefix(ref, "path:") {
				continue
			}
			path := cleanRepoRelPath(strings.TrimPrefix(ref, "path:"))
			if canonical, ok := targetSet[strings.ToLower(path)]; ok &&
				verificationLanguageFamiliesIntersect(targetFamilies[canonical], probeFamilies) {
				exact = appendUniqueRepoPath(exact, canonical)
			}
		}
		// A non-path changed-symbol ref is path-unambiguous only for a single
		// changed source file. Multi-file plans require explicit path: refs.
		if len(exact) == 0 && len(targets) == 1 && len(probe.ChangedSymbolRefs) > 0 &&
			verificationLanguageFamiliesIntersect(targetFamilies[targets[0]], probeFamilies) {
			exact = append(exact, targets[0])
		}
		for _, path := range exact {
			out[path] = changedPathCoverageEvidence{
				caliber: types.ChangedPathVerificationProbe,
				runner:  "verification_probe",
				source:  strings.TrimSpace(probe.ID),
			}
		}
	}
	return out
}

func changedPathCoverageCaliberRank(caliber types.ChangedPathVerificationCaliber) int {
	switch caliber {
	case types.ChangedPathVerificationProjectRunner:
		return 3
	case types.ChangedPathVerificationProbe:
		return 2
	case types.ChangedPathVerificationSourceCheck:
		return 1
	default:
		return 0
	}
}

func verificationLanguageFamiliesIntersect(a, b []types.VerificationLanguageFamily) bool {
	set := map[types.VerificationLanguageFamily]bool{}
	for _, family := range a {
		set[family] = true
	}
	for _, family := range b {
		if set[family] {
			return true
		}
	}
	return false
}

func repoPathWithinWorkingDir(path, workingDir string) bool {
	path = cleanRepoRelPath(path)
	workingDir = cleanRepoRelPath(workingDir)
	if workingDir == "" || workingDir == "." {
		return true
	}
	pathLower := strings.ToLower(path)
	dirLower := strings.ToLower(strings.TrimSuffix(workingDir, "/"))
	return pathLower == dirLower || strings.HasPrefix(pathLower, dirLower+"/")
}

func normalizeCoveredTargetPaths(paths, targets []string) []string {
	targetSet := repoPathKeySet(targets)
	var out []string
	for _, raw := range paths {
		path := cleanRepoRelPath(raw)
		if canonical, ok := targetSet[strings.ToLower(path)]; ok {
			out = appendUniqueRepoPath(out, canonical)
		}
	}
	return out
}

func repoPathKeySet(paths []string) map[string]string {
	out := map[string]string{}
	for _, raw := range paths {
		path := cleanRepoRelPath(raw)
		if path != "" {
			out[strings.ToLower(path)] = path
		}
	}
	return out
}

func appendUniqueRepoPath(paths []string, path string) []string {
	path = cleanRepoRelPath(path)
	if path == "" {
		return paths
	}
	for _, existing := range paths {
		if strings.EqualFold(cleanRepoRelPath(existing), path) {
			return paths
		}
	}
	return append(paths, path)
}

func repoRelativeCoveragePaths(repoRoot string, files []string) []string {
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		rootAbs = filepath.Clean(repoRoot)
	}
	var out []string
	for _, file := range files {
		abs, err := filepath.Abs(file)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil || filepath.IsAbs(rel) {
			continue
		}
		path := cleanRepoRelPath(filepath.ToSlash(rel))
		if path == "" || path == ".." || strings.HasPrefix(path, "../") {
			continue
		}
		out = appendUniqueRepoPath(out, path)
	}
	return out
}

func changedPathVerificationFailureSummary(uncovered []string) string {
	paths := append([]string(nil), uncovered...)
	sort.Strings(paths)
	const maxShown = 8
	shown := paths
	suffix := ""
	if len(shown) > maxShown {
		shown = shown[:maxShown]
		suffix = fmt.Sprintf(" (+%d more)", len(paths)-maxShown)
	}
	return fmt.Sprintf(
		"local verification did not cover changed source path(s): %s%s; a successful runner for another language or directory cannot authorize these paths",
		strings.Join(shown, ", "),
		suffix,
	)
}

func syntaxCheckReportExitCode(report *types.ChangeReport) int {
	if report != nil && report.Passed {
		return 0
	}
	return 1
}

func finishedReportSummary(report *types.ChangeReport, fallback string) string {
	if report == nil ||
		report.FailureKind != types.FailureKindVerificationIncomplete ||
		strings.TrimSpace(report.FailureSummary) == "" {
		return fallback
	}
	return "[run_tests: verdict=UNAVAILABLE reason_code=" +
		changedPathVerificationUncoveredReasonCode + "] " +
		strings.TrimSpace(report.FailureSummary)
}
