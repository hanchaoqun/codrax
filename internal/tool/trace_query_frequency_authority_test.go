package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryFrequencyTransitionCountStaysBackgroundOnly(t *testing.T) {
	result := tracequery.Result{
		View: "event_search",
		CPUFrequencyCensus: &tracequery.CPUFrequencyCensus{
			MatchedFrequencyRows:   172,
			DisplayedFrequencyRows: 40,
		},
	}
	authority := traceQueryEvidenceAuthority(result)
	if authority == nil {
		t.Fatal("missing trace evidence authority")
	}
	if authority.FrequencyTransitionEventCount != 172 ||
		authority.FrequencyTransitionAuthority != "background_only" ||
		authority.FrequencySupplyConclusion != "unproven_from_transition_count" ||
		len(authority.FrequencyTypedSupplyEvidence) != 0 {
		t.Fatalf("count-only frequency authority = %+v", authority)
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "event_search"}, "customer.systrace", "")
	for _, want := range []string{
		"frequency_authority cpu_frequency_rows=172",
		"clock_set_rate_events=0",
		"transition_authority=background_only",
		"frequency_supply_conclusion=unproven_from_transition_count",
		"two typed counts are separate background activity",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestTraceQueryFrequencyAuthoritySeparatesTypedSupplyEvidence(t *testing.T) {
	result := tracequery.Result{
		View:      "window_stats",
		TimeStart: 13762.791708,
		TimeEnd:   13763.024898,
		WindowStats: &tracequery.WindowStats{
			CPUFrequencySampleRowCount: 12,
			ClockSetRateEventCount:     37,
			SupplyPressureSummary: &tracequery.SupplyPressureSummary{
				LowFrequencyCPUs: []int{0, 1},
			},
			ComputeSupplyBalance: &tracequery.ComputeSupplyBalance{
				LowFrequencyLossMs: 2.5,
			},
			CPUFrequencyLimits: []tracequery.CPUFrequencyLimit{{
				CPU: 0, MinFrequency: 418000, MaxFrequency: 1530000,
				Count: 16, Line: 8048, Ts: 13762.861720,
			}},
		},
	}
	authority := traceQueryEvidenceAuthority(result)
	if authority.FrequencyTransitionAuthority != "background_only" ||
		authority.FrequencySupplyConclusion != "bounded_by_typed_supply_evidence" ||
		authority.FrequencyTransitionEventCount != 12 ||
		authority.FrequencyClockSetRateEventCount != 37 {
		t.Fatalf("typed frequency authority = %+v", authority)
	}
	got := strings.Join(authority.FrequencyTypedSupplyEvidence, ",")
	for _, want := range []string{
		"frequency_residency_low_frequency",
		"compute_supply_low_frequency_deficit",
		"direct_in_window_policy_limit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed supply evidence missing %q: %q", want, got)
		}
	}
	if len(authority.FrequencyLimitWitnesses) != 1 {
		t.Fatalf("frequency limit witnesses = %+v, want one direct row", authority.FrequencyLimitWitnesses)
	}
	if authority.FrequencyPolicyLimitStatus != "present" ||
		authority.FrequencyLimitBindingCaliber != "limit_row_proves_ceiling_presence;binding_impact_requires_separate_overlap_or_supply_evidence" {
		t.Fatalf("frequency policy semantics = %+v", authority)
	}
	witness := authority.FrequencyLimitWitnesses[0]
	if witness.CPU != 0 || witness.MinFrequencyKHz != 418000 || witness.MaxFrequencyKHz != 1530000 ||
		witness.LimitRowCount != 16 || witness.WitnessLine != 8048 || witness.WitnessTs != 13762.861720 ||
		witness.WindowStartTs != result.TimeStart || witness.WindowEndTs != result.TimeEnd ||
		witness.Authority != "direct_in_window_policy_limit" {
		t.Fatalf("frequency limit witness = %+v", witness)
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "customer.systrace", "")
	for _, want := range []string{
		"frequency_limit_witness cpu=0 min=418000kHz max=1530000kHz",
		"limit_rows=16 witness_line=8048 witness_ts=13762.861720",
		"window=13762.791708..13763.024898",
		"authority=direct_in_window_policy_limit",
		"policy_limit_status=present",
		"binding_caliber=limit_row_proves_ceiling_presence;binding_impact_requires_separate_overlap_or_supply_evidence",
		"actual frequency below the ceiling neither negates that limit nor proves its binding performance impact",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("head-safe summary missing %q:\n%s", want, summary)
		}
	}
}

