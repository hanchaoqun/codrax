package tracequery

import "testing"

func TestHarmonySchedulerSemanticsMatrixPinsStateAndPriorityBoundaries(t *testing.T) {
	states := []struct {
		token string
		want  ThreadState
	}{
		{token: "S", want: StateSSleep},
		{token: "R", want: StateRunnable},
		{token: "R+", want: StateRunnable},
	}
	priorities := []struct {
		value int
		want  string
	}{
		{value: 20, want: "ohos_cfs"},
		{value: 41, want: "ohos_rt"},
		{value: 159, want: "ohos_rt"},
		{value: 160, want: "system_or_kernel"},
	}
	for _, state := range states {
		for _, priority := range priorities {
			t.Run(state.token+"/prio", func(t *testing.T) {
				if got := stateFromPrevState(state.token); got != state.want {
					t.Fatalf("prev_state=%s classified as %q, want %q", state.token, got, state.want)
				}
				if got := classifyTracePriority(TraceFlavorHarmonyHitrace, priority.value); got != priority.want {
					t.Fatalf("prio=%d classified as %q, want %q", priority.value, got, priority.want)
				}
			})
		}
	}
}
