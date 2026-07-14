package tool

// answer_document_projection_selfall_test.go — SELF-ALL 批 display pins
// (§29.61.2/§29.61.2a SELF-ALL, §29.58.3 SELF-LANE 件②, §29.58.5 件③; user
// rulings 2026-07-13, ledger docs/design/real_trace_campaign_20260705.md).
//
//	件①(显示半场)  the promoted wall-clock self seat renders the SAME cause
//	              grammar as every chain row — 行2 identity with the
//	              自身·墙钟席 qualifier + the chain-channel 根因排序#N chip,
//	              行3 breakdown, roster sub-rows (同形纪律, ONE composer).
//	件②a          the target's non-chain ◇ residual relocates into the self
//	              stanza wearing 非链 (+ the ⌗ caliber words); the ◇ stanza
//	              keeps only OTHER threads.
//	件②b          cross-channel same-thread mutual pointers (「本线程另有
//	              链上席/邻近席 [E#]」) — bidirectional; same-stanza pairs
//	              stamp nothing (noise gate).
//	件③a          component rows indent ONE level deeper (↳ in the indent
//	              position, single form mark ⋈) — rune-width sweep.
//	件③b          a dedup fold row keeps its STATE WORD on 行1
//	              (「主体 · IO等待 2次同值」 form, 主行三要素).
//
// MUTATION self-checks:
//   - dropping the runtimeTraceProjRelocateSelfNonChainSeats pass reds
//     TestSelfAllNonChainResidualRelocatesWithQualifier (target row stays ◇);
//   - dropping the cause-grammar lines from runtimeTraceProjSelfRowParts reds
//     TestSelfAllPromotedSeatRendersCauseGrammar (the ordinal chip vanishes);
//   - dropping the same-stanza noise gate in
//     runtimeTraceProjMarkCrossChannelSameThread reds
//     TestSelfAllCrossChannelMutualPointers (the target pair re-stamps);
//   - reverting the §29.58.5 component lead reds
//     TestSelfAllComponentRowIndentGeometry;
//   - reverting the §29.58.5 dedup 行1 state-word floor reds
//     TestSelfAllDedupRowKeepsStateWordOnRow1.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// 件①: the donghu 133136 witness form — the promoted seat wears the cause
// grammar with its chain-channel ordinal inside the self stanza.
func TestSelfAllPromotedSeatRendersCauseGrammar(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(revisit76SelfAllWallClockProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⛓ 自身·IO延迟 合计3.264ms") {
		t.Fatalf("the promoted seat must render inside the self stanza with the family value stem:\n%s", fence)
	}
	if !strings.Contains(fence, "IO阻塞候选·自身·墙钟席·根因排序#6·置信高") {
		t.Fatalf("行2 identity must wear the 自身·墙钟席 qualifier and the chain-channel ordinal (佩序数):\n%s", fence)
	}
	if !strings.Contains(fence, "有效归因 3.264ms = 合计(共5段,同线程)") {
		t.Fatalf("行3 breakdown must render the D/IO wall-clock family caliber (§29.61.2a 同形):\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkSelfWallClockBasis) {
		t.Fatalf("the 自身·墙钟席 legend mark must record at the emission site")
	}
	// The legend teaching line renders with the qualifier (mark ⇔ entry; the
	// legend rides the lead text).
	lead := runtimeTraceProjLeadText(revisit76SelfAllWallClockProjection(), model, "zh", true)
	if !strings.Contains(lead, "`自身·墙钟席`") {
		t.Fatalf("the 自身·墙钟席 legend entry must render:\n%s", lead)
	}
}

// 件②a: the target's ⌗ count residual leaves the ◇ stanza and re-seats 非链
// in the self stanza with the caliber words intact (count-equivalent 行1
// value — never bare wall-clock ms).
func TestSelfAllNonChainResidualRelocatesWithQualifier(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(revisit76SelfAllWallClockProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.Adjacent {
		if runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) == runtimeTraceCausalProjectionCanonicalNode(".ugc.aweme.lite-17267") {
			t.Fatalf("no target-subject row may keep a ◇ seat (§29.58.3 a): %+v", row.Node)
		}
	}
	relocated := false
	for _, row := range model.SelfRows {
		if row.SelfNonChainSeat {
			relocated = true
			if row.Node.ChainRelevance != "adjacent" {
				t.Fatalf("relocation is display placement ONLY — the typed channel stays adjacent: %+v", row.Node)
			}
		}
	}
	if !relocated {
		t.Fatalf("the ⌗ residual must relocate into the self stanza")
	}
	fence := runtimeTraceProjTreeFence(model, true)
	// §29.55 观察③ 两形一裁 (2026-07-14): the E29 donghu witness form
	// 计数当量81.616ms is retired — the ONE form is 计数当量81.616(非墙钟).
	if !strings.Contains(fence, "自身·页缓存抖动 计数当量81.616(非墙钟)") {
		t.Fatalf("the relocated count row must speak the count-equivalent value form (G3), never bare ms:\n%s", fence)
	}
	if !strings.Contains(fence, "非链") || !strings.Contains(fence, "⌗口径旁栏") {
		t.Fatalf("the relocated row must wear the 非链 qualifier and keep its ⌗ caliber word:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkSelfNonChainSeat) {
		t.Fatalf("the 非链 legend mark must record at the emission site")
	}
}

// 件②b: the cross-channel same-thread mutual pointers — bidirectional on the
// non-target pair (keva-3: tree seat E# ↔ ◇ seat E#), and NOTHING on the
// target pair (both its seats render in ONE stanza after the relocation — the
// noise gate).
func TestSelfAllCrossChannelMutualPointers(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(revisit76SelfAllWallClockProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "本线程另有链上席 [") || !strings.Contains(fence, "本线程另有邻近席 [") {
		t.Fatalf("the mutual pointer pair must render on the cross-stanza thread:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkCrossChannelPointer) {
		t.Fatalf("the pointer legend mark must record at the emission site")
	}
	// Typed stamps: the keva-3 pair points both ways.
	var adjRef, chainRef string
	for _, row := range model.Adjacent {
		if runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) == "keva-3-17439" {
			adjRef = row.CrossChannelChainRef
		}
	}
	for _, row := range model.TreeRows {
		if runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) == "keva-3-17439" {
			chainRef = row.CrossChannelAdjacentRef
		}
	}
	if adjRef == "" || chainRef == "" {
		t.Fatalf("both directions must stamp (adj→chain=%q, chain→adj=%q)", adjRef, chainRef)
	}
	// Noise gate: the target's relocated 非链 row and its chain seats share
	// the self stanza — no pointer may stamp on either side.
	for _, row := range model.SelfRows {
		if row.CrossChannelChainRef != "" || row.CrossChannelAdjacentRef != "" {
			t.Fatalf("same-stanza pair must stamp nothing (noise gate): %+v", row)
		}
	}
}

// 修复轮 件3 F3 (2026-07-13): the coverage/residual census speaks 「链上行」
// and bare wall-clock ms — a relocated 非链 ⌗ row (typed adjacent channel,
// count-equivalent value) must enter NEITHER the count nor the bare-ms MAX
// (the 81.616 count equivalent printed as 「单项最大 81.616ms」 and the
// typed-adjacent row was counted as an on-chain row).
func TestSelfAllCensusExcludesNonChainAndCaliberSideRows(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(revisit76SelfAllWallClockProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	relocated := false
	for _, row := range model.SelfRows {
		if row.SelfNonChainSeat {
			relocated = true
		}
	}
	if !relocated {
		t.Fatalf("fixture drifted: the ⌗ residual must have relocated into SelfRows")
	}
	n, x, _ := runtimeTraceProjUnadmittedOnChainDisclosure(model)
	if x == 81.616 {
		t.Fatalf("F3: the count-equivalent 81.616 must never publish as a bare-ms census MAX")
	}
	if n != 1 || x != 3.264 {
		t.Fatalf("F3: the census counts exactly the on-chain wall-clock residual (want n=1 x=3.264): n=%d x=%.3f", n, x)
	}
}

// 修复轮 件4 F4 (2026-07-13): the cross-channel pointer loops share ONE
// eligibility predicate — a tag-less ◌ blind-spot row can neither point nor
// be pointed at (双向或双无; the one-way 「本线程另有链上席」 on a row no
// reverse pointer could ever name is retired).
func TestSelfAllCrossChannelPointerBidirectionalOrNone(t *testing.T) {
	chainRow := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{Subject: "gap-thread-9", ChainRelevance: "on_chain",
			ImpactMS: 2.0, Rank: 3}, Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E7",
	}
	gapRow := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{Subject: "gap-thread-9", ChainRelevance: "adjacent",
			TypeToken: "trace_gap"}, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "",
	}
	// An unrelated ELIGIBLE cross-stanza pair keeps the pass's best maps
	// non-empty (突变自检加固: the empty-map early return masked the F4
	// knockout on the lone-pair shape).
	otherChain := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{Subject: "other-2", ChainRelevance: "on_chain",
			ImpactMS: 4.0, Rank: 1}, Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E1",
	}
	otherAdj := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{Subject: "other-2", ChainRelevance: "adjacent",
			ImpactMS: 1.0}, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E2",
	}
	model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{chainRow, otherChain},
		Adjacent: []runtimeTraceProjTreeRow{gapRow, otherAdj}}
	runtimeTraceProjMarkCrossChannelSameThread(&model)
	if model.Adjacent[0].CrossChannelChainRef != "" || model.TreeRows[0].CrossChannelAdjacentRef != "" {
		t.Fatalf("F4 双向或双无: a tag-less row must neither point nor be pointed at: adj=%q chain=%q",
			model.Adjacent[0].CrossChannelChainRef, model.TreeRows[0].CrossChannelAdjacentRef)
	}
	if model.Adjacent[1].CrossChannelChainRef != "E1" || model.TreeRows[1].CrossChannelAdjacentRef != "E2" {
		t.Fatalf("the unrelated eligible pair must stamp both ways: adj=%q chain=%q",
			model.Adjacent[1].CrossChannelChainRef, model.TreeRows[1].CrossChannelAdjacentRef)
	}
	// Control: give the ◇ row its tag — both directions stamp.
	model.Adjacent[0].EvidenceTag = "E9"
	runtimeTraceProjMarkCrossChannelSameThread(&model)
	if model.Adjacent[0].CrossChannelChainRef != "E7" || model.TreeRows[0].CrossChannelAdjacentRef != "E9" {
		t.Fatalf("F4 control: eligible pair must stamp both ways: adj=%q chain=%q",
			model.Adjacent[0].CrossChannelChainRef, model.TreeRows[0].CrossChannelAdjacentRef)
	}
}

