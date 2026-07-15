package tool

// answer_document_projection_rnb5b_test.go — RNB-5B (§29.96.2 终判 ②③④⑥⑦⑨⑪
// display batch) pins for the tool-half items:
//
//	件③  「接近<basis>」 double gate (绝对<1.0ms ∧ 相对<15%) — the E8 negative
//	     witness form (raw 1.409 / deficit 0.933, relative 66%) drops the 接近
//	     claim and states the deficit + R5b mention only; passing forms keep
//	     the legacy bytes.
//	件⑥  单次最大 equation face keys on the typed ENGINE wire-fold source bit
//	     (MergedWireFold) — the Σ-equals-largest-member coincidence no longer
//	     wears the word.
//	件⑦  chain-lane micro anchored cut seats (<0.1ms) fold into ONE counted ⛓
//	     row (其余N项微额锚定席, account-sum caliber) on the tree AND the ◎
//	     board; non-micro seats and single-micro shapes keep their rows.
//
// MUTATION self-checks:
//   - dropping the relative arm of runtimeTraceProjNearPeakDoubleGate reds
//     TestRNB5BNearPeakDoubleGateDropsNearClaim (the E8 form re-wears 接近);
//   - dropping the MergedWireFold requirement in
//     runtimeTraceProjCauseEventFoldRow reds
//     TestRNB5BSingleMaxCoincidenceDoesNotWearWord;
//   - dropping the runtimeTraceProjFoldMicroAnchorSeats pass reds
//     TestRNB5BMicroAnchorSeatsFoldOnTreeAndBoard (individual micro rows
//     resurface).

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func rnb5bSupplyFoldNode(ideal, deficit, eff float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		SupplyFoldComputed:  true,
		SupplyFoldIdealMS:   ideal,
		SupplyFoldDeficitMS: deficit,
		SupplyFoldKnownMS:   ideal + deficit,
		EffectiveImpactMS:   eff,
	}
}

// 件③ negative witness (E8 form: donghu 17267 E19 — raw 1.409, deficit 0.933,
// relative 66%): the 接近 claim drops; the clause states the deficit + the
// R5b mention only.
func TestRNB5BNearPeakDoubleGateDropsNearClaim(t *testing.T) {
	node := rnb5bSupplyFoldNode(0.476, 0.933, 0.933)
	text, keep, ok := runtimeTraceProjSupplyFoldClauseCore(node, 233.190, true)
	if !ok {
		t.Fatalf("the clause must render")
	}
	if strings.Contains(text, "接近") {
		t.Fatalf("relative-gate failure (66%%) must not wear the 接近 claim: %q", text)
	}
	if text != "供给折算缺口 0.933ms(运行频点非最高,已计入有效归因)" || keep != "供给折算缺口" {
		t.Fatalf("the gate-failed form states the deficit + R5b mention only: %q keep=%q", text, keep)
	}
	// EN twin.
	text, keep, ok = runtimeTraceProjSupplyFoldClauseCore(node, 233.190, false)
	if !ok || strings.Contains(text, "near ") ||
		text != "supply-fold deficit 0.933ms (running below peak frequency, counted into the attribution)" ||
		keep != "supply-fold deficit" {
		t.Fatalf("EN gate-failed form drifted: %q keep=%q ok=%v", text, keep, ok)
	}
}

// 件③ absolute arm: a deficit ≥1.0ms below the share threshold never wears
// 接近 either (double gate, both arms live).
func TestRNB5BNearPeakAbsoluteArmDropsNearClaim(t *testing.T) {
	node := rnb5bSupplyFoldNode(10.8, 1.2, 0)
	text, _, ok := runtimeTraceProjSupplyFoldClauseCore(node, 233.190, true)
	if !ok || strings.Contains(text, "接近") {
		t.Fatalf("absolute-gate failure (1.2ms) must not wear 接近: %q ok=%v", text, ok)
	}
	if text != "供给折算缺口 1.200ms(运行频点非最高,独立口径,不计入有效归因)" {
		t.Fatalf("uncounted gate-failed form drifted: %q", text)
	}
}

