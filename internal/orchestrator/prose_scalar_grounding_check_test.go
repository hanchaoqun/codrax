package orchestrator

// PSG batch pins (§25 ruling b, docs/design/real_trace_campaign_20260705.md,
// 2026-07-08; huadong_01 §22 C-P1). Covers:
//   P-b  — miss → ONE soft-fail with a retry hint listing the numerals;
//          matched control → zero touch; exemption arms; one-round cap;
//          same-attempt recheck consistency; non-trace runs stay inert.
//   P-d① — the aggregate-facts channel carries an actual-family scalar and
//          the P-b evidence set consumes it.
//   e2e  — huadong-shaped fixture through runContractCheck: soft-fail one
//          round with the hint, rewritten emit passes, and a stubborn
//          re-emit of the same numeral ships anyway (cap).

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func psgTraceRecord(id, claimKey, value string, notes ...string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "huadong.systrace"},
		Span:            types.ObservationSpan{LineStart: 100, LineEnd: 200},
		ClaimKey:        claimKey,
		Subject:         "app.main-42591",
		Predicate:       strings.SplitN(claimKey, ":", 2)[0],
		Object:          "s_sleep",
		Value:           value,
		Unit:            "ms",
		RichNotes:       notes,
		SupportRefs:     []string{"huadong.systrace:100-200"},
	}
}

func psgTraceMutable(records ...types.ObservationRecord) *types.MutableState {
	mut := types.NewMutableState("分析滑动卡顿")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: records,
	}}})
	return mut
}

func psgBus(mut *types.MutableState) *types.BusContext {
	return &types.BusContext{Ctx: context.Background(), Mutable: mut, Language: "zh"}
}

func psgProseDoc(prose string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: prose,
	}}}
}

func psgViolations(vs []types.Violation) []types.Violation {
	var out []types.Violation
	for _, v := range vs {
		if v.Kind == types.ViolProseScalarUngrounded {
			out = append(out, v)
		}
	}
	return out
}

// TestProseScalarGrounding_MissRaisesRetryHint — P-b miss arm: a fabricated
// 46.821ms-class prose numeral with no evidence-face member raises exactly
// one violation whose hint lists the numeral and gives the two allowed
// dispositions; the violation is strict (retry-eligible) for the bus gate.
func TestProseScalarGrounding_MissRaisesRetryHint(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:app.main-42591:s_sleep", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("聚合影响达 46.821ms，显著延长了端到端延迟。")

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 {
		t.Fatalf("miss must raise exactly one violation, got %d: %+v", len(got), got)
	}
	v := got[0]
	if v.Kind != types.ViolProseScalarUngrounded {
		t.Fatalf("kind = %q", v.Kind)
	}
	if !strings.Contains(v.Detail, "46.821ms") || !strings.Contains(v.Detail, `block "summary"`) {
		t.Fatalf("Detail must list the unmatched numeral with its block id:\n%s", v.Detail)
	}
	// Hint wording pin (LLM-facing; zero internal names): the two allowed
	// dispositions — cite the source view + window, or remove the number.
	if !strings.Contains(v.Repair, "source view and time window") ||
		!strings.Contains(v.Repair, "remove the number") {
		t.Fatalf("Repair must give the cite-or-remove guidance:\n%s", v.Repair)
	}
	if !isStrictViolationForBus(v, bus) {
		t.Fatalf("the single PSG raise must be retry-eligible via the bus strict arm")
	}
}

// TestProseScalarGrounding_MatchedZeroTouch — control: numerals present on
// the evidence face (record value, k=v note numerals, tolerance forms) never
// trigger.
func TestProseScalarGrounding_MatchedZeroTouch(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "wakeup_causal_aggregate:VSyncGenerator-2270",
		"30.986", "actual_impact=46.821ms", "actual_total=48.216ms", "running=1.590ms"))
	bus := psgBus(mut)
	doc := psgProseDoc("窗口1聚合影响 30.986ms（实际 46.821ms / 48.216ms），自身运行 1.59ms。")

	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("evidence-face members must not trigger, got %+v", got)
	}
	// 1.59 vs the published 1.590: last-digit tolerance arm. The check above
	// covers it inline; also pin the round-3 arm: 46.8213-class evidence
	// accepts a 46.821 prose form.
	mut2 := psgTraceMutable(psgTraceRecord("r2", "wakeup_causal_aggregate:x", "46.8213"))
	if got := runProseScalarGroundingCheck(psgProseDoc("实际影响 46.821ms。"), psgBus(mut2), mut2); len(got) != 0 {
		t.Fatalf("round-3 tolerance must accept 46.821 against 46.8213, got %+v", got)
	}
}

