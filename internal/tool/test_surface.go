package tool

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildTestSurface computes the typed runnable-verification inventory for a
// repository root by composing the existing runner detectors. It introduces
// no new language knowledge: manifests come from detectRunnerPlans, test-work
// signals from runnerHasNoTestWork, Makefile targets from
// detectMakeTestTargetFound, and native build readiness from
// detectNativeBuildDir. The returned surface is ordered by the selection rule
// "runnable test work dominates manifest priority" (types.NormalizeTestSurface)
// so a Makefile with a declared check target outranks a higher-priority
// manifest whose tree has no test work.
//
// repoRoot anchors labels and relative paths; walkRoot confines discovery
// (multi-repo active sub-repo), empty meaning repoRoot — the same contract as
// detectRunnerPlans.
func BuildTestSurface(repoRoot, walkRoot string) types.TestSurface {
	surface := types.TestSurface{GeneratedAt: time.Now()}
	for _, plan := range detectRunnerPlanCandidates(repoRoot, walkRoot) {
		surface.Candidates = append(surface.Candidates, testSurfaceCandidateForPlan(repoRoot, plan))
	}
	surface.Candidates = appendSyntheticPythonCandidates(repoRoot, surface.Candidates)
	return types.NormalizeTestSurface(surface)
}

// appendSyntheticPythonCandidates closes two typed gaps the manifest walk
// cannot see:
//
//   - a bare Python directory (test files by convention, no manifest at
//     all) yields ZERO candidates, so a missing-binary or zero-test dead
//     end has nowhere to escalate;
//   - a python/pytest candidate whose host lacks pytest has a runnable
//     stdlib sibling — python/unittest — that shares the directory but a
//     framework-blind candidate set hides it.
//
// Both syntheses read the same typed detectors the executor uses
// (hasPythonTestInfrastructure, detectPythonTestFramework,
// hasPythonUnittestSignal); python is the only runner synthesized because
// the stdlib unittest runner is the one framework that needs no installed
// dependency.
func appendSyntheticPythonCandidates(repoRoot string, cands []types.TestSurfaceCandidate) []types.TestSurfaceCandidate {
	seen := map[string]bool{}
	for _, c := range cands {
		seen[testSurfaceCandidateKey(c.Runner, c.Framework, c.WorkingDir)] = true
	}
	addPython := func(framework string) {
		key := testSurfaceCandidateKey("python", framework, ".")
		if seen[key] {
			return
		}
		seen[key] = true
		plan := runnerPlan{Runner: "python", Root: repoRoot, Framework: framework, Priority: 7}
		cand := testSurfaceCandidateForPlan(repoRoot, plan)
		cand.Source = "test-file convention"
		cands = append(cands, cand)
	}
	hasRootPython := false
	rootPytest := false
	for _, c := range cands {
		if c.Runner == "python" && c.WorkingDir == "." {
			hasRootPython = true
			if c.Framework == pythonFrameworkPytest {
				rootPytest = true
			}
		}
	}
	if !hasRootPython && hasPythonTestInfrastructure(repoRoot) {
		addPython(detectPythonTestFramework(repoRoot))
		if detectPythonTestFramework(repoRoot) == pythonFrameworkPytest && hasPythonUnittestSignal(repoRoot) {
			addPython(pythonFrameworkUnittest)
		}
		return cands
	}
	if rootPytest && hasPythonUnittestSignal(repoRoot) {
		addPython(pythonFrameworkUnittest)
	}
	return cands
}

