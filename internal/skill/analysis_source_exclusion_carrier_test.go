package skill

import (
	"strings"
	"testing"
)

func TestAnalysisSkillRoutesSourceExclusionOnlyToExternalObservationPolicy(t *testing.T) {
	cfg := BuildAnalysisSkill()
	if cfg == nil {
		t.Fatal("BuildAnalysisSkill returned nil")
	}
	for _, want := range []string{
		"Source code, the repository, and the current checkout are EVIDENCE SOURCES, not principal answer candidate roles",
		"belongs exclusively in `external_observation_policy.current_source_mode=exclude`",
		"Never set `is_exclusion_requested=true` with an empty `excluded_candidate_roles` list",
		"the ONLY carrier for excluding source-code/current-checkout evidence",
		"do not duplicate that boundary into answer_exclusion_policy",
	} {
		if !strings.Contains(cfg.OutputFormat, want) {
			t.Fatalf("analysis contract missing source/candidate exclusion boundary %q", want)
		}
	}
}
