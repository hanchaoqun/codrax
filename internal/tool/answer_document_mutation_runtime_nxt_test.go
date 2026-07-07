// answer_document_mutation_runtime_nxt_test.go — NXT 批 pins (账本
// real_trace_campaign_20260705.md §22 D-P1; RCX① drill-debt §12.3-1① 用户面
// next-step 半场; specimen cust_trace_huadong_01.txt):
//
//	N1 — when a projection's HEADLINE (the runtimeTraceProjLeadSelect product
//	     — same surface as the conclusion line) renders on the dedicated
//	     链上·深度未解析 tree lane (typed row Kind+Edge pair) or carries the
//	     typed UndrillableReason, the next-step list gains ONE pointed
//	     drilldown row naming the subject, why it is unresolved, and the
//	     tool-visible views to run (wakeup_chain / critical_blocking_calls).
//	     The huadong specimen shipped a rank=1 binder_wait headline with four
//	     generic/window-caliber rows and zero pointed guidance.
//	N2 — the pointed row keeps a guaranteed floor seat
//	     (runtimeTraceNextStepUndrilledHeadlineFloor): displacement of a
//	     generic row inside the base cap on ordinary shapes, cap extension
//	     when the comparison family (which emits in full by #69) already
//	     filled the base cap.
//
// Negative space: a drilled headline (resolved ChainDepth → chain lane), a
// flat render (no ≥2-node wakeup path — the flat header + RN-13(b) own that
// disclosure) and an unresolved-subject lead emit NO pointed row.
package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// nxtObs builds one hard-grounded trace_query observation on the given
// artifact.
func nxtObs(id, artifact, predicate, claimKey, subject, object, value string, span types.ObservationSpan, notes ...string) types.ObservationRecord {
	record := types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: predicate, ClaimKey: claimKey,
		Subject: subject, Object: object, Value: value, Unit: "ms", Confidence: 0.92,
		Span:      span,
		RichNotes: notes,
	}
	if artifact != "" {
		record.SourceRef = types.ObservationSourceRef{
			Kind:         types.ObservationSourceRuntimeArtifact,
			Path:         artifact,
			ArtifactKind: "trace",
		}
	}
	return record
}

// nxtHuadongObs mirrors the huadong_01 E16 shape on one artifact: a ≥2-node
// wakeup path (trunked render, 🎯 target = VSyncGenerator-2270) plus a rank=1
// tier=primary binder_wait row whose subject is NOT on the trunk and whose
// ChainDepth never resolved — the compiled tree seats it on the dedicated
// 链上·深度未解析 lane and the conclusion line names it as the 主根因.
func nxtHuadongObs(artifact string, extraPrimaryNotes ...string) []types.ObservationRecord {
	primaryNotes := append([]string{
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"type=binder_wait", "impact_ms=4.577", "cumulative_impact_ms=4.577",
		"selected_window=6793222.700000..6793222.800000",
	}, extraPrimaryNotes...)
	return []types.ObservationRecord{
		nxtObs("nxt-path", artifact, "wakeup_chain", "",
			"VSyncGenerator-2270", "tppmgr-idle-1-186 -> VSyncGenerator-2270", "",
			types.ObservationSpan{LineStart: 1000, LineEnd: 1100, StartTs: 6793222.700, EndTs: 6793222.800}),
		nxtObs("nxt-e16", artifact, "root_cause_primary", "root_cause_primary:nxt-e16",
			"oney.hmn.berlin-42591", "binder_wait", "4.577",
			types.ObservationSpan{LineStart: 1017021, LineEnd: 1031685},
			primaryNotes...),
	}
}

// nxtGenericStepObs carries one typed per-record next-step template payload.
func nxtGenericStepObs(id, artifact, kind, prose string, line int) types.ObservationRecord {
	return nxtObs(id, artifact, "state_churn", "", "oney.hmn.berlin-42591", "", "112.000",
		types.ObservationSpan{LineStart: line, LineEnd: line + 10},
		"next_step="+prose, "next_step_kind="+kind)
}