// TestProseScalarGrounding_ExemptionArms — the loose exemption arms: zero
// values, recomputable percentages (same-unit pair and ms/s pair), ratio
// form, pairwise ms sums, system-injected projection blocks as evidence,
// and citation quotes as evidence. 宁松勿严: clearing any of these arms
// would mis-flag a legitimate number, which costs more than a miss.
func TestProseScalarGrounding_ExemptionArms(t *testing.T) {
	base := psgTraceRecord("r1", "state_drilldown:app.main-42591:running",
		"2029.6", "window=3338", "fragments=1176")
	t.Run("zero_value", func(t *testing.T) {
		mut := psgTraceMutable(base)
		doc := psgProseDoc("其余节点为 0.000ms 占位。")
		if got := runProseScalarGroundingCheck(doc, psgBus(mut), mut); len(got) != 0 {
			t.Fatalf("zero values are exempt, got %+v", got)
		}
	})
	t.Run("percent_recompute_same_unit", func(t *testing.T) {
		mut := psgTraceMutable(base)
		doc := psgProseDoc("running 占比 60.8%。") // 2029.6/3338*100 = 60.803
		if got := runProseScalarGroundingCheck(doc, psgBus(mut), mut); len(got) != 0 {
			t.Fatalf("a percentage recomputable from an evidence pair is exempt, got %+v", got)
		}
	})
	t.Run("percent_recompute_ms_over_seconds", func(t *testing.T) {
		mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "2029.6", "window=3.338s"))
		doc := psgProseDoc("running 占比 60.8%。") // 2029.6/3.338*0.1
		if got := runProseScalarGroundingCheck(doc, psgBus(mut), mut); len(got) != 0 {
			t.Fatalf("an ms-over-seconds recomputable percentage is exempt, got %+v", got)
		}
	})
	t.Run("percent_ratio_form", func(t *testing.T) {
		mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "1", "share=0.608"))
		doc := psgProseDoc("占比 60.8%。")
		if got := runProseScalarGroundingCheck(doc, psgBus(mut), mut); len(got) != 0 {
			t.Fatalf("a 0-1 ratio evidence form covers the percent, got %+v", got)
		}
	})
	t.Run("ms_pairwise_sum", func(t *testing.T) {
		mut := psgTraceMutable(
			psgTraceRecord("r1", "wakeup_causal_impact:a", "4.115"),
			psgTraceRecord("r2", "wakeup_causal_impact:b", "2.770"),
		)
		doc := psgProseDoc("两窗合计 6.885ms。")
		if got := runProseScalarGroundingCheck(doc, psgBus(mut), mut); len(got) != 0 {
			t.Fatalf("an honest two-value sum is exempt, got %+v", got)
		}
	})
	t.Run("system_projection_block_is_evidence_not_prose", func(t *testing.T) {
		mut := psgTraceMutable(base)
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "跨窗合并行合计 63.831ms。"},
			{ID: "runtime_trace_causal_projection", Kind: types.BlockSection,
				Text: "├─ s_sleep ×14 63.831ms ▏63%"},
		}}
		if got := runProseScalarGroundingCheck(doc, psgBus(mut), mut); len(got) != 0 {
			t.Fatalf("projection-block numerals ground the prose AND are never scanned as prose, got %+v", got)
		}
	})
	t.Run("citation_quote_is_evidence", func(t *testing.T) {
		mut := psgTraceMutable(base)
		doc := psgProseDoc("超时阈值 300ms。")
		doc.Citations = []types.Citation{{File: "config/app.go", Line: 12, Quote: "timeoutMs = 300"}}
		if got := runProseScalarGroundingCheck(doc, psgBus(mut), mut); len(got) != 0 {
			t.Fatalf("citation-quote numerals ground the prose, got %+v", got)
		}
	})
	t.Run("caveat_and_diagram_blocks_not_scanned", func(t *testing.T) {
		mut := psgTraceMutable(base)
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "结论。"},
			{ID: "warn", Kind: types.BlockCaveat, Text: "个别 12.345ms 级别数字未复核。"},
		}}
		if got := runProseScalarGroundingCheck(doc, psgBus(mut), mut); len(got) != 0 {
			t.Fatalf("caveat blocks stay out of this lane, got %+v", got)
		}
	})
}

