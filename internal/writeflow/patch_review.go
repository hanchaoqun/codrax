package writeflow

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

var patchReviewEffectHardEventCodes = map[string]bool{
	"patch_effect_path_outside_worktree": true,
	"structured_file_missing":            true,
	"structured_file_parse_error":        true,
}

func ReviewAppliedPatchScope(plan *types.ChangePlan, activeSlice types.ChangePlanSlice) types.PatchReviewRecord {
	record := types.PatchReviewRecord{
		Source:    "post_apply_scope",
		CreatedAt: time.Now(),
	}
	if plan == nil {
		record.Status = "skipped"
		record.Findings = append(record.Findings, patchReviewFinding("missing_plan", types.PatchReviewSeverityWarning, "", "patch review skipped because no ChangePlan was active"))
		return types.NormalizePatchReviewRecord(record)
	}
	record.PlanID = strings.TrimSpace(plan.ID)
	record.SliceID = strings.TrimSpace(activeSlice.ID)
	record.TargetPaths = normalizePatchReviewPaths(plan.TargetPaths)
	record.AllowedPaths = patchReviewAllowedPaths(plan, activeSlice)
	declaredAppliedPaths := patchReviewAppliedPaths(plan)
	effectPaths := patchReviewEffectPaths(plan.PatchEffect)
	record.AppliedPaths = normalizePatchReviewPaths(append(declaredAppliedPaths, effectPaths...))
	if plan.PatchEffect != nil {
		record.PatchEffectID = strings.TrimSpace(plan.PatchEffect.RecordID)
		record.DiffFingerprint = strings.TrimSpace(plan.PatchEffect.DiffFingerprint)
	}
	record.ReviewID = patchReviewID(record.PlanID, record.SliceID)

	targetSet := patchReviewPathSet(record.TargetPaths)
	allowedSet := patchReviewPathSet(record.AllowedPaths)
	for _, applied := range declaredAppliedPaths {
		if len(targetSet) > 0 && !targetSet[applied] {
			record.Findings = append(record.Findings, patchReviewFinding(
				"applied_path_outside_plan_scope",
				types.PatchReviewSeverityError,
				applied,
				"applied path is not declared in ChangePlan.target_paths",
			))
		}
		if len(allowedSet) > 0 && !allowedSet[applied] {
			record.Findings = append(record.Findings, patchReviewFinding(
				"applied_path_outside_active_slice",
				types.PatchReviewSeverityError,
				applied,
				"applied path is outside the active ChangePlan slice",
			))
		}
	}
	for _, applied := range effectPaths {
		if len(targetSet) > 0 && !targetSet[applied] {
			record.Findings = append(record.Findings, patchReviewFinding(
				"patch_effect_path_outside_plan_scope",
				types.PatchReviewSeverityError,
				applied,
				"actual applied diff path is not declared in ChangePlan.target_paths",
			))
		}
		if len(allowedSet) > 0 && !allowedSet[applied] {
			record.Findings = append(record.Findings, patchReviewFinding(
				"patch_effect_path_outside_active_slice",
				types.PatchReviewSeverityError,
				applied,
				"actual applied diff path is outside the active ChangePlan slice",
			))
		}
	}
	for _, finding := range patchReviewEffectEventFindings(plan.PatchEffect) {
		record.Findings = append(record.Findings, finding)
	}
	if len(record.AppliedPaths) == 0 && patchReviewPlanClaimsApplied(plan) {
		record.Findings = append(record.Findings, patchReviewFinding(
			"applied_paths_missing",
			types.PatchReviewSeverityWarning,
			"",
			"plan is in an applied lifecycle state but carries no applied_paths or per-change applied status",
		))
	}
	return types.NormalizePatchReviewRecord(record)
}

func patchReviewAllowedPaths(plan *types.ChangePlan, activeSlice types.ChangePlanSlice) []string {
	if len(activeSlice.Paths) > 0 {
		return normalizePatchReviewPaths(activeSlice.Paths)
	}
	if len(activeSlice.ChangeIndexes) > 0 && plan != nil {
		out := make([]string, 0, len(activeSlice.ChangeIndexes))
		for _, idx := range activeSlice.ChangeIndexes {
			if idx < 0 || idx >= len(plan.Changes) {
				continue
			}
			out = append(out, plan.Changes[idx].Path, plan.Changes[idx].NewPath)
		}
		if normalized := normalizePatchReviewPaths(out); len(normalized) > 0 {
			return normalized
		}
	}
	if plan == nil {
		return nil
	}
	return normalizePatchReviewPaths(plan.TargetPaths)
}

func patchReviewAppliedPaths(plan *types.ChangePlan) []string {
	if plan == nil {
		return nil
	}
	out := normalizePatchReviewPaths(plan.AppliedPaths)
	for _, change := range plan.Changes {
		if change.Apply == nil || strings.TrimSpace(change.Apply.Status) != "applied" {
			continue
		}
		out = append(out, change.Path, change.NewPath)
	}
	return normalizePatchReviewPaths(out)
}

func patchReviewEffectEventFindings(effect *types.PatchEffectRecord) []types.PatchReviewFinding {
	if effect == nil {
		return nil
	}
	var findings []types.PatchReviewFinding
	for _, file := range effect.Files {
		for _, event := range file.Events {
			code := strings.TrimSpace(event.Code)
			if !patchReviewEffectHardEventCodes[code] || strings.TrimSpace(event.Severity) != "error" {
				continue
			}
			path := event.Path
			if path == "" {
				path = file.Path
			}
			findings = append(findings, patchReviewFinding(
				code,
				types.PatchReviewSeverityError,
				path,
				event.Message,
			))
			findings[len(findings)-1].EvidenceRef = strings.TrimSpace(event.EvidenceRef)
		}
	}
	return findings
}

func patchReviewEffectPaths(effect *types.PatchEffectRecord) []string {
	if effect == nil {
		return nil
	}
	out := make([]string, 0, len(effect.Files)*2)
	for _, file := range effect.Files {
		out = append(out, file.Path)
		if file.OldPath != "" && file.OldPath != file.Path {
			out = append(out, file.OldPath)
		}
	}
	return normalizePatchReviewPaths(out)
}

func patchReviewPlanClaimsApplied(plan *types.ChangePlan) bool {
	if plan == nil {
		return false
	}
	switch strings.TrimSpace(plan.Status) {
	case types.PlanStatusAppliedPendingVerify, types.PlanStatusApplied, types.PlanStatusUnverified, types.PlanStatusVerifyFailed:
		return true
	default:
		return false
	}
}

func patchReviewPathSet(paths []string) map[string]bool {
	out := make(map[string]bool, len(paths))
	for _, path := range paths {
		out[path] = true
	}
	return out
}

func normalizePatchReviewPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, raw := range paths {
		path := normalizePatchReviewPath(raw)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func normalizePatchReviewPath(raw string) string {
	path := filepath.ToSlash(strings.TrimSpace(raw))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func patchReviewFinding(code string, severity types.PatchReviewSeverity, path, message string) types.PatchReviewFinding {
	return types.PatchReviewFinding{
		Code:     strings.TrimSpace(code),
		Severity: severity,
		Path:     normalizePatchReviewPath(path),
		Message:  strings.TrimSpace(message),
	}
}

func patchReviewID(planID, sliceID string) string {
	planID = strings.TrimSpace(planID)
	sliceID = strings.TrimSpace(sliceID)
	if sliceID == "" {
		return fmt.Sprintf("patch-review:%s", planID)
	}
	return fmt.Sprintf("patch-review:%s:%s", planID, sliceID)
}
