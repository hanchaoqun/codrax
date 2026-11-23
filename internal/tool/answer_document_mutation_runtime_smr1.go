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
// 过渡臂退役条件 (工单 §④/CASE-4) — v5 P1 件① 落地后的诚实盘点 (2026-07-13,
// engine arms = types/trace_causal_projection_oneseat.go):
//   - WO-D3 mutual tags: the WITNESSED carriers (42729 E9/E15 · 62930 E9/E19
//     token-completeness forks; S8-TPF same-token cross-stanza pairs) now
//     converge at engine arm C and never reach this layer. The arm STAYS for
//     the pairs the engine deliberately fails open on (diverging cum/eff
//     accounts = W-A; ranked seats; ⌗ caliber-side tokens; ≥3 copies) —
//     unretired because engine coverage of those would break the W-A red
//     line, not because the mint is missing. 禁半形退役 honored: the arm is
//     intact, its carrier set narrowed.
//   - WO-D2 fold: its witnessed pair (56643 E5/E19) diverges on the eff
//     episode pick — engine arm C's W-A eff veto fails open BY DESIGN, so
//     the D4 dual-list disclosure keeps living here. 退役条件 = a typed
//     dual-eff wire carrier (future adjudication).
//   - WO-A1 pointers / WO-C1 sentences: their carriers are NESTED-account
//     shapes (component ⊂ seat / member ⊂ aggregate / two account systems),
//     not duplicate seats of one segment set — outside B.2's equal-
//     fingerprint scope by definition; WO-C1's own 退役条件 remains CASE-3.

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

// runtimeTraceProjRSPAOwnedStateAccount resolves the state family and scalar
// that a row owns specifically for an RSPA full-window/chain-anchor account.
// Most rows own their ordinary display scalar in their registered state
// family.  A priority-inversion scheduling-supply sub-seat is different: its
// dominant state remains the surrounding dependency's evidence context and
// its display value may also include a running compute-deficit component, but
// only the typed gated-runnable component belongs to the runnable wall-clock
// bipartition.  Keeping this exception in one typed helper prevents the
// coverage, same-source pairing and twin-visibility consumers from drifting.
func runtimeTraceProjRSPAOwnedStateAccount(node types.TraceCausalProjectionNode) (string, float64, bool) {
	token := firstNonEmptyAnswerString(node.TypeToken, node.Object)
	if runtimeTracePriorityInversionCandidateType(runtimeTraceCausalProjectionCanonicalNode(token)) &&
		node.GatedRunnableMS > 0 {
		return string(tracequery.CausalLaneSchedulingDemand), node.GatedRunnableMS, true
	}
	family := runtimeTraceProjSMR1StateFamily(node)
	value := runtimeTraceProjNodeDisplayImpact(node)
	return family, value, family != "" && value > 0
}

// Same-line account annotations describe an entire displayed state measure,
// not a sub-component. A PIC row can include both runnable time and a running
// compute deficit; its dependency's dominant state is only context. Reuse
// the typed family registry, but do not call a composite/unknown PIC scalar
// a single scheduler-state account. This changes annotation only.
func runtimeTraceProjSMR1WholeStateFamily(node types.TraceCausalProjectionNode) string {
	token := firstNonEmptyAnswerString(node.TypeToken, node.Object)
	if runtimeTracePriorityInversionCandidateType(runtimeTraceCausalProjectionCanonicalNode(token)) &&
		(node.GatedRunnableMS <= 0 || node.GatedRunningDeficitMS > 0 ||
			!runtimeTraceProjSMR1ValuesEqual(node.GatedRunnableMS, runtimeTraceProjNodeDisplayImpact(node))) {
		return ""
	}
	return runtimeTraceProjSMR1StateFamily(node)
}

// runtimeTraceProjSMR1HullsDisjointProven (修复轮三 R2-F2) reports whether
// two rows' typed occurrence hulls are PROVABLY disjoint: both carry valid
// [StartTs,EndTs] and the intervals do not intersect. Hull disjointness ⇒
// member disjointness (precise); anything else (overlap, missing ts) is
// unprovable and returns false (the display must keep the relation unknown).
func runtimeTraceProjSMR1HullsDisjointProven(a, b types.TraceCausalProjectionNode) bool {
	if !types.TraceCausalProjectionWindowPresent(a.StartTs, a.EndTs) ||
		!types.TraceCausalProjectionWindowPresent(b.StartTs, b.EndTs) {
		return false
	}
	return a.EndTs <= b.StartTs || b.EndTs <= a.StartTs
}

