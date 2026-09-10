package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This pre-pass only constrains the old state/board/hop admission rules. Equal
// event-line restrictions are not proof of interval identity or additivity.
// The selected account wins as before; no amount elects a replacement scope.
func runtimeTraceProjWaitMeasurementEligibility(model runtimeTraceProjTreeModel) ([]bool, bool) {
	type candidate struct {
		key            string
		ok, state, hop bool
	}
	rows := make([]candidate, len(model.SelfRows))
	stateCount := 0
	for i, row := range model.SelfRows {
		if row.SelfSymptomRelocated {
			continue
		}
		state := row.Node.Role != types.TraceCausalRoleCausalHop &&
			runtimeTraceProjSymptomFamilyStateKind(row.Node) && row.Node.ImpactMS > 0
		hop := row.Node.Role == types.TraceCausalRoleCausalHop && row.Node.IsSleepState() &&
			runtimeTraceProjNodeDisplayImpact(row.Node) > 0 && !runtimeTraceProjMultiWindowMergedRow(row.Node) &&
			(row.Node.WithinRequestedWindow == nil || *row.Node.WithinRequestedWindow)
		if !state && !hop {
			continue
		}
		key, ok := types.TraceSchedulerMeasurementRestrictionKey(row.Node.MeasurementOrigins)
		rows[i] = candidate{key: key, ok: ok, state: state, hop: hop}
		if state {
			stateCount++
		}
	}
	selected, groupable := types.TraceSchedulerMeasurementRestrictionKey(model.TargetMeasurementOrigins)
	if groupable && selected == "" {
		// Legacy/no-account keeps its own lane when present. If every old
		// candidate is natively known, one clean restriction may use the old
		// rules; multiple/mixed restrictions cannot elect themselves by size.
		legacy, mixed := false, false
		known := map[string]bool{}
		for _, row := range rows {
			if !row.state && !(stateCount == 0 && row.hop) {
				continue
			}
			if !row.ok {
				mixed = true
			} else if row.key == "" {
				legacy = true
			} else {
				known[row.key] = true
			}
		}
		if !legacy {
			groupable = !mixed && len(known) <= 1
			for key := range known {
				selected = key // exactly one, or groupable=false
			}
		}
	}
	eligible := make([]bool, len(rows))
	for i, row := range rows {
		eligible[i] = groupable && row.ok && row.key == selected
	}
	// Only an actual contributor under the old state/board/MAX rules can
	// change arithmetic when its source is excluded. Losing boards and
	// smaller sleep-MAX candidates stay in the census without falsely
	// invalidating a denominator they never contributed to.
	originalWait, originalAdmitted, _, _ := runtimeTraceProjTargetSymptomAdmissionWithEligibility(model, nil)
	limited := false
	for i, row := range rows {
		if originalAdmitted[i] && !eligible[i] {
			limited = true
		}
		// The old hop-only lane has no admitted wait denominator. Mixed or
		// unbound candidates cannot silently borrow the full-window fallback.
		if originalWait <= 0 && (row.state || row.hop) && !eligible[i] {
			limited = true
		}
	}
	return eligible, limited
}

func runtimeTraceProjWaitMeasurementScopeNote(wait float64, excluded int, maxMS, attributed float64, zh bool) string {
	if zh {
		prefix := "\n- 关注线程等待记录的事件筛选范围未能对齐，未合并为同一等待总量"
		if wait > 0 {
			prefix = fmt.Sprintf("\n- 仅计入同一事件筛选范围的等待 %.3fms", wait)
		}
		return fmt.Sprintf("%s；另有 %d 条状态行未合入等待总量(单项最大 %.3fms)，原值分列保留；链上单项最大 %.3fms。事件筛选范围未能对齐，不给出覆盖百分比，不计未归因。", prefix, excluded, maxMS, attributed)
	}
	prefix := "\n- Focused-thread wait records have unaligned event-filter scopes and are not combined into one wait total"
	if wait > 0 {
		prefix = fmt.Sprintf("\n- Only %.3fms of wait from one event-filter scope is counted", wait)
	}
	return fmt.Sprintf("%s; %d more state row(s) are outside the denominator (single largest %.3fms), with their original values retained separately; largest individual on-chain contribution %.3fms. Event-filter scopes are not aligned: no coverage percentage, no unattributed residual.", prefix, excluded, maxMS, attributed)
}
