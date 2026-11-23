package tool

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 13 (batch-owner ruling R-377-13 ①, 2026-11-22): one object per
// physical thread on the tree. Real windows at 532dc208d (review-population
// logs/final/mainclass.txt, evidence/D6_preexisting_batch_tree.txt; lane
// evidence fix13/mirror/evidence/pairs_532.txt):
//   - a chain hop row and the inversion rank seat that shows exactly the
//     hop's measured account were two additive rows, e.g. w07
//     binder:11043_3-59698 ⇅ 8.294 [E13(+1)] beside ➋ ⇅ 8.327 [E30]
//     (runnable 8.294 + 8.294 + ◇ 118.784 against raw 127.078);
//   - a case-B clipped ready-time seat priced on its hop's segments stood
//     beside the hop with no disclosure, e.g. w19 dtmp.dispatch-59777
//     ⇅ 4.692 [E19(+1)] beside ⧖ 4.692 [E32] (raw 5.074);
//   - an occurrence row of a multi-occurrence inversion account stood beside
//     its aggregate seat when the sibling occurrence was drawn only inside an
//     overflow fold row (w42 android.bg-7973 5.938 beside 40.804);
//   - the ◇ remainder half of a semantic span's 同源二分 stood beside the
//     span's own ✦ record and repeated the anchored part (w22m
//     tbs_basemodule_-58968 ✦ 1.202 beside ◇ 0.292).

const r13Thread = "binder:11043_3-59698"

func r13Hop(id, key string) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: id, Subject: r13Thread,
		Predicate: "wakeup_causal_impact", Object: "runnable", StateKind: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1, ChainBranch: 3,
		ImpactMS: 8.294, CumulativeImpactMS: 8.294, EffectiveImpactMS: 8.327, EffectiveImpactPublished: true,
		PriorityInversionCandidate: true, GatedRunnableMS: 8.294, GatedRunningDeficitMS: 0.033,
		RunnableMS: 8.294, RunningMS: 0.166, StateAccountKey: key, StateAccountComplete: true,
		LineStart: 113278, LineEnd: 115050, StartTs: 236715.517319, EndTs: 236715.525779,
		Confidence: 0.78, Unit: "ms", Value: "8.294",
	}
}