// 修复轮 件1 F1 + 件2 F2 (2026-07-13) — the 133136 A/B witness on the FULL
// deterministic pipeline (multi-view document shape, the h3 run form):
//   - F1 佩序席折叠豁免: the promotion's +1 pushed seat #7 past the compile
//     positional cap → row invisible + ordinal hole #6→#8. Every seated row
//     now renders; the rendered ordinal set is CONTIGUOUS.
//   - F2 IOFAM-SELF 显示折叠 basis 维: the overlap-proven 1.347ms seat (#12)
//     folded into the self-basis family's 「同段IO另有」 note although the
//     segments are pairwise disjoint (同段 false claim + seat invisible).
//     Mixed-basis seats now render as separate rows.
func TestSelfAllFixRoundDonghuABWitness(t *testing.T) {
	idx, err := tracequery.BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	q := tracequery.Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace}
	var records []types.ObservationRecord
	for i, view := range []string{"window_stats", "root_cause_rank", "critical_blocking_calls", "thread_timeline"} {
		vq := q
		vq.View = view
		result := tracequery.Run(idx, vq)
		records = append(records, traceQueryTypedObservations(result, "donghu.ftrace",
			fmt.Sprintf("payload-%d", i), "raw-ref", "", time.Unix(1751600000, 0).UTC())...)
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) == 0 {
		t.Fatal("no projection")
	}
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	// F1: rendered seat ordinals are contiguous — every engine seat between 1
	// and the max rendered ordinal holds a visible row (fold peers included:
	// a rank twin folded into a rendered row keeps its published seat).
	seen := map[int]bool{}
	var maxRank int
	for _, lane := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for _, row := range lane {
			if row.Node.Rank > 0 {
				seen[row.Node.Rank] = true
				if row.Node.Rank > maxRank {
					maxRank = row.Node.Rank
				}
			}
			for _, peer := range row.RankFoldPeers {
				if peer.Rank > 0 {
					seen[peer.Rank] = true
					if peer.Rank > maxRank {
						maxRank = peer.Rank
					}
				}
			}
		}
	}
	if maxRank < 8 {
		t.Fatalf("fixture drifted: the witness board must seat at least 8 ordinals, got max=%d", maxRank)
	}
	for r := 1; r <= maxRank; r++ {
		if !seen[r] {
			t.Fatalf("F1 序数连续承诺: seat #%d is invisible on every render surface (ordinal hole):\nseen=%v", r, seen)
		}
	}
	// F2: the overlap-proven io seat and the self-basis family render as
	// SEPARATE self rows (mixed proof bases never fold), and the false 同段
	// claim over the disjoint pair is gone — the 1.347 value never appears
	// inside a 同段IO另有 note.
	var overlapSeat, selfFamily bool
	for _, row := range model.SelfRows {
		token := runtimeTraceCausalProjectionCanonicalNode(row.Node.TypeToken)
		if token != "io_latency" {
			continue
		}
		if row.Node.OnChainBasis == "" && row.Node.ImpactMS > 1.3 && row.Node.ImpactMS < 1.4 {
			overlapSeat = true
		}
		if row.Node.OnChainBasis == "self_wall_clock_interval" {
			selfFamily = true
		}
	}
	if !overlapSeat || !selfFamily {
		t.Fatalf("F2 两把尺分行: overlap seat visible=%v, self-basis family visible=%v", overlapSeat, selfFamily)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "同段IO另有") && strings.Contains(line, "io_latency） 1.347") {
			t.Fatalf("F2 同段词面: the disjoint overlap seat must not be claimed 同段 inside a fold note: %q", line)
		}
	}
}

