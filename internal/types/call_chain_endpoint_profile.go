package types

import "strings"

type CallChainSinkResolutionMode string

// CallChainEndpointProfileTeaching is the single semantic teaching source for
// the analyzer skill and emit_analysis schema. Field types and enum admission
// remain schema-owned; this sentence only distinguishes the three mutually
// exclusive endpoint-authority shapes.
const CallChainEndpointProfileTeaching = "For a source-code call chain, use call_chain_endpoints.sink_mode=exact only when the current request names both code identities; use discover with an exact current-request source and an empty sink only when the requested answer is which runtime implementation/class/handler is reached; use discover_path with empty source and sink when the request names only conceptual path boundaries and grounded exploration must identify both code endpoints. A current-checkout typed pre-scan candidate only selects an investigation target and NEVER proves an endpoint or path; it never becomes endpoint authority."

const (
	CallChainSinkResolutionExact        CallChainSinkResolutionMode = "exact"
	CallChainSinkResolutionDiscover     CallChainSinkResolutionMode = "discover"
	CallChainSinkResolutionDiscoverPath CallChainSinkResolutionMode = "discover_path"
)

func CallChainSinkResolutionModeValues() []string {
	return []string{
		string(CallChainSinkResolutionExact),
		string(CallChainSinkResolutionDiscover),
		string(CallChainSinkResolutionDiscoverPath),
	}
}

// CallChainEndpointProfile is the analyzer-authored ordered endpoint carrier
// for source-code call-chain requests. Identity lists such as ExactTargets and
// MentionedEntities remain unordered sets; only this profile may drive a
// source->sink hard gate. DiscoverPath carries no endpoint hard authority.
type CallChainEndpointProfile struct {
	Source   string                      `json:"source"`
	Sink     string                      `json:"sink"`
	SinkMode CallChainSinkResolutionMode `json:"sink_mode,omitempty"`
}

func (p *CallChainEndpointProfile) Active() bool {
	return p.ExactActive() || p.DiscoverSinkActive() || p.DiscoverPathActive()
}

func (p *CallChainEndpointProfile) ExactActive() bool {
	if p == nil {
		return false
	}
	source := strings.TrimSpace(p.Source)
	sink := strings.TrimSpace(p.Sink)
	mode := p.SinkMode
	if mode == "" {
		mode = CallChainSinkResolutionExact
	}
	return mode == CallChainSinkResolutionExact && source != "" && sink != "" && !strings.EqualFold(source, sink) &&
		IsCodeIdentitySurface(source) && IsCodeIdentitySurface(sink) &&
		!callChainEndpointHintLooksLikePath(source) && !callChainEndpointHintLooksLikePath(sink)
}

func (p *CallChainEndpointProfile) DiscoverSinkActive() bool {
	if p == nil || p.SinkMode != CallChainSinkResolutionDiscover || strings.TrimSpace(p.Sink) != "" {
		return false
	}
	source := strings.TrimSpace(p.Source)
	return source != "" && IsCodeIdentitySurface(source) && !callChainEndpointHintLooksLikePath(source)
}

// DiscoverPathActive is the role-bounded call-chain lane. It applies when the
// current request asks for a directional source-code path but names only
// conceptual boundaries rather than exact code identities. Grounded
// exploration owns both endpoint identities; this lane does not assert that
// the unknown destination is a runtime-selected implementation.
func (p *CallChainEndpointProfile) DiscoverPathActive() bool {
	return p != nil && p.SinkMode == CallChainSinkResolutionDiscoverPath &&
		strings.TrimSpace(p.Source) == "" && strings.TrimSpace(p.Sink) == ""
}

