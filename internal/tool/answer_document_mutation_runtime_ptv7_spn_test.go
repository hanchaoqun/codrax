package tool

// PTV7-SPN batch pins (§22 dim A, docs/design/real_trace_campaign_20260705.md,
// huadong_01 audit 2026-07-07):
//   F1 (P0) — generic span-name rows consume node.SpanName on all THREE
//        display faces (tree row / detail-table node cell / lossless block);
//        semantic-span rows keep their pre-PTV7 rendering (control).
//   F2 (P2) — the evidence-index audit summary carries span=<name> and its
//        truncation cuts at part boundaries (96-rune cap), never mid-token.
//   F4 (P2) — the detail-table node cell mirrors the C39 fold naming + zh
//        fallback (no more EN placeholder phrase on the zh panel; the EN
//        word itself is "(unnamed causal node)" since RUN2FIX-A CR-4).
//   F5 (P3) — diagnostic-lane rows (trace_gap 数据盲区) and all-zero ×N folds
//        wear the — no-value form (no 0.000ms bar, no candidate chip); the
//        trace_gap row carries the user-ruled inline disclosure + legend.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ptv7SpnE21Node mirrors the huadong_01 E21 shape at the node level: a GENERIC
// trace_span rank row (predicate=root_cause_tertiary — it can never pass the
// semantic-span gate) whose typed span_name note reached the display model.
func ptv7SpnE21Node() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E21",
		Subject: "oney.hmn.berlin-42591", Predicate: "root_cause_tertiary",
		Object: "trace_span", SpanName: "H:ReceiveVsync",
		ChainRelevance: "adjacent", Tier: "tertiary",
		Causality: "adjacent_to_wakeup_chain", Rank: 7,
		ImpactMS: 9.169, Confidence: 0.75,
	}
}

// ptv7SpnE21Record is the same shape at the WIRE level: the compile path must
// parse the span_name typed note into node.SpanName (the projection half of
// the F1 chain).
func ptv7SpnE21Record() types.ObservationRecord {
	return types.ObservationRecord{
		ID:              "trace_query:q1#root_cause_rank:7",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: types.ClaimGroundingHard,
		ClaimKey:        "root_cause_tertiary",
		Subject:         "oney.hmn.berlin-42591",
		Predicate:       "root_cause_tertiary",
		Object:          "trace_span",
		Value:           "9.169",
		Unit:            "ms",
		RichNotes: []string{
			"rank=7", "tier=tertiary", "type=trace_span",
			"causality=adjacent_to_wakeup_chain", "chain_relevance=adjacent",
			"span_name=H:ReceiveVsync",
		},
		SupportRefs: []string{"berlin.systrace:1013562-1016996"},
		Span:        types.ObservationSpan{LineStart: 1013562, LineEnd: 1016996},
		Confidence:  0.75,
	}
}

// --- F1: three faces consume the span name -----------------------------------

