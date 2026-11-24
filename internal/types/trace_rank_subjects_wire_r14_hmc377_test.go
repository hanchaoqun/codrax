package types

// HMC377 round 14 — batch-owner ruling R-377-14 ⑧ (2026-11-24): typed carriers
// are synced on every wire.
//
// TraceRankSubjects is an in-process carrier (`json:"-"` on the observation
// record and on the projection node). The one production wire that restores
// observation records from stored bytes is the generation-local native
// retention wire, and it carries the rank subjects beside the rows
// (nativeFactWire.RankSubjects; the inventory of every such field is pinned by
// TestHMC325NativeRetentionFieldOwnershipInventory and the round trip by the
// HMC363 retained-publication test). No other production decode target can
// carry an observation record or a projection node (type-aware scan of every
// decode site, round-14 evidence).
//
// What remains is a rank result that reaches the ledger as text only (no typed
// rows): the summary re-parse compiles it as audit context, and the projection
// refuses it — a row whose rank subjects cannot be known is never drawn, so it
// can never be drawn as an additive row of the wrong thread. This file pins
// that refusal.

import (
	"strings"
	"testing"
)

func TestHMC377R14TextOnlyRankResultDrawsNoProjectionRow(t *testing.T) {
	typed := typedTraceToolResult("trace_query:p.json#root_cause_rank:1")
	typed.Observations[0].TraceRankSubjects = &TraceRankSubjects{
		Target: TraceThreadCoordinate{TID: 20, Comm: "app"}, Member: TraceThreadCoordinate{TID: 20, Comm: "app"}}
	live := CompileTraceCausalProjection(CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{typed}}))
	rows := func(p TraceCausalProjection) []TraceCausalProjectionNode {
		out := append([]TraceCausalProjectionNode(nil), p.PrimaryRootCauses...)
		out = append(out, p.OnChainCauses...)
		out = append(out, p.AdjacentCauses...)
		out = append(out, p.BackgroundCauses...)
		return append(out, p.SupportingHops...)
	}
	if got := rows(live); len(got) != 1 || !got[0].RankSubjects.MemberIsTarget() {
		t.Fatalf("precondition: the typed row is drawn once, with its rank subjects: %d rows", len(got))
	}

	text := typedTraceToolResult("unused")
	text.Observations = nil
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{text}})
	reparsed := false
	for _, record := range ledger.Records {
		if strings.Contains(record.ID, "#trace_query:root_cause_rank:1") {
			reparsed = true
			if record.TraceRankSubjects != nil {
				t.Fatalf("a re-parsed row cannot know its rank subjects: %+v", record.TraceRankSubjects)
			}
		}
	}
	if !reparsed {
		t.Fatal("precondition: the summary re-parse compiled the rank line")
	}
	drawn := CompileTraceCausalProjection(ledger)
	if got := rows(drawn); drawn.Active() || len(got) != 0 {
		t.Errorf("a text-only rank result drew %d row(s) without rank subjects (active=%t)", len(got), drawn.Active())
	}
}