// runtimeTraceProjSMR1WindowsCompatible — the SFD F1 cross-window veto shared
// by every SMR-1 arm: two valid-but-different typed query windows are two
// measurements and never relate; absence never vetoes (and never proves).
func runtimeTraceProjSMR1WindowsCompatible(a, b types.TraceCausalProjectionNode) bool {
	if !types.TraceCausalProjectionWindowPresent(a.QueryWindowStartTs, a.QueryWindowEndTs) ||
		!types.TraceCausalProjectionWindowPresent(b.QueryWindowStartTs, b.QueryWindowEndTs) {
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
		if sum <= 0 && !node.MergedIntervalUnion && !node.MergedCrossWindowMax && !node.MergedSameSegmentMirror {
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
// exact-lane token-absence relaxation (types layer, pre-R2) and the v5 P1
// 件① engine arm C (post-R2, different-envelope twins — the WITNESSED D3
// carriers) are the root arms — when either converges the pair, this display
// tag has nothing to do (若双行则必互指). Remaining carriers here = the
// engine's deliberate W-A fail-open pairs (diverging cum/eff, ranked seats,
// ⌗-side, ≥3 copies).
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
//	    IDENTICAL evidence-line envelope with diverging cumulative accounts.
//	    The shared location does not prove identical physical segments.
//	(2) family window seat ↔ chain rank seat (SMR-S6/S1-TPF: 15.317 family ×4
//	    vs 17.819 rank io): same thread, same d/io family, two accounting
//	    systems. Without a shared complete member/segment inventory, neither
//	    membership nor physical overlap follows from the family or its hull.
//	    Proven hull disjointness survives; all other physical relations stay
//	    unknown. The account cross-pointers never authorize direct addition.
//
// 过渡: the S1-TPF pair keeps this sentence until CASE-3 adjudicates the
// dedicated 双 rank 席 arm.
//
// Pair class (0) — RSPA §29.61.10b 同源二分 (2026-07-14) — runs FIRST: the
// engine bipartition pair is typed-provably SAME-SOURCE (one segment set split
// by chain credentials), so it must claim its rows before the class-(1)
// same-span arm could word the pair 「两套账目·不可相加」 (the one pair where
// that verdict would be FALSE — anchored + remainder == full exactly).
func runtimeTraceProjMarkAccountRelations(model *runtimeTraceProjTreeModel, zh bool) {
	all := runtimeTraceProjSMR1AllRows(model)
	// HMC377 fix round 5: every holder word reads the engine-resolved share
	// claim of its row over the SAME rendered-row universe the pairing arms
	// read — the holder's [E#] when it renders here, the compaction form
	// otherwise, a see-detail form on a node without a typed reference.
	// HMC377 fix round 6: "renders here" is the drawn-record index — every
	// record id the projection draws, wherever it is drawn (a row's own
	// record, a record merged into it, a rank twin folded into a chain row,
	// every other folded peer, an engine-absorbed chain row inside its family
	// row), under the [E#] it is drawn with.
	anchoredRefs := runtimeTraceProjStampShareClaimRefs(model, all)

	// Pair class (0): ◇ remainder half ↔ ⛓ counterpart. HMC377 fix round 5:
	// the counterpart is the ONE seat the remainder's anchored-share claim
	// names, when the engine resolved it, it renders here and it publishes
	// exactly that share (the claim's typed Exact verdict) — never a search
	// of the board by family, window or value.
	for _, rem := range all {
		rn := rem.Node
		if !rem.HasData || !rn.ChainAnchorRemainderSeat || rn.ChainAnchorFullMS <= 0 {
			continue
		}
		// RNB-1 (§29.88 R2, 2026-07-14): a case-A' remainder seat carries the
		// typed ownership-divergence verdict — the 「合计还原全窗账」 pair
		// sentence would be FALSE on it (the chain seat's value plus this
		// remainder does not restore the full account). Its dedicated 行2
		// double-account sentence owns the relation disclosure instead.
		if rn.ChainAnchorOwnershipDivergent {
			continue
		}
		// RNB-2 件1(i) (§29.88 W3 病②, 2026-07-15): anchored==0 ⇒ NO seat holds
		// an anchored share — the pair sentence 「⛓席=凭证锚定段合计…合计还原全
		// 窗账」 has no true referent, and naming one anyway wrote an
		// arithmetically false hard-fact sentence (customer E32↔E9: the
		// 0.000-anchored remainder was paired to a 5.368 chain VIEW row and the
		// sentence claimed 5.368 restores a 9.272 account). 宁漏勿假指 — the 行2
		// bipartition disclosure (件3 zero form) carries the account alone.
		if rn.ChainAnchoredMS <= 0 {
			continue
		}
		// HMC377 round 11 (R-377-11 ②): a remainder whose rest is partly
		// counted on the same thread's cause-specific row no longer restores
		// the account with its ⛓ counterpart alone — its 行2 sentence names
		// all three parts instead (宁漏勿假指).
		if rem.ChainAnchorSubsetMS > 0 {
			continue
		}
		if rem.AccountRelRef != "" || strings.TrimSpace(rem.EvidenceTag) == "" {
			continue
		}
		if !runtimeTraceProjSMR1WallClockRow(rn) {
			continue
		}
		if _, _, familyOK := runtimeTraceProjRSPAOwnedStateAccount(rn); !familyOK {
			continue
		}
		ref := anchoredRefs[rem]
		// HMC377 fix round 6: the counterpart must be the holder's OWN row —
		// a holder drawn inside another row (a rank twin folded into a chain
		// row) is named by its [E#], but that row publishes the host's
		// account, never this share.
		if ref.state != runtimeTraceProjShareRefRendered || !ref.exact || !ref.own || len(ref.rows) != 1 {
			continue
		}
		pick := ref.rows[0]
		if pick == rem || !pick.HasData || strings.TrimSpace(pick.EvidenceTag) == "" || !runtimeTraceProjSMR1WallClockRow(pick.Node) {
			continue
		}
		rem.AccountRelRef = strings.TrimSpace(pick.EvidenceTag)
		rem.AccountRelSameSourceFullMS = rn.ChainAnchorFullMS
		rem.AccountRelSameSourceAnchoredSide = false
		// HMC377 fix round 3: a three-way split's pair names its third seat
		// on both sides (the typed unwalked part and the seat holding it, as
		// its typed claim names it).
		if rn.ChainAnchorUnwalkedMS > 0 {
			rem.AccountRelSameSourceUnwalkedMS = rn.ChainAnchorUnwalkedMS
			rem.AccountRelSameSourceUnwalkedRef = rem.ChainAnchorUnwalkedRef
			rem.AccountRelSameSourceUnwalkedSeatMS = rem.ChainAnchorUnwalkedSeatMS
			rem.AccountRelSameSourceUnwalkedExact = rem.ChainAnchorUnwalkedExact
			rem.AccountRelSameSourceUnwalkedUnreferenced = rem.ChainAnchorUnwalkedUnreferenced
		}
		if pick.AccountRelRef == "" {
			pick.AccountRelRef = strings.TrimSpace(rem.EvidenceTag)
			pick.AccountRelSameSourceFullMS = rn.ChainAnchorFullMS
			pick.AccountRelSameSourceAnchoredSide = true
			pick.AccountRelSameSourceUnwalkedMS = rem.AccountRelSameSourceUnwalkedMS
			pick.AccountRelSameSourceUnwalkedRef = rem.AccountRelSameSourceUnwalkedRef
			pick.AccountRelSameSourceUnwalkedSeatMS = rem.AccountRelSameSourceUnwalkedSeatMS
			pick.AccountRelSameSourceUnwalkedExact = rem.AccountRelSameSourceUnwalkedExact
			pick.AccountRelSameSourceUnwalkedUnreferenced = rem.AccountRelSameSourceUnwalkedUnreferenced
		}
	}

	// Pair class (0b) — 件1 (修复轮, 2026-07-14; donghu E12(+3) witness): the
	// full-window MIRROR row. An UNDECOMPOSED wall-clock row of the SAME
	// thread + state family whose display value equals a rendered bipartition
	// pair's full account at 3 decimals is another lane's face of the SAME
	// full-window account (e.g. the critical_blocking view) — without a
	// relation sentence the board reads three unrelated values (36.757 beside
	// 3.598 + 33.159). Typed value identity only; ≥2 candidate pairs or ≥2
	// mirror candidates fail open (禁猜).
	for _, mirror := range all {
		mn := mirror.Node
		// RNB-2 件2 捎带 (2026-07-15): a merged row whose members' bipartition
		// triples were cleared (MergedChainAnchorMemberAccounts) is a KNOWN Σ
		// of split accounts, never an undecomposed full-window face — without
		// this gate the clearing would newly admit it to mirror candidacy.
		if !mirror.HasData || mn.ChainAnchorFullMS > 0 || mn.MergedChainAnchorMemberAccounts ||
			mirror.AccountRelRef != "" ||
			mirror.AccountRelMirrorAnchoredRef != "" || strings.TrimSpace(mirror.EvidenceTag) == "" {
			continue
		}
		if !runtimeTraceProjSMR1WallClockRow(mn) {
			continue
		}
		family := runtimeTraceProjSMR1StateFamily(mn)
		if family == "" {
			continue
		}
		value := runtimeTraceProjNodeDisplayImpact(mn)
		if value <= 0 {
			continue
		}
		var anchored, remainder *runtimeTraceProjTreeRow
		ambiguous := false
		for _, other := range all {
			on := other.Node
			if other == mirror || !other.HasData || on.ChainAnchorFullMS <= 0 {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(on.Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(mn.Subject) {
				continue
			}
			if runtimeTraceProjSMR1StateFamily(on) != family || !runtimeTraceProjSMR1WallClockRow(on) {
				continue
			}
			if !runtimeTraceProjSMR1WindowsCompatible(on, mn) {
				continue
			}
			if fmt.Sprintf("%.3f", on.ChainAnchorFullMS) != fmt.Sprintf("%.3f", value) {
				continue
			}
			if on.ChainAnchorRemainderSeat {
				if remainder != nil {
					ambiguous = true
					break
				}
				remainder = other
			} else {
				if anchored != nil {
					ambiguous = true
					break
				}
				anchored = other
			}
		}
		if ambiguous || anchored == nil || remainder == nil ||
			strings.TrimSpace(anchored.EvidenceTag) == "" || strings.TrimSpace(remainder.EvidenceTag) == "" {
			continue
		}
		mirror.AccountRelMirrorAnchoredRef = strings.TrimSpace(anchored.EvidenceTag)
		mirror.AccountRelMirrorRemainderRef = strings.TrimSpace(remainder.EvidenceTag)
		mirror.AccountRelSameSourceFullMS = anchored.Node.ChainAnchorFullMS
		if anchored.AccountRelMirrorRef == "" {
			anchored.AccountRelMirrorRef = strings.TrimSpace(mirror.EvidenceTag)
		}
		if remainder.AccountRelMirrorRef == "" {
			remainder.AccountRelMirrorRef = strings.TrimSpace(mirror.EvidenceTag)
		}
	}

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
		if a.AccountRelRef != "" || b.AccountRelRef != "" ||
			a.AccountRelMirrorAnchoredRef != "" || b.AccountRelMirrorAnchoredRef != "" {
			continue
		}
		// 两把尺: ⌗ caliber-side rows never enter a wall-clock account pair
		// (96717 复放实锤: a composite-score row grew a nonsense sentence).
		if !runtimeTraceProjSMR1WallClockRow(a.Node) || !runtimeTraceProjSMR1WallClockRow(b.Node) {
			continue
		}
		// Source line envelopes can contain mutually exclusive states and
		// cover distinct measurements. They cannot override typed differences
		// in state ownership, query windows or disjoint occurrence hulls.
		// Apply before BOTH the equal-value mirror and account-relation arms;
		// keep every row/value/candidate, omitting only the false relation.
		family := runtimeTraceProjSMR1WholeStateFamily(a.Node)
		if family == "" || family != runtimeTraceProjSMR1WholeStateFamily(b.Node) ||
			!runtimeTraceProjSMR1WindowsCompatible(a.Node, b.Node) ||
			runtimeTraceProjSMR1HullsDisjointProven(a.Node, b.Node) {
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
		if !family.HasData || fn.FamilyMemberCount <= 1 || family.AccountRelRef != "" ||
			family.AccountRelMirrorAnchoredRef != "" {
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
		// 修复轮三 R2-F2 (tieba E4↔E25 冷读 witness): typed hull disjointness
		// supplies a one-way proof — hull-disjoint ⇒
		// member-disjoint (precise, e.g. a partition-sibling pair whose
		// fragments never coexist). Hull overlap or missing ts proves neither
		// overlap nor disjointness of the measured members; false is unknown,
		// not a positive overlap credential. No pairing or value changes here.
		disjoint := runtimeTraceProjSMR1HullsDisjointProven(fn, best.Node)
		family.AccountRelRef, family.AccountRelOwn, family.AccountRelPeer =
			strings.TrimSpace(best.EvidenceTag), famCaliber, chainCaliber
		family.AccountRelDisjoint = disjoint
		if best.AccountRelRef == "" {
			best.AccountRelRef, best.AccountRelOwn, best.AccountRelPeer =
				strings.TrimSpace(family.EvidenceTag), chainCaliber, famCaliber
			best.AccountRelDisjoint = disjoint
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
				!types.TraceCausalProjectionWindowPresent(n.StartTs, n.EndTs) {
				continue
			}
			if runtimeTraceProjTraceGapNode(n) || strings.TrimSpace(n.SemanticClass) != "" {
				continue
			}
			if !runtimeTraceProjSMR1RelationFree(row) || row.AccountRelRef != "" ||
				row.AccountRelMirrorAnchoredRef != "" {
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
			if types.TraceCausalProjectionWindowPresent(n.QueryWindowStartTs, n.QueryWindowEndTs) {
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

// runtimeTraceProjShareRef is one part of a row's account as its typed share
// claims name the holder (HMC377 fix round 5). own (HMC377 fix round 6): every
// holder is drawn as its row's own account (the row's record or one merged
// into it), not as a peer folded into another row.
type runtimeTraceProjShareRef struct {
	state  int
	rows   []*runtimeTraceProjTreeRow
	tags   []string
	exact  bool
	own    bool
	seatMS float64
	// successor (HMC377 round 11, R-377-11 ①): some holder is the published
	// successor of a seat the publication gate withdrew — another row of the
	// thread that counts the share, not a seat of the claim's role.
	successor bool
	// oneSuccessor (HMC377 round 12, R-377-12 ②): every claim resolved to a
	// withdrawn seat's successor and one drawn row holds the whole share —
	// the only successor disposition with a specific holder form. Any other
	// share a successor holds part of is a mixed disposition (see-detail).
	oneSuccessor bool
}

const (
	// runtimeTraceProjShareRefNone — the node carries no typed reference for
	// this part (a node compiled without the engine's resolved claims): the
	// sentence names no holder and claims no absence.
	runtimeTraceProjShareRefNone = iota
	// runtimeTraceProjShareRefRendered — the engine resolved the claim and
	// every holder record is drawn on this projection (in another row than
	// the claimant's).
	runtimeTraceProjShareRefRendered
	// runtimeTraceProjShareRefCompacted — the claim did not resolve against
	// the published board, or a holder record is drawn nowhere here.
	runtimeTraceProjShareRefCompacted
	// runtimeTraceProjShareRefPartial (HMC377 round 10, R-377-10 ③) — a
	// represented sentence that resolves only in part: the node's typed
	// ChainAnchorRepresentedHeldMS is counted at the holders of its resolved
	// claims (their [E#] in the refs when every such holder is drawn here,
	// none otherwise) and the row publishes its own account less that share.
	runtimeTraceProjShareRefPartial
)

// runtimeTraceProjDrawnRecord is where one record id is drawn on this
// projection (HMC377 fix round 6): the row it is drawn in, the [E#] under
// which it is drawn there, whether it is that row's own account (the row's
// record or a record merged into it — the row publishes its value) and its
// own display value.
type runtimeTraceProjDrawnRecord struct {
	row       *runtimeTraceProjTreeRow
	tag       string
	own       bool
	displayMS float64
}

// runtimeTraceProjDrawnRecordIndex is THE index of every record id the
// projection draws (HMC377 fix round 6; round 5 indexed the rows' own and
// merged ids only, so a seat drawn inside another row — the NetEase inversion
// candidate folded into its chain row — read as not drawn and its
// represented satellite re-entered the ◎ population as a second owner). Every
// holder lookup and, through the stamped holder state, the ◎ eligibility read
// it. Drawn-id classes:
//   - a row's own record and every record merged into it (self rows, tree
//     rows — trunk chain nodes, cause children, semantic and depthless rows —
//     adjacent and background stanza rows; the four lanes are every row the
//     projection draws), under the row's [E#];
//   - a folded peer drawn inside its host row (rank twin, self-symptom twin,
//     same-segment mirror, branch-twin copy — each in the row's [E#+E#]
//     bracket; a same-thread IO peer in the row's IO note), under the peer's
//     own [E#];
//   - an engine-absorbed chain-lane row, drawn inside its family row (its own
//     [E#] only in the detail stanza), under the family row's [E#].
//
// A row's own records win over a peer copy of the same record; a record is
// drawn only under an [E#] (a tagless carrier draws nothing a sentence could
// point at).
func runtimeTraceProjDrawnRecordIndex(model *runtimeTraceProjTreeModel, all []*runtimeTraceProjTreeRow) map[string]runtimeTraceProjDrawnRecord {
	index := map[string]runtimeTraceProjDrawnRecord{}
	put := func(ids []string, drawn runtimeTraceProjDrawnRecord) {
		if drawn.tag == "" {
			return
		}
		for _, id := range ids {
			if id = strings.TrimSpace(id); id == "" {
				continue
			}
			if _, seen := index[id]; !seen {
				index[id] = drawn
			}
		}
	}
	for _, row := range all {
		if !row.HasData {
			continue
		}
		put(runtimeTraceProjNodeRecordIDs(row.Node), runtimeTraceProjDrawnRecord{row: row, tag: strings.TrimSpace(row.EvidenceTag), own: true,
			displayMS: runtimeTraceProjNodeDisplayImpact(row.Node)})
	}
	peer := func(row *runtimeTraceProjTreeRow, tag string, drawn runtimeTraceProjPeerDrawn) {
		put(drawn.recordIDs, runtimeTraceProjDrawnRecord{row: row, tag: strings.TrimSpace(tag), displayMS: drawn.displayMS})
	}
	for _, row := range all {
		if !row.HasData {
			continue
		}
		for _, p := range row.RankFoldPeers {
			peer(row, p.EvidenceTag, p.drawn)
		}
		for _, p := range row.SelfSymptomFoldPeers {
			peer(row, p.EvidenceTag, p.drawn)
		}
		for _, p := range row.SameSegMirrorPeers {
			peer(row, p.EvidenceTag, p.drawn)
		}
		for _, p := range row.BranchTwinFoldPeers {
			peer(row, p.EvidenceTag, p.drawn)
		}
		for _, p := range row.IOFoldPeers {
			peer(row, p.EvidenceTag, p.drawn)
		}
	}
	if model != nil {
		hosts := runtimeTraceProjAbsorbedChainHosts(model)
		for _, node := range model.absorbedChainRows {
			if host := hosts[strings.TrimSpace(node.AbsorbedInto)]; host != nil && host.HasData {
				peer(host, host.EvidenceTag, runtimeTraceProjPeerDrawnOf(node))
			}
		}
	}
	return index
}

// runtimeTraceProjResolveShareRef reads the claims of the given parts (or the
// represented-sentence claims when represented) — never a board search. A
// holder drawn only inside the claimant's own row is no other seat of this
// board: the part then names no holder and claims no absence (see-detail
// form, HMC377 fix round 6).
func runtimeTraceProjResolveShareRef(node types.TraceCausalProjectionNode, index map[string]runtimeTraceProjDrawnRecord, self *runtimeTraceProjTreeRow, represented bool, parts ...string) runtimeTraceProjShareRef {
	var claims []types.TraceCausalProjectionShareClaim
	for _, c := range node.ShareClaims {
		if represented {
			if c.Represented {
				claims = append(claims, c)
			}
			continue
		}
		if c.Represented {
			continue
		}
		for _, part := range parts {
			if c.Part == part {
				claims = append(claims, c)
			}
		}
	}
	if len(claims) == 0 {
		return runtimeTraceProjShareRef{state: runtimeTraceProjShareRefNone}
	}
	out := runtimeTraceProjShareRef{state: runtimeTraceProjShareRefRendered, own: true}
	type drawnAt struct {
		row *runtimeTraceProjTreeRow
		tag string
	}
	seen := map[drawnAt]bool{}
	selfDrawn, allSuccessor := false, true
	for _, c := range claims {
		if !c.Resolved || len(c.HolderEvidenceIDs) == 0 {
			return runtimeTraceProjShareRef{state: runtimeTraceProjShareRefCompacted}
		}
		out.successor = out.successor || c.Successor
		allSuccessor = allSuccessor && c.Successor
		for _, id := range c.HolderEvidenceIDs {
			drawn, ok := index[strings.TrimSpace(id)]
			if !ok || drawn.row == nil {
				return runtimeTraceProjShareRef{state: runtimeTraceProjShareRefCompacted}
			}
			if drawn.row == self {
				selfDrawn = true
				continue
			}
			out.own = out.own && drawn.own
			if at := (drawnAt{drawn.row, drawn.tag}); !seen[at] {
				seen[at] = true
				out.rows = append(out.rows, drawn.row)
				out.tags = append(out.tags, drawn.tag)
				out.seatMS += drawn.displayMS
			}
		}
	}
	if selfDrawn {
		// HMC377 round 12 (R-377-12 ②): a holder drawn inside the claimant's
		// own row names no other seat — the see-detail form (the round-11
		// carried-here variant, which no reachable shape distinguished from
		// it, is removed).
		return runtimeTraceProjShareRef{state: runtimeTraceProjShareRefNone}
	}
	out.exact = len(claims) == 1 && claims[0].Exact && len(out.rows) == 1
	out.oneSuccessor = allSuccessor && len(out.rows) == 1
	return out
}

// runtimeTraceProjStampSubsetShare (HMC377 round 11, R-377-11 ②) stamps a
// ◇ remainder row with the share its resolved subset claims hand to the same
// thread's cause-specific row (the engine left it out of this row's value),
// and that row's [E#] and type when the drawn-record lookup proves it drawn
// in another row here.
//
// HMC377 round 12 (R-377-12 ③): both numbers are the engine's typed fields —
// the share rows on the published board count (ShareCountedElsewhereMS, the
// remainder's resolved carved claims: its subset claims) and what this row
// counts (ShareCountedHereMS). EVOLUTION RECORD: old → new = the display
// summed the resolved subset claims and printed rest − that sum as 本行计 →
// it prints the engine's two numbers; the per-claim resolved check is gone
// with the sum (the engine counts only resolved claims, so no reachable
// shape told the two apart — review-11 mutant T02 survived every suite).
func runtimeTraceProjStampSubsetShare(row *runtimeTraceProjTreeRow, index map[string]runtimeTraceProjDrawnRecord) {
	row.ChainAnchorSubsetMS, row.ChainAnchorSubsetOwnMS, row.ChainAnchorSubsetRef, row.ChainAnchorSubsetType = 0, 0, "", ""
	if row.Node.ShareCountedElsewhereMS <= 0 {
		return
	}
	row.ChainAnchorSubsetMS = row.Node.ShareCountedElsewhereMS
	row.ChainAnchorSubsetOwnMS = row.Node.ShareCountedHereMS
	ref := runtimeTraceProjResolveShareRef(row.Node, index, row, false, string(tracequery.RootCauseShareClaimSubset))
	if ref.state == runtimeTraceProjShareRefRendered && len(ref.rows) == 1 && ref.rows[0] != nil {
		row.ChainAnchorSubsetRef = strings.Join(ref.tags, ",")
		row.ChainAnchorSubsetType = strings.TrimSpace(ref.rows[0].Node.Object)
	}
}

// runtimeTraceProjRestCarrier is one row holding a ⛓ row's rest (HMC377 round
// 12, R-377-12 ③): its [E#], whether it is the thread's ◇ remainder seat, and
// its type token (the CPU-restriction row's label).
type runtimeTraceProjRestCarrier struct {
	Ref       string
	Remainder bool
	Object    string
}

// runtimeTraceProjStampRestCarriers names every row holding a ⛓ row's rest
// when the engine resolved its remainder claim to the same thread's
// CPU-restriction row besides (or instead of) the thread's ◇ remainder seat
// (the restriction row publishes those instants as its own): each holder by
// [E#] in claim order, only when each one is drawn here (twin, from
// runtimeTraceProjResolveShareRef) as its own row of a named kind — the ◇
// remainder seat or the restriction row. A restriction holder beside one that
// is drawn only inside another row (a folded overflow row) takes the
// see-detail form (restSeeDetail), which names no carrier; a rest no
// restriction row holds keeps the ◇ word's bytes.
func runtimeTraceProjStampRestCarriers(row *runtimeTraceProjTreeRow, twin runtimeTraceProjShareRef) {
	row.ChainAnchorRestCarriers, row.ChainAnchorRestSeeDetail = nil, false
	if twin.state != runtimeTraceProjShareRefRendered || twin.successor || len(twin.rows) == 0 || len(twin.rows) != len(twin.tags) {
		return
	}
	carriers := make([]runtimeTraceProjRestCarrier, 0, len(twin.rows))
	restriction, named := false, true
	for i, holder := range twin.rows {
		if holder == nil || strings.TrimSpace(twin.tags[i]) == "" {
			return
		}
		object := runtimeTraceCausalProjectionCanonicalNode(firstNonEmptyAnswerString(holder.Node.TypeToken, holder.Node.Object))
		isRestriction := object == "cpu_affinity_or_cpuset"
		restriction = restriction || isRestriction
		named = named && (isRestriction || holder.Node.ChainAnchorRemainderSeat)
		carriers = append(carriers, runtimeTraceProjRestCarrier{Ref: strings.TrimSpace(twin.tags[i]),
			Remainder: holder.Node.ChainAnchorRemainderSeat && !isRestriction, Object: object})
	}
	switch {
	case !restriction:
	case named:
		row.ChainAnchorRestCarriers = carriers
	default:
		row.ChainAnchorRestSeeDetail = true
	}
}

// runtimeTraceProjRestrictedPart is one part of a CPU-restriction row's
// restricted ready time that rows on the published board count (HMC377 round
// 12, R-377-12 ③): the resolved carved claim's own share, its holders' [E#]
// ("" → the see-detail form) and whether the chain seat keeps its own account
// for it (case A': the row's chain-account claim beside a walked part).
type runtimeTraceProjRestrictedPart struct {
	MS         float64
	Ref        string
	OwnAccount bool
}

// runtimeTraceProjStampRestrictedParts stamps a CPU-restriction row that left
// part of its restricted ready time to this thread's on-chain seats with the
// parts rows on the published board count: one per resolved claim of the
// part handed over (anchored / unwalked), in claim order, each holder named
// by [E#] only when every one of its records is drawn here.
func runtimeTraceProjStampRestrictedParts(row *runtimeTraceProjTreeRow, index map[string]runtimeTraceProjDrawnRecord) {
	row.RestrictedParts = nil
	rn := row.Node
	if rn.RestrictedReadyAccountMS <= 0 || rn.ShareCountedElsewhereMS <= 0 {
		return
	}
	ownAccount := false
	for _, c := range rn.ShareClaims {
		if c.Part == string(tracequery.RootCauseShareClaimChainAccount) {
			ownAccount = true
		}
	}
	for _, c := range rn.ShareClaims {
		if c.Represented || !c.Resolved || c.ShareMS <= 0 ||
			(c.Part != string(tracequery.RootCauseShareClaimAnchored) && c.Part != string(tracequery.RootCauseShareClaimUnwalked)) {
			continue
		}
		one := rn
		one.ShareClaims = []types.TraceCausalProjectionShareClaim{c}
		part := runtimeTraceProjRestrictedPart{MS: c.ShareMS,
			OwnAccount: ownAccount && c.Part == string(tracequery.RootCauseShareClaimAnchored) && c.Role == string(tracequery.RootCauseShareHolderChainSeat)}
		if ref := runtimeTraceProjResolveShareRef(one, index, row, false, c.Part); ref.state == runtimeTraceProjShareRefRendered && len(ref.tags) > 0 {
			part.Ref = strings.Join(ref.tags, ",")
		}
		row.RestrictedParts = append(row.RestrictedParts, part)
	}
}

// runtimeTraceProjResolvedRepresentedHolderTags (HMC377 round 10, R-377-10 ③)
// names the holders of a partially represented row's resolved claims — the
// rows that count its ChainAnchorRepresentedHeldMS: their [E#] when every such
// holder record is drawn on this projection in another row, nil otherwise (the
// see-detail form, which names no holder and claims nothing about the board).
// HMC377 round 11 (R-377-11 ①): a claim resolves per held segment, so a claim
// resolved in part names the holders of its resolved part; a holder that is a
// withdrawn seat's successor is not a chain seat — the form names none.
// HMC377 round 13 (R-377-13 ⑥): the partial [E#] form is the engine's
// held-in-part-and-cap-cut form only; a share part of which passed back to
// this row (ReturnedMS) is held + returned — a mixed disposition, which the
// engine words with the see-detail form — so the form names none.
func runtimeTraceProjResolvedRepresentedHolderTags(node types.TraceCausalProjectionNode, index map[string]runtimeTraceProjDrawnRecord, self *runtimeTraceProjTreeRow) []string {
	for _, c := range node.ShareClaims {
		if c.Represented && c.ReturnedMS > 0 {
			return nil
		}
	}
	var tags []string
	seen := map[string]bool{}
	for _, c := range node.ShareClaims {
		if !c.Represented || (!c.Resolved && c.ResolvedMS <= 0) {
			continue
		}
		if len(c.HolderEvidenceIDs) == 0 || c.Successor {
			return nil
		}
		for _, id := range c.HolderEvidenceIDs {
			drawn, ok := index[strings.TrimSpace(id)]
			if !ok || drawn.row == nil || drawn.row == self || drawn.tag == "" {
				return nil
			}
			if !seen[drawn.tag] {
				seen[drawn.tag] = true
				tags = append(tags, drawn.tag)
			}
		}
	}
	return tags
}

// runtimeTraceProjStampShareClaimRefs — HMC377 fix round 5 (replaces the RNB-1
// D1 twin-visibility sweep and the round-3/4 unwalked-seat marker lookup):
// every holder word of a row reads the row's engine-resolved share claims —
// the remainder's anchored-share holder (or, on case A', the chain seat's own
// account), the ⛓ seat's ◇ remainder, the unwalked part's holder, the
// represented sentence's holders. Returns the anchored-share references of the
// remainder rows (the pair class (0) counterpart).
func runtimeTraceProjStampShareClaimRefs(model *runtimeTraceProjTreeModel, all []*runtimeTraceProjTreeRow) map[*runtimeTraceProjTreeRow]runtimeTraceProjShareRef {
	index := runtimeTraceProjDrawnRecordIndex(model, all)
	anchored := map[*runtimeTraceProjTreeRow]runtimeTraceProjShareRef{}
	for _, row := range all {
		rn := row.Node
		if !row.HasData {
			continue
		}
		if rn.ChainAnchorFullMS > 0 {
			var twin runtimeTraceProjShareRef
			switch {
			case rn.ChainAnchorRemainderSeat && rn.ChainAnchorOwnershipDivergent:
				twin = runtimeTraceProjResolveShareRef(rn, index, row, false, string(tracequery.RootCauseShareClaimChainAccount))
			case rn.ChainAnchorRemainderSeat:
				twin = runtimeTraceProjResolveShareRef(rn, index, row, false, string(tracequery.RootCauseShareClaimAnchored))
				if twin.successor {
					// HMC377 round 12 (R-377-12 ②): a holder that is a
					// withdrawn seat's successor is no chain seat — the
					// 「(⛓链上席)」 word and the pair class never name it; the
					// see-detail form names no holder.
					twin = runtimeTraceProjShareRef{state: runtimeTraceProjShareRefNone}
				}
				anchored[row] = twin
			default:
				twin = runtimeTraceProjResolveShareRef(rn, index, row, false, string(tracequery.RootCauseShareClaimRemainder))
				// HMC377 round 12 (R-377-12 ③): the rest's carriers by [E#].
				runtimeTraceProjStampRestCarriers(row, twin)
			}
			row.ChainAnchorTwinInvisible = twin.state == runtimeTraceProjShareRefCompacted
			row.ChainAnchorTwinUnreferenced = twin.state == runtimeTraceProjShareRefNone
			if rn.ChainAnchorRemainderSeat {
				// HMC377 round 11 (R-377-11 ②): the part of the rest the
				// same thread's cause-specific row counts.
				runtimeTraceProjStampSubsetShare(row, index)
			}
			if rn.ChainAnchorUnwalkedMS > 0 {
				unwalked := runtimeTraceProjResolveShareRef(rn, index, row, false, string(tracequery.RootCauseShareClaimUnwalked))
				row.ChainAnchorUnwalkedRef = strings.Join(unwalked.tags, ",")
				row.ChainAnchorUnwalkedSeatMS = unwalked.seatMS
				row.ChainAnchorUnwalkedExact = unwalked.exact
				row.ChainAnchorUnwalkedInvisible = unwalked.state == runtimeTraceProjShareRefCompacted
				row.ChainAnchorUnwalkedUnreferenced = unwalked.state == runtimeTraceProjShareRefNone
			}
		}
		// HMC377 round 12 (R-377-12 ③): a restriction row's parts counted on
		// this thread's on-chain seats.
		runtimeTraceProjStampRestrictedParts(row, index)
		if rn.ChainAnchorRepresentedHeldMS > 0 && !rn.ChainAnchorRepresentedByChainSeat {
			row.RepresentedState = runtimeTraceProjShareRefPartial
			row.RepresentedRefs = runtimeTraceProjResolvedRepresentedHolderTags(rn, index, row)
			continue
		}
		if represented := runtimeTraceProjResolveShareRef(rn, index, row, true); represented.state != runtimeTraceProjShareRefNone || rn.ChainAnchorRepresentedByChainSeat {
			row.RepresentedState = represented.state
			row.RepresentedRefs = represented.tags
			// HMC377 round 11 (R-377-11 ①): the holder is a withdrawn seat's
			// published successor — named as the row of this thread that
			// counts the share, never as a chain seat.
			row.RepresentedBySuccessor = represented.state == runtimeTraceProjShareRefRendered && represented.successor
			row.RepresentedOneSuccessor = row.RepresentedBySuccessor && represented.oneSuccessor
		}
	}
	return anchored
}