// testSurfaceCandidateForPlan converts one detected runnerPlan into the typed
// candidate shape, attaching the per-runner-family test-work signal.
func testSurfaceCandidateForPlan(repoRoot string, plan runnerPlan) types.TestSurfaceCandidate {
	rel := runnerPlanRel(repoRoot, plan)
	cand := types.TestSurfaceCandidate{
		ID:         runnerPlanLabel(repoRoot, plan),
		Runner:     plan.Runner,
		Framework:  plan.Framework,
		WorkingDir: rel,
		Priority:   plan.Priority,
	}
	if m := strings.TrimSpace(plan.Manifest); m != "" && m != "(LLM-selected)" {
		if rel == "." {
			cand.Source = m
		} else {
			cand.Source = rel + "/" + m
		}
	}
	// Render-only command preview; the executor re-renders with suite /
	// venv resolution at run time.
	cand.Command, _ = buildRunCommandForPlan(plan, "", "")
	switch plan.Runner {
	case "make":
		target, found := detectMakeTestTargetFound(plan.Root)
		cand.MakeTarget = target
		cand.HasTestSignal = found
	case "cmake", "meson":
		cand.HasTestSignal = detectNativeBuildDir(plan.Root) != ""
	default:
		cand.HasTestSignal = !runnerHasNoTestWork(plan.Runner, plan.Root)
	}
	return cand
}

// testSurfaceCandidateKey is the executed-set key for escalation decisions:
// one execution per (runner, framework, working_dir) triple. Framework is
// part of the key so a python/pytest dead end can escalate to the
// python/unittest candidate of the same directory.
func testSurfaceCandidateKey(runner, framework, workingDir string) string {
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		workingDir = "."
	}
	framework = strings.TrimSpace(framework)
	if framework != "" {
		runner += "/" + framework
	}
	return runner + "@" + workingDir
}

// nextTestSurfaceEscalation returns the highest-ranked candidate that carries
// a typed test signal and has not been executed yet, or nil. Both inputs are
// typed (surface ordering + executed-key set); no prose is consulted.
func nextTestSurfaceEscalation(surface types.TestSurface, executed map[string]bool) *types.TestSurfaceCandidate {
	return nextTestSurfaceEscalationForRunner(surface, executed, "")
}

func nextTestSurfaceEscalationForRunner(surface types.TestSurface, executed map[string]bool, preferredRunner string) *types.TestSurfaceCandidate {
	preferredRunner = strings.TrimSpace(preferredRunner)
	if preferredRunner != "" {
		if cand := nextTestSurfaceEscalationMatchingRunner(surface, executed, preferredRunner); cand != nil {
			return cand
		}
		return nil
	}
	return nextTestSurfaceEscalationMatchingRunner(surface, executed, "")
}

func nextTestSurfaceEscalationMatchingRunner(surface types.TestSurface, executed map[string]bool, runner string) *types.TestSurfaceCandidate {
	for i := range surface.Candidates {
		c := surface.Candidates[i]
		if runner != "" && c.Runner != runner {
			continue
		}
		if !c.HasTestSignal {
			continue
		}
		if executed[testSurfaceCandidateKey(c.Runner, c.Framework, c.WorkingDir)] {
			continue
		}
		out := c
		return &out
	}
	return nil
}

// runnerPlanFromSurfaceCandidate rebuilds an executable runnerPlan for an
// escalation candidate. Root is re-anchored under repoRoot so the plan runs
// inside the same verification tree the surface was computed from.
func runnerPlanFromSurfaceCandidate(repoRoot string, cand types.TestSurfaceCandidate) runnerPlan {
	root := repoRoot
	if rel := strings.TrimSpace(cand.WorkingDir); rel != "" && rel != "." {
		root = filepath.Join(repoRoot, filepath.FromSlash(rel))
	}
	return runnerPlan{
		Runner:    cand.Runner,
		Root:      root,
		Manifest:  cand.Source,
		Priority:  cand.Priority,
		Framework: cand.Framework,
		Suite:     surfaceCandidateSuite(cand),
	}
}

