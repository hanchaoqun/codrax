package types

import "strings"

// RuntimeTargetDeclaration records whether the current runtime-artifact
// request names a concrete process/thread identity. It is deliberately
// separate from RuntimeArtifactScopeProfile: artifact time/range and subject
// identity are orthogonal authorities.
type RuntimeTargetDeclaration string

const (
	RuntimeTargetDeclarationNotApplicable RuntimeTargetDeclaration = "not_applicable"
	RuntimeTargetDeclarationNoNamedTarget RuntimeTargetDeclaration = "no_named_target"
	RuntimeTargetDeclarationNamedTarget   RuntimeTargetDeclaration = "named_target"
	RuntimeTargetDeclarationUnspecified   RuntimeTargetDeclaration = "unspecified"
)

func AllRuntimeTargetDeclarations() []RuntimeTargetDeclaration {
	return []RuntimeTargetDeclaration{
		RuntimeTargetDeclarationNotApplicable,
		RuntimeTargetDeclarationNoNamedTarget,
		RuntimeTargetDeclarationNamedTarget,
		RuntimeTargetDeclarationUnspecified,
	}
}

func (d RuntimeTargetDeclaration) IsValid() bool {
	for _, candidate := range AllRuntimeTargetDeclarations() {
		if d == candidate {
			return true
		}
	}
	return false
}

// RuntimeTargetProfile is the analyzer's required typed declaration for the
// current request's runtime identity focus. NamedTarget is authoritative only
// after emit_analysis has retained an exact current-request quote and at least
// one structurally valid RuntimeTarget.
type RuntimeTargetProfile struct {
	Declaration RuntimeTargetDeclaration `json:"declaration"`
	SourceQuote string                   `json:"source_quote,omitempty"`
	Confidence  float64                  `json:"confidence,omitempty"`
	Rationale   string                   `json:"rationale,omitempty"`
}

func (p *RuntimeTargetProfile) NamedTarget() bool {
	return p != nil &&
		p.Declaration == RuntimeTargetDeclarationNamedTarget &&
		strings.TrimSpace(p.SourceQuote) != ""
}

func (p *RuntimeTargetProfile) ExplicitlyHasNoNamedTarget() bool {
	return p != nil && p.Declaration == RuntimeTargetDeclarationNoNamedTarget
}
