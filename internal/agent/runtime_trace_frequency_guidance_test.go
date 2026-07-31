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
		"Actual/average/residency frequency and frequency-transition count are separate operating facts",
		"must not replace the direct min/max witness",
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
