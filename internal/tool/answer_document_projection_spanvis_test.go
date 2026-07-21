package tool

// answer_document_projection_spanvis_test.go — SPANVIS-1 display pins (user
// ruling 2026-07-19 定形原则: 纯 advisory 提及面,不参与根因排序).
//
// Pinned here:
//   - 面1 tree fence: the ◈ advisory block (head + mention rows + truncation
//     row), zh/EN parity, B5b tail-kept name truncation, credential words;
//   - 面2 ◎ overview: the 业务优化线索 footnote family (⌗ 旁栏 precedent
//     form) with the SAME row word face;
//   - 零序数零种群: the block carries no badge/ordinal/bar/⛓; the ◎ board
//     lines (section ladders, subtotals, conservation, censuses) are
//     byte-identical with and without the mention face (the rows join no
//     population);
//   - zero-byte negative arm: no admissible mention → the whole render is
//     byte-identical to the mention-free projection;
//   - display row gates: any invalid typed field drops the row (fail-open);
//     all rows invalid → no block, no head, no mark, no legend entry;
//   - 件4 阅读参考 dual-lever teaching entry renders exactly with the ◈ word
//     face (SCORE-DERIV 承诺面双向) and judges no row (零判词勾稽);
//   - 件3 truncation honesty: the 另有 N 个 row renders only when the engine
//     counted omitted families.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func spanvisMentions() []types.TraceCausalProjectionBusinessSpanMention {
	return []types.TraceCausalProjectionBusinessSpanMention{
		{
			Subject: "LegoHandler-17585",
			Name:    "monitor contention with owner ransmitThread (38414)",
			Count:   2, TotalMS: 0.303, MaxMS: 0.295,
			StartLine: 21605, EndLine: 22024,
			Basis: "self", Hidden: 2,
		},
		{
			Subject: "com.baidu.tieba-61839",
			Name:    "transact[android.app.IActivityManager:6]",
			Count:   4, TotalMS: 2.401, MaxMS: 1.000,
			StartLine: 1899, EndLine: 2746,
			Basis: "host_wakeup_edge", Hidden: 4,
		},
	}
}

// spanvisMentionProjection is the representative legend fixture (registered
// in the revisit76 bidirectional sweep): a plain IO board plus the advisory
// mention side channel.
func spanvisMentionProjection() types.TraceCausalProjection {
	proj := revisit76IOProjection()
	proj.BusinessSpanMentions = spanvisMentions()
	proj.BusinessSpanMentionOmitted = 2
	return proj
}

