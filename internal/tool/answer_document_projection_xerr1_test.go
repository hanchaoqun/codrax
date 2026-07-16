package tool

// answer_document_projection_xerr1_test.go — XERR1-FIX display pins
// (§29.104.3/.4, 2026-07-15; local anchor §29.104.9 形④).
//
// The customer E1 lesion: a payload-less blocking_span row published the span
// WINDOW-ENVELOPE (199.992ms = 100% of the analysis window) wearing
// 「⊗ 阻塞等待(对端 RenderThread-48660)」 + 明细 「锁竞争·持锁」 while the
// same thread ran 54% of the window. These pins hold the display half of the
// fix: the basis-forked word faces (件2), the ⚠ budget disclosure (件3) and
// the blocking↔sleep 互指 (件1). Values mirror the LOCAL tieba anchor
// (donghu_tieba_frame.systrace, window 34579.540..580, tid 8091: envelope
// 29.843 → Σ(sleep 4.084 + D 2.553 + io 0) = 6.637, non-running 16.572,
// running 13.271).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// xerr1WaitSegmentsProjection is the converged tieba-anchor shape: the
// payload-less blocking row (basis wait_segments) beside the SAME thread's
// sleep window seat (互指 pair) on the on-chain tier.
func xerr1WaitSegmentsProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"OS_FFRT_2_142-61764", "os.FusionSearch-8091"},
		WindowStartTs: 34579.540,
		WindowEndTs:   34579.580,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "xerr1-bspan",
				Subject: "os.FusionSearch-8091", Object: "OS_FFRT_2_142-61764",
				TypeToken: "blocking_span", Predicate: "critical_blocking",
				ChainRelevance: "on_chain",
				ImpactMS:       6.637, CumulativeImpactMS: 6.637, EffectiveImpactMS: 6.637,
				StartTs: 34579.544452, EndTs: 34579.574295,
				LineStart: 63, LineEnd: 64, Confidence: 0.62,
				BlockingValueBasis:             "wait_segments",
				BlockingWaitSegmentMS:          6.637,
				BlockingWaitSleepMS:            4.084,
				BlockingSpanEnvelopeMS:         29.843,
				BlockingWaitBudgetExceeded:     true,
				BlockingWaitBudgetNonRunningMS: 16.572,
				BlockingWaitBudgetRunningMS:    13.271,
			},
			{
				// The thread's own sleep window seat — the 互指 counterpart
				// (interval contains the blocking row's span∩window interval).
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "xerr1-sleep",
				Subject: "os.FusionSearch-8091", Object: "sleep_wait",
				TypeToken: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain",
				ImpactMS:       12.5, CumulativeImpactMS: 12.5, EffectiveImpactMS: 12.5,
				StartTs: 34579.540, EndTs: 34579.580,
				LineStart: 10, LineEnd: 200, Confidence: 0.8,
			},
		},
	}
}

// xerr1WaitSegmentsLegendProjection / xerr1SpanEnvelopeLegendProjection are
// the NON-self twins for the representative legend sweep: the subject is not
// the analysis target, so the rows render on the bar lane (the self-stanza
// rows of the face-pin fixtures draw no bar, which would strand the header's
// 满格=窗口 token without its mark).
func xerr1WaitSegmentsLegendProjection() types.TraceCausalProjection {
	projection := xerr1WaitSegmentsProjection()
	projection.WakeupPath = []string{"os.FusionSearch-8091", "app-100"}
	return projection
}

func xerr1SpanEnvelopeLegendProjection() types.TraceCausalProjection {
	projection := xerr1SpanEnvelopeProjection()
	projection.WakeupPath = []string{"app-42", "main-9"}
	return projection
}

// xerr1SpanEnvelopeProjection is the 词面退路 shape: convergence impossible,
// the value stays the envelope and the word face must not claim a blocking
// wait.
func xerr1SpanEnvelopeProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-77", "app-42"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "xerr1-env",
			Subject: "app-42", Object: "waker-77",
			TypeToken: "blocking_span", Predicate: "critical_blocking",
			ChainRelevance: "on_chain",
			ImpactMS:       50.0, CumulativeImpactMS: 50.0, EffectiveImpactMS: 50.0,
			StartTs: 100.05, EndTs: 100.1,
			LineStart: 30, LineEnd: 40, Confidence: 0.62,
			BlockingValueBasis:     "span_envelope",
			BlockingSpanEnvelopeMS: 50.0,
		}},
	}
}

