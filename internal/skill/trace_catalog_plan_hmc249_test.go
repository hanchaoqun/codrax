package skill

import (
	"strings"
	"testing"
)

func TestHMC249CatalogTeachingPlansBeforeSelectionWithoutMandatingRoute(t *testing.T) {
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
		plan, selection := strings.Index(paragraph, "first register queries"), strings.Index(paragraph, "action=representatives")
		if plan < 0 || selection <= plan {
			t.Fatal("representative teaching skipped required query-specification planning")
		}
		for _, want := range []string{"queries.items", "requested view", "task-supported object/window/filter", "does not reset attempts", "discovery never runs them", "Ordinary trace_query remains available", "representative_status", "Historical success does not replace a fresh query"} {
			if !strings.Contains(paragraph, want) {
				t.Errorf("missing plan/navigation boundary: %q", want)
			}
		}
		return
	}
	t.Fatal("catalog workflow absent")
}
