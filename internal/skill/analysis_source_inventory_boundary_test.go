package skill

import (
	"strings"
	"testing"
)

func TestAnalysisSkillConceptualMemberSetStaysOutsideSourceInventory(t *testing.T) {
	cfg := BuildAnalysisSkill()
	if cfg == nil {
		t.Fatal("BuildAnalysisSkill returned nil")
	}
	for _, want := range []string{
		"A bounded conceptual member set is also not a source inventory",
		"responsibilities, transitions, handoffs, or a mechanism diagram",
		"source declarations are supporting evidence, not the answer universe",
		"principal answer rows are the source declarations or constructs themselves",
	} {
		if !strings.Contains(cfg.OutputFormat, want) {
			t.Fatalf("analysis output contract missing conceptual/source-inventory boundary %q", want)
		}
	}

	workflow := strings.Join(cfg.Workflow, "\n")
	for _, want := range []string{
		"bounded mechanism asks for conceptual stages/phases/steps/modes/components",
		"relation/mechanism member-set answers",
		"keep the known conceptual members in entities",
		"principal answer rows are the source declarations or constructs themselves",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("analysis workflow missing conceptual/source-inventory decision shortcut %q", want)
		}
	}
}

func TestAnalysisSkillConceptualMemberSetGuidanceDoesNotCreateHardProseGate(t *testing.T) {
	cfg := BuildAnalysisSkill()
	corpus := cfg.OutputFormat + "\n" + strings.Join(cfg.Workflow, "\n")
	for _, forbidden := range []string{
		"scan the user's prose",
		"reject when the request contains",
		"hard reject conceptual",
		"rewrite the answer",
	} {
		if strings.Contains(strings.ToLower(corpus), strings.ToLower(forbidden)) {
			t.Fatalf("soft classification guidance introduced forbidden hard/prose ownership phrase %q", forbidden)
		}
	}
}
