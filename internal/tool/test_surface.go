package tool

import (
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
	return types.NormalizeTestSurface(surface)
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
// one execution per (runner, working_dir) pair.
func testSurfaceCandidateKey(runner, workingDir string) string {
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		workingDir = "."
	}
	return runner + "@" + workingDir
}

// nextTestSurfaceEscalation returns the highest-ranked candidate that carries
// a typed test signal and has not been executed yet, or nil. Both inputs are
// typed (surface ordering + executed-key set); no prose is consulted.
func nextTestSurfaceEscalation(surface types.TestSurface, executed map[string]bool) *types.TestSurfaceCandidate {
	for i := range surface.Candidates {
		c := surface.Candidates[i]
		if !c.HasTestSignal {
			continue
		}
		if executed[testSurfaceCandidateKey(c.Runner, c.WorkingDir)] {
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
	}
}