// TestProseScalarGrounding_EvidenceFeedStaysTight — adversarial-review
// negative pins (PSG 收尾①, 2026-07-08): the evidence-FEED face admits only
// exact system spellings. Three laundering shapes must all still raise:
// a model-authored next_steps block repeating the fabricated numeral (the
// bare next_steps id is scan-exempt but NOT evidence), a model-authored
// runtime_trace_* lookalike id, and a rejected-draft recovery attachment.
// The skip face may be loose; the feed face must not.
func TestProseScalarGrounding_EvidenceFeedStaysTight(t *testing.T) {
	newMut := func() *types.MutableState {
		return psgTraceMutable(psgTraceRecord("r1", "state_drilldown:app.main-42591:s_sleep", "38.996"))
	}
	t.Run("model_next_steps_block_never_grounds", func(t *testing.T) {
		mut := newMut()
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "聚合影响 46.821ms。"},
			// Model-authored next-step lane (preserved content, scan-exempt)
			// repeating its own fabricated numeral — self-laundering shape.
			{ID: "next_steps", Kind: types.BlockBulletList, Items: []types.AnswerBlockItem{
				{ID: "n1", Text: "复核 46.821ms 聚合影响的来源视图"},
			}},
		}}
		got := runProseScalarGroundingCheck(doc, psgBus(mut), mut)
		if len(got) != 1 || !strings.Contains(got[0].Detail, "46.821ms") {
			t.Fatalf("a model next_steps block must not ground prose numerals, got %+v", got)
		}
	})
	t.Run("runtime_trace_lookalike_block_never_grounds", func(t *testing.T) {
		mut := newMut()
		doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "聚合影响 46.821ms。"},
			// Shares the prefix but is NOT a system spelling (the projection
			// guard's own lookalike class): must not enter the evidence set.
			{ID: "runtime_trace_causal_projection_extra", Kind: types.BlockSection,
				Text: "补充:聚合影响 46.821ms"},
		}}
		got := runProseScalarGroundingCheck(doc, psgBus(mut), mut)
		if len(got) != 1 || !strings.Contains(got[0].Detail, "46.821ms") {
			t.Fatalf("a runtime_trace_* lookalike block must not ground prose numerals, got %+v", got)
		}
	})
	t.Run("rejected_draft_attachment_never_grounds", func(t *testing.T) {
		mut := newMut()
		// Recovered rejected-draft content — model-authored text that failed
		// structured validation; outside the §25 evidence enumeration.
		mut.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{{
			Kind: types.AnswerDisplayAttachmentText, Body: "草稿:聚合影响 46.821ms",
		}})
		doc := psgProseDoc("聚合影响 46.821ms。")
		got := runProseScalarGroundingCheck(doc, psgBus(mut), mut)
		if len(got) != 1 || !strings.Contains(got[0].Detail, "46.821ms") {
			t.Fatalf("rejected-draft attachments must not ground prose numerals, got %+v", got)
		}
	})
}

// TestProseScalarGrounding_TokenWordBoundaries — extraction-level pins (PSG
// 收尾②③): identifier-glued numerals ("v1.5ms" tails), unit-glued words
// ("46.821msec"), and thousands-separated fragments ("1,234.5ms") are never
// extracted; the plain forms stay extracted (positive control). Removing any
// boundary guard turns this red.
func TestProseScalarGrounding_TokenWordBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"版本 v1.5ms 标识符尾不提取", 0},
		{"耗时 46.821msec 词粘连不提取", 0},
		{"共 1,234.5ms 千分位不提取", 0},
		{"耗时 46.821ms 正常提取", 1},
		{"占比 60.8% 正常提取", 1},
		{"耗时 46.821ms。句读收尾提取", 1},
	} {
		got := extractProseScalarTokens("b", tc.text)
		if len(got) != tc.want {
			t.Fatalf("extract(%q) = %d token(s) (%+v), want %d", tc.text, len(got), got, tc.want)
		}
	}
}

