package types

import (
	"path"
	"sort"
	"strings"
)

const (
	sourceLocalizationMaxPaths       = 96
	sourceLocalizationMaxReasonCodes = 24
	sourceLocalizationMaxEvidence    = 24
)

// SourceLocalizationStatus is the typed localization-quality verdict shared by
// read-mode handoff and write-mode planning. It is derived from paths and
// evidence refs, never from user prose, model rationale, terminal logs, or
// prompt text.
type SourceLocalizationStatus string

const (
	SourceLocalizationUnknown   SourceLocalizationStatus = "unknown"
	SourceLocalizationObserved  SourceLocalizationStatus = "observed"
	SourceLocalizationSupported SourceLocalizationStatus = "supported"
	SourceLocalizationWeak      SourceLocalizationStatus = "weak"
	SourceLocalizationMissing   SourceLocalizationStatus = "missing"
)

// SourceLocalizationReview is the durable typed view of whether source paths
// in a read/write task were localized by prior evidence. SourcePaths are
// production-like code paths; AuxiliaryPaths preserve tests/docs/fixtures as
// supporting context without letting them satisfy owner-boundary coverage.
type SourceLocalizationReview struct {
	Status            SourceLocalizationStatus      `json:"status,omitempty"`
	Source            string                        `json:"source,omitempty"`
	PlanID            string                        `json:"plan_id,omitempty"`
	BatchID           string                        `json:"batch_id,omitempty"`
	Goal              string                        `json:"goal,omitempty"`
	ReasonCodes       []string                      `json:"reason_codes,omitempty"`
	SourcePaths       []string                      `json:"source_paths,omitempty"`
	PriorContextPaths []string                      `json:"prior_context_paths,omitempty"`
	SupportedPaths    []string                      `json:"supported_paths,omitempty"`
	MissingPaths      []string                      `json:"missing_paths,omitempty"`
	AuxiliaryPaths    []string                      `json:"auxiliary_paths,omitempty"`
	EvidenceRefs      []WriteExplorationEvidenceRef `json:"evidence_refs,omitempty"`
	SupportRatio      float64                       `json:"support_ratio,omitempty"`
}

func NormalizeSourceLocalizationReview(in SourceLocalizationReview) SourceLocalizationReview {
	in.Source = trimSourceLocalizationText(in.Source)
	in.PlanID = trimSourceLocalizationText(in.PlanID)
	in.BatchID = trimSourceLocalizationText(in.BatchID)
	in.Goal = trimSourceLocalizationText(in.Goal)
	in.SourcePaths = dedupSourceLocalizationPaths(in.SourcePaths, sourceLocalizationMaxPaths)
	in.PriorContextPaths = dedupSourceLocalizationPaths(in.PriorContextPaths, sourceLocalizationMaxPaths)
	in.SupportedPaths = dedupSourceLocalizationPaths(in.SupportedPaths, sourceLocalizationMaxPaths)
	in.MissingPaths = dedupSourceLocalizationPaths(in.MissingPaths, sourceLocalizationMaxPaths)
	in.AuxiliaryPaths = dedupSourceLocalizationPaths(in.AuxiliaryPaths, sourceLocalizationMaxPaths)
	in.ReasonCodes = dedupSourceLocalizationStrings(in.ReasonCodes, sourceLocalizationMaxReasonCodes)
	in.EvidenceRefs = normalizeSourceLocalizationEvidenceRefs(in.EvidenceRefs)
	if len(in.SourcePaths) > 0 && len(in.SupportedPaths) > 0 {
		in.SupportRatio = float64(len(in.SupportedPaths)) / float64(len(in.SourcePaths))
	} else if len(in.SourcePaths) > 0 && in.SupportRatio < 0 {
		in.SupportRatio = 0
	}
	if in.SupportRatio < 0 {
		in.SupportRatio = 0
	}
	if in.SupportRatio > 1 {
		in.SupportRatio = 1
	}
	switch in.Status {
	case SourceLocalizationObserved, SourceLocalizationSupported, SourceLocalizationWeak, SourceLocalizationMissing:
	default:
		in.Status = SourceLocalizationUnknown
	}
	if in.Status == SourceLocalizationUnknown {
		in.Status = inferSourceLocalizationStatus(in)
	}
	if in.Status == SourceLocalizationUnknown &&
		len(in.ReasonCodes) == 0 &&
		len(in.SourcePaths) == 0 &&
		len(in.PriorContextPaths) == 0 &&
		len(in.AuxiliaryPaths) == 0 &&
		len(in.EvidenceRefs) == 0 {
		return SourceLocalizationReview{}
	}
	return in
}

