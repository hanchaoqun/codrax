package tool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	verificationWorktreeTrackedDriftReason = "verification_tracked_worktree_drift"
	verificationWorktreeAuditUnavailable   = "verification_worktree_audit_unavailable"
)

type verificationWorktreeEntry struct {
	status      string
	fingerprint string
}

type verificationWorktreeSnapshot struct {
	applicable  bool
	unavailable bool
	reasonCode  string
	root        string
	tracked     map[string]verificationWorktreeEntry
	untracked   map[string]struct{}
}

// captureVerificationWorktreeSnapshot reads git's NUL-delimited porcelain
// surface. Only dirty tracked paths need fingerprints: a clean baseline plus a
// post-run dirty row is already a complete witness, while fingerprints also
// catch a runner changing a path that was dirty before verification.
func captureVerificationWorktreeSnapshot(parent context.Context, repoRoot string) verificationWorktreeSnapshot {
	out := verificationWorktreeSnapshot{
		tracked:   map[string]verificationWorktreeEntry{},
		untracked: map[string]struct{}{},
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return out
	}
	auditRoot, ok := verificationWorktreeGitRootCandidate(root)
	if !ok {
		return out
	}
	out.applicable = true
	out.root = auditRoot
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	probe.Dir = auditRoot
	probeOut, probeErr := probe.Output()
	if probeErr != nil || strings.TrimSpace(string(probeOut)) != "true" {
		out.unavailable = true
		out.reasonCode = verificationWorktreeAuditUnavailable
		return out
	}

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "-z", "--untracked-files=all")
	cmd.Dir = auditRoot
	payload, err := cmd.Output()
	if err != nil {
		out.unavailable = true
		out.reasonCode = verificationWorktreeAuditUnavailable
		return out
	}
	fields := bytes.Split(payload, []byte{0})
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if len(field) < 4 || field[2] != ' ' {
			continue
		}
		status := string(field[:2])
		path := normalizeVerificationWorktreePath(string(field[3:]))
		if path == "" {
			continue
		}
		if status == "??" {
			out.untracked[path] = struct{}{}
		} else {
			out.tracked[path] = verificationWorktreeEntry{
				status:      status,
				fingerprint: verificationWorktreePathFingerprint(auditRoot, path),
			}
		}
		// In porcelain -z form, rename/copy rows are followed by the source
		// path as a second NUL field without an XY prefix.
		if status[0] == 'R' || status[0] == 'C' || status[1] == 'R' || status[1] == 'C' {
			i++
		}
	}
	return out
}

// verificationWorktreeGitRootCandidate proves applicability without invoking
// git. Ordinary non-git fixtures, including those whose PATH intentionally
// omits git, therefore keep their historical verification semantics.
func verificationWorktreeGitRootCandidate(root string) (string, bool) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	cur := filepath.Clean(abs)
	for {
		if _, err := os.Lstat(filepath.Join(cur, ".git")); err == nil {
			return cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

func normalizeVerificationWorktreePath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "./")
	if value == "" || value == "." || value == ".git" || strings.HasPrefix(value, ".git/") {
		return ""
	}
	return value
}

func verificationWorktreePathFingerprint(root, rel string) string {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "unavailable"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(abs)
		if err != nil {
			return "symlink:unavailable"
		}
		sum := sha256.Sum256([]byte(target))
		return "symlink:" + hex.EncodeToString(sum[:])
	}
	if !info.Mode().IsRegular() {
		return fmt.Sprintf("mode:%s:size:%d", info.Mode().String(), info.Size())
	}
	f, err := os.Open(abs)
	if err != nil {
		return "file:unavailable"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "file:unavailable"
	}
	return "file:" + hex.EncodeToString(h.Sum(nil))
}

func verificationWorktreeEffects(before, after verificationWorktreeSnapshot) []types.VerificationWorktreeEffect {
	var effects []types.VerificationWorktreeEffect
	for path, post := range after.tracked {
		pre, ok := before.tracked[path]
		if ok && pre.status == post.status && pre.fingerprint == post.fingerprint {
			continue
		}
		effects = append(effects, types.VerificationWorktreeEffect{
			Path:         path,
			Kind:         types.VerificationWorktreeEffectTrackedChanged,
			BeforeStatus: pre.status,
			AfterStatus:  post.status,
			Ownership:    "git_tracked",
			Action:       "verification_failed_retained_for_review",
		})
	}
	for path, pre := range before.tracked {
		if _, ok := after.tracked[path]; ok {
			continue
		}
		effects = append(effects, types.VerificationWorktreeEffect{
			Path:         path,
			Kind:         types.VerificationWorktreeEffectTrackedChanged,
			BeforeStatus: pre.status,
			AfterStatus:  "clean",
			Ownership:    "git_tracked",
			Action:       "verification_failed_retained_for_review",
		})
	}
	for path := range after.untracked {
		if _, existed := before.untracked[path]; existed {
			continue
		}
		effects = append(effects, types.VerificationWorktreeEffect{
			Path:        path,
			Kind:        types.VerificationWorktreeEffectUntrackedCreated,
			AfterStatus: "??",
			Ownership:   "unproven_generated_artifact",
			Action:      "retained_not_committed_not_auto_deleted",
		})
	}
	sort.Slice(effects, func(i, j int) bool {
		if effects[i].Kind != effects[j].Kind {
			return effects[i].Kind < effects[j].Kind
		}
		return effects[i].Path < effects[j].Path
	})
	return effects
}

