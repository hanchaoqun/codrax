package tool

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const runtimeTraceFrequencyAuthorityBlockID = "runtime_trace_frequency_authority"

// materializeRuntimeTraceFrequencyAuthorityCaveat keeps frequency transition
// activity on the background lane even when the model prose promoted a large
// count into a low-frequency/throttling claim. The count itself never
// authorizes supply causality; any stronger wording must bind to the separate
// typed supply-evidence roster. The authority is a leading system block rather
// than a footer caveat so a contradictory model principal cannot precede the
// typed policy-limit/binding boundary.
func materializeRuntimeTraceFrequencyAuthorityCaveat(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil {
		return false
	}
	if !runtimeTraceFullReportMaterializationAllowed(ctx) {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	results := append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...)
	count := 0
	clockSetRateCount := 0
	evidenceSet := map[string]bool{}
	var limitWitnesses []types.TraceFrequencyLimitAuthority
	for _, result := range results {
		if strings.TrimSpace(result.ToolName) != "trace_query" || result.TraceEvidenceAuthority == nil {
			continue
		}
		authority := result.TraceEvidenceAuthority
		count = max(count, authority.FrequencyTransitionEventCount)
		clockSetRateCount = max(clockSetRateCount, authority.FrequencyClockSetRateEventCount)
		for _, token := range authority.FrequencyTypedSupplyEvidence {
			token = strings.TrimSpace(token)
			if token != "" {
				evidenceSet[token] = true
			}
		}
		limitWitnesses = append(limitWitnesses, authority.FrequencyLimitWitnesses...)
	}
	limitWitnesses = runtimeTraceDedupFrequencyLimitWitnesses(limitWitnesses, 8)
	if count <= 0 && clockSetRateCount <= 0 && len(limitWitnesses) == 0 {
		return false
	}
	evidence := make([]string, 0, len(evidenceSet))
	for token := range evidenceSet {
		evidence = append(evidence, token)
	}
	sort.Strings(evidence)
	conclusion := "unproven_from_transition_count"
	if len(evidence) > 0 || len(limitWitnesses) > 0 {
		conclusion = "bounded_by_typed_supply_evidence"
	}
	frequencyRows := "not_reported"
	if count > 0 {
		frequencyRows = strconv.Itoa(count)
	}
	clockSetRateEvents := "not_reported"
	if clockSetRateCount > 0 {
		clockSetRateEvents = strconv.Itoa(clockSetRateCount)
	}
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	var caveat string
	if zh {
		caveat = fmt.Sprintf(
			"频率证据权限：cpu_frequency_rows=%s，clock_set_rate_events=%s，transition_authority=background_only；两类计数分别表示 CPU 频点样本和通用时钟变更活动，均不单独证明低频、降频、限频或计算供给不足。frequency_supply_conclusion=%s",
			frequencyRows, clockSetRateEvents, conclusion,
		)
		if len(evidence) == 0 {
			caveat += "；typed_supply_evidence=none，任何低频/供给因果措辞均未获当前计数授权"
		} else {
			caveat += "；typed_supply_evidence=" + strings.Join(evidence, ",") +
				"；低频/供给措辞只能绑定这些 typed 证据及其链/排序口径，不能归因于上述两类计数"
		}
		if len(limitWitnesses) > 0 {
			caveat += "；direct_in_window_policy_limits=" + runtimeTraceFrequencyLimitWitnessRoster(limitWitnesses) +
				"；policy_limit_status=present：这些 min/max 行直接证明窗内存在 policy ceiling，实际/平均/驻留频率低于 ceiling 不能反推「无策略限制」；binding_caliber=limit_row_proves_ceiling_presence;binding_impact_requires_separate_overlap_or_supply_evidence，是否顶到 ceiling 及其性能影响须由独立 overlap/compute-supply 证据证明；thermal_or_policy_mechanism=requires_typed_causal_witness：低于 ceiling 的实际/驻留频率不能单独区分 workload demand、policy、thermal 或其他治理机制，也不能单独证明热节流"
		}
	} else {
		caveat = fmt.Sprintf(
			"Frequency evidence authority: cpu_frequency_rows=%s, clock_set_rate_events=%s, transition_authority=background_only; the counts separately represent CPU frequency samples and generic clock-change activity, and neither count by itself proves low frequency, throttling, a frequency limit, or compute-supply shortage. frequency_supply_conclusion=%s",
			frequencyRows, clockSetRateEvents, conclusion,
		)
		if len(evidence) == 0 {
			caveat += "; typed_supply_evidence=none, so no low-frequency or supply-causal wording is authorized by the count"
		} else {
			caveat += "; typed_supply_evidence=" + strings.Join(evidence, ",") +
				"; low-frequency/supply wording must bind to that typed evidence and its chain/rank caliber, never to either count above"
		}
		if len(limitWitnesses) > 0 {
			caveat += "; direct_in_window_policy_limits=" + runtimeTraceFrequencyLimitWitnessRoster(limitWitnesses) +
				"; policy_limit_status=present: these min/max rows directly prove that a policy ceiling existed in the window, and an actual/average/residency frequency below the ceiling cannot be used to conclude that no policy limit existed; binding_caliber=limit_row_proves_ceiling_presence;binding_impact_requires_separate_overlap_or_supply_evidence, so hitting the ceiling and its performance impact require independent overlap/compute-supply evidence; thermal_or_policy_mechanism=requires_typed_causal_witness: an actual/residency frequency below the ceiling cannot by itself distinguish workload demand, policy, thermal, or another governance mechanism and does not by itself prove thermal throttling"
		}
	}
	title := "系统权威：CPU 频率供给与策略限制"
	lead := "本块是频率样本、供给证据与 policy-limit 权限的系统 authority；后续模型正文若对 ceiling 是否存在、是否触顶、性能影响或 thermal/policy 成因给出冲突结论，以本块为准。"
	if !zh {
		title = "System authority: CPU frequency supply and policy limits"
		lead = "This block is the system authority for frequency samples, supply evidence, and policy-limit permissions. If later model prose conflicts about ceiling presence, hitting the ceiling, performance impact, or a thermal/policy mechanism, this block takes precedence."
	}
	return insertRuntimeTraceLeadAuthorityBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceFrequencyAuthorityBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  lead + "\n\n" + caveat,
	})
}

