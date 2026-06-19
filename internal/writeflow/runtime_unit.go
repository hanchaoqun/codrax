package writeflow

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RuntimeUnitView is the deterministic edit/run/observe unit consumed by the
// online write loop. It is projected from durable workflow slices and typed
// artifacts only; model prose, progress strings, and logs never authorize hard
// routing.
type RuntimeUnitView struct {
	RunID   string `json:"run_id,omitempty"`
	BatchID string `json:"batch_id,omitempty"`
	PlanID  string `json:"plan_id,omitempty"`

	UnitID string `json:"unit_id,omitempty"`
	Index  int    `json:"index,omitempty"`
	Total  int    `json:"total,omitempty"`
	Active bool   `json:"active,omitempty"`

	Status         types.ChangePlanSliceStatus `json:"status,omitempty"`
	ChangeIndexes  []int                       `json:"change_indexes,omitempty"`
	Paths          []string                    `json:"paths,omitempty"`
	DependsOnUnits []string                    `json:"depends_on_units,omitempty"`

	ContractRefs       []string                       `json:"contract_refs,omitempty"`
	ChangedSymbolRefs  []string                       `json:"changed_symbol_refs,omitempty"`
	VerificationProbes []string                       `json:"verification_probes,omitempty"`
	OwnerAnchors       []types.OwnerAnchorViewItem    `json:"owner_anchors,omitempty"`
	ImpactObligations  []types.ImpactObligation       `json:"impact_obligations,omitempty"`
	Permission         RuntimeUnitPermissionView      `json:"permission,omitempty"`
	PatchTruth         RuntimeUnitPatchTruthView      `json:"patch_truth,omitempty"`
	PatchReview        RuntimeUnitPatchReviewView     `json:"patch_review,omitempty"`
	Observation        ObservationAuthorityView       `json:"observation,omitempty"`
	Completion         *types.WriteWorkflowCompletion `json:"completion,omitempty"`
	Checkpoint         *types.WriteWorkflowCheckpoint `json:"checkpoint,omitempty"`
	Restore            *types.WriteWorkflowRestore    `json:"restore,omitempty"`
}

// RuntimeUnitPermissionView is the unit-scoped projection of the plan approval
// authority. Approval remains plan-wide today; this view makes that explicit so
// future per-unit approval can extend the same typed lane.
type RuntimeUnitPermissionView struct {
	State        ApprovalExecutionState `json:"state,omitempty"`
	Action       ApprovalAction         `json:"action,omitempty"`
	UserDecision string                 `json:"user_decision,omitempty"`
	RiskLevel    string                 `json:"risk_level,omitempty"`
	ReasonCode   string                 `json:"reason_code,omitempty"`
	CanApply     bool                   `json:"can_apply,omitempty"`
	CanAutoApply bool                   `json:"can_auto_apply,omitempty"`
	RequiresUser bool                   `json:"requires_user,omitempty"`
	Denied       bool                   `json:"denied,omitempty"`
}

// RuntimeUnitPatchTruthView is the factual actual-diff/checkpoint surface for
// one runtime unit.
type RuntimeUnitPatchTruthView struct {
	HasActualDiff   bool     `json:"has_actual_diff,omitempty"`
	PatchEffectID   string   `json:"patch_effect_id,omitempty"`
	DiffFingerprint string   `json:"diff_fingerprint,omitempty"`
	HeadRef         string   `json:"head_ref,omitempty"`
	AppliedPaths    []string `json:"applied_paths,omitempty"`
	ApplyRef        string   `json:"apply_ref,omitempty"`
	CheckpointRef   string   `json:"checkpoint_ref,omitempty"`
	CheckpointSHA   string   `json:"checkpoint_sha,omitempty"`
	RestoreRef      string   `json:"restore_ref,omitempty"`
	RestoreReason   string   `json:"restore_reason,omitempty"`
}

