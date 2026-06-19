package types

import (
	"sort"
	"strings"
)

const localizationRequirementMaxItems = 64

const localizationRequirementRequiredOwnerEvidence = "typed_owner_localization_anchor"

type LocalizationRequirementStatus string

const (
	LocalizationRequirementOpen      LocalizationRequirementStatus = "open"
	LocalizationRequirementSatisfied LocalizationRequirementStatus = "satisfied"
)

type LocalizationRequirementKind string

const (
	LocalizationRequirementOwnerEvidence LocalizationRequirementKind = "owner_evidence"
	LocalizationRequirementPriorContext  LocalizationRequirementKind = "prior_context"
)

// LocalizationRequirement is a shared read/write obligation over typed source
// localization anchors. It is derived from SourceLocalizationReview,
// WriteContextPack, ChangePlan, and OwnerAnchorView fields only; prompt text,
// user prose, model rationale, terminal logs, and rendered summaries never
// participate.
type LocalizationRequirement struct {
	ID               string                        `json:"id,omitempty"`
	Path             string                        `json:"path,omitempty"`
	Role             SourcePathRole                `json:"role,omitempty"`
	Kind             LocalizationRequirementKind   `json:"kind,omitempty"`
	Status           LocalizationRequirementStatus `json:"status,omitempty"`
	Priority         WriteContextPriority          `json:"priority,omitempty"`
	Consumer         WriteContextConsumer          `json:"consumer,omitempty"`
	Source           string                        `json:"source,omitempty"`
	SourceStage      string                        `json:"source_stage,omitempty"`
	BatchID          string                        `json:"batch_id,omitempty"`
	SliceID          string                        `json:"slice_id,omitempty"`
	ReasonCode       string                        `json:"reason_code,omitempty"`
	RequiredEvidence string                        `json:"required_evidence,omitempty"`
	OwnerAnchors     []OwnerAnchorViewItem         `json:"owner_anchors,omitempty"`
	EvidenceRefs     []WriteExplorationEvidenceRef `json:"evidence_refs,omitempty"`
}

type LocalizationRequirementSet struct {
	Items                    []LocalizationRequirement `json:"items,omitempty"`
	OpenItems                int                       `json:"open_items,omitempty"`
	VisibleItems             int                       `json:"visible_items,omitempty"`
	Truncated                bool                      `json:"truncated,omitempty"`
	MissingPriorContextPaths []string                  `json:"missing_prior_context_paths,omitempty"`
	MissingOwnerAnchorPaths  []string                  `json:"missing_owner_anchor_paths,omitempty"`
	CandidatePaths           []string                  `json:"candidate_paths,omitempty"`
	EvidenceRequirements     []string                  `json:"evidence_requirements,omitempty"`
}

func LocalizationRequirementsFromWritePlanContext(batchID, sliceID string, consumer WriteContextConsumer, prior []WriteContextPack, plan *ChangePlan, limit int) LocalizationRequirementSet {
	if plan == nil {
		return LocalizationRequirementSet{}
	}
	planPaths := writeContextCoveragePlanPaths(plan)
	if len(planPaths) == 0 {
		return LocalizationRequirementSet{}
	}
	contextPaths := writeContextCoveragePriorPaths(prior)
	ownerView := OwnerAnchorViewFromWriteContextPacks(prior, consumer, batchID, sliceID, 0)
	items := make([]LocalizationRequirement, 0, len(planPaths))
	for _, p := range planPaths {
		ownerAnchors := localizationRequirementOwnerAnchorsForPath(ownerView, p)
		hasPrior := len(contextPaths) > 0 && writeContextCoveragePathCoveredByContext(p, contextPaths)
		switch {
		case len(ownerAnchors) > 0:
			items = append(items, LocalizationRequirement{
				Path:         p,
				Kind:         LocalizationRequirementOwnerEvidence,
				Status:       LocalizationRequirementSatisfied,
				Priority:     WriteContextP1,
				Consumer:     consumer,
				Source:       "write_plan_context",
				SourceStage:  "plan_context",
				BatchID:      batchID,
				SliceID:      sliceID,
				ReasonCode:   "plan_source_path_owner_anchor_satisfied",
				OwnerAnchors: ownerAnchors,
				EvidenceRefs: localizationRequirementEvidenceRefs(ownerAnchors),
			})
		case !hasPrior:
			reason := "plan_source_path_missing_prior_context"
			if len(contextPaths) == 0 {
				reason = "plan_source_path_without_prior_context"
			}
			items = append(items, LocalizationRequirement{
				Path:             p,
				Kind:             LocalizationRequirementPriorContext,
				Status:           LocalizationRequirementOpen,
				Priority:         WriteContextP1,
				Consumer:         consumer,
				Source:           "write_plan_context",
				SourceStage:      "plan_context",
				BatchID:          batchID,
				SliceID:          sliceID,
				ReasonCode:       reason,
				RequiredEvidence: localizationRequirementRequiredOwnerEvidence,
			})
		default:
			items = append(items, LocalizationRequirement{
				Path:             p,
				Kind:             LocalizationRequirementOwnerEvidence,
				Status:           LocalizationRequirementOpen,
				Priority:         WriteContextP1,
				Consumer:         consumer,
				Source:           "write_plan_context",
				SourceStage:      "plan_context",
				BatchID:          batchID,
				SliceID:          sliceID,
				ReasonCode:       "plan_source_path_without_owner_anchor",
				RequiredEvidence: localizationRequirementRequiredOwnerEvidence,
			})
		}
	}
	return NormalizeLocalizationRequirementSet(LocalizationRequirementSet{
		Items:          items,
		CandidatePaths: OwnerAnchorCandidatePaths(ownerView, planPaths, 16),
	}, limit)
}

