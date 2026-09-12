package tool

import (
	"encoding/json"
	"github.com/hanchaoqun/codrax/internal/types"
	"reflect"
	"testing"
)

func TestTraceMemberStateDisplayRetainsEachWindowBeforeRepeatB1626(t *testing.T) {
	var in []types.TraceTargetStateScopeAuthority
	for i, ordinal := range []int{1, 1, 1, 1, 2, 0} {
		role := types.TraceQueryWindowScopeRequestedPrincipal
		if ordinal == 0 {
			role = types.TraceQueryWindowScopeSupportingExploration
		}
		in = append(in, types.TraceTargetStateScopeAuthority{ArtifactKey: "capture", Subject: "worker", RunningMS: float64(i), WindowScope: types.TraceQueryWindowScope{RequestedWindowCount: 2, RequestedWindowOrdinal: ordinal, Role: role}})
	}
	before, _ := json.Marshal(in)
	got := runtimeTraceMemberStateDisplayOrder(in)
	var indices []int
	for _, state := range got {
		indices = append(indices, int(state.RunningMS))
	}
	if !reflect.DeepEqual(indices, []int{0, 4, 1, 2, 3, 5}) {
		t.Fatalf("A repeated results consumed B display slot or rows vanished: %v", indices)
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("display ordering edited state evidence")
	}
}