func r13Seat(id, key string) types.TraceCausalProjectionNode {
	node := elimChainNode(id, r13Thread, "priority_inversion_candidate", "runnable", 2, 8.327, 113278)
	node.LineEnd = 115050
	node.ImpactMS, node.CumulativeImpactMS = 8.327, 8.46
	node.ChainBranch, node.StartTs, node.EndTs = 3, 236715.517319, 236715.525779
	node.GatedRunnableMS, node.GatedRunningDeficitMS, node.RunnableMS, node.RunningMS = 8.294, 0.033, 8.294, 0.166
	node.ChainAnchoredMS, node.ChainAnchorFullMS = 8.294, 127.078
	node.MeasuredStateAccountKey = key
	node.FixDirection, node.Confidence = "lock_priority", 0.91
	node.ShareClaims = []types.TraceCausalProjectionShareClaim{{Part: "remainder", Role: "remainder_seat", ShareMS: 118.784, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r13-rem"}}}
	return node
}

func r13Remainder() types.TraceCausalProjectionNode {
	node := elimChainNode("r13-rem", r13Thread, "runnable_wait", "runnable", 1, 118.784, 89488)
	node.Predicate, node.Tier, node.LineEnd = "root_cause_adjacent", "tertiary", 145810
	node.ChainRelevance, node.Causality, node.ChainDepth = "adjacent", "adjacent_to_wakeup_chain", 0
	node.ChainAnchorRemainderSeat, node.ChainAnchoredMS, node.ChainAnchorFullMS = true, 8.294, 127.078
	node.RunnableMS = 118.784
	node.ShareClaims = []types.TraceCausalProjectionShareClaim{{Part: "anchored", Role: "chain_seat", ShareMS: 8.294, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r13-seat"}}}
	return node
}

func r13Projection(nodes ...types.TraceCausalProjectionNode) types.TraceCausalProjection {
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{"surfaceflinger-700", ".ugc.aweme.lite-59591"},
		WindowStartTs: 236715.408787, WindowEndTs: 236716.010733,
	}
	for _, node := range nodes {
		if node.ChainRelevance == "adjacent" {
			projection.AdjacentCauses = append(projection.AdjacentCauses, node)
			continue
		}
		projection.OnChainCauses = append(projection.OnChainCauses, node)
	}
	return projection
}

// r13HeadLines returns the tree headlines of subject's thread (matched by its
// tid suffix: a long name is mid-truncated on the row).
func r13HeadLines(fence, subject string) []string {
	tid := subject[strings.LastIndex(subject, "-"):]
	var out []string
	for _, l := range strings.Split(fence, "\n") {
		if strings.Contains(l, tid+" ·") && regexp.MustCompile(`[0-9]+\.[0-9]{3}ms`).MatchString(l) && strings.Contains(l, "[E") {
			out = append(out, l)
		}
	}
	return out
}

// R-377-13 ①: the chain hop row and the rank seat that shows exactly the
// hop's measured account are one row on the tree — the hop row keeps its tree
// position and its accounts, the seat's rank (and badge), its split sentence
// and its [E#] travel with it, and its ◇ remainder names the one row.
func TestHMC377R13HopAndItsRankSeatAreOneRow(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	projection := r13Projection(r13Hop("r13-hop", key), r13Seat("r13-seat", key), r13Remainder())
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		model, fence := hmc377R12Fence(t, projection, zh)
		t.Run(name+"/one chain row", func(t *testing.T) {
			if row := xlane2FindRowByEvidenceID(&model, "r13-seat"); row != nil {
				t.Fatalf("the rank seat must not be a second tree row:\n%s", fence)
			}
			host := xlane2FindRowByEvidenceID(&model, "r13-hop")
			if host == nil || len(host.RankFoldPeers) != 1 || host.Node.Rank != 2 || host.Badge != 2 {
				t.Fatalf("the hop row hosts the seat with its rank and badge: %+v\n%s", host, fence)
			}
			if host.Node.Object != "priority_inversion_candidate" || host.Node.StateKind != "runnable" {
				t.Fatalf("the one row names the seat's cause and keeps the hop's state: %q %q", host.Node.Object, host.Node.StateKind)
			}
			heads := r13HeadLines(fence, r13Thread)
			if len(heads) != 2 { // the chain row and the ◇ remainder
				t.Fatalf("one chain row and one ◇ remainder expected, got %d:\n%s", len(heads), fence)
			}
			seatTag := host.RankFoldPeers[0].EvidenceTag
			if !strings.Contains(heads[0], "➋") || !strings.Contains(heads[0], "["+host.EvidenceTag+"+"+seatTag+"]") {
				t.Fatalf("the one row wears the seat's badge and its [E#]: %q", heads[0])
			}
			want := "同源二分:全窗127.078ms=本行锚定8.294ms+其余118.784ms"
			if !zh {
				want = "same-source split: full-window 127.078ms=this row 8.294ms anchored + remainder 118.784ms"
			}
			if !strings.Contains(hmc377R12Joined(fence), want) {
				t.Fatalf("the seat's split sentence stays on the one row (%q):\n%s", want, fence)
			}
			cum := "链上累计8.460ms"
			if !zh {
				cum = "chain cum 8.460ms"
			}
			if !strings.Contains(hmc377R12Joined(fence), cum) {
				t.Fatalf("the seat's chain cumulative stays on the one row (%q):\n%s", cum, fence)
			}
		})
		t.Run(name+"/the remainder names the one row", func(t *testing.T) {
			rem := xlane2FindRowByEvidenceID(&model, "r13-rem")
			host := xlane2FindRowByEvidenceID(&model, "r13-hop")
			if rem == nil || host == nil || rem.AccountRelRef == "" || host.AccountRelRef != strings.TrimSpace(rem.EvidenceTag) {
				t.Fatalf("the ◇ remainder pairs with the one row: rem=%+v host=%+v", rem, host)
			}
		})
		t.Run(name+"/◎ keeps one seat at its value", func(t *testing.T) {
			var seats []runtimeTraceProjElimEntry
			for _, entry := range runtimeTraceProjElimBoard(model) {
				if entry.row.Node.Subject == r13Thread && entry.row.Node.ChainRelevance == "on_chain" {
					seats = append(seats, entry)
				}
			}
			if len(seats) != 1 || seats[0].row.Node.EffectiveImpactMS != 8.327 {
				t.Fatalf("◎ keeps exactly the one chain seat at 8.327: %+v", seats)
			}
		})
	}
}

// The typed fold needs the typed identity: a seat that shows another account
// of the same thread stays its own row.
func TestHMC377R13RankSeatOfAnotherAccountStaysItsOwnRow(t *testing.T) {
	projection := r13Projection(r13Hop("r13-hop", "state_account:v2:a"), r13Seat("r13-seat", "state_account:v2:b"), r13Remainder())
	model, fence := hmc377R12Fence(t, projection, true)
	if xlane2FindRowByEvidenceID(&model, "r13-seat") == nil {
		t.Fatalf("a seat of another account keeps its own row:\n%s", fence)
	}
}

// A trunk thread's rows stay under the trunk head that draws them (the
// protected Honeybird [11343.248,11343.310) TID 64156 face: the drill row of
// Signal Catcher-64176 and its 构成 child); a seated host keeps its own seat.
func TestHMC377R13TypedFoldLeavesTrunkThreadsAndSeatedHosts(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	t.Run("trunk thread", func(t *testing.T) {
		projection := r13Projection(r13Hop("r13-hop", key), r13Seat("r13-seat", key), r13Remainder())
		projection.WakeupPath, projection.WakeupPathBranch = []string{r13Thread, ".ugc.aweme.lite-59591"}, 3
		model, fence := hmc377R12Fence(t, projection, true)
		if xlane2FindRowByEvidenceID(&model, "r13-seat") == nil {
			t.Fatalf("a trunk thread's seat stays under its trunk head:\n%s", fence)
		}
		// The same thread off the elected trunk domain is a flat row: one row.
		projection.WakeupPathBranch = 4
		model, fence = hmc377R12Fence(t, projection, true)
		if xlane2FindRowByEvidenceID(&model, "r13-seat") != nil {
			t.Fatalf("off the trunk domain the pair is one row:\n%s", fence)
		}
	})
	t.Run("seated host", func(t *testing.T) {
		hop := r13Hop("r13-hop", key)
		hop.Rank, hop.Tier = 5, "tertiary"
		model, fence := hmc377R12Fence(t, r13Projection(hop, r13Seat("r13-seat", key), r13Remainder()), true)
		if xlane2FindRowByEvidenceID(&model, "r13-seat") == nil {
			t.Fatalf("a seated host keeps the other seat as its own row:\n%s", fence)
		}
	})
}

// The legacy same-span fold (equal effective and cumulative accounts) keeps
// its 532dc208d form: the host does not take the seat's account disclosure.
// Taking it is the R-377-13 ① form, but it moves the protected NetEase reboot5
// [153.763595,153.887340) TID 11077 face (HeapTaskDaemon-11098 E4(+1),
// wk:1/1/0/8-6934 E5(+3)+E26 grow their 同源二分 sentences) — stopped and
// reported (fix13/mirror/evidence/stopped_protected/).
func TestHMC377R13LegacyFoldKeepsItsForm(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	seat := r13Seat("r13-seat", key)
	seat.CumulativeImpactMS = 8.294
	hop := r13Hop("r13-hop", key)
	hop.EffectiveImpactMS = seat.EffectiveImpactMS
	model, fence := hmc377R12Fence(t, r13Projection(hop, seat, r13Remainder()), true)
	host := xlane2FindRowByEvidenceID(&model, "r13-hop")
	if host == nil || len(host.RankFoldPeers) != 1 {
		t.Fatalf("the legacy pair folds:\n%s", fence)
	}
	if host.Node.ChainAnchorFullMS != 0 || strings.Contains(hmc377R12Joined(fence), "本行锚定8.294ms") {
		t.Fatalf("the legacy fold keeps its form (no adopted split sentence):\n%s", fence)
	}
}

func r13CensusSeat(id, key string) types.TraceCausalProjectionNode {
	node := elimChainNode(id, "dtmp.dispatch-59777", "runnable_wait", "runnable", 11, 4.692, 105378)
	node.LineEnd = 112045
	node.ChainDepth, node.RunnableMS = 0, 4.692
	node.ChainAnchoredMS, node.ChainAnchorFullMS = 4.692, 5.074
	node.MeasuredStateAccountKey = key
	node.FixDirection = "scheduling_supply"
	return node
}

func r13PricedHop(id, key string) types.TraceCausalProjectionNode {
	hop := r13Hop(id, key)
	hop.Subject = "dtmp.dispatch-59777"
	hop.ImpactMS, hop.CumulativeImpactMS, hop.EffectiveImpactMS = 4.692, 4.692, 0.343
	hop.GatedRunnableMS, hop.GatedRunningDeficitMS, hop.RunnableMS, hop.RunningMS = 0, 0.343, 4.692, 0.493
	hop.LineStart, hop.LineEnd = 105378, 105679
	return hop
}

// R-377-13 ①: a census seat priced on its own ruler (the case-B clipped
// ready-time seat, 调度供给) that shows exactly its chain hop's measured
// account stays its own row beside the hop priced on the chain ruler
// (inversion candidacy, 锁与优先级) — both prices and both must-not-lose
// classes stay drawn — and each line wears the existing typed non-additive
// disclosure naming the other.
func TestHMC377R13TwoRulersOnOneAccountWearTheMirrorTag(t *testing.T) {
	const key = "state_account:v2:r13-59777"
	projection := r13Projection(r13PricedHop("r13-hop", key), r13CensusSeat("r13-census", key))
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		model, fence := hmc377R12Fence(t, projection, zh)
		t.Run(name, func(t *testing.T) {
			hop, seat := xlane2FindRowByEvidenceID(&model, "r13-hop"), xlane2FindRowByEvidenceID(&model, "r13-census")
			if hop == nil || seat == nil {
				t.Fatalf("both rulers stay drawn:\n%s", fence)
			}
			for _, pair := range [][2]*runtimeTraceProjTreeRow{{hop, seat}, {seat, hop}} {
				want := "同段镜像·与[" + pair[1].EvidenceTag + "]同一物理时间,不可相加"
				if !zh {
					want = "same-seg mirror · same physical time as [" + pair[1].EvidenceTag + "], never additive"
				}
				if pair[0].ValueMirrorRef != pair[1].EvidenceTag || !strings.Contains(hmc377R12Joined(fence), want) {
					t.Fatalf("%s must wear %q:\n%s", pair[0].EvidenceTag, want, fence)
				}
			}
		})
	}
	t.Run("another account wears nothing", func(t *testing.T) {
		model, _ := hmc377R12Fence(t, r13Projection(r13PricedHop("r13-hop", key), r13CensusSeat("r13-census", "state_account:v2:other")), true)
		if row := xlane2FindRowByEvidenceID(&model, "r13-census"); row == nil || row.ValueMirrorRef != "" {
			t.Fatalf("a seat of another account is not a mirror: %+v", row)
		}
	})
	t.Run("a trunk head and its makeup child wear nothing", func(t *testing.T) {
		seat := r13CensusSeat("r13-census", key)
		seat.ChainBranch = 3
		projection := r13Projection(r13PricedHop("r13-hop", key), seat)
		projection.WakeupPath, projection.WakeupPathBranch = []string{"dtmp.dispatch-59777", ".ugc.aweme.lite-59591"}, 3
		model, fence := hmc377R12Fence(t, projection, true)
		hop, child := xlane2FindRowByEvidenceID(&model, "r13-hop"), xlane2FindRowByEvidenceID(&model, "r13-census")
		if hop == nil || child == nil || hop.Kind != runtimeTraceProjTreeRowChain || child.Kind != runtimeTraceProjTreeRowCause {
			t.Fatalf("fixture: the trunk head and its makeup child: %+v %+v\n%s", hop, child, fence)
		}
		if hop.ValueMirrorRef != "" || child.ValueMirrorRef != "" {
			t.Fatalf("a trunk head and its makeup child stay under the head:\n%s", fence)
		}
	})
}

// R-377-13 ①: an occurrence row whose occurrence-group identity is the
// measured account of the thread's aggregate seat is that seat's constituent,
// even when the sibling occurrence is drawn only inside an overflow fold.
func TestHMC377R13OccurrenceRowPointsAtItsAggregateSeat(t *testing.T) {
	const group = "state_account:v2:r13-7973-aggregate"
	seat := elimChainNode("r13-agg", "android.bg-7973", "priority_inversion_candidate", "runnable", 2, 40.804, 161482)
	seat.LineEnd = 217955
	seat.MeasuredStateAccountKey = group
	hop := r13Hop("r13-occ", "state_account:v2:r13-7973-occurrence")
	hop.Subject = "android.bg-7973"
	hop.LineStart, hop.LineEnd = 161482, 162432
	hop.ImpactMS, hop.CumulativeImpactMS, hop.EffectiveImpactMS = 5.938, 5.938, 5.938
	hop.MeasuredOccurrenceGroupKey = group
	model, fence := hmc377R12Fence(t, r13Projection(seat, hop), true)
	occ, agg := xlane2FindRowByEvidenceID(&model, "r13-occ"), xlane2FindRowByEvidenceID(&model, "r13-agg")
	if occ == nil || agg == nil || occ.AggregateSeatRef != agg.EvidenceTag {
		t.Fatalf("the occurrence row points at its aggregate seat: %+v\n%s", occ, fence)
	}
	if !strings.Contains(hmc377R12Joined(fence), "归因已计入["+agg.EvidenceTag+"](聚合席),本行为构成段,不另计") {
		t.Fatalf("the occurrence row wears the constituent pointer:\n%s", fence)
	}
	other := hop
	other.MeasuredOccurrenceGroupKey = "state_account:v2:other"
	model, _ = hmc377R12Fence(t, r13Projection(seat, other), true)
	if row := xlane2FindRowByEvidenceID(&model, "r13-occ"); row == nil || row.AggregateSeatRef != "" {
		t.Fatalf("an occurrence of another account points nowhere: %+v", row)
	}
	// Two drawn seats of the thread show the one account: which seat the
	// occurrence is part of is ambiguous, so the pointer fails open.
	twin := elimChainNode("r13-agg-twin", "android.bg-7973", "priority_inversion_candidate", "runnable", 3, 12.5, 230000)
	twin.MeasuredStateAccountKey = group
	model, fence = hmc377R12Fence(t, r13Projection(seat, twin, hop), true)
	if xlane2FindRowByEvidenceID(&model, "r13-agg-twin") == nil || xlane2FindRowByEvidenceID(&model, "r13-agg") == nil {
		t.Fatalf("both seats are drawn:\n%s", fence)
	}
	if row := xlane2FindRowByEvidenceID(&model, "r13-occ"); row == nil || row.AggregateSeatRef != "" {
		t.Fatalf("an occurrence whose account two drawn seats show points nowhere: %+v", row)
	}
}

// R-377-13 ①: the ◇ remainder half of a semantic span's 同源二分 rides the
// span's own ✦ record row, which draws the span's whole in-window wall.
func TestHMC377R13SemanticRemainderRidesItsSpanRecord(t *testing.T) {
	const subject = "tbs_basemodule_-58968"
	span := "VerifyClass com.tencent.common.threadpool.debug.ThreadPoolSnapshot"
	record := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "r13-record", Subject: subject,
		Predicate: "trace_semantic_span", Object: "class_verification", SemanticClass: "class_verification", SpanName: span,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ImpactMS: 1.202, CumulativeImpactMS: 1.202,
		EffectiveImpactMS: 0.292, LineStart: 182417, LineEnd: 182530, Unit: "ms", Value: "1.202",
	}
	remainder := elimChainNode("r13-sem-rem", subject, "class_verification", "", 0, 0, 182417)
	remainder.Predicate, remainder.Tier, remainder.Rank, remainder.LineEnd = "root_cause_context_only", types.TraceCausalTierContextOnly, 0, 182530
	remainder.Object, remainder.SemanticClass, remainder.SpanName = "class_verification", "class_verification", span
	remainder.ChainRelevance, remainder.Causality, remainder.ChainDepth = "adjacent", "adjacent_to_wakeup_chain", 0
	remainder.ImpactMS, remainder.CumulativeImpactMS, remainder.EffectiveImpactMS = 0.910, 0.910, 0
	remainder.ChainAnchorRemainderSeat, remainder.ChainAnchoredMS, remainder.ChainAnchorFullMS = true, 0.292, 1.202
	projection := r13Projection(remainder)
	projection.SemanticSpans = []types.TraceCausalProjectionNode{record}
	model, fence := hmc377R12Fence(t, projection, true)
	if row := xlane2FindRowByEvidenceID(&model, "r13-sem-rem"); row != nil {
		t.Fatalf("the semantic remainder must not be a second row beside its span record:\n%s", fence)
	}
	host := xlane2FindRowByEvidenceID(&model, "r13-record")
	if host == nil || len(host.RankFoldPeers) != 1 || !strings.Contains(fence, "+"+host.RankFoldPeers[0].EvidenceTag+"]") {
		t.Fatalf("the remainder rides the record's bracket: %+v\n%s", host, fence)
	}
	other := remainder
	other.LineStart = 182000
	projection.AdjacentCauses = []types.TraceCausalProjectionNode{other}
	model, _ = hmc377R12Fence(t, projection, true)
	if xlane2FindRowByEvidenceID(&model, "r13-sem-rem") == nil {
		t.Fatal("a remainder of another span keeps its own row")
	}
	// A record that is itself the ⛓ half (it carries the split and draws the
	// anchored share) keeps its additive ◇ remainder (the R3 straddle form).
	half := record
	half.ImpactMS, half.CumulativeImpactMS, half.ChainAnchoredMS, half.ChainAnchorFullMS = 0.292, 0.292, 0.292, 1.202
	projection.AdjacentCauses = []types.TraceCausalProjectionNode{remainder}
	projection.SemanticSpans = []types.TraceCausalProjectionNode{half}
	model, _ = hmc377R12Fence(t, projection, true)
	if xlane2FindRowByEvidenceID(&model, "r13-sem-rem") == nil {
		t.Fatal("the remainder of a record that is the ⛓ half keeps its row")
	}
}

