package types

import (
	"path"
	"sort"
	"strconv"
	"strings"
)

const (
	sourceLocalizationMaxPaths       = 96
	sourceLocalizationMaxReasonCodes = 24
	sourceLocalizationMaxEvidence    = 24
	sourceLocalizationMaxAnchors     = 64
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

// SourceLocalizationAnchorKind describes the structured origin of a
// localization anchor. It is intentionally derived from typed tool/evidence
// metadata, not prose, so downstream write gates can decide whether a path is
// merely observed or actually owner-supported.
type SourceLocalizationAnchorKind string

const (
	SourceLocalizationAnchorReadFile          SourceLocalizationAnchorKind = "read_file"
	SourceLocalizationAnchorGroundedEvidence  SourceLocalizationAnchorKind = "grounded_evidence"
	SourceLocalizationAnchorRecoveredEvidence SourceLocalizationAnchorKind = "recovered_evidence"
	SourceLocalizationAnchorEvidence          SourceLocalizationAnchorKind = "evidence"
	SourceLocalizationAnchorScope             SourceLocalizationAnchorKind = "scope_anchor"
)

// SourceLocalizationAnchorStrength is the typed support level for one path.
// Owner/supporting anchors may satisfy write planning localization coverage;
// observed anchors are useful handoff/audit context but not owner proof.
type SourceLocalizationAnchorStrength string

const (
	SourceLocalizationAnchorObserved   SourceLocalizationAnchorStrength = "observed"
	SourceLocalizationAnchorSupporting SourceLocalizationAnchorStrength = "supporting"
	SourceLocalizationAnchorOwner      SourceLocalizationAnchorStrength = "owner"
)

// SourceLocalizationAnchor is a read/write shared owner-localization pointer.
// Text renderers may summarize it for prompts, but hard gates must consume this
// typed struct directly.
type SourceLocalizationAnchor struct {
	Path         string                           `json:"path,omitempty"`
	Role         SourcePathRole                   `json:"role,omitempty"`
	SourceStage  string                           `json:"source_stage,omitempty"`
	Kind         SourceLocalizationAnchorKind     `json:"kind,omitempty"`
	Strength     SourceLocalizationAnchorStrength `json:"strength,omitempty"`
	EvidenceRef  *WriteExplorationEvidenceRef     `json:"evidence_ref,omitempty"`
	Subject      string                           `json:"subject,omitempty"`
	OwnerSymbol  string                           `json:"owner_symbol,omitempty"`
	AnchorSymbol string                           `json:"anchor_symbol,omitempty"`
	ReasonCode   string                           `json:"reason_code,omitempty"`
}

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
	Anchors           []SourceLocalizationAnchor    `json:"anchors,omitempty"`
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
	in.Anchors = normalizeSourceLocalizationAnchors(in.Anchors)
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
		len(in.EvidenceRefs) == 0 &&
		len(in.Anchors) == 0 {
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
		Anchors:           append([]SourceLocalizationAnchor(nil), in.Anchors...),
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
		Anchors:           append(append([]SourceLocalizationAnchor(nil), prior.Anchors...), current.Anchors...),
	}
	out = NormalizeSourceLocalizationReview(out)
	if sourceLocalizationReviewIsEmpty(out) {
		return nil
	}
	return &out
}