func TestTraceQueryFrequencyLimitAuthorityRejectsDisplayZeros(t *testing.T) {
	authority := traceQueryEvidenceAuthority(tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			CPUFrequencyLimits: []tracequery.CPUFrequencyLimit{
				{CPU: 0, MaxFrequency: 0, Count: 1, Line: 10},
				{CPU: 1, MaxFrequency: 1530000, Count: 0, Line: 11},
				{CPU: 2, MaxFrequency: 1530000, Count: 1, Line: 0},
			},
		},
	})
	if authority == nil {
		t.Fatal("missing trace evidence authority")
	}
	if len(authority.FrequencyLimitWitnesses) != 0 {
		t.Fatalf("invalid display rows minted direct authority: %+v", authority.FrequencyLimitWitnesses)
	}
	if got := strings.Join(authority.FrequencyTypedSupplyEvidence, ","); strings.Contains(got, "direct_in_window_policy_limit") {
		t.Fatalf("invalid display rows minted direct evidence token: %q", got)
	}
}

func TestTraceQueryAutoWindowFrequencyLimitAuthorityDeduplicatesWitnesses(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats", TimeStart: 1, TimeEnd: 2,
		WindowStats: &tracequery.WindowStats{CPUFrequencyLimits: []tracequery.CPUFrequencyLimit{{
			CPU: 4, MinFrequency: 558000, MaxFrequency: 2100000,
			Count: 28, Line: 17113, Ts: 1.5,
		}}},
	}
	authority := traceQueryAutoWindowEvidenceAuthority([]traceQueryAutoWindowChild{
		{Result: result},
		{Result: result},
	})
	if authority == nil || len(authority.FrequencyLimitWitnesses) != 1 {
		t.Fatalf("combined frequency witnesses = %+v, want one deduplicated row", authority)
	}
	if authority.FrequencyPolicyLimitStatus != "present" ||
		authority.FrequencyLimitBindingCaliber == "" {
		t.Fatalf("combined frequency semantics = %+v", authority)
	}
}

func TestRuntimeTraceFrequencyAuthorityCaveatRejectsCountOnlyCausality(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "CPU 频率调整 172 次，因此可以确定持续低频并造成算力不足。",
		}},
	}
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			FrequencyTransitionEventCount:   172,
			FrequencyClockSetRateEventCount: 323,
			FrequencyTransitionAuthority:    "background_only",
			FrequencySupplyConclusion:       "unproven_from_transition_count",
		},
	}}}
	original := doc.Blocks[0].Text
	if !materializeRuntimeTraceFrequencyAuthorityCaveat(doc, ctx) {
		t.Fatal("expected frequency authority caveat")
	}
	if doc.Blocks[0].Text != original {
		t.Fatalf("model prose was rewritten: %q", doc.Blocks[0].Text)
	}
	got := strings.Join(doc.Caveats, "\n")
	for _, want := range []string{
		"cpu_frequency_rows=172",
		"clock_set_rate_events=323",
		"transition_authority=background_only",
		"两类计数分别表示 CPU 频点样本和通用时钟变更活动",
		"frequency_supply_conclusion=unproven_from_transition_count",
		"typed_supply_evidence=none",
		"低频/供给因果措辞均未获当前计数授权",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("frequency caveat missing %q:\n%s", want, got)
		}
	}
}

func TestRuntimeTraceFrequencyAuthorityCaveatNamesIndependentTypedEvidence(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "窗口存在频率变化。",
		}},
	}
	ctx := &types.BusContext{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			FrequencyTransitionEventCount:   12,
			FrequencyClockSetRateEventCount: 7,
			FrequencyTransitionAuthority:    "background_only",
			FrequencySupplyConclusion:       "bounded_by_typed_supply_evidence",
			FrequencyTypedSupplyEvidence: []string{
				"frequency_residency_low_frequency",
			},
			FrequencyLimitWitnesses: []types.TraceFrequencyLimitAuthority{{
				CPU: 0, MinFrequencyKHz: 418000, MaxFrequencyKHz: 1530000,
				LimitRowCount: 16, WitnessLine: 8048, WitnessTs: 13762.861720,
				WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
				Authority: "direct_in_window_policy_limit",
			}},
		},
	}}}
	if !materializeRuntimeTraceFrequencyAuthorityCaveat(doc, ctx) {
		t.Fatal("expected frequency authority caveat")
	}
	got := strings.Join(doc.Caveats, "\n")
	if !strings.Contains(got, "typed_supply_evidence=frequency_residency_low_frequency") ||
		!strings.Contains(got, "不能归因于上述两类计数") ||
		!strings.Contains(got, "direct_in_window_policy_limits=cpu0[min=418000kHz,max=1530000kHz") ||
		!strings.Contains(got, "policy_limit_status=present") ||
		!strings.Contains(got, "低于 ceiling 不能反推「无策略限制」") ||
		!strings.Contains(got, "binding_impact_requires_separate_overlap_or_supply_evidence") {
		t.Fatalf("typed frequency caveat = %q", got)
	}
}
