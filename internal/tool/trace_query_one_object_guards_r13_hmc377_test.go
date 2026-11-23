package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 13 (R-377-13 ① and ⑦): the guards of the one-object rules.
// Every arm that decides whether two tree lines of one physical thread become
// one row, wear the mirror tag or wear the constituent pointer is pinned here
// by the shape that distinguishes it (lane evidence
// fix13/mirror/cont/mutation/: each mutant below fails its test).

// The typed join keeps the shared vetoes of the same-span fold: a seat ranked
// in another query window is another board's receipt and never rides the hop
// row, even on one account identity (the producer hash does not name the view
// window, so a wider rank query publishes the same account).
func TestHMC377R13TypedFoldKeepsACrossWindowSeatItsOwnRow(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	build := func(t *testing.T, seatStart, seatEnd float64) (runtimeTraceProjTreeModel, string) {
		hop, seat := r13Hop("r13-hop", key), r13Seat("r13-seat", key)
		hop.QueryWindowStartTs, hop.QueryWindowEndTs = 236715.408787, 236716.010733
		seat.QueryWindowStartTs, seat.QueryWindowEndTs = seatStart, seatEnd
		return hmc377R12Fence(t, r13Projection(hop, seat, r13Remainder()), true)
	}
	model, fence := build(t, 236715.408787, 236716.010733)
	if xlane2FindRowByEvidenceID(&model, "r13-seat") != nil {
		t.Fatalf("fixture: in one query window the pair is one row:\n%s", fence)
	}
	model, fence = build(t, 236715.108787, 236716.510733)
	if xlane2FindRowByEvidenceID(&model, "r13-seat") == nil || xlane2FindRowByEvidenceID(&model, "r13-hop") == nil {
		t.Fatalf("a seat ranked in another query window keeps its own row:\n%s", fence)
	}
}

// Two rank seats that show one account leave the join ambiguous: neither
// rides the hop row (fail open, the same-span fold's donor-conflict rule).
func TestHMC377R13TypedFoldFailsOpenOnTwoSeatsOfOneAccount(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	second := r13Seat("r13-seat-2", key)
	second.Rank, second.Tier, second.Predicate = 3, "tertiary", "root_cause_tertiary"
	second.LineStart, second.LineEnd = 113300, 115000
	model, fence := hmc377R12Fence(t, r13Projection(r13Hop("r13-hop", key), r13Seat("r13-seat", key), second, r13Remainder()), true)
	host := xlane2FindRowByEvidenceID(&model, "r13-hop")
	if host == nil || len(host.RankFoldPeers) != 0 || host.Node.Rank != 0 {
		t.Fatalf("two seats of one account: the hop row hosts neither: %+v\n%s", host, fence)
	}
}

// The seat's chain cumulative replaces the hop's only as the larger scope of
// the one fact: a seat whose cumulative is the smaller one never lowers the
// row's published cumulative.
func TestHMC377R13FoldedRowNeverLowersItsCumulative(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	seat := r13Seat("r13-seat", key)
	seat.CumulativeImpactMS = 8.1
	model, fence := hmc377R12Fence(t, r13Projection(r13Hop("r13-hop", key), seat, r13Remainder()), true)
	host := xlane2FindRowByEvidenceID(&model, "r13-hop")
	if host == nil || len(host.RankFoldPeers) != 1 || host.Node.CumulativeImpactMS != 8.294 {
		t.Fatalf("the one row keeps the larger cumulative 8.294: %+v\n%s", host, fence)
	}
}

