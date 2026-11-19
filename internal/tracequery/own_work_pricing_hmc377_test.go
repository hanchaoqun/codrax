package tracequery

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

// HMC377 own-work pricing (lanes E1+E6). Engine-level, corpus-free tests of
// the one fold entry every consumer reads (SemanticSpanFamiliesForStats) and of
// the own-work lane of the analysed thread. Expected values are written down
// from the synthetic inputs, never read back from the engine.

func hmc377Near(got, want float64) bool { return math.Abs(got-want) < 1e-6 }

func hmc377Chain(target ThreadRef, nodes ...ChainNode) *ChainResult {
	return &ChainResult{Target: target, Window: TimeWindow{StartTs: 5.0, EndTs: 6.0}, Nodes: nodes}
}

// hmc377BoundedStats lists only the given spans while carrying the complete
// inventory, exactly the shape the window-stats pass hands its consumers after
// the display bound.
func hmc377BoundedStats(listed, inventory []TraceSpanSummary) WindowStats {
	return WindowStats{Window: TimeWindow{StartTs: 5.0, EndTs: 6.0}, TraceSpans: listed, traceSpanFullInventory: inventory}
}

// The target verifies 20 classes of 1 ms each; 15 longer verifications on
// other threads and the target's largest one fill the 16 built-in seats, so the bound lists one of the
// target's verifications. Every published quantity must still be the 20
// members and their 20.000 ms.
func TestHMC377OwnWorkFamilyTotalIsNotTheListedSample(t *testing.T) {
	target := ThreadRef{Comm: "main", PID: 100}
	var inventory []TraceSpanSummary
	for i := 0; i < 15; i++ {
		other := ThreadRef{Comm: fmt.Sprintf("verify-%d", 300+i), PID: 300 + i}
		inventory = append(inventory, rcmSpan(other, fmt.Sprintf("VerifyClass com.example.Other%d", i), 5.100, 5.130, 1000+i, 1100+i))
	}
	for i := 0; i < 20; i++ {
		start := 5.200 + float64(i)*0.002
		inventory = append(inventory, rcmSpan(target, fmt.Sprintf("VerifyClass com.example.Own%d", i), start, start+0.001, 2000+2*i, 2001+2*i))
	}
	listed, info := boundTraceMarkSpansWithInfo(append([]TraceSpanSummary(nil), inventory...), 8)
	own := 0
	for _, span := range listed {
		if span.Thread.PID == target.PID {
			own++
		}
	}
	if own != 1 || info.totalSpans != 35 || info.keptSpans != 16 {
		t.Fatalf("fixture drifted: the bound must list exactly one of the target's 20 spans: own=%d info=%+v", own, info)
	}
	chain := hmc377Chain(target, ChainNode{Thread: ThreadRef{Comm: "waker", PID: 500}, Window: TimeWindow{StartTs: 5.5, EndTs: 5.6}, Dominant: StateRunning})
	stats := hmc377BoundedStats(listed, inventory)
	var fam *SemanticSpanFamily
	families := SemanticSpanFamiliesForStats(chain, &stats)
	for i := range families {
		if sameThreadRef(families[i].Thread, target) {
			fam = &families[i]
		}
	}
	if fam == nil || !fam.OnChain || fam.OnChainBasis != RootCauseOnChainBasisSelfDeterministicSpan {
		t.Fatalf("the target's own verification family must be on the own-work lane: %+v", fam)
	}
	if len(fam.Members) != 20 || !hmc377Near(fam.TotalMs, 20.0) || !hmc377Near(fam.PricedOwnWorkMs(), 20.0) {
		t.Fatalf("member count and total must come from all 20 members, not the listed sample: members=%d total=%.6f", len(fam.Members), fam.TotalMs)
	}
	item, ok := rootCauseItemFromSemanticSpanFamily(Query{TimeStart: 5.0, TimeEnd: 6.0}, *fam, true)
	if !ok || item.MemberCount != 20 || !hmc377Near(item.EffectiveImpactMs, 20.0) || item.MemberNotListedCount != 0 || len(item.MemberRoster) != 20 {
		t.Fatalf("the rank row must publish 20 members / 20.000 ms with a complete roster: %+v", item)
	}
	// Background families keep their listing membership: every one of the 15
	// other threads has its one span listed, and each is published exactly once.
	background := 0
	for _, f := range families {
		if !f.OnChain {
			background++
		}
	}
	if background != 15 {
		t.Fatalf("every listed background family is published, and only those: %d", background)
	}
}

