package repomap

import (
	"fmt"
	"strings"
	"time"

	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

func repoMapNavigationTypedObservation(view, rawRef, renderedOutput string, observedAt time.Time) []ctypes.ObservationRecord {
	route := ctypes.NormalizeRepoMapNavigationRoute(view)
	if !route.Valid() {
		return nil
	}
	scope := relationMapObservationScope(rawRef, renderedOutput)
	at := observedAt.Format("2006-01-02T15:04:05Z07:00")
	origin := ctypes.AnswerEvidenceOriginCrossRepoIndex
	role := ctypes.AnswerAggregateRoleSupportingCoverage
	return []ctypes.ObservationRecord{{
		ID:              fmt.Sprintf("repo_map:%s#navigation:%s", scope, route),
		Origin:          origin,
		Producer:        "repo_map",
		Role:            role,
		GroundingPolicy: ctypes.AnswerClaimBindingGroundingPolicy(origin, role),
		SourceRef: ctypes.ObservationSourceRef{
			Kind:   ctypes.ObservationSourceCrossRepoIndex,
			RawRef: strings.TrimSpace(rawRef),
		},
		ClaimKey:   string(route),
		Predicate:  ctypes.RepoMapNavigationObservationPredicate,
		Object:     string(route),
		Value:      string(route),
		Summary:    fmt.Sprintf("repo_map view observed: %s", route),
		ObservedAt: at,
		Confidence: 1.0,
	}}
}
