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
// typed supply-evidence roster. The trusted system marker remains internal;
// the visible block is a reader-facing data boundary placed after the answer's
// narrative and causal decision surfaces.
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
			"窗内记录了 %s 条 CPU 频点样本和 %s 条通用时钟变更事件；这两类计数只表示观测活动，均不能单独证明低频、降频、限频或计算供给不足。",
			frequencyRows, clockSetRateEvents,
		)
		if len(evidence) == 0 {
			caveat += " 当前没有单独的频率供给证据，因此不能仅凭这些计数判断供给影响。"
		} else {
			caveat += " 可用于供给判断的单独证据为：" + runtimeTraceFrequencyEvidenceRoster(evidence, true) +
				"；相关结论只能按这些证据各自的链路和排序口径解释，不能归因于上述事件计数。"
		}
		if len(limitWitnesses) > 0 {
			caveat += " 窗内策略上下限记录：" + runtimeTraceFrequencyLimitWitnessRoster(limitWitnesses, true) +
				"。这些 min/max 记录可以证明策略上限存在，但不能证明已经触顶或已造成性能影响；实际、平均或驻留频率低于上限，也不能单独区分负载需求、策略控制、热约束或其他治理机制，更不能单独证明热节流。"
		}
	} else {
		caveat = fmt.Sprintf(
			"The window contains %s CPU-frequency samples and %s generic clock-change events. These counts describe observation activity only; neither count by itself proves low frequency, throttling, a frequency limit, or a compute-supply shortage.",
			frequencyRows, clockSetRateEvents,
		)
		if len(evidence) == 0 {
			caveat += " No separate frequency-supply evidence is available, so the counts alone cannot establish a supply impact."
		} else {
			caveat += " Separate evidence available for supply analysis: " + runtimeTraceFrequencyEvidenceRoster(evidence, false) +
				". Any supply conclusion must follow the chain and ranking scope of that evidence, not the event counts above."
		}
		if len(limitWitnesses) > 0 {
			caveat += " In-window policy-limit records: " + runtimeTraceFrequencyLimitWitnessRoster(limitWitnesses, false) +
				". These min/max rows prove that a policy ceiling existed, but not that it was reached or had a performance impact. An actual, average, or residency frequency below the ceiling cannot by itself distinguish workload demand, policy control, thermal constraints, or another governance mechanism, and does not by itself prove thermal throttling."
		}
	}
	title := "频率证据与结论边界"
	if !zh {
		title = "Frequency evidence and conclusion limits"
	}
	return insertRuntimeTraceDataBoundaryBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceFrequencyAuthorityBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  caveat,
	})
}

func runtimeTraceFrequencyEvidenceRoster(in []string, zh bool) string {
	rows := make([]string, 0, len(in))
	for _, token := range in {
		label := runtimeTraceFrequencyEvidenceLabel(token, zh)
		if label != "" {
			rows = append(rows, label)
		}
	}
	separator := ", "
	if zh {
		separator = "、"
	}
	return strings.Join(rows, separator)
}

func runtimeTraceFrequencyEvidenceLabel(token string, zh bool) string {
	token = strings.TrimSpace(token)
	labels := map[string][2]string{
		"frequency_residency_low_frequency":    {"low-frequency residency", "低频驻留"},
		"direct_in_window_policy_limit":        {"in-window policy limits", "窗内策略上下限"},
		"cluster_frequency_ceiling":            {"cluster frequency ceilings", "簇频率上限"},
		"frequency_limit_or_cluster_ceiling":   {"policy or cluster frequency ceilings", "策略或簇频率上限"},
		"compute_supply_low_frequency_deficit": {"low-frequency compute-supply deficit", "低频运行折算缺口"},
		"ranked_frequency_supply_evidence":     {"ranked frequency-supply evidence", "根因榜中的频率影响项"},
		"ranked_cap_or_supply_deficit":         {"ranked cap or supply deficit", "根因榜中的频率/核类受限项"},
	}
	if pair, ok := labels[token]; ok {
		if zh {
			return pair[1]
		}
		return pair[0]
	}
	return token
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

func runtimeTraceFrequencyLimitWitnessRoster(in []types.TraceFrequencyLimitAuthority, zh bool) string {
	rows := make([]string, 0, len(in))
	for _, witness := range in {
		if zh {
			rows = append(rows, fmt.Sprintf(
				"CPU%d %d–%dkHz（%d条，样例行%d@%.6fs，窗口%.6f..%.6f）",
				witness.CPU,
				witness.MinFrequencyKHz,
				witness.MaxFrequencyKHz,
				witness.LimitRowCount,
				witness.WitnessLine,
				witness.WitnessTs,
				witness.WindowStartTs,
				witness.WindowEndTs,
			))
		} else {
			rows = append(rows, fmt.Sprintf(
				"CPU%d %d–%dkHz (%d rows, sample line %d at %.6fs, window %.6f..%.6f)",
				witness.CPU,
				witness.MinFrequencyKHz,
				witness.MaxFrequencyKHz,
				witness.LimitRowCount,
				witness.WitnessLine,
				witness.WitnessTs,
				witness.WindowStartTs,
				witness.WindowEndTs,
			))
		}
	}
	return strings.Join(rows, "；")
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