// Forty members of 0.5 ms: the roster lists the 32 largest and the row states,
// as a typed count, the 8 it does not list. Value and count are complete.
func TestHMC377OwnWorkRowStatesMembersNotListed(t *testing.T) {
	target := ThreadRef{Comm: "main", PID: 100}
	var spans []TraceSpanSummary
	for i := 0; i < 40; i++ {
		start := 5.100 + float64(i)*0.001
		spans = append(spans, rcmSpan(target, fmt.Sprintf("VerifyClass com.example.K%d", i), start, start+0.0005, 10+2*i, 11+2*i))
	}
	chain := hmc377Chain(target, ChainNode{Thread: ThreadRef{Comm: "waker", PID: 500}, Window: TimeWindow{StartTs: 5.5, EndTs: 5.6}, Dominant: StateRunning})
	listed, _ := boundTraceMarkSpansWithInfo(append([]TraceSpanSummary(nil), spans...), 8)
	stats := hmc377BoundedStats(listed, spans)
	families := SemanticSpanFamiliesForStats(chain, &stats)
	if len(families) != 1 {
		t.Fatalf("one own-work family expected: %d", len(families))
	}
	item, ok := rootCauseItemFromSemanticSpanFamily(Query{TimeStart: 5.0, TimeEnd: 6.0}, families[0], true)
	if !ok || item.MemberCount != 40 || !hmc377Near(item.EffectiveImpactMs, 20.0) {
		t.Fatalf("40 members x 0.5 ms = 20.000 ms: members=%d effective=%.6f", item.MemberCount, item.EffectiveImpactMs)
	}
	if len(item.MemberRoster) != 32 || item.MemberNotListedCount != 8 {
		t.Fatalf("the roster lists 32 and states the 8 it leaves out: roster=%d not_listed=%d", len(item.MemberRoster), item.MemberNotListedCount)
	}
}

// Below the bound nothing changes: with no span dropped the one entry is the
// listing fold itself.
func TestHMC377UnboundedWindowKeepsTheListingFold(t *testing.T) {
	target := ThreadRef{Comm: "main", PID: 100}
	var spans []TraceSpanSummary
	for i := 0; i < traceMarkSemanticSpanCap; i++ {
		start := 5.100 + float64(i)*0.002
		spans = append(spans, rcmSpan(target, fmt.Sprintf("VerifyClass com.example.K%d", i), start, start+0.001, 10+2*i, 11+2*i))
	}
	chain := hmc377Chain(target, ChainNode{Thread: ThreadRef{Comm: "waker", PID: 500}, Window: TimeWindow{StartTs: 5.5, EndTs: 5.6}, Dominant: StateRunning})
	listed, info := boundTraceMarkSpansWithInfo(append([]TraceSpanSummary(nil), spans...), 32)
	if info.totalSpans != 0 || len(listed) != len(spans) {
		t.Fatalf("fixture drifted: nothing may be dropped: %+v", info)
	}
	stats := hmc377BoundedStats(listed, spans)
	if got, want := SemanticSpanFamiliesForStats(chain, &stats), FoldSemanticSpanFamilies(chain, listed); !reflect.DeepEqual(got, want) {
		t.Fatalf("an unbounded window must fold exactly the listing:\n got %+v\nwant %+v", got, want)
	}
}

