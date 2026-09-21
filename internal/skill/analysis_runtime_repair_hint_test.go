package skill

import (
	"strings"
	"testing"
)

func TestRuntimeRepairHintTeachingKeepsClassifierConflictModelOwned(t *testing.T) {
	cfg := BuildAnalysisSkill()
	workflow := strings.Join(append([]string{cfg.Goal, cfg.OutputFormat}, cfg.Workflow...), "\n")
	for name, teaching := range map[string]string{"workflow": workflow, "schema": AnalysisRuntimeScopeSchemaTeaching} {
		t.Run(name, func(t *testing.T) {
			if strings.Count(teaching, AnalysisRuntimeConflictingClassifierRepairTeaching) != 1 {
				t.Fatal("shared classifier-conflict repair teaching must occur exactly once")
			}
			for _, want := range []string{
				"do not treat the missing causal role as proof that the request is finite",
				"required causal dimension beside the independent target_effect_verdict",
				"non-root-cause intent/scenario and false diagnostic flags",
				"Classifier labels alone never grant causal breadth",
				"Preserve independent dimensions and work/frame decisions",
			} {
				if !strings.Contains(teaching, want) {
					t.Errorf("missing repair boundary %q", want)
				}
			}
		})
	}
	if !strings.Contains(workflow, "Precedence shortcut for coherent non-root-cause classifiers:") {
		t.Fatal("finite precedence shortcut must state its cross-field coherence precondition")
	}
}