func TestPTV7SpnGenericSpanNameThreeFaces(t *testing.T) {
	node := ptv7SpnE21Node()
	// Face 1 — tree row (stanza/chain shared composer).
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true}
	if got := runtimeTraceProjRowName(row, true); got != "oney.hmn.berlin-42591 · H:ReceiveVsync(trace span)" {
		t.Fatalf("F1 tree row (zh) must show the real span name in the object slot: %q", got)
	}
	// RULE3-1 件8 (§29.182②, 2026-07-21): the EN garnish speaks the reader
	// word (trace span) — the raw token keeps the detail 类型/type seats.
	if got := runtimeTraceProjRowName(row, false); got != "oney.hmn.berlin-42591 · H:ReceiveVsync(trace span)" {
		t.Fatalf("F1 tree row (en) must show the real span name: %q", got)
	}
	// Face 2 — detail-table node cell. EVOLUTION RECORD (用户裁定 2026-07-07:
	// trace 是专用名词不翻译,"跟踪span"→"trace span"): the composite
	// "H:ReceiveVsync(trace span)" is 26 runes > the 22-rune cell budget, so
	// the cell keeps the BARE real name and drops the type-word garnish — a
	// truncated composite ("trace …") would be strictly worse than the name
	// alone. The lossless block (face 3) still carries the full composite.
	if got := runtimeTraceCausalProjectionNodeSubjectCell(node, true); got != "oney.hmn.berlin-42591 / H:ReceiveVsync" {
		t.Fatalf("F1 detail-table cell (zh) must fall back to the bare real span name when the composite exceeds the cell budget: %q", got)
	}
	// Face 3 — lossless block: full name AND the dedicated "- span:" line.
	if got := runtimeTraceProjDetailFullName(node, true); got != "oney.hmn.berlin-42591 / H:ReceiveVsync(trace span)" {
		t.Fatalf("F1 lossless full name (zh) must honor 完整名称不截断 with the real name: %q", got)
	}
	model := runtimeTraceProjTreeModel{
		WindowMS: 101,
		Adjacent: []runtimeTraceProjTreeRow{{Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E21"}},
	}
	stanza := runtimeTraceProjDetailFullText(model, true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: "- span:" →
	// "- span 原文:" backtick-wrapped (structural item 5); 完整名称 line
	// DELETED — the block HEADING carries the untruncated full name.
	if !strings.Contains(stanza, "- span 原文: `H:ReceiveVsync`") {
		t.Fatalf("F1 lossless stanza must carry the dedicated span line:\n%s", stanza)
	}
	if !strings.Contains(stanza, "**[E21] oney.hmn.berlin-42591 / H:ReceiveVsync(trace span)**") {
		t.Fatalf("F1 lossless stanza heading must carry the real full name:\n%s", stanza)
	}
}

// TestPTV7SpnCompilePathE21Shape drives the WIRE shape through the projection
// compile (span_name typed note → node.SpanName) and pins the fence render —
// the end-to-end half of the F1 chain (the specimen's E21 rendered as a bare
// "trace span" type word on every face while the name sat parsed in the model).
func TestPTV7SpnCompilePathE21Shape(t *testing.T) {
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{
		Records: []types.ObservationRecord{ptv7SpnE21Record()},
	})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	if len(projection.AdjacentCauses) != 1 {
		t.Fatalf("E21 record must seat in the adjacent stanza: %+v", projection)
	}
	if got := projection.AdjacentCauses[0].SpanName; got != "H:ReceiveVsync" {
		t.Fatalf("compile must parse the span_name note: %q", got)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "H:ReceiveVsync") {
		t.Fatalf("fence must render the real span name for the generic rank row:\n%s", fence)
	}
}

// TestPTV7SpnSemanticSpanControl pins the NO-regression control: semantic-span
// rows keep their pre-PTV7 rendering on every face (name only — never the
// parenthesized generic composition), and the semantic gate keeps the wider
// 36-rune object cell.
func TestPTV7SpnSemanticSpanControl(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "E5",
		Subject: "T7@ZeusThreadPo-61839", Object: "class_verification",
		SpanName: "VerifyClass com.example.Foo", ImpactMS: 0.285, Confidence: 0.8,
	}
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowSemantic, HasData: true}
	if got := runtimeTraceProjRowName(row, true); got != "VerifyClass com.example.Foo" {
		t.Fatalf("semantic tree row must keep the name-only form: %q", got)
	}
	if got := runtimeTraceCausalProjectionNodeSubjectCell(node, true); got != "T7@ZeusThreadPo-61839 / VerifyClass com.example.Foo" {
		t.Fatalf("semantic detail cell must keep the name-only object (36-rune lane): %q", got)
	}
	if got := runtimeTraceProjDetailFullName(node, true); got != "T7@ZeusThreadPo-61839 / VerifyClass com.example.Foo" {
		t.Fatalf("semantic lossless full name must stay name-only: %q", got)
	}
	for _, face := range []string{
		runtimeTraceProjRowName(row, true),
		runtimeTraceCausalProjectionNodeSubjectCell(node, true),
		runtimeTraceProjDetailFullName(node, true),
	} {
		if strings.Contains(face, "(类校验)") || strings.Contains(face, "(class_verification)") {
			t.Fatalf("semantic face must not grow the generic parenthesized type word: %q", face)
		}
	}
}

// --- F2: evidence-index audit summary ----------------------------------------