// An on-chain family none of whose members is listed is still published; a
// background family none of whose members is listed is not (it is counted by
// the bound's own disclosure).
func TestHMC377OnChainFamilyIsNeverLostToTheListing(t *testing.T) {
	target := ThreadRef{Comm: "main", PID: 100}
	var inventory []TraceSpanSummary
	for i := 0; i < 17; i++ {
		other := ThreadRef{Comm: fmt.Sprintf("jit-%d", 300+i), PID: 300 + i}
		inventory = append(inventory, rcmSpan(other, fmt.Sprintf("VerifyClass com.example.Other%d", i), 5.100, 5.150, 1000+i, 1100+i))
	}
	inventory = append(inventory, rcmSpan(target, "VerifyClass com.example.Own", 5.300, 5.301, 3000, 3001))
	listed, info := boundTraceMarkSpansWithInfo(append([]TraceSpanSummary(nil), inventory...), 8)
	for _, span := range listed {
		if span.Thread.PID == target.PID {
			t.Fatalf("fixture drifted: the target's span must be left out of the listing (%+v)", info)
		}
	}
	chain := hmc377Chain(target, ChainNode{Thread: ThreadRef{Comm: "waker", PID: 500}, Window: TimeWindow{StartTs: 5.5, EndTs: 5.6}, Dominant: StateRunning})
	stats := hmc377BoundedStats(listed, inventory)
	onChain, background := 0, 0
	for _, fam := range SemanticSpanFamiliesForStats(chain, &stats) {
		if sameThreadRef(fam.Thread, target) && fam.OnChain {
			onChain++
		}
		if !fam.OnChain {
			background++
		}
	}
	if onChain != 1 {
		t.Fatalf("the target's own family must be published although the listing left it out")
	}
	if background != 16 {
		t.Fatalf("only the 16 listed background families are published: %d", background)
	}
}

// E6: the only chain window on the target is one of its own running segments.
// The collection the target runs is its own work for its whole in-window
// extent: it is priced 30.000 ms on the own-work lane, not by the 14.7 ms
// running fragment the chain happened to name.
func TestHMC377OwnSpanIsNotPricedByItsOwnRunningFragment(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	span := rcmSpan(target, "H:FullGC::RunPhases;GCReason2", 5.100, 5.130, 10, 20)
	chain := hmc377Chain(target, ChainNode{Thread: target, Window: TimeWindow{StartTs: 5.110, EndTs: 5.1247}, Dominant: StateRunning})
	lane, _, _ := semanticSpanFamilyLane(chain, span)
	if lane != semanticSpanLaneChainSelf {
		t.Fatalf("the target's own running window must not turn its own work into an overlap: lane=%v", lane)
	}
	item, ok := rootCauseItemFromSemanticTraceSpan(Query{TimeStart: 5.0, TimeEnd: 6.0}, *chain, span, true)
	if !ok || item.OnChainBasis != RootCauseOnChainBasisSelfDeterministicSpan || !hmc377Near(item.EffectiveImpactMs, 30.0) || item.OverlapMs != 0 {
		t.Fatalf("the collection is 30.000 ms of own work, not the running fragment: %+v", item)
	}
	// Another thread's running window keeps the interval lane: running work of
	// a dependency is a real overlap.
	worker := ThreadRef{Comm: "worker", PID: 200}
	other := rcmSpan(worker, "H:FullGC::RunPhases;GCReason2", 5.100, 5.130, 30, 40)
	otherChain := hmc377Chain(target, ChainNode{Thread: worker, Window: TimeWindow{StartTs: 5.110, EndTs: 5.1247}, Dominant: StateRunning})
	if lane, _, _ := semanticSpanFamilyLane(otherChain, other); lane != semanticSpanLaneChainOverlap {
		t.Fatalf("a dependency thread's running window keeps the overlap lane: lane=%v", lane)
	}
}