// TestXERR1WaitSegmentsRowFace — 件1/件2/件3 display pins on the converged
// tieba-anchor shape: the ⊖ glyph replaces ⊗, the peer word demotes to
// span 期间最后唤醒者(推断), the ⚠ budget line discloses X>Y with running Z,
// the detail 值口径 line speaks Σ + envelope, and the blocking↔sleep 互指
// sentences ride both rows. 负向: no ⊗, no 锁竞争·持锁, and the envelope
// 29.843 never appears as the row value.
func TestXERR1WaitSegmentsRowFace(t *testing.T) {
	projection := xerr1WaitSegmentsProjection()
	node := projection.OnChainCauses[0]
	if form := runtimeTraceProjImpactFormForNode(node, ""); form != runtimeTraceProjImpactFormBlockingWait {
		t.Fatalf("converged payload-less blocking_span must classify onto the blocking-wait form, got %d", form)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var rowLine string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "6.637ms") {
			rowLine = line
			break
		}
	}
	if rowLine == "" {
		t.Fatalf("fixture drifted: no rendered converged blocking row:\n%s", fence)
	}
	if !strings.Contains(rowLine, "⊖") {
		t.Fatalf("converged row must wear the ⊖ blocking-wait glyph: %q", rowLine)
	}
	if strings.Contains(rowLine, "⊗") {
		t.Fatalf("件2: the ⊗ lock glyph is withdrawn from payload-less rows: %q", rowLine)
	}
	if !strings.Contains(fence, "阻塞等待(span 期间最后唤醒者(推断) OS_FFRT_2_142-61764)") {
		t.Fatalf("件2: the peer word must demote to span 期间最后唤醒者(推断):\n%s", fence)
	}
	if strings.Contains(fence, "阻塞等待(对端 OS_FFRT_2_142-61764)") {
		t.Fatalf("件2: the payload-confirmed 对端 word must not dress a wakeup-edge sample:\n%s", fence)
	}
	if strings.Contains(fence, "锁竞争·持锁") {
		t.Fatalf("件2: 锁竞争·持锁 must not dress a payload-less row:\n%s", fence)
	}
	if !strings.Contains(fence, "⚠ span 包络 29.843ms > 窗内非 running 16.572ms:含 running 13.271ms,非阻塞等待段") {
		t.Fatalf("件3: the budget ⚠ disclosure line is missing:\n%s", fence)
	}
	// 件1 互指: both directions.
	if !strings.Contains(fence, "等待段含 sleep 分量,与[E2]自身 sleep 席同段物理时间(两账口径不同,不可相加)") {
		t.Fatalf("件1: the blocking→sleep 互指 sentence is missing:\n%s", fence)
	}
	if !strings.Contains(fence, "阻塞等待行[E1]的等待段落在本席同段物理时间(不可相加)") {
		t.Fatalf("件1: the sleep→blocking back-pointer is missing:\n%s", fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "等待段合计(span∩窗内 sleep+D+iowait=6.637ms;span 包络 29.843ms 含运行,非阻塞等待值)") {
		t.Fatalf("件1: the detail 值口径 line is missing:\n%s", detail)
	}
	if !strings.Contains(detail, "⚠ span 包络 29.843ms > 窗内非 running 16.572ms:含 running 13.271ms,非阻塞等待段") {
		t.Fatalf("件3: the detail 等待预算核查 line is missing:\n%s", detail)
	}
	// EN faces carry the same forks (zh 主/EN 槽逐核).
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := runtimeTraceProjTreeFence(modelEN, false)
	if !strings.Contains(fenceEN, "blocking wait (last waker during span, inferred: OS_FFRT_2_142-61764)") {
		t.Fatalf("件2 EN: demoted peer word missing:\n%s", fenceEN)
	}
	// The fence soft-wraps long EN lines — probe the head (pre-wrap) only.
	if !strings.Contains(fenceEN, "⚠ span envelope 29.843ms > in-window non-running 16.572ms") {
		t.Fatalf("件3 EN: budget line missing:\n%s", fenceEN)
	}
}

