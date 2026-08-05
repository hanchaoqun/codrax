package types

import "strings"

type CallChainSinkResolutionMode string

const (
	CallChainSinkResolutionExact    CallChainSinkResolutionMode = "exact"
	CallChainSinkResolutionDiscover CallChainSinkResolutionMode = "discover"
)

func CallChainSinkResolutionModeValues() []string {
	return []string{string(CallChainSinkResolutionExact), string(CallChainSinkResolutionDiscover)}
}

// CallChainEndpointProfile is the analyzer-authored ordered endpoint carrier
// for source-code call-chain requests. Identity lists such as ExactTargets and
// MentionedEntities remain unordered sets; only this profile may drive a
// source->sink hard gate.
type CallChainEndpointProfile struct {
	Source   string                      `json:"source"`
	Sink     string                      `json:"sink"`
	SinkMode CallChainSinkResolutionMode `json:"sink_mode,omitempty"`
}

func (p *CallChainEndpointProfile) Active() bool {
	return p.ExactActive() || p.DiscoverSinkActive()
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

// NormalizeCallChainEndpointProfile validates the ordered carrier's structural
// shape and separates a user-named exact destination from a destination that
// classification merely preselected. Exact source-to-sink reachability is a
// hard contract, so both endpoint identities must have current-request
// provenance. When the source is named but the sink is not, preserve the
// useful source and deterministically demote to discover mode; investigation
// evidence, not the analyzer candidate, must select the runtime destination.
func NormalizeCallChainEndpointProfile(in *CallChainEndpointProfile, requestMentioned []string) (*CallChainEndpointProfile, string) {
	if in == nil || (strings.TrimSpace(in.Source) == "" && strings.TrimSpace(in.Sink) == "") {
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
	if profile.SinkMode == CallChainSinkResolutionDiscover && profile.Sink != "" {
		profile.Sink = ""
		if !profile.DiscoverSinkActive() {
			return nil, "call_chain_endpoints discover mode requires one valid source-code source identity"
		}
		return profile, "call_chain_endpoints discover mode discarded a preselected sink; grounded exploration must identify the runtime destination"
	}
	if !profile.Active() {
		return nil, "call_chain_endpoints requires either exact mode with two distinct source-code identities or discover mode with one source and an empty sink"
	}
	if profile.DiscoverSinkActive() {
		return profile, ""
	}
	allowed := make(map[string]bool, len(requestMentioned))
	for _, raw := range requestMentioned {
		if value := strings.TrimSpace(raw); value != "" {
			allowed[strings.ToLower(value)] = true
		}
	}
	sourceMentioned := allowed[strings.ToLower(profile.Source)]
	sinkMentioned := allowed[strings.ToLower(profile.Sink)]
	if sourceMentioned && !sinkMentioned {
		candidate := profile.Sink
		profile.Sink = ""
		profile.SinkMode = CallChainSinkResolutionDiscover
		return profile, "call_chain_endpoints exact sink " + candidate + " was not a current-request identity; demoted to discover mode so grounded exploration selects the destination"
	}
	if !sourceMentioned || !sinkMentioned {
		return nil, "call_chain_endpoints exact mode requires both source and sink to be current-request identities; analyzer-resolved candidates cannot become directional hard-gate authority"
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