// TestProseScalarGrounding_OneRoundCap — the anti-livelock latch: once a
// dispatched retry surface carried the hint, the gate NEVER raises again
// this run — including after a later retry rebuilds the surface without the
// prose-scalar entry. Removing either latch half turns this red.
func TestProseScalarGrounding_OneRoundCap(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("聚合影响 46.821ms。")

	first := runProseScalarGroundingCheck(doc, bus, mut)
	if len(first) != 1 {
		t.Fatalf("first pass must raise, got %+v", first)
	}
	// The retry decision publishes the hint on the typed retry surface.
	mut.SetRetryState(&types.RetryState{Attempt: 1, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolProseScalarUngrounded, Severity: types.SeverityMedium, Layer: "answer_oracle",
	}}})
	// The retried emit STILL carries the numeral — the round is spent, the
	// answer ships (soft-gate nature: never a second raise).
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("the hint round is final for this lane, got %+v", got)
	}
	if !mut.ProseScalarGroundingHintDelivered() {
		t.Fatalf("observing the dispatched hint must set the sticky run latch")
	}
	// A LATER retry (other violation kinds) rebuilds the surface WITHOUT the
	// prose-scalar entry; the sticky latch still holds the cap.
	mut.SetRetryState(&types.RetryState{Attempt: 2, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolBlockCoverageMissing, Severity: types.SeverityHigh, Layer: "v2_oracle",
	}}})
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("the run-level latch caps the gate at one round total, got %+v", got)
	}
}

// TestProseScalarGrounding_SameAttemptRecheckConsistent — within ONE finalize
// attempt (no retry dispatched yet) the verdict is repeatable: the metadata
// auto-repair recheck must see the same violation, otherwise the hint would
// be dropped before any dispatch consumed it. Kills the "latch at raise
// time" mutation.
func TestProseScalarGrounding_SameAttemptRecheckConsistent(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	doc := psgProseDoc("聚合影响 46.821ms。")
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 1 {
		t.Fatalf("same-attempt recheck must re-raise consistently, got %+v", got)
	}
}

// TestProseScalarGrounding_InertWithoutTraceEvidence — scope gate: a run
// whose ledger carries no deterministic runtime-query observation never
// enters this lane, whatever the prose says.
func TestProseScalarGrounding_InertWithoutTraceEvidence(t *testing.T) {
	mut := types.NewMutableState("普通代码问题")
	bus := psgBus(mut)
	doc := psgProseDoc("该函数平均耗时 46.821ms，占比 60.8%。")
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("non-trace runs stay inert, got %+v", got)
	}
}

