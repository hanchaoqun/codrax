package tool

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 15 (batch-owner ruling R-377-15 ⑦, 2026-11-24; review finding
// R14E-3, universes Z1cTwinOnly_H, Y2roleWin_H, Y4seatVO_H, Y4seatVOvo_H), end
// to end through production (publication gate -> typed observations ->
// projection -> tree model -> fences): the overview lists what the tree prices.
//
// The gate withdraws an inversion seat to a context-only row and releases its
// claimless scheduler-latency twin, the only carrier of the seat's 1.903ms, to
// the board with no ordinal. Round 14 keeps the twin a row of its own beside
// the context row. On a face that also draws a wakeup-chain hop of the thread
// over the same account, the hop row and the twin are one fact and one row:
// the twin rides in the hop row. At c797b464c that row printed the attribution
//
//	├─ ⧖ binder:458_5-6326 · runnable      █░░░░░░░░░     1.903ms  <1%  影响点 调度延迟 · [E2(+1)]
//	│ · 同段镜像·与[E3]同一物理时间,不可相加 · 有效归因 1.903ms
//
// while ◎ did not list it: the twin handed the hop row no ordinal, and with
// the ordinal went its board membership. The seat a row publishes is the seat
// ◎ admits, also when that seat holds no ordinal.

// hmc377R15WithHop draws a wakeup-chain hop of the thread over the account of
// every on-chain inversion seat (the engine mints the seat on that hop: same
// thread, lines, interval and state split, one measured state account).
func hmc377R15WithHop(result tracequery.Result) tracequery.Result {
	rank := *result.RootCauseRank
	rank.Items = append([]tracequery.RootCauseRankItem(nil), rank.Items...)
	chain := *result.WakeupChain
	chain.CausalImpacts = append([]tracequery.WakeupCausalImpact(nil), chain.CausalImpacts...)
	for i := range rank.Items {
		it := &rank.Items[i]
		if it.Type != "priority_inversion_candidate" || it.ChainRelevance != "on_chain" {
			continue
		}
		key := fmt.Sprintf("state_account:v2:hmc377r15-%d-%d", it.Thread.PID, it.LineStart)
		it.MeasuredStateAccountKey = key
		chain.CausalImpacts = append(chain.CausalImpacts, hmc377R15Hop(*it, key))
	}
	result.RootCauseRank, result.WakeupChain = &rank, &chain
	return result
}

func hmc377R15Hop(it tracequery.RootCauseRankItem, key string) tracequery.WakeupCausalImpact {
	return tracequery.WakeupCausalImpact{
		Thread: it.Thread, ChainDepth: 2, DominantState: "runnable", DominantImpactMs: it.RunnableMs, TotalMs: it.RunnableMs + it.RunningMs,
		RunnableMs: it.RunnableMs, RunningMs: it.RunningMs, OnChain: true, LineStart: it.LineStart, LineEnd: it.LineEnd,
		Window:                     tracequery.TimeWindow{StartTs: it.StartTs, EndTs: it.EndTs},
		PriorityInversionCandidate: it.Type == "priority_inversion_candidate", PriorityRelation: "lower_priority_dependency",
		PriorityRelationCaliber: it.PriorityRelationCaliber, PriorityRelationProvenLowerMs: it.PriorityRelationProvenLowerMs,
		PriorityRelationArtifactSources: it.PriorityRelationArtifactSources,
		PriorityInversionGatedMs:        it.EffectiveImpactMs, GatedRunnableMs: it.GatedRunnableMs, GatedRunningDeficitMs: it.GatedRunningDeficitMs,
		StateAccountKey: key,
	}
}

// hmc377R15Published is the published form of the round-13 universe on its hop
// face: [0] the other thread's seat, [1] the withdrawn seat as a context row,
// [2] the released twin, priced, with no ordinal; and the two hops.
func hmc377R15Published(t *testing.T) tracequery.Result {
	t.Helper()
	pub := traceQueryPriorityResultForPublication(hmc377R15WithHop(hmc377R13WithdrawnAbsorberResult()))
	items := pub.RootCauseRank.Items
	if len(items) != 3 || len(pub.RootCauseRank.AbsorbedItems) != 0 ||
		items[1].Tier != tracequery.RootCauseTierContextOnly || items[1].EffectiveImpactMs != 0 || items[1].MeasuredStateAccountKey == "" ||
		items[2].Type != "scheduler_latency" || items[2].EffectiveImpactMs != 1.903 || items[2].Rank != 0 || items[2].BackgroundRank != 0 ||
		len(pub.WakeupChain.CausalImpacts) != 2 || pub.WakeupChain.CausalImpacts[1].StateAccountKey != items[1].MeasuredStateAccountKey {
		t.Fatalf("precondition: the gate publishes the withdrawn seat as context and its released twin priced with no ordinal, beside the thread's hop: %+v", items)
	}
	// The variants below edit a copy of the published board.
	rank := *pub.RootCauseRank
	rank.Items = append([]tracequery.RootCauseRankItem(nil), items...)
	chain := *pub.WakeupChain
	chain.CausalImpacts = append([]tracequery.WakeupCausalImpact(nil), chain.CausalImpacts...)
	pub.RootCauseRank, pub.WakeupChain = &rank, &chain
	return pub
}

// hmc377R15Thread is what the faces say about the dependency thread.
type hmc377R15Thread struct {
	f hmc377R9Face
	// context and priced are the thread's drawn data rows, by typed tier.
	context, priced []*runtimeTraceProjTreeRow
	// board is the thread's ◎ members: evidence tag -> listed value.
	board map[string]float64
	// overview, native and llm are the thread's sums on ◎, on the published
	// rank board and in the model-facing summary.
	overview, native, llm float64
}

var hmc377R15PreviewRow = regexp.MustCompile(`root_cause_rank_preview_row .*tier=(\S*) .*subject=(\S+) .*effective_impact=([0-9.]+)ms`)

func hmc377R15Run(t *testing.T, result tracequery.Result) hmc377R15Thread {
	t.Helper()
	return hmc377R15RunLedger(t, result, hmc377R15Records(result, "payload"))
}

func hmc377R15Records(result tracequery.Result, scope string) []types.ObservationRecord {
	return traceQueryTypedObservations(result, "capture.ftrace", scope, "raw", "", time.Unix(0, 0).UTC())
}

// hmc377R15RunLedger compiles records (the publications of result, one or
// more) and reads the dependency thread's rows and sums off every face.
func hmc377R15RunLedger(t *testing.T, result tracequery.Result, records []types.ObservationRecord) hmc377R15Thread {
	t.Helper()
	out := hmc377R15Thread{board: map[string]float64{}}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("projections %d", len(set.Projections))
	}
	projection := set.Projections[0]
	en := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	out.f.en, out.f.elimEN = runtimeTraceProjTreeFence(en, false), runtimeTraceProjElimOverviewFence(projection, en, false)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	out.f.zh, out.f.elimZH = runtimeTraceProjTreeFence(model, true), runtimeTraceProjElimOverviewFence(projection, model, true)
	for _, row := range runtimeTraceProjSMR1AllRows(&model) {
		if !row.HasData {
			continue
		}
		out.f.rows = append(out.f.rows, fmt.Sprintf("%s %s %s rank=%d tier=%s val=%.3f merged=%d elim=%t", row.EvidenceTag, row.Node.Subject, row.Node.Predicate,
			row.Node.Rank, row.Node.Tier, runtimeTraceProjNodeDisplayImpact(row.Node), len(row.Node.MergedEvidenceIDs), runtimeTraceProjElimEligible(*row)))
		if row.Node.Subject != hmc377R9DepSubject {
			continue
		}
		if row.Node.IsContextOnlyRow() {
			out.context = append(out.context, row)
		} else {
			out.priced = append(out.priced, row)
		}
	}
	for _, e := range runtimeTraceProjElimBoard(model) {
		if e.row.Node.Subject == hmc377R9DepSubject {
			out.f.depCarriers = append(out.f.depCarriers, fmt.Sprintf("%s/%s/%.3f", e.row.EvidenceTag, e.row.Node.Predicate, runtimeTraceProjNodeDisplayImpact(e.row.Node)))
			out.board[e.row.EvidenceTag] += runtimeTraceProjNodeDisplayImpact(e.row.Node)
			out.overview += runtimeTraceProjNodeDisplayImpact(e.row.Node)
		}
	}
	pub := traceQueryPriorityResultForPublication(result)
	for _, it := range pub.RootCauseRank.Items {
		if it.Thread.PID == hmc377R9Dep.PID && it.Tier != tracequery.RootCauseTierContextOnly {
			out.native += it.EffectiveImpactMs
		}
	}
	for _, line := range strings.Split(traceQuerySummary(pub, traceQueryParams{View: "root_cause_rank"}, "capture.ftrace", "blob:x"), "\n") {
		m := hmc377R15PreviewRow.FindStringSubmatch(line)
		if m == nil || m[2] != hmc377R9DepSubject || m[1] == tracequery.RootCauseTierContextOnly {
			continue
		}
		v, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			t.Fatalf("summary row %q: %v", line, err)
		}
		out.llm += v
	}
	return out
}

