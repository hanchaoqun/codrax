package types

import "strings"

// SourceInventoryCoverageState describes how a source-inventory row should be
// consumed. It is deliberately structural: callers should not infer semantic
// answer intent from these values.
type SourceInventoryCoverageState string

const (
	SourceInventoryCoverageUnknown   SourceInventoryCoverageState = ""
	SourceInventoryCoverageObserved  SourceInventoryCoverageState = "observed"
	SourceInventoryCoverageAmbiguous SourceInventoryCoverageState = "ambiguous"
	SourceInventoryCoverageNeedsRead SourceInventoryCoverageState = "needs_read"
	SourceInventoryCoverageNoIndex   SourceInventoryCoverageState = "no_index"
)

// SourceInventoryObservation is the auditable companion to
// SourceInventoryAdvisory. It preserves the same candidate rows, while making
// count/list invariants and row support refs explicit for downstream ledgers and
// validators. It is still not final-answer text and does not decide semantic
// roles for the model.
type SourceInventoryObservation struct {
	Active       bool                            `json:"active"`
	AdvisoryOnly bool                            `json:"advisory_only,omitempty"`
	Complete     bool                            `json:"complete,omitempty"`
	Scopes       []string                        `json:"scopes,omitempty"`
	Provenance   []string                        `json:"provenance,omitempty"`
	Lens         []string                        `json:"lens,omitempty"`
	Sets         []SourceInventoryObservationSet `json:"sets,omitempty"`
}

// SourceInventoryLensQuery is the model/tool-facing query surface for an
// active repo lens. Empty fields mean "use the already typed request context".
// It is intentionally small and typed so callers can drive exploration without
// adding prose classifiers or bespoke inventory modes.
type SourceInventoryLensQuery struct {
	Path              string                `json:"path,omitempty"`
	Scopes            []string              `json:"scopes,omitempty"`
	Roles             []AnswerCandidateRole `json:"roles,omitempty"`
	AttributeRoles    []AnswerCandidateRole `json:"attribute_roles,omitempty"`
	IncludeAttributes bool                  `json:"include_attributes,omitempty"`
	IncludeCounts     bool                  `json:"include_counts,omitempty"`
	TopN              int                   `json:"top_n,omitempty"`
	Offset            int                   `json:"offset,omitempty"`
	Cursor            string                `json:"cursor,omitempty"`
	Query             string                `json:"query,omitempty"`
	Provenance        []string              `json:"provenance,omitempty"`
}

func (o SourceInventoryObservation) IsActive() bool {
	return o.Active && len(o.Sets) > 0
}

// SourceInventoryObservationSet is one role-bounded member set. Count is a
// machine-checkable invariant and is always normalized to len(Members) by the
// clone/merge/build helpers.
type SourceInventoryObservationSet struct {
	Role     AnswerCandidateRole                `json:"role,omitempty"`
	Complete bool                               `json:"complete,omitempty"`
	Count    int                                `json:"count,omitempty"`
	Members  []SourceInventoryObservationMember `json:"members,omitempty"`
}

// SourceInventoryObservationMember is a mechanical repo-lens row: a member,
// file, package, or symbol that the repo map / filesystem inventory can locate.
type SourceInventoryObservationMember struct {
	Name          string                                `json:"name,omitempty"`
	Key           string                                `json:"key,omitempty"`
	SupportRef    string                                `json:"support_ref,omitempty"`
	Note          string                                `json:"note,omitempty"`
	Role          AnswerCandidateRole                   `json:"role,omitempty"`
	Exported      bool                                  `json:"exported,omitempty"`
	File          string                                `json:"file,omitempty"`
	Line          int                                   `json:"line,omitempty"`
	Language      string                                `json:"language,omitempty"`
	CoverageState SourceInventoryCoverageState          `json:"coverage_state,omitempty"`
	Attributes    []SourceInventoryObservationAttribute `json:"attributes,omitempty"`
}

// SourceInventoryObservationAttribute is row-local context, for example
// candidate callable entrypoints under a package/directory member. Attributes
// remain advisory unless the model later cites or emits them as evidence.
type SourceInventoryObservationAttribute struct {
	Name          string                       `json:"name,omitempty"`
	Key           string                       `json:"key,omitempty"`
	SupportRef    string                       `json:"support_ref,omitempty"`
	Note          string                       `json:"note,omitempty"`
	Role          AnswerCandidateRole          `json:"role,omitempty"`
	Exported      bool                         `json:"exported,omitempty"`
	File          string                       `json:"file,omitempty"`
	Line          int                          `json:"line,omitempty"`
	Language      string                       `json:"language,omitempty"`
	CoverageState SourceInventoryCoverageState `json:"coverage_state,omitempty"`
	Ambiguity     string                       `json:"ambiguity,omitempty"`
	Reason        string                       `json:"reason,omitempty"`
}

