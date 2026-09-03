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
	caliber    types.ChangedPathVerificationCaliber
	capability types.VerificationCapability
	runner     string
	source     string
}

// changeReportHasExecutionCapabilityDebt reports whether a successful typed
// changed-path ledger still proves only syntax/static properties for at least
// one production target. It deliberately ignores prose, runner output and
// commands; callers may use it to continue to another already-discovered test
// surface, never to declare success by itself.
func changeReportHasExecutionCapabilityDebt(report *types.ChangeReport) bool {
	if report == nil || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		return false
	}
	for _, row := range report.ChangedPathCoverage {
		if row.Status != types.ChangedPathVerificationCovered {
			continue
		}
		switch row.Capability {
		case types.VerificationCapabilitySyntaxOnly, types.VerificationCapabilitySourceStatic:
			return true
		}
	}
	return false
}

// applyChangedPathVerificationCoverage closes the report-level authority gap
// where a successful runner for one language used to authorize unrelated
// changed source paths. It consumes only typed plan paths, runner identities,
// working directories, exact CoveredPaths and probe metadata.
func applyChangedPathVerificationCoverage(ctx *types.BusContext, report *types.ChangeReport) {
	if ctx == nil || ctx.Mutable == nil || report == nil {
		return
	}
	applyChangedPathVerificationCoverageForPlan(ctx, ctx.Mutable.ChangePlan(), report, true)
}

