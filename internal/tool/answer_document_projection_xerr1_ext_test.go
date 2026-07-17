package tool

// answer_document_projection_xerr1_ext_test.go — XERR1-EXT display pins
// (§29.104.17 裁定⑤, 2026-07-16).
//
// The engine now converges payload-TYPED contention rows' values (basis
// wait_segments riding the wire). The 件2 display contract for payload rows:
// the LOCK word family (⊗ glyph, 锁竞争·阻塞/持锁, 对端/holder vocabulary,
// LOCKNS presence clauses) stays byte-stable — ONLY the value-caliber detail
// line, the ⚠ budget line and the coverage line follow the typed basis. The
// basis-keyed kind/form forks stay payload-less-only, and the blocking↔sleep
// 互指 arm skips payload rows (宁漏勿假指: their converged Σ windows on the
// fold value-winner interval, which is not on the wire).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// xerr1ExtPayloadProjection is the converged donghu 形A shape: a waiter-subject
// payload lock row carrying the full basis lane, beside the SAME thread's
// containing sleep seat (the pair the 互指 arm must SKIP on payload rows).
func xerr1ExtPayloadProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"ransmitThread-18130", "LegoHandler-17585"},
		WindowStartTs: 13762.9805,
		WindowEndTs:   13762.9820,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ext-lock",
				Subject: "LegoHandler-17585", Object: "ransmitThread-18130",
				TypeToken: "blocking_span", Predicate: "critical_blocking",
				BlockingKind:   "monitor_contention",
				ChainRelevance: "on_chain",
				ImpactMS:       0.185, CumulativeImpactMS: 0.185, EffectiveImpactMS: 0.185,
				StartTs: 13762.980917, EndTs: 13762.981212,
				LineStart: 21605, LineEnd: 21644, Confidence: 0.67,
				BlockingValueBasis:             "wait_segments",
				BlockingWaitSegmentMS:          0.185,
				BlockingWaitSleepMS:            0.185,
				BlockingSpanEnvelopeMS:         0.295,
				BlockingWaitBudgetExceeded:     true,
				BlockingWaitBudgetNonRunningMS: 0.214,
				BlockingWaitBudgetRunningMS:    0.081,
			},
			{
				// The waiter's own sleep window seat CONTAINING the lock row's
				// interval — the shape that WOULD pair on a payload-less row.
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ext-sleep",
				Subject: "LegoHandler-17585", Object: "sleep_wait",
				TypeToken: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain",
				ImpactMS:       0.9, CumulativeImpactMS: 0.9, EffectiveImpactMS: 0.9,
				StartTs: 13762.9805, EndTs: 13762.9820,
				LineStart: 21500, LineEnd: 21700, Confidence: 0.8,
			},
		},
	}
}

// TestXERR1EXTPayloadRowKeepsLockWordFace — 件2: the basis-carrying payload
// row keeps the LOCK word family on every classifying fork — form family,
// C7 family word, shape cell, peer-relation kind — and never wears the ⊖/⊓
// payload-less vocabulary.
func TestXERR1EXTPayloadRowKeepsLockWordFace(t *testing.T) {
	node := xerr1ExtPayloadProjection().OnChainCauses[0]
	if form := runtimeTraceProjImpactFormForNode(node, ""); form != runtimeTraceProjImpactFormLock {
		t.Fatalf("件2: a basis-carrying payload row must stay in the lock form family, got %d", form)
	}
	if got := runtimeTraceProjImpactFormFamilyWord(node, true); got != "锁竞争·阻塞" {
		t.Fatalf("件2: the waiter lock family word must hold, got %q", got)
	}
	if cell := runtimeTraceCausalProjectionImpactShapeCell(node, true); cell != "锁竞争·阻塞" {
		t.Fatalf("件2: the shape cell must hold, got %q", cell)
	}
	if kind := runtimeTraceCausalProjectionResolvedPeerKind(node); kind != "blocking_span" {
		t.Fatalf("件2: the peer-relation kind fork is payload-less-only — payload rows keep the legacy kind, got %q", kind)
	}
	if _, ok := runtimeTraceProjBlockingSpanBasisImpactForm(node); ok {
		t.Fatalf("件2: the basis impact-form fork must decline payload rows")
	}
	// Fence sanity: no ⊖/⊓ glyph and no payload-less demoted-peer word may
	// dress the payload row.
	model := buildRuntimeTraceProjTreeModel(xerr1ExtPayloadProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "⊖") || strings.Contains(fence, "⊓") {
		t.Fatalf("件2: payload rows must not wear the payload-less basis glyphs:\n%s", fence)
	}
}

