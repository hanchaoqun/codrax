package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 14 (batch-owner ruling R-377-14 ②, 2026-11-24): the table's
// 窗口投影 cell, its legend sentence and the ◎ short mark say one thing about an
// inversion row. Real windows at cff599c7c (lane evidence
// fix14/mirror/scan/composite_head.tsv; review-pins finding R13N-2): a hop row
// that took its inversion seat's account (R-377-13 ①) is named 优先级反转候选 in
// the table and wears 「·构成」 on ◎, while its 窗口投影 cell printed the hop's
// single-state time bare and the legend sentence that explains the composite
// cell was not printed — 294 rows on 119 of 315 windows, 35 windows with the
// ◎ mark and no legend sentence (dio w04 #tp-io-17-13410: 「| … / 优先级反转候选
// [E17(+1)] | 1.216ms | 1.475ms | 1.396ms |」; at 532dc208d the seat's own row
// read 「| … / 优先级反转候选 [E30] | 1.396ms(构成,见明细) | 1.475ms | 1.396ms |」).

func r14TableCell(t *testing.T, model runtimeTraceProjTreeModel, zh bool, tag string) string {
	t.Helper()
	_, rows := runtimeTraceProjDetailTable(model, zh)
	for _, row := range rows {
		if strings.Contains(row.Cells[0], "["+tag+"]") {
			return row.Cells[1]
		}
	}
	t.Fatalf("no table row wears [%s]", tag)
	return ""
}

// The one row of a hop and the inversion seat whose account it draws is the
// table row of that seat: its 窗口投影 cell is the seat's composite value with
// the annotation, and the legend sentence that explains the cell is raised by
// the same row.
func TestHMC377R14FoldedInversionRowKeepsItsCompositeCell(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	projection := r13Projection(r13Hop("r13-hop", key), r13Seat("r13-seat", key), r13Remainder())
	for _, zh := range []bool{true, false} {
		model, fence := hmc377R12Fence(t, projection, zh)
		host := xlane2FindRowByEvidenceID(&model, "r13-hop")
		if host == nil || len(host.RankFoldPeers) != 1 || host.RankFoldPeers[0].ownSeat == nil || host.Node.ImpactMS != 8.294 {
			t.Fatalf("fixture: the hop row draws its seat's account as its own and keeps its own state time (zh=%t):\n%s", zh, fence)
		}
		want := "8.327ms(构成,见明细)"
		mark := " ·构成"
		if !zh {
			want, mark = "8.327ms (composite, see the detail blocks)", " ·composition"
		}
		if cell := r14TableCell(t, model, zh, host.EvidenceTag); cell != want {
			t.Fatalf("the folded inversion row's cell is the seat's composite with its annotation (zh=%t): got %q want %q", zh, cell, want)
		}
		if !runtimeTraceProjDetailTableLegendFlagsFor(model, zh).gatedProjection {
			t.Fatalf("the annotated cell raises the legend sentence that explains it (zh=%t)", zh)
		}
		elim := runtimeTraceProjElimOverviewFence(projection, model, zh)
		if !strings.Contains(elim, mark) {
			t.Fatalf("fixture: the ◎ line of the row wears the composite short mark (zh=%t):\n%s", zh, elim)
		}
	}
}