// TestProseScalarGrounding_AggregateFactChannelCarries — P-d① pin: the
// existing aggregate-facts channel (scalar_value kind + unit + provenance,
// 16-entry cap) structurally carries an actual-family scalar, and the P-b
// evidence set consumes it — no new role or field is needed.
func TestProseScalarGrounding_AggregateFactChannelCarries(t *testing.T) {
	fact := types.AnswerAggregateFact{
		Kind:       types.AnswerAggregateScalar,
		Label:      "窗口1 聚合实际影响",
		Value:      "46.821",
		Unit:       "ms",
		Role:       types.AnswerAggregateRoleSupportingCoverage,
		Provenance: "runtime_trace",
	}
	// Channel half: a full 16-entry payload containing the actual-family
	// scalar normalizes cleanly inside the cap.
	facts := []types.AnswerAggregateFact{fact}
	for i := 1; i < types.MaxAnswerAggregateFacts; i++ {
		facts = append(facts, types.AnswerAggregateFact{
			Kind: types.AnswerAggregateScalar, Label: fmt.Sprintf("其他标量 %d", i), Value: fmt.Sprintf("%d.5", i), Unit: "ms",
		})
	}
	normalized, err := types.NormalizeAnswerAggregateFacts(facts)
	if err != nil {
		t.Fatalf("16-entry payload with the actual-family scalar must normalize: %v", err)
	}
	if len(normalized) != types.MaxAnswerAggregateFacts {
		t.Fatalf("normalized count = %d, want %d", len(normalized), types.MaxAnswerAggregateFacts)
	}
	// Consumption half: the carried scalar grounds the prose numeral.
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	mut.SetInvestigationAggregateFacts(normalized)
	mut.SetInvestigationComplete("trace analysis finished")
	bus := psgBus(mut)
	doc := psgProseDoc("窗口1实际聚合影响 46.821ms。")
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("an aggregate-facts scalar is an evidence-face member, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// PSG-2 second arm pins (§24.14 B-3/D-2, cmp_78_01, 2026-07-08): the binding
// audit — a value that IS on the evidence face but is bound in the prose to
// a different window or thread than the one its evidence rows published it
// under. Witness shapes are taken from the audited comparison specimen:
//   B-3   — "sleep 占 63.6%（1430ms）" stated under the 1800ms window's name
//           while the 1430.101 snapshot row is the 2250ms window (and the
//           63.6% recomputes only against 2250).
//   D-2(b)— "runnable 占 2.1%（41ms）" under the 1645ms window; 40.766 was
//           published by the 2250ms-window row.
//   D-2(c)— "如 wk:1/1/0/8-6537 阻塞约 50ms"; the ~50ms row belongs to
//           binder:8815_1-6581.
// Controls pin the loose dispositions: consistent binding, no window/thread
// token in the sentence, and a window-silent carrier row all stay silent.

// psgSnapshot70 returns the two 7.0-side metric-snapshot rows (2250ms and
// 1800ms windows) in the system spelling the runtime materializer uses.
func psgSnapshot70() types.AnswerBlock {
	return types.AnswerBlock{
		ID: "runtime_trace_metric_snapshot", Kind: types.BlockSection, Text: strings.Join([]string{
			"- 查询窗 3680.569–3682.819s · demo.systrace · com.xs.fm.lite-6565 状态切换(state_churn) — running 518.776ms(26%) · runnable 21.939ms(1%) · sleep 1430.101ms(72%) · D-state 25.952ms(1%)",
			"- 查询窗 3680.819–3682.619s · demo.systrace · com.xs.fm.lite-6565 状态切换(state_churn) — running 460.975ms(26%) · runnable 20.047ms(1%) · sleep 1295.329ms(72%) · D-state 20.417ms(1%)",
		}, "\n"),
	}
}

func psgBindingDoc(prose string, evidence types.AnswerBlock) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: prose},
		evidence,
	}}
}

// TestProseScalarGrounding_BindingWindowMismatchB3 — §24.14 B-3 flagship
// shape: the prose repeats a 2250ms-window measurement (1430.101 → "1430ms")
// under the 1800ms window's name, and derives "63.6%" against the 2250
// denominator while the sentence names 1800. Both must raise as BINDING
// mismatches (not membership misses), naming the published window and the
// stated window; the single raise stays retry-eligible via the strict arm.
func TestProseScalarGrounding_BindingWindowMismatchB3(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	doc := psgBindingDoc(
		"在 bindApplication 窗口（3680.819~3682.619s，1800ms）内，线程 com.xs.fm.lite-6565 的状态分布：sleep 占 63.6%（1430ms），是绝对主导状态。",
		psgSnapshot70())

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 {
		t.Fatalf("B-3 shape must raise exactly one violation, got %d: %+v", len(got), got)
	}
	v := got[0]
	if v.Kind != types.ViolProseScalarUngrounded {
		t.Fatalf("kind = %q", v.Kind)
	}
	if strings.Contains(v.Detail, "match nothing") {
		t.Fatalf("both numerals ARE evidence members — the raise must be the binding class, not the membership class:\n%s", v.Detail)
	}
	if !strings.Contains(v.Detail, "binds 2 numeric value(s)") {
		t.Fatalf("Detail must report both misbound values (1430ms and 63.6%%):\n%s", v.Detail)
	}
	// The 1430ms entry names the row's window (2250ms span) AND the
	// sentence's window (1800ms).
	if !strings.Contains(v.Detail, "1430ms") ||
		!strings.Contains(v.Detail, "3680.569–3682.819s (≈2250ms)") ||
		!strings.Contains(v.Detail, "1800ms") {
		t.Fatalf("Detail must name the value, its published window and the stated window:\n%s", v.Detail)
	}
	// The 63.6% entry is the percent-recompute extension: it reproduces
	// only against the 2250 denominator.
	if !strings.Contains(v.Detail, "63.6%") || !strings.Contains(v.Detail, "recomputes only against window ≈2250ms") {
		t.Fatalf("the recompute extension must flag 63.6%% against the 2250 window:\n%s", v.Detail)
	}
	// Binding guidance rides the same repair string.
	if !strings.Contains(v.Repair, "restate the number under its own window and thread") ||
		!strings.Contains(v.Repair, "normalize each side by its own window length") {
		t.Fatalf("Repair must carry the binding + normalize-before-comparing guidance:\n%s", v.Repair)
	}
	if !isStrictViolationForBus(v, bus) {
		t.Fatalf("the binding raise must be retry-eligible via the bus strict arm")
	}
}