// The one row is the seat's cause on the counted-share caliber too: the ready
// time it does not count is worded by the seat's typed kind (dio w38
// #tp-io-28-51675 at 532dc208d: seat 「全额计入已证明优先级低于其下游等待线程
// 的部分」), not by the generic fallback an untyped hop row would speak.
func TestHMC377R13FoldedRowSpeaksTheSeatsCountedShareCaliber(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	hop, seat := r13Hop("r13-hop", key), r13Seat("r13-seat", key)
	for _, node := range []*types.TraceCausalProjectionNode{&hop, &seat} {
		node.GatedRunnableMS, node.EffectiveImpactMS = 6.0, 6.033
	}
	seat.ImpactMS = 6.033
	for _, zh := range []bool{true, false} {
		model, fence := hmc377R12Fence(t, r13Projection(hop, seat, r13Remainder()), zh)
		host := xlane2FindRowByEvidenceID(&model, "r13-hop")
		if host == nil || len(host.RankFoldPeers) != 1 || host.Node.TypeToken != "priority_inversion_candidate" {
			t.Fatalf("the one row carries the seat's cause token pair: %+v\n%s", host, fence)
		}
		want := "未证明优先级更低,不计入"
		if !zh {
			want = "has no proven lower priority and is not counted"
		}
		text := hmc377R12Joined(fence) + "\n" + runtimeTraceProjDetailFullText(model, zh)
		if !strings.Contains(text, want) {
			t.Fatalf("the one row words its uncounted ready time by the seat's kind (%q):\n%s\n%s", want, fence, runtimeTraceProjDetailFullText(model, zh))
		}
	}
}

// The folded seat keeps its own caliber word on ◎. The rank lane publishes a
// seat's window projection as its counted value, so a seat that counts the
// proven part of a ready wait in full wears no caliber word there; on the one
// row the bar is the hop's raw wait, and the generic shown-when-different word
// is still told apart from the seat's own projection (pop w17m
// DetectViewRect-51191: at 532dc208d the seat's ◎ line 「0.480ms … 优先级反转
// 候选 ·交集证明」; on the one row it read 「·折算 ·交集证明」 beside tree lines
// saying 「runnable(全额) 0.480ms … 其余0.255ms未证明优先级更低,不计入」).
func TestHMC377R13FoldedSeatKeepsItsOwnBoardCaliber(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	board := func(t *testing.T, seatProjection float64, sameSpan, zh bool) (string, runtimeTraceProjMarkSet) {
		hop, seat := r13Hop("r13-hop", key), r13Seat("r13-seat", key)
		for _, node := range []*types.TraceCausalProjectionNode{&hop, &seat} {
			node.GatedRunnableMS, node.GatedRunningDeficitMS, node.EffectiveImpactMS = 6.0, 0, 6.0
		}
		seat.ImpactMS = seatProjection
		if sameSpan {
			seat.CumulativeImpactMS = hop.CumulativeImpactMS // equal accounts: the same-span fold takes the pair
		}
		model, fence := hmc377R12Fence(t, r13Projection(hop, seat, r13Remainder()), zh)
		host := xlane2FindRowByEvidenceID(&model, "r13-hop")
		if host == nil || len(host.RankFoldPeers) != 1 || host.Node.ImpactMS != 8.294 {
			t.Fatalf("fixture: the hop row hosts the seat and keeps its raw wait as its bar: %+v\n%s", host, fence)
		}
		var marks runtimeTraceProjMarkSet
		for _, entry := range runtimeTraceProjElimBoard(model) {
			if entry.row.Node.Subject == r13Thread && entry.row.Node.ChainRelevance == "on_chain" {
				return runtimeTraceProjElimRowLine(entry, entry.row.Node.EffectiveImpactMS, &marks, zh, false, false), marks
			}
		}
		t.Fatalf("the one row is a ◎ seat:\n%s", fence)
		return "", marks
	}
	for _, zh := range []bool{true, false} {
		lang, word := "zh", "折算"
		if !zh {
			lang, word = "en", "discounted"
		}
		t.Run(lang+"/a seat counted in full wears no caliber word", func(t *testing.T) {
			line, marks := board(t, 6.0, false, zh)
			if !strings.Contains(line, "6.000ms") || strings.Contains(line, word) || marks.seen[runtimeTraceProjMarkStanzaDiscount] {
				t.Fatalf("the seat stood on ◎ at 6.000ms with no %q; the one row must too: %q", word, line)
			}
		})
		t.Run(lang+"/a seat whose own projection differs keeps the word", func(t *testing.T) {
			line, marks := board(t, 7.5, false, zh)
			if !strings.Contains(line, word) || !marks.seen[runtimeTraceProjMarkStanzaDiscount] {
				t.Fatalf("the seat stood on ◎ with %q against its own 7.500ms projection; the one row must too: %q", word, line)
			}
		})
		// The same-span fold transfers annotation only, never an account: its
		// host keeps its 532dc208d form on ◎ (TestHMC377R13LegacyFoldKeepsItsForm
		// is the tree half of this boundary).
		t.Run(lang+"/the same-span fold keeps its form", func(t *testing.T) {
			line, marks := board(t, 6.0, true, zh)
			if !strings.Contains(line, word) || !marks.seen[runtimeTraceProjMarkStanzaDiscount] {
				t.Fatalf("the same-span host reads its own projection as at 532dc208d (%q): %q", word, line)
			}
		})
	}
}

