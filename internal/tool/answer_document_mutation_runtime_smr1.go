package tool

// answer_document_mutation_runtime_smr1.go — SMR-1 批 (同线程多行处置,
// 显示/词面层; 权威工单 smr_audit_report_20260712 §②/§④, 2026-07-12).
//
// The passes here extend the EXISTING mirror/tag arm family (twin / mirror /
// 家族席 / G5 / §29.50.1 过渡指针) — never a second merge taxonomy:
//   - WO-A1  runtimeTraceProjMarkNonAdditivePointers — the unified
//     「不可相加/包含」cross-seat pointer (§④: one typed judgment source, one
//     word template, three carriers: self seat / addition identity /
//     aggregate↔member).
//   - WO-D2/D4 runtimeTraceProjFoldBranchTwinAggregates — the trunk/flat
//     same-source ×N aggregate fold (typed full-equality fingerprint; trunk
//     keeps the seat; eff calibers dual-listed).
//   - WO-D3  runtimeTraceProjMarkMergedTwinMirrors — the double-merged twin
//     mutual mirror tags (the shape all three pre-SMR arms structurally
//     missed: 同段孪生键要精确区间相等、P5 只收 root_evidence、家族镜像臂要
//     count+total 恒等).
//   - WO-C1  runtimeTraceProjMarkAccountRelations — the C-type account
//     relation sentence (W-A 双行存续 + 理由句; 三禁令 per S6 vnote).
//   - WO-B1  runtimeTraceProjMarkOccurrenceSeries — the B-type disjoint
//     occurrence interval note + series total.
//
// 全批红线 (工单 §② 总原则): 精确信号才可硬折/硬 tag (µs 值恒等 / typed 段集
// 指纹 / 同源浮点); 重叠启发式只作软引导; W-A 不同账目绝不折 (C 型只改词);
// 零静默消失 (折叠=吸收/佩记号, 数值不重计, 证据照发); 两把尺 (⌗ 异尺行只
// tag 不并 — CausalTokenCaliberSideClass 单源守门).
//
// 过渡臂退役条件 (工单 §④/CASE-4): the v5 P1 engine-side one-seat-per-segment
// mint covers each carrier at the source — when it lands, the WO-A1 pointer,
// the WO-D2 fold and the WO-D3 mutual tags retire with the CR-2 P5 display
// folds (same EVOLUTION contract as
// runtimeTraceProjFoldSameSegmentContextMirrors).

import (
	"fmt"
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjSMR1ValueTieMS is the µs-equality band shared by every SMR-1
// typed value judgment — the ONE exported DIAG A1 constant (值差 < 0.0005ms
// 视同值), never a new tolerance (容差常量禁跨语义借用 does not apply: this IS
// the same semantic — µs-identical published values).
const runtimeTraceProjSMR1ValueTieMS = types.TraceCausalProjectionSameValueTieMS

func runtimeTraceProjSMR1ValuesEqual(a, b float64) bool {
	return a > 0 && b > 0 && math.Abs(a-b) < runtimeTraceProjSMR1ValueTieMS
}

// runtimeTraceProjSMR1StateFamily resolves a node's duration-state semantic
// family (工单 §④ 判定: sleep 族 / d_state·io 族 / runnable 族) from the
// causal-token REGISTRY lanes — 注册表单源, 禁词面比对. Token first (the row
// identity), then the typed dominant-state enum, then the root_evidence
// Predicate token (root-type authority tokens). "" = the node is outside the
// three duration families and never joins an SMR-1 arm.
func runtimeTraceProjSMR1StateFamily(node types.TraceCausalProjectionNode) string {
	if token := runtimeTraceCausalProjectionCanonicalNode(node.TypeToken); token != "" {
		if spec, ok := tracequery.CausalTokenSpecFor(token); ok {
			switch spec.Lane {
			case tracequery.CausalLaneSchedulingDemand, tracequery.CausalLaneWakeupChain, tracequery.CausalLaneIOBlocking:
				return string(spec.Lane)
			}
			return ""
		}
	}
	switch runtimeTraceCausalProjectionCanonicalNode(node.StateKind) {
	case "runnable":
		return string(tracequery.CausalLaneSchedulingDemand)
	case "s_sleep", "sleep", "sleep_wait":
		return string(tracequery.CausalLaneWakeupChain)
	case "d_state", "d_sleep", "uninterruptible_sleep", "io_wait", "iowait":
		return string(tracequery.CausalLaneIOBlocking)
	}
	if token := runtimeTraceCausalProjectionCanonicalNode(node.Predicate); token != "" {
		if spec, ok := tracequery.CausalTokenSpecFor(token); ok {
			switch spec.Lane {
			case tracequery.CausalLaneSchedulingDemand, tracequery.CausalLaneWakeupChain, tracequery.CausalLaneIOBlocking:
				return string(spec.Lane)
			}
		}
	}
	return ""
}

// runtimeTraceProjSMR1WindowsCompatible — the SFD F1 cross-window veto shared
// by every SMR-1 arm: two valid-but-different typed query windows are two
// measurements and never relate; absence never vetoes (and never proves).
func runtimeTraceProjSMR1WindowsCompatible(a, b types.TraceCausalProjectionNode) bool {
	if a.QueryWindowStartTs <= 0 || a.QueryWindowEndTs <= a.QueryWindowStartTs ||
		b.QueryWindowStartTs <= 0 || b.QueryWindowEndTs <= b.QueryWindowStartTs {
		return true
	}
	return math.Abs(a.QueryWindowStartTs-b.QueryWindowStartTs) <= types.TraceCausalProjectionSameWindowToleranceS &&
		math.Abs(a.QueryWindowEndTs-b.QueryWindowEndTs) <= types.TraceCausalProjectionSameWindowToleranceS
}

// runtimeTraceProjSMR1MergedMemberValues derives an ×N merged row's member
// value multiset where it is LOSSLESSLY derivable from the typed extrema/sum
// (count 2 = {min,max}; count 3 with a clean positive Σ = {min, Σ−min−max,
// max}); larger counts expose only the extrema (middle members underdetermined
// — fail-open, 宁漏勿猜). Valueless members (G12) kill the middle derivation.
func runtimeTraceProjSMR1MergedMemberValues(node types.TraceCausalProjectionNode) []float64 {
	if node.MergedCount < 2 || node.MergedMinMS <= 0 || node.MergedMaxMS < node.MergedMinMS {
		return nil
	}
	if node.MergedCount == 2 {
		return []float64{node.MergedMinMS, node.MergedMaxMS}
	}
	if node.MergedCount == 3 && node.MergedValuelessCount == 0 {
		sum := node.MergedSumMS
		if sum <= 0 && !node.MergedIntervalUnion && !node.MergedCrossWindowMax {
			sum = node.ImpactMS
		}
		if sum > 0 {
			middle := sum - node.MergedMinMS - node.MergedMaxMS
			if middle > 0 && middle >= node.MergedMinMS-runtimeTraceProjSMR1ValueTieMS &&
				middle <= node.MergedMaxMS+runtimeTraceProjSMR1ValueTieMS {
				return []float64{node.MergedMinMS, middle, node.MergedMaxMS}
			}
		}
	}
	return []float64{node.MergedMinMS, node.MergedMaxMS}
}

// runtimeTraceProjSMR1LineWithinEnvelope — P2-3 必要性否决 (SMR-1 修复轮,
// 2026-07-13): membership claims (D1① raw fold-in, A1(c) member pointers)
// additionally require the inner row's line interval to sit INSIDE the
// outer row's line envelope. This is a NEGATIVE gate only — envelope
// containment never PROVES membership (行号包络连通判禁令 untouched); it
// rejects impossible memberships that µs value coincidence alone could
// admit. Missing intervals never veto (absence proves nothing either way).
func runtimeTraceProjSMR1LineWithinEnvelope(inner, outer types.TraceCausalProjectionNode) bool {

	if inner.LineStart <= 0 || inner.LineEnd < inner.LineStart ||
		outer.LineStart <= 0 || outer.LineEnd < outer.LineStart {
		return true
	}
	return inner.LineStart >= outer.LineStart && inner.LineEnd <= outer.LineEnd
}

// runtimeTraceProjSMR1WallClockRow — 两把尺 gate: count-equivalent /
// composite-score tokens (⌗ 口径旁栏) never join a wall-clock relation arm.
func runtimeTraceProjSMR1WallClockRow(node types.TraceCausalProjectionNode) bool {
	token := runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)
	if token == "" {
		return true
	}
	return tracequery.CausalTokenCaliberSideClass(token) == tracequery.CausalCaliberSideNone
}