func nxtNextStepTexts(t *testing.T, bus *types.BusContext) []string {
	t.Helper()
	items := runtimeTraceNextStepItems(&types.AnswerDocumentV2{DocumentModel: "v2"}, bus)
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.Text)
	}
	return texts
}

// nxtPointedRowZH is the N1 verbatim pin for the huadong binder_wait form:
// subject (+ cause word + engine rank), WHY (深度未解析 — the tree edge's own
// user-facing vocabulary) and HOW (the tool-visible view names).
const nxtPointedRowZH = "对主根因 oney.hmn.berlin-42591(binder等待,rank=1)在其发生窗执行 wakeup_chain / critical_blocking_calls 下钻:该行当前深度未解析,尚无已核实的上游因果"

// TestRuntimeTraceNextStepUndrilledHeadlinePointedRow — N1 positive pin
// (huadong_01 shape): the pointed row leads the single-trace list VERBATIM,
// the multi-window caliber rows keep their seats, and the displaced row is
// the LAST generic template (3 通用+1 点名 under the unchanged base cap=4 —
// the N2 displacement ruling).
func TestRuntimeTraceNextStepUndrilledHeadlinePointedRow(t *testing.T) {
	bus := newBusForMutationTest()
	obs := nxtHuadongObs("cust_trace_huadong_01.systrace")
	obs = append(obs,
		// A second DISTINCT typed query window engages the PTV5 Q3
		// multi-window lane (two caliber rows), mirroring the specimen list.
		nxtGenericStepObs("nxt-ns1", "cust_trace_huadong_01.systrace", "s_sleep",
			"investigate the peer threads that repeatedly wake it, binder waits, lock and condition waits", 200),
		nxtGenericStepObs("nxt-ns2", "cust_trace_huadong_01.systrace", "d_sleep_io",
			"inspect sched_blocked_reason and block IO evidence", 400),
	)
	obs[len(obs)-2].RichNotes = append(obs[len(obs)-2].RichNotes, "selected_window=6793222.031000..6793225.370000")
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}

	texts := nxtNextStepTexts(t, bus)
	if len(texts) != runtimeTraceNextStepMaxItems {
		t.Fatalf("single-trace shape keeps the base cap (displacement, not extension): %q", texts)
	}
	if texts[0] != nxtPointedRowZH {
		t.Fatalf("N1 pointed row must lead the single-trace list verbatim:\nwant %q\n got %q", nxtPointedRowZH, texts[0])
	}
	// The multi-window caliber rows keep their seats behind the pointed row.
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 同口径因果采样 →
	// 双窗对比:对每个查询窗分别做同样的根因分析(wakeup_chain/root_cause_rank),
	// 再逐窗对比 (next-steps item 9).
	if !strings.Contains(texts[1], "个查询窗") ||
		!strings.Contains(texts[2], "双窗对比:对每个查询窗分别做同样的根因分析(wakeup_chain/root_cause_rank),再逐窗对比") {
		t.Fatalf("multi-window caliber rows must keep their seats behind the pointed row: %q", texts)
	}
	// One generic template survives; the LAST one is what the pointed row
	// displaced out of the cap.
	if !strings.Contains(texts[3], "排查反复唤醒它的对端线程") {
		t.Fatalf("first generic template keeps the last seat: %q", texts)
	}
	if strings.Contains(strings.Join(texts, "\n"), "sched_blocked_reason") {
		t.Fatalf("the displaced row must be the trailing generic template: %q", texts)
	}
}

// TestRuntimeTraceNextStepUndrilledHeadlineViewNamesAndNoJargon — the pointed
// row speaks tool-visible view names and zero internal pipeline vocabulary.
func TestRuntimeTraceNextStepUndrilledHeadlineViewNamesAndNoJargon(t *testing.T) {
	bus := newBusForMutationTest()
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true,
		Observations: nxtHuadongObs("cust_trace_huadong_01.systrace")}}
	texts := nxtNextStepTexts(t, bus)
	if len(texts) == 0 {
		t.Fatalf("undrilled headline must emit the pointed row")
	}
	row := texts[0]
	if !strings.Contains(row, "wakeup_chain") || !strings.Contains(row, "critical_blocking_calls") {
		t.Fatalf("pointed row must name the tool-visible drilldown views: %q", row)
	}
	for _, banned := range []string{"depthless", "undrilled", "Undrillable", "lead", "projection", "LeadSelect", "row.Kind"} {
		if strings.Contains(row, banned) {
			t.Fatalf("pointed row must carry no internal pipeline vocabulary (%q): %q", banned, row)
		}
	}
}

