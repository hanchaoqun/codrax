package tool

// answer_document_projection_uxr1_test.go — UXR-1 件② display pins (§29.36
// 全裁定链, user rulings 2026-07-11, ledger real_trace_campaign_20260705.md).
//
// Witness-isomorphic fixtures:
//   - 4165 (real_trace_a5): three-channel interleaving — 根因排序#1/#2 in the
//     ▒ stanza + ◇ rows wearing ⛓ + qualifier position variants;
//   - 140554 (hitrace_jank): the (唤醒链路径未解析;…平铺) head, the ×2同值
//     orphan line and the ◦-beside-阻塞等待 pairing;
//   - 54476: deep-chain bar-column drift (L3 bar left of the shared column);
//   - 2549 (real_trace_a4): qualifier mid-value-column variant + 裸尾巴 row2.
//
// MUTATION self-checks (recorded in the batch report):
//   - M-D1 restore the 背景榜位 chip arm → the channel-word pins red;
//   - M-D2 print 根因排序#N on adjacent rows → TestUXR1ChannelChipWords red;
//   - M-D3 revert the Row2 qualifier demotion → TestUXR1QualifierRow2Slot red;
//   - M-D4 revert the ⛓ off-chain fork → TestUXR1OffChainDStateGlyph red;
//   - M-D5 restore the ×N同值 orphan tag → TestUXR1DedupChipRidesName red;
//   - M-D6 restore the 以下各行按层级平铺 head note → TestUXR1UndrillableBanner red;
//   - M-D7 drop the mention-floor stamp → TestUXR1MentionObligationChannel red;
//   - M-D8 un-group the ▒ stanza → TestUXR1BackgroundCaliberGrouping red.
//   - M-D9 (复核 P2-3) drop the stale-ordinal stamp pass →
//     TestUXR1StaleAdjacentOrdinalFailClose red (邻近影响#9 on a 1-row ◇ 段);
//   - M-D10 (复核 P2-2) remove the trunk-admission relevance arm →
//     TestUXR1AdjacentRelevanceNeverTakesChainSeat red (trunk name capture);
//   - M-D11 (复核 P3-④) remove the blocking_span token-family arm →
//     TestUXR1BlockingSpanWearsLockGlyph red (◦配对阻塞等待 mixed signal);
//   - M-D12 (复核 P3-⑦) fork the channel word on either face →
//     TestUXR1ChannelChipWords red (fence + detail read one word source).

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/hanchaoqun/codrax/internal/types"
)

func uxr1AdjacentNode(id, object, typeToken string, rank int, eff float64, line int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: "[GT]ColdPool#6-36644", Object: object, TypeToken: typeToken,
		Predicate: "root_cause_tertiary", Rank: rank, Tier: "tertiary",
		ImpactMS: eff, CumulativeImpactMS: eff, EffectiveImpactMS: eff,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		LineStart: line, LineEnd: line + 5, Confidence: 0.6,
	}
}

func uxr1BackgroundNode(id, subject, object, typeToken, stateKind string, eff float64, line int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: subject, Object: object, TypeToken: typeToken, StateKind: stateKind,
		Predicate: "root_cause_context",
		ImpactMS:  eff, CumulativeImpactMS: eff, EffectiveImpactMS: eff,
		ChainRelevance: "background", Causality: "background",
		LineStart: line, LineEnd: line + 5, Confidence: 0.6,
	}
}

