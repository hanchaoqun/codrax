package types

import "testing"

// TestCompileRequiredMechanismAnchors_FiltersProjectNames is the F1
// regression for the 2026-05-16 architectural-comparison finalize
// loop. The analyzer's MentionedEntities include "codrax" /
// "opencode" — repository names that look identifier-shaped to
// InferContractTermKind but have no code definition site. Without
// the sub-repo-name filter, CompileRequiredMechanismAnchors promotes
// them into RequiredMechanismAnchor; the pre-emit normaliser then
// auto-injects them as item labels in `required_mechanism_anchors`,
// and the post-emit `enumeration_label_ungrounded` oracle rejects
// the auto-injected labels because they match no evidence anchor or
// QuestionBucket. The result is a same-error-class retry loop that
// burns the finalizer budget and falls back to raw content with
// cited=0.
func TestCompileRequiredMechanismAnchors_FiltersProjectNames(t *testing.T) {
	rm := RequestModel{
		RawRequest: "对比一下 codrax 和 opencode 在防止模型幻觉方面各有什么优缺点？",
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"codrax", "opencode", "Orchestrator"},
		},
	}
	contract := AnswerContract{}

	// Without sub-repo filter (single-repo posture / no MultiGraph
	// wired): legacy behaviour — entities flow through.
	got := CompileRequiredMechanismAnchors(rm, contract, QFArchitecture, nil)
	hasCodrax := false
	hasOpencode := false
	hasOrch := false
	for _, a := range got {
		switch a.Text {
		case "codrax":
			hasCodrax = true
		case "opencode":
			hasOpencode = true
		case "Orchestrator":
			hasOrch = true
		}
	}
	if !hasCodrax || !hasOpencode {
		t.Fatalf("baseline (no sub-repo filter) should keep project names: %+v", got)
	}
	if !hasOrch {
		t.Fatalf("baseline should include real symbol Orchestrator: %+v", got)
	}

	// With sub-repo filter (multi-repo posture, codrax + opencode
	// active in topology): project names dropped, real symbol kept.
	got = CompileRequiredMechanismAnchors(rm, contract, QFArchitecture,
		[]string{"codrax", "opencode", "claude/codrax", "small/codrax-small"})
	for _, a := range got {
		if a.Text == "codrax" || a.Text == "opencode" {
			t.Errorf("project-name %q must be filtered out, got %+v", a.Text, got)
		}
	}
	foundOrch := false
	for _, a := range got {
		if a.Text == "Orchestrator" {
			foundOrch = true
		}
	}
	if !foundOrch {
		t.Errorf("real symbol Orchestrator must survive filter: %+v", got)
	}
}

func TestCompileRequiredMechanismAnchors_FilterIsCaseInsensitive(t *testing.T) {
	rm := RequestModel{
		RawRequest: "Codrax vs opencode",
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: AnalyzerHints{
			// Mixed case to verify requiredAnchorKey's ToLower normalises.
			MentionedEntities: []string{"Codrax", "OpenCode"},
		},
	}
	got := CompileRequiredMechanismAnchors(rm, AnswerContract{}, QFArchitecture,
		[]string{"codrax", "opencode"})
	if len(got) != 0 {
		t.Errorf("case variants of project names must be filtered, got %+v", got)
	}
}