type RuntimeUnitPatchReviewView struct {
	ReviewID        string                           `json:"review_id,omitempty"`
	Verdict         types.PatchReviewCoverageVerdict `json:"verdict,omitempty"`
	HardBlock       bool                             `json:"hard_block,omitempty"`
	ReasonCodes     []string                         `json:"reason_codes,omitempty"`
	UnverifiedKinds []string                         `json:"unverified_kinds,omitempty"`
	BlockReason     string                           `json:"block_reason,omitempty"`
	TotalFindings   int                              `json:"total_findings,omitempty"`
	ErrorFindings   int                              `json:"error_findings,omitempty"`
	PatchEffectID   string                           `json:"patch_effect_id,omitempty"`
	DiffFingerprint string                           `json:"diff_fingerprint,omitempty"`
}

func DeriveRuntimeUnitViews(mode types.PipelineMode, run types.WriteWorkflowRun, plan *types.ChangePlan, report *types.ChangeReport) []RuntimeUnitView {
	_ = mode.Normalize()
	run = types.NormalizeWriteWorkflowRun(run)
	batch, ok := workflowExecutionActiveBatch(run)
	if !ok {
		return nil
	}
	activePlan := workflowExecutionActivePlan(batch, plan)
	sourceSlices := runtimeUnitSourceSlices(batch, activePlan)
	if len(sourceSlices) == 0 {
		return nil
	}
	planSlices := runtimeUnitPlanSliceByID(activePlan)
	ownerAnchors := runtimeUnitOwnerAnchorsByPath(activePlan)
	obligations := runtimeUnitImpactObligationsBySlice(activePlan)
	permission := runtimeUnitPermission(activePlan)
	activeID := strings.TrimSpace(batch.ActiveSliceID)
	if activeID == "" {
		activeID, _ = workflowExecutionActiveSlice(batch)
	}
	out := make([]RuntimeUnitView, 0, len(sourceSlices))
	for i, slice := range sourceSlices {
		id := strings.TrimSpace(slice.ID)
		unit := RuntimeUnitView{
			RunID:          strings.TrimSpace(run.RunID),
			BatchID:        strings.TrimSpace(batch.ID),
			PlanID:         firstNonEmptyRuntimeUnit(slice.PlanID, batch.PlanID),
			UnitID:         id,
			Index:          i + 1,
			Total:          len(sourceSlices),
			Active:         id != "" && id == activeID,
			Status:         slice.Status,
			ChangeIndexes:  append([]int(nil), slice.ChangeIndexes...),
			Paths:          dedupTrimRuntimeUnitStrings(slice.Paths),
			DependsOnUnits: dedupTrimRuntimeUnitStrings(slice.DependsOnSlices),
			Permission:     permission,
			Completion:     cloneRuntimeUnitCompletion(slice.Completion),
		}
		if slice.Checkpoint != nil {
			cp := *slice.Checkpoint
			unit.Checkpoint = &cp
		}
		if slice.Restore != nil {
			restore := *slice.Restore
			unit.Restore = &restore
		}
		if planSlice, ok := planSlices[id]; ok {
			unit.ContractRefs = dedupTrimRuntimeUnitStrings(planSlice.ContractRefs)
			unit.ChangedSymbolRefs = dedupTrimRuntimeUnitStrings(planSlice.ChangedSymbolRefs)
			unit.VerificationProbes = dedupTrimRuntimeUnitStrings(planSlice.VerificationProbes)
			if len(unit.Paths) == 0 {
				unit.Paths = dedupTrimRuntimeUnitStrings(planSlice.Paths)
			}
		}
		unit.OwnerAnchors = runtimeUnitOwnerAnchorsForPaths(ownerAnchors, unit.Paths, id)
		unit.ImpactObligations = runtimeUnitObligationsForSlice(obligations, id, unit.Paths)
		unit.PatchTruth = runtimeUnitPatchTruth(activePlan, slice, len(sourceSlices))
		unit.PatchReview = runtimeUnitPatchReview(activePlan, slice, len(sourceSlices))
		unit.Observation = runtimeUnitObservation(slice, activePlan, report, unit.Active)
		out = append(out, unit)
	}
	return out
}

func ActiveRuntimeUnitView(mode types.PipelineMode, run types.WriteWorkflowRun, plan *types.ChangePlan, report *types.ChangeReport) (RuntimeUnitView, bool) {
	units := DeriveRuntimeUnitViews(mode, run, plan, report)
	for _, unit := range units {
		if unit.Active {
			return unit, true
		}
	}
	if len(units) > 0 {
		return units[0], true
	}
	return RuntimeUnitView{}, false
}