// TestRuntimeTraceNextStepDrilledHeadlineEmitsNoPointedRow — N1 negative pin:
// the SAME headline with a RESOLVED ChainDepth seats on the chain lane (wake
// edge) and the pointed row must not appear (its 深度未解析 claim would be
// false). 对抗复核收尾 (2026-07-07): the fixture ALSO carries an E17/E18-form
// SIBLING on-chain row without a resolved depth — it seats on the
// 链上·深度未解析 lane but is NOT the headline, so this one assertion kills
// both survived mutant classes: a gate that drops the node-key equality
// (fires on the sibling's depthless row) and the full relaxation to "any
// Kind==depthless row on the tree". The pointed row must key on the HEADLINE
// row itself, never on the mere existence of a depthless lane.
func TestRuntimeTraceNextStepDrilledHeadlineEmitsNoPointedRow(t *testing.T) {
	bus := newBusForMutationTest()
	obs := nxtHuadongObs("cust_trace_huadong_01.systrace", "chain_depth=1")
	obs = append(obs, nxtObs("nxt-e17", "cust_trace_huadong_01.systrace",
		"root_cause_primary", "root_cause_primary:nxt-e17",
		"OS_FFRT_2_2-43037", "running", "0.051",
		types.ObservationSpan{LineStart: 1620572, LineEnd: 1620584},
		"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"type=running", "impact_ms=0.051", "cumulative_impact_ms=0.051"))
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	texts := nxtNextStepTexts(t, bus)
	joined := strings.Join(texts, "\n")
	if strings.Contains(joined, "深度未解析") || strings.Contains(joined, "critical_blocking_calls 下钻") {
		t.Fatalf("a drilled (depth-resolved) headline must not synthesize the pointed row, even beside a non-headline depthless sibling:\n%s", joined)
	}
}

// TestRuntimeTraceNextStepOwnIOEdgeLeadEmitsNoPointedRow — Edge-judgment pin
// (对抗复核收尾 2026-07-07): a headline whose own row sits on the depthless
// OWN-PROCESS IO caliber edge (Kind depthless + Edge own — the F2 lane that
// deliberately does NOT claim 深度未解析 on any tree surface) must not
// synthesize the pointed row either. Kills the Edge-only mutant (a gate that
// checks Kind + node-key equality but drops the chain-unresolved Edge
// judgment).
func TestRuntimeTraceNextStepOwnIOEdgeLeadEmitsNoPointedRow(t *testing.T) {
	bus := newBusForMutationTest()
	obs := []types.ObservationRecord{
		// Trunked render whose 🎯 target is the user thread (pid 42591).
		nxtObs("nxt-path", "cust_trace_huadong_01.systrace", "wakeup_chain", "",
			"oney.hmn.berlin-42591", "tppmgr-idle-1-186 -> oney.hmn.berlin-42591", "",
			types.ObservationSpan{LineStart: 1000, LineEnd: 1100, StartTs: 6793222.700, EndTs: 6793222.800}),
		// rank=1 IO caliber row of the target's OWN process (same pid,
		// different thread name, typed io_wait token) — stamped Edge=own at
		// model build (runtimeTraceProjOwnProcessIONode).
		nxtObs("nxt-own-io", "cust_trace_huadong_01.systrace",
			"root_cause_primary", "root_cause_primary:nxt-own-io",
			"OS_binder-42591", "io_wait", "3.000",
			types.ObservationSpan{LineStart: 2000, LineEnd: 2100},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"type=io_wait", "impact_ms=3.000", "cumulative_impact_ms=3.000"),
	}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	if joined := strings.Join(nxtNextStepTexts(t, bus), "\n"); strings.Contains(joined, "深度未解析") {
		t.Fatalf("an own-process IO caliber lead (Edge=own) must not claim 深度未解析:\n%s", joined)
	}
}

// TestRuntimeTraceNextStepFlatRenderEmitsNoPointedRow — boundary pin: a flat
// render (no ≥2-node wakeup path) leaves the disclosure to the dedicated flat
// header + RN-13(b) recovery lane; the depth-unresolved wording belongs to the
// trunked 链上·深度未解析 lane only.
func TestRuntimeTraceNextStepFlatRenderEmitsNoPointedRow(t *testing.T) {
	bus := newBusForMutationTest()
	obs := nxtHuadongObs("cust_trace_huadong_01.systrace")[1:] // drop the wakeup path record
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	texts := nxtNextStepTexts(t, bus)
	if joined := strings.Join(texts, "\n"); strings.Contains(joined, "深度未解析") {
		t.Fatalf("flat render must not claim 深度未解析 on the next-step list:\n%s", joined)
	}
}

// TestRuntimeTraceNextStepUndrilledHeadlineFloorOnComparisonShape — N2 floor
// pin: the comparison family (3 fixed + RTC-2 disjoint) already fills the
// base cap; the pointed row STILL seats (cap extension = the floor), the
// PTS-2 per-record floor stays intact behind it, and the artifact label
// disambiguates the pointed subject on the multi-artifact ledger.
func TestRuntimeTraceNextStepUndrilledHeadlineFloorOnComparisonShape(t *testing.T) {
	bus := newBusForMutationTest()
	obs := nxtHuadongObs("nxt_a.systrace")
	// Give artifact A a time base disjoint from artifact B (RTC-2 row gate).
	obs[0].Span = types.ObservationSpan{LineStart: 1000, LineEnd: 1100, StartTs: 3679.899, EndTs: 3681.129}
	obs[1].Span = types.ObservationSpan{LineStart: 32642, LineEnd: 199899, StartTs: 3679.899, EndTs: 3681.129}
	obs[1].RichNotes = append(obs[1].RichNotes[:0:0], "rank=1", "tier=primary",
		"chain_relevance=on_chain", "causality=on_wakeup_chain", "type=binder_wait",
		"impact_ms=4.577", "cumulative_impact_ms=4.577")
	obs = append(obs,
		// Artifact B: an ordinary drilled-shape (flat) primary — active
		// projection, no pointed row of its own.
		nxtObs("nxt-b-run", "nxt_b.systrace", "root_cause_primary", "root_cause_primary:nxt-b-run",
			"OS_FFRT_2_6-18695", "sleep_wait", "701.000",
			types.ObservationSpan{LineStart: 31022, LineEnd: 123248, StartTs: 8143.800, EndTs: 8144.501},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"impact_ms=701.000", "cumulative_impact_ms=701.000"),
		nxtGenericStepObs("nxt-ns1", "nxt_a.systrace", "s_sleep",
			"investigate the peer threads that repeatedly wake it, binder waits, lock and condition waits", 200),
		nxtGenericStepObs("nxt-ns2", "nxt_a.systrace", "d_sleep_io",
			"inspect sched_blocked_reason and block IO evidence", 400),
	)
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}

	texts := nxtNextStepTexts(t, bus)
	if len(texts) != 4+runtimeTraceNextStepUndrilledHeadlineFloor+runtimeTraceNextStepComparisonRecordFloor {
		t.Fatalf("comparison family + pointed floor + per-record floor must coexist: %q", texts)
	}
	// Comparison rows LEAD unchanged (CMP-6/#69 adjudication).
	if !strings.Contains(texts[0], "对比两 trace") || !strings.Contains(texts[3], "时间基准不相交") {
		t.Fatalf("comparison rows must lead the list: %q", texts)
	}
	// The pointed row takes its guaranteed floor seat right behind the
	// comparison family, artifact-labeled on the multi-artifact ledger.
	want := "对主根因 oney.hmn.berlin-42591(binder等待,rank=1,nxt_a.systrace)在其发生窗执行 wakeup_chain / critical_blocking_calls 下钻:该行当前深度未解析,尚无已核实的上游因果"
	if texts[4] != want {
		t.Fatalf("N2 floor: pointed row must survive a cap-full comparison family:\nwant %q\n got %q", want, texts[4])
	}
	// PTS-2 per-record floor stays intact behind the pointed row.
	if !strings.Contains(texts[5], "排查反复唤醒它的对端线程") || !strings.Contains(texts[6], "sched_blocked_reason") {
		t.Fatalf("per-record floor rows must keep their seats behind the pointed row: %q", texts)
	}
}

// TestRuntimeTraceNextStepUndrilledHeadlineEnglishWording — EN surface keeps
// the same admission logic with the English pointed wording (three elements:
// subject / why / views).
func TestRuntimeTraceNextStepUndrilledHeadlineEnglishWording(t *testing.T) {
	bus := newBusForMutationTest()
	bus.Language = "en"
	bus.AnalysisIR = &types.AnalysisIR{AnswerContract: types.AnswerContract{Language: "en"}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true,
		Observations: nxtHuadongObs("cust_trace_huadong_01.systrace")}}
	texts := nxtNextStepTexts(t, bus)
	want := "Drill into the primary root cause oney.hmn.berlin-42591 (binder_wait, rank=1) with wakeup_chain / critical_blocking_calls in its occurrence window: its chain depth is unresolved and no verified upstream cause is attached yet"
	if len(texts) == 0 || texts[0] != want {
		t.Fatalf("EN pointed row missing or drifted:\nwant %q\n got %q", want, texts)
	}
}

