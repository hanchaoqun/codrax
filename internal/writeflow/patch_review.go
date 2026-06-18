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
	"duplicate_inserted_block_added":     true,
	"patch_effect_path_outside_worktree": true,
	"python_duplicate_symbol_added":      true,
	"python_top_level_self_method":       true,
	"structured_file_missing":            true,
	"structured_file_parse_error":        true,
}

var patchReviewEffectSoftEventCodes = map[string]bool{
	"caller_return_shape_adapter_added":                true,
	"control_flow_guard_touched":                       true,
	"call_site_touched":                                true,
	"diagnostic_signal_conditionally_suppressed":       true,
	"external_private_state_sync_workaround":           true,
	"state_assignment_touched":                         true,
	"go_nested_string_map_assignment_added":            true,
	"java_chained_string_map_get_added":                true,
	"javascript_nested_string_key_direct_access_added": true,
	"kotlin_chained_string_map_get_added":              true,
	"production_test_scaffold_added":                   true,
	"python_nested_string_key_direct_access_added":     true,
	"ruby_nested_key_direct_access_added":              true,
	"typescript_nested_string_key_direct_access_added": true,
}

var patchReviewEffectUnknownCoverageEventCodes = map[string]bool{
	"caller_return_shape_adapter_added":                true,
	"diagnostic_signal_conditionally_suppressed":       true,
	"external_private_state_sync_workaround":           true,
	"go_nested_string_map_assignment_added":            true,
	"java_chained_string_map_get_added":                true,
	"javascript_nested_string_key_direct_access_added": true,
	"kotlin_chained_string_map_get_added":              true,
	"production_test_scaffold_added":                   true,
	"python_nested_string_key_direct_access_added":     true,
	"ruby_nested_key_direct_access_added":              true,
	"typescript_nested_string_key_direct_access_added": true,
}

const patchReviewMaxSemanticFindings = 32

// PatchReviewEffectUnknownCoverage reports whether code names an actual-diff
// effect that must stay uncovered until a typed verifier signal proves the
// boundary. It lets controller-side coverage projection consume the same
// language registry as PatchReview without duplicating event names.
func PatchReviewEffectUnknownCoverage(code string) bool {
	return patchReviewEffectUnknownCoverageEventCodes[strings.TrimSpace(code)]
}

type SemanticPatchReviewInput struct {
	Plan              *types.ChangePlan
	ActiveSlice       types.ChangePlanSlice
	ImpactAnalysis    *types.ImpactAnalysisResult
	ImpactObligations *types.ImpactObligationSet
	ConventionGraph   *types.ConventionGraph
}

func ReviewAppliedPatchScope(plan *types.ChangePlan, activeSlice types.ChangePlanSlice) types.PatchReviewRecord {
	return ReviewAppliedPatchSemantic(SemanticPatchReviewInput{
		Plan:        plan,
		ActiveSlice: activeSlice,
	})
}