// attachVerificationWorktreeAudit turns the before/after snapshot into report
// truth. Tracked drift invalidates the verification verdict; untracked output
// remains a disclosed side effect without pretending the production patch or
// tests failed. No path is ever removed here.
func attachVerificationWorktreeAudit(parent context.Context, report *types.ChangeReport, baseline verificationWorktreeSnapshot, repoRoot string, drift verificationWorktreeDriftInput) {
	if report == nil || !baseline.applicable {
		return
	}
	audit := &types.VerificationWorktreeAudit{
		BaselineDirtyTracked: len(baseline.tracked),
		BaselineUntracked:    len(baseline.untracked),
	}
	if baseline.unavailable {
		audit.Status = types.VerificationWorktreeAuditUnavailable
		audit.ReasonCode = firstNonEmptyRunTests(baseline.reasonCode, verificationWorktreeAuditUnavailable)
		report.WorktreeAudit = audit
		markVerificationWorktreeAuditUnavailable(report)
		return
	}
	after := captureVerificationWorktreeSnapshot(parent, repoRoot)
	if !after.applicable || after.unavailable {
		audit.Status = types.VerificationWorktreeAuditUnavailable
		audit.ReasonCode = firstNonEmptyRunTests(after.reasonCode, verificationWorktreeAuditUnavailable)
		report.WorktreeAudit = audit
		markVerificationWorktreeAuditUnavailable(report)
		return
	}
	audit.Effects = verificationWorktreeEffects(baseline, after)
	for _, effect := range audit.Effects {
		switch effect.Kind {
		case types.VerificationWorktreeEffectTrackedChanged:
			audit.TrackedEffectCount++
		case types.VerificationWorktreeEffectUntrackedCreated:
			audit.UntrackedEffectCount++
		}
	}
	switch {
	case audit.TrackedEffectCount > 0:
		// V5-2: classify each tracked row; disclose owned classes, refuse
		// unclassified drift (run_tests_worktree_drift.go).
		if strings.TrimSpace(drift.repoRoot) == "" {
			drift.repoRoot = repoRoot
		}
		decideVerificationTrackedDrift(parent, report, audit, baseline, drift)
	case audit.UntrackedEffectCount > 0:
		audit.Status = types.VerificationWorktreeAuditUntrackedSideEffects
		audit.ReasonCode = "verification_untracked_outputs_retained"
		report.WorktreeAudit = audit
	default:
		audit.Status = types.VerificationWorktreeAuditClean
		report.WorktreeAudit = audit
	}
}

func markVerificationWorktreeAuditUnavailable(report *types.ChangeReport) {
	report.Passed = false
	report.FailureKind = types.FailureKindVerificationIncomplete
	report.FailureReasonCode = appendVerificationReasonCode(report.FailureReasonCode, verificationWorktreeAuditUnavailable)
	report.FailureSummary = appendVerificationFailureSummary(report.FailureSummary,
		"verification worktree integrity audit was unavailable; test results are retained but a clean post-run tree was not proven")
	report.VerificationDiagnostics = mergeVerificationDiagnostics(report.VerificationDiagnostics, []types.VerificationDiagnostic{{
		Source: "git_worktree_audit", Category: "worktree_integrity", Severity: "warning",
		ReasonCode: verificationWorktreeAuditUnavailable, Outcome: "unavailable",
	}})
}

