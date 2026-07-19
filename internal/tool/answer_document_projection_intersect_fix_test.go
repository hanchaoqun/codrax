package tool

// answer_document_projection_intersect_fix_test.go — INTERSECT-FIX render
// live pin (§29.143 INTERSECT-REG 归因, 2026-07-19): the donghu 17267
// flagship board's ∩ cross-direction mutual clauses are back on the FULL
// production chain (BuildIndex → typed observations → projection compile →
// zh render). Under this harness's query shape (wakeup_chain +
// root_cause_rank, MinDurationMs 0.5, Limit 12) the folded io_latency family
// holds 6 member segments (union == published 4.611ms to the µs) and its
// typed intersection with the self running seat's support union is 0.230ms —
// the live pair the §29.136 CHAIN-BUDGET budget widening silently erased
// (family fold absorbed the single-segment partner; the 偏离④ closed set
// refused the family inventory wholesale). 突变职责: stripping the
// family_member_segment_intervals basis arm turns this red (regression
// reproduces); 值面 assertions keep the fix disclosure-only.

import (
	"os"
	"strings"
	"testing"
)

func TestIntersectFixMutualClauseDonghuFlagship(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticDonghuTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	md := elimSemanticRealMarkdown(t, elimSemanticDonghuTrace, 17267, 13762.791708, 13763.024898)

	// 互指成对 (both-or-neither): the 0.230ms mutual clause appears on BOTH
	// seats' tree rows.
	if got := strings.Count(md, "同段重叠 0.230ms"); got < 2 {
		t.Fatalf("回归 pin: the flagship board must render the 0.230ms mutual clause on both rows, got %d occurrence(s):\n%s", got, md)
	}
	// The pair names both directions at the clause site (partner 修向 word).
	if !strings.Contains(md, "(修向 IO与依赖)同段重叠 0.230ms") {
		t.Fatalf("the running row must point at the IO partner with its 修向 word:\n%s", md)
	}
	if !strings.Contains(md, "(修向 频率与热治理)同段重叠 0.230ms") {
		t.Fatalf("the io family row must point at the running partner with its 修向 word:\n%s", md)
	}
	// The ◎ transcription + tree chip marker faces re-appear with the pair.
	if !strings.Contains(md, "∩ 跨方向重叠对(") {
		t.Fatalf("the ◎ overview must transcribe the ∩ pair line:\n%s", md)
	}
	if !strings.Contains(md, "·∩[E") {
		t.Fatalf("the tree rows must wear the ∩ chip marker:\n%s", md)
	}
	// 值面零动: the pair rides the published values unchanged (the running
	// 折算席 58.320 and the io family 合计 4.611 of this harness's shape).
	if !strings.Contains(md, "58.320ms") || !strings.Contains(md, "4.611ms") {
		t.Fatalf("published values must stay untouched by the disclosure fix:\n%s", md)
	}
}