// TestXERR1EXTPayloadRowValueBasisDetailLine — 件2: the detail 值口径 line and
// the ⚠ budget line render off the typed basis for payload rows (zh + EN),
// with the true envelope figure (never the converged value dressed as the
// envelope).
func TestXERR1EXTPayloadRowValueBasisDetailLine(t *testing.T) {
	projection := xerr1ExtPayloadProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "等待段合计(span∩窗内 sleep+D+iowait=0.185ms;span 包络 0.295ms 含运行,非阻塞等待值)") {
		t.Fatalf("件2: the payload row's detail 值口径 line is missing:\n%s", detail)
	}
	if !strings.Contains(detail, "⚠ span 包络 0.295ms > 窗内非 running 0.214ms:含 running 0.081ms,非阻塞等待段") {
		t.Fatalf("件3: the payload row's ⚠ budget line must carry the TRUE envelope figure:\n%s", detail)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	detailEN := runtimeTraceProjDetailFullText(modelEN, false)
	if !strings.Contains(detailEN, "wait-segment total (sleep+D+iowait inside span∩window = 0.185ms; the span envelope 0.295ms contains run time and is not a blocking-wait value)") {
		t.Fatalf("件2 EN: the payload row's value-basis line is missing:\n%s", detailEN)
	}
}

// TestXERR1EXTPayloadRowSkipsSleepMutualPointer — 宁漏勿假指: the payload
// row's converged Σ windows on the fold value-winner interval, which is NOT
// on the wire — the containment proof would read the survivor's display
// interval, so payload rows skip the blocking↔sleep 互指 pair entirely (the
// containing sleep seat sits right there and must stay unpaired).
func TestXERR1EXTPayloadRowSkipsSleepMutualPointer(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(xerr1ExtPayloadProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "等待段含 sleep 分量") || strings.Contains(fence, "阻塞等待行[") {
		t.Fatalf("宁漏勿假指: the 互指 pair must skip payload rows:\n%s", fence)
	}
}

// TestXERR1EXTHolderSubjectValueBasisReferent — 件G② discipline extended: on
// a holder-subject rank record the 值口径 account describes the WAITER (the
// record's peer) — the line carries the per-lane referent annotation; the
// waiter-subject face never wears it.
func TestXERR1EXTHolderSubjectValueBasisReferent(t *testing.T) {
	projection := xerr1ExtPayloadProjection()
	node := &projection.OnChainCauses[0]
	node.Subject, node.Object = node.Object, node.Subject
	node.BlockingSubjectIsHolder = true
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "非阻塞等待值)(账目主体=等待方)") {
		t.Fatalf("EXT: the holder-subject 值口径 line must carry the referent annotation:\n%s", detail)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	detailEN := runtimeTraceProjDetailFullText(modelEN, false)
	if !strings.Contains(detailEN, "not a blocking-wait value) (account subject = the waiter)") {
		t.Fatalf("EXT EN: the holder-subject referent annotation is missing:\n%s", detailEN)
	}
	// 负向: the waiter-subject face keeps the plain line (no referent).
	clean := buildRuntimeTraceProjTreeModel(xerr1ExtPayloadProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if detail := runtimeTraceProjDetailFullText(clean, true); strings.Contains(detail, "账目主体=等待方") {
		t.Fatalf("EXT 负向: the waiter-subject face must not wear the referent:\n%s", detail)
	}
}

// TestXERR1EXTPayloadCoverageLineStaysNumberless — 件F on payload rows: the
// convergence interval (fold value-winner) is not on the wire, so the 覆盖核查
// line keeps the numberless claim (不造数 — deriving a denominator from the
// display endpoints could print the wrong window on a folded row); the
// payload-less numeric form stays byte-identical (existing 件F pins).
func TestXERR1EXTPayloadCoverageLineStaysNumberless(t *testing.T) {
	projection := xerr1ExtPayloadProjection()
	node := &projection.OnChainCauses[0]
	node.BlockingWaitCoveragePartial = true
	node.BlockingWaitAccountCoveredMS = 0.25
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "等待段账目未满覆盖 span 窗:收敛值为已证下界") {
		t.Fatalf("件F payload: the numberless coverage claim is missing:\n%s", detail)
	}
	if strings.Contains(detail, "账目 0.250ms") {
		t.Fatalf("件F payload 不造数: no denominator may derive from display endpoints:\n%s", detail)
	}
}

