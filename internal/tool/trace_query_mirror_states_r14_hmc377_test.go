package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 14 (batch-owner ruling R-377-14 ①, 2026-11-24): the mirror tag
// 「同段镜像·与[E#]同一物理时间,不可相加」 needs typed proof for what both lines
// draw. Real windows at cff599c7c (lane evidence
// fix14/mirror/scan/pairs_head.tsv; review-pins finding R13N-1):
//   - an inversion rank seat is minted on a hop and copies the hop's dominant
//     state kind and state split, so it shows the hop's measured account; its
//     drawn value is its gated composition (runnable in full + running
//     discounted). On a sleep or D-state hop the two lines draw disjoint
//     instants and both wore the tag: dio w24m wk:2/1/0/8-447 ☾ sleep 1.586
//     [E11(+1)] beside ⇅ runnable+running 0.089 [E20] (raw: running 0.083 +
//     ready 0.031 + S 1.586 = the 1.700 ms segment); w56 ThreadCountOptG-11053
//     sleep 40.547 beside runnable 33.044; w36 #Alog_instance--41024 D-state
//     2.067 beside runnable+running 0.941;
//   - one tag named a line drawn only inside a fold row (dio w24m
//     m.android.phone-26690 [E14(+1)] naming [E21] of 「其余 4 项微额锚定席
//     [E18+E21+E24+E19]」).
// A pair that counts the account's own state on both lines keeps the tag, at
// equal values (w19 dtmp.dispatch-59777, the R13 pin) or not (new n17m
// hwservicemanager-7155 running ×4 3.561 beside ➍ 2.917 = runnable 0.993 +
// running 1.924).

type r14Split struct{ running, runnable, sleep, dState float64 }

func (s r14Split) apply(node *types.TraceCausalProjectionNode) {
	node.RunningMS, node.RunnableMS, node.SleepMS, node.DStateSplitMS = s.running, s.runnable, s.sleep, s.dState
	node.StateOccupancyPresence = 0
	for bit, value := range []float64{s.running, s.runnable, s.sleep, s.dState} {
		if value > 0 {
			node.StateOccupancyPresence |= 1 << bit
		}
	}
}

func (s r14Split) of(kind string) float64 {
	switch kind {
	case "running":
		return s.running
	case "runnable":
		return s.runnable
	case "s_sleep":
		return s.sleep
	}
	return s.dState
}

const r14Subject = "wk:2/1/0/8-447"

// r14Hop is the wakeup hop row of one occurrence: it is priced at its own
// dominant-state account and draws that state's occupancy.
func r14Hop(id, kind, key string, split r14Split) types.TraceCausalProjectionNode {
	hop := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: id, Subject: r14Subject,
		Predicate: "wakeup_causal_impact", Object: kind, StateKind: kind,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1, ChainBranch: 4,
		ImpactMS: split.of(kind), CumulativeImpactMS: split.of(kind), EffectiveImpactPublished: true,
		StateAccountKey: key, StateAccountComplete: true,
		LineStart: 166710, LineEnd: 169379, StartTs: 11097.276072, EndTs: 11097.277772,
		Confidence: 0.78, Unit: "ms", Value: fmt.Sprintf("%.3f", split.of(kind)),
	}
	if kind == "s_sleep" || kind == "d_sleep" {
		hop.Tier = types.TraceCausalTierContextOnly
	}
	split.apply(&hop)
	return hop
}

// r14MintedSeat is the inversion rank seat the engine mints on host: it copies
// the host's dominant state kind and state split, names the host's account as
// its measured account, and publishes its gated composition as its value.
func r14MintedSeat(id string, host types.TraceCausalProjectionNode, split r14Split, gatedRunnable, gatedDeficit float64) types.TraceCausalProjectionNode {
	seat := elimChainNode(id, host.Subject, "priority_inversion_candidate", host.StateKind, 7, gatedRunnable+gatedDeficit, host.LineStart)
	seat.LineEnd, seat.ChainBranch, seat.StartTs, seat.EndTs = host.LineEnd, host.ChainBranch, host.StartTs, host.EndTs
	seat.GatedRunnableMS, seat.GatedRunningDeficitMS = gatedRunnable, gatedDeficit
	split.apply(&seat)
	seat.MeasuredStateAccountKey = strings.TrimSpace(host.MeasuredStateFactKey())
	seat.EffectiveImpactPublished = true
	seat.FixDirection, seat.Confidence = "lock_priority", 0.91
	return seat
}

