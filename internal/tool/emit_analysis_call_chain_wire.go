package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

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
	if destination, ok := callChainUniqueExactDestination(profile, exactTargets); ok {
		return "call_chain_endpoints sink_mode=discover contradicts typed exact_targets: after removing source \"" +
			strings.TrimSpace(profile.Source) + "\", the unique named destination is \"" + destination +
			"\"; use sink_mode=exact and set sink to that destination so directed reachability or a typed no_directed_path boundary is investigated"
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
	if profile == nil || profile.SinkMode != types.CallChainSinkResolutionDiscover || strings.TrimSpace(profile.Sink) != "" {
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
