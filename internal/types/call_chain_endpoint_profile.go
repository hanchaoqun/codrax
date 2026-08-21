package types

import "strings"

type CallChainSinkResolutionMode string

// CallChainEndpointProfileTeaching is the single semantic teaching source for
// the analyzer skill and emit_analysis schema. Field types and enum admission
// remain schema-owned; this sentence only distinguishes the four mutually
// exclusive endpoint-authority shapes.
const CallChainEndpointLowMindRule = "LOW-MIND RULE: when the CURRENT request is not asking for a source-code call chain, emit source=\"\", sink=\"\", sink_mode=exact; never invent endpoints from repository pre-scan, search, or evidence merely because this object is required."

const CallChainEndpointProfileTeaching = "For a source-code call chain, use call_chain_endpoints.sink_mode=exact only when the current request names both code identities; use discover with an exact current-request code identity as source (never a file path or pre-scan candidate) and an empty sink only when the requested answer is which runtime implementation/class/handler is selected; use discover_terminal with an exact current-request code identity as source and an empty sink when grounded static call evidence must identify the terminal code endpoint for a conceptual destination; use discover_path with empty source and sink when the request names only conceptual path boundaries and grounded exploration must identify both code endpoints. A current-checkout typed pre-scan candidate only selects an investigation target and NEVER proves an endpoint or path; it never becomes endpoint authority. Runtime/tool choice under different conditions is declared separately in runtime_selection_profile, not in this endpoint object."

// RuntimeSelectionProfileTeaching keeps condition-dependent tool/provider
// choice independent from ordered call-chain endpoint identity.
const RuntimeSelectionProfileTeaching = "Emit runtime_selection_profile on every call. Set is_selection_question=true only when the CURRENT request asks which implementation, tool, backend, plugin, provider, handler, or path is used or available under different runtime conditions, including initial/full output versus retry/error/patch; copy the shortest contiguous verbatim phrase into source_quote. Otherwise emit false with source_quote empty. This profile is independent of call-chain endpoint direction and generic function/purpose dimensions."

const (
	CallChainSinkResolutionExact            CallChainSinkResolutionMode = "exact"
	CallChainSinkResolutionDiscover         CallChainSinkResolutionMode = "discover"
	CallChainSinkResolutionDiscoverTerminal CallChainSinkResolutionMode = "discover_terminal"
	CallChainSinkResolutionDiscoverPath     CallChainSinkResolutionMode = "discover_path"
)

func CallChainSinkResolutionModeValues() []string {
	return []string{
		string(CallChainSinkResolutionExact),
		string(CallChainSinkResolutionDiscover),
		string(CallChainSinkResolutionDiscoverTerminal),
		string(CallChainSinkResolutionDiscoverPath),
	}
}

// CallChainEndpointProfile is the analyzer-authored ordered endpoint carrier
// for source-code call-chain requests. Identity lists such as ExactTargets and
// MentionedEntities remain unordered sets; only this profile may drive a
// source->sink hard gate. DiscoverPath carries no endpoint hard authority.
type CallChainEndpointProfile struct {
	Source                      string                      `json:"source"`
	Sink                        string                      `json:"sink"`
	SinkMode                    CallChainSinkResolutionMode `json:"sink_mode,omitempty"`
	RuntimeSelectionRequired    bool                        `json:"runtime_selection_required"`
	RuntimeSelectionSourceQuote string                      `json:"runtime_selection_source_quote"`
}

// RequiresRuntimeSelectionEvidence is the single typed selector for the
// runtime-selection evidence lane. Discover mode already means that the
// concrete destination itself must be selected. Exact/discover_terminal/
// discover_path requests enter the same lane only through the analyzer-authored,
// verbatim-anchored boolean above. Consumers never infer this obligation from
// request, tool, or answer prose.
func (p *CallChainEndpointProfile) RequiresRuntimeSelectionEvidence() bool {
	return p != nil && (p.DiscoverSinkActive() || p.RuntimeSelectionRequired)
}

