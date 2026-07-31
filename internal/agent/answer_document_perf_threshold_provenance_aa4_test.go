package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfThresholdProvenanceSeparatesValidatorBudgetFromUserComparatorAA4(t *testing.T) {
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
		"validator_jank_budget_ms=16.67",
		"authority=`deterministic_validator_constant`",
		"duration_ms=86.111; validator_janky=true",
		"different threshold supplied by the request",
		"must not be renamed as Codrax's internal jank rule",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("perf threshold provenance missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "duration_ms=8.000") {
		t.Fatalf("model-supplied janky=true below the validator budget must not receive deterministic threshold authority:\n%s", got)
	}
}