// defaultRunnerPlansFromTestSurface builds the system-owned verify queue for a
// run_tests call with no explicit runner. It consumes typed filesystem surface
// data, typed impact-priority runner plans, and an optional plan-touched runner
// preference. The model no longer needs a pre-verification directory-inspection
// turn just to select the first runner; dead-end escalation still reuses the
// same TestSurface later.
func defaultRunnerPlansFromTestSurface(repoRoot string, surface types.TestSurface, preferredRunner string, priorityPlans ...runnerPlan) []runnerPlan {
	surface = types.NormalizeTestSurface(surface)
	preferredRunner = strings.TrimSpace(preferredRunner)
	var out []runnerPlan
	seenDirs := map[string]bool{}
	seenPriorityPlans := map[string]bool{}
	addPriorityPlan := func(plan runnerPlan) {
		workingDir := runnerPlanRel(repoRoot, plan)
		if workingDir == "" {
			workingDir = "."
		}
		key := runnerPlanQueueKey(repoRoot, plan)
		if key == "" || seenPriorityPlans[key] {
			return
		}
		seenPriorityPlans[key] = true
		seenDirs[workingDir] = true
		out = append(out, plan)
	}
	add := func(cand types.TestSurfaceCandidate) {
		workingDir := strings.TrimSpace(cand.WorkingDir)
		if workingDir == "" {
			workingDir = "."
		}
		if seenDirs[workingDir] {
			return
		}
		seenDirs[workingDir] = true
		out = append(out, runnerPlanFromSurfaceCandidate(repoRoot, cand))
	}
	for _, plan := range priorityPlans {
		if strings.TrimSpace(plan.Runner) == "" || strings.TrimSpace(plan.Root) == "" {
			continue
		}
		addPriorityPlan(plan)
	}
	if preferredRunner != "" {
		if cand := nextTestSurfaceEscalationForRunner(surface, nil, preferredRunner); cand != nil {
			add(*cand)
		} else {
			root := repoRoot
			framework := ""
			if preferredRunner == "python" {
				framework = normalizePythonFrameworkChoice(root, "")
			}
			out = append(out, runnerPlan{
				Runner:    preferredRunner,
				Root:      root,
				Manifest:  "(plan-touched)",
				Priority:  0,
				Framework: framework,
			})
			seenDirs["."] = true
		}
	}
	for _, cand := range surface.Candidates {
		add(cand)
	}
	return out
}

func runnerPlanQueueKey(repoRoot string, plan runnerPlan) string {
	key := testSurfaceCandidateKey(plan.Runner, plan.Framework, runnerPlanRel(repoRoot, plan))
	suite := strings.TrimSpace(plan.Suite)
	if suite != "" {
		key += "\x00suite=" + suite
	}
	return strings.Trim(key, "\x00")
}

func impactRunnerPlansFromChangePlan(repoRoot string, surface types.TestSurface, plan *types.ChangePlan) []runnerPlan {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" || plan == nil {
		return nil
	}
	surface = types.NormalizeTestSurface(surface)
	targets := impactVerificationTargetsFromChangePlan(plan)
	out := make([]runnerPlan, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		if strings.TrimSpace(target.Kind) != "test_surface" {
			continue
		}
		related := safeImpactRelatedPath(repoRoot, target.RelatedPath)
		if related == "" {
			continue
		}
		cand := impactCandidateForRelatedPath(surface, related)
		if cand == nil {
			continue
		}
		suite := impactSuiteForCandidate(*cand, related)
		if suite == "" {
			continue
		}
		if validateRunTestsSuiteSelector(suite) != "" {
			continue
		}
		next := runnerPlanFromSurfaceCandidate(repoRoot, *cand)
		next.Suite = suite
		key := strings.Join([]string{
			testSurfaceCandidateKey(next.Runner, next.Framework, runnerPlanRel(repoRoot, next)),
			next.Suite,
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, next)
	}
	return out
}

