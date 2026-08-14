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
func attachVerificationWorktreeAudit(parent context.Context, report *types.ChangeReport, baseline verificationWorktreeSnapshot, repoRoot string) {
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
		audit.Status = types.VerificationWorktreeAuditTrackedDrift
		audit.ReasonCode = verificationWorktreeTrackedDriftReason
		report.WorktreeAudit = audit
		markVerificationTrackedWorktreeDrift(report, audit)
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
	paths := verificationWorktreeEffectPaths(audit.Effects, types.VerificationWorktreeEffectTrackedChanged, 8)
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
