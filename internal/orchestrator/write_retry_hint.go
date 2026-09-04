package orchestrator

// write_retry_hint.go — the ChangePlan verify→plan retry-hint concern
// (§8.14): the heuristic PlanningHint the planner receives after a failed
// apply/verify attempt, the "current vs best" regression delta, and the
// bounded per-path plan content diff that backs it. Moved out of
// orchestrator.go under the IR-delivery LOC ratchet (§40.27 V7-5): the
// ratchet is paid by extracting a concern, never by raising a budget or
// compressing comments. Write path only — callers are the apply/verify
// stage hooks (stage_hooks.go) and the orchestrator's retry loop; the read
// scheduler loop never reaches this file (L1).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/hanchaoqun/codrax/internal/types"
)

func buildRetryHint(report *types.ChangeReport, plan *types.ChangePlan, prevAttempt int) string {
	var b strings.Builder
	if report == nil {
		fmt.Fprintf(&b, "Previous attempt %d failed without producing a ChangeReport (apply or runner error). ", prevAttempt)
	} else {
		fmt.Fprintf(&b, "Previous attempt %d verify failed. ", prevAttempt)
		// Resource-exhaustion classifications get a kind-specific
		// header BEFORE the test summary so the planner reads "your
		// code allocated unboundedly / spun a CPU loop" first, not
		// the inevitable downstream "tests didn't complete" symptom.
		// Without this surfacing the planner re-derives the same
		// wrong corrective direction from buried stderr — the OOM
		// event that motivated this code is the textbook instance.
		// Failure-mode classifications stay as neutral one-line tags
		// — the model reads the raw FailureSummary below and decides
		// the fix. Pre-2026-04-30 these branches injected paragraphs
		// of system-prescribed corrective directions ("Most common
		// causes: ... Revise the plan to: ... DO NOT raise the cap")
		// — that violated the "feed error to model, let model decide"
		// red line. The structural label is enough: the model sees
		// the kind plus the stderr and chooses how to respond.
		switch report.FailureKind {
		case types.FailureKindOOM:
			b.WriteString("\n\n## Failure mode: out-of-memory (memory limit fired). ")
		case types.FailureKindCPULimit:
			b.WriteString("\n\n## Failure mode: CPU-time limit exceeded. ")
		case types.FailureKindTimeout:
			b.WriteString("\n\n## Failure mode: wall-clock timeout. ")
		}
		if report.FailureSummary != "" {
			// Pass the full FailureSummary verbatim — the model needs
			// complete unambiguous error context to decide the fix.
			// Pre-2026-04-30 truncation at 300 chars dropped the line
			// that named the actual error (pytest's `E ` line is
			// ~10-15 lines into a fixture trace), forcing the model
			// to guess from header noise. Operator-facing log lines
			// keep their own caps; THIS path is LLM-facing context.
			fmt.Fprintf(&b, "Summary: %s ", report.FailureSummary)
		}
		const (
			maxFailingTests = 3
			maxDetailChars  = 600 // upgraded from 140: the previous floor took only the first line, which is pytest's "self = <Test fixture>" header — useless. 600 fits the assertion + expected/actual + 1-2 stack frames.
		)
		shown := 0
		for _, tr := range report.TestResults {
			if tr.Passed {
				continue
			}
			if shown == 0 {
				b.WriteString("Failing tests:")
			}
			shown++
			// FailureDetail extraction. ExtractFailureSignal isolates
			// the actually-error-bearing lines (pytest E-marked lines,
			// go test "FAIL:" / panic frames, JUnit assertion-failed
			// messages) instead of the first line, which on most
			// runners is fixture / setup boilerplate. Bug fix from
			// Batch E robot-name analysis: previously `SplitN("\n",
			// 2)[0]` returned `self = <Test fixture>`, leaving the
			// reflector blind to the actual assertion failure.
			detail := ExtractFailureSignal(tr.FailureDetail, maxDetailChars)
			if tr.Suite != "" {
				fmt.Fprintf(&b, "\n  - %s (%s)", tr.AssertionID, tr.Suite)
			} else {
				fmt.Fprintf(&b, "\n  - %s", tr.AssertionID)
			}
			if detail != "" {
				fmt.Fprintf(&b, ": %s", detail)
			}
			if shown >= maxFailingTests {
				break
			}
		}
		if shown > 0 {
			b.WriteString("\n")
		}
	}
	if plan != nil && len(plan.TargetPaths) > 0 {
		const maxPaths = 10
		paths := plan.TargetPaths
		extra := 0
		if len(paths) > maxPaths {
			extra = len(paths) - maxPaths
			paths = paths[:maxPaths]
		}
		b.WriteString("\nFiles modified by the previous plan (suspect list — the regression is in the edits to these files):\n")
		for _, p := range paths {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
		if extra > 0 {
			fmt.Fprintf(&b, "  - … (+%d more)\n", extra)
		}
		b.WriteString("\nRepair scope: keep the next plan focused on the failing batch and these suspect paths unless read-only investigation proves another file is required. If the scope expands, state the new path and reason in the plan summary.\n")
	}
	b.WriteString("Revise the plan to address these failures; do not repeat the same changes.")
	return b.String()
}

// buildRetryHintWithBest extends buildRetryHint with a "current vs
// best" delta when an earlier retry iteration produced a strictly-
// better (passed, total) score than the current iteration. Lets the
// planner see what it just lost — generic across runners since it
// only uses the (passed, total) score returned by ChangeReport.Score.
//
// When the current iteration IS the best (or no prior iteration was
// better), the output equals buildRetryHint's exactly so the existing
// behaviour is preserved on monotonic-improvement trajectories.
//
// On regression, the hint also includes a unified diff between the
// best plan's NewContent (or Patch) and the current iteration's,
// per overlapping path. Without the diff, the planner only sees
// "regressed from 51 to 45" and a file list — it has to reconstruct
// from memory which specific edits broke things. Showing the actual
// code delta closes that information gap so the LLM can revert
// targeted lines instead of re-deriving the whole solution.
//
// Bug provenance: Batch L forth-py — best at iter 1 was 51/54;
// iters 2-4 regressed to 46→45→0 because reflector hints were
// abstract ("preserve the lazy-resolution snapshot") and the LLM
// kept rebuilding the wrong pieces. Showing the diff would have
// surfaced "you changed lookup() in this exact way; the test that
// regressed exercises lookup()".
func buildRetryHintWithBest(curReport *types.ChangeReport, curPlan *types.ChangePlan, bestReport *types.ChangeReport, bestPlan *types.ChangePlan, prevAttempt int) string {
	base := buildRetryHint(curReport, curPlan, prevAttempt)
	if bestReport == nil || !bestReport.IsBetterThan(curReport) {
		return base
	}
	bp, bt := bestReport.Score()
	cp, ct := curReport.Score()
	delta := fmt.Sprintf(
		"\n\n## Regression detected\nCurrent attempt scored %d/%d; an earlier attempt in this retry loop scored %d/%d. The plan is moving in the WRONG direction. Re-examine which previous edits were correct and preserve them; isolate the change that introduced the regression.",
		cp, ct, bp, bt,
	)
	if bestPlan != nil && len(bestPlan.TargetPaths) > 0 {
		const maxPaths = 10
		paths := bestPlan.TargetPaths
		extra := 0
		if len(paths) > maxPaths {
			extra = len(paths) - maxPaths
			paths = paths[:maxPaths]
		}
		delta += "\n\nFiles modified by the best-scoring earlier plan (the better baseline you regressed FROM):\n"
		for _, p := range paths {
			delta += fmt.Sprintf("  - %s\n", p)
		}
		if extra > 0 {
			delta += fmt.Sprintf("  - … (+%d more)\n", extra)
		}
	}
	if diff := buildPlanContentDiff(bestPlan, curPlan, retryHintDiffMaxBytes); diff != "" {
		delta += "\n\nDiff from the best-scoring earlier plan to your current attempt (`-` = best, `+` = current — the lines marked `+` are what you added that REGRESSED). Revert them or refactor so the best version's behaviour is preserved while still addressing the failing tests:\n```diff\n" + diff + "```\n"
	}
	return base + delta
}

// retryHintDiffMaxBytes caps the unified-diff section appended to
// retry hints on regression. 4 KB fits typical small-file
// edits (an exercism-shape stub diff is usually <500 B; a complex
// refactor diff is usually <2 KB) while leaving headroom for the
// reflector critique + heuristic hint above it. Diffs that exceed
// the cap are truncated mid-hunk with an explicit "(truncated …)"
// marker; the planner still sees the first hunks, which usually
// carry the regression signal.
const retryHintDiffMaxBytes = 4096

// buildPlanContentDiff returns a unified diff between best and current
// plan contents, keyed by overlapping FileChange.Path. Empty string
// when either plan is nil, no paths overlap, or all overlapping paths
// have identical content.
//
// Per FileChange.Kind:
//   - "create" / "modify": diff the two NewContent blobs. Most common
//     case for exercism-shape tasks where the LLM rewrites a stub.
//   - "patch": diff the two Patch payloads (each is itself a unified
//     diff; the resulting "diff of diffs" is admittedly noisy but
//     still informative — you can see which hunks the planner removed
//     or added between iterations).
//   - "delete": no diff produced (kind change between best and current
//     is rare; if it happens, the path-list section above flags it).
//
// Output order: alphabetical by path so the prompt is deterministic
// across runs (otherwise prompt cache invalidates on every regenerate).
//
// The total diff is capped at maxBytes; once the cap is reached, the
// remaining paths are summarized as "(N more files truncated)" so the
// hint stays bounded.
func buildPlanContentDiff(best *types.ChangePlan, current *types.ChangePlan, maxBytes int) string {
	if best == nil || current == nil {
		return ""
	}
	bestByPath := make(map[string]types.FileChange, len(best.Changes))
	for _, c := range best.Changes {
		bestByPath[c.Path] = c
	}
	overlapping := make([]string, 0, len(current.Changes))
	for _, c := range current.Changes {
		if _, ok := bestByPath[c.Path]; ok {
			overlapping = append(overlapping, c.Path)
		}
	}
	if len(overlapping) == 0 {
		return ""
	}
	sort.Strings(overlapping)
	curByPath := make(map[string]types.FileChange, len(current.Changes))
	for _, c := range current.Changes {
		curByPath[c.Path] = c
	}
	var b strings.Builder
	truncatedAt := -1
	for i, p := range overlapping {
		if b.Len() >= maxBytes {
			truncatedAt = i
			break
		}
		bc := bestByPath[p]
		cc := curByPath[p]
		var bestText, curText string
		switch {
		case bc.Kind == "patch" || cc.Kind == "patch":
			// Both kind=patch (or kind transitioned) — diff the patch
			// payloads themselves so the planner sees how the patch
			// content shifted. If exactly one side has Patch and the
			// other has NewContent, fall through to NewContent diff
			// against empty for the patch side (rough but informative).
			bestText = bc.Patch
			curText = cc.Patch
			if bestText == "" {
				bestText = bc.NewContent
			}
			if curText == "" {
				curText = cc.NewContent
			}
		default:
			bestText = bc.NewContent
			curText = cc.NewContent
		}
		if bestText == curText {
			continue
		}
		d := udiff.Unified("best/"+p, "current/"+p, bestText, curText)
		if d == "" {
			continue
		}
		// Keep the diff bounded per file too — a single 10K-line file
		// rewrite shouldn't crowd out other paths.
		const perFileCap = 2048
		if len(d) > perFileCap {
			d = types.CutPrefixRuneSafe(d, perFileCap) + "\n… (per-file diff truncated)\n"
		}
		// If appending d would overflow the total cap, truncate to
		// what fits + a marker.
		if b.Len()+len(d) > maxBytes {
			remaining := maxBytes - b.Len()
			if remaining > 64 {
				b.WriteString(types.CutPrefixRuneSafe(d, remaining))
				b.WriteString("\n… (truncated)\n")
			}
			truncatedAt = i + 1
			break
		}
		b.WriteString(d)
	}
	if truncatedAt > 0 && truncatedAt < len(overlapping) {
		fmt.Fprintf(&b, "\n(+%d more files omitted)\n", len(overlapping)-truncatedAt)
	}
	return b.String()
}