func hmc377R15Near(a, b float64) bool { return math.Abs(a-b) < 0.0005 }

// hmc377R15Block is the drawn tree block of the row wearing tag.
func hmc377R15Block(fence, tag string) string {
	for _, block := range hmc377R14RowBlocks(fence) {
		if head, _, _ := strings.Cut(block, "\n"); strings.HasSuffix(strings.TrimSpace(head), "["+tag+"]") {
			return block
		}
	}
	return ""
}

// hmc377R15Listed reports the ◎ lines that list tag with value, above the
// auxiliary accounts.
func hmc377R15Listed(elim, tag, value string) int {
	n := 0
	for _, line := range strings.Split(elim, "\n") {
		if strings.HasPrefix(line, "— ") {
			break
		}
		if strings.Contains(line, value) && strings.HasSuffix(strings.TrimSpace(line), "["+tag+"]") {
			n++
		}
	}
	return n
}

func hmc377R15Line(fence, word string) string {
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, word) {
			return line
		}
	}
	return ""
}

func TestHMC377R15OverviewListsTheReleasedTwinThatRidesInAHopRow(t *testing.T) {
	engine := hmc377R15WithHop(hmc377R13WithdrawnAbsorberResult())
	hmc377R15Published(t) // the precondition on what the gate publishes
	th := hmc377R15Run(t, engine)
	if len(th.context) != 1 || len(th.priced) != 1 {
		t.Fatalf("the thread has one context row and one priced row: context %d priced %d\n%s", len(th.context), len(th.priced), th.f.dump())
	}
	context, hop := th.context[0], th.priced[0]
	if hop.Kind == runtimeTraceProjTreeRowCause || strings.HasPrefix(hop.Node.Predicate, "root_cause_") || hop.Node.Rank != 0 || hop.Node.EffectiveImpactMS != 1.903 {
		t.Fatalf("precondition: the priced row is the thread's hop row, with no ordinal: predicate %s rank %d effective %.3f\n%s",
			hop.Node.Predicate, hop.Node.Rank, hop.Node.EffectiveImpactMS, th.f.dump())
	}
	// The seat the hop row publishes is the released twin.
	seat := runtimeTraceProjRowPublishedSeat(*hop)
	if !strings.HasPrefix(seat.Predicate, "root_cause_") || seat.Object != "scheduler_latency" || seat.EffectiveImpactMS != 1.903 ||
		seat.Rank != 0 || seat.IsContextOnlyRow() {
		t.Errorf("the hop row publishes the released twin's seat: predicate %s object %s effective %.3f rank %d tier %s\n%s",
			seat.Predicate, seat.Object, seat.EffectiveImpactMS, seat.Rank, seat.Tier, th.f.dump())
	}
	// ◎ lists the row the tree prices, once, with the tree's number.
	if !runtimeTraceProjElimEligible(*hop) || len(th.board) != 1 || !hmc377R15Near(th.board[hop.EvidenceTag], 1.903) {
		t.Errorf("◎ lists the hop row [%s] with 1.903ms and no other row of the thread: %v\n%s", hop.EvidenceTag, th.board, th.f.dump())
	}
	if runtimeTraceProjElimEligible(*context) || context.Node.EffectiveImpactMS != 0 {
		t.Errorf("the context row carries no attribution and is no ◎ member\n%s", th.f.dump())
	}
	// One share, one number on every face: ◎ = the rank board = the summary = raw.
	if !hmc377R15Near(th.overview, 1.903) || !hmc377R15Near(th.native, 1.903) || !hmc377R15Near(th.llm, 1.903) {
		t.Errorf("the thread's 1.903ms: ◎ %.3f rank board %.3f summary %.3f\n%s", th.overview, th.native, th.llm, th.f.dump())
	}
	for _, face := range []struct {
		tree, elim                                 string
		contextWord, attribution, mirror, unpriced string
	}{
		{th.f.zh, th.f.elimZH, "上下文·不参与根因排序", "有效归因 1.903ms", "同段镜像·与[%s]同一物理时间", "⛓ 1 行(最大 1.903ms [%s])"},
		{th.f.en, th.f.elimEN, "context · not ranked", "attribution 1.903ms", "same physical time as [%s]", "⛓ 1 row(s) (largest 1.903ms [%s])"},
	} {
		hopBlock, contextBlock := hmc377R15Block(face.tree, hop.EvidenceTag), hmc377R15Block(face.tree, context.EvidenceTag)
		// The tree prices the hop row; ◎ lists that row with that number.
		if !strings.Contains(hopBlock, face.attribution) || hmc377R15Listed(face.elim, hop.EvidenceTag, "1.903ms") != 1 {
			t.Errorf("the row the tree prices is the row ◎ lists, 1.903ms on [%s]:\n%s\n%s", hop.EvidenceTag, face.elim, face.tree)
		}
		// The context row: its words, no attribution, and the mirror tag
		// that names the drawn hop row of the same account.
		if !strings.Contains(contextBlock, face.contextWord) || strings.Contains(contextBlock, face.attribution) ||
			!strings.Contains(contextBlock, fmt.Sprintf(face.mirror, hop.EvidenceTag)) ||
			!strings.Contains(hopBlock, fmt.Sprintf(face.mirror, context.EvidenceTag)) || strings.Contains(hopBlock, face.contextWord) {
			t.Errorf("the context row [%s] prints no attribution and mirrors the hop row [%s]:\n%s", context.EvidenceTag, hop.EvidenceTag, face.tree)
		}
		if hmc377R15Listed(face.elim, context.EvidenceTag, "1.903ms") != 0 {
			t.Errorf("◎ lists the context row [%s]:\n%s", context.EvidenceTag, face.elim)
		}
		// Unpriced occupancy counts the context row and never the priced row.
		word := "未计价占用"
		if face.contextWord != "上下文·不参与根因排序" {
			word = "unpriced occupancy"
		}
		if line := hmc377R15Line(face.elim, word); !strings.Contains(line, fmt.Sprintf(face.unpriced, context.EvidenceTag)) {
			t.Errorf("unpriced occupancy counts one row, the context row [%s]: %q\n%s", context.EvidenceTag, line, face.elim)
		}
	}
}