func TestPTV7SpnAuditDetailCarriesSpanPart(t *testing.T) {
	got := runtimeTraceCausalProjectionAuditDetail(ptv7SpnE21Node(), true, false)
	if !strings.Contains(got, "span=H:ReceiveVsync") {
		t.Fatalf("audit summary must carry the span part: %q", got)
	}
	// The span part rides AFTER the predicate part (追加 — stable audit order).
	if strings.Index(got, "predicate=") > strings.Index(got, "span=") {
		t.Fatalf("span part must follow the predicate part: %q", got)
	}
}

func TestPTV7SpnAuditCellTextPartBoundary(t *testing.T) {
	// The huadong E21 verdict shape: rune-cut at 72 shipped "confidenc…".
	details := "tier=tertiary · causality=adjacent_to_wakeup_chain · rank=7 · confidence=0.75 · predicate=root_cause_tertiary · span=H:ReceiveVsync"
	got := runtimeTraceCausalProjectionAuditCellText(details, 96)
	if got != "tier=tertiary · causality=adjacent_to_wakeup_chain · rank=7 · confidence=0.75…" {
		t.Fatalf("audit cut must keep whole parts and drop the tail: %q", got)
	}
	if strings.Contains(got, "confidenc…") || strings.Contains(got, "predicate=root_c…") {
		t.Fatalf("audit cut must never split a token: %q", got)
	}
	// Under the cap: byte-identical pass-through.
	short := "tier=primary · rank=1 · confidence=0.86"
	if got := runtimeTraceCausalProjectionAuditCellText(short, 96); got != short {
		t.Fatalf("under-cap audit text must pass through: %q", got)
	}
	// Degenerate over-wide single part: falls back to the rune cut (never an
	// empty cell).
	long := "merged_ids=" + strings.Repeat("x", 200)
	if got := runtimeTraceCausalProjectionAuditCellText(long, 96); len([]rune(got)) != 96 || !strings.HasSuffix(got, "…") {
		t.Fatalf("degenerate single part must fall back to the rune cut: %q", got)
	}
	// The E1 shape now fits whole at 96 (it was cut mid-predicate at 72).
	e1 := "causality=on_wakeup_chain · confidence=0.78 · predicate=wakeup_causal_impact"
	if got := runtimeTraceCausalProjectionAuditCellText(e1, 96); got != e1 {
		t.Fatalf("the E1 shape must render whole at the 96 cap: %q", got)
	}
}

// TestPTV7SpnEvidenceIndexUsesPartBoundaryCut pins the call site: the rendered
// evidence-index audit cell of an over-long summary ends on a part boundary
// (reverting the site to the 72-rune CompactCellText mid-token cut reds this).
func TestPTV7SpnEvidenceIndexUsesPartBoundaryCut(t *testing.T) {
	idx := newRuntimeTraceCausalProjectionEvidenceIndex()
	node := ptv7SpnE21Node()
	node.SupportRefs = []string{"berlin.systrace:1013562-1016996"}
	if id := idx.add(node, true); id != "E1" {
		t.Fatalf("fixture must index the node, got id %q", id)
	}
	_, items := runtimeTraceProjEvidenceBlockParts(idx, true)
	if len(items) != 1 {
		t.Fatalf("expected one evidence item, got %d", len(items))
	}
	text := items[0].Text
	if strings.Contains(text, "confidenc…") {
		t.Fatalf("evidence index leaked the mid-token cut: %q", text)
	}
	if !strings.Contains(text, "置信度：中(0.75)") {
		t.Fatalf("evidence index must keep the reader-facing confidence part whole: %q", text)
	}
	for _, raw := range []string{"tier=", "causality=", "predicate="} {
		if strings.Contains(text, raw) {
			t.Fatalf("evidence index must not expose raw metadata %q: %q", raw, text)
		}
	}
}

// --- F4: detail-table fold naming + zh fallback (C39 漏面) --------------------