func SourceLocalizationReviewFromTurnA(readFiles []string, evidence []EvidenceItem) SourceLocalizationReview {
	out := SourceLocalizationReview{Source: "read_turn_a"}
	addAnchor := func(anchor SourceLocalizationAnchor) {
		anchor = normalizeSourceLocalizationAnchor(anchor)
		if anchor.Path == "" {
			return
		}
		out.Anchors = append(out.Anchors, anchor)
	}
	addPath := func(raw string) {
		p := sourceLocalizationPath(raw)
		if p == "" {
			return
		}
		role := ClassifySourcePathRole(p)
		if SourcePathRoleIsAuxiliary(role) {
			out.AuxiliaryPaths = append(out.AuxiliaryPaths, p)
			return
		}
		out.SourcePaths = append(out.SourcePaths, p)
		addAnchor(SourceLocalizationAnchor{
			Path:        p,
			Role:        role,
			SourceStage: "read_turn_a",
			Kind:        SourceLocalizationAnchorReadFile,
			Strength:    SourceLocalizationAnchorObserved,
			ReasonCode:  "read_file_observed",
		})
	}
	for _, p := range readFiles {
		addPath(p)
	}
	for _, ev := range evidence {
		p := sourceLocalizationPath(ev.Source)
		addPath(ev.Source)
		if p == "" {
			continue
		}
		ownerSymbol := sourceLocalizationEvidenceOwnerSymbol(ev)
		ref := WriteExplorationEvidenceRef{
			ID:           ev.ID,
			Kind:         string(ev.Kind),
			Source:       ev.Source,
			LineStart:    ev.LineStart,
			LineEnd:      ev.LineEnd,
			Subject:      ev.Subject,
			OwnerSymbol:  ownerSymbol,
			AnchorSymbol: ev.AnchorSymbol,
			Summary:      ev.Summary,
		}
		out.EvidenceRefs = append(out.EvidenceRefs, ref)
		role := ClassifySourcePathRole(p)
		if SourcePathRoleIsAuxiliary(role) {
			continue
		}
		kind, strength, reason := sourceLocalizationAnchorFromEvidence(ev)
		if strength == "" {
			continue
		}
		addAnchor(SourceLocalizationAnchor{
			Path:         p,
			Role:         role,
			SourceStage:  "read_turn_a",
			Kind:         kind,
			Strength:     strength,
			EvidenceRef:  &ref,
			Subject:      ev.Subject,
			OwnerSymbol:  ownerSymbol,
			AnchorSymbol: ev.AnchorSymbol,
			ReasonCode:   reason,
		})
	}
	if len(out.SourcePaths) > 0 {
		out.Status = SourceLocalizationObserved
		out.ReasonCodes = append(out.ReasonCodes, "read_turn_a_source_observed")
		if sourceLocalizationHasStrength(out.Anchors, SourceLocalizationAnchorOwner) {
			out.ReasonCodes = append(out.ReasonCodes, "read_turn_a_owner_anchor_observed")
		} else if sourceLocalizationHasStrength(out.Anchors, SourceLocalizationAnchorSupporting) {
			out.ReasonCodes = append(out.ReasonCodes, "read_turn_a_supporting_anchor_observed")
		}
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
	contextAnchors := writeContextCoveragePriorAnchors(prior)
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
			out.Anchors = append(out.Anchors, writeContextCoveragePriorAnchorsForPath(contextAnchors, p)...)
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

func renderSourceLocalizationAnchorContext(anchor SourceLocalizationAnchor) string {
	anchor = normalizeSourceLocalizationAnchor(anchor)
	if anchor.Path == "" {
		return ""
	}
	parts := []string{
		"path=" + anchor.Path,
		"role=" + string(anchor.Role),
		"kind=" + string(anchor.Kind),
		"strength=" + string(anchor.Strength),
	}
	if anchor.SourceStage != "" {
		parts = append(parts, "source_stage="+anchor.SourceStage)
	}
	if anchor.Subject != "" {
		parts = append(parts, "subject="+anchor.Subject)
	}
	if anchor.OwnerSymbol != "" {
		parts = append(parts, "owner="+anchor.OwnerSymbol)
	}
	if anchor.AnchorSymbol != "" {
		parts = append(parts, "anchor="+anchor.AnchorSymbol)
	}
	if anchor.ReasonCode != "" {
		parts = append(parts, "reason="+anchor.ReasonCode)
	}
	if anchor.EvidenceRef != nil && anchor.EvidenceRef.ID != "" {
		parts = append(parts, "evidence_ref="+anchor.EvidenceRef.ID)
	}
	return strings.Join(parts, " ")
}

func sourceLocalizationAnchorSatisfiesPriorContext(anchor SourceLocalizationAnchor) bool {
	anchor = normalizeSourceLocalizationAnchor(anchor)
	if anchor.Path == "" || SourcePathRoleIsAuxiliary(anchor.Role) {
		return false
	}
	switch anchor.Strength {
	case SourceLocalizationAnchorOwner, SourceLocalizationAnchorSupporting:
		return true
	default:
		return false
	}
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

func sourceLocalizationAnchorFromEvidence(ev EvidenceItem) (SourceLocalizationAnchorKind, SourceLocalizationAnchorStrength, string) {
	if !sourceLocalizationEvidenceCanAnchorOwner(ev) {
		return "", "", ""
	}
	switch ev.GroundingStatus {
	case GroundingGrounded:
		return SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorOwner, "grounded_evidence_owner"
	case GroundingRecovered:
		return SourceLocalizationAnchorRecoveredEvidence, SourceLocalizationAnchorOwner, "recovered_evidence_owner"
	case GroundingUngrounded:
		return "", "", ""
	}
	if ev.LineStart > 0 && ev.Scope.IsLineShaped() {
		return SourceLocalizationAnchorEvidence, SourceLocalizationAnchorSupporting, "line_evidence_supporting"
	}
	if ev.Scope == ScopeFile || ev.Scope == ScopeSection || ev.Scope == ScopeCrossfile {
		return SourceLocalizationAnchorEvidence, SourceLocalizationAnchorSupporting, "scoped_evidence_supporting"
	}
	return "", "", ""
}

func sourceLocalizationEvidenceCanAnchorOwner(ev EvidenceItem) bool {
	if ev.Kind.IsLLMEmittable() {
		return true
	}
	return ev.ContextRole == EvidenceContextRoleDefining
}

func sourceLocalizationEvidenceOwnerSymbol(ev EvidenceItem) string {
	if owner := trimSourceLocalizationText(ev.OwnerSymbol); owner != "" {
		return owner
	}
	if owner := sourceLocalizationOwnerSymbolFromSourceSubject(ev.Source, ev.Subject); owner != "" {
		return owner
	}
	if ev.ContextRole == EvidenceContextRoleDefining && sourceLocalizationLooksOwnerSymbol(ev.Subject) {
		return trimSourceLocalizationText(ev.Subject)
	}
	if ev.AnchorKind == AnchorDefinition && sourceLocalizationLooksOwnerSymbol(ev.AnchorSymbol) {
		return trimSourceLocalizationText(ev.AnchorSymbol)
	}
	return ""
}

func sourceLocalizationOwnerSymbolFromSourceSubject(source, subject string) string {
	p := sourceLocalizationPath(source)
	subject = trimSourceLocalizationText(subject)
	if p == "" || subject == "" {
		return ""
	}
	for _, prefix := range []string{p + "::", p + ":", path.Base(p) + ":"} {
		if strings.HasPrefix(subject, prefix) {
			candidate := strings.TrimPrefix(subject, prefix)
			candidate = strings.Trim(candidate, " \t`'\"")
			if sourceLocalizationLooksOwnerSymbol(candidate) {
				return trimSourceLocalizationText(candidate)
			}
		}
	}
	return ""
}

func sourceLocalizationLooksOwnerSymbol(raw string) bool {
	raw = trimSourceLocalizationText(raw)
	if raw == "" || strings.ContainsAny(raw, "/\\ \t\r\n") {
		return false
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == ':' || r == '#':
		default:
			return false
		}
	}
	return true
}

func sourceLocalizationHasStrength(anchors []SourceLocalizationAnchor, strength SourceLocalizationAnchorStrength) bool {
	for _, anchor := range anchors {
		anchor = normalizeSourceLocalizationAnchor(anchor)
		if anchor.Strength == strength {
			return true
		}
	}
	return false
}

func normalizeSourceLocalizationAnchors(in []SourceLocalizationAnchor) []SourceLocalizationAnchor {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]SourceLocalizationAnchor, 0, len(in))
	for _, anchor := range in {
		anchor = normalizeSourceLocalizationAnchor(anchor)
		if anchor.Path == "" {
			continue
		}
		key := sourceLocalizationAnchorKey(anchor)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, anchor)
		if len(out) >= sourceLocalizationMaxAnchors {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if sourceLocalizationAnchorStrengthRank(out[i].Strength) != sourceLocalizationAnchorStrengthRank(out[j].Strength) {
			return sourceLocalizationAnchorStrengthRank(out[i].Strength) > sourceLocalizationAnchorStrengthRank(out[j].Strength)
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].OwnerSymbol != out[j].OwnerSymbol {
			return out[i].OwnerSymbol < out[j].OwnerSymbol
		}
		return out[i].AnchorSymbol < out[j].AnchorSymbol
	})
	return out
}

