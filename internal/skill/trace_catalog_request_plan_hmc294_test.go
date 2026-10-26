package skill

import (
	"strings"
	"testing"
)

func TestHMC294CatalogTeachingSeparatesAcceptedWorkAndManualPlans(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	s, err := r.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range s.Workflow {
		if !strings.HasPrefix(p, "For a directory of captures") {
			continue
		}
		for _, want := range []string{"accepted artifact work", "request_plan", "explicit view", "Do not combine", "queries", "representatives", "manual", "filters", "targets", "window", "status and representative_status", "same-generation"} {
			if !strings.Contains(p, want) {
				t.Errorf("missing lane teaching %q", want)
			}
		}
		return
	}
	t.Fatal("catalog teaching absent")
}