func runtimeTraceProjSMR1AllRows(model *runtimeTraceProjTreeModel) []*runtimeTraceProjTreeRow {
	var all []*runtimeTraceProjTreeRow
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent, model.Background} {
		for i := range rows {
			all = append(all, &rows[i])
		}
	}
	return all
}

func runtimeTraceProjSMR1RelationFree(row *runtimeTraceProjTreeRow) bool {
	return row.NonAdditiveRef == "" && row.ValueMirrorRef == "" &&
		row.FamilyMirrorRef == "" && row.MergedTwinMirrorRef == ""
}

// --- WO-D2/D4: trunk/flat same-source aggregate fold -------------------------

// runtimeTraceProjBranchTwinFingerprint is the typed FULL-equality fingerprint
// of one ×N aggregate published twice (S2-TPF: 56643 E5/E19 — same subject,
// same state, same ×3 member window extrema, same display 17.442, same query
// window; only the eff episode pick differs). Precise signals only — a ⊂
// variant (45903/42729 非全等) never matches and stays for v5 P1 (泛化臂无
// typed inventory 不可证).
func runtimeTraceProjBranchTwinFingerprint(node types.TraceCausalProjectionNode) string {
	subject := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
	if subject == "" || node.MergedCount <= 1 || node.OnChainOverflowFold {
		return ""
	}
	display := runtimeTraceProjNodeDisplayImpact(node)
	if display <= 0 {
		return ""
	}
	if !runtimeTraceProjSMR1WallClockRow(node) {
		return ""
	}
	return subject + "\x00" + strings.TrimSpace(node.StateKind) + "\x00" +
		fmt.Sprintf("%d|%.3f|%.3f|%.3f|%.3f..%.3f", node.MergedCount, display,
			node.MergedMinMS, node.MergedMaxMS, node.QueryWindowStartTs, node.QueryWindowEndTs)
}