// TestProseScalarGrounding_BindingWindowMismatchD2b — §24.14 D-2(b): in the
// 6.0-side sentence naming the 1645ms window, only the 41ms figure (the
// 2250ms-window runnable 40.766) is misbound; the sibling figures published
// by the 1645ms row (834/493/283ms and the recomputable percentages) stay
// silent.
func TestProseScalarGrounding_BindingWindowMismatchD2b(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	snapshot := types.AnswerBlock{
		ID: "runtime_trace_metric_snapshot", Kind: types.BlockSection, Text: strings.Join([]string{
			"- 查询窗 8144.358–8146.608s · demo.systrace · com.xs.fm.lite-21538 状态切换(state_churn) — running 713.339ms(35%) · runnable 40.766ms(2%) · sleep 974.469ms(48%) · D-state 318.138ms(16%)",
			"- 查询窗 8144.608–8146.253s · demo.systrace · com.xs.fm.lite-21538 状态切换(state_churn) — running 493.447ms(30%) · runnable 32.941ms(2%) · sleep 833.006ms(51%) · D-state 282.693ms(17%)",
		}, "\n"),
	}
	doc := psgBindingDoc(
		"在 bindApplication 窗口（8144.608~8146.253s，1645ms）内，线程 com.xs.fm.lite-21538 的状态分布：sleep 占 50.7%（834ms）、running 占 30.0%（493ms）、d_state 占 17.2%（283ms）、runnable 占 2.1%（41ms）。",
		snapshot)

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 {
		t.Fatalf("D-2(b) shape must raise exactly one violation, got %d: %+v", len(got), got)
	}
	v := got[0]
	if !strings.Contains(v.Detail, "binds 1 numeric value(s)") ||
		!strings.Contains(v.Detail, "41ms") ||
		!strings.Contains(v.Detail, "8144.358–8146.608s") ||
		!strings.Contains(v.Detail, "1645ms") {
		t.Fatalf("only the 41ms figure is misbound (2250ms row stated under the 1645ms window):\n%s", v.Detail)
	}
	for _, silent := range []string{"834ms", "493ms", "283ms", "50.7%", "30.0%", "17.2%"} {
		if strings.Contains(v.Detail, silent) {
			t.Fatalf("%s is published by (or recomputes against) the sentence's own window and must stay silent:\n%s", silent, v.Detail)
		}
	}
}

// TestProseScalarGrounding_BindingThreadMismatchD2c — §24.14 D-2(c): the
// prose attributes a ~50ms wait to wk:1/1/0/8-6537 while the only evidence
// row carrying that value belongs to binder:8815_1-6581. Thread facet
// raises; tid equality (not name spelling) is the agreement test.
func TestProseScalarGrounding_BindingThreadMismatchD2c(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	table := types.AnswerBlock{
		ID: "runtime_trace_causal_projection", Kind: types.BlockSection,
		Columns: []string{"节点[E#]", "窗口投影", "有效归因", "置信"},
		Items: []types.AnswerBlockItem{
			{ID: "e18", Cells: []string{"binder:8815_1-6581 / runnable [E18(+1)]", "50.057ms", "50.057ms", "中"}},
			{ID: "e4", Cells: []string{"main-6565 / sleep（s_sleep） ×15(1.107–97.342ms) [E4(+14)]", "456.725ms", "97.342ms", "中"}},
		},
	}
	doc := psgBindingDoc(
		"主线程 main-6565 在等待唤醒时遭遇优先级反转，另有同步 binder 等待（如 wk:1/1/0/8-6537）阻塞约 50ms。",
		table)

	got := runProseScalarGroundingCheck(doc, bus, mut)
	if len(got) != 1 {
		t.Fatalf("D-2(c) shape must raise exactly one violation, got %d: %+v", len(got), got)
	}
	v := got[0]
	if !strings.Contains(v.Detail, "50ms") ||
		!strings.Contains(v.Detail, "published for thread binder:8815_1-6581") ||
		!strings.Contains(v.Detail, "wk:1/1/0/8-6537") {
		t.Fatalf("Detail must name the publishing thread and the stated thread:\n%s", v.Detail)
	}
}

