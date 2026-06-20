package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func assertToolRuntimeTimingPhases(t *testing.T, timings []types.ToolRuntimeTiming, want ...string) {
	t.Helper()
	seen := map[string]types.ToolRuntimeTiming{}
	for _, timing := range timings {
		seen[timing.Phase] = timing
	}
	for _, phase := range want {
		timing, ok := seen[phase]
		if !ok {
			t.Fatalf("missing runtime timing phase %q in %+v", phase, timings)
		}
		if timing.ElapsedMillis < 0 {
			t.Fatalf("phase %q has negative elapsed: %+v", phase, timing)
		}
		if timing.Status != "success" {
			t.Fatalf("phase %q status=%q, want success", phase, timing.Status)
		}
	}
}

func TestFormatToolRuntimeTimingPhasesCompactsAndCaps(t *testing.T) {
	timings := []types.ToolRuntimeTiming{
		{Phase: "decode", ElapsedMillis: 3, Count: 1},
		{Phase: "preflight", ElapsedMillis: 1200, Count: 12},
		{Phase: "", ElapsedMillis: -1, Count: -2},
	}
	got := formatToolRuntimeTimingPhases(timings, 2)
	want := "decode:3ms/1,preflight:1200ms/12,+1"
	if got != want {
		t.Fatalf("timing summary = %q, want %q", got, want)
	}
}
