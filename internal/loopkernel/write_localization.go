package loopkernel

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// LocalizationReviewFromWriteWorkflowPlan separates the accepted delivery
// domain from paths merely read while exploring it. The cumulative scope is
// controller-owned verification metadata, not permission to modify more files.
// Unknown plan identity or unapportioned applied history keeps the legacy
// conservative review instead of silently dropping an older obligation.
func LocalizationReviewFromWriteWorkflowPlan(run types.WriteWorkflowRun, batchID string, plan *types.ChangePlan) (*types.SourceLocalizationReview, bool) {
	batchID = strings.TrimSpace(batchID)
	if plan == nil || batchID == "" || strings.TrimSpace(plan.ID) == "" {
		return nil, false
	}
	var active *types.WriteWorkflowBatch
	for i := range run.Batches {
		if strings.TrimSpace(run.Batches[i].ID) == batchID {
			active = &run.Batches[i]
			break
		}
	}
	if active == nil || strings.TrimSpace(active.PlanID) != strings.TrimSpace(plan.ID) {
		return nil, false
	}
	required := map[string]bool{}
	addRequired := func(raw string) {
		if p := strings.TrimSpace(raw); p != "" {
			required[p] = true
		}
	}
	for _, p := range plan.TargetPaths {
		addRequired(p)
	}
	for _, p := range plan.AppliedPaths {
		addRequired(p)
	}
	for _, change := range plan.Changes {
		addRequired(change.Path)
		if change.Kind == "rename" {
			addRequired(change.NewPath)
		}
	}
	if len(required) == 0 || !WorkflowLocalizationAppliedHistoryCovered(run, plan) {
		return nil, false
	}
	for _, p := range active.ExpectedPaths {
		addRequired(p)
	}
	if scope := plan.CumulativeVerificationScope; scope != nil {
		for _, p := range scope.TargetPaths {
			addRequired(p)
		}
	}
	legacy := LocalizationReviewFromWriteWorkflowRun(run, batchID)
	if legacy == nil {
		legacy = &types.SourceLocalizationReview{}
	}
	// Read scope declarations from the source packs, not the separately capped
	// anchor preview in a localization review. That preview may omit a required
	// scope row even though the durable pack still contains it.
	for _, pack := range run.ContextPacks {
		if pack.BatchID != "" && strings.TrimSpace(pack.BatchID) != batchID {
			continue
		}
		for _, item := range pack.Items {
			if item.BatchID != "" && strings.TrimSpace(item.BatchID) != batchID {
				continue
			}
			if anchor := item.LocalizationAnchor; anchor != nil && anchor.Kind == types.SourceLocalizationAnchorScope {
				addRequired(anchor.Path)
			}
		}
	}
	out := types.SourceLocalizationReview{
		Source: "write_workflow_delivery_scope", PlanID: strings.TrimSpace(plan.ID), BatchID: batchID, Goal: run.Goal,
		PriorContextPaths: append([]string(nil), legacy.SourcePaths...),
		AuxiliaryPaths:    append([]string(nil), legacy.AuxiliaryPaths...),
		EvidenceRefs:      append([]types.WriteExplorationEvidenceRef(nil), legacy.EvidenceRefs...),
		Anchors:           append([]types.SourceLocalizationAnchor(nil), legacy.Anchors...),
	}
	owners := map[string]bool{}
	for _, p := range legacy.OwnerSupportedPaths {
		owners[p] = true
	}
	// A current plan review may carry owner anchors not retained by a bounded
	// context pack. Only exact-generation anchors can supplement that evidence;
	// its aggregate supported status never replaces a conflicting workflow row.
	if review := plan.LocalizationReview; review != nil && strings.TrimSpace(review.PlanID) == strings.TrimSpace(plan.ID) &&
		(strings.TrimSpace(review.BatchID) == "" || strings.TrimSpace(review.BatchID) == batchID) {
		for _, p := range review.SourcePaths {
			addRequired(p)
		}
		for _, p := range review.OwnerMissingPaths {
			addRequired(p)
		}
		for _, p := range review.MissingPaths {
			addRequired(p)
		}
		for _, anchor := range review.Anchors {
			out.Anchors = append(out.Anchors, anchor)
			if anchor.Strength == types.SourceLocalizationAnchorOwner && !types.SourcePathRoleIsAuxiliary(anchor.Role) {
				owners[strings.TrimSpace(anchor.Path)] = true
			}
		}
	}
	paths := make([]string, 0, len(required))
	for p := range required {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(p)) {
			out.AuxiliaryPaths = append(out.AuxiliaryPaths, p)
			continue
		}
		out.SourcePaths = append(out.SourcePaths, p)
		if owners[p] {
			out.OwnerSupportedPaths = append(out.OwnerSupportedPaths, p)
			out.SupportedPaths = append(out.SupportedPaths, p)
		} else {
			out.OwnerMissingPaths = append(out.OwnerMissingPaths, p)
		}
	}
	out = types.NormalizeSourceLocalizationReview(out)
	return &out, true
}