func TestPTV7SpnDetailTableFoldCellMirrorsC39(t *testing.T) {
	fold := types.TraceCausalProjectionNode{
		ChainRelevance: "on_chain", OnChainOverflowFold: true,
		MergedCount: 9, MergedMinMS: 0, MergedMaxMS: 0,
		MergedSubjects: []string{"OS_FFRT_2_2-43037", "tppmgr-idle-1-186"},
		Confidence:     0.6,
	}
	zhCell := runtimeTraceCausalProjectionNodeSubjectCell(fold, true)
	// P2a rider 件1 (§29.55.3, 2026-07-13): the dedup name form 其余N项(折叠)
	// mirrors the tree row (lockstep 行名铸造点).
	if !strings.Contains(zhCell, "其余 9 项(折叠)") || !strings.Contains(zhCell, "OS_FFRT_2_2-43037") {
		t.Fatalf("zh fold cell must name the count + roster: %q", zhCell)
	}
	if strings.Contains(zhCell, "unnamed causal node") {
		t.Fatalf("zh fold cell must not leak the EN fallback: %q", zhCell)
	}
	enCell := runtimeTraceCausalProjectionNodeSubjectCell(fold, false)
	if !strings.Contains(enCell, "9 more (folded)") {
		t.Fatalf("en fold cell must carry the fold naming: %q", enCell)
	}
	// Subject/object-less NON-fold rows: zh fallback speaks zh (C39), EN keeps
	// its placeholder phrase (RUN2FIX-A 复核 CR-4, 刻意更新非静默: the bare
	// "trace causal node" read as a real thread name — the parenthesized form
	// reads as the placeholder it is).
	bare := types.TraceCausalProjectionNode{Confidence: 0.6}
	if got := runtimeTraceCausalProjectionNodeSubjectCell(bare, true); got != "(未命名因果节点)" {
		t.Fatalf("zh bare cell must use the C39 fallback: %q", got)
	}
	if got := runtimeTraceCausalProjectionNodeSubjectCell(bare, false); got != "(unnamed causal node)" {
		t.Fatalf("en bare cell keeps the placeholder phrase: %q", got)
	}
}

// --- F5: trace_gap 数据盲区 + no-value form ------------------------------------

// ptv7SpnTraceGapProjection is the fixture home for the F5 shape (also wired
// into the revisit76 representative set): one on-chain trunk row plus the
// huadong trace_gap adjacent row (zero value, diagnostic lane).
func ptv7SpnTraceGapProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.1,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
			Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
			ImpactMS: 30, Confidence: 0.8,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "tppmgr-idle-3-188",
			Object: "trace_gap", ChainRelevance: "adjacent", Confidence: 0.6,
		}},
	}
}

// ptv7SpnTraceGapBelowFloorProjection (DISP-2 G2 措辞按 kind 分形, §27.2,
// 2026-07-09) is the same shape with the typed no_eligible_wait criterion:
// the window HELD scheduler intervals, all below the MinDurationMs floor —
// the inline disclosure and legend fork off the legacy 窗内无调度数据 wording.
// The kind literal is a deliberate wire-format double-write (test files keep
// verbatim tokens).
func ptv7SpnTraceGapBelowFloorProjection() types.TraceCausalProjection {
	projection := ptv7SpnTraceGapProjection()
	projection.AdjacentCauses[0].TraceGapKind = "no_eligible_wait"
	return projection
}

// disp2AllZeroFoldProjection (DISP-2 G19, §27.5, 2026-07-09) is the all-zero
// on-chain overflow fold shape (huadong_79 "9线程取最大(单项0.000~0.000ms)" noise
// witness): every folded member is a data blind spot with no measurable
// in-window duration — the fold row wears the honest one-line note instead of
// a member-MAX claim over zeros.
func disp2AllZeroFoldProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.1,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
				ImpactMS: 30, Confidence: 0.8},
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "disp2-zero-fold",
				ChainRelevance: "on_chain", OnChainOverflowFold: true,
				MergedCount: 9, MergedMinMS: 0, MergedMaxMS: 0, MergedAllDataGap: true,
				MergedSubjects: []string{"OS_FFRT_2_2-43037", "OS_FFRT_3_1-43041"},
				Confidence:     0.6},
		},
	}
}