// TestXERR1EXTLegacyPayloadRowRendersNoBasisLine — fail-open 负向 pin: a
// payload lock node WITHOUT the typed basis (legacy wire artifact) renders no
// 值口径 line and keeps the legacy lock face byte-identically.
func TestXERR1EXTLegacyPayloadRowRendersNoBasisLine(t *testing.T) {
	projection := xerr1ExtPayloadProjection()
	node := &projection.OnChainCauses[0]
	node.BlockingValueBasis = ""
	node.BlockingWaitSegmentMS = 0
	node.BlockingWaitSleepMS = 0
	node.BlockingSpanEnvelopeMS = 0
	node.BlockingWaitBudgetExceeded = false
	node.BlockingWaitBudgetNonRunningMS = 0
	node.BlockingWaitBudgetRunningMS = 0
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if strings.Contains(detail, "值口径") || strings.Contains(detail, "等待段合计") {
		t.Fatalf("fail-open: a basis-less payload row must render no 值口径 line:\n%s", detail)
	}
	if got := runtimeTraceCausalProjectionImpactShapeCell(projection.OnChainCauses[0], true); got != "锁竞争·阻塞" {
		t.Fatalf("fail-open: the legacy lock shape cell must hold, got %q", got)
	}
}

// TestXERR1EXTTwinPortCarriesBasisLane — the BLK §15.C twin-port lane carries
// the value-basis family onto the surviving holder-subject rank record, and
// zero-drops it when the twin never converged (legacy artifacts).
func TestXERR1EXTTwinPortCarriesBasisLane(t *testing.T) {
	notes := traceQueryTypedLockTwinPortNotes(tracequery.CriticalBlockingCandidate{
		BlockingValueBasis: tracequery.BlockingValueBasisWaitSegments,
		WaitSegmentMs:      0.185, WaitSleepMs: 0.185, SpanEnvelopeMs: 0.295,
		WaitCoveragePartial: true, WaitAccountCoveredMs: 0.25,
	})
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"blocking_value_basis=wait_segments",
		"blocking_wait_segment_ms=0.185",
		"blocking_wait_sleep_ms=0.185",
		"blocking_span_envelope_ms=0.295",
		"blocking_wait_coverage_partial=true",
		"blocking_wait_account_covered_ms=0.250",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("EXT: the twin-port must carry %q, got:\n%s", want, joined)
		}
	}
	empty := strings.Join(traceQueryTypedLockTwinPortNotes(tracequery.CriticalBlockingCandidate{}), "\n")
	for _, banned := range []string{"blocking_value_basis", "blocking_wait_segment_ms", "blocking_span_envelope_ms", "blocking_wait_coverage_partial"} {
		if strings.Contains(empty, banned) {
			t.Fatalf("EXT 负向: an unconverged twin must zero-drop the basis lane: %s", empty)
		}
	}
}
