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
	axis types.PredicateAxis,
	runtimeArtifactCarrier bool,
	profile *types.CallChainEndpointProfile,
	exactTargets []string,
) string {
	if runtimeArtifactCarrier || types.NormalizeRequirementKind(kind) != types.ReqCallChain ||
		axis != types.AxisCall || profile == nil {
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