// TestProseScalarGrounding_BindingConsistentControl — positive control: the
// same window-and-thread-naming sentence quoting values from THEIR OWN row
// (1800ms window, com.xs.fm.lite-6565) never triggers, including the
// derived window length and the row-published percentage.
func TestProseScalarGrounding_BindingConsistentControl(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	doc := psgBindingDoc(
		"在窗口（3680.819~3682.619s，1800ms）内，线程 com.xs.fm.lite-6565 的 sleep 为 1295.329ms（占 72%）。",
		psgSnapshot70())
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("consistently bound values must stay silent, got %+v", got)
	}
}

// TestProseScalarGrounding_BindingSameTidAliasControl — the thread
// agreement test is TID EQUALITY, never name spelling (adversarial-review
// probe A11a): trace surfaces publish the same thread under truncated or
// aliased names (main-6565 vs com.xs.fm.lite-6565), so a value published
// for foo.worker-6565 and stated in prose as bar.renamed-6565 (same tid,
// matching window) must stay silent. Mutating the agreement to raw-name
// equality turns this red.
func TestProseScalarGrounding_BindingSameTidAliasControl(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	snapshot := types.AnswerBlock{
		ID: "runtime_trace_metric_snapshot", Kind: types.BlockSection,
		Text: "- 查询窗 3680.819–3682.619s · demo.systrace · foo.worker-6565 状态切换(state_churn) — sleep 1295.329ms(72%)",
	}
	doc := psgBindingDoc(
		"在窗口（3680.819~3682.619s，1800ms）内，线程 bar.renamed-6565 的 sleep 为 1295.329ms。",
		snapshot)
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("same-tid aliased thread names must agree (tid equality, not spelling), got %+v", got)
	}
}

// TestProseScalarGrounding_BindingNoTokenControl — 句内无窗/主体 token 时不核:
// a sentence with no window/thread naming keeps membership sufficient, even
// for a value whose only carrier row belongs to a different window.
func TestProseScalarGrounding_BindingNoTokenControl(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	doc := psgBindingDoc("本窗口内 sleep 合计 1430ms，详见指标快照。", psgSnapshot70())
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("a sentence naming no window/thread is never binding-audited, got %+v", got)
	}
}

// TestProseScalarGrounding_BindingUnboundCarrierStaysLoose — loose
// discipline: when ANY carrier row is window-silent (here an accepted
// aggregate fact carrying the same 1430.101), the misbinding assertion is
// off the table — the B-3 sentence that raises without it stays silent.
func TestProseScalarGrounding_BindingUnboundCarrierStaysLoose(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	facts, err := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateScalar, Label: "唤醒链聚合睡眠合计",
		Value: "1430.101", Unit: "ms",
		Role: types.AnswerAggregateRoleSupportingCoverage, Provenance: "runtime_trace",
	}})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	mut.SetInvestigationAggregateFacts(facts)
	mut.SetInvestigationComplete("trace analysis finished")
	bus := psgBus(mut)
	doc := psgBindingDoc(
		"在窗口（3680.819~3682.619s，1800ms）内 sleep 合计 1430ms。",
		psgSnapshot70())
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("a window-silent carrier row must keep the token silent (宁松勿严), got %+v", got)
	}
}