func LocalizationRequirementsFromCandidatePaths(paths []string, ownerView OwnerAnchorView, batchID, sliceID, source string, limit int) LocalizationRequirementSet {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "controller_preplan"
	}
	ownerView = NormalizeOwnerAnchorView(ownerView, 0)
	var items []LocalizationRequirement
	for _, raw := range paths {
		p := writeContextCoveragePath(raw)
		if p == "" || writeContextCoveragePathIsTest(p) {
			continue
		}
		ownerAnchors := localizationRequirementOwnerAnchorsForPath(ownerView, p)
		if len(ownerAnchors) > 0 {
			items = append(items, LocalizationRequirement{
				Path:         p,
				Kind:         LocalizationRequirementOwnerEvidence,
				Status:       LocalizationRequirementSatisfied,
				Priority:     WriteContextP1,
				Consumer:     WriteConsumerPlanner,
				Source:       source,
				SourceStage:  source,
				BatchID:      batchID,
				SliceID:      sliceID,
				ReasonCode:   "candidate_path_owner_anchor_satisfied",
				OwnerAnchors: ownerAnchors,
				EvidenceRefs: localizationRequirementEvidenceRefs(ownerAnchors),
			})
			continue
		}
		items = append(items, LocalizationRequirement{
			Path:             p,
			Kind:             LocalizationRequirementOwnerEvidence,
			Status:           LocalizationRequirementOpen,
			Priority:         WriteContextP1,
			Consumer:         WriteConsumerPlanner,
			Source:           source,
			SourceStage:      source,
			BatchID:          batchID,
			SliceID:          sliceID,
			ReasonCode:       "candidate_path_without_owner_anchor",
			RequiredEvidence: localizationRequirementRequiredOwnerEvidence,
		})
	}
	return NormalizeLocalizationRequirementSet(LocalizationRequirementSet{
		Items:          items,
		CandidatePaths: OwnerAnchorCandidatePaths(ownerView, paths, 16),
	}, limit)
}

func LocalizationRequirementsFromSourceLocalizationReview(review SourceLocalizationReview, limit int) LocalizationRequirementSet {
	review = NormalizeSourceLocalizationReview(review)
	ownerView := OwnerAnchorViewFromSourceLocalizationReview(review, 0)
	var paths []string
	paths = append(paths, review.SourcePaths...)
	paths = append(paths, review.OwnerMissingPaths...)
	paths = append(paths, review.MissingPaths...)
	if len(paths) == 0 {
		for _, anchor := range review.Anchors {
			if p := writeContextCoveragePath(anchor.Path); p != "" && !writeContextCoveragePathIsTest(p) {
				paths = append(paths, p)
			}
		}
	}
	paths = dedupSourceLocalizationPaths(paths, sourceLocalizationMaxPaths)
	var items []LocalizationRequirement
	for _, p := range paths {
		ownerAnchors := localizationRequirementOwnerAnchorsForPath(ownerView, p)
		if len(ownerAnchors) > 0 {
			items = append(items, LocalizationRequirement{
				Path:         p,
				Kind:         LocalizationRequirementOwnerEvidence,
				Status:       LocalizationRequirementSatisfied,
				Priority:     WriteContextP1,
				Consumer:     WriteConsumerPlanner,
				Source:       sourceLocalizationFirstNonEmpty(review.Source, "source_localization_review"),
				SourceStage:  sourceLocalizationFirstNonEmpty(review.Source, "source_localization_review"),
				BatchID:      review.BatchID,
				ReasonCode:   "read_source_path_owner_anchor_satisfied",
				OwnerAnchors: ownerAnchors,
				EvidenceRefs: localizationRequirementEvidenceRefs(ownerAnchors),
			})
			continue
		}
		items = append(items, LocalizationRequirement{
			Path:             p,
			Kind:             LocalizationRequirementOwnerEvidence,
			Status:           LocalizationRequirementOpen,
			Priority:         WriteContextP1,
			Consumer:         WriteConsumerPlanner,
			Source:           sourceLocalizationFirstNonEmpty(review.Source, "source_localization_review"),
			SourceStage:      sourceLocalizationFirstNonEmpty(review.Source, "source_localization_review"),
			BatchID:          review.BatchID,
			ReasonCode:       "read_source_path_without_owner_anchor",
			RequiredEvidence: localizationRequirementRequiredOwnerEvidence,
		})
	}
	return NormalizeLocalizationRequirementSet(LocalizationRequirementSet{
		Items:          items,
		CandidatePaths: OwnerAnchorCandidatePaths(ownerView, paths, 16),
	}, limit)
}