func r13SemanticRecord(id string) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, EvidenceID: id, Subject: "tbs_basemodule_-58968",
		Predicate: "trace_semantic_span", Object: "class_verification", SemanticClass: "class_verification",
		SpanName:       "VerifyClass com.tencent.common.threadpool.debug.ThreadPoolSnapshot",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ImpactMS: 1.202, CumulativeImpactMS: 1.202,
		EffectiveImpactMS: 0.292, LineStart: 182417, LineEnd: 182530, Unit: "ms", Value: "1.202",
	}
}

func r13SemanticRemainder(id string) types.TraceCausalProjectionNode {
	record := r13SemanticRecord("")
	remainder := elimChainNode(id, record.Subject, "class_verification", "", 0, 0, record.LineStart)
	remainder.Predicate, remainder.Tier, remainder.Rank, remainder.LineEnd = "root_cause_context_only", types.TraceCausalTierContextOnly, 0, record.LineEnd
	remainder.SemanticClass, remainder.SpanName = record.SemanticClass, record.SpanName
	remainder.ChainRelevance, remainder.Causality, remainder.ChainDepth = "adjacent", "adjacent_to_wakeup_chain", 0
	remainder.ImpactMS, remainder.CumulativeImpactMS, remainder.EffectiveImpactMS = 0.910, 0.910, 0
	remainder.ChainAnchorRemainderSeat, remainder.ChainAnchoredMS, remainder.ChainAnchorFullMS = true, 0.292, 1.202
	return remainder
}

// Only the ◇ remainder half rides the span's record: another off-chain row of
// the same span (a context row that is not the remainder of a split), a
// remainder of another semantic class on the same lines, and a remainder
// whose span two records publish all keep their own rows.
func TestHMC377R13SemanticFoldTakesOnlyTheRemainderHalfOfItsOneRecord(t *testing.T) {
	semantic := func(t *testing.T, adjacent types.TraceCausalProjectionNode, records ...types.TraceCausalProjectionNode) (runtimeTraceProjTreeModel, string) {
		projection := r13Projection(adjacent)
		projection.SemanticSpans = records
		return hmc377R12Fence(t, projection, true)
	}
	model, fence := semantic(t, r13SemanticRemainder("r13-sem-rem"), r13SemanticRecord("r13-record"))
	if xlane2FindRowByEvidenceID(&model, "r13-sem-rem") != nil {
		t.Fatalf("fixture: the remainder rides its record:\n%s", fence)
	}
	t.Run("a context row that is no remainder", func(t *testing.T) {
		context := r13SemanticRemainder("r13-sem-ctx")
		context.ChainAnchorRemainderSeat, context.ChainAnchoredMS, context.ChainAnchorFullMS = false, 0, 0
		model, fence := semantic(t, context, r13SemanticRecord("r13-record"))
		if xlane2FindRowByEvidenceID(&model, "r13-sem-ctx") == nil {
			t.Fatalf("an off-chain row of the span that is not its remainder keeps its row:\n%s", fence)
		}
	})
	t.Run("another class on the same lines", func(t *testing.T) {
		other := r13SemanticRemainder("r13-sem-rem")
		other.Object, other.TypeToken, other.SemanticClass = "jit_compile", "jit_compile", "jit_compile"
		model, fence := semantic(t, other, r13SemanticRecord("r13-record"))
		if xlane2FindRowByEvidenceID(&model, "r13-sem-rem") == nil {
			t.Fatalf("a remainder of another semantic class keeps its row:\n%s", fence)
		}
	})
	t.Run("two records of the span", func(t *testing.T) {
		twin := r13SemanticRecord("r13-record-2")
		twin.ChainRelevance, twin.Causality = "background", "background"
		model, fence := semantic(t, r13SemanticRemainder("r13-sem-rem"), r13SemanticRecord("r13-record"), twin)
		if xlane2FindRowByEvidenceID(&model, "r13-record") == nil || xlane2FindRowByEvidenceID(&model, "r13-record-2") == nil {
			t.Fatalf("fixture: both records are drawn:\n%s", fence)
		}
		if xlane2FindRowByEvidenceID(&model, "r13-sem-rem") == nil {
			t.Fatalf("a remainder whose span two records publish keeps its row:\n%s", fence)
		}
	})
}