// r14OccurrenceFold is the ×N row of an account's occurrences: it names the
// account its members share and draws their folded dominant-state time.
func r14OccurrenceFold(id, kind, group string, count int, folded float64) types.TraceCausalProjectionNode {
	fold := r14Hop(id, kind, "state_account:v2:r14-seed-occurrence", r14Split{})
	fold.Tier, fold.PriorityInversionCandidate = "", true
	fold.ImpactMS, fold.CumulativeImpactMS, fold.Value = folded, folded, fmt.Sprintf("%.3f", folded)
	fold.MergedCount, fold.MergedMinMS, fold.MergedMaxMS = count, folded/float64(count)/2, folded/2
	fold.MeasuredOccurrenceGroupKey = group
	return fold
}

func r14Pair(nodes ...types.TraceCausalProjectionNode) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{"surfaceflinger-700", "unyuan.app.chat-60814"},
		WindowStartTs: 11097.159689, WindowEndTs: 11097.314975,
		OnChainCauses: nodes,
	}
}

func r14MirrorWords(zh bool) string {
	if zh {
		return "同段镜像·与["
	}
	return "same-seg mirror · same physical time as ["
}

// R-377-14 ①: a seat whose priced composition counts none of the account's own
// state draws other instants than the line that draws the account. Neither
// line wears the tag and both stay drawn, additive as at 532dc208d. The shapes
// are the engine's: the seat copies its host's state kind and split.
func TestHMC377R14MirrorNeedsIntersectingDrawnStates(t *testing.T) {
	const key = "state_account:v2:r14-447"
	for _, c := range []struct {
		name           string
		host           func(split r14Split) types.TraceCausalProjectionNode
		split          r14Split
		gatedRunnable  float64
		gatedDeficit   float64
		wantSeatHeader string
	}{
		{name: "dio w24m 447: a sleep hop beside a runnable+running seat",
			host:  func(s r14Split) types.TraceCausalProjectionNode { return r14Hop("r14-host", "s_sleep", key, s) },
			split: r14Split{running: 0.083, runnable: 0.031, sleep: 1.586}, gatedRunnable: 0.031, gatedDeficit: 0.058},
		{name: "dio w56 11053: a sleep hop beside a runnable-only seat",
			host:  func(s r14Split) types.TraceCausalProjectionNode { return r14Hop("r14-host", "s_sleep", key, s) },
			split: r14Split{running: 0.345, runnable: 39.994, sleep: 40.547}, gatedRunnable: 33.044},
		{name: "dio w36 41024: a D-state seat beside a runnable+running seat",
			host: func(s r14Split) types.TraceCausalProjectionNode {
				seat := elimChainNode("r14-host", r14Subject, "d_state_or_io_wait", "d_sleep", 8, s.dState, 184103)
				seat.LineEnd, seat.StateAccountKey, seat.DStateSplitMS, seat.StateOccupancyPresence = 184451, key, s.dState, 8
				return seat
			},
			split: r14Split{running: 0.448, runnable: 0.767, dState: 2.067}, gatedRunnable: 0.767, gatedDeficit: 0.174},
	} {
		for _, zh := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s zh=%t", c.name, zh), func(t *testing.T) {
				host := c.host(c.split)
				seat := r14MintedSeat("r14-seat", host, c.split, c.gatedRunnable, c.gatedDeficit)
				if !types.TraceMeasuredStateFactSame(host, seat) {
					t.Fatalf("fixture: the seat shows its host's measured account: %+v %+v", host, seat)
				}
				model, fence := hmc377R12Fence(t, r14Pair(host, seat), zh)
				hostRow, seatRow := xlane2FindRowByEvidenceID(&model, "r14-host"), xlane2FindRowByEvidenceID(&model, "r14-seat")
				if hostRow == nil || seatRow == nil {
					t.Fatalf("both lines stay drawn:\n%s", fence)
				}
				if hostRow.ValueMirrorRef != "" || seatRow.ValueMirrorRef != "" || strings.Contains(hmc377R12Joined(fence), r14MirrorWords(zh)) {
					t.Fatalf("lines that draw disjoint states wear no mirror tag (host→%q seat→%q):\n%s", hostRow.ValueMirrorRef, seatRow.ValueMirrorRef, fence)
				}
			})
		}
	}
}