// 件③ positive arm: a genuinely近 seat (abs <1.0 ∧ rel <15%) keeps the legacy
// 接近 bytes.
func TestRNB5BNearPeakDoubleGateKeepsNearClaim(t *testing.T) {
	node := rnb5bSupplyFoldNode(10.0, 0.05, 0.05)
	text, keep, ok := runtimeTraceProjSupplyFoldClauseCore(node, 233.190, true)
	if !ok || text != "接近全域最大核最高频,缺口仅 0.050ms(运行频点非最高,已计入有效归因)" ||
		keep != "接近全域最大核最高频" {
		t.Fatalf("double-gate pass must keep the legacy 接近 form: %q keep=%q ok=%v", text, keep, ok)
	}
}

// 件⑥ negative pin (coincidence form): a display-merged cause row whose Σ
// happens to equal its largest member (other members zero-eff) must NOT wear
// the 单次最大 equation — no typed wire-fold source bit.
func TestRNB5BSingleMaxCoincidenceDoesNotWearWord(t *testing.T) {
	node := elimChainNode("E-coin", "worker-77", "io_latency", "", 3, 5.0, 100)
	node.MergedCount = 3
	node.MergedMinMS = 0.0
	node.MergedMaxMS = 5.0
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E-coin"}
	if runtimeTraceProjCauseEventFoldRow(row) {
		t.Fatalf("the numeric coincidence (Σ==largest member) must not trigger the event form")
	}
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-77", "app-1"},
		WindowStartTs: 100.0, WindowEndTs: 100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "单次最大") {
		t.Fatalf("the coincidence row must not render the 单次最大 word:\n%s", fence)
	}
}

// 件⑥ positive arm: the typed wire-fold source bit (engine folded_* lane)
// keeps the event form alive.
func TestRNB5BSingleMaxWireFoldBitTriggersWord(t *testing.T) {
	node := elimChainNode("E-wire", "worker-77", "io_latency", "", 3, 5.0, 100)
	node.MergedCount = 3
	node.MergedMinMS = 1.0
	node.MergedMaxMS = 5.0
	node.MergedWireFold = true
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E-wire"}
	if !runtimeTraceProjCauseEventFoldRow(row) {
		t.Fatalf("the typed wire-fold bit + µs identity must trigger the event form")
	}
	// 修复轮 P3-⑥: the µs-identity consistency guard stays live ON TOP of the
	// source bit — a wire-fold row whose eff drifted off its member MAX must
	// not print the false equation (拒渲绝不造数).
	drifted := node
	drifted.EffectiveImpactMS = 4.2
	driftedRow := runtimeTraceProjTreeRow{Node: drifted, Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E-wire"}
	if runtimeTraceProjCauseEventFoldRow(driftedRow) {
		t.Fatalf("eff≠MergedMaxMS must fail the event form even with the wire-fold bit")
	}
}

// 件⑥ wire-parse pin lives beside the parser:
// internal/types/trace_causal_projection_rnb5b_test.go
// (TestRNB5BWireFoldBitMintsAtFoldedNotesOnly).

// 件② display half: a target-self count row carrying the NON-CHANNEL
// self_caliber_side wire token re-seats in the self stanza on the ⌗ 口径旁栏
// lane — the ⌗ caliber word alone, NO 非链 channel qualifier — and the ◎
// footnote keeps mentioning it; the legacy adjacent-relevance relocated form
// keeps the 非链 word byte-identically (the selfall pins).
func TestRNB5BSelfCaliberSideTokenDropsChannelWords(t *testing.T) {
	count := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-pgc",
		Subject: "app-42", Object: "page_cache_churn", TypeToken: "page_cache_churn",
		ChainRelevance: "self_caliber_side", Tier: types.TraceCausalTierCaliberSide,
		Predicate: "root_cause_context", ImpactMS: 81.616, CumulativeImpactMS: 81.616,
		EffectiveImpactMS: 81.616, Confidence: 0.66, LineStart: 955, LineEnd: 20250,
	}
	chainSeat := elimChainNode("E-run", "app-42", "runnable_wait", "runnable", 1, 3.0, 100)
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"waker-1", "app-42"},
		WindowStartTs:           100.0, WindowEndTs: 100.24,
		OnChainCauses:  []types.TraceCausalProjectionNode{chainSeat},
		AdjacentCauses: []types.TraceCausalProjectionNode{count},
	}
	model, elim := elimRenderOverview(t, projection, true)
	fence := runtimeTraceProjTreeFence(model, true)
	// The row relocates into the self stanza (no target row may keep a ◇ seat).
	for _, row := range model.Adjacent {
		if runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) == "app-42" {
			t.Fatalf("no target-subject row may keep a ◇ seat: %+v", row.Node)
		}
	}
	if !strings.Contains(fence, "⌗口径旁栏") {
		t.Fatalf("the ⌗ caliber word must ride the relocated row:\n%s", fence)
	}
	if strings.Contains(fence, "非链") {
		t.Fatalf("件②: the non-channel token row must not wear the 非链 channel qualifier:\n%s", fence)
	}
	// The ◎ footnote still mentions the ⌗ row (排除≠消失).
	if !strings.Contains(elim, "81.616·⌗口径旁栏·计数当量(非墙钟,不占序数)") {
		t.Fatalf("the ◎ ⌗ footnote must keep mentioning the side-rail row:\n%s", elim)
	}
}

