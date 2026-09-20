package tool

import (
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
)

func runtimeTraceProjNewIOFoldPeer(node types.TraceCausalProjectionNode, evidence *runtimeTraceCausalProjectionEvidenceIndex, zh bool) runtimeTraceProjIOFoldPeer {
	value, source := runtimeTraceProjNodeDisplayImpactSource(node)
	peer := runtimeTraceProjIOFoldPeer{
		Token: strings.TrimSpace(node.TypeToken), ImpactMS: value,
		EvidenceTag: runtimeTraceProjEvidenceTag(node, evidence, zh),
	}
	// Existing score/count family disclosures remain authoritative and must
	// not be replaced by a millisecond label, even for rank publications.
	if tracequery.CausalTokenCaliberSideClass(strings.ToLower(peer.Token)) != tracequery.CausalCaliberSideNone {
		return peer
	}
	switch source {
	case runtimeTraceProjImpactSourceWindow:
		// This is the producer's typed predicate family, not raw model prose.
		// A rank impact is not a fresh physical-duration measurement even if
		// its numeric value happens to equal another observation's duration.
		if strings.HasPrefix(strings.TrimSpace(node.Predicate), "root_cause_") {
			peer.Caliber = runtimeTraceProjIOFoldRankImpact
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

func runtimeTraceProjIOFoldLayerWord(token string, caliber runtimeTraceProjIOFoldCaliber, zh bool) string {
	switch caliber {
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
