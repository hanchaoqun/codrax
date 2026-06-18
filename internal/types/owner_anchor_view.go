package types

import (
	"sort"
	"strings"
)

const ownerAnchorViewMaxItems = 64

// OwnerAnchorView is the ranked, consumer-facing view of source localization
// anchors. It is derived from typed SourceLocalizationAnchor / WriteContextPack
// fields only; model prose, summaries, and user text never participate.
type OwnerAnchorView struct {
	Items        []OwnerAnchorViewItem `json:"items,omitempty"`
	TotalItems   int                   `json:"total_items,omitempty"`
	VisibleItems int                   `json:"visible_items,omitempty"`
	StrongPaths  []string              `json:"strong_paths,omitempty"`
	HasOwner     bool                  `json:"has_owner,omitempty"`
	HasStrong    bool                  `json:"has_strong,omitempty"`
	Truncated    bool                  `json:"truncated,omitempty"`
}

// OwnerAnchorViewItem is one normalized anchor row. Rank is deterministic and
// lower is stronger; downstream schedulers may sort or slice on Rank without
// reading advisory prompt text.
type OwnerAnchorViewItem struct {
	ID           string                           `json:"id,omitempty"`
	Path         string                           `json:"path,omitempty"`
	Role         SourcePathRole                   `json:"role,omitempty"`
	Kind         SourceLocalizationAnchorKind     `json:"kind,omitempty"`
	Strength     SourceLocalizationAnchorStrength `json:"strength,omitempty"`
	Priority     WriteContextPriority             `json:"priority,omitempty"`
	SourceStage  string                           `json:"source_stage,omitempty"`
	SourceID     string                           `json:"source_id,omitempty"`
	BatchID      string                           `json:"batch_id,omitempty"`
	SliceID      string                           `json:"slice_id,omitempty"`
	Subject      string                           `json:"subject,omitempty"`
	OwnerSymbol  string                           `json:"owner_symbol,omitempty"`
	AnchorSymbol string                           `json:"anchor_symbol,omitempty"`
	ReasonCode   string                           `json:"reason_code,omitempty"`
	EvidenceRef  *WriteExplorationEvidenceRef     `json:"evidence_ref,omitempty"`
	Rank         int                              `json:"rank,omitempty"`
}

func OwnerAnchorViewFromSourceLocalizationReview(review SourceLocalizationReview, limit int) OwnerAnchorView {
	review = NormalizeSourceLocalizationReview(review)
	var items []OwnerAnchorViewItem
	for _, anchor := range review.Anchors {
		if item, ok := ownerAnchorViewItemFromAnchor(anchor, WriteContextP1, review.Source, review.PlanID, review.BatchID, ""); ok {
			items = append(items, item)
		}
	}
	return NormalizeOwnerAnchorView(OwnerAnchorView{Items: items}, limit)
}

func OwnerAnchorViewFromWriteContextPack(pack WriteContextPack, consumer WriteContextConsumer, batchID, sliceID string, limit int) OwnerAnchorView {
	pack = NormalizeWriteContextPack(pack)
	view := pack.ViewForScope(consumer, 0, batchID, sliceID)
	items := make([]OwnerAnchorViewItem, 0, len(view.Items))
	for _, item := range view.Items {
		if item.LocalizationAnchor != nil {
			if out, ok := ownerAnchorViewItemFromAnchor(*item.LocalizationAnchor, item.Priority, item.SourceStage, item.SourceID, item.BatchID, item.SliceID); ok {
				out.ID = ownerAnchorFirstNonEmpty(item.ID, out.ID)
				items = append(items, out)
			}
			continue
		}
		if item.Kind == "scope_anchor" && (item.Priority == WriteContextP0 || item.Priority == WriteContextP1) {
			anchor := SourceLocalizationAnchor{
				Path:        item.Text,
				Role:        ClassifySourcePathRole(item.Text),
				SourceStage: item.SourceStage,
				Kind:        SourceLocalizationAnchorScope,
				Strength:    SourceLocalizationAnchorSupporting,
				ReasonCode:  "scope_anchor_prior_context",
			}
			if out, ok := ownerAnchorViewItemFromAnchor(anchor, item.Priority, item.SourceStage, item.SourceID, item.BatchID, item.SliceID); ok {
				out.ID = ownerAnchorFirstNonEmpty(item.ID, out.ID)
				items = append(items, out)
			}
		}
	}
	out := NormalizeOwnerAnchorView(OwnerAnchorView{Items: items}, limit)
	return out
}

