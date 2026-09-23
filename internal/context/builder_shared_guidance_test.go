package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSharedGuidanceOwnership_UsesActualWorkflowVisibility(t *testing.T) {
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	sk, err := registry.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	matrix := skill.RenderTraceQueryViewMatrix()
	ownerIndex := -1
	for i, item := range sk.WorkflowTierB {
		if item.ProvidesSharedGuidance(skill.TraceQueryViewMatrixGuidance) {
			ownerIndex = i
		}
	}
	if ownerIndex < 0 {
		t.Fatal("default owner missing")
	}
	for _, tc := range []struct {
		name    string
		ctx     *types.AgentContext
		visible bool
	}{
		{"nil", nil, false},
		{"no_trace", &types.AgentContext{Stage: types.StageExplore}, false},
		{"attached_trace", &types.AgentContext{Stage: types.StageExplore, AttachedHitraceSource: "trace.systrace"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := SkillWorkflowProvidesSharedGuidance(tc.ctx, sk, skill.TraceQueryViewMatrixGuidance)
			visible := strings.Contains(strings.Join(skillTierAwareWorkflow(tc.ctx, sk), "\n"), matrix)
			if owner != tc.visible || owner != visible {
				t.Fatalf("owner=%v actual visible=%v expected=%v", owner, visible, tc.visible)
			}
		})
	}
	// Retry admission can render a rule whose normal applicability is false.
	copy := *sk
	copy.WorkflowTierB = append([]skill.TierBItem(nil), sk.WorkflowTierB...)
	copy.WorkflowTierB[ownerIndex].OnViolation = []types.ViolationKind{types.ViolDiagramEdgeLabelMismatch}
	mut := types.NewMutableState("retry")
	mut.SetRetryState(&types.RetryState{ActiveViolations: []types.ScoredViolation{{Kind: types.ViolDiagramEdgeLabelMismatch}}})
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mut}
	if !SkillWorkflowProvidesSharedGuidance(ctx, &copy, skill.TraceQueryViewMatrixGuidance) || !strings.Contains(strings.Join(skillTierAwareWorkflow(ctx, &copy), "\n"), matrix) {
		t.Fatal("retry-visible owner and actual rendered matrix must agree")
	}
	if SkillWorkflowProvidesSharedGuidance(ctx, nil, skill.TraceQueryViewMatrixGuidance) {
		t.Fatal("nil skill cannot own guidance")
	}
}
