package skill

import (
	"strings"
	"testing"
)

func TestB1626AnalysisSkillExplainsMemberWindowsWithoutEnvelopeAuthority(t *testing.T) {
	config := BuildAnalysisSkill()
	if !strings.Contains(config.OutputFormat, AnalysisRuntimeWindowMembersTeaching) {
		t.Fatal("real analyzer skill lacks the shared typed member protocol")
	}
	for _, want := range []string{"time_windows", "ordered array", "source_quote", "Do not also emit scalar endpoints", "overlapping, nested, or repeated", "never replace them with their envelope", "no capture/target binding"} {
		if !strings.Contains(AnalysisRuntimeWindowMembersTeaching, want) {
			t.Errorf("member teaching lacks %q", want)
		}
	}
}

func TestHMC225ScopeTeachingHasOneSourceShapeDecision(t *testing.T) {
	contract := BuildAnalysisSkill().OutputFormat
	if !strings.Contains(contract, AnalysisRuntimeArtifactScopeTeaching) || strings.Count(contract, AnalysisRuntimeComparisonTeaching) != 1 {
		t.Fatal("analyzer scope branches are missing or duplicated")
	}
	for _, want := range []string{"Without comparison", "single-capture", "within that same capture"} {
		if !strings.Contains(AnalysisRuntimeWindowMembersTeaching, want) {
			t.Errorf("pooled windows lack single-source qualification %q", want)
		}
	}
	for _, want := range []string{"otherwise unspecified", "source_quote outside comparison", "Preserve valid comparison members"} {
		if !strings.Contains(AnalysisRuntimeComparisonTeaching, want) {
			t.Errorf("comparison teaching lost range/identity separation %q", want)
		}
	}
}