// TestXERR1SpanEnvelopeRowFace — 件2 词面退路: convergence impossible, the
// row wears ⊓ + span 包络(含运行) and never claims a blocking wait.
func TestXERR1SpanEnvelopeRowFace(t *testing.T) {
	projection := xerr1SpanEnvelopeProjection()
	node := projection.OnChainCauses[0]
	if form := runtimeTraceProjImpactFormForNode(node, ""); form != runtimeTraceProjImpactFormSpanEnvelope {
		t.Fatalf("envelope-basis payload-less blocking_span must classify onto the span-envelope form, got %d", form)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⊓") {
		t.Fatalf("envelope row must wear the ⊓ glyph:\n%s", fence)
	}
	if !strings.Contains(fence, "span 包络(含运行;span 期间最后唤醒者(推断) waker-77)") {
		t.Fatalf("件2: the envelope word face is missing:\n%s", fence)
	}
	if strings.Contains(fence, "阻塞等待") {
		t.Fatalf("件2 词面退路: an envelope row must not claim 阻塞等待 anywhere:\n%s", fence)
	}
	if strings.Contains(fence, "⊗") || strings.Contains(fence, "锁竞争") {
		t.Fatalf("件2: no lock-family word/glyph on the envelope row:\n%s", fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "span 包络 50.000ms(含运行;span 窗内无该线程时间线,等待段不可得,非阻塞等待实测)") {
		t.Fatalf("件1: the envelope 值口径 detail line is missing:\n%s", detail)
	}
}

// TestXERR1LockFamilyWordForksOnHolder — 件2 顺手修 (§29.104.4 ②, rcr.go 词条
// bug): the lock family's C7 family word forks on BlockingSubjectIsHolder —
// waiter rows never wear the HOLDER word.
func TestXERR1LockFamilyWordForksOnHolder(t *testing.T) {
	waiter := types.TraceCausalProjectionNode{
		TypeToken: "blocking_span", BlockingKind: "monitor_contention",
	}
	if got := runtimeTraceProjImpactFormFamilyWord(waiter, true); got != "锁竞争·阻塞" {
		t.Fatalf("waiter lock row family word must be 锁竞争·阻塞, got %q", got)
	}
	if got := runtimeTraceProjImpactFormFamilyWord(waiter, false); got != "lock contention · blocked" {
		t.Fatalf("waiter lock row EN family word drifted: %q", got)
	}
	holder := waiter
	holder.BlockingSubjectIsHolder = true
	if got := runtimeTraceProjImpactFormFamilyWord(holder, true); got != "锁竞争·持锁" {
		t.Fatalf("holder lock row family word must stay 锁竞争·持锁, got %q", got)
	}
}

// TestXERR1LegacyBasisLessRowKeepsLockFamily — fail-open 负向 pin: a
// payload-less blocking_span node WITHOUT the typed basis (legacy artifact)
// keeps the UXR-1 §29.36.1 lock-family classification byte-identically.
func TestXERR1LegacyBasisLessRowKeepsLockFamily(t *testing.T) {
	node := types.TraceCausalProjectionNode{TypeToken: "blocking_span"}
	if form := runtimeTraceProjImpactFormForNode(node, ""); form != runtimeTraceProjImpactFormLock {
		t.Fatalf("legacy basis-less blocking_span must keep the lock family, got %d", form)
	}
	if kind := runtimeTraceCausalProjectionResolvedPeerKind(node); kind != "blocking_span" {
		t.Fatalf("legacy basis-less kind must stay blocking_span, got %q", kind)
	}
}

// TestXERR1FixBBasisConstantMirrorsEngine — 件B wire-constant pin: the
// types-side R2-exemption gate and the engine's typed basis value are the
// SAME wire string (types cannot import tracequery, so the mirror equality
// is pinned here where both packages are visible).
func TestXERR1FixBBasisConstantMirrorsEngine(t *testing.T) {
	if types.TraceCausalProjectionBlockingBasisWaitSegments != tracequery.BlockingValueBasisWaitSegments {
		t.Fatalf("件B: the types-side basis mirror drifted: %q vs %q",
			types.TraceCausalProjectionBlockingBasisWaitSegments, tracequery.BlockingValueBasisWaitSegments)
	}
}

// xerr1FenceZH renders the zh tree fence for a projection (件E/件F/件G
// helper).
func xerr1FenceZH(projection types.TraceCausalProjection) string {
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	return runtimeTraceProjTreeFence(model, true)
}

// TestXERR1FixEMutualPointerNegativeArms — 件E (对抗复核 P3-1): the 件1 互指
// pair rule's three negative branches, each pinned to render NOTHING (宁漏勿
// 假指):
//
//	① ≥2 containing sleep seats → ambiguous, the whole pair skips;
//	② sleep seat without typed endpoints → endpoints prove nothing, skip;
//	③ sleep seat whose interval does NOT contain the blocking interval →
//	  containment is the proof, skip.
func TestXERR1FixEMutualPointerNegativeArms(t *testing.T) {
	assertNoPointers := func(name, fence string) {
		t.Helper()
		if strings.Contains(fence, "等待段含 sleep 分量") || strings.Contains(fence, "阻塞等待行[") {
			t.Fatalf("件E %s: the 互指 pair must skip whole:\n%s", name, fence)
		}
	}
	// ① ambiguity: a SECOND sleep seat that also contains the blocking
	// interval — two candidates, no pick.
	ambiguous := xerr1WaitSegmentsProjection()
	second := ambiguous.OnChainCauses[1]
	second.EvidenceID = "xerr1-sleep-2"
	second.StartTs, second.EndTs = 34579.542, 34579.578
	second.ImpactMS, second.CumulativeImpactMS, second.EffectiveImpactMS = 11.0, 11.0, 11.0
	second.LineStart, second.LineEnd = 12, 190
	ambiguous.OnChainCauses = append(ambiguous.OnChainCauses, second)
	assertNoPointers("①≥2候选", xerr1FenceZH(ambiguous))
	// ② missing endpoints on the sleep seat.
	endpointless := xerr1WaitSegmentsProjection()
	endpointless.OnChainCauses[1].StartTs = 0
	endpointless.OnChainCauses[1].EndTs = 0
	assertNoPointers("②端点缺失", xerr1FenceZH(endpointless))
	// ③ non-containment: the sleep seat starts AFTER the blocking row's
	// interval head.
	nonContaining := xerr1WaitSegmentsProjection()
	nonContaining.OnChainCauses[1].StartTs = 34579.550
	assertNoPointers("③区间非包含", xerr1FenceZH(nonContaining))
}

// TestXERR1FixFCoverageCheckDetailLine — 件F display pins: the typed
// partial-coverage marker renders the detail 覆盖核查 line with the covered
// account beside the span window (zh/EN), and its absence renders nothing.
func TestXERR1FixFCoverageCheckDetailLine(t *testing.T) {
	projection := xerr1WaitSegmentsProjection()
	node := &projection.OnChainCauses[0]
	node.BlockingWaitCoveragePartial = true
	node.BlockingWaitAccountCoveredMS = 25.0
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "等待段账目未满覆盖 span 窗(账目 25.000ms/span 窗 29.843ms):收敛值为已证下界") {
		t.Fatalf("件F: the detail 覆盖核查 line is missing:\n%s", detail)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	detailEN := runtimeTraceProjDetailFullText(modelEN, false)
	if !strings.Contains(detailEN, "the wait-segment account covers only 25.000ms of the 29.843ms span window: the converged value is a proven lower bound") {
		t.Fatalf("件F EN: the coverage-check line is missing:\n%s", detailEN)
	}
	// 负向: the unmarked fixture renders no coverage line (absence renders
	// nothing).
	clean := xerr1WaitSegmentsProjection()
	cleanModel := buildRuntimeTraceProjTreeModel(clean, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if strings.Contains(runtimeTraceProjDetailFullText(cleanModel, true), "已证下界") {
		t.Fatalf("件F 负向: no coverage line without the typed marker")
	}
}

// xerr1HolderSubjectBudgetProjection is the 件G② shape: a holder-subject
// rank record (payload-typed lock row) carrying the twin-ported budget trio —
// the budget describes the WAITER (the record's peer), and the ⚠ sentence
// must say so.
func xerr1HolderSubjectBudgetProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waiter-100", "holder-200"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "xerr1-holder",
			Subject: "holder-200", Object: "waiter-100",
			TypeToken: "blocking_span", Predicate: "critical_blocking",
			BlockingKind:            "monitor_contention",
			BlockingSubjectIsHolder: true,
			ChainRelevance:          "on_chain",
			ImpactMS:                6.0, CumulativeImpactMS: 6.0, EffectiveImpactMS: 6.0,
			StartTs: 100.05, EndTs: 100.1,
			LineStart: 30, LineEnd: 40, Confidence: 0.62,
			BlockingWaitBudgetExceeded:     true,
			BlockingWaitBudgetNonRunningMS: 2.5,
			BlockingWaitBudgetRunningMS:    3.5,
		}},
	}
}