func CloneSourceLocalizationReview(in SourceLocalizationReview) SourceLocalizationReview {
	return NormalizeSourceLocalizationReview(SourceLocalizationReview{
		Status:            in.Status,
		Source:            in.Source,
		PlanID:            in.PlanID,
		BatchID:           in.BatchID,
		Goal:              in.Goal,
		ReasonCodes:       append([]string(nil), in.ReasonCodes...),
		SourcePaths:       append([]string(nil), in.SourcePaths...),
		PriorContextPaths: append([]string(nil), in.PriorContextPaths...),
		SupportedPaths:    append([]string(nil), in.SupportedPaths...),
		MissingPaths:      append([]string(nil), in.MissingPaths...),
		AuxiliaryPaths:    append([]string(nil), in.AuxiliaryPaths...),
		EvidenceRefs:      append([]WriteExplorationEvidenceRef(nil), in.EvidenceRefs...),
		SupportRatio:      in.SupportRatio,
	})
}

func CloneSourceLocalizationReviewPtr(in *SourceLocalizationReview) *SourceLocalizationReview {
	if in == nil {
		return nil
	}
	out := CloneSourceLocalizationReview(*in)
	if sourceLocalizationReviewIsEmpty(out) {
		return nil
	}
	return &out
}

func MergeSourceLocalizationReviews(prior, current *SourceLocalizationReview) *SourceLocalizationReview {
	if prior == nil {
		return CloneSourceLocalizationReviewPtr(current)
	}
	if current == nil {
		return CloneSourceLocalizationReviewPtr(prior)
	}
	out := SourceLocalizationReview{
		Status:            strongerSourceLocalizationStatus(prior.Status, current.Status),
		Source:            sourceLocalizationFirstNonEmpty(prior.Source, current.Source),
		PlanID:            sourceLocalizationFirstNonEmpty(current.PlanID, prior.PlanID),
		BatchID:           sourceLocalizationFirstNonEmpty(current.BatchID, prior.BatchID),
		Goal:              sourceLocalizationFirstNonEmpty(current.Goal, prior.Goal),
		ReasonCodes:       append(append([]string(nil), prior.ReasonCodes...), current.ReasonCodes...),
		SourcePaths:       append(append([]string(nil), prior.SourcePaths...), current.SourcePaths...),
		PriorContextPaths: append(append([]string(nil), prior.PriorContextPaths...), current.PriorContextPaths...),
		SupportedPaths:    append(append([]string(nil), prior.SupportedPaths...), current.SupportedPaths...),
		MissingPaths:      append(append([]string(nil), prior.MissingPaths...), current.MissingPaths...),
		AuxiliaryPaths:    append(append([]string(nil), prior.AuxiliaryPaths...), current.AuxiliaryPaths...),
		EvidenceRefs:      append(append([]WriteExplorationEvidenceRef(nil), prior.EvidenceRefs...), current.EvidenceRefs...),
	}
	out = NormalizeSourceLocalizationReview(out)
	if sourceLocalizationReviewIsEmpty(out) {
		return nil
	}
	return &out
}

