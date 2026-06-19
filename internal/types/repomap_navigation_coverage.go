package types

import (
	"sort"
	"strings"
)

const RepoMapNavigationObservationPredicate = "repo_map_navigation_route"

type RepoMapNavigationCoverageState string

const (
	RepoMapNavigationCoverageNotRequired RepoMapNavigationCoverageState = "not_required"
	RepoMapNavigationCoverageCovered     RepoMapNavigationCoverageState = "covered"
	RepoMapNavigationCoveragePartial     RepoMapNavigationCoverageState = "partial"
	RepoMapNavigationCoverageMissing     RepoMapNavigationCoverageState = "missing"
)

type RepoMapNavigationCoverage struct {
	State          RepoMapNavigationCoverageState `json:"state,omitempty"`
	ReasonCode     string                         `json:"reason_code,omitempty"`
	RequiredRoutes []RepoMapNavigationRoute       `json:"required_routes,omitempty"`
	ObservedRoutes []RepoMapNavigationRoute       `json:"observed_routes,omitempty"`
	CoveredRoutes  []RepoMapNavigationRoute       `json:"covered_routes,omitempty"`
	MissingRoutes  []RepoMapNavigationRoute       `json:"missing_routes,omitempty"`
	EvidenceRefs   []string                       `json:"evidence_refs,omitempty"`
}

func NormalizeRepoMapNavigationRoute(raw string) RepoMapNavigationRoute {
	route := strings.ToLower(strings.TrimSpace(raw))
	route = strings.ReplaceAll(route, "-", "_")
	switch route {
	case "", "default":
		return ""
	case "overview":
		return RepoMapNavigationRouteOverview
	case "filemap", "file_map":
		return RepoMapNavigationRouteFileMap
	case "taskmap", "task_map":
		return RepoMapNavigationRouteTaskMap
	case "callpath", "call_path":
		return RepoMapNavigationRouteCallPath
	case "editimpact", "edit_impact":
		return RepoMapNavigationRouteEditImpact
	case "semanticsubgraph", "semantic_subgraph":
		return RepoMapNavigationRouteSemanticGraph
	case "relationmap", "relation_map":
		return RepoMapNavigationRouteRelationMap
	case "sourceinventory", "source_inventory":
		return RepoMapNavigationRouteSourceInventory
	case "implementors", "implementers":
		return RepoMapNavigationRouteImplementers
	default:
		return ""
	}
}

func (r RepoMapNavigationRoute) Valid() bool {
	switch r {
	case RepoMapNavigationRouteOverview,
		RepoMapNavigationRouteTaskMap,
		RepoMapNavigationRouteFileMap,
		RepoMapNavigationRouteSourceInventory,
		RepoMapNavigationRouteImplementers,
		RepoMapNavigationRouteRelationMap,
		RepoMapNavigationRouteEditImpact,
		RepoMapNavigationRouteSemanticGraph,
		RepoMapNavigationRouteCallPath:
		return true
	default:
		return false
	}
}

func RepoMapNavigationCoverageFromToolResults(policy RepoMapNavigationPolicy, results []ToolResult) RepoMapNavigationCoverage {
	if len(results) == 0 {
		return RepoMapNavigationCoverageFromObservations(policy, nil)
	}
	var records []ObservationRecord
	for _, result := range results {
		if !result.Success {
			continue
		}
		records = append(records, result.Observations...)
	}
	return RepoMapNavigationCoverageFromObservations(policy, records)
}

