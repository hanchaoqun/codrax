package tool

import (
	"strings"
	"testing"
	"time"

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
	if len(doc.Blocks) != 2 ||
		doc.Blocks[0].Text != original ||
		doc.Blocks[1].ID != runtimeTraceFrequencyAuthorityBlockID ||
		!RuntimeTraceSystemBlock(doc.Blocks[1]) {
		t.Fatalf("frequency data boundary must follow model prose as a trusted system block: %+v", doc.Blocks)
	}
	if len(doc.Caveats) != 0 {
		t.Fatalf("frequency authority must not remain stranded in footer caveats: %+v", doc.Caveats)
	}
	got := types.AnswerBlockVisibleSurface(doc.Blocks[1])
	for _, want := range []string{
		"频率证据与结论边界",
		"172 条 CPU 频点样本",
		"323 条通用时钟变更事件",
		"两类计数只表示观测活动",
		"不能单独证明低频、降频、限频或计算供给不足",
		"当前没有独立的频率供给证据",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("frequency caveat missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"系统权威", "系统 authority", "后续模型正文", "以本块为准", "typed_supply_evidence"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("internal authority protocol leaked through %q:\n%s", forbidden, got)
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
	if materializeRuntimeTraceFrequencyAuthorityCaveat(doc, ctx) {
		t.Fatal("frequency authority block must be idempotent")
	}
	if len(doc.Blocks) != 2 ||
		doc.Blocks[0].ID != "summary" ||
		doc.Blocks[1].ID != runtimeTraceFrequencyAuthorityBlockID ||
		!RuntimeTraceSystemBlock(doc.Blocks[1]) {
		t.Fatalf("typed frequency data boundary must follow model prose: %+v", doc.Blocks)
	}
	got := types.AnswerBlockVisibleSurface(doc.Blocks[1])
	if !strings.Contains(got, "低频驻留") ||
		!strings.Contains(got, "不能归因于上述事件计数") ||
		!strings.Contains(got, "CPU0 418000–1530000kHz") ||
		!strings.Contains(got, "证明策略上限存在") ||
		!strings.Contains(got, "不能证明已经触顶或已造成性能影响") ||
		!strings.Contains(got, "不能单独证明热节流") {
		t.Fatalf("typed frequency caveat = %q", got)
	}
}

func TestRuntimeTraceFrequencyBoundaryUsesReaderFacingEnglish(t *testing.T) {
	start, end := 13762.791708, 13763.024898
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model answer",
	}}}
	ctx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Language: "en",
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
					RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
					TimeStart:      &start,
					TimeEnd:        &end,
					SourceQuote:    "13762.791708..13763.024898",
				},
			},
			AnswerContract: types.AnswerContract{Language: "en"},
		},
		ToolResults: []types.ToolResult{{
			ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				FrequencyTransitionEventCount: 12,
				FrequencyLimitWitnesses: []types.TraceFrequencyLimitAuthority{{
					CPU: 0, MinFrequencyKHz: 418000, MaxFrequencyKHz: 1530000,
					LimitRowCount: 16, WitnessLine: 8048, WitnessTs: 13762.861720,
					WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
					Authority: "direct_in_window_policy_limit",
				}},
			},
		}},
	}
	if !materializeRuntimeTraceFrequencyAuthorityCaveat(doc, ctx) {
		t.Fatal("English frequency boundary did not materialize")
	}
	if doc.Blocks[0].ID != "summary" || doc.Blocks[1].ID != runtimeTraceFrequencyAuthorityBlockID {
		t.Fatalf("English frequency boundary must follow the model answer: %+v", doc.Blocks)
	}
	surface := types.AnswerBlockVisibleSurface(doc.Blocks[1])
	for _, want := range []string{
		"Frequency evidence and conclusion limits",
		"12 CPU-frequency samples",
		"CPU0 418000–1530000kHz",
		"prove that a policy ceiling existed",
		"does not by itself prove thermal throttling",
	} {
		if !strings.Contains(surface, want) {
			t.Fatalf("English frequency boundary missing %q:\n%s", want, surface)
		}
	}
	for _, forbidden := range []string{"System authority", "later model prose", "takes precedence"} {
		if strings.Contains(surface, forbidden) {
			t.Fatalf("internal English authority protocol leaked through %q:\n%s", forbidden, surface)
		}
	}
}

func TestPersistMergedAnswerDocumentPublishesFrequencyBoundaryAfterModelProse(t *testing.T) {
	ctx := newBusForMutationTest()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{Intent: types.IntentRootCause},
		AnswerContract: types.AnswerContract{
			Language: "zh",
		},
	}
	ctx.ToolResults = []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			FrequencyTransitionEventCount:   830,
			FrequencyClockSetRateEventCount: 767,
			FrequencyTypedSupplyEvidence: []string{
				"compute_supply_low_frequency_deficit",
				"direct_in_window_policy_limit",
			},
			FrequencyLimitWitnesses: []types.TraceFrequencyLimitAuthority{{
				CPU: 4, MinFrequencyKHz: 558000, MaxFrequencyKHz: 2100000,
				LimitRowCount: 28, WitnessLine: 17113, WitnessTs: 13762.940114,
				WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
				Authority: "direct_in_window_policy_limit",
			}},
		},
	}}
	modelText := "558MHz 低于 2.10GHz，所以明确是热节流而不是 policy 限制。"
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID: "summary", Kind: types.BlockSummary, Text: modelText,
		}},
	}
	result, err := ApplyAndPersistMutation(
		ctx,
		"test_emit",
		types.NewReplaceAllMutation(doc),
		nil,
		time.Now(),
	)
	if err != nil || !result.Success {
		t.Fatalf("persist frequency authority failed: result=%+v err=%v", result, err)
	}
	persisted := ctx.Mutable.AnswerDocumentV2()
	if persisted == nil || len(persisted.Blocks) != 2 ||
		persisted.Blocks[0].Text != modelText ||
		persisted.Blocks[1].ID != runtimeTraceFrequencyAuthorityBlockID ||
		!RuntimeTraceSystemBlock(persisted.Blocks[1]) {
		t.Fatalf("production persist must keep model prose before the trusted frequency boundary: %+v", persisted)
	}
	surface := types.AnswerBlockVisibleSurface(persisted.Blocks[1])
	for _, want := range []string{
		"CPU4 558000–2100000kHz",
		"证明策略上限存在",
		"不能证明已经触顶或已造成性能影响",
		"不能单独证明热节流",
	} {
		if !strings.Contains(surface, want) {
			t.Fatalf("production frequency authority missing %q:\n%s", want, surface)
		}
	}
}