// E6 with a real wait inside: the target sleeps 6 ms inside its own 30 ms
// verification on a chain window. The wait row keeps the intersection lane
// (6.000 ms, the sleep), the own-work row carries the other 24.000 ms, and
// the two add up to the span: nothing is counted twice, nothing is dropped.
func TestHMC377OwnSpanWithAWaitInsideSplitsByTime(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	span := rcmSpan(target, "VerifyClass com.example.Big", 5.100, 5.130, 10, 20)
	chain := hmc377Chain(target,
		ChainNode{Thread: target, Window: TimeWindow{StartTs: 5.110, EndTs: 5.116}, Dominant: StateSSleep},
		ChainNode{Thread: target, Window: TimeWindow{StartTs: 5.120, EndTs: 5.125}, Dominant: StateRunning})
	stats := WindowStats{Window: TimeWindow{StartTs: 5.0, EndTs: 6.0}, TraceSpans: []TraceSpanSummary{span}}
	families := SemanticSpanFamiliesForStats(chain, &stats)
	if len(families) != 2 {
		t.Fatalf("a wait family and an own-work family of the same span expected: %+v", families)
	}
	var wait, own *SemanticSpanFamily
	for i := range families {
		switch families[i].OnChainBasis {
		case "":
			wait = &families[i]
		case RootCauseOnChainBasisSelfDeterministicSpan:
			own = &families[i]
		}
	}
	if wait == nil || own == nil {
		t.Fatalf("both lanes expected: %+v", families)
	}
	if !hmc377Near(wait.ProjectedImpactMs, 6.0) || wait.DominantState != string(StateSSleep) {
		t.Fatalf("the wait share is the 6 ms sleep: %+v", wait)
	}
	ownMs, waitMs, split := own.OwnWorkSplit()
	if !split || !hmc377Near(ownMs, 24.0) || !hmc377Near(waitMs, 6.0) || !hmc377Near(ownMs+waitMs, own.TotalMs) {
		t.Fatalf("own work 24.000 + wait 6.000 = 30.000: own=%.6f wait=%.6f total=%.6f", ownMs, waitMs, own.TotalMs)
	}
	q := Query{TimeStart: 5.0, TimeEnd: 6.0}
	waitItem, ok1 := rootCauseItemFromSemanticTraceSpan(q, *chain, span, true)
	ownItem, ok2 := rootCauseItemFromSemanticSpanFamily(q, *own, true)
	if !ok1 || !ok2 || !hmc377Near(waitItem.EffectiveImpactMs, 6.0) || !hmc377Near(ownItem.EffectiveImpactMs, 24.0) ||
		ownItem.OnChainBasis != RootCauseOnChainBasisSelfDeterministicSpan || waitItem.OnChainBasis != "" {
		t.Fatalf("rows: wait=%.6f (%q) own=%.6f (%q)", waitItem.EffectiveImpactMs, waitItem.OnChainBasis, ownItem.EffectiveImpactMs, ownItem.OnChainBasis)
	}
	// The own-work row's support inventory is its priced share: it never
	// contains an instant of the wait.
	var supportMs float64
	for _, iv := range ownItem.semanticMemberIntervals {
		if iv.start < 5.116 && iv.end > 5.110 {
			t.Fatalf("own-work support overlaps the wait: %+v", ownItem.semanticMemberIntervals)
		}
		supportMs += (iv.end - iv.start) * 1000
	}
	if !hmc377Near(supportMs, 24.0) {
		t.Fatalf("support inventory = priced share: %.6f", supportMs)
	}
}

// A marker that records a wait (vocabulary stance "waits") is never own work:
// it keeps only its wait row, priced by the sleep, exactly as before.
func TestHMC377RecordedWaitGainsNoOwnWorkRow(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	span := rcmSpan(target, "GC: Wait For Completion Alloc", 5.100, 5.130, 10, 20)
	if !semanticSpanRecordsWait(span.Name) {
		t.Fatalf("fixture drifted: the marker must carry the waits stance")
	}
	chain := hmc377Chain(target, ChainNode{Thread: target, Window: TimeWindow{StartTs: 5.101, EndTs: 5.129}, Dominant: StateSSleep})
	stats := WindowStats{Window: TimeWindow{StartTs: 5.0, EndTs: 6.0}, TraceSpans: []TraceSpanSummary{span}}
	families := SemanticSpanFamiliesForStats(chain, &stats)
	if len(families) != 1 || families[0].OnChainBasis != "" || !hmc377Near(families[0].ProjectedImpactMs, 28.0) {
		t.Fatalf("one wait family priced by its 28 ms sleep: %+v", families)
	}
	if _, _, split := families[0].OwnWorkSplit(); split {
		t.Fatalf("a recorded wait has no own-work share")
	}
}

