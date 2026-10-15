package skill

import (
	"strings"
	"testing"
)

func TestHMC248CatalogTeachingSeparatesSamplingExhaustiveAndRecovery(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	var catalog []string
	for _, line := range sk.Workflow {
		if strings.HasPrefix(line, "For a directory of captures") {
			catalog = append(catalog, line)
		}
	}
	if len(catalog) != 1 {
		t.Fatalf("expected one catalog workflow, got %d", len(catalog))
	}
	for _, want := range []string{"For a requested sample comparison", "action=representatives", "representative_status", "For exhaustive checks", "query_calls unchanged", "Catalog metadata and historical snapshots are navigation", "Historical success does not replace a fresh query", "not a physical thread or a common cause"} {
		if !strings.Contains(catalog[0], want) {
			t.Errorf("catalog teaching lost %q", want)
		}
	}
	if strings.Contains(catalog[0], "Then call trace_query separately with each returned exact source") {
		t.Fatal("unconditional exhaustive execution conflicts with a requested sample")
	}
}