func TestPTV7SpnTraceGapRowWording(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(ptv7SpnTraceGapProjection(), nil, true)
	fence := runtimeTraceProjTreeFence(model, true)
	// 用户措辞裁定 (一字不改): 显示词=数据盲区, 行内披露=窗内无调度数据·链止.
	if !strings.Contains(fence, "数据盲区") {
		t.Fatalf("trace_gap row must wear the 数据盲区 display word:\n%s", fence)
	}
	if !strings.Contains(fence, "窗内无调度数据·链止") {
		t.Fatalf("trace_gap row must carry the inline disclosure:\n%s", fence)
	}
	if strings.Contains(fence, "候选根因") {
		t.Fatalf("trace_gap row must not wear the candidate chip:\n%s", fence)
	}
	gapLine := ""
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "tppmgr-idle-3-188") {
			gapLine = line
			break
		}
	}
	if gapLine == "" {
		t.Fatalf("trace_gap row missing from the fence:\n%s", fence)
	}
	if strings.Contains(gapLine, "0.000ms") || strings.Contains(gapLine, "░") || strings.Contains(gapLine, "█") {
		t.Fatalf("trace_gap row must not draw a bar or a fake 0.000ms value: %q", gapLine)
	}
	if !strings.Contains(gapLine, "—") {
		t.Fatalf("trace_gap row must render the — no-value form (树表两口径对齐): %q", gapLine)
	}
	// The raw token stays off the zh row face (lossless 类型 lane keeps it).
	lead := runtimeTraceProjLeadText(ptv7SpnTraceGapProjection(), model, "zh", true)
	if !strings.Contains(lead, "- `数据盲区` = 窗内无调度数据,下钻链止。") {
		t.Fatalf("legend must carry the 数据盲区 entry (用户措辞裁定):\n%s", lead)
	}
}

// TestPTV7SpnDiagnosticValueRowKeepsBar guards the value-preserving half: a
// diagnostic-lane row WITH a real measured value keeps its bar and value (the
// no-value form is gated on the empty display-impact chain, never on the lane
// alone) — while the candidate chip stays off for the whole lane.
func TestPTV7SpnDiagnosticValueRowKeepsBar(t *testing.T) {
	churn := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "w-1", Object: "state_churn", ImpactMS: 5, Confidence: 0.8,
		},
		Kind: runtimeTraceProjTreeRowChain, HasData: true, marks: &runtimeTraceProjMarkSet{},
	}
	base, tags := runtimeTraceProjRowMetricParts(churn, 100, true, true)
	if !strings.Contains(base, "5.000ms") || !strings.Contains(base, "█") {
		t.Fatalf("valued diagnostic row must keep bar + value: %q", base)
	}
	for _, tag := range tags {
		if tag.Text == "候选根因" || tag.Text == "candidate cause" {
			t.Fatalf("diagnostic-lane row must not wear the candidate chip: %+v", tags)
		}
	}
}

func TestPTV7SpnAllZeroFoldNoValueForm(t *testing.T) {
	fold := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			ChainRelevance: "on_chain", OnChainOverflowFold: true,
			MergedCount: 9, MergedMinMS: 0, MergedMaxMS: 0,
			MergedSubjects: []string{"OS_FFRT_2_2-43037"},
			Confidence:     0.6,
		},
		Kind: runtimeTraceProjTreeRowDepthless, HasData: true, marks: &runtimeTraceProjMarkSet{},
	}
	base, tags := runtimeTraceProjRowMetricParts(fold, 100, true, true)
	if strings.Contains(base, "0.000ms") {
		t.Fatalf("all-zero fold row must not draw a fake 0.000ms: %q", base)
	}
	if !strings.Contains(base, "—") {
		t.Fatalf("all-zero fold row must render the — no-value form: %q", base)
	}
	for _, tag := range tags {
		if tag.Text == "候选根因" || tag.Text == "candidate cause" {
			t.Fatalf("all-zero fold row must not wear the candidate chip: %+v", tags)
		}
	}
	// EVOLUTION RECORD (DISP-2 G19, §27.5, 2026-07-09): the former "×N data
	// token survives" pin (9线程取最大(单项0.000~0.000ms)) is RETIRED on the all-zero
	// shape — a member-MAX claim over nothing but zeros taught noise
	// (huadong_79 witness). The shape now speaks the honest one-line note; the
	// count survives on the row NAME (其余 9 项(折叠)), and value-bearing
	// folds keep the ×N form byte-identically (TestDisp2AllZeroFoldNote pins
	// both directions).
	joined := ""
	for _, tag := range tags {
		joined += tag.Text + " · "
	}
	if strings.Contains(joined, "9次(0.000~0.000ms)") {
		t.Fatalf("the all-zero fold must not claim a member-MAX over zeros: %s", joined)
	}
	if !strings.Contains(joined, "窗内无有效时长") {
		t.Fatalf("the all-zero fold must carry the honest one-line note: %s", joined)
	}
}