func impactVerificationTargetsFromChangePlan(plan *types.ChangePlan) []types.ImpactVerificationTarget {
	if plan == nil {
		return nil
	}
	var targets []types.ImpactVerificationTarget
	seen := map[string]bool{}
	add := func(in []types.ImpactVerificationTarget) {
		for _, target := range in {
			targetsKey := strings.Join([]string{
				strings.TrimSpace(target.Kind),
				strings.TrimSpace(target.Path),
				strings.TrimSpace(target.RelatedPath),
				strings.TrimSpace(target.ContractRef),
				strings.TrimSpace(target.ProbeID),
				strings.TrimSpace(target.EvidenceRef),
			}, "\x00")
			if strings.Trim(targetsKey, "\x00") == "" || seen[targetsKey] {
				continue
			}
			seen[targetsKey] = true
			targets = append(targets, target)
		}
	}
	if plan.ImpactAnalysis != nil {
		analysis := types.NormalizeImpactAnalysisResult(*plan.ImpactAnalysis)
		add(analysis.VerificationTargets)
		if analysis.ObligationSet != nil {
			result := types.ImpactAnalysisResultFromObligationSet(plan.ID, analysis.PatchEffectID, *analysis.ObligationSet, time.Time{})
			add(result.VerificationTargets)
		}
	}
	if plan.ImpactObligations != nil {
		result := types.ImpactAnalysisResultFromObligationSet(plan.ID, "", *plan.ImpactObligations, time.Time{})
		add(result.VerificationTargets)
	}
	return targets
}

func safeImpactRelatedPath(repoRoot, related string) string {
	related = strings.TrimSpace(strings.ReplaceAll(related, "\\", "/"))
	related = strings.TrimPrefix(related, "./")
	if related == "" || related == "." || strings.HasPrefix(related, "/") {
		return ""
	}
	cleaned := path.Clean(related)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return ""
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(cleaned))
	if !pathWithinRoot(repoRoot, abs) {
		return ""
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		return ""
	}
	return cleaned
}

func impactCandidateForRelatedPath(surface types.TestSurface, related string) *types.TestSurfaceCandidate {
	var best *types.TestSurfaceCandidate
	bestDepth := -1
	for i := range surface.Candidates {
		cand := surface.Candidates[i]
		if !cand.HasTestSignal || !impactCandidateSupportsSuite(cand) {
			continue
		}
		workingDir := strings.TrimSpace(cand.WorkingDir)
		if workingDir == "" {
			workingDir = "."
		}
		if !testSurfaceWorkingDirContainsPath(workingDir, related) {
			continue
		}
		depth := impactWorkingDirDepth(workingDir)
		if best == nil || depth > bestDepth {
			next := cand
			best = &next
			bestDepth = depth
		}
	}
	return best
}

func impactWorkingDirDepth(rel string) int {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return -1
	}
	return strings.Count(rel, "/")
}

func impactCandidateSupportsSuite(cand types.TestSurfaceCandidate) bool {
	switch cand.Runner {
	case "go", "java", "node", "ruby", "rust", "swift":
		return true
	case "python":
		return cand.Framework == "" ||
			cand.Framework == pythonFrameworkPytest ||
			cand.Framework == pythonFrameworkDjango ||
			cand.Framework == pythonFrameworkUnittest
	default:
		return false
	}
}

func testSurfaceWorkingDirContainsPath(workingDir, related string) bool {
	workingDir = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(workingDir, "\\", "/"), "./"))
	related = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(related, "\\", "/"), "./"))
	if workingDir == "" || workingDir == "." {
		return true
	}
	return related == workingDir || strings.HasPrefix(related, workingDir+"/")
}