func SourceInventoryObservationFromAdvisory(advisory SourceInventoryAdvisory) SourceInventoryObservation {
	if !advisory.IsActive() {
		return SourceInventoryObservation{}
	}
	out := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: advisory.AdvisoryOnly,
		Complete:     advisory.Complete,
		Scopes:       append([]string(nil), advisory.Scopes...),
		Provenance:   append([]string(nil), advisory.Provenance...),
		Lens:         []string{"members", "symbols", "attributes", "count"},
		Sets:         make([]SourceInventoryObservationSet, 0, len(advisory.Sets)),
	}
	for _, set := range advisory.Sets {
		if len(set.Candidates) == 0 {
			continue
		}
		obsSet := SourceInventoryObservationSet{
			Role:     set.Role,
			Complete: set.Complete,
			Members:  make([]SourceInventoryObservationMember, 0, len(set.Candidates)),
		}
		for _, candidate := range set.Candidates {
			member := sourceInventoryObservationMemberFromAdvisory(candidate)
			if strings.TrimSpace(member.Name) == "" {
				continue
			}
			obsSet.Members = append(obsSet.Members, member)
		}
		obsSet.Count = len(obsSet.Members)
		if obsSet.Count == 0 {
			continue
		}
		out.Sets = append(out.Sets, obsSet)
	}
	return normalizeSourceInventoryObservation(out)
}

func sourceInventoryObservationMemberFromAdvisory(candidate SourceInventoryAdvisoryCandidate) SourceInventoryObservationMember {
	attrs := make([]SourceInventoryObservationAttribute, 0, len(candidate.Attributes))
	attributeAmbiguity := ""
	if len(candidate.Attributes) > 1 {
		attributeAmbiguity = "one_of_many_candidate_attributes"
	}
	for _, attr := range candidate.Attributes {
		item := SourceInventoryObservationAttribute{
			Name:          attr.Member,
			Key:           attr.Key,
			SupportRef:    attr.SupportRef,
			Note:          attr.Note,
			Role:          attr.Role,
			Exported:      attr.Exported,
			File:          attr.File,
			Line:          attr.Line,
			Language:      attr.Language,
			CoverageState: sourceInventoryObservationCoverage(attr.SupportRef, attr.File),
			Ambiguity:     attributeAmbiguity,
		}
		if attributeAmbiguity != "" {
			item.Reason = "Multiple graph-backed callable attributes are present under this member; the model must choose or disclose ambiguity."
			item.CoverageState = SourceInventoryCoverageAmbiguous
		}
		if strings.TrimSpace(item.Name) != "" {
			attrs = append(attrs, item)
		}
	}
	return SourceInventoryObservationMember{
		Name:          candidate.Member,
		Key:           candidate.Key,
		SupportRef:    candidate.SupportRef,
		Note:          candidate.Note,
		Role:          candidate.Role,
		Exported:      candidate.Exported,
		File:          candidate.File,
		Line:          candidate.Line,
		Language:      candidate.Language,
		CoverageState: sourceInventoryObservationCoverage(candidate.SupportRef, candidate.File),
		Attributes:    attrs,
	}
}

func sourceInventoryObservationCoverage(supportRef, file string) SourceInventoryCoverageState {
	if strings.TrimSpace(supportRef) != "" || strings.TrimSpace(file) != "" {
		return SourceInventoryCoverageObserved
	}
	return SourceInventoryCoverageNeedsRead
}

func CloneSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	out := in
	out.Scopes = append([]string(nil), in.Scopes...)
	out.Provenance = append([]string(nil), in.Provenance...)
	out.Lens = append([]string(nil), in.Lens...)
	if in.Sets != nil {
		out.Sets = make([]SourceInventoryObservationSet, len(in.Sets))
		for i, set := range in.Sets {
			out.Sets[i] = set
			out.Sets[i].Members = cloneSourceInventoryObservationMembers(set.Members)
			out.Sets[i].Count = len(out.Sets[i].Members)
		}
	}
	return normalizeSourceInventoryObservation(out)
}