// uxr1FourOneSixFiveProjection is the 4165-isomorphic display shape: no
// resolved chain, ◇ IO/runnable contenders on the adjacent channel, ▒
// magnitude-dominant background rows (the witness's fake #1/#2), one
// cross-thread aggregate.
func uxr1FourOneSixFiveProjection() types.TraceCausalProjection {
	// The aggregate is deliberately SMALLER than the wall-clock rows: caliber
	// grouping (not magnitude) must put it first — a magnitude-only sort is
	// the mutation this fixture exists to catch.
	aggregate := uxr1BackgroundNode("E-agg", "调度压力(需求积压)·聚合", "supply_pressure", "supply_pressure", "", 30.103, 400)
	aggregate.SubjectKind = types.TraceCausalSubjectKindAggregateMetric
	return types.TraceCausalProjection{
		WindowStartTs: 2942.298, WindowEndTs: 2942.300,
		AdjacentCauses: []types.TraceCausalProjectionNode{
			uxr1AdjacentNode("E-blk", "block_io_by_inode", "block_io_by_inode", 1, 0.198, 110),
			uxr1AdjacentNode("E-lat", "io_latency", "io_latency", 2, 0.099, 120),
			uxr1AdjacentNode("E-run", "runnable_wait", "runnable_wait", 3, 0.088, 150),
		},
		BackgroundCauses: []types.TraceCausalProjectionNode{
			uxr1StaleRankBackgroundNode(),
			uxr1BackgroundNode("E-bg2", "wc_srvinit_0-37254", "d_state_or_io_wait", "d_state_or_io_wait", "d_sleep", 54.608, 210),
			aggregate,
		},
	}
}

// uxr1StaleRankBackgroundNode is the 4165 fake-#1 form: a ▒ row that still
// carries a stale persisted global ordinal — no face may print it as a seat.
func uxr1StaleRankBackgroundNode() types.TraceCausalProjectionNode {
	node := uxr1BackgroundNode("E-bg1", "SearchDaemon-37188", "runnable_wait", "runnable_wait", "runnable", 54.701, 200)
	node.Rank = 7
	node.Tier = "primary"
	return node
}