func SourceLocalizationReviewFromTurnA(readFiles []string, evidence []EvidenceItem) SourceLocalizationReview {
	out := SourceLocalizationReview{Source: "read_turn_a"}
	addPath := func(raw string) {
		p := sourceLocalizationPath(raw)
		if p == "" {
			return
		}
		if SourcePathRoleIsAuxiliary(ClassifySourcePathRole(p)) {
			out.AuxiliaryPaths = append(out.AuxiliaryPaths, p)
			return
		}
		out.SourcePaths = append(out.SourcePaths, p)
	}
	for _, p := range readFiles {
		addPath(p)
	}
	for _, ev := range evidence {
		addPath(ev.Source)
		if sourceLocalizationPath(ev.Source) == "" {
			continue
		}
		ref := WriteExplorationEvidenceRef{
			ID:           ev.ID,
			Kind:         string(ev.Kind),
			Source:       ev.Source,
			LineStart:    ev.LineStart,
			LineEnd:      ev.LineEnd,
			Subject:      ev.Subject,
			AnchorSymbol: ev.AnchorSymbol,
			Summary:      ev.Summary,
		}
		out.EvidenceRefs = append(out.EvidenceRefs, ref)
	}
	if len(out.SourcePaths) > 0 {
		out.Status = SourceLocalizationObserved
		out.ReasonCodes = append(out.ReasonCodes, "read_turn_a_source_observed")
	} else if len(out.AuxiliaryPaths) > 0 || len(out.EvidenceRefs) > 0 {
		out.Status = SourceLocalizationWeak
		out.ReasonCodes = append(out.ReasonCodes, "read_turn_a_auxiliary_only")
	} else {
		out.Status = SourceLocalizationUnknown
		out.ReasonCodes = append(out.ReasonCodes, "read_turn_a_no_source_paths")
	}
	return NormalizeSourceLocalizationReview(out)
}

func SourceLocalizationReviewFromWritePlanContext(batchID, goal string, prior []WriteContextPack, plan *ChangePlan) SourceLocalizationReview {
	if plan == nil {
		return SourceLocalizationReview{}
	}
	contextPaths := writeContextCoveragePriorPaths(prior)
	planPaths := writeContextCoveragePlanPaths(plan)
	out := SourceLocalizationReview{
		Source:            "write_plan_context",
		PlanID:            strings.TrimSpace(plan.ID),
		BatchID:           trimSourceLocalizationText(batchID),
		Goal:              trimSourceLocalizationText(goal),
		SourcePaths:       append([]string(nil), planPaths...),
		PriorContextPaths: append([]string(nil), contextPaths...),
	}
	if len(planPaths) == 0 {
		out.Status = SourceLocalizationUnknown
		out.ReasonCodes = append(out.ReasonCodes, "plan_no_source_paths")
		return NormalizeSourceLocalizationReview(out)
	}
	if len(contextPaths) == 0 {
		out.Status = SourceLocalizationWeak
		out.MissingPaths = append(out.MissingPaths, planPaths...)
		out.ReasonCodes = append(out.ReasonCodes, "plan_source_paths_without_prior_context")
		return NormalizeSourceLocalizationReview(out)
	}
	for _, p := range planPaths {
		if writeContextCoveragePathCoveredByContext(p, contextPaths) {
			out.SupportedPaths = append(out.SupportedPaths, p)
			continue
		}
		out.MissingPaths = append(out.MissingPaths, p)
	}
	switch {
	case len(out.MissingPaths) == 0:
		out.Status = SourceLocalizationSupported
		out.ReasonCodes = append(out.ReasonCodes, "plan_source_paths_supported_by_prior_context")
	case len(out.SupportedPaths) == 0:
		out.Status = SourceLocalizationMissing
		out.ReasonCodes = append(out.ReasonCodes, "plan_source_paths_missing_prior_context")
	default:
		out.Status = SourceLocalizationWeak
		out.ReasonCodes = append(out.ReasonCodes, "plan_source_paths_partially_outside_prior_context")
	}
	return NormalizeSourceLocalizationReview(out)
}

func SourceLocalizationReviewHasSignal(review *SourceLocalizationReview) bool {
	if review == nil {
		return false
	}
	normalized := NormalizeSourceLocalizationReview(*review)
	return !sourceLocalizationReviewIsEmpty(normalized)
}