func ReviewAppliedPatchSemantic(in SemanticPatchReviewInput) types.PatchReviewRecord {
	plan := in.Plan
	record := types.PatchReviewRecord{
		Source:    "post_apply_semantic",
		CreatedAt: time.Now(),
	}
	if plan == nil {
		record.Status = "skipped"
		record.Findings = append(record.Findings, patchReviewFindingWithCategory("missing_plan", types.PatchReviewSeverityWarning, types.PatchReviewCategoryScope, "", "patch review skipped because no ChangePlan was active"))
		return types.NormalizePatchReviewRecord(record)
	}
	record.PlanID = strings.TrimSpace(plan.ID)
	record.SliceID = strings.TrimSpace(in.ActiveSlice.ID)
	record.TargetPaths = normalizePatchReviewPaths(plan.TargetPaths)
	record.AllowedPaths = patchReviewAllowedPaths(plan, in.ActiveSlice)
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
			record.Findings = append(record.Findings, patchReviewFindingWithCategory(
				"applied_path_outside_plan_scope",
				types.PatchReviewSeverityError,
				types.PatchReviewCategoryScope,
				applied,
				"applied path is not declared in ChangePlan.target_paths",
			))
		}
		if len(allowedSet) > 0 && !allowedSet[applied] {
			record.Findings = append(record.Findings, patchReviewFindingWithCategory(
				"applied_path_outside_active_slice",
				types.PatchReviewSeverityError,
				types.PatchReviewCategoryScope,
				applied,
				"applied path is outside the active ChangePlan slice",
			))
		}
	}
	for _, applied := range effectPaths {
		if len(targetSet) > 0 && !targetSet[applied] {
			record.Findings = append(record.Findings, patchReviewFindingWithCategory(
				"patch_effect_path_outside_plan_scope",
				types.PatchReviewSeverityError,
				types.PatchReviewCategoryScope,
				applied,
				"actual applied diff path is not declared in ChangePlan.target_paths",
			))
		}
		if len(allowedSet) > 0 && !allowedSet[applied] {
			record.Findings = append(record.Findings, patchReviewFindingWithCategory(
				"patch_effect_path_outside_active_slice",
				types.PatchReviewSeverityError,
				types.PatchReviewCategoryScope,
				applied,
				"actual applied diff path is outside the active ChangePlan slice",
			))
		}
	}
	for _, finding := range patchReviewEffectEventFindings(plan.PatchEffect) {
		record.Findings = append(record.Findings, finding)
	}
	obligations := in.ImpactObligations
	if obligations == nil && in.ImpactAnalysis != nil {
		obligations = in.ImpactAnalysis.ObligationSet
	}
	if obligations == nil {
		obligations = plan.ImpactObligations
	}
	if obligations == nil && plan.ImpactAnalysis != nil {
		obligations = plan.ImpactAnalysis.ObligationSet
	}
	for _, finding := range patchReviewSemanticCoverageFindings(obligations) {
		record.Findings = append(record.Findings, finding)
	}
	for _, finding := range patchReviewConventionFindings(in.ConventionGraph, record.AppliedPaths, obligations) {
		record.Findings = append(record.Findings, finding)
	}
	if len(record.AppliedPaths) == 0 && patchReviewPlanClaimsApplied(plan) {
		record.Findings = append(record.Findings, patchReviewFindingWithCategory(
			"applied_paths_missing",
			types.PatchReviewSeverityWarning,
			types.PatchReviewCategoryScope,
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
			severity := types.PatchReviewSeverity("")
			switch {
			case patchReviewEffectHardEventCodes[code] && strings.TrimSpace(event.Severity) == "error":
				severity = types.PatchReviewSeverityError
			case patchReviewEffectSoftEventCodes[code] && strings.TrimSpace(event.Severity) == "warning":
				severity = types.PatchReviewSeverityWarning
			default:
				continue
			}
			path := event.Path
			if path == "" {
				path = file.Path
			}
			category := types.PatchReviewCategoryStructural
			if severity == types.PatchReviewSeverityWarning {
				category = types.PatchReviewCategorySemanticCoverage
			}
			findings = append(findings, patchReviewFindingWithCategory(
				code,
				severity,
				category,
				path,
				event.Message,
			))
			findings[len(findings)-1].EvidenceRef = strings.TrimSpace(event.EvidenceRef)
			if patchReviewEffectUnknownCoverageEventCodes[code] {
				findings[len(findings)-1].CoverageStatus = types.PatchReviewCoverageUnknown
				findings[len(findings)-1].ImpactKind = types.PatchReviewImpactKindEffectFollowup
			}
		}
	}
	return findings
}