// applyChangedPathVerificationCoverageForPlan lets the plan-stage probe lane
// evaluate the exact probe that just ran instead of accidentally borrowing the
// active ChangePlan's stored probe roster. enforceComplete is false for a
// planner observation: uncovered sibling paths remain typed rows but do not
// turn an otherwise useful exploratory observation into a failed execution.
// Post-apply verification keeps the original fail-closed all-path behavior.
func applyChangedPathVerificationCoverageForPlan(
	ctx *types.BusContext,
	plan *types.ChangePlan,
	report *types.ChangeReport,
	enforceComplete bool,
) {
	if ctx == nil || report == nil {
		return
	}
	if plan == nil {
		return
	}
	var workflow *types.WriteWorkflowRun
	if ctx.Mutable != nil {
		workflow = ctx.Mutable.WriteWorkflowRun()
	}
	targetPaths := types.ChangePlanVerificationTargetPaths(plan, workflow)
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
		if existing, ok := evidence[path]; !ok || changedPathCoverageEvidenceRank(probeEvidence) > changedPathCoverageEvidenceRank(existing) {
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
			row.Capability = proof.capability
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
	if !enforceComplete || len(uncovered) == 0 || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
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
		runnerFamilies, declaredPaths := executedCommandCoverageAuthority(*cmd, surface)
		if len(runnerFamilies) == 0 {
			continue
		}
		for _, path := range targets {
			if !repoPathWithinWorkingDir(path, cmd.WorkingDir) ||
				!executedCommandAuthorizesChangedPath(path, targetFamilies[path], runnerFamilies, declaredPaths) {
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
		var capability types.VerificationCapability
		switch {
		case outcome == "executed" && cmd.ExitCode == 0 && strings.TrimSpace(cmd.Runner) != "verification_probe":
			caliber = types.ChangedPathVerificationProjectRunner
			capability = types.VerificationCapabilityTargetBehavior
		case (outcome == "syntax_check_fallback" || outcome == "syntax_preflight") && cmd.ExitCode == 0:
			caliber = types.ChangedPathVerificationSourceCheck
			capability = types.VerificationCapabilitySyntaxOnly
		default:
			continue
		}
		runnerFamilies, declaredPaths := executedCommandCoverageAuthority(cmd, surface)
		for _, raw := range cmd.CoveredPaths {
			path := cleanRepoRelPath(raw)
			canonical, ok := targetSet[strings.ToLower(path)]
			if !ok ||
				!executedCommandAuthorizesChangedPath(canonical, targetFamilies[canonical], runnerFamilies, declaredPaths) {
				continue
			}
			pathCaliber := caliber
			pathCapability := capability
			if caliber == types.ChangedPathVerificationProjectRunner &&
				!verificationLanguageFamiliesIntersect(targetFamilies[canonical], runnerFamilies) {
				pathCaliber = types.ChangedPathVerificationDeclaredProjectCheck
				pathCapability = types.VerificationCapabilitySourceStatic
			}
			next := changedPathCoverageEvidence{
				caliber:    pathCaliber,
				capability: pathCapability,
				runner:     strings.TrimSpace(cmd.Runner),
				source:     strings.TrimSpace(cmd.Source),
			}
			if existing, ok := out[canonical]; !ok ||
				changedPathCoverageEvidenceRank(next) > changedPathCoverageEvidenceRank(existing) {
				out[canonical] = next
			}
		}
	}
	return out
}

// executedCommandAuthorizesChangedPath keeps the meta-runner driver language
// and the checked artifact language on separate axes. Ordinary runners retain
// language+working-directory authority. A matched repository-declared target
// carries a non-nil exact roster; in that lane membership is the whole target
// authority and the driver language cannot broaden it to sibling files.
func executedCommandAuthorizesChangedPath(
	path string,
	targetFamilies []types.VerificationLanguageFamily,
	runnerFamilies []types.VerificationLanguageFamily,
	declaredPaths map[string]bool,
) bool {
	if declaredPaths != nil {
		return declaredPaths[strings.ToLower(cleanRepoRelPath(path))]
	}
	return verificationLanguageFamiliesIntersect(targetFamilies, runnerFamilies)
}

// executedCommandCoverageAuthority resolves the concrete execution family of
// one successful command. Ordinary runners retain their existing language
// identity. A meta runner may use a repository-declared family only when the
// executed command matches the exact typed surface candidate and suite; in
// that lane the exact declared path roster is also mandatory.
func executedCommandCoverageAuthority(
	cmd types.ExecutedCommand,
	surface *types.TestSurface,
) ([]types.VerificationLanguageFamily, map[string]bool) {
	base := sourceVerificationLanguageFamilies(
		types.VerificationLanguageFamiliesFromRunner(cmd.Runner, cmd.Framework),
	)
	if surface == nil {
		return base, nil
	}
	key := testSurfaceCandidateKey(cmd.Runner, cmd.Framework, cmd.WorkingDir)
	for _, cand := range surface.Candidates {
		if !cand.HasTestSignal ||
			testSurfaceCandidateKey(cand.Runner, cand.Framework, cand.WorkingDir) != key ||
			strings.TrimSpace(cand.MakeTarget) != strings.TrimSpace(cmd.Suite) ||
			len(cand.DeclaredExecutionLanguageFamilies) == 0 ||
			len(cand.DeclaredCoveragePaths) == 0 {
			continue
		}
		families := sourceVerificationLanguageFamilies(cand.DeclaredExecutionLanguageFamilies)
		if len(families) == 0 {
			return nil, nil
		}
		paths := make(map[string]bool, len(cand.DeclaredCoveragePaths))
		for _, raw := range cand.DeclaredCoveragePaths {
			if path := cleanRepoRelPath(raw); path != "" {
				paths[strings.ToLower(path)] = true
			}
		}
		return families, paths
	}
	return base, nil
}

func changedPathCoverageFromPassedProbes(
	plan *types.ChangePlan,
	report *types.ChangeReport,
	targets []string,
	targetFamilies map[string][]types.VerificationLanguageFamily,
) map[string]changedPathCoverageEvidence {
	passedIDs := passedVerificationProbeIDs(report)
	if len(passedIDs) == 0 {
		return nil
	}
	out := map[string]changedPathCoverageEvidence{}
	for _, probe := range types.ChangePlanVerificationProbes(plan) {
		if !passedIDs[strings.TrimSpace(probe.ID)] {
			continue
		}
		capability := types.VerificationCapabilityTargetExecution
		if len(probe.ContractRefs) > 0 {
			capability = types.VerificationCapabilityTargetBehavior
		}
		for _, path := range verificationProbeChangedTargetPaths(probe, targets, targetFamilies) {
			out[path] = changedPathCoverageEvidence{
				caliber:    types.ChangedPathVerificationProbe,
				capability: capability,
				runner:     "verification_probe",
				source:     strings.TrimSpace(probe.ID),
			}
		}
	}
	return out
}

func passedVerificationProbeIDs(report *types.ChangeReport) map[string]bool {
	passedIDs := map[string]bool{}
	if report == nil {
		return passedIDs
	}
	for _, result := range report.TestResults {
		if !result.Passed || !strings.HasPrefix(strings.TrimSpace(result.Suite), "verification_probe/") {
			continue
		}
		if id := strings.TrimSpace(result.AssertionID); id != "" {
			passedIDs[id] = true
		}
	}
	return passedIDs
}

// verificationProbeChangedTargetPaths resolves the typed identity boundary
// for one probe. Exact path: refs may select any compatible changed target;
// a language-level symbol is unambiguous only when exactly one changed source
// target belongs to the probe's language family. Contract labels and probe
// code/prose do not participate in this authority decision.
func verificationProbeChangedTargetPaths(
	probe types.VerificationProbe,
	targets []string,
	targetFamilies map[string][]types.VerificationLanguageFamily,
) []string {
	probeFamilies := sourceVerificationLanguageFamilies(
		types.VerificationProbeDirectSourceFamilies(probe.Language),
	)
	if len(probeFamilies) == 0 || len(probe.ChangedSymbolRefs) == 0 {
		return nil
	}
	targetSet := repoPathKeySet(targets)
	var exact []string
	hasNonPathRef := false
	for _, ref := range probe.ChangedSymbolRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if !strings.HasPrefix(ref, "path:") {
			hasNonPathRef = true
			continue
		}
		path := cleanRepoRelPath(strings.TrimPrefix(ref, "path:"))
		if canonical, ok := targetSet[strings.ToLower(path)]; ok &&
			verificationLanguageFamiliesIntersect(targetFamilies[canonical], probeFamilies) {
			exact = appendUniqueRepoPath(exact, canonical)
		}
	}
	if len(exact) > 0 || !hasNonPathRef {
		return exact
	}
	var compatible []string
	for _, target := range targets {
		if verificationLanguageFamiliesIntersect(targetFamilies[target], probeFamilies) {
			compatible = append(compatible, target)
		}
	}
	if len(compatible) == 1 {
		return compatible
	}
	return nil
}

func changedPathCoverageCaliberRank(caliber types.ChangedPathVerificationCaliber) int {
	switch caliber {
	case types.ChangedPathVerificationProjectRunner:
		return 4
	case types.ChangedPathVerificationDeclaredProjectCheck:
		return 3
	case types.ChangedPathVerificationProbe:
		return 2
	case types.ChangedPathVerificationSourceCheck:
		return 1
	default:
		return 0
	}
}

func changedPathCoverageEvidenceRank(evidence changedPathCoverageEvidence) int {
	capability := 0
	switch evidence.capability {
	case types.VerificationCapabilityTargetBehavior:
		capability = 4
	case types.VerificationCapabilityTargetExecution:
		capability = 3
	case types.VerificationCapabilitySourceStatic:
		capability = 2
	case types.VerificationCapabilitySyntaxOnly:
		capability = 1
	}
	return capability*10 + changedPathCoverageCaliberRank(evidence.caliber)
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
	base := fallback
	if report == nil ||
		report.FailureKind != types.FailureKindVerificationIncomplete ||
		strings.TrimSpace(report.FailureSummary) == "" {
		return base + renderRunTestsWorktreeAuditSummary(report)
	}
	base = "[run_tests: verdict=UNAVAILABLE reason_code=" +
		changedPathVerificationUncoveredReasonCode + "] " +
		strings.TrimSpace(report.FailureSummary)
	return base + renderRunTestsWorktreeAuditSummary(report)
}

func renderRunTestsWorktreeAuditSummary(report *types.ChangeReport) string {
	if report == nil || report.WorktreeAudit == nil {
		return ""
	}
	audit := report.WorktreeAudit
	// The untracked lane renders independently of the tracked lane's
	// disposition (shared sentence; refused runs included).
	untracked := verificationWorktreeUntrackedRetainedSentence(audit)
	switch audit.Status {
	case types.VerificationWorktreeAuditUntrackedSideEffects:
		if untracked == "" {
			return ""
		}
		return " [worktree_audit: " + strings.TrimPrefix(untracked, "; ") + "]"
	case types.VerificationWorktreeAuditTrackedDrift:
		paths := verificationWorktreeRefusedEffectPaths(audit.Effects, 8)
		note := ""
		if audit.DisclosedTrackedEffectCount > 0 {
			note = "; " + verificationWorktreeDisclosedRowsSummary(audit)
		}
		return " [worktree_audit: tracked drift; verification failed: " + strings.Join(paths, ", ") + note + untracked + "]"
	case types.VerificationWorktreeAuditTrackedDriftDisclosed:
		return " [worktree_audit: tracked side effects disclosed, verdict kept, not committed, not auto-reverted: " + verificationWorktreeDisclosedRowsSummary(audit) + untracked + "]"
	case types.VerificationWorktreeAuditUnavailable:
		return " [worktree_audit: unavailable]"
	default:
		return ""
	}
}

// verificationWorktreeDisclosedRowsSummary renders "path=class(owner)" for the
// disclosed rows (V5-2), so refused and disclosed drift stay distinguishable;
// an unproven lockfile fixed point is named in plain words on the row.
func verificationWorktreeDisclosedRowsSummary(audit *types.VerificationWorktreeAudit) string {
	var parts []string
	for _, effect := range audit.Effects {
		if effect.Disposition != types.VerificationWorktreeEffectDisclosed {
			continue
		}
		parts = append(parts, verificationWorktreeEffectRowSummary(effect))
		if len(parts) >= 8 {
			break
		}
	}
	return strings.Join(parts, ", ")
}