// TestUXR1ChannelChipWords — §29.36.2 配套不变量: seat chips are
// channel-worded (根因排序#N chain-only / 邻近影响#N on ◇ / nothing on ▒;
// 禁裸 #N, 禁跨通道词).
func TestUXR1ChannelChipWords(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(uxr1FourOneSixFiveProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, want := range []string{"邻近影响#1", "邻近影响#2", "邻近影响#3"} {
		if !strings.Contains(fence, want) {
			t.Fatalf("◇ contender must wear its channel chip %q:\n%s", want, fence)
		}
	}
	if strings.Contains(fence, "根因排序#") {
		t.Fatalf("a chainless board must not print the on-chain 根因排序 word (4165 自相矛盾形):\n%s", fence)
	}
	if strings.Contains(fence, "背景榜位#") {
		t.Fatalf("the 背景榜位 chip is retired (§29.36.2):\n%s", fence)
	}
	// The ▒ stale-rank row (4165 fake-#1 form) prints NO seat chip on the
	// fence face either (通道3 无序数, all faces).
	if strings.Contains(fence, "#7") {
		t.Fatalf("a ▒ stale rank must not print any seat chip (§29.36.2):\n%s", fence)
	}
	// ▒ rows carry no ordinal chip on the detail face either.
	detail := runtimeTraceProjDetailFullText(model, true)
	if strings.Contains(detail, "根因排序: #") {
		t.Fatalf("▒ rows must not print seat lines (§29.36.2 通道3 无序数):\n%s", detail)
	}
	// 复核 P3-⑦ (单一词源正面半): the detail table's ◇ seat line speaks the
	// SAME channel word the fence chip prints (runtimeTraceProjSeatChannelWord
	// is the one constructor both faces consume — forking either face reds
	// this together with the fence assertions above).
	if !strings.Contains(detail, "邻近影响: ") {
		t.Fatalf("the detail face must print the ◇ seat line under the shared channel word:\n%s", detail)
	}
}

// TestUXR1OffChainDStateGlyph — §29.36② 三面同一来源: ⛓ claims chain
// membership, so ◇/▒ D-state/IO rows wear ⧗; the on-chain form keeps ⛓.
func TestUXR1OffChainDStateGlyph(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(uxr1FourOneSixFiveProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	stanzaStart := strings.Index(fence, "◇")
	if stanzaStart < 0 {
		t.Fatalf("fixture drifted: no ◇ stanza:\n%s", fence)
	}
	if strings.Contains(fence[stanzaStart:], "⛓") {
		t.Fatalf("§29.36②: no ◇/▒ row may wear the chain glyph ⛓:\n%s", fence)
	}
	if !strings.Contains(fence[stanzaStart:], "⧗") {
		t.Fatalf("off-chain D-state/IO rows must wear ⧗:\n%s", fence)
	}
}

// TestUXR1BackgroundCaliberGrouping — §29.36.2 通道3 口径分组: cross-thread
// aggregate rows group before wall-clock rows; magnitude order within groups.
func TestUXR1BackgroundCaliberGrouping(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(uxr1FourOneSixFiveProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(model.Background) != 3 {
		t.Fatalf("fixture drifted: want 3 ▒ rows, got %d", len(model.Background))
	}
	if !model.Background[0].Node.IsAggregateMetric() {
		t.Fatalf("the cross-thread aggregate group must lead the ▒ stanza: %+v", model.Background[0].Node)
	}
	if model.Background[1].Node.EvidenceID != "E-bg1" || model.Background[2].Node.EvidenceID != "E-bg2" {
		t.Fatalf("wall-clock rows must follow in magnitude order: %+v", model.Background)
	}
}

// TestUXR1UndrillableBanner — §29.36①: the flat head is the unified
// 「⊘ <短结论>(<短因>)」 form; the 按层级平铺 note lives on the legend head
// clause only (95072 long-sentence form compressed).
func TestUXR1UndrillableBanner(t *testing.T) {
	projection := uxr1FourOneSixFiveProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⊘ 唤醒链路径未解析") {
		t.Fatalf("the flat head must wear the unified ⊘ banner:\n%s", fence)
	}
	if strings.Contains(fence, "按层级平铺") {
		t.Fatalf("the render note must leave the head line (legend head clause carries it):\n%s", fence)
	}
	// The missing-wakeup arm compresses the 95072 long sentence.
	long := projection
	long.OnChainCauses = append(long.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-ud",
		Subject: "sleeper-1", Object: "missing_wakeup", TypeToken: "missing_wakeup",
		Predicate: "root_cause_context", UndrillableReason: "missing_wakeup",
		ImpactMS: 1.0, CumulativeImpactMS: 1.0,
		LineStart: 600, LineEnd: 610, Confidence: 0.5,
	})
	longModel := buildRuntimeTraceProjTreeModel(long, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	longFence := runtimeTraceProjTreeFence(longModel, true)
	if !strings.Contains(longFence, "⊘ 唤醒链无法上溯(窗内无唤醒记录)") {
		t.Fatalf("the missing-wakeup head must speak the compressed banner:\n%s", longFence)
	}
	if strings.Contains(longFence, "睡眠区间在查询窗内无 sched_wakeup 记录") {
		t.Fatalf("the 95072 long-sentence head must stay retired:\n%s", longFence)
	}
}

// TestUXR1QualifierRow2Slot — §29.36④+.4④: the participation qualifier owns
// the 行2 限定词槽 (never 行1), qualifier first and its reasons behind it —
// the 2549 mid-value-column variant and the bare 「· running」 tail both die.
func TestUXR1QualifierRow2Slot(t *testing.T) {
	// The 2549 E2 shape: a context-only running row with a subject-only name
	// (no cause word on 行1) — the qualifier used to sit mid-value-column
	// with a bare 「· running」 row2 under it.
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-ctx",
		Subject:   "com.baidu.tieba-59566",
		StateKind: "running", Predicate: "root_cause_context",
		Tier:     types.TraceCausalTierContextOnly,
		ImpactMS: 100.0, CumulativeImpactMS: 100.0, EffectiveImpactMS: 0,
		LineStart: 10, LineEnd: 20, Confidence: 0.6,
	}
	projection := types.TraceCausalProjection{
		WindowStartTs: 100.0, WindowEndTs: 100.1,
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var mainLine, row2 string
	lines := strings.Split(fence, "\n")
	for i, line := range lines {
		if strings.Contains(line, "com.baidu.tieba-59566") && strings.Contains(line, "ms") {
			mainLine = line
			if i+1 < len(lines) {
				row2 = lines[i+1]
			}
		}
	}
	if mainLine == "" {
		t.Fatalf("fixture drifted: no rendered row:\n%s", fence)
	}
	if strings.Contains(mainLine, "上下文·不参与根因排序") {
		t.Fatalf("§29.36④: the qualifier must never sit on 行1 (mid-value-column variant): %q", mainLine)
	}
	if !strings.Contains(row2, "上下文·不参与根因排序") {
		t.Fatalf("the qualifier owns the 行2 限定词槽: %q\n%s", row2, fence)
	}
	// 限定词前、原因后: the state word follows the qualifier on the SAME row2
	// (no bare 「· running」 orphan).
	qualifierIdx := strings.Index(row2, "上下文·不参与根因排序")
	runningIdx := strings.Index(row2, "running")
	if runningIdx < 0 || runningIdx < qualifierIdx {
		t.Fatalf("the reason must follow the qualifier on row2 (裸尾巴灭): %q", row2)
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "· running" {
			t.Fatalf("bare 「· running」 orphan tail resurfaced:\n%s", fence)
		}
	}
}

// TestUXR1DedupChipRidesName — §29.36④ (140554): the ×N同值 chip rides the
// 词位 (same form as the detail table); the lone 「· ×2同值」 orphan line dies.
func TestUXR1DedupChipRidesName(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-dup",
		Subject: "com.example-5821", Object: "trace_span", TypeToken: "trace_span",
		SpanName: "H:RenderService:DoFrame", Predicate: "root_cause_context",
		Tier:     types.TraceCausalTierContextOnly,
		ImpactMS: 86.111, CumulativeImpactMS: 86.111, EffectiveImpactMS: 0,
		DuplicatePublications: 2,
		ChainRelevance:        "adjacent", Causality: "adjacent_to_wakeup_chain",
		LineStart: 30, LineEnd: 40, Confidence: 0.5,
	}
	projection := types.TraceCausalProjection{
		WindowStartTs:  100.0,
		WindowEndTs:    100.0861,
		AdjacentCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "×2同值") {
		t.Fatalf("the ×2同值 chip must render:\n%s", fence)
	}
	for _, line := range strings.Split(fence, "\n") {
		if strings.TrimSpace(line) == "· ×2同值" {
			t.Fatalf("the lone ×2同值 orphan line must stay retired (§29.36④):\n%s", fence)
		}
	}
	// 与明细表同形: the chip follows the name token on whichever line carries
	// the name.
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "×2同值") {
			if idx := strings.Index(line, "×2同值"); idx >= 0 {
				head := line[:idx]
				if !strings.Contains(head, "DoFrame") && !strings.Contains(head, "com.example-5821") &&
					!strings.Contains(head, "…") {
					t.Fatalf("the chip must ride the name token: %q", line)
				}
			}
		}
	}
}