// What rides in the hop row decides: only a priced board row that had no
// ordinal to hand over is a seat the hop row publishes. A plain hop row, a hop
// row that took a board row without attribution, a row of the analysed
// thread's own state tier, a lossless detail of another row, or a seat whose
// whole account a chain seat represents (the arbitration under which an
// ordinal follows the fact) stays off ◎ as before; a hop row that took an
// ordinal publishes its own seat as before, whichever view it met first; and a
// board row that keeps the fact publishes itself.
func TestHMC377R15AHopRowPublishesOnlyAPricedBoardSeatWithoutOrdinal(t *testing.T) {
	const twin = 2
	for _, tc := range []struct {
		name string
		// edit changes the published board; the records it returns are a
		// second publication folded into the same ledger.
		edit func(pub *tracequery.Result) []types.ObservationRecord
		// hop: the thread's priced row is its hop row. merged: the
		// publications folded into that row. carried: the row's node carries
		// a board seat that had no ordinal. ownSeat: the seat the row
		// publishes is its own node. listed: the row is a ◎ member.
		hop                      bool
		merged                   int
		carried, ownSeat, listed bool
		rank                     int
	}{
		{"the released twin, priced, no ordinal", func(*tracequery.Result) []types.ObservationRecord { return nil }, true, 1, true, false, true, 0},
		{"no board row rides in the hop row", func(pub *tracequery.Result) []types.ObservationRecord {
			pub.RootCauseRank.Items = pub.RootCauseRank.Items[:twin]
			return nil
		}, true, 0, false, true, false, 0},
		{"two publications of the face: the row is listed once", func(pub *tracequery.Result) []types.ObservationRecord {
			return hmc377R15Records(*pub, "second")
		}, true, 3, true, false, true, 0},
		{"a board row without attribution (background channel)", func(pub *tracequery.Result) []types.ObservationRecord {
			pub.RootCauseRank.Items[twin].ChainRelevance, pub.RootCauseRank.Items[twin].Causality = "background", "background"
			return nil
		}, true, 1, false, true, false, 0},
		{"a row of the analysed thread's own state tier", func(pub *tracequery.Result) []types.ObservationRecord {
			pub.RootCauseRank.Items[twin].Tier = tracequery.RootCauseTierTargetSelfState
			return nil
		}, true, 1, false, true, false, 0},
		{"a lossless detail of a row that is not on this board", func(pub *tracequery.Result) []types.ObservationRecord {
			item := &pub.RootCauseRank.Items[twin]
			item.Tier, item.AbsorbedByRankFamily, item.AbsorbedIntoFamily = tracequery.RootCauseTierAbsorbed, true, "rank_pair:not-on-this-board"
			return nil
		}, true, 1, false, true, false, 0},
		{"a seat whose whole account a chain seat represents", func(pub *tracequery.Result) []types.ObservationRecord {
			pub.RootCauseRank.Items[twin].ChainAnchorRepresentedByChainSeat = true
			return nil
		}, true, 1, false, true, false, 0},
		{"a board row with an ordinal hands it over", func(pub *tracequery.Result) []types.ObservationRecord {
			pub.RootCauseRank.Items[twin].Rank, pub.RootCauseRank.Items[twin].Tier = 2, "secondary"
			return nil
		}, true, 1, false, true, true, 2},
		{"the twin first, then a view of the fact with an ordinal", func(pub *tracequery.Result) []types.ObservationRecord {
			seated := pub.RootCauseRank.Items[twin]
			seated.Rank, seated.Tier, seated.Type, seated.Source = 2, "secondary", "runnable_wait", "wakeup_chain.causal_impacts"
			pub.RootCauseRank.Items = append(pub.RootCauseRank.Items, seated)
			return nil
		}, true, 2, true, true, true, 2},
		{"no hop row: the twin keeps the fact and takes a second ordinal-less view of it", func(pub *tracequery.Result) []types.ObservationRecord {
			second := pub.RootCauseRank.Items[twin]
			second.Type, second.Source = "runnable_wait", "window_stats"
			pub.RootCauseRank.Items = append(pub.RootCauseRank.Items, second)
			pub.WakeupChain.CausalImpacts = pub.WakeupChain.CausalImpacts[:1]
			return nil
		}, false, 1, true, true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pub := hmc377R15Published(t)
			second := tc.edit(&pub)
			th := hmc377R15RunLedger(t, pub, append(hmc377R15Records(pub, "payload"), second...))
			if len(th.priced) != 1 || len(th.context) != 1 {
				t.Fatalf("precondition: the thread's rows are one priced row and its context row: priced %d context %d\n%s", len(th.priced), len(th.context), th.f.dump())
			}
			row := th.priced[0]
			if isHop := !strings.HasPrefix(row.Node.Predicate, "root_cause_"); isHop != tc.hop || len(row.Node.MergedEvidenceIDs) != tc.merged {
				t.Fatalf("precondition: hop row %t (want %t) with %d folded publications (want %d): %s %v\n%s",
					isHop, tc.hop, len(row.Node.MergedEvidenceIDs), tc.merged, row.Node.Predicate, row.Node.MergedEvidenceIDs, th.f.dump())
			}
			if carried := row.Node.SameFactRankSeat != nil; carried != tc.carried {
				t.Errorf("the row's node carries a board seat that had no ordinal: %t, want %t\n%s", carried, tc.carried, th.f.dump())
			}
			seat := runtimeTraceProjRowPublishedSeat(*row)
			if own := seat.EvidenceID == row.Node.EvidenceID && seat.Predicate == row.Node.Predicate; own != tc.ownSeat {
				t.Errorf("published seat is the row's own node: %t, want %t (seat %s %s)\n%s", own, tc.ownSeat, seat.Predicate, seat.EvidenceID, th.f.dump())
			}
			if row.Node.Rank != tc.rank {
				t.Errorf("ordinal %d, want %d\n%s", row.Node.Rank, tc.rank, th.f.dump())
			}
			_, listed := th.board[row.EvidenceTag]
			if listed != tc.listed || runtimeTraceProjElimEligible(*row) != tc.listed || (tc.listed && len(th.board) != 1) || (!tc.listed && len(th.board) != 0) {
				t.Errorf("◎ lists the row: %t, want %t; ◎ members of the thread %v\n%s", listed, tc.listed, th.board, th.f.dump())
			}
			if runtimeTraceProjElimEligible(*th.context[0]) {
				t.Errorf("the context row is a ◎ member\n%s", th.f.dump())
			}
		})
	}
}