func patchReviewSemanticCoverageFindings(obligations *types.ImpactObligationSet) []types.PatchReviewFinding {
	if obligations == nil {
		return nil
	}
	set := types.NormalizeImpactObligationSet(*obligations)
	seen := map[string]bool{}
	var findings []types.PatchReviewFinding
	for _, ob := range set.Obligations {
		code, message := patchReviewSemanticCoverageCode(ob)
		if code == "" {
			continue
		}
		path := ob.SubjectPath
		if path == "" {
			path = ob.RelatedPath
		}
		finding := patchReviewFindingWithCategory(code, types.PatchReviewSeverityWarning, types.PatchReviewCategorySemanticCoverage, path, message)
		finding.Relation = strings.TrimSpace(ob.Relation)
		finding.ImpactKind = strings.TrimSpace(ob.Kind)
		finding.RelatedPath = normalizePatchReviewPath(ob.RelatedPath)
		finding.SubjectSymbol = strings.TrimSpace(ob.SubjectSymbol)
		finding.Strength = strings.TrimSpace(string(ob.Strength))
		finding.CoverageStatus = types.PatchReviewCoverageUnverified
		finding.EvidenceRef = strings.TrimSpace(ob.EvidenceRef)
		key := strings.Join([]string{
			finding.Code,
			finding.Path,
			finding.RelatedPath,
			finding.SubjectSymbol,
			finding.EvidenceRef,
		}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		findings = append(findings, finding)
		if len(findings) >= patchReviewMaxSemanticFindings {
			break
		}
	}
	return findings
}

func patchReviewSemanticCoverageCode(ob types.ImpactObligation) (string, string) {
	switch strings.TrimSpace(ob.Kind) {
	case "changed_symbol":
		return "changed_symbol_without_probe_coverage", "changed symbol from the actual patch needs typed verification coverage"
	case "dependent":
		return "dependent_surface_without_verify_coverage", "dependent surface from impact analysis needs typed verification coverage"
	case "test_surface":
		return "related_test_surface_unverified", "related test surface from impact analysis has not been verified"
	case "behavior_contract":
		return "behavior_contract_without_verify_coverage", "declared behavior contract needs typed verification coverage"
	default:
		return "", ""
	}
}

func patchReviewConventionFindings(graph *types.ConventionGraph, appliedPaths []string, obligations *types.ImpactObligationSet) []types.PatchReviewFinding {
	if graph == nil {
		return nil
	}
	normalized := types.NormalizeConventionGraph(*graph)
	pathSet := patchReviewPathSet(appliedPaths)
	symbolSet := patchReviewImpactSymbolSet(obligations)
	seen := map[string]bool{}
	var findings []types.PatchReviewFinding
	for _, node := range normalized.Nodes {
		source := normalizePatchReviewPath(node.Source)
		symbol := strings.TrimSpace(firstNonEmptyPatchReviewString(node.AnchorSymbol, node.Subject))
		if source == "" && symbol == "" {
			continue
		}
		if source != "" && !pathSet[source] && symbol != "" && !symbolSet[symbol] {
			continue
		}
		if source != "" && !pathSet[source] && symbol == "" {
			continue
		}
		if symbol != "" && !symbolSet[symbol] && source == "" {
			continue
		}
		finding := patchReviewFindingWithCategory(
			"convention_surface_available",
			types.PatchReviewSeverityInfo,
			types.PatchReviewCategoryConvention,
			source,
			"repository convention context is available for the actual patch surface",
		)
		finding.Relation = string(node.Category)
		finding.SubjectSymbol = symbol
		finding.Strength = strings.TrimSpace(node.Strength)
		finding.CoverageStatus = types.PatchReviewCoverageAdvisory
		finding.SourceStage = strings.TrimSpace(node.SourceStage)
		finding.LineStart = node.LineStart
		finding.LineEnd = node.LineEnd
		finding.ContextSummary = strings.TrimSpace(node.Summary)
		finding.EvidenceRef = strings.TrimSpace(node.EvidenceRefID)
		key := strings.Join([]string{finding.Code, finding.Path, finding.SubjectSymbol, finding.Relation, finding.SourceStage, finding.EvidenceRef}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		findings = append(findings, finding)
		if len(findings) >= patchReviewMaxSemanticFindings {
			break
		}
	}
	return findings
}

func patchReviewImpactSymbolSet(obligations *types.ImpactObligationSet) map[string]bool {
	out := map[string]bool{}
	if obligations == nil {
		return out
	}
	set := types.NormalizeImpactObligationSet(*obligations)
	for _, ob := range set.Obligations {
		if symbol := strings.TrimSpace(ob.SubjectSymbol); symbol != "" {
			out[symbol] = true
		}
	}
	return out
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

func patchReviewFindingWithCategory(code string, severity types.PatchReviewSeverity, category types.PatchReviewFindingCategory, path, message string) types.PatchReviewFinding {
	finding := patchReviewFinding(code, severity, path, message)
	finding.Category = category
	return finding
}

func patchReviewID(planID, sliceID string) string {
	planID = strings.TrimSpace(planID)
	sliceID = strings.TrimSpace(sliceID)
	if sliceID == "" {
		return fmt.Sprintf("patch-review:%s", planID)
	}
	return fmt.Sprintf("patch-review:%s:%s", planID, sliceID)
}

func firstNonEmptyPatchReviewString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