func (p *CallChainEndpointProfile) Active() bool {
	return p.ExactActive() || p.DiscoverSinkActive() || p.DiscoverTerminalActive() || p.DiscoverPathActive()
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

// DiscoverTerminalActive preserves a current-request exact source while
// leaving a conceptual terminal boundary to grounded static call evidence.
// Unlike discover, it does not imply that a runtime implementation was
// selected through registration, binding, construction, or dispatch.
func (p *CallChainEndpointProfile) DiscoverTerminalActive() bool {
	if p == nil || p.SinkMode != CallChainSinkResolutionDiscoverTerminal || strings.TrimSpace(p.Sink) != "" {
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
// useful source and deterministically demote to discover for an explicit
// runtime-selection request, otherwise to discover_terminal. When neither
// analyzer candidate has current-request provenance, discard both into
// discover_path; grounded evidence then owns the path endpoint identities.
func NormalizeCallChainEndpointProfile(in *CallChainEndpointProfile, requestMentioned []string) (*CallChainEndpointProfile, string) {
	if in == nil {
		return nil, ""
	}
	profile := &CallChainEndpointProfile{
		Source:                      strings.TrimSpace(in.Source),
		Sink:                        strings.TrimSpace(in.Sink),
		SinkMode:                    in.SinkMode,
		RuntimeSelectionRequired:    in.RuntimeSelectionRequired,
		RuntimeSelectionSourceQuote: strings.TrimSpace(in.RuntimeSelectionSourceQuote),
	}
	if profile.SinkMode == "" {
		profile.SinkMode = CallChainSinkResolutionExact
	}
	if profile.Source == "" && profile.Sink == "" && profile.SinkMode != CallChainSinkResolutionDiscoverPath {
		// Runtime selection is an answer obligation, not endpoint authority.  A
		// mechanism/flow request may need to explain an availability or dispatch
		// choice without asking for a source-to-sink call chain at all.  Preserve
		// that explicitly declared carrier while keeping the endpoint profile
		// inactive; downstream consumers must use
		// RequiresRuntimeSelectionEvidence rather than Active for this lane.
		if profile.RuntimeSelectionRequired {
			return profile, ""
		}
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
	if profile.SinkMode == CallChainSinkResolutionDiscover || profile.SinkMode == CallChainSinkResolutionDiscoverTerminal {
		discardedSink := profile.Sink != ""
		profile.Sink = ""
		// A discover source is still endpoint authority. If the analyzer copied a
		// file path or another non-code navigation candidate into this field,
		// fail-safe by removing that authority and retaining the requested
		// role-bound path investigation. This is the same demotion already used
		// for an unproven code-looking source; it reads only the typed carrier and
		// current-request provenance set.
		active := profile.DiscoverSinkActive() || profile.DiscoverTerminalActive()
		if !active || !allowed[strings.ToLower(profile.Source)] {
			return &CallChainEndpointProfile{
					SinkMode:                    CallChainSinkResolutionDiscoverPath,
					RuntimeSelectionRequired:    profile.RuntimeSelectionRequired,
					RuntimeSelectionSourceQuote: profile.RuntimeSelectionSourceQuote,
				},
				"call_chain_endpoints discover source was not an exact current-request code identity; normalized to discover_path so a file path or analyzer candidate cannot become endpoint authority"
		}
		if discardedSink {
			return profile, "call_chain_endpoints " + string(profile.SinkMode) + " mode discarded a preselected sink; grounded exploration must identify the destination"
		}
		return profile, ""
	}
	if profile.SinkMode == CallChainSinkResolutionExact {
		sourceMentioned := allowed[strings.ToLower(profile.Source)]
		sinkMentioned := allowed[strings.ToLower(profile.Sink)]
		sourceCodeIdentity := profile.Source != "" && IsCodeIdentitySurface(profile.Source) && !callChainEndpointHintLooksLikePath(profile.Source)
		sinkCodeIdentity := profile.Sink != "" && IsCodeIdentitySurface(profile.Sink) && !callChainEndpointHintLooksLikePath(profile.Sink)
		if sourceCodeIdentity && sourceMentioned && (!sinkCodeIdentity || !sinkMentioned) {
			candidate := profile.Sink
			profile.Sink = ""
			if profile.RuntimeSelectionRequired {
				profile.SinkMode = CallChainSinkResolutionDiscover
				return profile, "call_chain_endpoints exact sink " + candidate + " was not an exact current-request code identity; demoted to discover mode so grounded exploration proves the requested runtime selection"
			}
			profile.SinkMode = CallChainSinkResolutionDiscoverTerminal
			return profile, "call_chain_endpoints exact sink " + candidate + " was not an exact current-request code identity; demoted to discover_terminal so grounded static call evidence identifies the conceptual destination without inventing runtime selection"
		}
		if (!sourceCodeIdentity || !sinkCodeIdentity || !sourceMentioned || !sinkMentioned) &&
			!strings.EqualFold(profile.Source, profile.Sink) {
			return &CallChainEndpointProfile{
					SinkMode:                    CallChainSinkResolutionDiscoverPath,
					RuntimeSelectionRequired:    profile.RuntimeSelectionRequired,
					RuntimeSelectionSourceQuote: profile.RuntimeSelectionSourceQuote,
				},
				"call_chain_endpoints exact candidates were not both current-request code identities; normalized to discover_path so grounded exploration owns both endpoint identities"
		}
	}
	if !profile.Active() {
		return nil, "call_chain_endpoints requires exact mode with two distinct identities, discover/discover_terminal mode with one exact source and an empty sink, or discover_path with both endpoint fields empty"
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