// 件② pointer word: a cross-stanza (chain seat ↔ ⌗ side-rail row) pair
// speaks 口径旁栏行, never 邻近席 (the channel word would lie).
func TestRNB5BSelfCaliberSidePointerWord(t *testing.T) {
	chainRow := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{Subject: "thread-9", ChainRelevance: "on_chain",
			ImpactMS: 2.0, EffectiveImpactMS: 2.0, Rank: 3}, Kind: runtimeTraceProjTreeRowChain,
		HasData: true, EvidenceTag: "E1",
	}
	caliberRow := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{Subject: "thread-9", ChainRelevance: "self_caliber_side",
			Tier: types.TraceCausalTierCaliberSide, TypeToken: "page_cache_churn",
			ImpactMS: 81.0, EffectiveImpactMS: 81.0}, Kind: runtimeTraceProjTreeRowSelf,
		HasData: true, EvidenceTag: "E2",
	}
	model := runtimeTraceProjTreeModel{
		SelfRows: []runtimeTraceProjTreeRow{caliberRow},
		TreeRows: []runtimeTraceProjTreeRow{chainRow},
	}
	runtimeTraceProjMarkCrossChannelSameThread(&model)
	if model.TreeRows[0].CrossChannelCaliberRef != "E2" {
		t.Fatalf("the chain seat must point at the side-rail row via the caliber ref: %+v", model.TreeRows[0])
	}
	if model.TreeRows[0].CrossChannelAdjacentRef != "" {
		t.Fatalf("件②: the 邻近席 pointer must not name a ⌗ row: %+v", model.TreeRows[0])
	}
	if model.SelfRows[0].CrossChannelChainRef != "E1" {
		t.Fatalf("the side-rail row keeps the forward 链上席 pointer: %+v", model.SelfRows[0])
	}
}

// 默认小件a (§29.95 UX-2 「最大席最寡言」): the SELF running-deficit seat's
// stanza block carries the single-source supply-fold mechanism clause (R5b
// mention + thermal sentence) the flat/chain rows wear.
func TestRNB5BSelfRunningDeficitSeatCarriesMechanismClause(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E-selfrun",
		Subject: "app-42", Object: "running", TypeToken: "running",
		StateKind: "running", ChainRelevance: "on_chain", OnChainBasis: "self_wall_clock_interval",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 157.248, CumulativeImpactMS: 157.248, EffectiveImpactMS: 58.320,
		SupplyFoldComputed: true, SupplyFoldIdealMS: 98.928, SupplyFoldDeficitMS: 58.320,
		SupplyFoldKnownMS: 157.248,
		ThermalCapKHz:     1530000, ThermalCapWitnessed: true,
		Confidence: 0.86, LineStart: 10, LineEnd: 20,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app-42"},
		WindowStartTs: 100.0, WindowEndTs: 100.24,
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	joined := strings.ReplaceAll(fence, "\n", "")
	if !strings.Contains(joined, "供给折算缺口 58.320ms(运行频点非最高,按全域最大核最高频折算,下界)为主") {
		t.Fatalf("the self seat must carry the same-source mechanism clause (R5b mention riding):\n%s", fence)
	}
	if !strings.Contains(joined, "窗内该簇受热限压至 1.53GHz") {
		t.Fatalf("the THERM disclosure must ride the self seat's clause:\n%s", fence)
	}
}