// runtimeTraceProjFoldBranchTwinAggregates (WO-D2/D4, SMR-1 批, 2026-07-12;
// witness 56643 E5(+2)/E19(+2): ONE physical ×3 D-state aggregate published as
// the resolved trunk hop AND a flat 父节点未确认 copy — two 15% bars, two
// non-empty effs into the ranking pool, 直接违 §29.50.4 合计参赛裁定): inside
// the chain universe, exactly TWO nodes with the full-equality fingerprint
// where exactly ONE passes the trunk domain admit (branch+window — the same
// gate the depth attach consumes) fold: the trunk copy keeps the seat (已解析
// 链位信息严格更多), the flat copy rides the peer carrier (E# into the
// bracket; its diverging eff caliber dual-lists verbatim — D4). P0-E's
// (branch,window) domain gate itself is untouched — this fold runs AFTER it
// would admit both publications and de-duplicates the same-source pair only.
// Ambiguity (≥3 copies, both/neither admitting) fails open. 退役条件: v5 P1
// engine one-seat mint (跨 path 家族扩张去重 at the source).
func runtimeTraceProjFoldBranchTwinAggregates(nodes []types.TraceCausalProjectionNode,
	electedBranch int, trunkWindowed bool, trunkWS, trunkWE float64) ([]types.TraceCausalProjectionNode, map[string][]types.TraceCausalProjectionNode) {
	groups := map[string][]int{}
	var order []string
	for i, node := range nodes {
		key := runtimeTraceProjBranchTwinFingerprint(node)
		if key == "" {
			continue
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}
	folded := map[int]bool{}
	peers := map[string][]types.TraceCausalProjectionNode{}
	for _, key := range order {
		members := groups[key]
		if len(members) != 2 {
			continue // exactly the twin pair; ≥3 copies fail open
		}
		a, b := members[0], members[1]
		if runtimeTraceCausalProjectionCanonicalNode(nodes[a].EvidenceID) != "" &&
			runtimeTraceCausalProjectionCanonicalNode(nodes[a].EvidenceID) ==
				runtimeTraceCausalProjectionCanonicalNode(nodes[b].EvidenceID) {
			continue // one observation's own copy — renderers dedupe by key
		}
		admitA := runtimeTraceProjTrunkDomainAdmit(nodes[a], electedBranch, trunkWindowed, trunkWS, trunkWE)
		admitB := runtimeTraceProjTrunkDomainAdmit(nodes[b], electedBranch, trunkWindowed, trunkWS, trunkWE)
		if admitA == admitB {
			continue // both/neither trunk-admissible — no typed keeper, fail open
		}
		keeper, flat := a, b
		if admitB {
			keeper, flat = b, a
		}
		folded[flat] = true
		peers[runtimeTraceCausalProjectionNodeKey(nodes[keeper])] = append(
			peers[runtimeTraceCausalProjectionNodeKey(nodes[keeper])], nodes[flat])
	}
	if len(folded) == 0 {
		return nodes, nil
	}
	out := make([]types.TraceCausalProjectionNode, 0, len(nodes)-len(folded))
	for i, node := range nodes {
		if folded[i] {
			continue
		}
		out = append(out, node)
	}
	return out, peers
}

// --- WO-D3 短期臂: double-merged twin mutual mirror tags ---------------------

// runtimeTraceProjMarkMergedTwinMirrors (WO-D3, SMR-1 批 S3-TPF/S8-TPF,
// 2026-07-12; witnesses 42729 E9/E15 · 62930 E9/E19: 13.418 ×3(4.265–4.884)
// published twice with word faces forked purely by typed-token completeness):
// exactly TWO rendered ×N merged rows with the identical member fingerprint
// (subject + µs display + count + member extrema + query window) are one
// physical segment set on two lanes — both wear the mutual 同段镜像 tag
// (tag-only, C-2/A1 ValueMirror precedent; accounts untouched). Works across
// ALL four render lanes (S8-TPF 链/▒ 通道漂移 covered); ⌗ caliber-side rows
// never join (两把尺); ≥3 matches fail open. 明令禁止 typed 家族键合并 (vnote
// 实证: ×N is a SUM — a family-key merge would double-book into ×6). The V4
// exact-lane token-absence relaxation (types layer) is the root arm — when it
// folds the pair pre-merge, this display tag has nothing to do (若双行则必互指).
func runtimeTraceProjMarkMergedTwinMirrors(model *runtimeTraceProjTreeModel) {
	fingerprint := func(node types.TraceCausalProjectionNode) string {
		subject := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		if subject == "" || node.MergedCount <= 1 || node.OnChainOverflowFold {
			return ""
		}
		display := runtimeTraceProjNodeDisplayImpact(node)
		if display <= 0 || !runtimeTraceProjSMR1WallClockRow(node) {
			return ""
		}
		return subject + "\x00" + fmt.Sprintf("%d|%.3f|%.3f|%.3f|%.3f..%.3f",
			node.MergedCount, display, node.MergedMinMS, node.MergedMaxMS,
			node.QueryWindowStartTs, node.QueryWindowEndTs)
	}
	groups := map[string][]*runtimeTraceProjTreeRow{}
	for _, row := range runtimeTraceProjSMR1AllRows(model) {
		if !row.HasData {
			continue
		}
		if key := fingerprint(row.Node); key != "" {
			groups[key] = append(groups[key], row)
		}
	}
	for _, rows := range groups {
		if len(rows) != 2 {
			continue // ambiguity fails open
		}
		a, b := rows[0], rows[1]
		if runtimeTraceCausalProjectionCanonicalNode(a.Node.EvidenceID) ==
			runtimeTraceCausalProjectionCanonicalNode(b.Node.EvidenceID) {
			continue // one node's cross-bucket copy, not a twin publication
		}
		if a.FamilyMirrorRef != "" || b.FamilyMirrorRef != "" {
			continue // the P5 family arm already relates one side
		}
		if strings.TrimSpace(a.EvidenceTag) == "" || strings.TrimSpace(b.EvidenceTag) == "" {
			continue
		}
		a.MergedTwinMirrorRef = strings.TrimSpace(b.EvidenceTag)
		b.MergedTwinMirrorRef = strings.TrimSpace(a.EvidenceTag)
	}
}

// --- WO-A1: unified non-additive pointer arm ---------------------------------

// runtimeTraceProjMarkNonAdditivePointers (WO-A1, SMR-1 批, 工单 §④,
// 2026-07-12) — the §29.50.1 过渡候选反向指针 generalized. One typed judgment
// source per carrier, one word template (runtimeTraceProjSameSegMirrorTagTexts),
// mounting default = 从属/子集席 points at the full seat (the additive reading
// happens on seeing the SECOND row), symmetric「已含」on the full seat where
// the pair is unambiguous. 全等形→twin/mirror 臂 (本臂不收); 不同账目→W-A +
// WO-C1; 不相交→WO-B1 — 四臂词面家族统一但判定互斥.
//
// Carriers:
//
//	(a) 自身席 (SMR-S1 残余 41006/45701 E3): a self binder_wait row is carved
//	    from the target's OWN sleep wall clock by construction (binder
//	    attribution P9) — typed token + unique sleep seat + value sanity ≤.
//	    自身席载体去掉「从属披露在场」条件 (工单护栏: self rows carry none).
//	(b) 加法恒等式 (SMR-S5 31693 E4/E11): a d/io rank seat whose typed split
//	    proves display = d_state + io_wait beside a same-thread row µs-equal
//	    to one side of the split — the side row is the typed COMPONENT.
//	(c) 聚合↔成员 (S5-TPF 42729 E17/E4; SMR-S3 游离位 56643 E17/E18): a
//	    single-occurrence row µs-identical to a derivable MEMBER value of a
//	    same-thread same-family ×N aggregate — 成员盘存隶属 by µs identity,
//	    禁行号包络连通判 (SMR-S13 E25 反例).
func runtimeTraceProjMarkNonAdditivePointers(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)

	// Carrier (a): self binder_wait ⊂ self sleep seat.
	var sleepSeats []*runtimeTraceProjTreeRow
	for i := range model.SelfRows {
		row := &model.SelfRows[i]
		if !row.HasData || !row.Node.IsSleepState() || runtimeTraceProjNodeDisplayImpact(row.Node) <= 0 {
			continue
		}
		// 5200 复放复核 (2026-07-12): a cadence-idle row rides the s_sleep
		// StateKind but is 语义 alien to the 等依赖 sleep family (CAL-1 件⑤)
		// — it is not the sleep SEAT and must not turn the census ambiguous.
		if runtimeTraceProjIdleRowKind(row.Node) != "" {
			continue
		}
		sleepSeats = append(sleepSeats, row)
	}
	if len(sleepSeats) == 1 && strings.TrimSpace(sleepSeats[0].EvidenceTag) != "" {
		seat := sleepSeats[0]
		seatDisplay := runtimeTraceProjNodeDisplayImpact(seat.Node)
		for i := range model.SelfRows {
			row := &model.SelfRows[i]
			if row == seat || !row.HasData || !runtimeTraceProjSMR1RelationFree(row) {
				continue
			}
			// 96879 复放实锤: the root_evidence binder rows carry the typed
			// token on the Predicate lane (no type= note) — both typed lanes
			// are the identity, never the Object word.
			if runtimeTraceCausalProjectionCanonicalNode(row.Node.TypeToken) != "binder_wait" &&
				runtimeTraceCausalProjectionCanonicalNode(row.Node.Predicate) != "binder_wait" {
				continue
			}
			display := runtimeTraceProjNodeDisplayImpact(row.Node)
			if display <= 0 || display > seatDisplay+runtimeTraceProjSMR1ValueTieMS {
				continue // a binder account exceeding the sleep account is a different window shape
			}
			if !runtimeTraceProjSMR1WindowsCompatible(row.Node, seat.Node) {
				continue
			}
			row.NonAdditiveRef = strings.TrimSpace(seat.EvidenceTag)
			row.NonAdditiveKind = runtimeTraceProjNonAdditiveComponent
		}
	}

	// Carrier (b): addition-identity component (typed d/io split).
	for _, side := range all {
		if !side.HasData || !runtimeTraceProjSMR1RelationFree(side) {
			continue
		}
		sideNode := side.Node
		if runtimeTraceProjSMR1StateFamily(sideNode) != string(tracequery.CausalLaneIOBlocking) {
			continue
		}
		sideDisplay := runtimeTraceProjNodeDisplayImpact(sideNode)
		if sideDisplay <= 0 {
			continue
		}
		var sum *runtimeTraceProjTreeRow
		ambiguous := false
		for _, x := range all {
			if x == side || !x.HasData {
				continue
			}
			n := x.Node
			if runtimeTraceCausalProjectionCanonicalNode(n.Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(sideNode.Subject) {
				continue
			}
			if runtimeTraceProjSMR1StateFamily(n) != string(tracequery.CausalLaneIOBlocking) {
				continue
			}
			if n.DStateSplitMS <= 0 || n.IOWaitSplitMS <= 0 {
				continue // no typed complement = no identity proof
			}
			display := runtimeTraceProjNodeDisplayImpact(n)
			if display <= 0 || math.Abs(display-(n.DStateSplitMS+n.IOWaitSplitMS)) >= runtimeTraceProjSMR1ValueTieMS {
				continue // the row's own split must reproduce its display exactly
			}
			if !runtimeTraceProjSMR1ValuesEqual(sideDisplay, n.DStateSplitMS) &&
				!runtimeTraceProjSMR1ValuesEqual(sideDisplay, n.IOWaitSplitMS) {
				continue
			}
			if !runtimeTraceProjSMR1WindowsCompatible(sideNode, n) {
				continue
			}
			if sum != nil {
				ambiguous = true
				break
			}
			sum = x
		}
		if sum == nil || ambiguous || strings.TrimSpace(sum.EvidenceTag) == "" {
			continue
		}
		side.NonAdditiveRef = strings.TrimSpace(sum.EvidenceTag)
		side.NonAdditiveKind = runtimeTraceProjNonAdditiveComponent
		if runtimeTraceProjSMR1RelationFree(sum) && strings.TrimSpace(side.EvidenceTag) != "" {
			// 对称短 tag (§④ 可选面): the full seat discloses containment too.
			sum.NonAdditiveRef = strings.TrimSpace(side.EvidenceTag)
			sum.NonAdditiveKind = runtimeTraceProjNonAdditiveContains
		}
	}

	// Carrier (c): aggregate↔member (µs member identity).
	type aggregateRef struct {
		row     *runtimeTraceProjTreeRow
		family  string
		members []float64
	}
	var aggregates []aggregateRef
	for _, row := range all {
		n := row.Node
		if !row.HasData || n.MergedCount < 2 || n.OnChainOverflowFold {
			continue
		}
		if !runtimeTraceProjSMR1WallClockRow(n) {
			continue
		}
		family := runtimeTraceProjSMR1StateFamily(n)
		if family == "" || strings.TrimSpace(row.EvidenceTag) == "" {
			continue
		}
		members := runtimeTraceProjSMR1MergedMemberValues(n)
		if len(members) == 0 {
			continue
		}
		aggregates = append(aggregates, aggregateRef{row: row, family: family, members: members})
	}
	if len(aggregates) == 0 {
		return
	}
	// Carrier (d) — 冷读扩臂② (SMR-1 修复轮, 2026-07-13; 42729/96717 E9↔E14
	// 形: the ×3 critical twin's members are three of the ×4 family's four —
	// two µs-precise identities prove the containment WITHOUT a member
	// inventory: familyDisplay − familyMin == mergedDisplay AND mergedMax ==
	// familyMax): the merged row points 组成部分→family, the family wears the
	// symmetric 已含 face. Narrow by construction — the general ⊂ shape stays
	// CASE-1/v5 P1 territory (this arm fires only on the exact N−1 全等形).
	//
	// PRODUCTION REACHABILITY (29424 复放核, 2026-07-13): today's engine
	// publishes member_min/max as SEGMENT-level extrema (0.038/4.269 on the
	// tieba witness) while the twin's merged members are per-CPU GROUP sums
	// (4.265/4.269/4.884) — the group inventory lives only in roster STRINGS
	// (禁词面比对), so the identities cannot fire on the live shape yet. The
	// arm is the display consumer-in-waiting of CASE-1 工程缺口(b)
	// (rootCauseDIOStateFamilyItems 填 familyMemberIntervals/组值 typed 库存);
	// until that lands the pair stays honestly unpointed (宁漏勿假).
	for _, m := range all {
		mn := m.Node
		if !m.HasData || mn.MergedCount < 2 || mn.OnChainOverflowFold ||
			!runtimeTraceProjSMR1RelationFree(m) || !runtimeTraceProjSMR1WallClockRow(mn) {
			continue
		}
		mDisplay := runtimeTraceProjNodeDisplayImpact(mn)
		if mDisplay <= 0 || mn.MergedMaxMS <= 0 {
			continue
		}
		family := runtimeTraceProjSMR1StateFamily(mn)
		if family != string(tracequery.CausalLaneIOBlocking) {
			continue
		}
		var host *runtimeTraceProjTreeRow
		ambiguous := false
		for _, f := range all {
			fn := f.Node
			if f == m || !f.HasData || fn.FamilyMemberCount <= 1 {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(fn.Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(mn.Subject) {
				continue
			}
			if runtimeTraceProjSMR1StateFamily(fn) != family {
				continue
			}
			fDisplay := runtimeTraceProjNodeDisplayImpact(fn)
			if fDisplay <= 0 || fn.FamilyMemberMinMS <= 0 || fn.FamilyMemberMaxMS <= 0 {
				continue
			}
			if fn.FamilyMemberCount != mn.MergedCount+1 {
				continue
			}
			if !runtimeTraceProjSMR1ValuesEqual(fDisplay-fn.FamilyMemberMinMS, mDisplay) ||
				!runtimeTraceProjSMR1ValuesEqual(mn.MergedMaxMS, fn.FamilyMemberMaxMS) {
				continue
			}
			if !runtimeTraceProjSMR1WindowsCompatible(mn, fn) {
				continue
			}
			if host != nil {
				ambiguous = true
				break
			}
			host = f
		}
		if host == nil || ambiguous || strings.TrimSpace(host.EvidenceTag) == "" {
			continue
		}
		m.NonAdditiveRef = strings.TrimSpace(host.EvidenceTag)
		m.NonAdditiveKind = runtimeTraceProjNonAdditiveComponent
		if runtimeTraceProjSMR1RelationFree(host) && host.AccountRelRef == "" &&
			strings.TrimSpace(m.EvidenceTag) != "" {
			host.NonAdditiveRef = strings.TrimSpace(m.EvidenceTag)
			host.NonAdditiveKind = runtimeTraceProjNonAdditiveContains
		}
	}
	for _, row := range all {
		n := row.Node
		if !row.HasData || n.MergedCount > 1 || n.OnChainOverflowFold ||
			!runtimeTraceProjSMR1RelationFree(row) || !runtimeTraceProjSMR1WallClockRow(n) {
			continue
		}
		display := runtimeTraceProjNodeDisplayImpact(n)
		if display <= 0 {
			continue
		}
		family := runtimeTraceProjSMR1StateFamily(n)
		if family == "" {
			continue
		}
		var match *aggregateRef
		ambiguous := false
		for i := range aggregates {
			agg := &aggregates[i]
			if agg.row == row || agg.family != family {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(agg.row.Node.Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(n.Subject) {
				continue
			}
			if runtimeTraceProjSMR1ValuesEqual(display, runtimeTraceProjNodeDisplayImpact(agg.row.Node)) {
				continue // 全等形 → twin/mirror 臂 (本臂不收)
			}
			if !runtimeTraceProjSMR1WindowsCompatible(n, agg.row.Node) {
				continue
			}
			if !runtimeTraceProjSMR1LineWithinEnvelope(n, agg.row.Node) {
				continue // P2-3 必要性否决: outside the aggregate's envelope
			}
			hit := false
			for _, member := range agg.members {
				if runtimeTraceProjSMR1ValuesEqual(display, member) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
			if match != nil {
				ambiguous = true
				break
			}
			match = agg
		}
		if match == nil || ambiguous {
			continue
		}
		// 冷读扩臂⑥ (2026-07-13, 单向即充分论证): the member pointer stays
		// ONE-WAY (member→aggregate). The additive misread happens on seeing
		// the SECOND row — every member row says so; the aggregate row's own
		// ×N(a~b) form already declares it is a merge of members, and K
		// back-references on wide families would flood 行2 (width discipline)
		// while adding no anti-sum information the ×N form lacks.
		row.NonAdditiveRef = strings.TrimSpace(match.row.EvidenceTag)
		row.NonAdditiveKind = runtimeTraceProjNonAdditiveMember
	}
}

// --- WO-C1: C-type account-relation sentences --------------------------------

// runtimeTraceProjSMR1AccountCaliber names a row's own accounting method for
// the WO-C1 sentence — METHOD self-identification only (三禁令: the words must
// not form a coverage-direction pair such as 全窗 vs 发生段, must never say
// 同段, and never quantify the overlap).
func runtimeTraceProjSMR1AccountCaliber(row *runtimeTraceProjTreeRow, zh bool) string {
	node := row.Node
	switch {
	case node.FamilyMemberCount > 1:
		// 96717 复放复核 (2026-07-12): the 状态类互斥 self-description is true
		// ONLY for the scheduler-state window families; a non-state family
		// seat (io_latency block-layer family) speaks the neutral family
		// word — typed token fork, never a word-face guess.
		switch runtimeTraceCausalProjectionCanonicalNode(node.TypeToken) {
		case "d_state_or_io_wait", "io_wait", "sleep_wait", "runnable_wait",
			"priority_inversion_runnable_wait", "fragmented_d_state_or_io_wait",
			"fragmented_sleep_wait", "fragmented_runnable_wait":
			if zh {
				return "按状态类互斥归账(状态族合计)"
			}
			return "mutually-exclusive per-state accounting (state-family total)"
		}
		if zh {
			return "按同类家族合计归账(家族席)"
		}
		return "same-kind family-total accounting (family seat)"
	case node.Rank > 0:
		if zh {
			// UXG1 F2: the channel word rides the tracefence constant, never a
			// hand-copied literal.
			return "按链上聚合归账(" + tracefence.SeatChannelChainZH + "席)"
		}
		return "chain-aggregation accounting (rank seat)"
	default:
		if zh {
			return "按本行发生段归账"
		}
		return "own-occurrence accounting"
	}
}

// runtimeTraceProjMarkAccountRelations (WO-C1, SMR-1 批 SMR-S6/SMR-S15 +
// S1-TPF 过渡, 2026-07-12): the W-A ruling's missing reason half. Two pair
// classes, both typed:
//
//	(1) same-span scope divergence (SMR-S15 45701 E14/E25, the
//	    TestCR2FixRankChainCumDivergenceStaysTwoRows pair): two rows over the
//	    IDENTICAL evidence span with diverging cumulative accounts — same
//	    physical segments, two scopes. Overlap is structural (same span).
//	(2) family window seat ↔ chain rank seat (SMR-S6/S1-TPF: 15.317 family ×4
//	    vs 17.819 rank io): same thread, same d/io family, two accounting
//	    systems. Overlap is structural: the family seat totals ALL of the
//	    thread's in-window family-state time, and the rank row's positive
//	    window projection is family-state time of the SAME thread in the SAME
//	    window — a nonempty subset of what the family seat covers (no ms is
//	    quantified — 禁量化重叠, the cross-lane ts inventory is v5 P1).
//
// 过渡: the S1-TPF pair keeps this sentence until CASE-3 adjudicates the
// dedicated 双 rank 席 arm.
func runtimeTraceProjMarkAccountRelations(model *runtimeTraceProjTreeModel, zh bool) {
	all := runtimeTraceProjSMR1AllRows(model)

	// Pair class (1): identical span, diverging cumulative scope accounts.
	bySpan := map[string][]*runtimeTraceProjTreeRow{}
	for _, row := range all {
		if !row.HasData || runtimeTraceProjNodeDisplayImpact(row.Node) <= 0 {
			continue
		}
		key := runtimeTraceProjSameSegmentTwinKey(row.Node)
		if key == "" {
			continue
		}
		bySpan[key] = append(bySpan[key], row)
	}
	for _, rows := range bySpan {
		if len(rows) != 2 {
			continue
		}
		a, b := rows[0], rows[1]
		if a.AccountRelRef != "" || b.AccountRelRef != "" {
			continue
		}
		// 两把尺: ⌗ caliber-side rows never enter a wall-clock account pair
		// (96717 复放实锤: a composite-score row grew a nonsense sentence).
		if !runtimeTraceProjSMR1WallClockRow(a.Node) || !runtimeTraceProjSMR1WallClockRow(b.Node) {
			continue
		}
		// 96717 复放复核: FAMILY seats route through pair class (2) only —
		// their same-span critical twin (13.418 vs 15.317, the SMR-S7/S4-TPF
		// subset pair) is CASE-1's adjudication territory (⊂ 泛化臂无 typed
		// inventory, 禁显示层绕道), never a 「两套账目」 sentence.
		if a.Node.FamilyMemberCount > 1 || b.Node.FamilyMemberCount > 1 {
			continue
		}
		if a.Node.CumulativeImpactMS <= 0 || b.Node.CumulativeImpactMS <= 0 ||
			runtimeTraceProjRound3Equal(a.Node.CumulativeImpactMS, b.Node.CumulativeImpactMS) {
			continue // equal accounts are the twin arms' business
		}
		// 冷读扩臂⑤ (SMR-1 修复轮, 2026-07-13; F-8: tieba E6↔E10 both 19.933
		// over the SAME occurrences were labeled 「覆盖集不同」 — 指错比没有
		// 更糟): µs-equal DISPLAYS over one span are the SAME coverage on two
		// scopes — the pair speaks the mirror word (同一物理时间), never the
		// account sentence.
		if runtimeTraceProjSMR1ValuesEqual(runtimeTraceProjNodeDisplayImpact(a.Node), runtimeTraceProjNodeDisplayImpact(b.Node)) {
			if a.ValueMirrorRef == "" && b.ValueMirrorRef == "" {
				a.ValueMirrorRef = strings.TrimSpace(b.EvidenceTag)
				b.ValueMirrorRef = strings.TrimSpace(a.EvidenceTag)
			}
			continue
		}
		if strings.TrimSpace(a.EvidenceTag) == "" || strings.TrimSpace(b.EvidenceTag) == "" {
			continue
		}
		// A1's containment pointer already relates this pair — the account
		// sentence would double-tag the same relation (96717 E4/E13 实锤).
		if a.NonAdditiveRef == strings.TrimSpace(b.EvidenceTag) ||
			b.NonAdditiveRef == strings.TrimSpace(a.EvidenceTag) {
			continue
		}
		ownA, ownB := runtimeTraceProjSMR1AccountCaliber(a, zh), runtimeTraceProjSMR1AccountCaliber(b, zh)
		if ownA == ownB {
			// Identical self-descriptions cannot claim 「两套账目」 — the
			// sentence would contradict itself (fail-open, no tag).
			continue
		}
		a.AccountRelRef, a.AccountRelOwn, a.AccountRelPeer = strings.TrimSpace(b.EvidenceTag), ownA, ownB
		b.AccountRelRef, b.AccountRelOwn, b.AccountRelPeer = strings.TrimSpace(a.EvidenceTag), ownB, ownA
	}

	// Pair class (2): d/io family window seat ↔ chain rank seat.
	for _, family := range all {
		fn := family.Node
		if !family.HasData || fn.FamilyMemberCount <= 1 || family.AccountRelRef != "" {
			continue
		}
		if !runtimeTraceProjSMR1WallClockRow(fn) {
			continue // 两把尺
		}
		if runtimeTraceProjSMR1StateFamily(fn) != string(tracequery.CausalLaneIOBlocking) {
			continue
		}
		familyDisplay := runtimeTraceProjNodeDisplayImpact(fn)
		if familyDisplay <= 0 || strings.TrimSpace(family.EvidenceTag) == "" {
			continue
		}
		var best *runtimeTraceProjTreeRow
		for _, chain := range all {
			cn := chain.Node
			if chain == family || !chain.HasData || cn.Rank <= 0 || cn.FamilyMemberCount > 1 {
				continue
			}
			if !runtimeTraceProjSMR1WallClockRow(cn) {
				continue // 两把尺
			}
			if runtimeTraceCausalProjectionCanonicalNode(cn.Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(fn.Subject) {
				continue
			}
			if runtimeTraceProjSMR1StateFamily(cn) != string(tracequery.CausalLaneIOBlocking) {
				continue
			}
			display := runtimeTraceProjNodeDisplayImpact(cn)
			if display <= 0 || runtimeTraceProjSMR1ValuesEqual(display, familyDisplay) {
				continue // equality → twin arms
			}
			// A1's addition-identity / member relations own their pairs.
			if chain.NonAdditiveRef == strings.TrimSpace(family.EvidenceTag) ||
				family.NonAdditiveRef == strings.TrimSpace(chain.EvidenceTag) {
				continue
			}
			if !runtimeTraceProjSMR1WindowsCompatible(cn, fn) {
				continue
			}
			if best == nil || display > runtimeTraceProjNodeDisplayImpact(best.Node) {
				best = chain
			}
		}
		if best == nil || strings.TrimSpace(best.EvidenceTag) == "" {
			continue
		}
		famCaliber, chainCaliber := runtimeTraceProjSMR1AccountCaliber(family, zh), runtimeTraceProjSMR1AccountCaliber(best, zh)
		family.AccountRelRef, family.AccountRelOwn, family.AccountRelPeer =
			strings.TrimSpace(best.EvidenceTag), famCaliber, chainCaliber
		if best.AccountRelRef == "" {
			best.AccountRelRef, best.AccountRelOwn, best.AccountRelPeer =
				strings.TrimSpace(family.EvidenceTag), chainCaliber, famCaliber
		}
	}
}

// --- WO-B1: B-type occurrence-series notes -----------------------------------

// runtimeTraceProjStampOccurrenceSeries (WO-B1, SMR-1 批 SMR-S8/S10/S11,
// 2026-07-12; witnesses 31552 E5/E10 · 64414 E6/E7 · 45701 E17/E19 — 词面全同
// 仅值异, readers see repetition/contradiction): same-(thread, state, object,
// type) single rows whose occurrence intervals are typed-provably PAIRWISE
// DISJOINT are independent occurrences — each keeps its seat (改词保双席,
// repair_mid 丢席形 is the pinned regression) and gains the interval short
// note + sibling refs + the series total (disjoint same-identity segments are
// the one legitimately additive shape; the total rides the pointer word —
// 禁虚指 E#, no clean total seat exists). Interval preferred over 第N次
// ordinals (S10 vnote: value order vs time order can invert). The
// 父节点未确认 lane is NEVER erased by this pass (诚实席位红线) — the note is
// additive wording only. Overlapping or interval-less members fail open.
func runtimeTraceProjStampOccurrenceSeries(model *runtimeTraceProjTreeModel) {
	groups := map[string][]*runtimeTraceProjTreeRow{}
	var order []string
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows} {
		for i := range rows {
			row := &rows[i]
			n := row.Node
			if !row.HasData || n.MergedCount > 1 || n.OnChainOverflowFold {
				continue
			}
			if runtimeTraceProjNodeDisplayImpact(n) <= 0 ||
				n.StartTs <= 0 || n.EndTs <= n.StartTs {
				continue
			}
			if runtimeTraceProjTraceGapNode(n) || strings.TrimSpace(n.SemanticClass) != "" {
				continue
			}
			if !runtimeTraceProjSMR1RelationFree(row) || row.AccountRelRef != "" {
				continue
			}
			if runtimeTraceProjSMR1StateFamily(n) == "" {
				continue
			}
			// P3 (修复轮 2026-07-13): the series identity carries the typed
			// query-window dimension — two valid-but-different windows are two
			// measurement scopes and never one series (absence keys "" and
			// stays groupable; the ts-disjointness gate below remains the
			// physical proof).
			window := ""
			if n.QueryWindowStartTs > 0 && n.QueryWindowEndTs > n.QueryWindowStartTs {
				window = fmt.Sprintf("%.3f..%.3f", n.QueryWindowStartTs, n.QueryWindowEndTs)
			}
			key := runtimeTraceCausalProjectionCanonicalNode(n.Subject) + "\x00" +
				strings.TrimSpace(n.StateKind) + "\x00" +
				runtimeTraceCausalProjectionCanonicalNode(n.Object) + "\x00" +
				runtimeTraceCausalProjectionCanonicalNode(n.TypeToken) + "\x00" +
				strings.TrimSpace(n.UndrillableReason) + "\x00" + window
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], row)
		}
	}
	for _, key := range order {
		rows := groups[key]
		// 2..4 occurrences: the ≥3 same-kind shape is normally the R2 merge's
		// business already; larger survivor groups fail open (width discipline).
		if len(rows) < 2 || len(rows) > 4 {
			continue
		}
		disjoint := true
		tagged := true
		total := 0.0
		for i := 0; i < len(rows) && disjoint; i++ {
			if strings.TrimSpace(rows[i].EvidenceTag) == "" {
				tagged = false
				break
			}
			total += runtimeTraceProjNodeDisplayImpact(rows[i].Node)
			for j := i + 1; j < len(rows); j++ {
				if runtimeTraceProjTimeSpansOverlap(rows[i].Node, rows[j].Node) {
					disjoint = false
					break
				}
			}
		}
		if !disjoint || !tagged {
			continue // overlap = not the B shape (fail open to bare rows)
		}
		for i := range rows {
			var refs []string
			for j := range rows {
				if j != i {
					refs = append(refs, strings.TrimSpace(rows[j].EvidenceTag))
				}
			}
			rows[i].OccurrenceSeriesRefs = refs
			rows[i].OccurrenceSeriesTotalMS = total
			rows[i].OccurrenceSeriesCount = len(rows)
		}
	}
}

// runtimeTraceProjMarkSeriesAggregateSeats — 冷读扩臂① (SMR-1 修复轮,
// 2026-07-13; F-1: tieba E13 #6 14.597 = E7 7.843 + E8 6.754 µs-exact over
// the same occurrence windows, zero pointers between the three): a rendered
// row whose display µs-equals a same-(subject, state class) occurrence
// SERIES' additive total IS that series' aggregate seat — it wears
// 「不可相加·已含[E7]+[E8]」 and each series row points 「组成部分」 back.
// Runs AFTER the occurrence-series stamp (consumes its typed totals).
func runtimeTraceProjMarkSeriesAggregateSeats(model *runtimeTraceProjTreeModel) {
	type seriesGroup struct {
		subject string
		class   string
		total   float64
		rows    []*runtimeTraceProjTreeRow
	}
	var groups []seriesGroup
	seen := map[string]bool{}
	all := runtimeTraceProjSMR1AllRows(model)
	for _, row := range all {
		if row.OccurrenceSeriesCount < 2 || row.OccurrenceSeriesTotalMS <= 0 || !row.HasData {
			continue
		}
		subject := runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject)
		class := types.TraceCausalProjectionStateClass(row.Node.StateKind)
		key := subject + "\x00" + class + fmt.Sprintf("\x00%.3f", row.OccurrenceSeriesTotalMS)
		if subject == "" {
			continue
		}
		found := false
		for i := range groups {
			if groups[i].subject == subject && groups[i].class == class &&
				runtimeTraceProjSMR1ValuesEqual(groups[i].total, row.OccurrenceSeriesTotalMS) {
				groups[i].rows = append(groups[i].rows, row)
				found = true
				break
			}
		}
		if !found && !seen[key] {
			seen[key] = true
			groups = append(groups, seriesGroup{subject: subject, class: class,
				total: row.OccurrenceSeriesTotalMS, rows: []*runtimeTraceProjTreeRow{row}})
		}
	}
	for gi := range groups {
		group := &groups[gi]
		if len(group.rows) < 2 {
			continue
		}
		var seat *runtimeTraceProjTreeRow
		ambiguous := false
		for _, row := range all {
			n := row.Node
			if !row.HasData || n.OnChainOverflowFold || !runtimeTraceProjSMR1WallClockRow(n) {
				continue
			}
			if row.OccurrenceSeriesCount > 0 {
				continue // a series member is never its own aggregate seat
			}
			if runtimeTraceCausalProjectionCanonicalNode(n.Subject) != group.subject ||
				types.TraceCausalProjectionStateClass(n.StateKind) != group.class {
				continue
			}
			if !runtimeTraceProjSMR1ValuesEqual(runtimeTraceProjNodeDisplayImpact(n), group.total) {
				continue
			}
			if seat != nil {
				ambiguous = true
				break
			}
			seat = row
		}
		if seat == nil || ambiguous || strings.TrimSpace(seat.EvidenceTag) == "" {
			continue
		}
		var refs []string
		for _, row := range group.rows {
			if tag := strings.TrimSpace(row.EvidenceTag); tag != "" {
				refs = append(refs, tag)
			}
		}
		if len(refs) != len(group.rows) {
			continue
		}
		if runtimeTraceProjSMR1RelationFree(seat) && seat.AccountRelRef == "" {
			seat.NonAdditiveRef = strings.Join(refs, "]+[")
			seat.NonAdditiveKind = runtimeTraceProjNonAdditiveContains
		}
		for _, row := range group.rows {
			if runtimeTraceProjSMR1RelationFree(row) {
				row.NonAdditiveRef = strings.TrimSpace(seat.EvidenceTag)
				row.NonAdditiveKind = runtimeTraceProjNonAdditiveComponent
			}
		}
	}
}

// --- WO-D1③ 多引用 tag resolution (display half) -----------------------------

// runtimeTraceProjResolveOverflowMirrorRefs resolves an overflow fold row's
// typed OverflowMirrorEvidenceIDs (31552 E25 shape) into the rendered rows'
// E# tags — the display half of the WO-D1③ headline mirror. A missing
// rendered row (capped/relocated) drops silently from the ref list; an empty
// resolution leaves the row untouched (fail-open, no tag).
func runtimeTraceProjResolveOverflowMirrorRefs(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	var folds []*runtimeTraceProjTreeRow
	for _, row := range all {
		if row.HasData && row.Node.OnChainOverflowFold && len(row.Node.OverflowMirrorEvidenceIDs) > 0 {
			folds = append(folds, row)
		}
	}
	if len(folds) == 0 {
		return
	}
	tagByID := map[string]string{}
	for _, row := range all {
		if !row.HasData || strings.TrimSpace(row.EvidenceTag) == "" {
			continue
		}
		if id := runtimeTraceCausalProjectionCanonicalNode(row.Node.EvidenceID); id != "" {
			tagByID[id] = strings.TrimSpace(row.EvidenceTag)
		}
	}
	for _, fold := range folds {
		var refs []string
		for _, id := range fold.Node.OverflowMirrorEvidenceIDs {
			if tag := tagByID[runtimeTraceCausalProjectionCanonicalNode(id)]; tag != "" {
				refs = append(refs, tag)
			}
		}
		fold.OverflowMirrorRefs = refs
	}
}

// runtimeTraceProjResolveOverflowProjectionRefs (P2-2 display half,
// 2026-07-13): resolve the typed cross-caliber projection host id to its
// rendered E# — same fail-open discipline as the multi-ref resolver above.
func runtimeTraceProjResolveOverflowProjectionRefs(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	tagByID := map[string]string{}
	for _, row := range all {
		if !row.HasData || strings.TrimSpace(row.EvidenceTag) == "" {
			continue
		}
		if id := runtimeTraceCausalProjectionCanonicalNode(row.Node.EvidenceID); id != "" {
			tagByID[id] = strings.TrimSpace(row.EvidenceTag)
		}
	}
	for _, row := range all {
		if !row.HasData || !row.Node.OnChainOverflowFold {
			continue
		}
		if id := runtimeTraceCausalProjectionCanonicalNode(row.Node.OverflowProjectionEvidenceID); id != "" {
			row.OverflowProjectionRef = tagByID[id]
		}
	}
	// P2-2 谦逊注 fallback (2026-07-13): a ref-less wire-fold pool whose
	// subject also holds rendered same-family seats gets the ADVISORY humble
	// note — the Σ identity is typed-unreachable on the wire-fold shape
	// (folded_* notes carry min/max only), so no E# is named (宁弱勿假指).
	for _, row := range all {
		n := row.Node
		if !row.HasData || !n.OnChainOverflowFold || n.MergedCount < 2 ||
			row.OverflowProjectionRef != "" || len(row.OverflowMirrorRefs) > 0 {
			continue
		}
		subject := ""
		if len(n.MergedSubjects) > 0 {
			subject = runtimeTraceCausalProjectionCanonicalNode(n.MergedSubjects[0])
		}
		if subject == "" {
			continue
		}
		family := runtimeTraceProjSMR1StateFamily(n)
		if family == "" {
			continue
		}
		for _, other := range all {
			if other == row || !other.HasData || other.Node.OnChainOverflowFold {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(other.Node.Subject) != subject {
				continue
			}
			if runtimeTraceProjSMR1StateFamily(other.Node) != family {
				continue
			}
			if runtimeTraceProjNodeDisplayImpact(other.Node) <= 0 {
				continue
			}
			row.OverflowProjectionHumble = true
			break
		}
	}
}

// runtimeTraceProjStampOverflowSeriesMirrors (WO-D1③ 第二臂, 96879 复放追修,
// 2026-07-12): a pool fold row WITHOUT typed mirror ids whose headline (取最
// 大) value µs-equals a rendered occurrence SERIES' additive total (WO-B1
// typed proof: pairwise-disjoint same-identity rows, total = Σ) re-publishes
// that series' combined physical time (96879 E29: headline 20.816 = E5
// 15.565 + E11 5.251). The series rows' E# ride the multi-ref mirror tag;
// the fold row and its count stay honest. Precise gates only: µs total
// equality + the series subject present in the fold's subject roster.
func runtimeTraceProjStampOverflowSeriesMirrors(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	type seriesRef struct {
		subject string
		total   float64
		refs    []string
	}
	var series []seriesRef
	seen := map[string]bool{}
	for _, row := range all {
		if row.OccurrenceSeriesCount < 2 || row.OccurrenceSeriesTotalMS <= 0 || !row.HasData {
			continue
		}
		subject := runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject)
		key := subject + fmt.Sprintf("\x00%.3f", row.OccurrenceSeriesTotalMS)
		if subject == "" || seen[key] {
			continue
		}
		seen[key] = true
		refs := append([]string{strings.TrimSpace(row.EvidenceTag)}, row.OccurrenceSeriesRefs...)
		series = append(series, seriesRef{subject: subject, total: row.OccurrenceSeriesTotalMS, refs: refs})
	}
	if len(series) == 0 {
		return
	}
	for _, row := range all {
		if !row.HasData || !row.Node.OnChainOverflowFold || len(row.OverflowMirrorRefs) > 0 {
			continue
		}
		display := runtimeTraceProjNodeDisplayImpact(row.Node)
		if display <= 0 {
			continue
		}
		roster := map[string]bool{}
		for _, subject := range row.Node.MergedSubjects {
			roster[runtimeTraceCausalProjectionCanonicalNode(subject)] = true
		}
		var match *seriesRef
		ambiguous := false
		for i := range series {
			if math.Abs(series[i].total-display) >= runtimeTraceProjSMR1ValueTieMS {
				continue
			}
			if len(roster) > 0 && !roster[series[i].subject] {
				continue
			}
			if match != nil {
				ambiguous = true
				break
			}
			match = &series[i]
		}
		if match == nil || ambiguous {
			continue
		}
		row.OverflowMirrorRefs = append([]string(nil), match.refs...)
	}
}
