package skill

import (
	"strings"
	"testing"
)

func TestTraceDependencyWindowSkillTeachingDoesNotClaimSingleState(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	body := allWorkflowBodies(sk)
	for _, want := range []string{"actual_window is the all-state envelope", "not the dominant state's continuous interval", "precise state occurrences come from state_drilldown/thread_timeline"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing window meaning %q", want)
		}
	}
	if strings.Contains(body, "actual_impact_ms/actual_total_ms/actual_window describe the underlying scheduler state segment") {
		t.Fatal("aggregate envelope is still taught as a single state interval")
	}
}
