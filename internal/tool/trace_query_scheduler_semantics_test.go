package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryEvidenceAuthorityPublishesSchedulerStateAndPrioritySemantics(t *testing.T) {
	authority := traceQueryEvidenceAuthority(tracequery.Result{
		View:              "window_stats",
		PrioritySemantics: tracequery.PrioritySemanticsForFlavor(tracequery.TraceFlavorHarmonyHitrace),
	})
	if authority == nil {
		t.Fatal("trace evidence authority missing")
	}
	for _, want := range []string{
		"prev_state=S proves a sleeping/blocking transition",
		"not preemption or voluntary yield",
		"only R/R+ supports a still-runnable preemption candidate",
		"running-slice count is not wakeup count",
	} {
		if !strings.Contains(authority.SchedulerSemantics, want) {
			t.Fatalf("scheduler authority missing %q: %s", want, authority.SchedulerSemantics)
		}
	}
	for _, want := range []string{
		"larger numeric value means higher priority",
		"1-40=CFS",
		"41-159=RT",
		"above 159",
		"system_or_kernel/raw",
	} {
		if !strings.Contains(authority.PrioritySemantics, want) {
			t.Fatalf("priority authority missing %q: %s", want, authority.PrioritySemantics)
		}
	}
}
