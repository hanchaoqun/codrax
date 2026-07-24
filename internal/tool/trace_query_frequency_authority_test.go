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
		"frequency_authority transition_events=172",
		"transition_authority=background_only",
		"frequency_supply_conclusion=unproven_from_transition_count",
		"transition count is background activity only",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestTraceQueryFrequencyAuthoritySeparatesTypedSupplyEvidence(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			EventCounts: map[tracequery.EventType]int{
				tracequery.EventCPUFrequency: 12,
			},
			SupplyPressureSummary: &tracequery.SupplyPressureSummary{
				LowFrequencyCPUs: []int{0, 1},
			},
			ComputeSupplyBalance: &tracequery.ComputeSupplyBalance{
				LowFrequencyLossMs: 2.5,
			},
		},
	}
	authority := traceQueryEvidenceAuthority(result)
	if authority.FrequencyTransitionAuthority != "background_only" ||
		authority.FrequencySupplyConclusion != "bounded_by_typed_supply_evidence" {
		t.Fatalf("typed frequency authority = %+v", authority)
	}
	got := strings.Join(authority.FrequencyTypedSupplyEvidence, ",")
	for _, want := range []string{
		"frequency_residency_low_frequency",
		"compute_supply_low_frequency_deficit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed supply evidence missing %q: %q", want, got)
		}
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
			FrequencyTransitionEventCount: 172,
			FrequencyTransitionAuthority:  "background_only",
			FrequencySupplyConclusion:     "unproven_from_transition_count",
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
		"transition_events=172",
		"transition_authority=background_only",
		"事件计数只证明调频活动",
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
			FrequencyTransitionEventCount: 12,
			FrequencyTransitionAuthority:  "background_only",
			FrequencySupplyConclusion:     "bounded_by_typed_supply_evidence",
			FrequencyTypedSupplyEvidence: []string{
				"frequency_residency_low_frequency",
			},
		},
	}}}
	if !materializeRuntimeTraceFrequencyAuthorityCaveat(doc, ctx) {
		t.Fatal("expected frequency authority caveat")
	}
	got := strings.Join(doc.Caveats, "\n")
	if !strings.Contains(got, "typed_supply_evidence=frequency_residency_low_frequency") ||
		!strings.Contains(got, "不能归因于 transition count") {
		t.Fatalf("typed frequency caveat = %q", got)
	}
}