// R-377-14 ①: a seat whose priced composition counts the account's own state
// keeps the tag beside the line that draws the account, whatever the two
// drawn values are. Each gated component speaks for its own state only.
func TestHMC377R14MirrorKeepsPairsThatCountTheAccountState(t *testing.T) {
	const group = "state_account:v2:r14-7155-aggregate"
	for _, c := range []struct {
		name          string
		kind          string
		folded        float64
		split         r14Split
		gatedRunnable float64
		gatedDeficit  float64
		tagged        bool
	}{
		{name: "new n17m 7155: running ×4 3.561 beside runnable 0.993 + running 1.924", kind: "running", folded: 3.561,
			split: r14Split{running: 3.561, runnable: 0.993, dState: 0.264}, gatedRunnable: 0.993, gatedDeficit: 1.924, tagged: true},
		{name: "dio w13 12005: runnable ×3 24.656 beside runnable 20.115 + running 0.369", kind: "runnable", folded: 24.656,
			split: r14Split{running: 0.661, runnable: 24.656, sleep: 0.134}, gatedRunnable: 20.115, gatedDeficit: 0.369, tagged: true},
		{name: "a running account beside a runnable-only composition", kind: "running", folded: 3.561,
			split: r14Split{running: 3.561, runnable: 0.993}, gatedRunnable: 0.993},
		{name: "a runnable account beside a running-only composition", kind: "runnable", folded: 4.692,
			split: r14Split{running: 0.493, runnable: 4.692}, gatedDeficit: 0.343},
	} {
		t.Run(c.name, func(t *testing.T) {
			fold := r14OccurrenceFold("r14-fold", c.kind, group, 4, c.folded)
			seat := r14MintedSeat("r14-seat", fold, c.split, c.gatedRunnable, c.gatedDeficit)
			if !types.TraceMeasuredStateFactSame(fold, seat) {
				t.Fatalf("fixture: the seat shows the fold's measured account: %+v %+v", fold, seat)
			}
			model, fence := hmc377R12Fence(t, r14Pair(fold, seat), true)
			foldRow, seatRow := xlane2FindRowByEvidenceID(&model, "r14-fold"), xlane2FindRowByEvidenceID(&model, "r14-seat")
			if foldRow == nil || seatRow == nil {
				t.Fatalf("both lines stay drawn:\n%s", fence)
			}
			if !c.tagged {
				if foldRow.ValueMirrorRef != "" || seatRow.ValueMirrorRef != "" {
					t.Fatalf("a composition that counts none of the account's state is no mirror (fold→%q seat→%q):\n%s", foldRow.ValueMirrorRef, seatRow.ValueMirrorRef, fence)
				}
				return
			}
			if foldRow.ValueMirrorRef != seatRow.EvidenceTag || seatRow.ValueMirrorRef != foldRow.EvidenceTag {
				t.Fatalf("both lines count the account's state and name each other (fold→%q seat→%q):\n%s", foldRow.ValueMirrorRef, seatRow.ValueMirrorRef, fence)
			}
			for _, want := range []string{
				"同段镜像·与[" + seatRow.EvidenceTag + "]同一物理时间,不可相加",
				"同段镜像·与[" + foldRow.EvidenceTag + "]同一物理时间,不可相加",
			} {
				if !strings.Contains(hmc377R12Joined(fence), want) {
					t.Fatalf("the tree must print %q:\n%s", want, fence)
				}
			}
		})
	}
}

// The tag also needs the whole account on both lines: a ×N row that folds only
// part of the account's occurrences (the rest are drawn elsewhere) shows
// another amount than the seat of the whole account and is not told it repeats
// it.
func TestHMC377R14MirrorNeedsTheWholeAccountOnBothLines(t *testing.T) {
	const group = "state_account:v2:r14-7155-aggregate"
	split := r14Split{running: 3.561, runnable: 0.993}
	whole := r14OccurrenceFold("r14-fold", "running", group, 4, 3.561)
	seat := r14MintedSeat("r14-seat", whole, split, 0.993, 1.924)
	part := r14OccurrenceFold("r14-fold", "running", group, 3, 2.926)
	model, fence := hmc377R12Fence(t, r14Pair(part, seat), true)
	foldRow, seatRow := xlane2FindRowByEvidenceID(&model, "r14-fold"), xlane2FindRowByEvidenceID(&model, "r14-seat")
	if foldRow == nil || seatRow == nil {
		t.Fatalf("both lines stay drawn:\n%s", fence)
	}
	if foldRow.ValueMirrorRef != "" || seatRow.ValueMirrorRef != "" {
		t.Fatalf("a row of part of the account is no mirror of the whole account's seat (fold→%q seat→%q):\n%s", foldRow.ValueMirrorRef, seatRow.ValueMirrorRef, fence)
	}
}