// 默认小件c (§29.95 UX-4 对称): family-arm self D/IO seats wear 自身·墙钟席
// even without the SELF-ALL basis token; symptom/context/semantic self rows
// stay bare (each has its own word).
func TestRNB5BSelfWallClockQualifierCoversFamilySeats(t *testing.T) {
	family := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-iofam",
		Subject: "app-42", Object: "io_latency", TypeToken: "io_latency",
		StateKind: "io_wait", ChainRelevance: "on_chain", // no OnChainBasis token
		Predicate: "root_cause_tertiary", Rank: 2, Tier: "tertiary",
		ImpactMS: 3.264, CumulativeImpactMS: 3.264, EffectiveImpactMS: 3.264,
		Confidence: 0.8, LineStart: 30, LineEnd: 40,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app-42"},
		WindowStartTs: 100.0, WindowEndTs: 100.24,
		OnChainCauses: []types.TraceCausalProjectionNode{family},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "自身·墙钟席") {
		t.Fatalf("a family-arm self D/IO cause seat must wear the qualifier (UX-4 对称):\n%s", fence)
	}
	// Negative arm: the target's wait-SYMPTOM row never wears it.
	symptom := family
	symptom.EvidenceID = "E-sym"
	symptom.Object = "binder_wait"
	symptom.TypeToken = "binder_wait"
	symptom.Tier = types.TraceCausalTierTargetSelfState
	symptom.Predicate = "root_cause_target_self_state"
	symptom.Rank = 0
	projection.OnChainCauses = []types.TraceCausalProjectionNode{symptom}
	model = buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "自身·墙钟席") {
		t.Fatalf("a self wait-symptom row must stay bare:\n%s", fence)
	}
}

// 默认小件e (§29.97 冷读观察③): the R3 edge-anchored seat wears the short
// 边锚定 qualification chip on the ◎ board (chip family of 自身·墙钟席).
func TestRNB5BElimBoardEdgeAnchoredChip(t *testing.T) {
	projection := rnb5bMicroAnchorFoldProjection()
	edge := elimChainNode("E-edge", "hosthread-88", "trace_semantic_span", "running", 2, 0.285, 200)
	edge.SemanticClass = "class_verify"
	edge.OnChainBasis = "host_wakeup_edge_pre_span"
	projection.OnChainCauses = append(projection.OnChainCauses, edge)
	_, elim := elimRenderOverview(t, projection, true)
	found := false
	for _, line := range elimOverviewMemberLines(elim) {
		if strings.Contains(line, "hosthread-88") {
			found = true
			if !strings.Contains(line, "·边锚定") {
				t.Fatalf("the edge-anchored seat must wear the 边锚定 chip: %q", line)
			}
		}
	}
	if !found {
		t.Fatalf("fixture drifted: the edge-anchored seat must sit on the board:\n%s", elim)
	}
}

// 件⑨: a multi-window merged seat whose chip window is typed-unresolvable
// states 多窗(端点见明细) on 行2 instead of guessing endpoints; a seat with a
// resolvable window keeps the legacy 窗X~Ys(供席成员窗,成员跨K窗) chip.
func rnb5bMultiWindowNoEndpointProjection() types.TraceCausalProjection {
	// Merged values follow the legend probe canon (3次(10.000~30.000ms) — the
	// ×N sum form's probe values) so the bidirectional fixture set can adopt
	// this projection as the 件⑨ representative shape.
	node := elimChainNode("E-mw", "worker-31", "runnable_wait", "runnable", 2, 60.0, 100)
	node.ChainRelevance = "adjacent"
	node.Causality = ""
	node.MergedCount = 3
	node.MergedMinMS = 10.0
	node.MergedMaxMS = 30.0
	node.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{
		{StartTs: 100.0, EndTs: 100.2}, {StartTs: 200.0, EndTs: 200.3},
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"other-1", "app-2"},
		WindowStartTs: 100.0, WindowEndTs: 100.2,
		AdjacentCauses: []types.TraceCausalProjectionNode{node},
	}
}

func TestRNB5BMultiWindowChipEndpointsUnresolvable(t *testing.T) {
	projection := rnb5bMultiWindowNoEndpointProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "·多窗(端点见明细)") {
		t.Fatalf("the unresolvable multi-window seat must state the endpoint-less chip on 行2:\n%s", fence)
	}
	if strings.Contains(fence, "窗0.000~0.000s") {
		t.Fatalf("no fabricated endpoints may render:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkMultiWindowNoEndpoints) {
		t.Fatalf("the endpoint-less chip legend mark must record at the emission site")
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "`多窗(端点见明细)`") {
		t.Fatalf("the endpoint-less chip legend entry must render:\n%s", lead)
	}
	// Negative arm: a resolvable window keeps the legacy chip byte-identically.
	projection.AdjacentCauses[0].QueryWindowStartTs = 100.0
	projection.AdjacentCauses[0].QueryWindowEndTs = 100.2
	model = buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "窗100.000~100.200s(供席成员窗,成员跨2窗)") {
		t.Fatalf("the resolvable form keeps the legacy chip:\n%s", fence)
	}
	if strings.Contains(fence, "多窗(端点见明细)") {
		t.Fatalf("the resolvable form must not wear the endpoint-less chip:\n%s", fence)
	}
}