// The seat's family key travels with it: the lossless detail rows the engine
// absorbed into the seat (the R-377-4 inversion window row) stay hosted by
// the one row.
func TestHMC377R13FoldedSeatKeepsItsAbsorbedDetailRows(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	seat := r13Seat("r13-seat", key)
	seat.RankFamilyKey = "rank_pair:priority_inversion_candidate>priority_inversion_runnable_wait|pid:59698"
	detail := elimChainNode("r13-detail", r13Thread, "priority_inversion_runnable_wait", "runnable", 0, 0.388, 89488)
	detail.AbsorbedByRankFamily, detail.AbsorbedInto = true, seat.RankFamilyKey
	projection := r13Projection(r13Hop("r13-hop", key), seat, r13Remainder())
	projection.AbsorbedChainRows = []types.TraceCausalProjectionNode{detail}
	model, fence := hmc377R12Fence(t, projection, true)
	host := xlane2FindRowByEvidenceID(&model, "r13-hop")
	if host == nil || len(host.RankFoldPeers) != 1 || len(host.AbsorbedChainPeers) != 1 {
		t.Fatalf("the one row hosts the seat's absorbed detail: %+v\n%s", host, fence)
	}
}

// The mirror tag names one other line: when three lines of one thread show one
// account, no line can name "the other" and none is tagged (fail open).
func TestHMC377R13MirrorNeedsExactlyTwoLines(t *testing.T) {
	const key = "state_account:v2:r13-59777"
	third := r13CensusSeat("r13-census-2", key)
	third.LineStart, third.LineEnd = 120000, 120400
	model, fence := hmc377R12Fence(t, r13Projection(r13PricedHop("r13-hop", key), r13CensusSeat("r13-census", key), third), true)
	for _, id := range []string{"r13-hop", "r13-census", "r13-census-2"} {
		if row := xlane2FindRowByEvidenceID(&model, id); row == nil || row.ValueMirrorRef != "" {
			t.Fatalf("three lines of one account wear no mirror tag (%s):\n%s", id, fence)
		}
	}
}

