package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceGuidanceCarriesDirectFrequencyLimitWitnesses(t *testing.T) {
	mut := types.NewMutableState("分析显式窗口内 CPU 供给")
	witness := types.TraceFrequencyLimitAuthority{
		CPU: 0, MinFrequencyKHz: 418000, MaxFrequencyKHz: 1530000,
		LimitRowCount: 16, WitnessLine: 8048, WitnessTs: 13762.861720,
		WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
		Authority: "direct_in_window_policy_limit",
	}
	for i := 0; i < 2; i++ {
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName: "trace_query",
			Success:  true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				View:                    "window_stats",
				FrequencyLimitWitnesses: []types.TraceFrequencyLimitAuthority{witness},
			},
		})
	}
	ctx := &types.AgentContext{Mutable: mut}

	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"Runtime direct frequency-limit authority",
		"`cpu=0 min=418000kHz max=1530000kHz limit_rows=16 witness_line=8048 witness_ts=13762.861720 window=13762.791708..13763.024898 authority=direct_in_window_policy_limit`",
		"`policy_limit_status=present`",
		"`target_binding_status=unproven_without_slice_overlap_or_binding_carrier`",
		"the direct yes/no conclusion is governed by `target_binding_status`, not by policy-ceiling presence",
		"`policy ceiling present; target binding unproven`",
		"do not turn that pair into an affirmative `the target was frequency-restricted`",
		"actual/average/residency frequency below that ceiling does not negate the policy limit",
		"`min_frequency_khz` is the lower policy bound and `max_frequency_khz` is the upper policy ceiling",
		"must not be described as equal to the maximum",
		"whether that ceiling bound this target's running slices remains unproven",
		"neither that lower observed frequency nor the limit row proves that the workload hit the ceiling",
		"does not by itself identify the lower-frequency cause or prove governance binding",
		"Normalize every frequency comparison to one unit",
		"`cpu_scope=dominant_state_slice_representative_not_exclusive`",
		"Never call that CPU the target's only CPU",
		"enumerate every available target-owned CPU row",
		"A policy row for one CPU binds only to target running evidence on that same CPU",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("frequency guidance missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "cpu=0 min=418000kHz max=1530000kHz") != 1 {
		t.Fatalf("duplicate typed witness was not deduplicated:\n%s", got)
	}
}

func TestRuntimeTraceGuidanceDoesNotInventFrequencyLimitWithoutTypedWitness(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("分析 trace")}
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	if strings.Contains(got, "Runtime direct frequency-limit authority") {
		t.Fatalf("frequency-limit guidance appeared without typed authority:\n%s", got)
	}
}