func runtimeTraceDedupFrequencyLimitWitnesses(in []types.TraceFrequencyLimitAuthority, limit int) []types.TraceFrequencyLimitAuthority {
	if len(in) == 0 {
		return nil
	}
	byKey := map[string]types.TraceFrequencyLimitAuthority{}
	var keys []string
	for _, witness := range in {
		key := fmt.Sprintf(
			"%d/%d/%d/%d/%d/%.9f/%.9f/%.9f/%s",
			witness.CPU,
			witness.MinFrequencyKHz,
			witness.MaxFrequencyKHz,
			witness.LimitRowCount,
			witness.WitnessLine,
			witness.WitnessTs,
			witness.WindowStartTs,
			witness.WindowEndTs,
			strings.TrimSpace(witness.Authority),
		)
		if _, ok := byKey[key]; ok {
			continue
		}
		byKey[key] = witness
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	out := make([]types.TraceFrequencyLimitAuthority, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func runtimeTraceFrequencyLimitWitnessRoster(in []types.TraceFrequencyLimitAuthority) string {
	rows := make([]string, 0, len(in))
	for _, witness := range in {
		rows = append(rows, fmt.Sprintf(
			"cpu%d[min=%dkHz,max=%dkHz,limit_rows=%d,line=%d,ts=%.6f,window=%.6f..%.6f,authority=%s]",
			witness.CPU,
			witness.MinFrequencyKHz,
			witness.MaxFrequencyKHz,
			witness.LimitRowCount,
			witness.WitnessLine,
			witness.WitnessTs,
			witness.WindowStartTs,
			witness.WindowEndTs,
			strings.TrimSpace(witness.Authority),
		))
	}
	return strings.Join(rows, "|")
}

// materializeRuntimeTraceVsyncAuthorityCaveat — GAP-B3 (§13.3, 2026-07-25):
// the vsync generator census is the ONLY period authority in the run, yet it
// never reached the answer face — the fourth replay's prose promoted capped
// consumer-callback samples ("6 signals vs a theoretical 13-14", 16→33ms)
// into a signal-loss claim unchallenged. When the census observation is in
// the ledger, one deterministic note states the caliber boundary. Precise
// trigger (typed census predicate), wording only.
func materializeRuntimeTraceVsyncAuthorityCaveat(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil {
		return false
	}
	if !runtimeTraceFullReportMaterializationAllowed(ctx) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	generators := 0
	periodPrints := 0
	for _, record := range ledger.Records {
		if strings.TrimSpace(record.Predicate) != "vsync_generator_census" {
			continue
		}
		generators++
		if raw := runtimeTraceObservationRichNoteValue(record.RichNotes, "vsync_generator_census_period_prints"); raw != "" {
			if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && parsed > 0 {
				periodPrints += parsed
			}
		}
	}
	if generators == 0 {
		return false
	}
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	var caveat string
	if zh {
		caveat = fmt.Sprintf(
			"VSync 周期权威：帧节拍发生器普查=%d 个发生器（period_prints=%d）；发生器自身的 period 打印才是信号周期权威；消费者回调间距（如 onVsync/VSync-rs 间隔）只测帧节拍、可跨越跳帧，不得当作 vsync 信号周期或「信号丢失」的证据",
			generators, periodPrints,
		)
	} else {
		caveat = fmt.Sprintf(
			"VSync period authority: the generator census identified %d generator(s) (period_prints=%d); only a generator's own period print states the signal period. Consumer callback spacing (e.g. onVsync/VSync-rs intervals) measures frame pacing, may span skipped frames, and must not be reported as the vsync period or as signal-loss evidence",
			generators, periodPrints,
		)
	}
	for _, existing := range doc.Caveats {
		if strings.TrimSpace(existing) == caveat {
			return false
		}
	}
	doc.Caveats = append(doc.Caveats, caveat)
	return true
}