// WorkflowLocalizationAppliedHistoryCovered checks whether the current plan
// and its controller-stamped cumulative scope account for applied history.
// A false result must also prevent a current-plan supported-review fallback;
// otherwise an unknown old delivery domain could be silently certified.
func WorkflowLocalizationAppliedHistoryCovered(run types.WriteWorkflowRun, plan *types.ChangePlan) bool {
	if plan == nil || strings.TrimSpace(plan.ID) == "" {
		return false
	}
	covered := map[string]bool{strings.TrimSpace(plan.ID): true}
	if scope := plan.CumulativeVerificationScope; scope != nil {
		if len(scope.SourcePlanIDs) == 0 || len(scope.TargetPaths) == 0 {
			return false
		}
		for _, rawID := range scope.SourcePlanIDs {
			id := strings.TrimSpace(rawID)
			if id == "" {
				return false
			}
			covered[id] = true
		}
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ApplyRef) != "" && !covered[strings.TrimSpace(batch.PlanID)] {
			return false
		}
		for _, attempt := range batch.Attempts {
			if strings.TrimSpace(attempt.Kind) != "apply" {
				continue
			}
			if strings.TrimSpace(attempt.Status) != "applied" || !covered[strings.TrimSpace(attempt.PlanID)] {
				return false
			}
		}
		for _, slice := range batch.Slices {
			if strings.TrimSpace(slice.ApplyRef) != "" && !covered[strings.TrimSpace(slice.PlanID)] {
				return false
			}
		}
	}
	return true
}

func LocalizationReviewFromWriteWorkflowRun(run types.WriteWorkflowRun, batchID string) *types.SourceLocalizationReview {
	run = types.NormalizeWriteWorkflowRun(run)
	batchID = strings.TrimSpace(batchID)
	review := types.SourceLocalizationReview{
		Source:  "write_workflow_context_packs",
		BatchID: batchID,
		Goal:    run.Goal,
	}
	matchedBatch := false
	ownerSupported := map[string]bool{}
	sourceSeen := map[string]bool{}
	addSource := func(path string, role types.SourcePathRole) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if role == "" {
			role = types.ClassifySourcePathRole(path)
		}
		if types.SourcePathRoleIsAuxiliary(role) {
			review.AuxiliaryPaths = append(review.AuxiliaryPaths, path)
			return
		}
		review.SourcePaths = append(review.SourcePaths, path)
		sourceSeen[path] = true
	}
	for _, batch := range run.Batches {
		if batchID != "" && strings.TrimSpace(batch.ID) != batchID {
			continue
		}
		matchedBatch = true
		for _, path := range batch.ExpectedPaths {
			addSource(path, types.ClassifySourcePathRole(path))
			if p := strings.TrimSpace(path); p != "" {
				review.Anchors = append(review.Anchors, types.SourceLocalizationAnchor{
					Path:        p,
					Role:        types.ClassifySourcePathRole(p),
					SourceStage: "write_workflow_batch",
					Kind:        types.SourceLocalizationAnchorScope,
					Strength:    types.SourceLocalizationAnchorSupporting,
					ReasonCode:  "batch_expected_path_requires_owner_localization",
				})
			}
		}
	}
	for _, pack := range run.ContextPacks {
		pack = types.NormalizeWriteContextPack(pack)
		if batchID != "" && pack.BatchID != "" && pack.BatchID != batchID {
			continue
		}
		for _, item := range pack.Items {
			if batchID != "" && item.BatchID != "" && item.BatchID != batchID {
				continue
			}
			if item.EvidenceRef != nil {
				review.EvidenceRefs = append(review.EvidenceRefs, *item.EvidenceRef)
			}
			if item.LocalizationAnchor == nil {
				continue
			}
			anchor := *item.LocalizationAnchor
			if anchor.Role == "" {
				anchor.Role = types.ClassifySourcePathRole(anchor.Path)
			}
			addSource(anchor.Path, anchor.Role)
			review.Anchors = append(review.Anchors, anchor)
			if !types.SourcePathRoleIsAuxiliary(anchor.Role) && anchor.Strength == types.SourceLocalizationAnchorOwner {
				review.OwnerSupportedPaths = append(review.OwnerSupportedPaths, anchor.Path)
				review.SupportedPaths = append(review.SupportedPaths, anchor.Path)
				ownerSupported[anchor.Path] = true
			}
		}
	}
	for path := range sourceSeen {
		if !ownerSupported[path] {
			review.OwnerMissingPaths = append(review.OwnerMissingPaths, path)
		}
	}
	review = types.NormalizeSourceLocalizationReview(review)
	if !types.SourceLocalizationReviewHasSignal(&review) {
		if matchedBatch {
			missing := types.NormalizeSourceLocalizationReview(types.SourceLocalizationReview{
				Status:      types.SourceLocalizationMissing,
				Source:      review.Source,
				BatchID:     batchID,
				Goal:        run.Goal,
				ReasonCodes: []string{"write_workflow_no_localization_signal"},
			})
			return &missing
		}
		return nil
	}
	return &review
}
