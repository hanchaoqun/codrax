package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// normalizeCallChainEndpointWireShape repairs endpoint-mode mismatches whose
// intended schema shape is fully determined by the structured fields: both
// discover, discover_terminal, and discover_path exclude a preselected ordered
// pair, while two named endpoint fields are an exact pair. It does not decide whether either
// identity came from the current request; NormalizeCallChainEndpointProfile
// remains the provenance authority and will demote unproven candidates after
// this local enum repair.
func normalizeCallChainEndpointWireShape(profile *types.CallChainEndpointProfile) (*types.CallChainEndpointProfile, string) {
	if profile == nil ||
		(profile.SinkMode != types.CallChainSinkResolutionDiscover &&
			profile.SinkMode != types.CallChainSinkResolutionDiscoverTerminal &&
			profile.SinkMode != types.CallChainSinkResolutionDiscoverPath) ||
		strings.TrimSpace(profile.Source) == "" || strings.TrimSpace(profile.Sink) == "" {
		return profile, ""
	}
	normalized := *profile
	previousMode := normalized.SinkMode
	normalized.SinkMode = types.CallChainSinkResolutionExact
	return &normalized, "normalized call_chain_endpoints sink_mode from " + string(previousMode) + " to exact because both ordered endpoint fields were present; current-request provenance checks still apply"
}

// normalizeCallChainEndpointFromRequiredDiagramParticipants repairs the
// remaining schema-to-schema contradiction where a required call visual names
// exactly two current-request code participants, while the ordered call-chain
// profile keeps the first participant as source but discards the other one by
// selecting discover/discover_terminal mode. parseDiagramHint has already
// verified that every retained participant and the shared relation scope are
// verbatim current-request carriers. The profile supplies direction; the
// incident participant set supplies only the unique other endpoint.
//
// Some analyzer responses correctly preserve the required relation-scope quote
// but emit an empty participant roster. In that shape, and only that shape, the
// same repair may recover the two identities from typed entities whose complete
// code-identity surfaces occur inside the already validated relation-scope
// quote. Entity order still supplies no direction, and path/context entities
// outside the quote cannot enter the pair.
//
// This is intentionally narrow: optional diagrams, context-only participants,
// non-call visual families, more than two incident participants, ambiguous
// aliases, and untyped identities all fail open. Grounded exploration still
// owns whether a path exists and may return a typed no_directed_path boundary.
func normalizeCallChainEndpointFromRequiredDiagramParticipants(
	kind string,
	axis types.PredicateAxis,
	profile *types.CallChainEndpointProfile,
	hint *types.DiagramHint,
	entities []string,
) (*types.CallChainEndpointProfile, string) {
	if types.NormalizeRequirementKind(kind) != types.ReqCallChain || axis != types.AxisCall ||
		profile == nil || hint == nil || !hint.Required ||
		(hint.Kind != types.DiagramSequence && hint.Kind != types.DiagramCallDAG) ||
		(profile.SinkMode != types.CallChainSinkResolutionDiscover &&
			profile.SinkMode != types.CallChainSinkResolutionDiscoverTerminal) ||
		strings.TrimSpace(profile.Source) == "" || strings.TrimSpace(profile.Sink) != "" {
		return profile, ""
	}

	incident := make([]string, 0, 2)
	if len(hint.Participants) > 0 {
		for _, participant := range hint.Participants {
			if participant.Role != types.DiagramParticipantIncidentRequired {
				return profile, ""
			}
			identity := strings.TrimSpace(participant.Identity)
			if identity == "" || !types.IsCodeIdentitySurface(identity) {
				return profile, ""
			}
			incident = append(incident, identity)
		}
	} else {
		endpoints := types.CallChainRequestedEndpointHints(types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{MentionedEntities: entities},
		})
		for _, endpoint := range endpoints {
			endpoint = strings.TrimSpace(endpoint)
			if !types.IsCodeIdentitySurface(endpoint) ||
				!types.RawRequestExplicitlyMentionsEntity(hint.RelationScopeQuote, endpoint) {
				continue
			}
			incident = append(incident, endpoint)
		}
	}
	if len(incident) != 2 {
		return profile, ""
	}

	source := strings.TrimSpace(profile.Source)
	sourceIndex := -1
	for i, identity := range incident {
		if types.CallChainEndpointCompatible(identity, source) ||
			types.CallChainEndpointCompatible(source, identity) {
			if sourceIndex >= 0 {
				return profile, ""
			}
			sourceIndex = i
		}
	}
	if sourceIndex < 0 {
		return profile, ""
	}
	destination := incident[1-sourceIndex]
	if types.CallChainEndpointCompatible(destination, source) ||
		types.CallChainEndpointCompatible(source, destination) {
		return profile, ""
	}

	normalized := *profile
	previousMode := normalized.SinkMode
	normalized.Sink = destination
	normalized.SinkMode = types.CallChainSinkResolutionExact
	repairSource := "the required typed call diagram names one unique other incident participant"
	if len(hint.Participants) == 0 {
		repairSource = "the required typed call diagram relation scope contains one unique other typed code identity"
	}
	return &normalized, "normalized call_chain_endpoints sink_mode from " + string(previousMode) +
		" to exact because " + repairSource + "; current-request provenance and grounded reachability checks still apply"
}