// TestSpanvisTreeBlockRendersZH — 面1 zh: head + rows + truncation row, with
// verbatim typed values (提及行值=原始段值) and honest credential words.
func TestSpanvisTreeBlockRendersZH(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(spanvisMentionProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, want := range []string{
		"◈ 业务span提示(不参与根因排序 · 单次最长∪合计最长 TOP5 · 业务自查:能否减少这些 span 的时间占用):",
		"◈ 各族合计间不可相加(区间可重叠/嵌套)",
		"LegoHandler-17585 monitor contention with owner ransmitThread (38414) 单次最大0.295ms/2次 合计0.303ms 行21605..22024 凭证:自身",
		"单次最大1.000ms/4次 合计2.401ms 行1899..2746 凭证:唤醒边凭证(边前)",
		"另有 2 个 span 族(≥显著地板)未列出",
	} {
		if !strings.Contains(spanvisSquashFence(fence), spanvisSquashFence(want)) {
			t.Fatalf("zh tree block must carry %q:\n%s", want, fence)
		}
	}
	if !model.Marks.has(runtimeTraceProjMarkBusinessSpanMention) {
		t.Fatalf("the ◈ mark must record at the tree emission site")
	}
}

// TestSpanvisTreeBlockRendersEN — 面1 EN parity (same values, EN word faces).
func TestSpanvisTreeBlockRendersEN(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(spanvisMentionProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fence := runtimeTraceProjTreeFence(model, false)
	for _, want := range []string{
		"◈ business span leads (not in root-cause ranking · max-single ∪ total TOP5 · business self-check: can these spans' time be reduced):",
		"◈ family totals are not additive to each other (intervals may overlap or nest)",
		"max single 0.295ms ×2 · total 0.303ms · lines 21605..22024 · credential: self",
		"credential: wakeup-edge credential (pre-edge)",
		"2 more span families (at/above the significance floor) are not listed",
	} {
		if !strings.Contains(spanvisSquashFence(fence), spanvisSquashFence(want)) {
			t.Fatalf("EN tree block must carry %q:\n%s", want, fence)
		}
	}
}

// spanvisSquashFence — probe containment on the single-space-normalized line
// join (DISPLAY-WRAP 件① discipline: the token-aware wrap may break a long
// row at an inner space, dropping the break space on the emitted line).
func spanvisSquashFence(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(strings.TrimLeft(line, "│ \t"))
		b.WriteString(" ")
	}
	out := b.String()
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return out
}

// TestSpanvisZeroOrdinalZeroPopulation — 零序数零种群: the block lines carry
// no badge, no ordinal chip, no bar glyph, no ⛓, no percentage; and the ◎
// overview board lines are byte-identical with/without the mention face.
func TestSpanvisZeroOrdinalZeroPopulation(t *testing.T) {
	withProj := spanvisMentionProjection()
	withModel := buildRuntimeTraceProjTreeModel(withProj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(withModel, true)
	idx := strings.Index(fence, "◈ 业务span提示")
	if idx < 0 {
		t.Fatalf("block must render:\n%s", fence)
	}
	block := fence[idx:]
	if end := strings.Index(block, "```"); end >= 0 {
		block = block[:end]
	}
	for _, banned := range []string{"❶", "❷", "❸", "#", "█", "░", "⛓", "%"} {
		if strings.Contains(block, banned) {
			t.Fatalf("the advisory block must stay non-ordinal (found %q):\n%s", banned, block)
		}
	}
	// ◎ board identity (返工轮 P2-1: elimBoardProjection 形 fixture — the
	// board REALLY renders an overview; empty base = test failure, never a
	// silent pass): every non-◈ line of the overview fence must be
	// byte-identical with and without the mention face (section ladders /
	// subtotals / conservation / censuses untouched — the rows join no
	// population).
	base := elimBoardProjection()
	_, baseElim := elimRenderOverview(t, base, true)
	if strings.TrimSpace(baseElim) == "" {
		t.Fatalf("the board fixture must render an overview fence (empty base = the identity pin never runs)")
	}
	withBoard := elimBoardProjection()
	withBoard.BusinessSpanMentions = spanvisMentions()
	withBoard.BusinessSpanMentionOmitted = 2
	_, withElim := elimRenderOverview(t, withBoard, true)
	if !strings.Contains(withElim, "◈ 业务线索") {
		t.Fatalf("the board fixture must render the ◈ zone:\n%s", withElim)
	}
	// OMGCLEAN-1 (§29.175.6): the ◈ zone is a whole blank-separated zone —
	// strip it (blank line + ◈-led heads + value rows + tail) and the board
	// stays byte-identical outside it.
	var kept []string
	inMentionZone := false
	lines := strings.Split(withElim, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "◈ ") {
			inMentionZone = true
			// the zone separator blank line above the zone head leaves too
			if len(kept) > 0 && kept[len(kept)-1] == "" {
				kept = kept[:len(kept)-1]
			}
			continue
		}
		if inMentionZone {
			if line == "" {
				inMentionZone = false
				kept = append(kept, line)
				continue
			}
			continue // zone rows/tails
		}
		kept = append(kept, line)
	}
	if got, want := strings.Join(kept, "\n"), baseElim; got != want {
		t.Fatalf("◎ board lines must stay byte-identical outside the ◈ zone:\nwith:\n%s\nbase:\n%s", got, want)
	}
}

// TestSpanvisElimFootnoteRenders — 面2 (返工轮 P2-1: board fixture, zero
// SKIP): the 业务优化线索 footnote family renders on a REAL overview fence
// with the SAME row word face (词面单点); probes are wrap-normalized.
func TestSpanvisElimFootnoteRenders(t *testing.T) {
	proj := elimBoardProjection()
	proj.BusinessSpanMentions = spanvisMentions()
	proj.BusinessSpanMentionOmitted = 2
	model, elim := elimRenderOverview(t, proj, true)
	if strings.TrimSpace(elim) == "" {
		t.Fatalf("the board fixture must render an overview fence")
	}
	if !model.Marks.has(runtimeTraceProjMarkBusinessSpanMention) {
		t.Fatalf("the ◈ mark must be lit on the render")
	}
	// OMGCLEAN-1 件5+§29.175.6 (2026-07-20). EVOLUTION RECORD: the ◎ ◈ face
	// is now the compact TOP3 zone — 值·线程·span 名·次数(·单次最大), value
	// on the shared right-aligned grid, NO bar; 行号/凭证 words stay on the
	// tree ◈ block. 双复核修复 件8 (2026-07-21): the two head lines fold into
	// the ONE 定稿 head (the full selection-rule promise + F2 clause keep
	// their seats on the tree ◈ head + legend); 件4: the tail count speaks
	// the honest not-listed form (the tree pointer died with rider3).
	for _, want := range []string{
		"◈ 业务线索 · 业务自查减时(非确定性优化类 · 不占序数 · 族间不可相加)",
		"0.303ms LegoHandler-17585 · monitor contention with owner ransmitThread (38414) · 2次 ·单次最大0.295ms",
		"2.401ms com.baidu.tieba-61839 · transact[android.app.IActivityManager:6] · 4次 ·单次最大1.000ms",
	} {
		if !strings.Contains(spanvisSquashFence(elim), spanvisSquashFence(want)) {
			t.Fatalf("◎ ◈ zone must carry %q:\n%s", want, elim)
		}
	}
	if !strings.Contains(spanvisSquashFence(elim), spanvisSquashFence("另有 2 族(≥显著地板)未列出")) {
		t.Fatalf("◎ ◈ zone must carry the honest not-listed tail count:\n%s", elim)
	}
	if strings.Contains(elim, "见树◈块") {
		t.Fatalf("件4: the dead tree pointer must not render:\n%s", elim)
	}
	// 行号/凭证 words stay off the compact zone (tree ◈ block home).
	if strings.Contains(elim, "行21605") || strings.Contains(elim, "凭证:") {
		t.Fatalf("◎ ◈ compact rows must not carry line ranges / credential words:\n%s", elim)
	}
}

// TestSpanvisZeroByteNegativeArm — no admissible mention → the tree fence is
// byte-identical to the mention-free render (absence = zero bytes, no head,
// no mark, no legend entry).
func TestSpanvisZeroByteNegativeArm(t *testing.T) {
	base := revisit76IOProjection()
	baseModel := buildRuntimeTraceProjTreeModel(base, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	baseFence := runtimeTraceProjTreeFence(baseModel, true)

	empty := revisit76IOProjection()
	empty.BusinessSpanMentions = nil
	emptyModel := buildRuntimeTraceProjTreeModel(empty, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	emptyFence := runtimeTraceProjTreeFence(emptyModel, true)
	if baseFence != emptyFence {
		t.Fatalf("mention-free renders must stay byte-identical")
	}
	if strings.Contains(baseFence, "◈") {
		t.Fatalf("the base board must carry no ◈ bytes:\n%s", baseFence)
	}
	if baseModel.Marks.has(runtimeTraceProjMarkBusinessSpanMention) {
		t.Fatalf("no mention → no mark")
	}
	// Omitted counter WITHOUT any valid family row must also stay zero-byte
	// (the truncation row belongs to the block, never stands alone).
	orphan := revisit76IOProjection()
	orphan.BusinessSpanMentionOmitted = 3
	orphanModel := buildRuntimeTraceProjTreeModel(orphan, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if fence := runtimeTraceProjTreeFence(orphanModel, true); fence != baseFence {
		t.Fatalf("an orphan omitted counter must render nothing")
	}
}

// TestSpanvisDisplayRowGates — every display gate drops the invalid row
// (fail-open); all rows invalid → no block at all.
func TestSpanvisDisplayRowGates(t *testing.T) {
	valid := spanvisMentions()[0]
	mutate := map[string]func(*types.TraceCausalProjectionBusinessSpanMention){
		"count_zero":     func(m *types.TraceCausalProjectionBusinessSpanMention) { m.Count = 0 },
		"total_zero":     func(m *types.TraceCausalProjectionBusinessSpanMention) { m.TotalMS = 0 },
		"max_zero":       func(m *types.TraceCausalProjectionBusinessSpanMention) { m.MaxMS = 0 },
		"max_over_total": func(m *types.TraceCausalProjectionBusinessSpanMention) { m.MaxMS = m.TotalMS + 0.01 },
		"line_zero":      func(m *types.TraceCausalProjectionBusinessSpanMention) { m.StartLine = 0 },
		"line_inverted":  func(m *types.TraceCausalProjectionBusinessSpanMention) { m.EndLine = m.StartLine - 1 },
		"basis_unknown":  func(m *types.TraceCausalProjectionBusinessSpanMention) { m.Basis = "guessed" },
		// POOL2-1 件① EVOLUTION (§29.160①): hidden_zero is VALID now (the
		// positive arm below); negative/overflow still drop.
		"hidden_neg":    func(m *types.TraceCausalProjectionBusinessSpanMention) { m.Hidden = -1 },
		"hidden_over":   func(m *types.TraceCausalProjectionBusinessSpanMention) { m.Hidden = m.Count + 1 },
		"subject_empty": func(m *types.TraceCausalProjectionBusinessSpanMention) { m.Subject = " " },
		"name_empty":    func(m *types.TraceCausalProjectionBusinessSpanMention) { m.Name = "" },
	}
	for name, mut := range mutate {
		row := valid
		mut(&row)
		if _, ok := runtimeTraceProjBusinessSpanMentionRowText(row, nil, true); ok {
			t.Fatalf("%s: the display gate must drop the row", name)
		}
	}
	// POOL2-1 件① positive arm (§29.160① ruling: 提及义务限链上,含自身;
	// the bounded seat view is not the mention face): a fully-visible family
	// (Hidden==0) RENDERS — the former display-side third-gate mirror is gone.
	fullyVisible := valid
	fullyVisible.Hidden = 0
	if _, ok := runtimeTraceProjBusinessSpanMentionRowText(fullyVisible, nil, true); !ok {
		t.Fatalf("hidden==0 (fully-visible family) must render post-§29.160①")
	}
	// All rows invalid → zero bytes (no head, no mark).
	proj := revisit76IOProjection()
	bad := spanvisMentions()[0]
	bad.Basis = "guessed"
	proj.BusinessSpanMentions = []types.TraceCausalProjectionBusinessSpanMention{bad}
	proj.BusinessSpanMentionOmitted = 1
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "◈") || model.Marks.has(runtimeTraceProjMarkBusinessSpanMention) {
		t.Fatalf("all-invalid rows must render nothing:\n%s", fence)
	}
}

// TestSpanvisNameTruncationTailKept — B5b: an over-budget span name keeps its
// head word and its distinguishing tail around one "…".
func TestSpanvisNameTruncationTailKept(t *testing.T) {
	long := "H:void OHOS::AbilityRuntime::ContextImpl::InitResourceManager(const AppExecFwk::BundleInfo &, const std::shared_ptr<ContextImpl> &, bool, const std::string &, std::shared_ptr<Context>)"
	row := spanvisMentions()[0]
	row.Name = long
	text, ok := runtimeTraceProjBusinessSpanMentionRowText(row, nil, true)
	if !ok {
		t.Fatalf("row must render")
	}
	if strings.Contains(text, long) {
		t.Fatalf("over-budget name must truncate:\n%s", text)
	}
	if !strings.Contains(text, "…") || !strings.Contains(text, "std::shared_ptr<Context>)") {
		t.Fatalf("truncation must keep the distinguishing tail (B5b):\n%s", text)
	}
}

// TestSpanvisReadingReferenceEntry — 件4 阅读参考: the dual-lever teaching
// entry renders exactly with the ◈ word face (承诺面双向) and never mints a
// per-row judgment word.
func TestSpanvisReadingReferenceEntry(t *testing.T) {
	const entryZH = "- ◈ 业务span提示行(阅读参考):次数多而单次小→业务流程/调用次数方向;单次长→单次运行时长方向;三数(单次最大/次数/合计)均为窗内墙钟原始值,仅业务视角提示,不参与根因排序,不参与汇排。"
	withText := scoreDerivClusterText(t, spanvisMentionProjection(), "zh")
	if !strings.Contains(withText, entryZH) {
		t.Fatalf("the reading-reference entry must render verbatim with the ◈ face:\n%s", withText)
	}
	if strings.Contains(withText, "过于频繁") || strings.Contains(withText, "too frequent") {
		t.Fatalf("the face must never mint a frequency judgment word (树面零判词)")
	}
	enText := scoreDerivClusterText(t, spanvisMentionProjection(), "en")
	if !strings.Contains(enText, "- ◈ business span lead rows (reading reference): many occurrences with a small single occurrence → look toward the business flow / call count; a long single occurrence → look toward one run's duration; the trio (max single / count / total) are raw in-window wall-clock values — business-view leads only, not in root-cause ranking, not ranked here.") {
		t.Fatalf("EN reading-reference entry must render verbatim:\n%s", enText)
	}
	baseText := scoreDerivClusterText(t, revisit76IOProjection(), "zh")
	if strings.Contains(baseText, "阅读参考):次数多而单次小") {
		t.Fatalf("absent ◈ face must keep the entry off (承诺面双向):\n%s", baseText)
	}
}

// TestSpanvisTruncationRowHonesty — 件3: omitted==0 renders no 另有 row.
func TestSpanvisTruncationRowHonesty(t *testing.T) {
	proj := spanvisMentionProjection()
	proj.BusinessSpanMentionOmitted = 0
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "◈ 业务span提示") {
		t.Fatalf("block must render:\n%s", fence)
	}
	if strings.Contains(fence, "另有") && strings.Contains(fence, "span 族") {
		t.Fatalf("omitted==0 must render no truncation row:\n%s", fence)
	}
}

// TestSpanvisMentionObservationEmission — the wire half: one record per
// family with the full typed note set (all eight keys + selected_window),
// the omitted counter on every record, nil on absence, and the closed-set
// basis skip (an unknown token never publishes).
func TestSpanvisMentionObservationEmission(t *testing.T) {
	rank := &tracequery.RootCauseRankResult{
		Window: tracequery.TimeWindow{StartTs: 13762.9805, EndTs: 13762.985},
		BusinessSpanMentions: &tracequery.BusinessSpanMentionResult{
			Families: []tracequery.BusinessSpanMention{
				{
					Thread: tracequery.ThreadRef{PID: 17585, Comm: "LegoHandler"},
					Name:   "monitor contention with owner ransmitThread (38414)",
					Count:  2, TotalMs: 0.303, MaxSingleMs: 0.295,
					StartLine: 21605, EndLine: 22024,
					OnChainBasis: tracequery.BusinessSpanMentionBasisSelf,
					HiddenCount:  2,
				},
				{
					Thread: tracequery.ThreadRef{PID: 999, Comm: "bad"},
					Name:   "x", Count: 1, TotalMs: 1, MaxSingleMs: 1,
					StartLine: 1, EndLine: 2,
					OnChainBasis: "guessed", HiddenCount: 1,
				},
			},
			OmittedFamilies: 4,
		},
	}
	records := traceQueryTypedBusinessSpanMentionObservations(rank, types.ObservationSourceRef{}, "s", "2026-07-19T00:00:00Z")
	if len(records) != 1 {
		t.Fatalf("closed-set basis: the unknown-token family must never publish, got %d records", len(records))
	}
	record := records[0]
	if record.Predicate != "business_span_mention" || record.Subject != "LegoHandler-17585" {
		t.Fatalf("record identity: %+v", record)
	}
	// 返工轮 P3: the ClaimKey carries the family NAME too — two same-subject
	// families sharing one line envelope must never collide into one claim.
	if record.ClaimKey != "business_span_mention:LegoHandler-17585:monitor contention with owner ransmitThread (38414):21605..22024" {
		t.Fatalf("claim key must carry subject+name+envelope: %q", record.ClaimKey)
	}
	for _, want := range []string{
		types.TraceNoteKeyBusinessSpanName + "=monitor contention with owner ransmitThread (38414)",
		types.TraceNoteKeyBusinessSpanCount + "=2",
		types.TraceNoteKeyBusinessSpanTotalMS + "=0.303",
		types.TraceNoteKeyBusinessSpanMaxMS + "=0.295",
		types.TraceNoteKeyBusinessSpanLines + "=21605..22024",
		types.TraceNoteKeyBusinessSpanBasis + "=self",
		types.TraceNoteKeyBusinessSpanHidden + "=2",
		types.TraceNoteKeyBusinessSpanOmitted + "=4",
	} {
		found := false
		for _, note := range record.RichNotes {
			if note == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("typed note %q missing: %v", want, record.RichNotes)
		}
	}
	if record.Span.LineStart != 21605 || record.Span.LineEnd != 22024 {
		t.Fatalf("line span must ride the record: %+v", record.Span)
	}
	if traceQueryTypedBusinessSpanMentionObservations(nil, types.ObservationSourceRef{}, "s", "t") != nil {
		t.Fatalf("nil rank must emit nothing")
	}
	if traceQueryTypedBusinessSpanMentionObservations(&tracequery.RootCauseRankResult{}, types.ObservationSourceRef{}, "s", "t") != nil {
		t.Fatalf("absent mention face must emit nothing")
	}
}

// TestSpanvisMentionObservationEmissionHiddenZero — POOL2-1 复核收编
// (P2-1, 2026-07-20): the emit-side half of §29.160① had no tooth — reverting
// the emitter to traceQueryTypedCount (which swallows 0 into key absence)
// left every suite green while a fully-visible admitted family's record lost
// the business_span_hidden key and the strict parser dropped it whole. This
// arm pins the explicit "business_span_hidden=0" note on a HiddenCount:0
// family.
func TestSpanvisMentionObservationEmissionHiddenZero(t *testing.T) {
	rank := &tracequery.RootCauseRankResult{
		Window: tracequery.TimeWindow{StartTs: 13762.9805, EndTs: 13762.985},
		BusinessSpanMentions: &tracequery.BusinessSpanMentionResult{
			Families: []tracequery.BusinessSpanMention{
				{
					Thread: tracequery.ThreadRef{PID: 17585, Comm: "LegoHandler"},
					Name:   "fully visible span", Count: 2,
					TotalMs: 0.303, MaxSingleMs: 0.295,
					StartLine: 10, EndLine: 20,
					OnChainBasis: "self", HiddenCount: 0,
				},
			},
		},
	}
	records := traceQueryTypedBusinessSpanMentionObservations(rank, types.ObservationSourceRef{}, "s", "2026-07-20T00:00:00Z")
	if len(records) != 1 {
		t.Fatalf("HiddenCount:0 family must publish, got %d records", len(records))
	}
	found := false
	for _, note := range records[0].RichNotes {
		if note == types.TraceNoteKeyBusinessSpanHidden+"=0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("business_span_hidden=0 must publish explicitly (strict parser requires key presence), notes=%v", records[0].RichNotes)
	}
}