func ActiveRuntimeUnitApplySlice(plan *types.ChangePlan, run *types.WriteWorkflowRun) (types.ChangePlanSlice, bool) {
	if plan == nil || run == nil {
		return types.ChangePlanSlice{}, false
	}
	unit, ok := ActiveRuntimeUnitView(types.ModeApply, *run, plan, nil)
	if !ok || strings.TrimSpace(unit.UnitID) == "" || len(unit.ChangeIndexes) == 0 {
		return types.ChangePlanSlice{}, false
	}
	slice := types.ChangePlanSlice{
		ID:                 strings.TrimSpace(unit.UnitID),
		Status:             unit.Status,
		ChangeIndexes:      runtimeUnitValidChangeIndexes(unit.ChangeIndexes, len(plan.Changes)),
		Paths:              dedupTrimRuntimeUnitStrings(unit.Paths),
		DependsOnSlices:    dedupTrimRuntimeUnitStrings(unit.DependsOnUnits),
		ContractRefs:       dedupTrimRuntimeUnitStrings(unit.ContractRefs),
		ChangedSymbolRefs:  dedupTrimRuntimeUnitStrings(unit.ChangedSymbolRefs),
		VerificationProbes: dedupTrimRuntimeUnitStrings(unit.VerificationProbes),
	}
	if slice.Status == "" {
		slice.Status = types.ChangePlanSlicePending
	}
	if len(slice.ChangeIndexes) == 0 {
		return types.ChangePlanSlice{}, false
	}
	return slice, true
}

func ActiveRuntimeUnitApplyChanges(plan *types.ChangePlan, run *types.WriteWorkflowRun) ([]types.FileChange, bool) {
	if plan == nil {
		return nil, false
	}
	unit, ok := ActiveRuntimeUnitApplySlice(plan, run)
	if !ok {
		return append([]types.FileChange(nil), plan.Changes...), false
	}
	out := make([]types.FileChange, 0, len(unit.ChangeIndexes))
	for _, idx := range unit.ChangeIndexes {
		if idx < 0 || idx >= len(plan.Changes) {
			continue
		}
		out = append(out, plan.Changes[idx])
	}
	if len(out) == 0 {
		return append([]types.FileChange(nil), plan.Changes...), false
	}
	return out, true
}

func HasActiveRuntimeUnit(plan *types.ChangePlan, run *types.WriteWorkflowRun) bool {
	if plan == nil || run == nil {
		return false
	}
	_, ok := ActiveRuntimeUnitView(types.ModeApply, *run, plan, nil)
	return ok
}

func NextRunnableRuntimeUnitID(units []RuntimeUnitView) string {
	if len(units) == 0 {
		return ""
	}
	activeIndex := -1
	statusByID := make(map[string]types.ChangePlanSliceStatus, len(units))
	for i, unit := range units {
		id := strings.TrimSpace(unit.UnitID)
		statusByID[id] = unit.Status
		if unit.Active {
			activeIndex = i
		}
	}
	for i, unit := range units {
		if activeIndex >= 0 && i <= activeIndex {
			continue
		}
		if runtimeUnitStatusTerminal(unit.Status) {
			continue
		}
		if runtimeUnitDependenciesSatisfied(unit, statusByID) {
			return strings.TrimSpace(unit.UnitID)
		}
	}
	for _, unit := range units {
		if runtimeUnitStatusTerminal(unit.Status) {
			continue
		}
		if runtimeUnitDependenciesSatisfied(unit, statusByID) {
			return strings.TrimSpace(unit.UnitID)
		}
	}
	return ""
}

