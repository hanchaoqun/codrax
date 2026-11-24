package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 14 (batch-owner ruling R-377-14 ⑤, 2026-11-24; review finding
// R13H-2, universe Z1cTwinOnly), end to end through production (publication
// gate -> typed observations -> projection -> tree model -> fences): the gate
// withdraws an inversion seat to a context-only row and releases its claimless
// scheduler-latency twin, the only carrier of the seat's 1.903ms, to the board.
// At cff599c7c the reader face drew the two as ONE row
//
//	└─ ⧖ binder:458_5-6326 · unknown_state █░░░░░░░░░     1.903ms  <1%  [E2(+1)]
//	  · 上下文·不参与根因排序 · runnable
//	  · 影响点 调度延迟 · 有效归因 1.903ms
//
// a context row that prints an attribution, while ◎ listed the 1.903ms only
// as unpriced occupancy: the priced carrier was on no board. A context-only
// row is never folded with a priced row; the released twin is its own row.
func TestHMC377R14ReleasedTwinIsItsOwnRowBesideTheContextRow(t *testing.T) {
	result := hmc377R13WithdrawnAbsorberResult()
	rank := traceQueryPriorityResultForPublication(result).RootCauseRank
	if len(rank.Items) != 3 || rank.Items[1].Tier != tracequery.RootCauseTierContextOnly || rank.Items[1].EffectiveImpactMs != 0 ||
		rank.Items[2].Type != "scheduler_latency" || rank.Items[2].EffectiveImpactMs != 1.903 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("precondition: the engine publishes the withdrawn seat as context and the released twin priced: %+v", rank.Items)
	}

	records := traceQueryTypedObservations(result, "capture.ftrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) != 1 {
		t.Fatalf("projections %d", len(set.Projections))
	}
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var contextRows, pricedRows []*runtimeTraceProjTreeRow
	for _, row := range runtimeTraceProjSMR1AllRows(&model) {
		if !row.HasData || row.Node.Subject != hmc377R9DepSubject {
			continue
		}
		if row.Node.IsContextOnlyRow() {
			contextRows = append(contextRows, row)
		} else {
			pricedRows = append(pricedRows, row)
		}
	}
	f := hmc377R9Run(t, result)
	if len(contextRows) != 1 || len(pricedRows) != 1 {
		t.Fatalf("the thread has one context row and one priced row: context %d priced %d\n%s", len(contextRows), len(pricedRows), f.dump())
	}
	context, twin := contextRows[0], pricedRows[0]
	if context.Node.EffectiveImpactMS != 0 || len(context.Node.MergedEvidenceIDs) != 0 || len(context.Node.SecondaryObjects) != 0 ||
		runtimeTraceProjElimEligible(*context) {
		t.Errorf("the context row carries no attribution and hosts no other publication: effective %.3f merged %v objects %v\n%s",
			context.Node.EffectiveImpactMS, context.Node.MergedEvidenceIDs, context.Node.SecondaryObjects, f.dump())
	}
	if twin.Node.Object != "scheduler_latency" || twin.Node.EffectiveImpactMS != 1.903 || !runtimeTraceProjElimEligible(*twin) {
		t.Errorf("the released twin is its own priced row: object %s effective %.3f\n%s", twin.Node.Object, twin.Node.EffectiveImpactMS, f.dump())
	}
	// One priced carrier of the 1.903ms on ◎: the twin.
	if len(f.depCarriers) != 1 || !strings.HasPrefix(f.depCarriers[0], twin.EvidenceTag+"/") || !strings.HasSuffix(f.depCarriers[0], "/1.903") {
		t.Errorf("◎ carries the 1.903ms once, on the twin [%s]: %v\n%s", twin.EvidenceTag, f.depCarriers, f.dump())
	}
	// The faces: the context words and an attribution are never on one row.
	for _, face := range []struct{ fence, context, attribution string }{
		{f.zh, "上下文·不参与根因排序", "有效归因"},
		{f.en, "context · not ranked", "attribution"},
	} {
		blocks := hmc377R14RowBlocks(face.fence)
		contextBlocks, attributed := 0, 0
		for _, block := range blocks {
			if !strings.Contains(block, "6326") {
				continue
			}
			isContext := strings.Contains(block, face.context)
			if isContext {
				contextBlocks++
			}
			if strings.Contains(block, face.attribution+" 1.903ms") {
				attributed++
				if isContext {
					t.Errorf("a context row prints an attribution:\n%s", block)
				}
			}
		}
		if contextBlocks != 1 || attributed != 1 {
			t.Errorf("one context row and one row with the 1.903ms attribution: context %d attributed %d\n%s", contextBlocks, attributed, face.fence)
		}
	}
}

// hmc377R14RowBlocks splits a tree fence into one block per drawn row: the
// headline that wears the row's [E#] bracket and the lines under it.
func hmc377R14RowBlocks(fence string) []string {
	var blocks []string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "[E") && strings.Contains(line, "]") && strings.HasSuffix(strings.TrimSpace(line), "]") {
			blocks = append(blocks, line)
			continue
		}
		if len(blocks) > 0 {
			blocks[len(blocks)-1] += "\n" + line
		}
	}
	return blocks
}