// R-377-14 ①: both lines must be drawn tree lines. A seat the tree draws only
// inside the micro fold row is not a line a tag can name.
func TestHMC377R14MirrorPeerInsideAFoldRowIsNoPeer(t *testing.T) {
	const key = "state_account:v2:r14-9503"
	micro := func(id, subject string, rank int, anchored, full float64, line int) types.TraceCausalProjectionNode {
		node := elimChainNode(id, subject, "runnable_wait", "runnable", rank, anchored, line)
		node.ChainAnchoredMS, node.ChainAnchorFullMS, node.RunnableMS = anchored, full, anchored
		node.FixDirection = "scheduling_supply"
		return node
	}
	hop := r14Hop("r14-hop", "runnable", key, r14Split{running: 0.012, runnable: 0.026})
	hop.Subject, hop.PriorityInversionCandidate = "hilogcat-9503", true
	hop.LineStart, hop.LineEnd = 100, 105
	seat := micro("r14-micro", "hilogcat-9503", 5, 0.026, 17.292, 100)
	seat.MeasuredStateAccountKey = key
	other := micro("r14-micro-2", "logd.writer-9163", 6, 0.018, 49.656, 110)
	if !types.TraceMeasuredStateFactSame(hop, seat) {
		t.Fatalf("fixture: the micro seat shows its hop's measured account: %+v %+v", hop, seat)
	}
	t.Run("the lone micro seat is a drawn line", func(t *testing.T) {
		model, fence := hmc377R12Fence(t, r14Pair(hop, seat), true)
		hopRow, seatRow := xlane2FindRowByEvidenceID(&model, "r14-hop"), xlane2FindRowByEvidenceID(&model, "r14-micro")
		if hopRow == nil || seatRow == nil || hopRow.ValueMirrorRef != seatRow.EvidenceTag || seatRow.ValueMirrorRef != hopRow.EvidenceTag {
			t.Fatalf("fixture: a micro seat drawn as its own row is its hop's mirror: %+v %+v\n%s", hopRow, seatRow, fence)
		}
	})
	model, fence := hmc377R12Fence(t, r14Pair(hop, seat, other), true)
	if !strings.Contains(fence, "其余 2 项微额锚定席") || xlane2FindRowByEvidenceID(&model, "r14-micro") != nil {
		t.Fatalf("fixture: the two micro seats are drawn as one fold row:\n%s", fence)
	}
	hopRow := xlane2FindRowByEvidenceID(&model, "r14-hop")
	if hopRow == nil {
		t.Fatalf("the hop stays drawn:\n%s", fence)
	}
	if hopRow.ValueMirrorRef != "" || strings.Contains(hmc377R12Joined(fence), r14MirrorWords(true)) {
		t.Fatalf("a seat drawn only inside a fold row is not a peer (hop→%q):\n%s", hopRow.ValueMirrorRef, fence)
	}
}

// R-377-14 ①: the legend says what the tag is drawn on. The two lines can
// draw different values, so it no longer promises an equal-value form.
func TestHMC377R14MirrorLegendSaysWhatIsDrawn(t *testing.T) {
	const group = "state_account:v2:r14-7155-aggregate"
	fold := r14OccurrenceFold("r14-fold", "running", group, 4, 3.561)
	seat := r14MintedSeat("r14-seat", fold, r14Split{running: 3.561, runnable: 0.993}, 0.993, 1.924)
	projection := r14Pair(fold, seat)
	for lang, want := range map[string][2]string{
		"zh": {"同值双行形", "双行形两行各自的行2互指:两行计入同一线程同一状态的同一段时间(两行数值可以不同),不可相加"},
		"en": {"equal-value two-row", "on the two-row form each row's row 2 names the other: both rows count the same stretch of one thread's time in one state (their values can differ) and are never additive"},
	} {
		zh := lang == "zh"
		model, fence := hmc377R12Fence(t, projection, zh)
		if !model.Marks.has(runtimeTraceProjMarkSameSegMirror) {
			t.Fatalf("fixture: the face wears the mirror tag (%s):\n%s", lang, fence)
		}
		lead := runtimeTraceProjLeadText(projection, model, lang, zh)
		if strings.Contains(lead, want[0]) || !strings.Contains(lead, want[1]) {
			t.Fatalf("the %s legend must say what is drawn (%q), without %q:\n%s", lang, want[1], want[0], lead)
		}
	}
}