// The mirror tag needs the same shown occupancy too: a seat that shows another
// number of the same account (priced lower) is not told it repeats it.
func TestHMC377R13MirrorNeedsTheSameShownOccupancy(t *testing.T) {
	const key = "state_account:v2:r13-59777"
	seat := r13CensusSeat("r13-census", key)
	seat.RunnableMS = 4.0
	model, fence := hmc377R12Fence(t, r13Projection(r13PricedHop("r13-hop", key), seat), true)
	if row := xlane2FindRowByEvidenceID(&model, "r13-census"); row == nil || row.ValueMirrorRef != "" {
		t.Fatalf("another shown occupancy is not a mirror:\n%s", fence)
	}
}

// R-377-13 ①: the one row that draws the lead seat's account as its own keeps
// what the seat's detail block said about the thread — its process
// attribution and its causal position as the primary root cause (dio w21
// binder:14799_3-57612 at 532dc208d: seat [E8] 「进程: tgid=57534
// comm=unyuan.app.chat」「因果位置: 主根因(优先处理)」 beside its hop [E11(+1)]).
func TestHMC377R13FoldedLeadSeatKeepsItsPositionAndProcess(t *testing.T) {
	const key = "state_account:v2:r13-57612"
	seat := r13Seat("r13-seat", key)
	seat.Rank, seat.Predicate, seat.Tier, seat.Role = 1, "root_cause_primary", "primary", types.TraceCausalRolePrimaryRootCause
	seat.ProcessTGID, seat.ProcessComm = 57534, "unyuan.app.chat"
	projection := r13Projection(r13Hop("r13-hop", key), r13Remainder())
	projection.PrimaryRootCauses = []types.TraceCausalProjectionNode{seat}
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		t.Run(name, func(t *testing.T) {
			model, fence := hmc377R12Fence(t, projection, zh)
			host := xlane2FindRowByEvidenceID(&model, "r13-hop")
			if host == nil || len(host.RankFoldPeers) != 1 || host.Node.Rank != 1 || xlane2FindRowByEvidenceID(&model, "r13-seat") != nil {
				t.Fatalf("the lead seat rides its hop row: %+v\n%s", host, fence)
			}
			detail := runtimeTraceProjDetailFullText(model, zh)
			block := r13DetailBlock(detail, host.EvidenceTag)
			process, position := "- 进程: tgid=57534 comm=unyuan.app.chat", "- 因果位置: 主根因(优先处理)"
			if !zh {
				process, position = "- process: tgid=57534 comm=unyuan.app.chat", "- causal position: primary (handle first)"
			}
			if !strings.Contains(block, process) || !strings.Contains(block, position) {
				t.Fatalf("the one row's detail block keeps the seat's process and position:\n%s", block)
			}
		})
	}
	// When the conclusion consumes another thread's seat, the one row is not
	// the lead and keeps its on-chain position (RN-3(b)).
	lead := elimChainNode("r13-lead", "surfaceflinger-700", "runnable_wait", "runnable", 1, 40.5, 90000)
	lead.Predicate, lead.Role = "root_cause_primary", types.TraceCausalRolePrimaryRootCause
	projection.PrimaryRootCauses = []types.TraceCausalProjectionNode{lead, seat}
	model, fence := hmc377R12Fence(t, projection, true)
	host := xlane2FindRowByEvidenceID(&model, "r13-hop")
	if host == nil || len(host.RankFoldPeers) != 1 || model.LeadKey == runtimeTraceCausalProjectionNodeKey(host.Node) {
		t.Fatalf("the other thread's seat is the lead: lead=%q host=%+v\n%s", model.LeadKey, host, fence)
	}
	if block := r13DetailBlock(runtimeTraceProjDetailFullText(model, true), host.EvidenceTag); !strings.Contains(block, "- 因果位置: 链上(重点)") {
		t.Fatalf("a fold that is not the lead keeps its on-chain position:\n%s", block)
	}
}

// r13DetailBlock returns the detail block headed by [tag].
func r13DetailBlock(detail, tag string) string {
	var out []string
	in := false
	for _, l := range strings.Split(detail, "\n") {
		if strings.HasPrefix(l, "**[") {
			in = strings.HasPrefix(l, "**["+tag+"]")
		}
		if in {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