func markVerificationTrackedWorktreeDrift(report *types.ChangeReport, audit *types.VerificationWorktreeAudit) {
	paths := verificationWorktreeRefusedEffectPaths(audit.Effects, 8)
	detail := "verification command changed tracked worktree path(s): " + strings.Join(paths, ", ")
	report.Passed = false
	report.FailureKind = types.FailureKindVerificationSideEffect
	report.FailureReasonCode = appendVerificationReasonCode(report.FailureReasonCode, verificationWorktreeTrackedDriftReason)
	report.FailureSummary = appendVerificationFailureSummary(report.FailureSummary, detail)
	report.TestResults = append(report.TestResults, types.TestResult{
		Kind: types.TestResultKindUnit, AssertionID: "verification_worktree_integrity",
		Suite: "verification", Passed: false, FailureDetail: detail,
	})
	report.VerificationDiagnostics = mergeVerificationDiagnostics(report.VerificationDiagnostics, []types.VerificationDiagnostic{{
		Source: "git_worktree_audit", Category: "worktree_integrity", Severity: "error",
		ReasonCode: verificationWorktreeTrackedDriftReason, Outcome: "tracked_drift", Detail: detail,
	}})
}

// verificationWorktreeRefusedEffectPaths lists the tracked rows the gate
// refused (a row without a disposition is a legacy refused row); disclosed
// rows never appear in a failure summary, so a planner is not asked to
// "fix" a lockfile the toolchain owns.
func verificationWorktreeRefusedEffectPaths(effects []types.VerificationWorktreeEffect, limit int) []string {
	var paths []string
	for _, effect := range effects {
		if effect.Kind != types.VerificationWorktreeEffectTrackedChanged || effect.Disposition == types.VerificationWorktreeEffectDisclosed {
			continue
		}
		if strings.TrimSpace(effect.Path) == "" {
			continue
		}
		paths = append(paths, effect.Path)
		if limit > 0 && len(paths) >= limit {
			break
		}
	}
	return paths
}

func verificationWorktreeEffectPaths(effects []types.VerificationWorktreeEffect, kind types.VerificationWorktreeEffectKind, limit int) []string {
	var paths []string
	for _, effect := range effects {
		if effect.Kind != kind || strings.TrimSpace(effect.Path) == "" {
			continue
		}
		paths = append(paths, effect.Path)
		if limit > 0 && len(paths) >= limit {
			break
		}
	}
	return paths
}

func appendVerificationReasonCode(current, code string) string {
	parts := splitFailureReasonCodes(current)
	for _, part := range parts {
		if part == code {
			return strings.Join(parts, ",")
		}
	}
	parts = append(parts, code)
	return strings.Join(parts, ",")
}

func appendVerificationFailureSummary(current, detail string) string {
	current = strings.TrimSpace(current)
	detail = strings.TrimSpace(detail)
	if current == "" {
		return detail
	}
	if detail == "" || strings.Contains(current, detail) {
		return current
	}
	return current + " | " + detail
}

