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
// shape. Request mention is retained only as a soft provenance warning: one
// end of a directional request may be described by role/language/layer (for
// example "the Rust implementation") and resolved to a concrete symbol during
// typed pre-scan. Rejecting that concrete symbol here forces the analyzer to
// misclassify a real call chain as a generic mechanism. Final source/sink and
// path claims remain fail-closed on grounded evidence downstream.
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
	if !allowed[strings.ToLower(profile.Source)] || !allowed[strings.ToLower(profile.Sink)] {
		return profile, "call_chain_endpoints includes a concrete endpoint resolved beyond request-mentioned identities; preserve the ordered investigation target, but require grounded endpoint/path evidence before any answer claim"
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