// 件⑦: the donghu-2955 shape — three chain-lane anchored cut seats below the
// micro threshold fold into ONE counted ⛓ row on the tree and the ◎ board;
// the ◇ remainder twins and the non-micro seats stay untouched.
func rnb5bMicroAnchorFoldProjection() types.TraceCausalProjection {
	micro := func(id, subject string, rank int, anchored, full float64, line int) types.TraceCausalProjectionNode {
		node := elimChainNode(id, subject, "runnable_wait", "runnable", rank, anchored, line)
		node.ChainAnchoredMS = anchored
		node.ChainAnchorFullMS = full
		return node
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"CompThread_0-2955", "app-100"},
		WindowStartTs:           100.0, WindowEndTs: 100.24,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("E-dst", "CompThread_0-2955", "d_state_or_io_wait", "d_sleep", 1, 36.757, 50),
			micro("E-m1", "hilogcat-9503", 5, 0.026, 17.292, 100),
			micro("E-m2", "logd.writer-9163", 6, 0.018, 49.656, 110),
			micro("E-m3", "logd.reader.per-9522", 7, 0.016, 10.661, 120),
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-rem1",
			Subject: "hilogcat-9503", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 17.266, CumulativeImpactMS: 17.266,
			ChainAnchoredMS: 0.026, ChainAnchorFullMS: 17.292, ChainAnchorRemainderSeat: true,
			Rank: 1, Confidence: 0.8, LineStart: 100, LineEnd: 105,
		}},
	}
}

func TestRNB5BMicroAnchorSeatsFoldOnTreeAndBoard(t *testing.T) {
	projection := rnb5bMicroAnchorFoldProjection()
	model, elim := elimRenderOverview(t, projection, true)
	fence := runtimeTraceProjTreeFence(model, true)
	joined := strings.ReplaceAll(fence, "\n", "")
	if !strings.Contains(fence, "其余 3 项微额锚定席") {
		t.Fatalf("the tree must fold the three micro cut seats into one counted row:\n%s", fence)
	}
	if !strings.Contains(joined, "3席合计(账目合计,单席0.016~0.026ms,均<0.1ms)·凭证锚定段,仍属⛓链上通道·见明细") {
		t.Fatalf("the fold row's account-sum caliber tag must render:\n%s", fence)
	}
	// The Σ value channel (0.026+0.018+0.016 = 0.060) rides the fold row.
	if !strings.Contains(fence, "0.060ms") {
		t.Fatalf("the fold row must publish the account Σ 0.060ms:\n%s", fence)
	}
	// No individual micro seat row survives on the tree face (their E#s stay
	// reachable via the fold bracket and the evidence index).
	for _, form := range []string{"hilogcat-9503 · runnable ", "logd.writer-9163 · runnable "} {
		for _, line := range strings.Split(fence, "\n") {
			if strings.Contains(line, form) && strings.Contains(line, "0.0") &&
				!strings.Contains(line, "微额锚定席") && !strings.Contains(line, "17.266") {
				t.Fatalf("an individual micro cut seat must not keep its own tree row: %q", line)
			}
		}
	}
	// The fold bracket keeps every member's registered E# on-page (the ◇
	// twins' 同源二分 refs stay resolvable).
	if !strings.Contains(joined, "[E2+E3+E4]") {
		t.Fatalf("the fold bracket must keep every member E# on-page:\n%s", fence)
	}
	// ◎ board: the fold line renders on the ⛓ channel with the pointer.
	if !strings.Contains(elim, "⛓ 链上 · 其余 3 项微额锚定席·合计(账目合计,均<0.1ms)见明细") {
		t.Fatalf("the ◎ board must carry the fold line on the chain channel:\n%s", elim)
	}
	for _, line := range elimOverviewMemberLines(elim) {
		if strings.Contains(line, "0.026ms") || strings.Contains(line, "0.018ms") {
			t.Fatalf("no individual micro seat may keep a board line: %q", line)
		}
	}
	// The ◇ remainder twin stays untouched (negative arm).
	if !strings.Contains(fence, "17.266ms") {
		t.Fatalf("the ◇ remainder twin must keep its row:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkMicroAnchorFold) {
		t.Fatalf("the micro-fold legend mark must record at the emission site")
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "`其余N项微额锚定席`") {
		t.Fatalf("the micro-fold legend entry must render:\n%s", lead)
	}
	// 修复轮 U3 (席位记忆): the detail block remembers the folded members'
	// ordinal range; 修复轮 U4 (全员一致态词): three runnable members keep the
	// runnable state word on the fold node (never 「未分类」).
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "#5~#7(折叠合一)") {
		t.Fatalf("U3: the detail block must remember the folded seat range:\n%s", detail)
	}
	for _, row := range model.TreeRows {
		if row.Node.MicroAnchorFold {
			if row.Node.StateKind != "runnable" {
				t.Fatalf("U4: a uniform member state survives onto the fold node: %+v", row.Node)
			}
		}
	}
}

