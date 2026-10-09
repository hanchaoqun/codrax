package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceTeachingScopeUsesTypedQuestionNotWindowOrProse(t *testing.T) {
	r := skill.NewRegistry()
	skill.RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name              string
		profile           *types.RuntimeQuestionProfile
		report, scheduler bool
	}{
		{"legacy", nil, true, true},
		{"diagnosis", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}, true, true},
		{"relation", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeRelationAnalysis}, false, true},
		{"overview", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeSystemOverview}, false, true},
		{"inventory", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOccurrenceTime, types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOtherObservedValue}}, false, false},
		{"state", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState}}, false, true},
		{"effect_frequency", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedEffectVerdict, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactFrequencyResidency}}, false, false},
		{"effect_state", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedEffectVerdict, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState}}, false, true},
		{"effect_frequency_state", &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedEffectVerdict, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactFrequencyResidency, types.RuntimeQuestionFactTargetSchedulerState}}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end := 2.0, 2.25
			ac := &types.AgentContext{AttachedHitraceSource: "capture.trace", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				RuntimeQuestionProfile:      tc.profile,
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end},
			}}}
			ctx := buildAppliesToContext(ac)
			if ctx.HasTraceReport != tc.report || ctx.HasTraceScheduler != tc.scheduler {
				t.Fatalf("wrong teaching scope: %+v", ctx)
			}
			body := strings.Join(skillTierAwareWorkflow(ac, sk), "\n")
			for label, want := range map[string]bool{"TRACE ANSWER SKELETON:": tc.report, "ROOT-CAUSE BOARD ORDER:": tc.report, "STATE-DURATION CALIBER SEPARATION:": tc.scheduler, "PROSE NUMBER GROUNDING:": true, "INFERRED ATTRIBUTION DISCLOSURE:": true, "CHANNEL WORDS PER ROW:": true} {
				if strings.Contains(body, label) != want {
					t.Errorf("%s present=%v want=%v", label, !want, want)
				}
			}
			// A precise retry restores the relevant rule even on a finite task.
			ctx.RetryViolations = []types.ViolationKind{types.ViolProseLexiconBoardInconsistent}
			for _, item := range sk.WorkflowTierB {
				if strings.HasPrefix(item.Body, "ROOT-CAUSE BOARD ORDER:") && !item.ShouldRender(ctx) {
					t.Fatal("retry teaching was suppressed")
				}
			}
		})
	}
	custom := &skill.Config{Workflow: []string{"custom text stays byte-identical"}, WorkflowTierB: []skill.TierBItem{{Body: "custom trace rule", AppliesTo: skill.AppliesToFilter{RequiresTrace: true}}}}
	got := strings.Join(skillTierAwareWorkflow(&types.AgentContext{AttachedHitraceSource: "capture.trace"}, custom), "\n")
	if got != "custom text stays byte-identical\ncustom trace rule" {
		t.Fatal("changed custom skill fallback")
	}
}