// validateCallChainEndpointWireShape rejects a schema-level contradiction
// before normalization can silently erase directional authority. Discover
// mode carries only the named source; an already named sink belongs to exact
// mode. This reads typed analyzer fields only, never request/final prose.
func validateCallChainEndpointWireShape(
	kind string,
	_ types.PredicateAxis,
	runtimeArtifactCarrier bool,
	profile *types.CallChainEndpointProfile,
	exactTargets []string,
) string {
	if runtimeArtifactCarrier || types.NormalizeRequirementKind(kind) != types.ReqCallChain ||
		profile == nil {
		return ""
	}
	if profile.SinkMode == types.CallChainSinkResolutionDiscover && strings.TrimSpace(profile.Sink) != "" {
		return "call_chain_endpoints sink_mode=discover requires sink=\"\"; when the current request names that destination, keep the sink and use sink_mode=exact"
	}
	if profile.SinkMode == types.CallChainSinkResolutionDiscoverTerminal && strings.TrimSpace(profile.Sink) != "" {
		return "call_chain_endpoints sink_mode=discover_terminal requires sink=\"\"; when the current request names that destination, keep the sink and use sink_mode=exact"
	}
	if profile.SinkMode == types.CallChainSinkResolutionDiscoverPath &&
		(strings.TrimSpace(profile.Source) != "" || strings.TrimSpace(profile.Sink) != "") {
		return "call_chain_endpoints sink_mode=discover_path requires source=\"\" and sink=\"\"; role-bound path endpoints must be selected from grounded exploration, not carried as analyzer candidates"
	}
	if destination, ok := callChainUniqueExactDestination(profile, exactTargets); ok {
		return "call_chain_endpoints sink_mode=" + string(profile.SinkMode) + " contradicts typed exact_targets: after removing source \"" +
			strings.TrimSpace(profile.Source) + "\", the unique named destination is \"" + destination +
			"\"; use sink_mode=exact and set sink to that destination so directed reachability or a typed no_directed_path boundary is investigated"
	}
	return ""
}

// validateCallChainRuntimeSelectionDeclaration binds the independent runtime
// selection obligation to one exact current-request quote. It never classifies
// prose itself: the analyzer emits the boolean and this function only checks
// its provenance and mutually exclusive empty form.
func validateCallChainRuntimeSelectionDeclaration(
	raw string,
	_ string,
	_ types.PredicateAxis,
	runtimeArtifactCarrier bool,
	profile *types.CallChainEndpointProfile,
) string {
	if runtimeArtifactCarrier || profile == nil {
		return ""
	}
	quote := strings.TrimSpace(profile.RuntimeSelectionSourceQuote)
	if !profile.RuntimeSelectionRequired {
		if quote != "" {
			return "call_chain_endpoints runtime_selection_required=false requires runtime_selection_source_quote=\"\""
		}
		return ""
	}
	if quote == "" || !sourceQuotePresentInCurrentRequest(raw, quote) {
		return "call_chain_endpoints runtime_selection_required=true requires a contiguous verbatim CURRENT-request runtime_selection_source_quote; do not infer runtime selection from prescan, repository, or prior-conversation text"
	}
	return ""
}

// reconcileSourceCallChainAxis closes a typed self-consistency gap before the
// endpoint profile is admitted. For a non-scalar source-code call_chain, the
// relationship axis is logically fixed to call; leaving an omitted axis as
// unknown used to make emit_analysis silently discard an otherwise valid
// ordered endpoint profile and disabled every downstream direction gate.
//
// This is a schema-to-schema implication only. It does not read request text,
// model reasoning, or answer prose. An omitted axis is repaired to call, while
// an explicitly different axis is rejected as a contradiction rather than
// overwritten. Runtime-artifact chains and scalar role lookups retain their
// independent routing contracts.
func reconcileSourceCallChainAxis(
	kind string,
	axis types.PredicateAxis,
	runtimeArtifactCarrier bool,
	predicates types.SemanticPredicates,
) (types.PredicateAxis, string, string) {
	if runtimeArtifactCarrier || types.NormalizeRequirementKind(kind) != types.ReqCallChain ||
		predicates.IsScalarAnswer || predicates.IsRoleLocateLookup {
		return axis, "", ""
	}
	switch axis {
	case types.AxisUnknown:
		return types.AxisCall,
			"normalized missing predicate_axis to call because source-code question_kind=call_chain fixes the relationship axis",
			""
	case types.AxisCall:
		return axis, "", ""
	default:
		return axis, "", "source-code question_kind=call_chain contradicts predicate_axis=" + string(axis) +
			"; use predicate_axis=call, or change question_kind when the request is not a directional source-code call chain"
	}
}

// callChainUniqueExactDestination interprets no list order. It only reports a
// destination when the schema-validated exact target set contains the typed
// source plus exactly one other logical code identity. Multiple other targets,
// a missing source, paths, or qualified identity ambiguity all fail open.
func callChainUniqueExactDestination(profile *types.CallChainEndpointProfile, exactTargets []string) (string, bool) {
	if profile == nil ||
		(profile.SinkMode != types.CallChainSinkResolutionDiscover &&
			profile.SinkMode != types.CallChainSinkResolutionDiscoverTerminal) ||
		strings.TrimSpace(profile.Sink) != "" {
		return "", false
	}
	source := strings.TrimSpace(profile.Source)
	if source == "" {
		return "", false
	}
	endpoints := types.CallChainRequestedEndpointHints(types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{MentionedEntities: exactTargets},
	})
	sourcePresent := false
	var destinations []string
	for _, endpoint := range endpoints {
		if types.CallChainEndpointCompatible(endpoint, source) || types.CallChainEndpointCompatible(source, endpoint) {
			sourcePresent = true
			continue
		}
		merged := false
		for _, existing := range destinations {
			if types.CallChainEndpointCompatible(endpoint, existing) && types.CallChainEndpointCompatible(existing, endpoint) {
				merged = true
				break
			}
		}
		if !merged {
			destinations = append(destinations, endpoint)
		}
	}
	if !sourcePresent || len(destinations) != 1 {
		return "", false
	}
	return destinations[0], true
}
