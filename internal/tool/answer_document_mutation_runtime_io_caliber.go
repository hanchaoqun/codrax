package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The IO family does not identify the ruler of a published number. Only the
// node's optional measurement field does; closure, peer, state and prose do
// not select a ruler. This is display-only and grants no causal authority.
func runtimeTraceIOValueNode(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceIOValueToken(node) != ""
}

func runtimeTraceIOValueToken(node types.TraceCausalProjectionNode) string {
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		canonical := runtimeTraceCausalProjectionCanonicalNode(token)
		if types.TraceUsesIOValueCaliber(canonical, node.IOValueCaliber) {
			return canonical
		}
		if runtimeTraceProjImpactFormTokenFamily(canonical) != runtimeTraceProjImpactFormNone {
			return ""
		}
	}
	return ""
}

func runtimeTraceIOValueWord(node types.TraceCausalProjectionNode, zh bool) (string, bool) {
	if !runtimeTraceIOValueNode(node) {
		return "", false
	}
	return types.TraceIOValueCaliberLabel(node.IOValueCaliber, zh), true
}

func runtimeTraceIOValueCandidateWord(node types.TraceCausalProjectionNode, zh bool) (string, bool) {
	word, ok := runtimeTraceIOValueWord(node, zh)
	if !ok {
		return "", false
	}
	if zh {
		return word + "候选", true
	}
	return word + " candidate", true
}

func runtimeTraceIOValueCauseName(node types.TraceCausalProjectionNode, zh bool) (string, bool) {
	word, ok := runtimeTraceIOValueWord(node, zh)
	if !ok {
		return "", false
	}
	if runtimeTraceCausalProjectionUnknownSentinel(node.Object) {
		if zh {
			return word + "(对端未解析)", true
		}
		return word + " (peer unresolved)", true
	}
	if runtimeTraceCausalProjectionResolvedPeerObjectKind(node) == "io_latency" {
		peer := runtimeTraceCausalProjectionDisplayNodeName(strings.TrimSpace(node.Object), zh)
		if zh {
			return word + "(对端 " + peer + ")", true
		}
		return word + " (peer " + peer + ")", true
	}
	return word, true
}

func runtimeTraceCausalProjectionNarrativeCauseNameNode(node types.TraceCausalProjectionNode, zh bool) string {
	if word, ok := runtimeTraceIOValueWord(node, zh); ok {
		return word
	}
	return runtimeTraceCausalProjectionNarrativeCauseName(node.Object, zh)
}

func runtimeTraceProjIOFoldPeerTypeWord(peer runtimeTraceProjIOFoldPeer, zh bool) string {
	token := strings.TrimSpace(peer.Token)
	if types.TraceUsesIOValueCaliber(runtimeTraceCausalProjectionCanonicalNode(token), peer.IOValueCaliber) {
		return types.TraceIOValueCaliberLabel(peer.IOValueCaliber, zh)
	}
	if zh {
		if label := runtimeTraceRootCauseTypeZHLabel(token); label != "" && label != token {
			return label + "（" + token + "）"
		}
	}
	return token
}

// The display near-duplicate fold replaces only Impact/Cumulative MAX axes.
// Keep each positive axis's actual donor; publication evidence is not a donor.
func runtimeTraceIOValueDuplicateDonors(survivor *types.TraceCausalProjectionNode, original, dup types.TraceCausalProjectionNode) {
	if !runtimeTraceIOValueNode(original) && !runtimeTraceIOValueNode(dup) {
		return
	}
	caliber, seen := "", false
	include := func(value float64, ruler string) {
		if value <= 0 {
			return
		}
		if !seen {
			caliber, seen = types.NormalizeTraceIOValueCaliber(ruler), true
		} else {
			caliber = types.MergeTraceIOValueCalibers(caliber, ruler)
		}
	}
	maxAxis := func(value, previous, other float64) {
		ruler := original.IOValueCaliber
		if original.ImpactMS != dup.ImpactMS && other > previous {
			ruler = dup.IOValueCaliber
		}
		include(value, ruler)
	}
	maxAxis(survivor.ImpactMS, original.ImpactMS, dup.ImpactMS)
	maxAxis(survivor.CumulativeImpactMS, original.CumulativeImpactMS, dup.CumulativeImpactMS)
	include(survivor.TargetImpactMS, original.IOValueCaliber)
	include(survivor.EffectiveImpactMS, original.IOValueCaliber)
	include(survivor.ActualImpactMS, original.IOValueCaliber)
	survivor.IOValueCaliber = caliber
}
