package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfThresholdProvenanceKeepsPreTriageFrameVerdictUnprovenAA4(t *testing.T) {
	mut := types.NewMutableState("比较 trace 帧时长并解释当前实现")
	mut.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Frames: []types.PerfFrame{
			{FrameNo: 1, TsMs: 1000100, DurationMs: 86.111, Janky: true},
			// The model bit alone cannot mint validator authority below the
			// deterministic budget.
			{FrameNo: 2, TsMs: 1000200, DurationMs: 8, Janky: true},
		},
	})
	ctx := &types.AgentContext{Mutable: mut}

	got := renderAnswerDocPerfThresholdProvenanceAuthority(ctx)
	for _, want := range []string{
		"Perf Frame Verdict Authority",
		"does not carry a validator-owned device refresh rate, frame deadline, or frame budget",
		"duration_ms=86.111; jank_verdict_authority=`pretriage_model_extraction`",
		"not a deterministic jank/non-jank verdict",
		"leave the verdict unproven",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("perf threshold provenance missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "duration_ms=8.000; jank_verdict_authority=`pretriage_model_extraction`") {
		t.Fatalf("all model-supplied janky bits must retain the same pre-triage ceiling regardless of duration:\n%s", got)
	}
	for _, forbidden := range []string{"validator_jank_budget_ms", "deterministic_validator_constant", "validator_janky=true"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("default frame-budget authority leaked through %q:\n%s", forbidden, got)
		}
	}
}
