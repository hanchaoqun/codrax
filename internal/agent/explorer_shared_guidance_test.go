package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the messages sent to the ReAct loop, not just either prompt producer.
func TestExplorerInitialMessages_TraceMatrixSingleOwner(t *testing.T) {
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	defaultSkill, err := registry.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	matrix := skill.RenderTraceQueryViewMatrix()
	clone := func() *skill.Config {
		copy := *defaultSkill
		copy.WorkflowTierB = append([]skill.TierBItem(nil), defaultSkill.WorkflowTierB...)
		return &copy
	}
	ownerIndex := -1
	for i, item := range defaultSkill.WorkflowTierB {
		if strings.Contains(item.Body, matrix) {
			ownerIndex = i
		}
	}
	if ownerIndex < 0 {
		t.Fatal("default skill must retain the complete trace view matrix")
	}
	renamed := clone()
	renamed.Name = "custom-cloned-default"
	replaced := clone()
	replaced.WorkflowTierB[ownerIndex].Body = "CUSTOM TRACE RULE: retain artifact-local evidence."
	filtered := clone()
	filtered.WorkflowTierB[ownerIndex].AppliesTo.RequiresDiagram = true

	for _, tc := range []struct {
		name string
		sk   *skill.Config
		role string
	}{
		{"default", defaultSkill, "system"},
		{"renamed_clone", renamed, "system"},
		{"custom", &skill.Config{Name: "custom"}, "user"},
		{"custom_default_name", &skill.Config{Name: defaultSkill.Name}, "user"},
		{"replaced_owner_body", replaced, "user"},
		{"filtered_owner_rule", filtered, "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := traceQueryFirstTestContext(traceQueryFirstRuntimeRequestModel(), nil)
			base := NewBaseAgent(types.AgentExplorer, &Dependencies{}, &explorerEvaluator{})
			messages := base.buildInitialMessages(ctx, tc.sk)
			assertTraceMatrixOwner(t, messages, matrix, tc.role)
			// Tool-history pressure must not erase the shared contract on later rounds.
			messages = append(messages, llm.Message{Role: "tool", Content: strings.Repeat("old tool row", 100)})
			if !pruneToolHistory(messages, 1) {
				t.Fatal("test must exercise actual tool-history pruning")
			}
			assertTraceMatrixOwner(t, messages, matrix, tc.role)
		})
	}

	// Reusing the evaluator for subsequent dispatches must not cache an owner
	// from the preceding default/custom dispatch.
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{}, &explorerEvaluator{})
	for i, sk := range []*skill.Config{defaultSkill, replaced, defaultSkill} {
		ctx := traceQueryFirstTestContext(traceQueryFirstRuntimeRequestModel(), nil)
		ctx.Objective = fmt.Sprintf("runtime artifact dispatch %d", i)
		role := "system"
		if i == 1 {
			role = "user"
		}
		assertTraceMatrixOwner(t, base.buildInitialMessages(ctx, sk), matrix, role)
	}
	// The matrix remains in the system instruction when a same-task retry
	// switches to the existing gap-filling supplement instead of trace start.
	eval := &explorerEvaluator{}
	base = NewBaseAgent(types.AgentExplorer, &Dependencies{}, eval)
	ctx := traceQueryFirstTestContext(traceQueryFirstRuntimeRequestModel(), nil)
	assertTraceMatrixOwner(t, base.buildInitialMessages(ctx, defaultSkill), matrix, "system")
	eval.investigationNotes = []string{"bounded trace probe completed"}
	assertTraceMatrixOwner(t, base.buildInitialMessages(ctx, defaultSkill), matrix, "system")
}

func assertTraceMatrixOwner(t *testing.T, messages []llm.Message, matrix, role string) {
	t.Helper()
	count := 0
	for _, message := range messages {
		n := strings.Count(message.Content, matrix)
		count += n
		if n > 0 && message.Role != role {
			t.Errorf("matrix owned by %q message, want %q", message.Role, role)
		}
	}
	if count != 1 {
		t.Fatalf("initial/later messages contain %d complete matrices (%d bytes each), want exactly one", count, len(matrix))
	}
}

func TestExplorerInitialMessages_ExcludedSourceNavigation(t *testing.T) {
	rm := traceQueryFirstRuntimeRequestModel()
	rm.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:         []string{"只分析 trace"},
		Confidence:           0.9,
	}
	// A stale explanatory profile must not revive the excluded source lane.
	rm.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
		SourceQuotes:                        []string{"attached.systrace"},
		Confidence:                          0.9,
	}
	if rm.CurrentSourceLaneDecision() != types.CurrentSourceLaneExcluded {
		t.Fatal("fixture must resolve to typed source exclusion")
	}
	ctx := traceQueryFirstTestContext(rm, nil)
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{}, &explorerEvaluator{})
	messages := base.buildInitialMessages(ctx, &skill.Config{Name: "custom"})
	dynamic := messages[len(messages)-1].Content
	for _, contradictory := range []string{
		"use a focused source follow-up",
		"Then use source-owner tools only if",
		"If you later need current-code proof",
		"Emit one `emit_evidence(items=[...])` batch only for real current-source anchors",
		"Current-Source Mechanism Coverage Ladder",
	} {
		if strings.Contains(dynamic, contradictory) {
			t.Errorf("excluded-source dynamic instruction still suggests %q", contradictory)
		}
	}
	for _, required := range []string{
		"Current-source inspection is excluded",
		"artifact-local",
		"time_start",
		"emit_investigation_complete",
		skill.RenderTraceQueryViewMatrix(),
	} {
		if !strings.Contains(dynamic, required) {
			t.Errorf("excluded-source dynamic instruction lost %q", required)
		}
	}
}

func TestExplorerInitialMessages_MixedSourceNavigationPreserved(t *testing.T) {
	for _, soft := range []bool{false, true} {
		t.Run(fmt.Sprintf("soft_profile_%v", soft), func(t *testing.T) {
			rm := traceQueryFirstRuntimeRequestModel()
			if soft {
				rm.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
					SourceQuotes:                        []string{"current parser mechanism"},
					Confidence:                          0.9,
				}
			}
			ctx := traceQueryFirstTestContext(rm, nil)
			base := NewBaseAgent(types.AgentExplorer, &Dependencies{}, &explorerEvaluator{})
			messages := base.buildInitialMessages(ctx, &skill.Config{Name: "custom"})
			dynamic := messages[len(messages)-1].Content
			if strings.Contains(dynamic, "Current-source inspection is excluded") || !strings.Contains(dynamic, "use a focused source follow-up") || !strings.Contains(dynamic, "Then use source-owner tools only if") {
				t.Fatal("non-excluded mixed scope must retain focused source follow-up")
			}
			if soft && !strings.Contains(dynamic, "Current-Source Mechanism Coverage Ladder") {
				t.Fatal("requested mechanism scope must retain its source coverage guidance")
			}
			assertTraceMatrixOwner(t, messages, skill.RenderTraceQueryViewMatrix(), "user")
		})
	}
}