// xerr1CollapseFence strips whitespace and the box-drawing continuation
// glyph so a soft-wrapped fence line can be probed as one contiguous
// sentence (the fence wraps long zh/EN lines mid-word; collapse both sides
// of the comparison identically).
func xerr1CollapseFence(s string) string {
	return strings.NewReplacer(" ", "", "\n", "", "│", "").Replace(s)
}

// TestXERR1FixGHolderSubjectBudgetVariant — 件G② display pin: on a
// holder-subject record the ⚠ budget line carries the 「(预算主体=等待方)」
// referent annotation on both faces (fence row + detail), zh/EN; the
// waiter-subject face never wears it (负向 on the wait-segments fixture).
func TestXERR1FixGHolderSubjectBudgetVariant(t *testing.T) {
	projection := xerr1HolderSubjectBudgetProjection()
	fence := xerr1FenceZH(projection)
	want := "⚠ span 包络 6.000ms > 窗内非 running 2.500ms:含 running 3.500ms,非阻塞等待段(预算主体=等待方)"
	if !strings.Contains(xerr1CollapseFence(fence), xerr1CollapseFence(want)) {
		t.Fatalf("件G②: the holder-subject ⚠ variant is missing from the fence:\n%s", fence)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if detail := runtimeTraceProjDetailFullText(model, true); !strings.Contains(detail, want) {
		t.Fatalf("件G②: the holder-subject ⚠ variant is missing from the detail face:\n%s", detail)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := runtimeTraceProjTreeFence(modelEN, false)
	if !strings.Contains(xerr1CollapseFence(fenceEN), xerr1CollapseFence("not blocking-wait segments (budget subject = the waiter)")) {
		t.Fatalf("件G② EN: the annotated budget line is missing:\n%s", fenceEN)
	}
	// 负向: the waiter-subject converged row keeps the plain ⚠ sentence.
	waiterFence := xerr1CollapseFence(xerr1FenceZH(xerr1WaitSegmentsProjection()))
	if !strings.Contains(waiterFence, "非阻塞等待段") || strings.Contains(waiterFence, "预算主体=等待方") {
		t.Fatalf("件G② 负向: the waiter-subject face must not wear the holder annotation:\n%s", waiterFence)
	}
}

// TestXERR1FixGTwinPortCarriesBudgetTrio — 件G② note pin: the BLK §15.C
// twin-port lane carries the budget-sanity trio onto the surviving
// holder-subject rank record (随行), and zero-drops it when the marker never
// minted.
func TestXERR1FixGTwinPortCarriesBudgetTrio(t *testing.T) {
	notes := traceQueryTypedLockTwinPortNotes(tracequery.CriticalBlockingCandidate{
		WaitBudgetExceeded: true, WaitBudgetNonRunningMs: 2.5, WaitBudgetRunningMs: 3.5,
	})
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"blocking_wait_budget_exceeded=true",
		"blocking_wait_budget_non_running_ms=2.500",
		"blocking_wait_budget_running_ms=3.500",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("件G②: the twin-port must carry %q, got:\n%s", want, joined)
		}
	}
	empty := strings.Join(traceQueryTypedLockTwinPortNotes(tracequery.CriticalBlockingCandidate{}), "\n")
	if strings.Contains(empty, "blocking_wait_budget") {
		t.Fatalf("件G② 负向: an unminted marker must zero-drop from the twin-port: %s", empty)
	}
}