func NormalizeLocalizationRequirementSet(in LocalizationRequirementSet, limit int) LocalizationRequirementSet {
	if limit <= 0 || limit > localizationRequirementMaxItems {
		limit = localizationRequirementMaxItems
	}
	seen := map[string]int{}
	items := make([]LocalizationRequirement, 0, len(in.Items))
	for _, item := range in.Items {
		item = normalizeLocalizationRequirement(item)
		if item.Path == "" {
			continue
		}
		key := localizationRequirementKey(item)
		if idx, ok := seen[key]; ok {
			items[idx] = mergeLocalizationRequirement(items[idx], item)
			continue
		}
		seen[key] = len(items)
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if localizationRequirementRank(items[i]) != localizationRequirementRank(items[j]) {
			return localizationRequirementRank(items[i]) < localizationRequirementRank(items[j])
		}
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		return items[i].ReasonCode < items[j].ReasonCode
	})
	total := len(items)
	truncated := in.Truncated
	if len(items) > limit {
		items = items[:limit]
		truncated = true
	}
	out := LocalizationRequirementSet{
		Items:          items,
		VisibleItems:   len(items),
		Truncated:      truncated,
		CandidatePaths: dedupSourceLocalizationPaths(in.CandidatePaths, sourceLocalizationMaxPaths),
	}
	for _, item := range items {
		if item.Status != LocalizationRequirementOpen {
			continue
		}
		out.OpenItems++
		switch item.ReasonCode {
		case "plan_source_path_without_prior_context", "plan_source_path_missing_prior_context":
			out.MissingPriorContextPaths = append(out.MissingPriorContextPaths, item.Path)
		case "plan_source_path_without_owner_anchor", "candidate_path_without_owner_anchor", "read_source_path_without_owner_anchor":
			out.MissingOwnerAnchorPaths = append(out.MissingOwnerAnchorPaths, item.Path)
		}
	}
	out.MissingPriorContextPaths = dedupSourceLocalizationPaths(out.MissingPriorContextPaths, sourceLocalizationMaxPaths)
	out.MissingOwnerAnchorPaths = dedupSourceLocalizationPaths(out.MissingOwnerAnchorPaths, sourceLocalizationMaxPaths)
	out.EvidenceRequirements = localizationRequirementEvidenceRowsFromItems(out.Items, 8)
	if total == 0 {
		return LocalizationRequirementSet{}
	}
	return out
}

func LocalizationRequirementEvidenceRows(set LocalizationRequirementSet, limit int) []string {
	if limit <= 0 {
		limit = 8
	}
	set = NormalizeLocalizationRequirementSet(set, localizationRequirementMaxItems)
	return localizationRequirementEvidenceRowsFromItems(set.Items, limit)
}