func normalizeSourceLocalizationAnchor(in SourceLocalizationAnchor) SourceLocalizationAnchor {
	in.Path = sourceLocalizationPath(in.Path)
	in.SourceStage = trimSourceLocalizationText(in.SourceStage)
	in.Subject = trimSourceLocalizationText(in.Subject)
	in.OwnerSymbol = trimSourceLocalizationText(in.OwnerSymbol)
	in.AnchorSymbol = trimSourceLocalizationText(in.AnchorSymbol)
	in.ReasonCode = trimSourceLocalizationText(in.ReasonCode)
	if in.Role == SourcePathRoleUnknown && in.Path != "" {
		in.Role = ClassifySourcePathRole(in.Path)
	}
	switch in.Kind {
	case SourceLocalizationAnchorReadFile,
		SourceLocalizationAnchorGroundedEvidence,
		SourceLocalizationAnchorRecoveredEvidence,
		SourceLocalizationAnchorEvidence,
		SourceLocalizationAnchorScope:
	default:
		if in.EvidenceRef != nil {
			in.Kind = SourceLocalizationAnchorEvidence
		} else {
			in.Kind = SourceLocalizationAnchorReadFile
		}
	}
	switch in.Strength {
	case SourceLocalizationAnchorObserved,
		SourceLocalizationAnchorSupporting,
		SourceLocalizationAnchorOwner:
	default:
		switch in.Kind {
		case SourceLocalizationAnchorGroundedEvidence, SourceLocalizationAnchorRecoveredEvidence:
			in.Strength = SourceLocalizationAnchorOwner
		case SourceLocalizationAnchorEvidence, SourceLocalizationAnchorScope:
			in.Strength = SourceLocalizationAnchorSupporting
		default:
			in.Strength = SourceLocalizationAnchorObserved
		}
	}
	if in.EvidenceRef != nil {
		ref := normalizeWriteExplorationEvidenceRef(*in.EvidenceRef)
		if ref.Source == "" && ref.ID == "" && ref.Summary == "" {
			in.EvidenceRef = nil
		} else {
			in.EvidenceRef = &ref
		}
	}
	return in
}