// NormalizeCallChainEndpointProfile validates the ordered carrier's structural
// shape and separates a user-named exact destination from a destination that
// classification merely preselected. Exact source-to-sink reachability is a
// hard contract, so both endpoint identities must have current-request
// provenance. When the source is named but the sink is not, preserve the
// useful source and deterministically demote to discover mode. When neither
// analyzer candidate has current-request provenance, discard both into
// discover_path; grounded evidence then owns the path endpoint identities.
func NormalizeCallChainEndpointProfile(in *CallChainEndpointProfile, requestMentioned []string) (*CallChainEndpointProfile, string) {
	if in == nil {
		return nil, ""
	}
	profile := &CallChainEndpointProfile{
		Source:   strings.TrimSpace(in.Source),
		Sink:     strings.TrimSpace(in.Sink),
		SinkMode: in.SinkMode,
	}
	if profile.SinkMode == "" {
		profile.SinkMode = CallChainSinkResolutionExact
	}
	if profile.Source == "" && profile.Sink == "" && profile.SinkMode != CallChainSinkResolutionDiscoverPath {
		return nil, ""
	}
	if profile.SinkMode == CallChainSinkResolutionDiscoverPath {
		if profile.Source != "" || profile.Sink != "" {
			return nil, "call_chain_endpoints discover_path mode requires source=\"\" and sink=\"\" so analyzer candidates cannot become endpoint authority"
		}
		return profile, ""
	}
	allowed := make(map[string]bool, len(requestMentioned))
	for _, raw := range requestMentioned {
		if value := strings.TrimSpace(raw); value != "" {
			allowed[strings.ToLower(value)] = true
		}
	}
	if profile.SinkMode == CallChainSinkResolutionDiscover && profile.Sink != "" {
		profile.Sink = ""
		if !profile.DiscoverSinkActive() {
			return nil, "call_chain_endpoints discover mode requires one valid source-code source identity"
		}
		if !allowed[strings.ToLower(profile.Source)] {
			return &CallChainEndpointProfile{SinkMode: CallChainSinkResolutionDiscoverPath},
				"call_chain_endpoints discarded analyzer-resolved source and sink candidates; discover_path leaves both endpoint identities to grounded exploration"
		}
		return profile, "call_chain_endpoints discover mode discarded a preselected sink; grounded exploration must identify the runtime destination"
	}
	if profile.SinkMode == CallChainSinkResolutionExact {
		sourceMentioned := allowed[strings.ToLower(profile.Source)]
		sinkMentioned := allowed[strings.ToLower(profile.Sink)]
		sourceCodeIdentity := profile.Source != "" && IsCodeIdentitySurface(profile.Source) && !callChainEndpointHintLooksLikePath(profile.Source)
		sinkCodeIdentity := profile.Sink != "" && IsCodeIdentitySurface(profile.Sink) && !callChainEndpointHintLooksLikePath(profile.Sink)
		if sourceCodeIdentity && sourceMentioned && (!sinkCodeIdentity || !sinkMentioned) {
			candidate := profile.Sink
			profile.Sink = ""
			profile.SinkMode = CallChainSinkResolutionDiscover
			return profile, "call_chain_endpoints exact sink " + candidate + " was not an exact current-request code identity; demoted to discover mode so grounded exploration selects the runtime destination"
		}
		if (!sourceCodeIdentity || !sinkCodeIdentity || !sourceMentioned || !sinkMentioned) &&
			!strings.EqualFold(profile.Source, profile.Sink) {
			return &CallChainEndpointProfile{SinkMode: CallChainSinkResolutionDiscoverPath},
				"call_chain_endpoints exact candidates were not both current-request code identities; normalized to discover_path so grounded exploration owns both endpoint identities"
		}
	}
	if !profile.Active() {
		return nil, "call_chain_endpoints requires exact mode with two distinct identities, discover mode with one exact source and an empty sink, or discover_path with both endpoint fields empty"
	}
	if profile.DiscoverSinkActive() {
		if !allowed[strings.ToLower(profile.Source)] {
			return &CallChainEndpointProfile{SinkMode: CallChainSinkResolutionDiscoverPath},
				"call_chain_endpoints discover source was not a current-request identity; normalized to discover_path so the analyzer guess cannot become a hard answer anchor"
		}
		return profile, ""
	}
	return profile, ""
}

// CallChainOrderedEndpointHints returns the only endpoint carrier whose order
// is authoritative. Absence is intentional: consumers must stand down rather
// than falling back to entity mention order.
func CallChainOrderedEndpointHints(rm RequestModel) (string, string, bool) {
	if !rm.CallChainEndpointProfile.ExactActive() {
		return "", "", false
	}
	return strings.TrimSpace(rm.CallChainEndpointProfile.Source), strings.TrimSpace(rm.CallChainEndpointProfile.Sink), true
}