func PreserveVerifiedRuntimeUnits(existing []types.WriteWorkflowSlice, priorPlan, nextPlan *types.ChangePlan, derived []types.WriteWorkflowSlice) []types.WriteWorkflowSlice {
	if len(existing) == 0 || priorPlan == nil || nextPlan == nil || len(derived) == 0 {
		return cloneRuntimeWorkflowSlices(derived)
	}
	priorFingerprints := runtimeUnitSliceFingerprints(priorPlan)
	nextFingerprints := runtimeUnitSliceFingerprints(nextPlan)
	nextCounts := map[string]int{}
	for _, fp := range nextFingerprints {
		if fp != "" {
			nextCounts[fp]++
		}
	}
	priorStatusByID := map[string]types.ChangePlanSliceStatus{}
	preservedByFingerprint := map[string]types.WriteWorkflowSlice{}
	for _, prior := range existing {
		id := strings.TrimSpace(prior.ID)
		if id == "" {
			continue
		}
		priorStatusByID[id] = prior.Status
	}
	for _, prior := range existing {
		id := strings.TrimSpace(prior.ID)
		if id == "" || prior.Status != types.ChangePlanSliceVerified {
			continue
		}
		if !runtimeUnitPriorDependenciesPreserved(prior, priorStatusByID) {
			continue
		}
		fp := priorFingerprints[id]
		if fp == "" || nextCounts[fp] != 1 {
			continue
		}
		preservedByFingerprint[fp] = prior
	}
	out := cloneRuntimeWorkflowSlices(derived)
	for i := range out {
		fp := nextFingerprints[strings.TrimSpace(out[i].ID)]
		prior, ok := preservedByFingerprint[fp]
		if !ok {
			continue
		}
		mergeRuntimeUnitPreservation(&out[i], prior)
	}
	return out
}

func runtimeUnitSourceSlices(batch types.WriteWorkflowBatch, plan *types.ChangePlan) []types.WriteWorkflowSlice {
	if len(batch.Slices) > 0 {
		out := make([]types.WriteWorkflowSlice, len(batch.Slices))
		copy(out, batch.Slices)
		return out
	}
	if plan == nil {
		return nil
	}
	return types.WriteWorkflowSlicesFromChangePlan(plan)
}

func runtimeUnitPlanSliceByID(plan *types.ChangePlan) map[string]types.ChangePlanSlice {
	out := map[string]types.ChangePlanSlice{}
	if plan == nil {
		return out
	}
	for _, slice := range types.NormalizeChangePlanSlices(plan, types.ChangePlanSliceOptions{}) {
		if id := strings.TrimSpace(slice.ID); id != "" {
			out[id] = slice
		}
	}
	return out
}

func runtimeUnitPermission(plan *types.ChangePlan) RuntimeUnitPermissionView {
	if plan == nil {
		return RuntimeUnitPermissionView{}
	}
	approval := DeriveApprovalExecutionView(plan)
	view := RuntimeUnitPermissionView{
		State:        approval.State,
		Action:       approval.Action,
		UserDecision: approval.UserDecision,
		ReasonCode:   approval.ReasonCode,
		CanApply:     approval.CanApply,
		CanAutoApply: approval.CanAutoApply,
		RequiresUser: approval.RequiresUser,
		Denied:       approval.Denied,
	}
	if plan.Approval != nil {
		view.RiskLevel = strings.TrimSpace(plan.Approval.RiskLevel)
	}
	return view
}

func runtimeUnitPatchTruth(plan *types.ChangePlan, slice types.WriteWorkflowSlice, total int) RuntimeUnitPatchTruthView {
	view := RuntimeUnitPatchTruthView{
		ApplyRef: firstNonEmptyRuntimeUnit(slice.ApplyRef, ""),
	}
	if slice.Checkpoint != nil {
		view.CheckpointRef = strings.TrimSpace(slice.Checkpoint.Ref)
		view.CheckpointSHA = strings.TrimSpace(slice.Checkpoint.CommitSHA)
	}
	if slice.Restore != nil {
		view.RestoreRef = strings.TrimSpace(slice.Restore.CheckpointRef)
		view.RestoreReason = strings.TrimSpace(slice.Restore.ReasonCode)
	}
	if plan == nil || plan.PatchEffect == nil || !runtimeUnitArtifactMatchesSlice(plan.PatchEffect.SliceID, slice.ID, total) {
		return view
	}
	effect := plan.PatchEffect
	view.HasActualDiff = len(effect.Files) > 0 || strings.TrimSpace(effect.DiffFingerprint) != ""
	view.PatchEffectID = strings.TrimSpace(effect.RecordID)
	view.DiffFingerprint = strings.TrimSpace(effect.DiffFingerprint)
	view.HeadRef = strings.TrimSpace(effect.HeadRef)
	view.AppliedPaths = runtimeUnitPatchEffectPaths(effect)
	if view.ApplyRef == "" && strings.TrimSpace(effect.HeadRef) != "" {
		view.ApplyRef = strings.TrimSpace(effect.HeadRef)
	}
	return view
}