func mergeSourceLocalizationAnchor(existing, incoming SourceLocalizationAnchor) SourceLocalizationAnchor {
	out := normalizeSourceLocalizationAnchor(existing)
	incoming = normalizeSourceLocalizationAnchor(incoming)
	if out.Path == "" {
		out.Path = incoming.Path
	}
	if out.Role == SourcePathRoleUnknown {
		out.Role = incoming.Role
	}
	if out.SourceStage == "" {
		out.SourceStage = incoming.SourceStage
	}
	if out.Kind == "" {
		out.Kind = incoming.Kind
	}
	if out.Strength == "" {
		out.Strength = incoming.Strength
	}
	if out.EvidenceRef == nil && incoming.EvidenceRef != nil {
		ref := *incoming.EvidenceRef
		out.EvidenceRef = &ref
	} else if out.EvidenceRef != nil && incoming.EvidenceRef != nil {
		ref := mergeWriteExplorationEvidenceRef(*out.EvidenceRef, *incoming.EvidenceRef)
		out.EvidenceRef = &ref
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
	return normalizeSourceLocalizationAnchor(out)
}

func sourceLocalizationAnchorKey(anchor SourceLocalizationAnchor) string {
	ref := ""
	if anchor.EvidenceRef != nil {
		ref = strings.Join([]string{
			anchor.EvidenceRef.ID,
			sourceLocalizationPath(anchor.EvidenceRef.Source),
			fmtIntForSourceLocalization(anchor.EvidenceRef.LineStart),
			fmtIntForSourceLocalization(anchor.EvidenceRef.LineEnd),
		}, ":")
	}
	return strings.Join([]string{
		anchor.Path,
		string(anchor.Role),
		string(anchor.Kind),
		string(anchor.Strength),
		anchor.SourceStage,
		anchor.Subject,
		anchor.OwnerSymbol,
		anchor.AnchorSymbol,
		ref,
	}, "\x00")
}

func sourceLocalizationAnchorStrengthRank(strength SourceLocalizationAnchorStrength) int {
	switch strength {
	case SourceLocalizationAnchorOwner:
		return 3
	case SourceLocalizationAnchorSupporting:
		return 2
	case SourceLocalizationAnchorObserved:
		return 1
	default:
		return 0
	}
}

func fmtIntForSourceLocalization(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
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
			len(in.EvidenceRefs) == 0 &&
			len(in.Anchors) == 0)
}

func sourceLocalizationFirstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}