// decideVerificationTrackedDrift classifies every tracked effect and either
// discloses (all rows owned by a disclosed class and every lockfile owner's
// locked re-verify passed) or refuses (any unclassified row, or a failed
// re-verify). Disclosed-class rows keep their class/disposition even inside a
// refused run so a planner never "fixes" a lockfile.
func decideVerificationTrackedDrift(parent context.Context, report *types.ChangeReport, audit *types.VerificationWorktreeAudit, baseline verificationWorktreeSnapshot, in verificationWorktreeDriftInput) {
	roster := verificationDriftRoster(in, baseline.root)
	declared := verificationDriftDeclaredOutputs(in.plan)
	var refusedPaths []string
	lockfileOwners := map[string]verificationDriftRosterEntry{}
	for i := range audit.Effects {
		effect := &audit.Effects[i]
		if effect.Kind != types.VerificationWorktreeEffectTrackedChanged {
			continue
		}
		decision := classifyVerificationWorktreeDrift(*effect, in, baseline, roster, declared)
		effect.DriftClass = decision.class
		effect.OwnerRunner = decision.ownerRunner
		effect.OwnerWorkingDir = decision.workingDir
		if types.VerificationWorktreeDriftDisclosed(decision.class) {
			effect.Disposition = types.VerificationWorktreeEffectDisclosed
			effect.Action = "disclosed_not_committed_not_auto_reverted"
			audit.DisclosedTrackedEffectCount++
			if decision.class == types.VerificationWorktreeDriftDependencyLockfileRefresh {
				key := decision.ownerRunner + "\x00" + decision.workingDir
				lockfileOwners[key] = verificationDriftRosterEntry{runner: decision.ownerRunner, framework: decision.framework, suite: decision.suite, dirRel: decision.workingDir}
			}
			continue
		}
		effect.Disposition = types.VerificationWorktreeEffectRefused
		audit.RefusedTrackedEffectCount++
		refusedPaths = append(refusedPaths, effect.Path)
	}
	sort.Strings(refusedPaths)
	if len(refusedPaths) > 0 {
		audit.Status = types.VerificationWorktreeAuditTrackedDrift
		audit.ReasonCode = verificationWorktreeTrackedDriftReason
		report.WorktreeAudit = audit
		markVerificationTrackedWorktreeDrift(report, audit) // detail lists refused rows only (helper filters Disposition)
		return
	}
	// Every row is disclosed-class: prove lockfile fixed points — but only on
	// a passing report. A failing suite would fail its locked re-run for its
	// own reasons; that exit code says nothing about the lockfile, and the
	// primary failure must keep its own kind and summary.
	keys := make([]string, 0, len(lockfileOwners))
	for key := range lockfileOwners {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	failedOwners := map[string]bool{}
	reportPassing := report.Passed && report.NormalizeVerificationStatus() == types.VerificationStatusPassed
	for _, key := range keys {
		owner := lockfileOwners[key]
		if !reportPassing {
			audit.LockedReverify = append(audit.LockedReverify, types.VerificationLockedReverify{
				Runner: owner.runner, Framework: owner.framework, WorkingDir: owner.dirRel, Outcome: "skipped_report_failed",
			})
			continue
		}
		record := runVerificationLockedReverify(in, baseline.root, owner)
		audit.LockedReverify = append(audit.LockedReverify, record)
		if record.Outcome != "passed" {
			failedOwners[key] = true
		}
	}
	if len(failedOwners) > 0 {
		// Only the rows of an owner whose locked re-run failed are refused;
		// a proven fixed point stays disclosed.
		for i := range audit.Effects {
			effect := &audit.Effects[i]
			if effect.DriftClass != types.VerificationWorktreeDriftDependencyLockfileRefresh {
				continue
			}
			if !failedOwners[effect.OwnerRunner+"\x00"+effect.OwnerWorkingDir] {
				continue
			}
			effect.Disposition = types.VerificationWorktreeEffectRefused
			effect.Action = "verification_failed_retained_for_review"
			audit.DisclosedTrackedEffectCount--
			audit.RefusedTrackedEffectCount++
		}
		audit.Status = types.VerificationWorktreeAuditTrackedDrift
		audit.ReasonCode = verificationWorktreeTrackedDriftReason + "," + types.VerificationLockedReverifyFailedReason
		report.WorktreeAudit = audit
		markVerificationTrackedWorktreeDrift(report, audit)
		report.FailureReasonCode = appendVerificationReasonCode(report.FailureReasonCode, types.VerificationLockedReverifyFailedReason)
		return
	}
	audit.Status = types.VerificationWorktreeAuditTrackedDriftDisclosed
	audit.ReasonCode = types.VerificationTrackedSideEffectDisclosedReason
	report.WorktreeAudit = audit
	detail := verificationWorktreeDisclosedDetail(audit)
	report.VerificationDiagnostics = mergeVerificationDiagnostics(report.VerificationDiagnostics, []types.VerificationDiagnostic{{
		Source: "git_worktree_audit", Category: "worktree_integrity", Severity: "warning",
		ReasonCode: types.VerificationTrackedSideEffectDisclosedReason, Outcome: "tracked_drift_disclosed", Detail: detail,
	}})
	report.VerificationConfidence = mergeVerificationConfidenceRecords(report.VerificationConfidence, []types.VerificationConfidenceRecord{{
		Source: "git_worktree_audit", Category: "worktree_side_effect", Status: "advisory", Severity: "warning",
		ReasonCode: types.VerificationTrackedSideEffectDisclosedReason, Detail: detail,
	}})
}

func verificationWorktreeDisclosedDetail(audit *types.VerificationWorktreeAudit) string {
	var parts []string
	for _, effect := range audit.Effects {
		if effect.Kind != types.VerificationWorktreeEffectTrackedChanged {
			continue
		}
		part := effect.Path + "=" + string(effect.DriftClass)
		if effect.OwnerRunner != "" {
			part += "(" + effect.OwnerRunner + ")"
		}
		parts = append(parts, part)
		if len(parts) >= 8 {
			break
		}
	}
	detail := "verification changed tracked path(s) owned by a disclosed side-effect class: " + strings.Join(parts, ", ") +
		"; not part of the delivery commit, not auto-reverted"
	if audit.UntrackedEffectCount > 0 {
		detail += "; untracked verification output retained, not committed, not auto-deleted: " +
			strings.Join(verificationWorktreeEffectPaths(audit.Effects, types.VerificationWorktreeEffectUntrackedCreated, 8), ", ")
	}
	passed := 0
	for _, record := range audit.LockedReverify {
		if record.Outcome == "passed" {
			passed++
		}
	}
	if passed > 0 {
		detail += "; locked re-verify passed"
	}
	return detail
}
