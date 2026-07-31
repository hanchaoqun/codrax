package skill

import (
	"strings"
	"testing"
)

func TestTraceTargetStateAndCPUWideScopeRemainSeparatedAA1(t *testing.T) {
	item := finBindTierBItem(t, "TARGET THREAD VERSUS CPU SCOPE")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("target-vs-CPU scope guidance must be trace-gated: %+v", item.AppliesTo)
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("target-vs-CPU scope guidance must remain soft: %+v", item.OnViolation)
	}
	for _, want := range []string{
		"`target_window_states` partitions ONE target thread",
		"bounds only that target's scheduler queueing",
		"Neither value alone is CPU utilization",
		"separate typed per-CPU, core-class, process-domain, or system",
		"never rename target-thread running percentage as CPU utilization",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("target-vs-CPU scope guidance missing %q:\n%s", want, item.Body)
		}
	}
}