// 修复轮 P1-1 (F8 census, donghu 2955 engine-real): the micro fold's members
// stay in the 「另有 N 条链上行未计入」 census by member count — the fold
// silently shrank it 21→18 (the folded seats were individually counted
// Depthless rows pre-fold). MAX competes on the largest SINGLE member value,
// never the fold's Σ.
//
// MUTATION self-check: removing the MicroAnchorFold arm in the census
// TreeRows loop (runtimeTraceProjUnadmittedOnChainDisclosure) reds this pin
// (count falls back to 18).
func TestRNB5BMicroFoldCensusCountsMembersDonghu2955(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "fixture", "p-"+view, "r", "", at)...)
	}
	projection := types.TraceCausalProjectionFromObservationRecords(obs)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	// The micro fold must be live on this witness (fixture drift guard).
	foldLive := false
	for _, row := range model.TreeRows {
		if row.Node.MicroAnchorFold {
			foldLive = true
			if row.MicroAnchorFoldDepthlessMembers != 3 {
				t.Fatalf("the 2955 fold must carry 3 depthless members: %+v", row)
			}
		}
	}
	if !foldLive {
		t.Fatalf("fixture drifted: the 2955 micro fold must mint")
	}
	n, x, _ := runtimeTraceProjUnadmittedOnChainDisclosure(model)
	if n != 21 {
		t.Fatalf("F8: the census must count fold members (want 21, got %d)", n)
	}
	if x < 74.9 || x > 74.93 {
		t.Fatalf("the census MAX stays the largest single row value (want ≈74.915, got %.3f)", x)
	}
}

// 修复轮 D2: ⌗ 口径旁栏 rows wear the ⌗ row-head glyph — never the ⛓ channel
// glyph (self count row) nor the ⧗ scheduler-state glyph (◇ count row);
// wall-clock rows keep their state glyphs (negative arm).
//
// MUTATION self-check: removing the caliber-side icon arm in
// runtimeTraceProjStateIcon reds this pin (the self count row falls back
// into the ⛓ arm).
func TestRNB5BCaliberSideRowsWearOwnGlyph(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app-42"},
		WindowStartTs: 100.0, WindowEndTs: 100.24,
		OnChainCauses: []types.TraceCausalProjectionNode{elimChainNode("E-run", "app-42", "runnable_wait", "runnable", 1, 3.0, 100)},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-pgc",
			Subject: "app-42", Object: "page_cache_churn", TypeToken: "page_cache_churn",
			ChainRelevance: "self_caliber_side", Tier: types.TraceCausalTierCaliberSide,
			Predicate: "root_cause_context", ImpactMS: 81.616, CumulativeImpactMS: 81.616,
			EffectiveImpactMS: 81.616, Confidence: 0.66, LineStart: 955, LineEnd: 20250,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⌗ 自身·页缓存抖动") {
		t.Fatalf("the self count row must wear the ⌗ row-head glyph:\n%s", fence)
	}
	for _, wrong := range []string{"⛓ 自身·页缓存抖动", "⧗ 自身·页缓存抖动"} {
		if strings.Contains(fence, wrong) {
			t.Fatalf("the ⌗ row must not wear a channel/state glyph %q:\n%s", wrong, fence)
		}
	}
	if !model.Marks.has(runtimeTraceProjMarkIconCaliberSide) {
		t.Fatalf("the ⌗ glyph legend mark must record at the emission site")
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "`⌗`(行首)") {
		t.Fatalf("the ⌗ glyph legend entry must render:\n%s", lead)
	}
	// Negative arm: the wall-clock runnable seat keeps its state glyph.
	if !strings.Contains(fence, "⧖") {
		t.Fatalf("wall-clock rows keep their state glyphs:\n%s", fence)
	}
}

