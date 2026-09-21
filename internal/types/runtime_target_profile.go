package types

import (
	"strconv"
	"strings"
)

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

// RuntimeUserTargetAnchorEntities resolves only user-identity authority, not
// query/navigation targets or time windows. A present profile owns that
// authority: only its named declaration and valid user-explicit targets may
// elect a user anchor. Generic Entities/ExactTargets are not fallback identities.
// The bool distinguishes an explicit empty authorization from the legacy
// nil-profile lane, whose existing callers keep their original behavior.
// Quote anchoring remains emit_analysis's responsibility; this consumer neither
// scans request prose nor changes the request or its exploration targets.
func RuntimeUserTargetAnchorEntities(rm *RequestModel) ([]AnchorUserEntity, bool) {
	if rm == nil || rm.RuntimeTargetProfile == nil {
		return nil, false
	}
	if !rm.RuntimeTargetProfile.NamedTarget() {
		return nil, true
	}
	var out []AnchorUserEntity
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, AnchorUserEntity{Value: value, TypedLane: true})
		}
	}
	for _, target := range rm.RuntimeTargets {
		if strings.TrimSpace(target.Source) != "user_explicit" ||
			target.PID < 0 || target.PID > RuntimeTargetMaxPID {
			continue
		}
		kind := NormalizeRuntimeTargetKind(target.Kind)
		if kind == RuntimeTargetKindUnknown || !target.Active() ||
			(kind == RuntimeTargetKindProcess && target.PID == 0) {
			continue
		}
		if target.PID > 0 {
			add(strconv.Itoa(target.PID))
		}
		add(target.Thread)
	}
	return out, true
}

// RuntimeUserTargetAnchorMatchesEntity is the typed identity comparison shared
// by compile-time user-anchor election and present-profile display. The entity
// must come from RuntimeUserTargetAnchorEntities; this is not a raw-prose match.
func RuntimeUserTargetAnchorMatchesEntity(label, entity string) bool {
	return traceCausalProjectionAnchorLabelMatchesEntity(label, traceCausalProjectionAnchorEntity{value: entity, typedLane: true})
}
