package types

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

type ConversationReferenceSource string

const (
	ConversationReferenceSourceCurrentRequest ConversationReferenceSource = "current_request"
	ConversationReferenceSourcePriorContext   ConversationReferenceSource = "prior_context"
	ConversationReferenceSourceMixed          ConversationReferenceSource = "mixed"
)

func AllConversationReferenceSources() []ConversationReferenceSource {
	return []ConversationReferenceSource{
		ConversationReferenceSourceCurrentRequest,
		ConversationReferenceSourcePriorContext,
		ConversationReferenceSourceMixed,
	}
}

func (s ConversationReferenceSource) IsValid() bool {
	for _, declared := range AllConversationReferenceSources() {
		if s == declared {
			return true
		}
	}
	return false
}

type ConversationReferenceAmbiguity string

const (
	ConversationReferenceAmbiguityNone      ConversationReferenceAmbiguity = "none"
	ConversationReferenceAmbiguityAmbiguous ConversationReferenceAmbiguity = "ambiguous"
	ConversationReferenceAmbiguityMissing   ConversationReferenceAmbiguity = "missing"
)

func AllConversationReferenceAmbiguities() []ConversationReferenceAmbiguity {
	return []ConversationReferenceAmbiguity{
		ConversationReferenceAmbiguityNone,
		ConversationReferenceAmbiguityAmbiguous,
		ConversationReferenceAmbiguityMissing,
	}
}

func (a ConversationReferenceAmbiguity) IsValid() bool {
	for _, declared := range AllConversationReferenceAmbiguities() {
		if a == declared {
			return true
		}
	}
	return false
}

// ConversationReferenceProfile carries analyzer-resolved subjects that
// come from prior conversation rather than from the current request text.
// It is intentionally generic: log/trace observations use
// ArtifactObservationProfile, while ordinary follow-up questions use this
// profile to keep provenance explicit.
type ConversationReferenceProfile struct {
	RequiresPriorContext  bool                           `json:"requires_prior_context"`
	NeedsRepoVerification bool                           `json:"needs_repo_verification"`
	Ambiguity             ConversationReferenceAmbiguity `json:"ambiguity,omitempty"`
	ResolvedSubjects      []ResolvedConversationSubject  `json:"resolved_subjects,omitempty"`
}

type ResolvedConversationSubject struct {
	Surface          string                      `json:"surface"`
	Kind             AnswerSubjectKind           `json:"kind,omitempty"`
	Source           ConversationReferenceSource `json:"source"`
	Role             string                      `json:"role,omitempty"`
	UseAsExactTarget bool                        `json:"use_as_exact_target,omitempty"`
	Confidence       float64                     `json:"confidence,omitempty"`
}

func (p *ConversationReferenceProfile) SubjectCandidates() []string {
	if p == nil || !p.RequiresPriorContext {
		if p != nil && len(p.ResolvedSubjects) > 0 {
			logging.Debug("[conversation_reference] dropping %d resolved subject(s): requires_prior_context=false", len(p.ResolvedSubjects))
		}
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, subj := range p.ResolvedSubjects {
		surface := strings.TrimSpace(subj.Surface)
		key := strings.ToLower(surface)
		if surface == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, surface)
	}
	return out
}

func (p *ConversationReferenceProfile) ExactTargetSubjects() []ResolvedConversationSubject {
	if p == nil || !p.RequiresPriorContext {
		if p != nil && len(p.ResolvedSubjects) > 0 {
			logging.Debug("[conversation_reference] dropping exact-target projection of %d resolved subject(s): requires_prior_context=false", len(p.ResolvedSubjects))
		}
		return nil
	}
	if p.Ambiguity != "" && p.Ambiguity != ConversationReferenceAmbiguityNone {
		logging.Debug("[conversation_reference] dropping exact-target projection of %d resolved subject(s): ambiguity=%q", len(p.ResolvedSubjects), p.Ambiguity)
		return nil
	}
	seen := map[string]bool{}
	var out []ResolvedConversationSubject
	for _, subj := range p.ResolvedSubjects {
		surface := strings.TrimSpace(subj.Surface)
		if surface == "" || !subj.UseAsExactTarget {
			continue
		}
		switch subj.Source {
		case ConversationReferenceSourcePriorContext, ConversationReferenceSourceMixed:
		default:
			continue
		}
		key := strings.ToLower(surface)
		if seen[key] {
			continue
		}
		seen[key] = true
		subj.Surface = surface
		out = append(out, subj)
	}
	return out
}
