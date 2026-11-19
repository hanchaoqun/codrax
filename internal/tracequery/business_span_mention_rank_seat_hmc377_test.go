package tracequery

import (
	"testing"
)

// HMC377 fix round 2, item F2 (batch-owner ruling R-377-6): a semantic family
// "holds a seat" when the rank fold's family set publishes it — the same set
// the rank mints its rows from (SemanticSpanFamiliesForStats). The
// business-clue face reads that one set: a platform-vocabulary family the
// rank publishes is never also a business clue (one time, one face), and a
// vocabulary family the rank does not publish keeps its on-chain time visible
// on the face.
//
// Witness universe (reviewer repro): r3SyntheticChain, window
// [6.000, 6.010]; relay-200 (a depth-1 chain node, node window [6.000, 6.004])
// runs "H:FullGC::RunPhases;GCReason2" over [6.001, 6.003]. The bounded
// listing holds only the analysed thread's own business span, so the
// collector family is wholly outside the listing — the shape in which the
// rank fold still publishes an on-chain family from the complete inventory.
func hmc377F2Universe() (Query, ChainResult, WindowStats, TraceSpanSummary) {
	chain := r3SyntheticChain()
	q := Query{PID: 100, TimeStart: 6.000, TimeEnd: 6.010, Limit: 12}
	const collector = "H:FullGC::RunPhases;GCReason2"
	gc := TraceSpanSummary{Thread: ThreadRef{Comm: "relay", PID: 200}, Name: collector, Kind: "sync",
		SemanticClass: traceSpanSemanticClass(collector), StartTs: 6.001, EndTs: 6.003, DurationMs: 2,
		StartLine: 60010, EndLine: 60030}
	own := TraceSpanSummary{Thread: ThreadRef{Comm: "app", PID: 100}, Name: "H:LaunchAbility", Kind: "sync",
		StartTs: 6.000, EndTs: 6.004, DurationMs: 4, StartLine: 60000, EndLine: 60040}
	stats := WindowStats{TraceSpans: []TraceSpanSummary{own}, traceSpanFullInventory: []TraceSpanSummary{own, gc}}
	return q, chain, stats, gc
}

func hmc377F2FaceNames(res *BusinessSpanMentionResult) map[string]BusinessSpanMention {
	out := map[string]BusinessSpanMention{}
	if res == nil {
		return out
	}
	for _, fam := range res.Families {
		out[fam.Name] = fam
	}
	return out
}

// The rank publication and the face are checked together through the one
// rank build: the collector family is published by the fold (an on-chain
// family is never lost to the listing bound), the rank mints its row, and the
// business-clue face does not show the same time a second time.
func TestHMC377PublishedVocabularyFamilyHoldsItsSeatAndLeavesTheClueFace(t *testing.T) {
	if !semanticSpanMintedByPlatformVocabulary("H:FullGC::RunPhases;GCReason2") {
		t.Fatal("fixture premise: the collector cycle is classified by the platform vocabulary")
	}
	q, chain, stats, gc := hmc377F2Universe()
	published := false
	for _, fam := range SemanticSpanFamiliesForStats(&chain, &stats) {
		for _, member := range fam.Members {
			if member.Name == gc.Name && member.Thread.PID == gc.Thread.PID {
				published = fam.OnChain
			}
		}
	}
	if !published {
		t.Fatal("premise: the rank fold publishes the on-chain collector family although no member is listed")
	}
	rank := buildRootCauseRankFrom(nil, q, chain, stats)
	minted := false
	for _, pool := range [][]RootCauseRankItem{rank.Items, rank.preTruncationItems} {
		for _, item := range pool {
			if item.Thread.PID == gc.Thread.PID && item.SemanticClass == gc.SemanticClass && rootCauseItemIsOnChain(item) {
				minted = true
			}
		}
	}
	if !minted {
		t.Fatalf("the rank mints the published collector family as an on-chain row: %+v", rank.Items)
	}
	face := hmc377F2FaceNames(rank.BusinessSpanMentions)
	if _, twice := face[gc.Name]; twice {
		t.Fatalf("a family the rank publishes must not also be a business clue (one time, one face): %+v", face[gc.Name])
	}
	if _, ok := face["H:LaunchAbility"]; !ok {
		t.Fatalf("the face itself is still computed (the analysed thread's own phase): %+v", face)
	}
}

// The face's exception reads exactly the family set it is handed: with the
// rank's set the collector is seated and absent from the face; with a set
// that does not publish it, the exception keeps its on-chain time visible.
func TestHMC377ClueFaceReadsTheRankFamilySet(t *testing.T) {
	q, chain, stats, gc := hmc377F2Universe()
	chainThreads := wakeupChainThreadSet(chain)
	seated := computeBusinessSpanMentions(q, chain, chainThreads, stats, SemanticSpanFamiliesForStats(&chain, &stats))
	if _, ok := hmc377F2FaceNames(seated)[gc.Name]; ok {
		t.Fatalf("with the rank's family set the collector holds a seat and has no clue row: %+v", seated)
	}
	unseated := computeBusinessSpanMentions(q, chain, chainThreads, stats, nil)
	fam, ok := hmc377F2FaceNames(unseated)[gc.Name]
	if !ok || fam.OnChainBasis != BusinessSpanMentionBasisChainMember || fam.TotalMs < 1.999 || fam.TotalMs > 2.001 {
		t.Fatalf("a vocabulary family no published family holds stays on the face with its on-chain time: %+v ok=%v", fam, ok)
	}
}
