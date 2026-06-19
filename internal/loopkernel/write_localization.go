package loopkernel

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func LocalizationReviewFromWriteWorkflowRun(run types.WriteWorkflowRun, batchID string) *types.SourceLocalizationReview {
	run = types.NormalizeWriteWorkflowRun(run)
	batchID = strings.TrimSpace(batchID)
	review := types.SourceLocalizationReview{
		Source:  "write_workflow_context_packs",
		BatchID: batchID,
		Goal:    run.Goal,
	}
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
		return nil
	}
	return &review
}