// TestProseScalarGrounding_BindingSharesOneRoundLatch — the binding arm
// reuses the SAME one-round latch as the membership arm: within one attempt
// the verdict repeats, and once a dispatched retry surface carried the
// kind, the gate never raises again this run.
func TestProseScalarGrounding_BindingSharesOneRoundLatch(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	bus := psgBus(mut)
	doc := psgBindingDoc(
		"在 bindApplication 窗口（3680.819~3682.619s，1800ms）内，线程 com.xs.fm.lite-6565 的状态分布：sleep 占 63.6%（1430ms），是绝对主导状态。",
		psgSnapshot70())
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	// Same-attempt recheck stays consistent (latch never set at raise time).
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 1 {
		t.Fatalf("same-attempt recheck must re-raise, got %+v", got)
	}
	mut.SetRetryState(&types.RetryState{Attempt: 1, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolProseScalarUngrounded, Severity: types.SeverityMedium, Layer: "answer_oracle",
	}}})
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("the hint round is final for the binding arm too, got %+v", got)
	}
	if !mut.ProseScalarGroundingHintDelivered() {
		t.Fatalf("observing the dispatched hint must set the sticky run latch")
	}
	mut.SetRetryState(&types.RetryState{Attempt: 2, ActiveViolations: []types.ScoredViolation{{
		Kind: types.ViolBlockCoverageMissing, Severity: types.SeverityHigh, Layer: "v2_oracle",
	}}})
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 0 {
		t.Fatalf("the run-level latch caps both arms at one round total, got %+v", got)
	}
}

// TestProseScalarGrounding_HuadongFixtureEndToEnd — e2e through
// runContractCheck: a huadong-shaped trace run whose prose carries an
// unverifiable 46.821ms → the contract fails ONCE with the hint listing the
// numeral; after the retry decision, a rewritten emit (numeral removed)
// passes; and a stubborn emit keeping the numeral ALSO ships (one-round cap
// end to end).
func TestProseScalarGrounding_HuadongFixtureEndToEnd(t *testing.T) {
	mut := psgTraceMutable(
		psgTraceRecord("r1", "state_drilldown:app.main-42591:s_sleep", "38.996"),
		psgTraceRecord("r2", "wakeup_causal_impact:a", "4.115"),
	)
	doc := psgProseDoc("VSyncGenerator 两个热点窗口中聚合影响分别为 46.821ms 和 48.216ms。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)

	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = psgBus(mut)
	out := &agent.StageOutput{FinalAnswer: "答案正文"}

	res := runContractCheck(out, types.AnswerContract{}, mut, o, contractCheckSkipLLMReview())
	raised := psgViolations(res.Violations)
	if len(raised) != 1 {
		t.Fatalf("first finalize pass must soft-fail on the prose scalars, got %+v", res.Violations)
	}
	if !strings.Contains(raised[0].Detail, "46.821ms") || !strings.Contains(raised[0].Detail, "48.216ms") {
		t.Fatalf("the hint must list both unverifiable numerals:\n%s", raised[0].Detail)
	}
	if res.Passed {
		t.Fatalf("the single PSG round must be retry-eligible (Passed=false)")
	}

	// Retry decision publishes the hint (same call the scheduler makes).
	populateRetryState(mut, res, 0)

	// The model rewrites the prose without the unverifiable numerals.
	rewritten := psgProseDoc("VSyncGenerator 在两个热点窗口中的聚合影响见明细表；主线程 s_sleep 38.996ms。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, rewritten)
	res2 := runContractCheck(out, types.AnswerContract{}, mut, o, contractCheckSkipLLMReview())
	if got := psgViolations(res2.Violations); len(got) != 0 {
		t.Fatalf("the rewritten emit must pass this lane, got %+v", got)
	}
	if !res2.Passed {
		t.Fatalf("rewritten emit must pass the contract, violations: %+v", res2.Violations)
	}

	// Cap witness: even a stubborn emit that keeps the numeral ships — the
	// lane already spent its one round.
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, psgProseDoc("聚合影响仍为 46.821ms。"))
	res3 := runContractCheck(out, types.AnswerContract{}, mut, o, contractCheckSkipLLMReview())
	if got := psgViolations(res3.Violations); len(got) != 0 {
		t.Fatalf("the one-round cap must hold end to end, got %+v", got)
	}
}