func OwnerAnchorViewFromWriteContextPacks(packs []WriteContextPack, consumer WriteContextConsumer, batchID, sliceID string, limit int) OwnerAnchorView {
	var items []OwnerAnchorViewItem
	total := 0
	for _, pack := range packs {
		view := OwnerAnchorViewFromWriteContextPack(pack, consumer, batchID, sliceID, 0)
		total += view.TotalItems
		items = append(items, view.Items...)
	}
	out := NormalizeOwnerAnchorView(OwnerAnchorView{Items: items}, limit)
	if total > 0 {
		out.TotalItems = total
	}
	return out
}

// OwnerAnchorViewFromChangePlan returns the durable owner-anchor view stamped
// onto a plan. It falls back to the plan localization review for older plans or
// tests that have not yet persisted OwnerAnchors.
func OwnerAnchorViewFromChangePlan(plan *ChangePlan, limit int) OwnerAnchorView {
	if plan == nil {
		return OwnerAnchorView{}
	}
	if len(plan.OwnerAnchors) > 0 {
		return NormalizeOwnerAnchorView(OwnerAnchorView{Items: plan.OwnerAnchors}, limit)
	}
	if plan.LocalizationReview != nil {
		return OwnerAnchorViewFromSourceLocalizationReview(*plan.LocalizationReview, limit)
	}
	return OwnerAnchorView{}
}

// StampChangePlanOwnerAnchors records which typed owner/support anchors were
// selected for the current plan. The selection is derived from
// SourceLocalizationReview, not prompt text or model rationale.
func StampChangePlanOwnerAnchors(plan *ChangePlan, limit int) {
	if plan == nil {
		return
	}
	if plan.LocalizationReview == nil {
		plan.OwnerAnchors = nil
		return
	}
	plan.OwnerAnchors = OwnerAnchorViewFromSourceLocalizationReview(*plan.LocalizationReview, limit).Items
}

func NormalizeOwnerAnchorView(in OwnerAnchorView, limit int) OwnerAnchorView {
	seen := map[string]int{}
	items := make([]OwnerAnchorViewItem, 0, len(in.Items))
	for _, item := range in.Items {
		item = normalizeOwnerAnchorViewItem(item)
		if item.Path == "" {
			continue
		}
		key := ownerAnchorViewItemKey(item)
		if idx, ok := seen[key]; ok {
			items[idx] = strongerOwnerAnchorViewItem(items[idx], item)
			continue
		}
		seen[key] = len(items)
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Rank != items[j].Rank {
			return items[i].Rank < items[j].Rank
		}
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		if (items[i].OwnerSymbol != "") != (items[j].OwnerSymbol != "") {
			return items[i].OwnerSymbol != ""
		}
		if items[i].OwnerSymbol != items[j].OwnerSymbol {
			return items[i].OwnerSymbol < items[j].OwnerSymbol
		}
		if items[i].AnchorSymbol != items[j].AnchorSymbol {
			return items[i].AnchorSymbol < items[j].AnchorSymbol
		}
		return items[i].ID < items[j].ID
	})
	total := len(items)
	if limit <= 0 || limit > ownerAnchorViewMaxItems {
		limit = ownerAnchorViewMaxItems
	}
	truncated := false
	if len(items) > limit {
		items = items[:limit]
		truncated = true
	}
	out := OwnerAnchorView{
		Items:        items,
		TotalItems:   total,
		VisibleItems: len(items),
		Truncated:    truncated || in.Truncated,
	}
	strongSeen := map[string]struct{}{}
	for _, item := range items {
		if ownerAnchorViewItemIsStrong(item) {
			out.HasStrong = true
			if _, ok := strongSeen[item.Path]; !ok {
				strongSeen[item.Path] = struct{}{}
				out.StrongPaths = append(out.StrongPaths, item.Path)
			}
		}
		if item.Strength == SourceLocalizationAnchorOwner && !SourcePathRoleIsAuxiliary(item.Role) {
			out.HasOwner = true
		}
	}
	sort.Strings(out.StrongPaths)
	return out
}

func (v OwnerAnchorView) StrongCount() int {
	v = NormalizeOwnerAnchorView(v, 0)
	return len(v.StrongPaths)
}

func (v OwnerAnchorView) StrongSourcePaths() []string {
	v = NormalizeOwnerAnchorView(v, 0)
	return append([]string(nil), v.StrongPaths...)
}

