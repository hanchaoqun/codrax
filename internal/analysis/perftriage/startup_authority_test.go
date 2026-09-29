package perftriage

import (
	"github.com/hanchaoqun/codrax/internal/types"
	"testing"
)

func TestMergeStartupAuthorityPrecedesMagnitudeAndOrder(t *testing.T) {
	verified := &types.PerfBundle{Startup: &types.PerfStartup{Authority: types.PerfObservationAuthorityDeterministicValidator, Mode: "warm", AppLaunchMs: 20}}
	model := &types.PerfBundle{Startup: &types.PerfStartup{Authority: types.PerfObservationAuthorityPreTriageModelExtraction, Mode: "cold", AppLaunchMs: 9999}}
	for _, parts := range [][]*types.PerfBundle{{model, verified}, {verified, model}} {
		got := MergePerfBundles(parts, 0)
		if got.Startup != verified.Startup || got.IntentHint != "" {
			t.Fatalf("larger model record displaced source authority: %+v", got)
		}
	}
	got := MergePerfBundles([]*types.PerfBundle{model}, 0)
	if got.Startup != model.Startup || got.HasAuthoritativeStartup() || got.IntentHint != "" || len(got.Entities) != 0 {
		t.Fatalf("model startup promoted on merge: %+v", got)
	}
}
