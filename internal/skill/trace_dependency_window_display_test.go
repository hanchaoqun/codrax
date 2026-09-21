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
	for _, want := range []string{"actual_window is the all-state envelope", "not the dominant state's continuous interval", "precise state occurrences require a same-source thread_timeline interval", "state_drilldown is a cumulative state measurement scope"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing window meaning %q", want)
		}
	}
	if strings.Contains(body, "actual_impact_ms/actual_total_ms/actual_window describe the underlying scheduler state segment") {
		t.Fatal("aggregate envelope is still taught as a single state interval")
	}
	if strings.Contains(body, "precise state occurrences come from state_drilldown/thread_timeline") {
		t.Fatal("cumulative drilldown is still taught as an occurrence source")
	}
}
