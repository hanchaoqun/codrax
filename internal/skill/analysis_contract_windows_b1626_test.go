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
