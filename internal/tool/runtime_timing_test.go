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
