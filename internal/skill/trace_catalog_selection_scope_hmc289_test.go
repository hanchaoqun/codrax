package skill

import (
	"strings"
	"testing"
)

// The workflow must agree with the tool's two distinct selection entry points.
// This is a teaching regression, not a runtime gate on user/model wording.
func TestHMC289CatalogTeachingScopesSelectionWithoutBlockingExploration(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, paragraph := range sk.Workflow {
		if !strings.HasPrefix(paragraph, "For a directory of captures") {
			continue
		}
		for _, want := range []string{
			"discover with explicit queries and representatives together",
			"only those query specifications",
			"entire current catalog",
			"auxiliary exploration",
			"does not reset attempts",
			"Ordinary trace_query remains available",
			"representative_status",
			"Historical success does not replace a fresh query",
		} {
			if !strings.Contains(paragraph, want) {
				t.Errorf("catalog workflow lost selection boundary: %q", want)
			}
		}
		return
	}
	t.Fatal("catalog workflow absent")
}
