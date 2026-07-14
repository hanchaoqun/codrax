package types

// trace_supply_gap_dominance.go — INV-SUPPLY 件① typed dominance criterion
// (§29.61.11 用户裁定 + §29.61.11a 喂入补充, docs/design/
// real_trace_campaign_20260705.md, 2026-07-14).
//
// One PRECISE typed comparison shared by BOTH consumers so the two faces can
// never fork (the display compound word in internal/tool and the seat-
// composition named fact in internal/context judge the SAME inequality):
//
//	supply-fold deficit ms  ≥  effective attribution ms × share
//
// Both operands are ENGINE-PUBLISHED typed values (supply_fold_deficit_ms /
// effective_impact_ms notes — never re-derived, never prose-parsed), so the
// criterion satisfies the precise-signals discipline even though it only
// selects WORDING and feed lines (soft faces; ranking, gates and values are
// untouched).
//
// Threshold basis (dual-trace witness calibration, 2026-07-14): the ruling
// names 50% as the candidate and the witnesses support it —
//
//   - donghu 090607 window (13762.791708..13763.024898, target
//     .ugc.aweme.lite-17267): every fold-bearing inversion seat sits well
//     above the line — ❶ CompThread_0-2955 7.296/7.081 = 103%, ❷
//     JankManager-9655 2.951/4.596 = 64%, keva-3-17439 (#6) 2.286/3.183 =
//     72%; the runnable-only inversion seats (keva-1 #4, 全额) carry no fold
//     and are structurally outside the criterion.
//   - tieba window (donghu_tieba_frame.systrace 34579.x): no fold-bearing
//     inversion seat exists; its only fold seat is a PURE running
//     算力供给候选 seat whose eff==deficit identity makes the ratio 100% by
//     construction (a different word family — see the census note at the
//     display predicate).
//
// 0.50 therefore sits below the observed dominant cluster (64%–103%) with
// margin while keeping the plain-language semantics of the compound word
// ("the supply-gap magnitude is at majority scale relative to the seat's
// counted attribution"). 阈值常量禁跨语义借用 (§29.26 容差纪律): this
// constant is the INV-SUPPLY dominance share and nothing else — it must not
// be borrowed by (or borrowed from) the §7.10 supply-fold wording thresholds
// (runtimeTraceProjSupplyFoldDeficitShare 0.20 / floor 1ms), the runnable
// significance gate, or any other ratio.
const TraceSupplyGapDominanceShare = 0.50

// TraceSupplyGapDominant reports whether a seat's published supply-fold
// deficit dominates its published effective attribution per the INV-SUPPLY
// §29.61.11 criterion. Pure typed comparison; both zero/absent operands fail
// closed (an unpublished value never claims dominance).
func TraceSupplyGapDominant(supplyFoldDeficitMS, effectiveImpactMS float64) bool {
	return supplyFoldDeficitMS > 0 && effectiveImpactMS > 0 &&
		supplyFoldDeficitMS >= effectiveImpactMS*TraceSupplyGapDominanceShare
}