// TestUXR1MentionObligationChannel — §29.36.3 通道4: an on-chain semantic row
// without a channel-1 seat renders unconditionally with the typed obligation
// word (SEM-LEAD 提及地板 as an explicit channel member; EVOLUTION RECORD).
func TestUXR1MentionObligationChannel(t *testing.T) {
	projection := revisit76UXR1MentionFloorProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "✦") || !strings.Contains(fence, "优化点·未入根因排序前1") {
		t.Fatalf("the channel-4 obligation seat must render with its typed word:\n%s", fence)
	}
	// Promotion boundary: the same row WITH a seat is channel 1 (可冕可戴) and
	// never wears the obligation word.
	promoted := revisit76UXR1MentionFloorProjection()
	promoted.SemanticSpans[0].Rank = 1
	promotedModel := buildRuntimeTraceProjTreeModel(promoted, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	promotedFence := runtimeTraceProjTreeFence(promotedModel, true)
	if strings.Contains(promotedFence, "优化点·未入根因排序") {
		t.Fatalf("a TOP-N semantic row is channel 1 — no obligation word:\n%s", promotedFence)
	}
	if !strings.Contains(promotedFence, "根因排序#1") {
		t.Fatalf("the promoted semantic row must wear its channel-1 seat:\n%s", promotedFence)
	}
}