// 修复轮 U6/P3-⑦: the count family's 行3 equation and (a) table value cells
// drop the wall-clock ms suit (suffix-free count-equivalent form, 同 行1/◎
// 脚注单源).
func TestRNB5BCountFamilyEquationAndCellsSuffixFree(t *testing.T) {
	count := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E-pgc",
		Subject: "app-42", Object: "page_cache_churn", TypeToken: "page_cache_churn",
		ChainRelevance: "self_caliber_side", Tier: types.TraceCausalTierCaliberSide,
		Predicate: "root_cause_context", ImpactMS: 81.616, CumulativeImpactMS: 81.616,
		EffectiveImpactMS: 81.616, Confidence: 0.66, LineStart: 955, LineEnd: 20250,
		FamilyMemberCount: 2, FamilyMemberMaxMS: 84.3, FamilyMemberMinMS: 34.8,
		FamilyMemberSumMS: 119.1, FamilyFoldCaliber: "count_sum",
		FamilyMemberRoster: []string{"inode=0x25a01 dev=260:132 计数当量84.300(非墙钟)", "inode=0x14088d dev=260:132 计数当量34.800(非墙钟)"},
	}
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app-42"},
		WindowStartTs: 100.0, WindowEndTs: 100.24,
		OnChainCauses:  []types.TraceCausalProjectionNode{elimChainNode("E-run", "app-42", "runnable_wait", "runnable", 1, 3.0, 100)},
		AdjacentCauses: []types.TraceCausalProjectionNode{count},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	joined := strings.ReplaceAll(fence, "\n", "")
	if !strings.Contains(joined, "有效归因 计数当量81.616(非墙钟) = 计数当量") {
		t.Fatalf("the count family's equation value must be suffix-free:\n%s", fence)
	}
	if strings.Contains(joined, "有效归因 81.616ms") {
		t.Fatalf("the count family's equation must not wear the ms suit:\n%s", fence)
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	for _, row := range rows {
		if len(row.Cells) == 0 || !strings.Contains(row.Cells[0], "页缓存抖动") {
			continue
		}
		cells := strings.Join(row.Cells, " | ")
		if strings.Contains(cells, "81.616ms") {
			t.Fatalf("the count row's table cells must drop the ms suit: %q", cells)
		}
		if !strings.Contains(cells, "计数当量81.616(非墙钟)") {
			t.Fatalf("the count row's table cells must wear the count-equivalent form: %q", cells)
		}
	}
}

// 件⑦ negative arms: a single micro seat keeps its own row (fold-of-one =
// rename), and non-micro cut seats never fold.
func TestRNB5BMicroAnchorFoldNegativeArms(t *testing.T) {
	projection := rnb5bMicroAnchorFoldProjection()
	projection.OnChainCauses = projection.OnChainCauses[:2] // one micro seat only
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "微额锚定席") {
		t.Fatalf("a single micro seat must keep its own row:\n%s", fence)
	}
	if !strings.Contains(fence, "0.026ms") {
		t.Fatalf("the lone micro seat's row must survive:\n%s", fence)
	}
	// Non-micro anchored cut seat (≥0.1ms) never folds.
	projection = rnb5bMicroAnchorFoldProjection()
	for i := range projection.OnChainCauses {
		if projection.OnChainCauses[i].EvidenceID == "E-m1" {
			projection.OnChainCauses[i].ImpactMS = 0.5
			projection.OnChainCauses[i].CumulativeImpactMS = 0.5
			projection.OnChainCauses[i].EffectiveImpactMS = 0.5
			projection.OnChainCauses[i].ChainAnchoredMS = 0.5
		}
	}
	model = buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence = runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "其余 2 项微额锚定席") {
		t.Fatalf("only the sub-threshold seats fold (0.5ms stays):\n%s", fence)
	}
	if !strings.Contains(fence, "0.500ms") {
		t.Fatalf("the ≥0.1ms cut seat keeps its own row:\n%s", fence)
	}
}