// TestRuntimeTraceNextStepUndrilledHeadlineMissingWakeupArm — the typed
// UndrillableReason arm words the missing-wakeup reason (the conclusion
// line's ⊘ clause vocabulary) and keeps both view names in play.
func TestRuntimeTraceNextStepUndrilledHeadlineMissingWakeupArm(t *testing.T) {
	lead := types.TraceCausalProjectionNode{
		Subject:           "oney.hmn.berlin-42591",
		Object:            "sleep_wait",
		Rank:              1,
		UndrillableReason: "missing_wakeup",
	}
	if !lead.Undrillable() {
		t.Fatalf("typed UndrillableReason must report Undrillable()")
	}
	zhRow := runtimeTraceNextStepUndrilledHeadlineText(lead, "", false, true)
	if !strings.Contains(zhRow, "oney.hmn.berlin-42591") ||
		!strings.Contains(zhRow, "无匹配唤醒记录") ||
		!strings.Contains(zhRow, "wakeup_chain") || !strings.Contains(zhRow, "critical_blocking_calls") {
		t.Fatalf("missing-wakeup arm must keep subject/why/views: %q", zhRow)
	}
	enRow := runtimeTraceNextStepUndrilledHeadlineText(lead, "", false, false)
	if !strings.Contains(enRow, "no matching wakeup record") ||
		!strings.Contains(enRow, "wakeup_chain") || !strings.Contains(enRow, "critical_blocking_calls") {
		t.Fatalf("EN missing-wakeup arm must keep subject/why/views: %q", enRow)
	}
}

// TestRuntimeTraceNextStepUndrilledHeadlineUnresolvedSubjectYields — SG
// precedent: soft guidance never fabricates an identity — an unknown-thread
// lead admits no pointed row (helper-level gate check).
func TestRuntimeTraceNextStepUndrilledHeadlineUnresolvedSubjectYields(t *testing.T) {
	bus := newBusForMutationTest()
	obs := nxtHuadongObs("cust_trace_huadong_01.systrace")
	obs[1].Subject = "unknown-thread"
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	if joined := strings.Join(nxtNextStepTexts(t, bus), "\n"); strings.Contains(joined, "深度未解析") {
		t.Fatalf("an unresolved-subject lead must not synthesize a pointed row:\n%s", joined)
	}
}