// TestUXR1StaleAdjacentOrdinalFailClose — 复核 P2-3: a stale persisted GLOBAL
// ordinal (old single-rankPos engine note) replayed on the ◇ channel must not
// be re-worded as a fresh channel ordinal — an ordinal EXCEEDING the adjacent
// channel's rendered population fail-closes the chip and the detail seat line
// (symmetric with the ▒ stale-Rank fail-close); the row itself stays rendered
// with its confidence. Boundary: a rank within the population keeps its chip.
func TestUXR1StaleAdjacentOrdinalFailClose(t *testing.T) {
	stale := uxr1AdjacentNode("E-stale", "io_latency", "io_latency", 9, 0.099, 120)
	projection := types.TraceCausalProjection{
		WindowStartTs: 2942.298, WindowEndTs: 2942.300,
		AdjacentCauses: []types.TraceCausalProjectionNode{stale},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "#9") || strings.Contains(fence, "邻近影响#") {
		t.Fatalf("a stale global ordinal (9 > ◇ population 1) must not wear any channel chip:\n%s", fence)
	}
	if !strings.Contains(fence, "[GT]ColdPool#6-36644") {
		t.Fatalf("the stale-ordinal row itself must stay rendered:\n%s", fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if strings.Contains(detail, "邻近影响: ") {
		t.Fatalf("the detail seat line must fall to the bare 置信 form on a stale ordinal:\n%s", detail)
	}
	// Boundary: the same shape with a population-consistent ordinal keeps its
	// chip (the fail-close never robs a legitimate contiguous board).
	legit := uxr1AdjacentNode("E-ok", "io_latency", "io_latency", 1, 0.099, 120)
	okModel := buildRuntimeTraceProjTreeModel(types.TraceCausalProjection{
		WindowStartTs: 2942.298, WindowEndTs: 2942.300,
		AdjacentCauses: []types.TraceCausalProjectionNode{legit},
	}, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if !strings.Contains(runtimeTraceProjTreeFence(okModel, true), "邻近影响#1") {
		t.Fatalf("a population-consistent ◇ ordinal must keep its chip")
	}
}

// TestUXR1AdjacentRelevanceNeverTakesChainSeat — 复核 P2-2 裁定: an
// adjacent-relevance PrimaryRootCauses row whose canonical subject collides
// with a trunk subject must NOT be captured into a Kind="chain" tree seat
// (the trunk name capture / depth attach carry the typed relevance arm), and
// the CLOSE-1 shared valid-seat gate's channel belt keeps it off the ❶/crown
// lanes — it renders on its own ◇ channel with the 邻近影响#N chip word
// (same-page identity split killed at both layers).
func TestUXR1AdjacentRelevanceNeverTakesChainSeat(t *testing.T) {
	deep := func(subject string, depth int, eff float64, id string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: id,
			Subject: subject, Object: "sleep_wait", TypeToken: "sleep_wait",
			StateKind: "s_sleep", Predicate: "wakeup_causal_impact",
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: depth,
			ImpactMS: eff, CumulativeImpactMS: eff,
			LineStart: 100 + depth*10, LineEnd: 105 + depth*10, Confidence: 0.7,
		}
	}
	// The 54476 overlay probe shape (TestUXR1BarColumnAlignment fixture) plus
	// the leak row: a PRIMARY-lane row with typed ADJACENT relevance whose
	// subject name-collides with the depth-1 trunk subject.
	leak := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E-leak",
		Subject: "CookieMonsterCl-59843", Object: "io_latency", TypeToken: "io_latency",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 42.0, CumulativeImpactMS: 42.0, EffectiveImpactMS: 42.0,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		LineStart: 500, LineEnd: 510, Confidence: 0.7,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:        []string{"ThreadPoolForeg-60555", "NetworkService-60595", "CookieMonsterCl-59843", "com.baidu.tieba-59566"},
		WindowStartTs:     100.0, WindowEndTs: 100.11494,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{leak},
		OnChainCauses: []types.TraceCausalProjectionNode{
			deep("CookieMonsterCl-59843", 1, 42.131, "E-l1"),
			deep("NetworkService-60595", 2, 25.558, "E-l2"),
			deep("ThreadPoolForeg-60555", 3, 17.442, "E-l3"),
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if row.Node.EvidenceID == "E-leak" {
			t.Fatalf("adjacent-relevance row captured into the chain lane (Kind=%q) — P2-2 name-key leak:\n%+v",
				row.Kind, row.Node)
		}
	}
	var stanza *runtimeTraceProjTreeRow
	for i := range model.Adjacent {
		if model.Adjacent[i].Node.EvidenceID == "E-leak" {
			stanza = &model.Adjacent[i]
		}
	}
	if stanza == nil {
		t.Fatalf("the rejected row must keep its honest ◇ stanza seat (落其本通道渲染位): %+v", model.Adjacent)
	}
	if stanza.Badge != 0 {
		t.Fatalf("零徽章: a ◇ channel row never wears ❶..❺, got badge %d", stanza.Badge)
	}
	if seat, ok := runtimeTraceProjRowValidSeat(*stanza); ok || seat != 0 {
		t.Fatalf("the CLOSE-1 channel belt must refuse a non-chain-channel seat: seat=%d ok=%v", seat, ok)
	}
	// P2-2(b) channel-belt DIRECT arm (defense in depth beyond the lane-kind
	// switch): even a stale persisted CHAIN-lane row form carrying typed
	// adjacent relevance holds no valid seat — removing the belt alone (with
	// the admission arm intact) reds exactly here.
	leakChainForm := runtimeTraceProjTreeRow{Node: leak, Kind: runtimeTraceProjTreeRowChain, HasData: true}
	if seat, ok := runtimeTraceProjRowValidSeat(leakChainForm); ok || seat != 0 {
		t.Fatalf("channel belt: a Kind=chain row with adjacent relevance must hold no valid seat: seat=%d ok=%v", seat, ok)
	}
	if model.LeadKey == runtimeTraceCausalProjectionNodeKey(leak) {
		t.Fatalf("零冠: the ◇ row must not be crowned 主根因")
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "邻近影响#1") {
		t.Fatalf("the ◇ seat keeps its channel-worded chip:\n%s", fence)
	}
	if strings.Contains(fence, "根因排序#1") || strings.Contains(fence, "❶") {
		t.Fatalf("the leak row must not surface a chain seat or badge anywhere:\n%s", fence)
	}
}

// TestUXR1BlockingSpanWearsLockGlyph — 复核 P3-④ pin (the §29.36.1 140554
// arm previously had no guard): a blocking_span row whose BlockingKind never
// resolved still classifies onto the ⊗ lock family through its TYPED token —
// reverting the token-family arm re-creates the ◦-beside-阻塞等待 mixed
// signal and reds this.
func TestUXR1BlockingSpanWearsLockGlyph(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-bspan",
		Subject: "com.tencent.mm-36555", Object: "blocking_span", TypeToken: "blocking_span",
		SpanName: "MMKV lock wait", Predicate: "root_cause_secondary",
		Rank: 1, Tier: "secondary",
		ImpactMS: 12.0, CumulativeImpactMS: 12.0, EffectiveImpactMS: 12.0,
		LineStart: 700, LineEnd: 710, Confidence: 0.7,
	}
	if form := runtimeTraceProjImpactFormForNode(node, ""); form != runtimeTraceProjImpactFormLock {
		t.Fatalf("an unresolved blocking_span must classify onto the lock family via its typed token, got form %d", form)
	}
	projection := types.TraceCausalProjection{
		WindowStartTs: 100.0, WindowEndTs: 100.05,
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	var rowLine string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "com.tencent.mm-36555") && strings.Contains(line, "ms") {
			rowLine = line
			break
		}
	}
	if rowLine == "" {
		t.Fatalf("fixture drifted: no rendered blocking_span row:\n%s", fence)
	}
	if !strings.Contains(rowLine, "⊗") {
		t.Fatalf("the blocking_span row must wear the ⊗ lock glyph: %q", rowLine)
	}
	if strings.Contains(rowLine, "◦") {
		t.Fatalf("◦ beside a blocking form word is the 140554 mixed signal: %q", rowLine)
	}
}

