package skill

import (
	"strings"
	"testing"
)

func TestAnalysisDimensionProvenanceTeachingExplainsDependentRoleRepair(t *testing.T) {
	output := BuildAnalysisSkill().OutputFormat
	if strings.Contains(output, "Unanchored dimensions become advisory warnings instead of analysis retries.") {
		t.Fatal("dimension teaching still promises no retry when provenance loss removes a required role")
	}
	for _, want := range []string{
		"Unanchored dimension rows are dropped with warnings, not rejected on their own",
		"If that leaves a required typed role missing",
		"repair that row's verbatim source_quote",
		"preserving the model-chosen role/required values and the other requested dimensions",
	} {
		if strings.Count(output, want) != 1 {
			t.Errorf("analysis output must teach the provenance/dependent-contract distinction once: missing or duplicated %q", want)
		}
	}
}