// OwnerAnchorCandidatePaths returns deterministic repair/exploration candidate
// paths with strong typed owner/evidence anchors first, followed by caller
// fallbacks. It is a scheduling projection over typed anchors only; prompt text,
// model rationale, user prose, and terminal narratives never participate.
func OwnerAnchorCandidatePaths(view OwnerAnchorView, fallback []string, limit int) []string {
	if limit <= 0 {
		limit = writeExplorationMaxListItems
	}
	view = NormalizeOwnerAnchorView(view, 0)
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(limit, len(view.Items)+len(fallback)))
	add := func(raw string) {
		p := sourceLocalizationPath(raw)
		if p == "" {
			return
		}
		key := strings.ToLower(p)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	for _, item := range view.Items {
		if !ownerAnchorViewItemIsRepairCandidate(item) {
			continue
		}
		add(item.Path)
		if len(out) >= limit {
			return out
		}
	}
	for _, p := range fallback {
		add(p)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

// OwnerAnchorEvidenceRequirements renders compact deterministic requirement
// rows for owner/evidence anchors. They are soft planning/exploration guidance
// generated from typed fields, not a hard routing surface.
func OwnerAnchorEvidenceRequirements(view OwnerAnchorView, limit int) []string {
	if limit <= 0 {
		limit = 8
	}
	view = NormalizeOwnerAnchorView(view, 0)
	var out []string
	for _, item := range view.Items {
		if !ownerAnchorViewItemIsRepairCandidate(item) {
			continue
		}
		parts := []string{"owner_anchor", "path=" + item.Path}
		if item.OwnerSymbol != "" {
			parts = append(parts, "owner_symbol="+item.OwnerSymbol)
		}
		if item.AnchorSymbol != "" {
			parts = append(parts, "anchor_symbol="+item.AnchorSymbol)
		}
		if item.Subject != "" {
			parts = append(parts, "subject="+item.Subject)
		}
		if item.EvidenceRef != nil && item.EvidenceRef.ID != "" {
			parts = append(parts, "evidence_ref="+item.EvidenceRef.ID)
		}
		if item.ReasonCode != "" {
			parts = append(parts, "reason="+item.ReasonCode)
		}
		out = append(out, strings.Join(parts, " "))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func ownerAnchorViewItemFromAnchor(anchor SourceLocalizationAnchor, priority WriteContextPriority, sourceStage, sourceID, batchID, sliceID string) (OwnerAnchorViewItem, bool) {
	anchor = normalizeSourceLocalizationAnchor(anchor)
	if anchor.Path == "" {
		return OwnerAnchorViewItem{}, false
	}
	item := OwnerAnchorViewItem{
		ID:           sourceLocalizationAnchorKey(anchor),
		Path:         anchor.Path,
		Role:         anchor.Role,
		Kind:         anchor.Kind,
		Strength:     anchor.Strength,
		Priority:     normalizeWriteContextPriority(priority),
		SourceStage:  ownerAnchorFirstNonEmpty(sourceStage, anchor.SourceStage),
		SourceID:     strings.TrimSpace(sourceID),
		BatchID:      strings.TrimSpace(batchID),
		SliceID:      strings.TrimSpace(sliceID),
		Subject:      anchor.Subject,
		OwnerSymbol:  anchor.OwnerSymbol,
		AnchorSymbol: anchor.AnchorSymbol,
		ReasonCode:   anchor.ReasonCode,
	}
	if anchor.EvidenceRef != nil {
		ref := normalizeWriteExplorationEvidenceRef(*anchor.EvidenceRef)
		item.EvidenceRef = &ref
	}
	return normalizeOwnerAnchorViewItem(item), true
}

func normalizeOwnerAnchorViewItem(in OwnerAnchorViewItem) OwnerAnchorViewItem {
	anchor := normalizeSourceLocalizationAnchor(SourceLocalizationAnchor{
		Path:         in.Path,
		Role:         in.Role,
		Kind:         in.Kind,
		Strength:     in.Strength,
		SourceStage:  in.SourceStage,
		Subject:      in.Subject,
		OwnerSymbol:  in.OwnerSymbol,
		AnchorSymbol: in.AnchorSymbol,
		ReasonCode:   in.ReasonCode,
		EvidenceRef:  in.EvidenceRef,
	})
	in.ID = trimSourceLocalizationText(in.ID)
	in.Path = anchor.Path
	in.Role = anchor.Role
	in.Kind = anchor.Kind
	in.Strength = anchor.Strength
	in.Priority = normalizeWriteContextPriority(in.Priority)
	in.SourceStage = trimSourceLocalizationText(anchor.SourceStage)
	in.SourceID = trimSourceLocalizationText(in.SourceID)
	in.BatchID = trimSourceLocalizationText(in.BatchID)
	in.SliceID = trimSourceLocalizationText(in.SliceID)
	in.Subject = anchor.Subject
	in.OwnerSymbol = anchor.OwnerSymbol
	in.AnchorSymbol = anchor.AnchorSymbol
	in.ReasonCode = anchor.ReasonCode
	if anchor.EvidenceRef != nil {
		ref := *anchor.EvidenceRef
		in.EvidenceRef = &ref
	} else {
		in.EvidenceRef = nil
	}
	in.Rank = ownerAnchorViewItemRank(in)
	if in.ID == "" {
		in.ID = ownerAnchorViewItemKey(in)
	}
	return in
}

func strongerOwnerAnchorViewItem(existing, incoming OwnerAnchorViewItem) OwnerAnchorViewItem {
	if incoming.Rank < existing.Rank {
		return incoming
	}
	if incoming.Rank > existing.Rank {
		return existing
	}
	out := existing
	if out.EvidenceRef == nil && incoming.EvidenceRef != nil {
		ref := *incoming.EvidenceRef
		out.EvidenceRef = &ref
	} else if out.EvidenceRef != nil && incoming.EvidenceRef != nil {
		ref := mergeWriteExplorationEvidenceRef(*out.EvidenceRef, *incoming.EvidenceRef)
		out.EvidenceRef = &ref
	}
	if out.SourceStage == "" {
		out.SourceStage = incoming.SourceStage
	}
	if out.SourceID == "" {
		out.SourceID = incoming.SourceID
	}
	if out.Subject == "" {
		out.Subject = incoming.Subject
	}
	if out.OwnerSymbol == "" {
		out.OwnerSymbol = incoming.OwnerSymbol
	}
	if out.AnchorSymbol == "" {
		out.AnchorSymbol = incoming.AnchorSymbol
	}
	if out.ReasonCode == "" {
		out.ReasonCode = incoming.ReasonCode
	}
	return normalizeOwnerAnchorViewItem(out)
}

func ownerAnchorViewItemKey(item OwnerAnchorViewItem) string {
	ref := ""
	if item.EvidenceRef != nil {
		ref = strings.Join([]string{
			item.EvidenceRef.ID,
			sourceLocalizationPath(item.EvidenceRef.Source),
			fmtIntForSourceLocalization(item.EvidenceRef.LineStart),
			fmtIntForSourceLocalization(item.EvidenceRef.LineEnd),
		}, ":")
	}
	return strings.Join([]string{
		item.Path,
		string(item.Role),
		string(item.Kind),
		string(item.Strength),
		item.SourceStage,
		item.Subject,
		item.OwnerSymbol,
		item.AnchorSymbol,
		ref,
	}, "\x00")
}

func ownerAnchorViewItemRank(item OwnerAnchorViewItem) int {
	rolePenalty := 0
	if SourcePathRoleIsAuxiliary(item.Role) {
		rolePenalty = 20
	}
	priorityPenalty := writeContextPriorityRank(item.Priority)
	switch item.Strength {
	case SourceLocalizationAnchorOwner:
		return rolePenalty + priorityPenalty
	case SourceLocalizationAnchorSupporting:
		return rolePenalty + 10 + priorityPenalty
	case SourceLocalizationAnchorObserved:
		return rolePenalty + 20 + priorityPenalty
	default:
		return rolePenalty + 30 + priorityPenalty
	}
}

func ownerAnchorViewItemIsStrong(item OwnerAnchorViewItem) bool {
	if SourcePathRoleIsAuxiliary(item.Role) {
		return false
	}
	switch item.Strength {
	case SourceLocalizationAnchorOwner, SourceLocalizationAnchorSupporting:
		return true
	default:
		return false
	}
}

func ownerAnchorViewItemIsRepairCandidate(item OwnerAnchorViewItem) bool {
	item = normalizeOwnerAnchorViewItem(item)
	if !ownerAnchorViewItemIsStrong(item) || item.Kind == SourceLocalizationAnchorScope {
		return false
	}
	return item.EvidenceRef != nil ||
		item.OwnerSymbol != "" ||
		item.Subject != "" ||
		item.AnchorSymbol != "" ||
		item.Kind == SourceLocalizationAnchorGroundedEvidence ||
		item.Kind == SourceLocalizationAnchorRecoveredEvidence ||
		item.Kind == SourceLocalizationAnchorEvidence
}

func ownerAnchorFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