// The three faces read the seat the row publishes, never the fields the hop
// row happens to carry itself. On the 315 real windows every such hop row
// mirrors its seat's decomposition (338 of 338, lane evidence
// fix14/mirror/scan/foldgate_head.txt), so the faces agreed on the mark; the
// cell and the legend sentence forked from it because they read the hop's own
// window value. A hop row published without the decomposition must not fork
// them the other way.
func TestHMC377R14FoldedRowAnswersFromOneSeatOnEveryFace(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	hop := r13Hop("r13-hop", key)
	hop.GatedRunnableMS, hop.GatedRunningDeficitMS = 0, 0
	projection := r13Projection(hop, r13Seat("r13-seat", key), r13Remainder())
	for _, zh := range []bool{true, false} {
		model, fence := hmc377R12Fence(t, projection, zh)
		host := xlane2FindRowByEvidenceID(&model, "r13-hop")
		if host == nil || len(host.RankFoldPeers) != 1 || host.RankFoldPeers[0].ownSeat == nil ||
			runtimeTraceProjGatedCompositeSeat(host.Node) {
			t.Fatalf("fixture: the hop row draws its seat's account as its own and carries no decomposition itself (zh=%t):\n%s", zh, fence)
		}
		want, mark := "8.327ms(构成,见明细)", "优先级反转候选 ·构成"
		if !zh {
			want, mark = "8.327ms (composite, see the detail blocks)", " ·composition"
		}
		cell := r14TableCell(t, model, zh, host.EvidenceTag)
		legend := runtimeTraceProjDetailTableLegendFlagsFor(model, zh).gatedProjection
		elim := runtimeTraceProjElimOverviewFence(projection, model, zh)
		if cell != want || !legend || !strings.Contains(elim, mark) {
			t.Fatalf("cell, legend sentence and ◎ mark answer from the one published seat (zh=%t): cell %q legend %t\n%s", zh, cell, legend, elim)
		}
	}
}

// A folded seat whose value is runnable time alone is no composite: the cell
// stays the state time the row draws, bare, and raises no legend sentence.
func TestHMC377R14FoldedPureRunnableSeatKeepsTheBareCell(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	hop, seat := r13Hop("r13-hop", key), r13Seat("r13-seat", key)
	for _, node := range []*types.TraceCausalProjectionNode{&hop, &seat} {
		node.GatedRunningDeficitMS, node.EffectiveImpactMS = 0, 8.294
	}
	seat.ImpactMS = 8.294
	projection := r13Projection(hop, seat, r13Remainder())
	model, fence := hmc377R12Fence(t, projection, true)
	host := xlane2FindRowByEvidenceID(&model, "r13-hop")
	if host == nil || len(host.RankFoldPeers) != 1 || host.RankFoldPeers[0].ownSeat == nil {
		t.Fatalf("fixture: the hop row draws its seat's account as its own:\n%s", fence)
	}
	if cell := r14TableCell(t, model, true, host.EvidenceTag); cell != "8.294ms" {
		t.Fatalf("a pure runnable value is state time inside the window and stays bare: %q", cell)
	}
	if runtimeTraceProjDetailTableLegendFlagsFor(model, true).gatedProjection {
		t.Fatalf("no annotated cell, no legend sentence")
	}
}

// The older same-span fold keeps its 532dc208d form: its row is named by the
// hop's state, not as an inversion row, so its cell stays the hop's state
// time and it raises no legend sentence. The protected NetEase reboot5
// [153.763595,153.887340) TID 11077 face is of this form (HeapTaskDaemon-11098
// [E4(+1)] wears 「·构成」 on ◎ beside a table row named running and no legend
// sentence, as at d5ef1c6fe); changing it moves that face (reported, not
// committed: fix14/mirror/evidence/stopped_protected/).
func TestHMC377R14LegacyFoldKeepsItsStateCell(t *testing.T) {
	const key = "state_account:v2:r13-59698"
	seat := r13Seat("r13-seat", key)
	seat.CumulativeImpactMS = 8.294
	hop := r13Hop("r13-hop", key)
	hop.EffectiveImpactMS = seat.EffectiveImpactMS
	model, fence := hmc377R12Fence(t, r13Projection(hop, seat, r13Remainder()), true)
	host := xlane2FindRowByEvidenceID(&model, "r13-hop")
	if host == nil || len(host.RankFoldPeers) != 1 || host.RankFoldPeers[0].ownSeat != nil {
		t.Fatalf("fixture: the legacy pair folds without taking the seat's account:\n%s", fence)
	}
	if cell := r14TableCell(t, model, true, host.EvidenceTag); cell != "8.294ms" {
		t.Fatalf("the legacy fold's row keeps the hop's state time: %q", cell)
	}
	if runtimeTraceProjDetailTableLegendFlagsFor(model, true).gatedProjection {
		t.Fatalf("the legacy fold's row raises no legend sentence")
	}
}
