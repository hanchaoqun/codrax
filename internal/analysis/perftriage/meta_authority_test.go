package perftriage

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMergeMetaKeepsAuthorityAheadOfMagnitudeAndMajority(t *testing.T) {
	model := &types.PerfBundle{Meta: types.PerfMeta{Source: "hitrace", DurationMs: 9999, AppPID: 999, Signals: []string{"render-miss"}}}
	verified := &types.PerfBundle{Meta: types.PerfMeta{Authority: types.PerfObservationAuthorityDeterministicValidator, Source: "systrace", DurationMs: 25, AppPID: 12, Signals: []string{"jank"}}}
	for _, parts := range [][]*types.PerfBundle{{model, verified, model}, {verified, model, model}} {
		got := MergePerfBundles(parts, 0)
		if !got.HasAuthoritativeMeta() || got.Meta.DurationMs != 25 || got.Meta.AppPID != 12 || got.Meta.Source != "systrace" || !reflect.DeepEqual(got.AuthoritativeSignals(), []string{"jank"}) {
			t.Fatalf("mixed authority laundered: %+v", got.Meta)
		}
	}
	got := MergePerfBundles([]*types.PerfBundle{nil, model}, 0)
	if got.HasAuthoritativeMeta() || got.Meta.DurationMs != 9999 || len(got.AuthoritativeSignals()) != 0 {
		t.Fatalf("legacy audit data lost/promoted: %+v", got.Meta)
	}
}