// The aggregate-lane row of a multi-occurrence account already wears the
// value-mirror tag naming the ×N row of its occurrences (the tieba E6/E18
// form): that one-way form stays — the measured-account mirror does not add a
// second tag on the ×N row.
func TestHMC377R13MirrorLeavesTheAggregateTwinFormAlone(t *testing.T) {
	const group = "state_account:v2:r13-cookie-aggregate"
	projection := types.TraceCausalProjection{
		WakeupPath: []string{"waker-1", "tieba-1"}, WindowStartTs: 34579.472865, WindowEndTs: 34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "r13-fold",
				Subject: "cookiemonster-1", Object: "runnable", StateKind: "runnable", Predicate: "wakeup_causal_impact",
				ImpactMS: 23.748, CumulativeImpactMS: 23.748, EffectiveImpactMS: 8.307, ChainDepth: 1,
				MergedCount: 5, MergedMinMS: 1.661, MergedMaxMS: 8.307, MeasuredOccurrenceGroupKey: group,
				QueryWindowStartTs: 34579.473, QueryWindowEndTs: 34579.572,
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.78, LineStart: 7705, LineEnd: 8547},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "r13-agg",
				Subject: "cookiemonster-1", Object: "runnable_wait", StateKind: "runnable", Predicate: "wakeup_causal_aggregate",
				ImpactMS: 23.748, CumulativeImpactMS: 23.748, EffectiveImpactMS: 23.748, RunnableMS: 23.748,
				MeasuredStateAccountKey: group, QueryWindowStartTs: 34579.473, QueryWindowEndTs: 34579.572,
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.80, LineStart: 2892, LineEnd: 13060},
		},
	}
	model, fence := hmc377R12Fence(t, projection, true)
	fold, aggregate := xlane2FindRowByEvidenceID(&model, "r13-fold"), xlane2FindRowByEvidenceID(&model, "r13-agg")
	if fold == nil || aggregate == nil || !types.TraceMeasuredStateFactSame(fold.Node, aggregate.Node) {
		t.Fatalf("fixture: the ×N row and the aggregate row show one account: %+v %+v\n%s", fold, aggregate, fence)
	}
	if aggregate.ValueMirrorRef != fold.EvidenceTag || fold.ValueMirrorRef != "" {
		t.Fatalf("the aggregate twin form stays one-way: aggregate→%q fold→%q\n%s", aggregate.ValueMirrorRef, fold.ValueMirrorRef, fence)
	}
}

func r13AggregatePair() (types.TraceCausalProjectionNode, types.TraceCausalProjectionNode) {
	const group = "state_account:v2:r13-7973-aggregate"
	seat := elimChainNode("r13-agg", "android.bg-7973", "priority_inversion_candidate", "runnable", 2, 40.804, 161482)
	seat.LineEnd = 217955
	seat.MeasuredStateAccountKey = group
	hop := r13Hop("r13-occ", "state_account:v2:r13-7973-occurrence")
	hop.Subject = "android.bg-7973"
	hop.LineStart, hop.LineEnd = 161482, 162432
	hop.ImpactMS, hop.CumulativeImpactMS, hop.EffectiveImpactMS = 5.938, 5.938, 5.938
	hop.MeasuredOccurrenceGroupKey = group
	return seat, hop
}

