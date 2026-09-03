package skill

import (
	"strings"
	"testing"
)

// QUALGATE-1 (user ruling §40.30 V-QUAL-1 plan A, 2026-09-02): the analysis
// skill teaches the dedicated typed frame decision with its negative rule —
// bare 卡顿 / stutter is not a frame question — and never lets exploration or
// answer prose derive it (the R2' teaching spot of the new analyzer field).
func TestAnalysisSkillTeachesFrameCausalityDecision(t *testing.T) {
	cfg := BuildAnalysisSkill()
	out := analysisSkillPrompt(t) + "\n" + strings.Join(cfg.Workflow, "\n")
	for _, want := range []string{
		"Also set the required boolean `frame_causality_requested`",
		"rendering frame / dropped or delayed frame / frame window / vsync / render-deadline outcome",
		"bare 卡顿 / stutter alone is not a frame question",
		"Decision procedure: if the request text names a frame as the diagnosed thing",
		"`帧窗口内的卡顿原因` → true",
		"`这个短窗口卡顿原因`",
		"never pre-decides whether frame causality is proven and is never derived from exploration or answer prose",
		// R2' parity: the required-field recap and the field roster name it too.
		"runtime_question_profile (scope, runtime_work_relation_requested, frame_causality_requested, confidence)",
		"object with scope, runtime_work_relation_requested, frame_causality_requested, fact_families, and confidence",
		"`runtime_work_relation_requested` and `frame_causality_requested` are independent model decisions",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("analysis skill frame-decision teaching missing %q", want)
		}
	}
}