func RepoMapNavigationCoverageFromObservations(policy RepoMapNavigationPolicy, records []ObservationRecord) RepoMapNavigationCoverage {
	required := repoMapNavigationRequiredRoutes(policy)
	observed, evidenceRefs := repoMapNavigationObservedRoutes(records)
	coverage := RepoMapNavigationCoverage{
		RequiredRoutes: required,
		ObservedRoutes: observed,
		EvidenceRefs:   evidenceRefs,
	}
	if len(required) == 0 {
		coverage.State = RepoMapNavigationCoverageNotRequired
		coverage.ReasonCode = "repo_map_navigation_not_required"
		return coverage
	}
	for _, route := range required {
		if repoMapNavigationRouteCovered(route, observed) {
			coverage.CoveredRoutes = append(coverage.CoveredRoutes, route)
			continue
		}
		coverage.MissingRoutes = append(coverage.MissingRoutes, route)
	}
	switch {
	case len(coverage.MissingRoutes) == 0:
		coverage.State = RepoMapNavigationCoverageCovered
		coverage.ReasonCode = "repo_map_navigation_covered"
	case len(coverage.CoveredRoutes) > 0:
		coverage.State = RepoMapNavigationCoveragePartial
		coverage.ReasonCode = "repo_map_navigation_partial"
	default:
		coverage.State = RepoMapNavigationCoverageMissing
		coverage.ReasonCode = "repo_map_navigation_missing"
	}
	return coverage
}

func RepoMapNavigationRouteFromObservation(record ObservationRecord) (RepoMapNavigationRoute, bool) {
	if strings.TrimSpace(record.Producer) != "repo_map" {
		return "", false
	}
	if strings.TrimSpace(record.Predicate) != RepoMapNavigationObservationPredicate {
		return "", false
	}
	for _, raw := range []string{record.Object, record.Value, record.ClaimKey} {
		route := NormalizeRepoMapNavigationRoute(raw)
		if route.Valid() {
			return route, true
		}
	}
	return "", false
}

func repoMapNavigationRequiredRoutes(policy RepoMapNavigationPolicy) []RepoMapNavigationRoute {
	seen := map[RepoMapNavigationRoute]bool{}
	var out []RepoMapNavigationRoute
	for _, step := range policy.Steps {
		route := NormalizeRepoMapNavigationRoute(string(step.Route))
		if !route.Valid() || seen[route] {
			continue
		}
		seen[route] = true
		out = append(out, route)
	}
	return out
}

func repoMapNavigationObservedRoutes(records []ObservationRecord) ([]RepoMapNavigationRoute, []string) {
	seen := map[RepoMapNavigationRoute]bool{}
	evidenceSeen := map[string]bool{}
	var routes []RepoMapNavigationRoute
	var refs []string
	for _, record := range records {
		route, ok := RepoMapNavigationRouteFromObservation(record)
		if !ok {
			continue
		}
		if !seen[route] {
			seen[route] = true
			routes = append(routes, route)
		}
		for _, ref := range []string{record.SourceRef.RawRef, record.SourceRef.PayloadRef, record.ID} {
			ref = strings.TrimSpace(ref)
			if ref == "" || evidenceSeen[ref] {
				continue
			}
			evidenceSeen[ref] = true
			refs = append(refs, ref)
			break
		}
	}
	sort.SliceStable(routes, func(i, j int) bool {
		return repoMapNavigationRouteRank(routes[i]) < repoMapNavigationRouteRank(routes[j])
	})
	return routes, refs
}

func repoMapNavigationRouteCovered(required RepoMapNavigationRoute, observed []RepoMapNavigationRoute) bool {
	for _, route := range observed {
		if route == required {
			return true
		}
		if required == RepoMapNavigationRouteSourceInventory && route == RepoMapNavigationRouteImplementers {
			return true
		}
	}
	return false
}

func repoMapNavigationRouteRank(route RepoMapNavigationRoute) int {
	switch route {
	case RepoMapNavigationRouteOverview:
		return 10
	case RepoMapNavigationRouteSourceInventory:
		return 20
	case RepoMapNavigationRouteImplementers:
		return 21
	case RepoMapNavigationRouteTaskMap:
		return 30
	case RepoMapNavigationRouteFileMap:
		return 40
	case RepoMapNavigationRouteRelationMap:
		return 50
	case RepoMapNavigationRouteCallPath:
		return 60
	case RepoMapNavigationRouteEditImpact:
		return 70
	case RepoMapNavigationRouteSemanticGraph:
		return 80
	default:
		return 100
	}
}