func renderSourceLocalizationReviewContext(review *SourceLocalizationReview) string {
	if review == nil {
		return ""
	}
	normalized := NormalizeSourceLocalizationReview(*review)
	if normalized.Status == "" {
		return ""
	}
	parts := []string{
		"status=" + string(normalized.Status),
	}
	if len(normalized.ReasonCodes) > 0 {
		parts = append(parts, "reasons="+strings.Join(normalized.ReasonCodes, ","))
	}
	if len(normalized.SourcePaths) > 0 {
		parts = append(parts, "source_paths="+formatWriteContextCoveragePaths(normalized.SourcePaths))
	}
	if len(normalized.PriorContextPaths) > 0 {
		parts = append(parts, "prior_context_paths="+formatWriteContextCoveragePaths(normalized.PriorContextPaths))
	}
	if len(normalized.SupportedPaths) > 0 {
		parts = append(parts, "supported_paths="+formatWriteContextCoveragePaths(normalized.SupportedPaths))
	}
	if len(normalized.MissingPaths) > 0 {
		parts = append(parts, "missing_paths="+formatWriteContextCoveragePaths(normalized.MissingPaths))
	}
	return strings.Join(parts, " ")
}

func inferSourceLocalizationStatus(in SourceLocalizationReview) SourceLocalizationStatus {
	switch {
	case len(in.MissingPaths) > 0 && len(in.SupportedPaths) == 0 && len(in.PriorContextPaths) > 0:
		return SourceLocalizationMissing
	case len(in.MissingPaths) > 0:
		return SourceLocalizationWeak
	case len(in.SourcePaths) > 0 && len(in.SupportedPaths) == len(in.SourcePaths):
		return SourceLocalizationSupported
	case len(in.SourcePaths) > 0:
		return SourceLocalizationObserved
	case len(in.AuxiliaryPaths) > 0 || len(in.EvidenceRefs) > 0:
		return SourceLocalizationWeak
	default:
		return SourceLocalizationUnknown
	}
}

func strongerSourceLocalizationStatus(a, b SourceLocalizationStatus) SourceLocalizationStatus {
	if sourceLocalizationStatusRank(b) > sourceLocalizationStatusRank(a) {
		return b
	}
	if a != "" {
		return a
	}
	return b
}

func sourceLocalizationStatusRank(status SourceLocalizationStatus) int {
	switch status {
	case SourceLocalizationMissing:
		return 5
	case SourceLocalizationWeak:
		return 4
	case SourceLocalizationSupported:
		return 3
	case SourceLocalizationObserved:
		return 2
	case SourceLocalizationUnknown:
		return 1
	default:
		return 0
	}
}

func normalizeSourceLocalizationEvidenceRefs(in []WriteExplorationEvidenceRef) []WriteExplorationEvidenceRef {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]WriteExplorationEvidenceRef, 0, len(in))
	for _, ref := range in {
		ref = normalizeWriteExplorationEvidenceRef(ref)
		if ref.Source == "" && ref.ID == "" && ref.Summary == "" {
			continue
		}
		key := strings.TrimSpace(ref.ID)
		if key == "" {
			key = strings.Join([]string{
				sourceLocalizationPath(ref.Source),
				strings.TrimSpace(ref.Kind),
				strings.TrimSpace(ref.Subject),
				strings.TrimSpace(ref.AnchorSymbol),
				strings.TrimSpace(ref.Summary),
			}, "|")
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
		if len(out) >= sourceLocalizationMaxEvidence {
			break
		}
	}
	return out
}

func sourceLocalizationPath(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return ""
	}
	cleaned := path.Clean(raw)
	cleaned = strings.TrimPrefix(cleaned, "./")
	if idx := strings.Index(cleaned, ":"); idx > 0 {
		head := cleaned[:idx]
		if strings.Contains(head, "/") || path.Ext(head) != "" {
			cleaned = head
		}
	}
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return ""
	}
	return cleaned
}

func dedupSourceLocalizationPaths(in []string, limit int) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		p := sourceLocalizationPath(raw)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func dedupSourceLocalizationStrings(in []string, limit int) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := trimSourceLocalizationText(raw)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func trimSourceLocalizationText(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > writeContextPackTextLen {
		raw = raw[:writeContextPackTextLen]
	}
	return raw
}

func sourceLocalizationReviewIsEmpty(in SourceLocalizationReview) bool {
	return in.Status == "" ||
		(in.Status == SourceLocalizationUnknown &&
			len(in.SourcePaths) == 0 &&
			len(in.PriorContextPaths) == 0 &&
			len(in.SupportedPaths) == 0 &&
			len(in.MissingPaths) == 0 &&
			len(in.AuxiliaryPaths) == 0 &&
			len(in.EvidenceRefs) == 0)
}

func sourceLocalizationFirstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}