// 修复轮 件2 F2 unit half (2026-07-13): the display IO fold key's basis
// dimension is independently killable — on the FULL witness the F1 ranked
// exemption masks it (both rows seated), so this pin exercises the UNRANKED
// mixed-basis shape (rank twins truncated, the h3 FAIL-run form): an
// overlap-proven facet and a self-basis facet with overlapping intervals must
// never fold into one 同段 group (两把尺禁混折, engine key 同款).
func TestSelfAllDisplayIOFoldKeySplitsProofBases(t *testing.T) {
	overlap := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "f2-overlap",
		Subject: ".ugc.aweme.lite-17267", TypeToken: "io_latency", ChainRelevance: "on_chain",
		ImpactMS: 1.347, StartTs: 5.00, EndTs: 5.10, LineStart: 10, LineEnd: 20,
	}
	selfBasis := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "f2-self",
		Subject: ".ugc.aweme.lite-17267", TypeToken: "io_latency", ChainRelevance: "on_chain",
		Causality: "self_wall_clock", OnChainBasis: "self_wall_clock_interval",
		ImpactMS: 1.248, StartTs: 5.05, EndTs: 5.15, LineStart: 30, LineEnd: 40,
	}
	out, peers := runtimeTraceProjFoldSameSubjectIONodes([]types.TraceCausalProjectionNode{overlap, selfBasis})
	if len(out) != 2 || len(peers) != 0 {
		t.Fatalf("F2 两把尺禁混折: mixed-basis facets must never fold into one 同段 group: rows=%d peers=%v", len(out), peers)
	}
}

