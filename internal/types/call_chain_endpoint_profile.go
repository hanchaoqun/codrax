package types

import "strings"

// CallChainEndpointProfile is the analyzer-authored ordered endpoint carrier
// for source-code call-chain requests. Identity lists such as ExactTargets and
// MentionedEntities remain unordered sets; only this profile may drive a
// source->sink hard gate.
type CallChainEndpointProfile struct {
	Source string `json:"source"`
	Sink   string `json:"sink"`
}

func (p *CallChainEndpointProfile) Active() bool {
	if p == nil {
		return false
	}
	source := strings.TrimSpace(p.Source)
	sink := strings.TrimSpace(p.Sink)
	return source != "" && sink != "" && !strings.EqualFold(source, sink) &&
		IsCodeIdentitySurface(source) && IsCodeIdentitySurface(sink) &&
		!callChainEndpointHintLooksLikePath(source) && !callChainEndpointHintLooksLikePath(sink)
}

// NormalizeCallChainEndpointProfile validates the ordered carrier only against
// analyzer fields whose request provenance was already checked. It never reads
// RawRequest or infers direction from candidate order.
func NormalizeCallChainEndpointProfile(in *CallChainEndpointProfile, requestMentioned []string) (*CallChainEndpointProfile, string) {
	if in == nil || (strings.TrimSpace(in.Source) == "" && strings.TrimSpace(in.Sink) == "") {
		return nil, ""
	}
	profile := &CallChainEndpointProfile{
		Source: strings.TrimSpace(in.Source),
		Sink:   strings.TrimSpace(in.Sink),
	}
	if !profile.Active() {
		return nil, "call_chain_endpoints requires two distinct source-code symbol identities"
	}
	allowed := make(map[string]bool, len(requestMentioned))
	for _, raw := range requestMentioned {
		if value := strings.TrimSpace(raw); value != "" {
			allowed[strings.ToLower(value)] = true
		}
	}
	if !allowed[strings.ToLower(profile.Source)] || !allowed[strings.ToLower(profile.Sink)] {
		return nil, "call_chain_endpoints source and sink must both come from request-mentioned typed entities"
	}
	return profile, ""
}

// CallChainOrderedEndpointHints returns the only endpoint carrier whose order
// is authoritative. Absence is intentional: consumers must stand down rather
// than falling back to entity mention order.
func CallChainOrderedEndpointHints(rm RequestModel) (string, string, bool) {
	if !rm.CallChainEndpointProfile.Active() {
		return "", "", false
	}
	return strings.TrimSpace(rm.CallChainEndpointProfile.Source), strings.TrimSpace(rm.CallChainEndpointProfile.Sink), true
}
