package tool

import (
	"strings"
	"testing"
)

func TestEmitAnalysisSchemaSeparatesSourceAndCandidateExclusions(t *testing.T) {
	schema := string((&EmitAnalysis{}).Parameters())
	for _, want := range []string{
		"evidence sources rather than candidate roles",
		"belongs only in external_observation_policy.current_source_mode=exclude",
		"never encode it as true with an empty role list",
		"only carrier for excluding source-code/current-checkout evidence",
		"do not duplicate that source boundary into answer_exclusion_policy",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("emit_analysis schema missing source/candidate exclusion boundary %q", want)
		}
	}
}