func MergeSourceInventoryObservation(prior, current SourceInventoryObservation) SourceInventoryObservation {
	if !prior.IsActive() {
		return CloneSourceInventoryObservation(current)
	}
	if !current.IsActive() {
		return CloneSourceInventoryObservation(prior)
	}
	merged := CloneSourceInventoryObservation(prior)
	merged.Active = true
	merged.AdvisoryOnly = prior.AdvisoryOnly && current.AdvisoryOnly
	merged.Complete = prior.Complete && current.Complete
	merged.Scopes = mergeSourceInventoryAdvisoryStrings(merged.Scopes, current.Scopes)
	merged.Provenance = mergeSourceInventoryAdvisoryStrings(merged.Provenance, current.Provenance)
	merged.Lens = mergeSourceInventoryAdvisoryStrings(merged.Lens, current.Lens)
	byRole := make(map[AnswerCandidateRole]int, len(merged.Sets))
	for i := range merged.Sets {
		byRole[merged.Sets[i].Role] = i
	}
	for _, set := range current.Sets {
		if idx, ok := byRole[set.Role]; ok {
			merged.Sets[idx].Complete = merged.Sets[idx].Complete && set.Complete
			merged.Sets[idx].Members = mergeSourceInventoryObservationMembers(merged.Sets[idx].Members, set.Members)
			merged.Sets[idx].Count = len(merged.Sets[idx].Members)
			continue
		}
		byRole[set.Role] = len(merged.Sets)
		cloned := SourceInventoryObservationSet{
			Role:     set.Role,
			Complete: set.Complete,
			Members:  cloneSourceInventoryObservationMembers(set.Members),
		}
		cloned.Count = len(cloned.Members)
		merged.Sets = append(merged.Sets, cloned)
	}
	return normalizeSourceInventoryObservation(merged)
}

func normalizeSourceInventoryObservation(in SourceInventoryObservation) SourceInventoryObservation {
	if len(in.Sets) == 0 {
		return SourceInventoryObservation{}
	}
	in.Active = true
	for i := range in.Sets {
		in.Sets[i].Count = len(in.Sets[i].Members)
	}
	return in
}

func cloneSourceInventoryObservationMembers(in []SourceInventoryObservationMember) []SourceInventoryObservationMember {
	if in == nil {
		return nil
	}
	out := make([]SourceInventoryObservationMember, len(in))
	for i, member := range in {
		out[i] = member
		out[i].Attributes = append([]SourceInventoryObservationAttribute(nil), member.Attributes...)
	}
	return out
}

func mergeSourceInventoryObservationMembers(existing, incoming []SourceInventoryObservationMember) []SourceInventoryObservationMember {
	if len(existing) == 0 {
		return cloneSourceInventoryObservationMembers(incoming)
	}
	out := cloneSourceInventoryObservationMembers(existing)
	byKey := make(map[string]int, len(out)+len(incoming))
	for i, member := range out {
		if key := sourceInventoryObservationMemberKey(member); key != "" {
			byKey[key] = i
		}
	}
	for _, member := range incoming {
		key := sourceInventoryObservationMemberKey(member)
		if key == "" {
			continue
		}
		if idx, ok := byKey[key]; ok {
			out[idx].Attributes = mergeSourceInventoryObservationAttributes(out[idx].Attributes, member.Attributes)
			if out[idx].CoverageState == SourceInventoryCoverageUnknown {
				out[idx].CoverageState = member.CoverageState
			}
			continue
		}
		byKey[key] = len(out)
		cloned := member
		cloned.Attributes = append([]SourceInventoryObservationAttribute(nil), member.Attributes...)
		out = append(out, cloned)
	}
	return out
}

func mergeSourceInventoryObservationAttributes(existing, incoming []SourceInventoryObservationAttribute) []SourceInventoryObservationAttribute {
	if len(existing) == 0 {
		return append([]SourceInventoryObservationAttribute(nil), incoming...)
	}
	out := append([]SourceInventoryObservationAttribute(nil), existing...)
	seen := make(map[string]bool, len(out)+len(incoming))
	for _, attr := range out {
		if key := sourceInventoryObservationAttributeKey(attr); key != "" {
			seen[key] = true
		}
	}
	for _, attr := range incoming {
		key := sourceInventoryObservationAttributeKey(attr)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, attr)
	}
	return out
}

func sourceInventoryObservationMemberKey(member SourceInventoryObservationMember) string {
	key := strings.TrimSpace(member.Key)
	if key == "" {
		key = strings.TrimSpace(member.Name)
	}
	if key == "" {
		return ""
	}
	return string(member.Role) + "\x00" + key + "\x00" + strings.TrimSpace(member.File) + "\x00" + strings.TrimSpace(member.SupportRef)
}

func sourceInventoryObservationAttributeKey(attr SourceInventoryObservationAttribute) string {
	key := strings.TrimSpace(attr.Key)
	if key == "" {
		key = strings.TrimSpace(attr.Name)
	}
	if key == "" {
		return ""
	}
	return string(attr.Role) + "\x00" + key + "\x00" + strings.TrimSpace(attr.File) + "\x00" + strings.TrimSpace(attr.SupportRef)
}