// The constituent pointer names a rank seat of the occurrence's own board
// that counts the account: a seat ranked in another query window, the
// demoted clone of a gated-share split and the account's own aggregate-lane
// row are not that seat.
func TestHMC377R13OccurrencePointerNeedsACountingSeatOfItsBoard(t *testing.T) {
	pointer := func(t *testing.T, nodes ...types.TraceCausalProjectionNode) (string, string) {
		model, fence := hmc377R12Fence(t, r13Projection(nodes...), true)
		row := xlane2FindRowByEvidenceID(&model, "r13-occ")
		if row == nil || xlane2FindRowByEvidenceID(&model, "r13-agg") == nil {
			t.Fatalf("fixture: the occurrence row and the seat are drawn:\n%s", fence)
		}
		return row.AggregateSeatRef, fence
	}
	seat, hop := r13AggregatePair()
	if ref, fence := pointer(t, seat, hop); ref == "" {
		t.Fatalf("fixture: the occurrence points at its aggregate seat:\n%s", fence)
	}
	t.Run("a seat of another query window", func(t *testing.T) {
		seat, hop := r13AggregatePair()
		hop.QueryWindowStartTs, hop.QueryWindowEndTs = 236715.408787, 236716.010733
		seat.QueryWindowStartTs, seat.QueryWindowEndTs = 236715.108787, 236716.510733
		if ref, fence := pointer(t, seat, hop); ref != "" {
			t.Fatalf("a seat ranked in another query window is not named (%q):\n%s", ref, fence)
		}
	})
	t.Run("the demoted clone of a gated-share split", func(t *testing.T) {
		seat, hop := r13AggregatePair()
		seat.GatedShareConstituentSeat = true
		if ref, fence := pointer(t, seat, hop); ref != "" {
			t.Fatalf("a constituent seat counts nothing and is not named (%q):\n%s", ref, fence)
		}
	})
	t.Run("the aggregate-lane row beside the seat", func(t *testing.T) {
		seat, hop := r13AggregatePair()
		lane := hop
		lane.EvidenceID, lane.Predicate, lane.Object = "r13-lane", "wakeup_causal_aggregate", "runnable_wait"
		lane.StateAccountKey, lane.MeasuredOccurrenceGroupKey, lane.MeasuredStateAccountKey = "", "", seat.MeasuredStateAccountKey
		lane.ImpactMS, lane.CumulativeImpactMS, lane.EffectiveImpactMS, lane.RunnableMS = 40.804, 40.804, 40.804, 40.804
		lane.LineStart, lane.LineEnd = 161482, 217955
		model, fence := hmc377R12Fence(t, r13Projection(seat, hop, lane), true)
		row, agg := xlane2FindRowByEvidenceID(&model, "r13-occ"), xlane2FindRowByEvidenceID(&model, "r13-agg")
		if row == nil || agg == nil || xlane2FindRowByEvidenceID(&model, "r13-lane") == nil {
			t.Fatalf("fixture: the occurrence, the seat and the aggregate-lane row are drawn:\n%s", fence)
		}
		if row.AggregateSeatRef != agg.EvidenceTag {
			t.Fatalf("the rank seat is the one counting seat (%q):\n%s", row.AggregateSeatRef, fence)
		}
	})
}

// A ×N fold of an account's occurrences shows the account itself, so it names
// no occurrence group (types.MeasuredOccurrenceGroup) and wears no constituent
// pointer on its identity: beside a seat the seat/occurrence census does not
// pair it with (the census needs a seat of the chain), the ×N row stays plain.
func TestHMC377R13FoldOfOccurrencesWearsNoConstituentPointer(t *testing.T) {
	seat, hop := r13AggregatePair()
	seat.ChainDepth = 0
	model, fence := hmc377R12Fence(t, r13Projection(seat, hop), true)
	if row := xlane2FindRowByEvidenceID(&model, "r13-occ"); row == nil || row.AggregateSeatRef == "" {
		t.Fatalf("fixture: one occurrence points at the seat of its account:\n%s", fence)
	}
	hop.MergedCount, hop.MergedMinMS, hop.MergedMaxMS = 2, 2.0, 3.938
	model, fence = hmc377R12Fence(t, r13Projection(seat, hop), true)
	row := xlane2FindRowByEvidenceID(&model, "r13-occ")
	if row == nil || xlane2FindRowByEvidenceID(&model, "r13-agg") == nil {
		t.Fatalf("fixture: the ×N row and the seat are drawn:\n%s", fence)
	}
	if row.AggregateSeatRef != "" {
		t.Fatalf("a ×N row wears no constituent pointer on its identity (%q):\n%s", row.AggregateSeatRef, fence)
	}
}
