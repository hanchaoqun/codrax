package tool

import (
	"fmt"
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Display-only provenance of a folded value. The IO type identifies a family,
// not the ruler of every publication in that family. In particular a rank
// publication can price the same request below its measured residence time.
// Nothing here changes the value, folding eligibility, ordering or authority.
type runtimeTraceProjIOFoldCaliber uint8

const (
	runtimeTraceProjIOFoldFamilyCaliber runtimeTraceProjIOFoldCaliber = iota
	runtimeTraceProjIOFoldRankImpact
	runtimeTraceProjIOFoldCumulative
	runtimeTraceProjIOFoldEffective
	runtimeTraceProjIOFoldActual
	runtimeTraceProjIOFoldNativeDuration
)

func runtimeTraceProjNewIOFoldPeer(node types.TraceCausalProjectionNode, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) runtimeTraceProjIOFoldPeer {
	value, source := runtimeTraceProjNodeDisplayImpactSource(node)
	peer := runtimeTraceProjIOFoldPeer{
		Token: strings.TrimSpace(node.TypeToken), ImpactMS: value,
		IOValueCaliber:    node.IOValueCaliber,
		EvidenceTag:       runtimeTraceProjEvidenceTag(node, evidence, zh),
		FamilyMemberCount: node.FamilyMemberCount,
		FamilyMemberMaxMS: node.FamilyMemberMaxMS,
		FamilyFoldCaliber: node.FamilyFoldCaliber,
		StartTs:           node.StartTs, EndTs: node.EndTs,
		QueryWindowStartTs: node.QueryWindowStartTs, QueryWindowEndTs: node.QueryWindowEndTs,
		MeasurementOrigins: types.CloneTraceSchedulerMeasurementOrigins(node.MeasurementOrigins),
	}
	// Existing score/count family disclosures remain authoritative and must
	// not be replaced by a millisecond label, even for rank publications.
	if tracequery.CausalTokenCaliberSideClass(strings.ToLower(peer.Token)) != tracequery.CausalCaliberSideNone {
		return peer
	}
	switch source {
	case runtimeTraceProjImpactSourceWindow:
		// This is the producer's typed predicate family, not raw model prose.
		// Legacy rank impact is not a fresh physical-duration measurement.
		// Only the publisher's explicit native-duration marker changes that
		// wording; numeric equality with another observation proves nothing.
		if strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_") {
			peer.Caliber = runtimeTraceProjIOFoldRankImpact
			if node.RankValueCaliber == types.TraceRankValueCaliberNativeDuration {
				peer.Caliber = runtimeTraceProjIOFoldNativeDuration
			}
		}
	case runtimeTraceProjImpactSourceCumulative:
		peer.Caliber = runtimeTraceProjIOFoldCumulative
	case runtimeTraceProjImpactSourceEffective:
		peer.Caliber = runtimeTraceProjIOFoldEffective
	case runtimeTraceProjImpactSourceActual:
		peer.Caliber = runtimeTraceProjIOFoldActual
	}
	return peer
}

// Only this peer's published endpoints may describe its measurement. A
// locator can be an envelope; neither it nor the query identifies a request
// or proves a continuous occurrence. Missing ranges never borrow the seat.
func runtimeTraceProjIOFoldScopeText(peer runtimeTraceProjIOFoldPeer, zh bool) string {
	locator, query, unpublished := "locator range", "query range", " unpublished"
	if zh {
		locator, query, unpublished = "定位范围", "查询范围", "未发布"
	}
	rangeText := func(label string, start, end float64) string {
		if !types.TraceCausalProjectionWindowPresent(start, end) || math.IsInf(start, 0) || math.IsInf(end, 0) || math.IsNaN(start) || math.IsNaN(end) {
			return label + unpublished
		}
		return fmt.Sprintf("%s %.6f–%.6fs", label, start, end)
	}
	return "(" + rangeText(locator, peer.StartTs, peer.EndTs) + "; " + rangeText(query, peer.QueryWindowStartTs, peer.QueryWindowEndTs) + ")"
}

func runtimeTraceProjIOFoldLayerWord(token string, caliber runtimeTraceProjIOFoldCaliber, zh bool) string {
	switch caliber {
	case runtimeTraceProjIOFoldNativeDuration:
		if zh {
			return "观测计时"
		}
		return "observed duration"
	case runtimeTraceProjIOFoldRankImpact:
		if zh {
			return "排序影响"
		}
		return "ranking impact"
	case runtimeTraceProjIOFoldCumulative:
		if zh {
			return "累计口径"
		}
		return "cumulative value"
	case runtimeTraceProjIOFoldEffective:
		return runtimeTraceProjImpactCaliberWord(runtimeTraceProjImpactSourceEffective, zh)
	case runtimeTraceProjIOFoldActual:
		return runtimeTraceProjImpactCaliberWord(runtimeTraceProjImpactSourceActual, zh)
	default:
		return runtimeTraceProjIOFacetLayerWord(token, zh)
	}
}