// The on-CPU share of an own-work row is |own work ∩ running|.
func TestHMC377OwnWorkOnCPUIsTheRunningShareOfThePricedWork(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	items := []RootCauseRankItem{{
		Type: "gc_pause", Thread: target, SemanticClass: "gc_pause", SpanName: "H:FullGC::RunPhases;GCReason2",
		OnChainBasis: RootCauseOnChainBasisSelfDeterministicSpan, StartTs: 5.100, EndTs: 5.130,
	}, {
		Type: "gc_pause", Thread: target, SemanticClass: "gc_pause", SpanName: "GC: Wait For Completion Alloc",
		OnChainBasis: RootCauseOnChainBasisSelfDeterministicSpan, StartTs: 5.200, EndTs: 5.230,
	}}
	running := func() []foldInterval {
		return []foldInterval{{start: 5.090, end: 5.105}, {start: 5.110, end: 5.120}, {start: 5.210, end: 5.220}}
	}
	stampOwnWorkOnCPU(items, target, running)
	if !hmc377Near(items[0].OwnWorkOnCPUMs, 15.0) {
		t.Fatalf("5 ms + 10 ms of the collection ran on a CPU: %.6f", items[0].OwnWorkOnCPUMs)
	}
	if items[1].OwnWorkOnCPUMs != 0 {
		t.Fatalf("a recorded wait carries no own-work on-CPU share: %.6f", items[1].OwnWorkOnCPUMs)
	}
}

// The listing bound's disclosure no longer calls totals lower bounds, and the
// exactly-full warning is silent when the complete inventory was folded.
func TestHMC377BoundDisclosureSaysOnlyTheListingIsBounded(t *testing.T) {
	info := traceMarkSemanticBoundInfo{totalSpans: 147, keptSpans: 16, totalFamilies: 11}
	caveat := info.caveat()
	if strings.Contains(caveat, "lower bound") || !strings.Contains(caveat, "computed from all 147 classified span(s)") {
		t.Fatalf("the disclosure must say totals come from every classified span: %q", caveat)
	}
	worker := ThreadRef{Comm: "worker", PID: 200}
	var spans []TraceSpanSummary
	for i := 0; i < traceMarkSemanticSpanCap; i++ {
		start := 5.001 + float64(i)*0.001
		spans = append(spans, rcmSpan(worker, "VerifyClass com.example.K", start, start+0.0005, 100+i, 100+i))
	}
	inventory := append(append([]TraceSpanSummary(nil), spans...), rcmSpan(worker, "VerifyClass com.example.K", 5.5, 5.5005, 999, 999))
	if _, ok := semanticSpanCapLowerBoundCaveat(WindowStats{TraceSpans: spans, traceSpanFullInventory: inventory}); ok {
		t.Fatalf("with the complete inventory folded the exactly-full warning is false")
	}
	if _, ok := semanticSpanCapLowerBoundCaveat(WindowStats{TraceSpans: spans}); !ok {
		t.Fatalf("without an inventory the listed spans are all there is: the warning stays")
	}
}

// Deterministic running reads the complete inventory too.
func TestHMC377DeterministicRunningReadsTheCompleteInventory(t *testing.T) {
	target := ThreadRef{Comm: "main", PID: 100}
	var inventory []TraceSpanSummary
	for i := 0; i < 20; i++ {
		start := 5.200 + float64(i)*0.002
		inventory = append(inventory, rcmSpan(target, fmt.Sprintf("VerifyClass com.example.Own%d", i), start, start+0.001, 2000+2*i, 2001+2*i))
	}
	stats := hmc377BoundedStats(inventory[:3], inventory)
	window := TimeWindow{StartTs: 5.0, EndTs: 6.0}
	running := []foldInterval{{start: 5.0, end: 6.0}}
	if got := targetSemanticRunningMs(&stats, target, window, running); !hmc377Near(got, 20.0) {
		t.Fatalf("deterministic running is 20.000 ms over all 20 members, got %.6f", got)
	}
}