func runtimeUnitPatchReview(plan *types.ChangePlan, slice types.WriteWorkflowSlice, total int) RuntimeUnitPatchReviewView {
	if plan == nil || plan.PatchReview == nil || !runtimeUnitArtifactMatchesSlice(plan.PatchReview.SliceID, slice.ID, total) {
		return RuntimeUnitPatchReviewView{}
	}
	review := types.NormalizePatchReviewRecord(*plan.PatchReview)
	summary := types.SummarizePatchReviewCoverage(review)
	return RuntimeUnitPatchReviewView{
		ReviewID:        strings.TrimSpace(review.ReviewID),
		Verdict:         summary.Verdict,
		HardBlock:       summary.HardBlock,
		ReasonCodes:     dedupTrimRuntimeUnitStrings(summary.ReasonCodes),
		UnverifiedKinds: dedupTrimRuntimeUnitStrings(runtimeUnitUnverifiedImpactKinds(summary)),
		BlockReason:     strings.TrimSpace(summary.BlockReason),
		TotalFindings:   summary.TotalFindings,
		ErrorFindings:   summary.ErrorFindings,
		PatchEffectID:   strings.TrimSpace(review.PatchEffectID),
		DiffFingerprint: strings.TrimSpace(review.DiffFingerprint),
	}
}

func runtimeUnitObservation(slice types.WriteWorkflowSlice, plan *types.ChangePlan, report *types.ChangeReport, active bool) ObservationAuthorityView {
	if attempt := latestRuntimeUnitAttemptOfKind(slice, "observe"); attempt != nil {
		return DeriveObservationAuthorityFromAttempt(attempt)
	}
	if attempt := latestRuntimeUnitAttemptOfKind(slice, "verify"); attempt != nil {
		return DeriveObservationAuthorityFromAttempt(attempt)
	}
	if active && runtimeUnitReportMatchesPlan(plan, report) {
		return DeriveObservationAuthorityFromReport(report, nil)
	}
	return ObservationAuthorityView{}
}

func latestRuntimeUnitAttemptOfKind(slice types.WriteWorkflowSlice, kind string) *types.WriteWorkflowAttempt {
	kind = strings.TrimSpace(kind)
	for i := len(slice.Attempts) - 1; i >= 0; i-- {
		if strings.TrimSpace(slice.Attempts[i].Kind) == kind {
			return &slice.Attempts[i]
		}
	}
	return nil
}

func runtimeUnitReportMatchesPlan(plan *types.ChangePlan, report *types.ChangeReport) bool {
	if report == nil {
		return false
	}
	if plan == nil {
		return strings.TrimSpace(report.PlanID) == ""
	}
	reportPlanID := strings.TrimSpace(report.PlanID)
	planID := strings.TrimSpace(plan.ID)
	return reportPlanID == "" || planID == "" || reportPlanID == planID
}

func runtimeUnitArtifactMatchesSlice(artifactSliceID, unitID string, total int) bool {
	artifactSliceID = strings.TrimSpace(artifactSliceID)
	unitID = strings.TrimSpace(unitID)
	if artifactSliceID != "" {
		return artifactSliceID == unitID
	}
	return total <= 1
}

func runtimeUnitPatchEffectPaths(effect *types.PatchEffectRecord) []string {
	if effect == nil {
		return nil
	}
	var paths []string
	for _, file := range effect.Files {
		paths = append(paths, file.Path, file.OldPath)
	}
	return dedupTrimRuntimeUnitStrings(paths)
}

func runtimeUnitValidChangeIndexes(indexes []int, changeCount int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(indexes))
	for _, idx := range indexes {
		if idx < 0 || idx >= changeCount || seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, idx)
	}
	return out
}

func runtimeUnitStatusTerminal(status types.ChangePlanSliceStatus) bool {
	switch status {
	case types.ChangePlanSliceVerified, types.ChangePlanSliceUnverified, types.ChangePlanSliceFailed, types.ChangePlanSliceBlocked:
		return true
	default:
		return false
	}
}