func impactSuiteForCandidate(cand types.TestSurfaceCandidate, related string) string {
	rel := relatedPathInsideWorkingDir(cand.WorkingDir, related)
	if rel == "" {
		return ""
	}
	switch cand.Runner {
	case "go":
		if path.Ext(rel) != ".go" {
			return ""
		}
		dir := path.Dir(rel)
		if dir == "." || dir == "" {
			return "."
		}
		return "./" + dir
	case "java":
		if !javaTestPathExtension(rel) {
			return ""
		}
		selector := javaTestSelectorFromPath(rel)
		if selector == "" {
			return ""
		}
		// Maven and Gradle both accept class selectors. Keep this as
		// a normalized selector instead of passing file paths to the
		// shell command builder, where Maven/Gradle semantics differ.
		return selector
	case "python":
		if path.Ext(rel) != ".py" {
			return ""
		}
		switch cand.Framework {
		case pythonFrameworkUnittest:
			dir := path.Dir(rel)
			if dir == "." || dir == "" {
				return "."
			}
			return dir
		default:
			return rel
		}
	case "ruby":
		if path.Ext(rel) != ".rb" {
			return ""
		}
		return rel
	case "node":
		if !nodeSuiteSelectorPath(rel) {
			return ""
		}
		return rel
	case "rust":
		if suite := rustIntegrationTestSelectorFromPath(rel); suite != "" {
			return suite
		}
		return ""
	case "swift":
		if !swiftTestPath(rel) {
			return ""
		}
		return swiftTestFilterFromSuite(rel)
	default:
		return ""
	}
}

func javaTestPathExtension(rel string) bool {
	switch strings.ToLower(path.Ext(rel)) {
	case ".java", ".kt":
		return true
	default:
		return false
	}
}

func javaTestSelectorFromPath(rel string) string {
	rel = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "./"))
	if rel == "" || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || !javaTestPathExtension(rel) {
		return ""
	}
	noExt := strings.TrimSuffix(rel, path.Ext(rel))
	for _, prefix := range []string{
		"src/test/java/",
		"src/test/kotlin/",
		"src/integrationTest/java/",
		"src/integrationTest/kotlin/",
	} {
		if strings.HasPrefix(noExt, prefix) {
			className := strings.Trim(strings.TrimPrefix(noExt, prefix), "/")
			if className == "" {
				return ""
			}
			return strings.ReplaceAll(className, "/", ".")
		}
	}
	base := path.Base(noExt)
	if base == "." || base == "/" || base == "" {
		return ""
	}
	return base
}

func rustIntegrationTestSelectorFromPath(rel string) string {
	rel = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "./"))
	if rel == "" || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || path.Ext(rel) != ".rs" {
		return ""
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || parts[0] != "tests" {
		return ""
	}
	name := strings.TrimSuffix(parts[1], ".rs")
	if name == "" || strings.HasPrefix(name, ".") {
		return ""
	}
	return rel
}

func swiftTestPath(rel string) bool {
	rel = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "./"))
	if rel == "" || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || path.Ext(rel) != ".swift" {
		return false
	}
	base := strings.TrimSuffix(path.Base(rel), ".swift")
	return strings.HasSuffix(base, "Test") || strings.HasSuffix(base, "Tests")
}

func swiftTestFilterFromSuite(suite string) string {
	suite = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(suite, "\\", "/"), "./"))
	if suite == "" || strings.HasPrefix(suite, "../") || strings.Contains(suite, "/../") {
		return ""
	}
	if path.Ext(suite) == ".swift" {
		return strings.TrimSuffix(path.Base(suite), ".swift")
	}
	return suite
}

func relatedPathInsideWorkingDir(workingDir, related string) string {
	workingDir = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(workingDir, "\\", "/"), "./"))
	related = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(related, "\\", "/"), "./"))
	if workingDir == "" || workingDir == "." {
		return related
	}
	if related == workingDir {
		return ""
	}
	prefix := workingDir + "/"
	if strings.HasPrefix(related, prefix) {
		return strings.TrimPrefix(related, prefix)
	}
	return ""
}

func nodeSuiteSelectorPath(rel string) bool {
	rel = strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "./"))
	if rel == "" || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return false
	}
	switch strings.ToLower(path.Ext(rel)) {
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts":
		return true
	default:
		return false
	}
}

func surfaceCandidateSuite(cand types.TestSurfaceCandidate) string {
	if cand.Runner == "make" {
		return strings.TrimSpace(cand.MakeTarget)
	}
	return ""
}