// The other rows of the face keep their own account: a second withdrawn seat
// whose hop row carries no board seat stays off ◎ (its 0.500ms is listed once,
// by the ◇ seat that took it), and a standing seat of the thread is listed
// beside the hop row (the shapes of Y2roleWin_H and Y4seatVO_H).
func TestHMC377R15OverviewOnAHopFaceListsEachShareOnce(t *testing.T) {
	chainRow := func(tier string, rank int, kind string, ms float64, l0, l1 int, start float64) tracequery.RootCauseRankItem {
		return tracequery.RootCauseRankItem{Rank: rank, Tier: tier, Type: kind, Source: "wakeup_chain.causal_impacts", Thread: hmc377R9Dep,
			ImpactMs: ms, CumulativeImpactMs: ms, RunnableMs: ms, DominantState: "runnable",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.8, StartTs: start, EndTs: start + ms/1000, LineStart: l0, LineEnd: l1}
	}
	t.Run("a second context row with a plain hop row, and the ◇ seat that took its share", func(t *testing.T) {
		pub := hmc377R15Published(t)
		second := chainRow(tracequery.RootCauseTierContextOnly, 0, "unknown_state", 0.5, 21, 25, 10.3)
		second.MeasuredStateAccountKey = "state_account:v2:hmc377r15-6326-21"
		second.Summary = pub.RootCauseRank.Items[1].Summary
		taker := tracequery.RootCauseRankItem{Rank: 4, Tier: "tertiary", Type: "runnable_wait", Source: "window_stats", Thread: hmc377R9Dep,
			ImpactMs: 0.5, CumulativeImpactMs: 0.5, EffectiveImpactMs: 0.5, RunnableMs: 0.5, DominantState: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Confidence: 0.7, StartTs: 10.3, EndTs: 10.3005, LineStart: 60, LineEnd: 65}
		items := pub.RootCauseRank.Items
		pub.RootCauseRank.Items = []tracequery.RootCauseRankItem{items[0], items[1], second, taker, items[2]}
		pub.WakeupChain.CausalImpacts = append(pub.WakeupChain.CausalImpacts, hmc377R15Hop(second, second.MeasuredStateAccountKey))
		th := hmc377R15Run(t, pub)
		var carrier, plain *runtimeTraceProjTreeRow
		for _, row := range th.priced {
			if strings.HasPrefix(row.Node.Predicate, "root_cause_") {
				continue
			}
			if hmc377R15Near(row.Node.EffectiveImpactMS, 1.903) {
				carrier = row
			} else if hmc377R15Near(row.Node.EffectiveImpactMS, 0.5) {
				plain = row
			}
		}
		if carrier == nil || plain == nil || len(th.context) != 2 {
			t.Fatalf("precondition: two hop rows and two context rows of the thread: context %d\n%s", len(th.context), th.f.dump())
		}
		if _, listed := th.board[plain.EvidenceTag]; listed || runtimeTraceProjElimEligible(*plain) {
			t.Errorf("the hop row [%s] carries no board seat and is no ◎ member: %v\n%s", plain.EvidenceTag, th.board, th.f.dump())
		}
		if len(th.board) != 2 || !hmc377R15Near(th.board[carrier.EvidenceTag], 1.903) ||
			!hmc377R15Near(th.overview, 2.403) || !hmc377R15Near(th.native, 2.403) || !hmc377R15Near(th.llm, 2.403) {
			t.Errorf("the thread's 2.403ms (1.903 on [%s] and the ◇ seat's 0.500): ◎ %v = %.3f, rank board %.3f, summary %.3f\n%s",
				carrier.EvidenceTag, th.board, th.overview, th.native, th.llm, th.f.dump())
		}
		// Unpriced occupancy: the two context rows, the larger one named.
		if line := hmc377R15Line(th.f.elimZH, "未计价占用"); !strings.Contains(line, "⛓ 2 行(最大 1.903ms [") {
			t.Errorf("unpriced occupancy counts the two context rows: %q\n%s", line, th.f.elimZH)
		}
	})
	t.Run("a standing seat of the thread beside the hop row", func(t *testing.T) {
		pub := hmc377R15Published(t)
		seat := chainRow("secondary", 3, "runnable_wait", 1.0, 41, 45, 10.3)
		seat.EffectiveImpactMs = 1.0
		items := pub.RootCauseRank.Items
		pub.RootCauseRank.Items = []tracequery.RootCauseRankItem{items[0], items[1], seat, items[2]}
		th := hmc377R15Run(t, pub)
		var carrier *runtimeTraceProjTreeRow
		for _, row := range th.priced {
			if !strings.HasPrefix(row.Node.Predicate, "root_cause_") {
				carrier = row
			}
		}
		if carrier == nil || len(th.priced) != 2 || len(th.context) != 1 {
			t.Fatalf("precondition: the hop row, the standing seat and the context row: priced %d context %d\n%s", len(th.priced), len(th.context), th.f.dump())
		}
		if len(th.board) != 2 || !hmc377R15Near(th.board[carrier.EvidenceTag], 1.903) ||
			!hmc377R15Near(th.overview, 2.903) || !hmc377R15Near(th.native, 2.903) || !hmc377R15Near(th.llm, 2.903) {
			t.Errorf("the thread's 2.903ms (1.903 on [%s] and the seat's 1.000): ◎ %v = %.3f, rank board %.3f, summary %.3f\n%s",
				carrier.EvidenceTag, th.board, th.overview, th.native, th.llm, th.f.dump())
		}
		if line := hmc377R15Line(th.f.elimZH, "未计价占用"); !strings.Contains(line, "⛓ 1 行(最大 1.903ms ["+th.context[0].EvidenceTag+"])") {
			t.Errorf("unpriced occupancy counts the context row: %q\n%s", line, th.f.elimZH)
		}
	})
}