func localizationRequirementEvidenceRowsFromItems(items []LocalizationRequirement, limit int) []string {
	if limit <= 0 {
		limit = 8
	}
	var out []string
	for _, item := range items {
		if item.Status != LocalizationRequirementOpen {
			continue
		}
		parts := []string{"localization_requirement", "path=" + item.Path}
		if item.Kind != "" {
			parts = append(parts, "kind="+string(item.Kind))
		}
		if item.ReasonCode != "" {
			parts = append(parts, "reason="+item.ReasonCode)
		}
		if item.RequiredEvidence != "" {
			parts = append(parts, "required="+item.RequiredEvidence)
		}
		if item.Source != "" {
			parts = append(parts, "source="+item.Source)
		}
		out = append(out, strings.Join(parts, " "))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeLocalizationRequirement(in LocalizationRequirement) LocalizationRequirement {
	in.Path = writeContextCoveragePath(in.Path)
	in.Role = ClassifySourcePathRole(in.Path)
	if in.Path == "" || SourcePathRoleIsAuxiliary(in.Role) {
		return LocalizationRequirement{}
	}
	switch in.Kind {
	case LocalizationRequirementOwnerEvidence, LocalizationRequirementPriorContext:
	default:
		in.Kind = LocalizationRequirementOwnerEvidence
	}
	switch in.Status {
	case LocalizationRequirementSatisfied, LocalizationRequirementOpen:
	default:
		in.Status = LocalizationRequirementOpen
	}
	in.Priority = normalizeWriteContextPriority(in.Priority)
	in.Source = trimSourceLocalizationText(in.Source)
	in.SourceStage = trimSourceLocalizationText(in.SourceStage)
	in.BatchID = trimSourceLocalizationText(in.BatchID)
	in.SliceID = trimSourceLocalizationText(in.SliceID)
	in.ReasonCode = trimSourceLocalizationText(in.ReasonCode)
	in.RequiredEvidence = trimSourceLocalizationText(in.RequiredEvidence)
	if in.RequiredEvidence == "" && in.Status == LocalizationRequirementOpen {
		in.RequiredEvidence = localizationRequirementRequiredOwnerEvidence
	}
	in.OwnerAnchors = NormalizeOwnerAnchorView(OwnerAnchorView{Items: in.OwnerAnchors}, 8).Items
	in.EvidenceRefs = normalizeSourceLocalizationEvidenceRefs(in.EvidenceRefs)
	if in.ID == "" {
		in.ID = localizationRequirementKey(in)
	} else {
		in.ID = trimSourceLocalizationText(in.ID)
	}
	return in
}

func mergeLocalizationRequirement(a, b LocalizationRequirement) LocalizationRequirement {
	if localizationRequirementRank(b) < localizationRequirementRank(a) {
		a, b = b, a
	}
	out := a
	if out.RequiredEvidence == "" {
		out.RequiredEvidence = b.RequiredEvidence
	}
	if out.Source == "" {
		out.Source = b.Source
	}
	if out.SourceStage == "" {
		out.SourceStage = b.SourceStage
	}
	if out.BatchID == "" {
		out.BatchID = b.BatchID
	}
	if out.SliceID == "" {
		out.SliceID = b.SliceID
	}
	out.OwnerAnchors = append(out.OwnerAnchors, b.OwnerAnchors...)
	out.EvidenceRefs = append(out.EvidenceRefs, b.EvidenceRefs...)
	return normalizeLocalizationRequirement(out)
}

func localizationRequirementKey(item LocalizationRequirement) string {
	return strings.Join([]string{
		item.Path,
		string(item.Kind),
		string(item.Status),
		item.ReasonCode,
		item.Source,
		item.BatchID,
		item.SliceID,
	}, "\x00")
}

func localizationRequirementRank(item LocalizationRequirement) int {
	rank := writeContextPriorityRank(item.Priority)
	if item.Status == LocalizationRequirementSatisfied {
		rank += 100
	}
	switch item.Kind {
	case LocalizationRequirementPriorContext:
		return rank
	case LocalizationRequirementOwnerEvidence:
		return rank + 10
	default:
		return rank + 20
	}
}

func localizationRequirementOwnerAnchorsForPath(view OwnerAnchorView, planPath string) []OwnerAnchorViewItem {
	planPath = writeContextCoveragePath(planPath)
	if planPath == "" {
		return nil
	}
	view = NormalizeOwnerAnchorView(view, 0)
	var out []OwnerAnchorViewItem
	for _, item := range view.Items {
		if !ownerAnchorViewItemHasOwnerAuthority(item) {
			continue
		}
		if !writeContextCoveragePathCoveredByContext(planPath, []string{item.Path}) {
			continue
		}
		out = append(out, item)
	}
	return NormalizeOwnerAnchorView(OwnerAnchorView{Items: out}, 8).Items
}

func localizationRequirementEvidenceRefs(items []OwnerAnchorViewItem) []WriteExplorationEvidenceRef {
	var out []WriteExplorationEvidenceRef
	for _, item := range items {
		if item.EvidenceRef == nil {
			continue
		}
		out = append(out, *item.EvidenceRef)
	}
	return normalizeSourceLocalizationEvidenceRefs(out)
}