// ◎ 总览随动 (witness acceptance): the promoted seat enters the ◎ overview on
// the ⛓链上 channel wearing the 自身·墙钟席 qualifier — never a 候选 word (a
// wall-clock seat is a proven amount, not a conditional upper bound).
func TestSelfAllPromotedSeatEntersElimOverview(t *testing.T) {
	projection := revisit76SelfAllWallClockProjection()
	projection.RootCauseFamilyObserved = true
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjElimOverviewFence(projection, model, true)
	if fence == "" {
		t.Fatalf("the ◎ overview must render on this shape")
	}
	var seatLine string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "IO延迟") && strings.Contains(line, "3.264ms") {
			seatLine = line
			break
		}
	}
	if seatLine == "" {
		t.Fatalf("the promoted wall-clock seat must enter the ◎ board:\n%s", fence)
	}
	if !strings.Contains(seatLine, "⛓链上") {
		t.Fatalf("◎ 随动: the promoted seat rides the ⛓链上 channel word: %q", seatLine)
	}
	if !strings.Contains(seatLine, "自身·墙钟席") || strings.Contains(seatLine, "候选") {
		t.Fatalf("the ◎ qualifier must be 自身·墙钟席 with no 候选 word: %q", seatLine)
	}
}

// 件③a: the reseated component row indents ONE level deeper — ↳ occupies the
// indent cells and the single form mark ⋈ sits exactly 2 cells deeper than
// the sibling state glyphs (rune-width sweep, PIN-1 新 sweep discipline).
func TestSelfAllComponentRowIndentGeometry(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(p2aSelfCarveProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var hostLine, componentLine string
	for _, line := range strings.Split(fence, "\n") {
		if strings.HasPrefix(line, "│     ☾ 自身·sleep") {
			hostLine = line
		}
		if strings.Contains(line, "自身·binder") {
			componentLine = line
		}
	}
	if hostLine == "" || componentLine == "" {
		t.Fatalf("fixture drifted: host/component rows missing:\n%s", fence)
	}
	hostMarkCol := runewidth.StringWidth(hostLine[:strings.Index(hostLine, tracefence.GlyphSleep)])
	subIdx := strings.Index(componentLine, tracefence.GlyphSubordinate)
	markIdx := strings.Index(componentLine, tracefence.GlyphBinderWait)
	if subIdx < 0 || markIdx < 0 {
		t.Fatalf("component row must carry ↳ and the single ⋈ mark: %q", componentLine)
	}
	subCol := runewidth.StringWidth(componentLine[:subIdx])
	markCol := runewidth.StringWidth(componentLine[:markIdx])
	// §29.58.5 ①: the ↳ falls into the indent position (one level = 2 cells
	// deeper than the host mark column) and the row's single mark ⋈ sits one
	// further envelope along — never flush beside the host's glyph column.
	if subCol != hostMarkCol+2 {
		t.Fatalf("↳ must occupy the one-level-deeper indent cell: host mark col=%d ↳ col=%d\n%q", hostMarkCol, subCol, componentLine)
	}
	if markCol <= subCol {
		t.Fatalf("the single form mark ⋈ must follow the connector: ↳=%d ⋈=%d", subCol, markCol)
	}
}

// 件③b: a dedup fold row whose name cell cannot hold the full cause word
// keeps the SHORT state word on 行1 (「主体 · IO等待 2次同值」 — 主行三要素);
// the full cause word (with the peer tail) stays on the row-2 guarantee copy.
func TestSelfAllDedupRowKeepsStateWordOnRow1(t *testing.T) {
	// Production form (133136 E29): a ◇ io_latency dedup row on a long
	// pid-tailed subject whose full cause word IO等待(对端 udk-irq-1-76)
	// overflows the shared name column.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"app-9511", "CompThread_0-2955"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "selfall-dedup-chain",
			Subject: "CompThread_0-2955", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 36.757, CumulativeImpactMS: 36.757, EffectiveImpactMS: 36.757,
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "selfall-dedup-adj",
			Subject: ".ugc.aweme.lite-17267", Predicate: "critical_blocking",
			Object: "udk-irq-1-76", TypeToken: "io_latency",
			ChainRelevance: "adjacent", ImpactMS: 0.884, CumulativeImpactMS: 0.884,
			DuplicatePublications: 2, Confidence: 0.86, LineStart: 520, LineEnd: 540,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var row1 string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "2次同值") && strings.Contains(line, ".ugc.aweme") {
			row1 = line
			break
		}
	}
	if row1 == "" {
		t.Fatalf("fixture drifted: the dedup row must render:\n%s", fence)
	}
	if strings.Contains(row1, "IO等待(对端") {
		// The full cause word fits — the shape no longer exercises the cut;
		// the floor is untestable here. Fail loud so the fixture is retuned.
		t.Fatalf("fixture drifted: the name cell must be too narrow for the full cause word: %q", row1)
	}
	if !strings.Contains(row1, " · IO等待 2次同值") {
		t.Fatalf("§29.58.5 ③ 主行三要素: 行1 must keep the short state word beside the chip: %q", row1)
	}
	// The full cause word (with the peer tail) survives on the row-2 copy.
	if !strings.Contains(fence, "IO等待(对端 udk-irq-1-76)") {
		t.Fatalf("the full cause word must survive on the guarantee copy:\n%s", fence)
	}
}
