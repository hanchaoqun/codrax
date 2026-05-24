package types

import "strings"

// SourceInventoryAdvisory is a structured, language-neutral candidate carrier
// for scoped source inventories. It is not an answer and not a citation by
// itself; downstream agents may use it to avoid repeating broad repository
// enumeration, while validators continue to require normal grounded evidence
// or accepted aggregate facts for user-visible claims.
type SourceInventoryAdvisory struct {
	Active       bool                         `json:"active"`
	AdvisoryOnly bool                         `json:"advisory_only,omitempty"`
	Complete     bool                         `json:"complete,omitempty"`
	Scopes       []string                     `json:"scopes,omitempty"`
	Sets         []SourceInventoryAdvisorySet `json:"sets,omitempty"`
	Provenance   []string                     `json:"provenance,omitempty"`
}

func (a SourceInventoryAdvisory) IsActive() bool {
	return a.Active && len(a.Sets) > 0
}

// SourceInventoryAdvisorySet groups candidates for a single requested role.
// Complete means the underlying graph/index had source files for the requested
// scopes; it is a boundedness signal, not a guarantee that the user wanted this
// set rendered verbatim.
type SourceInventoryAdvisorySet struct {
	Role       AnswerCandidateRole                `json:"role,omitempty"`
	Complete   bool                               `json:"complete,omitempty"`
	Candidates []SourceInventoryAdvisoryCandidate `json:"candidates,omitempty"`
}

// SourceInventoryAdvisoryCandidate is one graph-backed candidate row.
type SourceInventoryAdvisoryCandidate struct {
	Member     string              `json:"member,omitempty"`
	Key        string              `json:"key,omitempty"`
	SupportRef string              `json:"support_ref,omitempty"`
	Note       string              `json:"note,omitempty"`
	Role       AnswerCandidateRole `json:"role,omitempty"`
	Exported   bool                `json:"exported,omitempty"`
	File       string              `json:"file,omitempty"`
	Line       int                 `json:"line,omitempty"`
	Language   string              `json:"language,omitempty"`
}

func CloneSourceInventoryAdvisory(in SourceInventoryAdvisory) SourceInventoryAdvisory {
	out := in
	out.Scopes = append([]string(nil), in.Scopes...)
	out.Provenance = append([]string(nil), in.Provenance...)
	if in.Sets != nil {
		out.Sets = make([]SourceInventoryAdvisorySet, len(in.Sets))
		for i, set := range in.Sets {
			out.Sets[i] = set
			out.Sets[i].Candidates = append([]SourceInventoryAdvisoryCandidate(nil), set.Candidates...)
		}
	}
	return out
}

func MergeSourceInventoryAdvisory(prior, current SourceInventoryAdvisory) SourceInventoryAdvisory {
	if !prior.IsActive() {
		return CloneSourceInventoryAdvisory(current)
	}
	if !current.IsActive() {
		return CloneSourceInventoryAdvisory(prior)
	}
	merged := CloneSourceInventoryAdvisory(prior)
	merged.Active = true
	merged.AdvisoryOnly = prior.AdvisoryOnly && current.AdvisoryOnly
	merged.Complete = prior.Complete && current.Complete
	merged.Scopes = mergeSourceInventoryAdvisoryStrings(merged.Scopes, current.Scopes)
	merged.Provenance = mergeSourceInventoryAdvisoryStrings(merged.Provenance, current.Provenance)
	byRole := make(map[AnswerCandidateRole]int, len(merged.Sets))
	for i := range merged.Sets {
		byRole[merged.Sets[i].Role] = i
	}
	for _, set := range current.Sets {
		if idx, ok := byRole[set.Role]; ok {
			merged.Sets[idx].Complete = merged.Sets[idx].Complete && set.Complete
			merged.Sets[idx].Candidates = mergeSourceInventoryAdvisoryCandidates(merged.Sets[idx].Candidates, set.Candidates)
			continue
		}
		byRole[set.Role] = len(merged.Sets)
		merged.Sets = append(merged.Sets, CloneSourceInventoryAdvisory(SourceInventoryAdvisory{Active: true, Sets: []SourceInventoryAdvisorySet{set}}).Sets[0])
	}
	return merged
}

func mergeSourceInventoryAdvisoryStrings(existing, incoming []string) []string {
	if len(existing) == 0 {
		return append([]string(nil), incoming...)
	}
	out := append([]string(nil), existing...)
	seen := make(map[string]bool, len(out)+len(incoming))
	for _, item := range out {
		if key := strings.TrimSpace(item); key != "" {
			seen[key] = true
		}
	}
	for _, item := range incoming {
		key := strings.TrimSpace(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func mergeSourceInventoryAdvisoryCandidates(existing, incoming []SourceInventoryAdvisoryCandidate) []SourceInventoryAdvisoryCandidate {
	if len(existing) == 0 {
		return append([]SourceInventoryAdvisoryCandidate(nil), incoming...)
	}
	out := append([]SourceInventoryAdvisoryCandidate(nil), existing...)
	seen := make(map[string]bool, len(out)+len(incoming))
	for _, candidate := range out {
		if key := sourceInventoryAdvisoryCandidateKey(candidate); key != "" {
			seen[key] = true
		}
	}
	for _, candidate := range incoming {
		key := sourceInventoryAdvisoryCandidateKey(candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}

func sourceInventoryAdvisoryCandidateKey(candidate SourceInventoryAdvisoryCandidate) string {
	key := strings.TrimSpace(candidate.Key)
	if key == "" {
		key = strings.TrimSpace(candidate.Member)
	}
	if key == "" {
		return ""
	}
	return string(candidate.Role) + "\x00" + key + "\x00" + strings.TrimSpace(candidate.File) + "\x00" + strings.TrimSpace(candidate.SupportRef)
}