func runtimeUnitDependenciesSatisfied(unit RuntimeUnitView, statusByID map[string]types.ChangePlanSliceStatus) bool {
	for _, dep := range unit.DependsOnUnits {
		status := statusByID[strings.TrimSpace(dep)]
		if status != types.ChangePlanSliceVerified && status != types.ChangePlanSliceUnverified {
			return false
		}
	}
	return true
}

func runtimeUnitOwnerAnchorsByPath(plan *types.ChangePlan) map[string][]types.OwnerAnchorViewItem {
	out := map[string][]types.OwnerAnchorViewItem{}
	if plan == nil {
		return out
	}
	view := types.OwnerAnchorViewFromChangePlan(plan, 0)
	for _, item := range view.Items {
		path := strings.TrimSpace(item.Path)
		if path == "" {
			continue
		}
		out[path] = append(out[path], item)
	}
	return out
}

func runtimeUnitOwnerAnchorsForPaths(byPath map[string][]types.OwnerAnchorViewItem, paths []string, sliceID string) []types.OwnerAnchorViewItem {
	seen := map[string]bool{}
	var out []types.OwnerAnchorViewItem
	for _, path := range paths {
		for _, item := range byPath[strings.TrimSpace(path)] {
			if sid := strings.TrimSpace(item.SliceID); sid != "" && sid != strings.TrimSpace(sliceID) {
				continue
			}
			key := firstNonEmptyRuntimeUnit(item.ID, item.Path+"|"+item.OwnerSymbol+"|"+item.AnchorSymbol)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	return types.NormalizeOwnerAnchorView(types.OwnerAnchorView{Items: out}, 8).Items
}

func runtimeUnitImpactObligationsBySlice(plan *types.ChangePlan) map[string][]types.ImpactObligation {
	out := map[string][]types.ImpactObligation{}
	if plan == nil || plan.ImpactObligations == nil {
		return out
	}
	set := types.NormalizeImpactObligationSet(*plan.ImpactObligations)
	for _, obligation := range set.Obligations {
		sliceID := strings.TrimSpace(obligation.SliceID)
		out[sliceID] = append(out[sliceID], obligation)
	}
	return out
}

func runtimeUnitObligationsForSlice(bySlice map[string][]types.ImpactObligation, sliceID string, paths []string) []types.ImpactObligation {
	seen := map[string]bool{}
	var out []types.ImpactObligation
	add := func(items []types.ImpactObligation) {
		for _, item := range items {
			key := firstNonEmptyRuntimeUnit(item.ID, item.Kind+"|"+item.SubjectPath+"|"+item.RelatedPath+"|"+item.ContractRef+"|"+item.SubjectSymbol)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	add(bySlice[strings.TrimSpace(sliceID)])
	pathSet := map[string]bool{}
	for _, path := range paths {
		pathSet[strings.TrimSpace(path)] = true
	}
	for _, item := range bySlice[""] {
		if pathSet[strings.TrimSpace(item.SubjectPath)] || pathSet[strings.TrimSpace(item.RelatedPath)] {
			add([]types.ImpactObligation{item})
		}
	}
	return out
}

func runtimeUnitUnverifiedImpactKinds(summary types.PatchReviewCoverageSummary) []string {
	var out []string
	for _, row := range summary.ImpactKindCoverage {
		if row.Unverified > 0 || row.Unknown > 0 || row.Unavailable > 0 {
			out = append(out, row.Kind)
		}
	}
	if len(out) == 0 && strings.TrimSpace(summary.PrimaryUncoveredImpactKind) != "" {
		out = append(out, summary.PrimaryUncoveredImpactKind)
	}
	return out
}

func cloneRuntimeUnitCompletion(in *types.WriteWorkflowCompletion) *types.WriteWorkflowCompletion {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneRuntimeWorkflowSlices(in []types.WriteWorkflowSlice) []types.WriteWorkflowSlice {
	out := make([]types.WriteWorkflowSlice, len(in))
	for i, slice := range in {
		out[i] = cloneRuntimeWorkflowSlice(slice)
	}
	return out
}

func cloneRuntimeWorkflowSlice(in types.WriteWorkflowSlice) types.WriteWorkflowSlice {
	out := in
	out.ChangeIndexes = append([]int(nil), in.ChangeIndexes...)
	out.Paths = append([]string(nil), in.Paths...)
	out.DependsOnSlices = append([]string(nil), in.DependsOnSlices...)
	out.ContextPackIDs = append([]string(nil), in.ContextPackIDs...)
	out.Attempts = append([]types.WriteWorkflowAttempt(nil), in.Attempts...)
	if in.Checkpoint != nil {
		cp := *in.Checkpoint
		out.Checkpoint = &cp
	}
	if in.Restore != nil {
		restore := *in.Restore
		out.Restore = &restore
	}
	out.Completion = cloneRuntimeUnitCompletion(in.Completion)
	return out
}

func mergeRuntimeUnitPreservation(next *types.WriteWorkflowSlice, prior types.WriteWorkflowSlice) {
	if next == nil {
		return
	}
	id := strings.TrimSpace(next.ID)
	planID := strings.TrimSpace(next.PlanID)
	changeIndexes := append([]int(nil), next.ChangeIndexes...)
	paths := append([]string(nil), next.Paths...)
	deps := append([]string(nil), next.DependsOnSlices...)
	created := next.CreatedAt
	updated := next.UpdatedAt
	*next = cloneRuntimeWorkflowSlice(prior)
	next.ID = id
	next.PlanID = planID
	next.ChangeIndexes = changeIndexes
	next.Paths = paths
	next.DependsOnSlices = deps
	if next.CreatedAt.IsZero() {
		next.CreatedAt = created
	}
	if !updated.IsZero() {
		next.UpdatedAt = updated
	}
}

func runtimeUnitPriorDependenciesPreserved(slice types.WriteWorkflowSlice, statusByID map[string]types.ChangePlanSliceStatus) bool {
	for _, dep := range slice.DependsOnSlices {
		switch statusByID[strings.TrimSpace(dep)] {
		case types.ChangePlanSliceVerified, types.ChangePlanSliceUnverified:
			continue
		default:
			return false
		}
	}
	return true
}

func runtimeUnitSliceFingerprints(plan *types.ChangePlan) map[string]string {
	out := map[string]string{}
	if plan == nil {
		return out
	}
	for _, slice := range types.NormalizeChangePlanSlices(plan, types.ChangePlanSliceOptions{}) {
		id := strings.TrimSpace(slice.ID)
		if id == "" {
			continue
		}
		out[id] = runtimeUnitSliceFingerprint(plan, slice)
	}
	return out
}

func runtimeUnitSliceFingerprint(plan *types.ChangePlan, slice types.ChangePlanSlice) string {
	if plan == nil || len(slice.ChangeIndexes) == 0 {
		return ""
	}
	payload := struct {
		Changes []runtimeUnitChangeFingerprintPayload `json:"changes"`
	}{
		Changes: make([]runtimeUnitChangeFingerprintPayload, 0, len(slice.ChangeIndexes)),
	}
	for _, idx := range runtimeUnitValidChangeIndexes(slice.ChangeIndexes, len(plan.Changes)) {
		change := plan.Changes[idx]
		payload.Changes = append(payload.Changes, runtimeUnitChangeFingerprintPayload{
			Path:       strings.TrimSpace(change.Path),
			Kind:       strings.TrimSpace(change.Kind),
			NewContent: change.NewContent,
			Patch:      change.Patch,
			Edits:      append([]types.StructuredEdit(nil), change.Edits...),
			NewPath:    strings.TrimSpace(change.NewPath),
			DependsOn:  dedupTrimRuntimeUnitStrings(change.DependsOn),
		})
	}
	if len(payload.Changes) == 0 {
		return ""
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

type runtimeUnitChangeFingerprintPayload struct {
	Path       string                 `json:"path,omitempty"`
	Kind       string                 `json:"kind,omitempty"`
	NewContent string                 `json:"new_content,omitempty"`
	Patch      string                 `json:"patch,omitempty"`
	Edits      []types.StructuredEdit `json:"edits,omitempty"`
	NewPath    string                 `json:"new_path,omitempty"`
	DependsOn  []string               `json:"depends_on,omitempty"`
}

func firstNonEmptyRuntimeUnit(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dedupTrimRuntimeUnitStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