// TestUXR1LanePrefixSimplified — §29.36④: the 深度未解析/未接入树 auxiliary
// words left the lane prefix (链上─), riding the 行2 chip family instead.
func TestUXR1LanePrefixSimplified(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0, WindowEndTs: 100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-dl",
			Subject: "helper-7", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_sleep", Predicate: "root_cause_secondary",
			Rank: 1, Tier: "secondary", ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			ImpactMS: 8.0, CumulativeImpactMS: 8.0, EffectiveImpactMS: 8.0,
			LineStart: 50, LineEnd: 60, Confidence: 0.7,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "链上·深度未解析─") || strings.Contains(fence, "链上·未接入树─") {
		t.Fatalf("the auxiliary words must leave the lane prefix (§29.36④):\n%s", fence)
	}
	if !strings.Contains(fence, "链上─") {
		t.Fatalf("the simplified 链上─ edge must render:\n%s", fence)
	}
	if !strings.Contains(fence, "链上·深度未解析") {
		t.Fatalf("the 深度未解析 word must ride 行2:\n%s", fence)
	}
}

// TestUXR1BarColumnAlignment — §29.36⑤ (54476): every bar in one fence starts
// at the SAME display column, deep chain levels included (bar 列全树固定).
func TestUXR1BarColumnAlignment(t *testing.T) {
	deep := func(subject string, depth int, eff float64, id string, last bool) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: id,
			Subject: subject, Object: "sleep_wait", TypeToken: "sleep_wait",
			StateKind: "s_sleep", Predicate: "wakeup_causal_impact",
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: depth,
			ImpactMS: eff, CumulativeImpactMS: eff,
			LineStart: 100 + depth*10, LineEnd: 105 + depth*10, Confidence: 0.7,
		}
	}
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"ThreadPoolForeg-60555", "NetworkService-60595", "CookieMonsterCl-59843", "com.baidu.tieba-59566"},
		WindowStartTs: 100.0, WindowEndTs: 100.11494,
		OnChainCauses: []types.TraceCausalProjectionNode{
			deep("CookieMonsterCl-59843", 1, 42.131, "E-l1", false),
			deep("NetworkService-60595", 2, 25.558, "E-l2", false),
			deep("ThreadPoolForeg-60555", 3, 17.442, "E-l3", true),
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	barCol := -1
	for _, line := range strings.Split(fence, "\n") {
		idx := strings.IndexAny(line, "█░▒")
		if idx < 0 || strings.HasPrefix(strings.TrimSpace(line), "▒ 背景") {
			continue
		}
		col := runewidth.StringWidth(line[:idx])
		if barCol < 0 {
			barCol = col
			continue
		}
		if col != barCol {
			t.Fatalf("bar columns drift across depths (§29.36⑤ bar 列全树固定): %d vs %d\n%s", col, barCol, fence)
		}
	}
	if barCol < 0 {
		t.Fatalf("fixture drifted: no bar rows rendered:\n%s", fence)
	}
}
