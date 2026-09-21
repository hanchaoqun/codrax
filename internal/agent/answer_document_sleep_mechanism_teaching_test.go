package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual initial message path, not just the dynamic supplement:
// the conflicting skeleton lived in the registered skill's system message.
// The public query fixture independently proves whether S/D closes an IO wait.
func TestTraceSleepMechanismTeachingInActualFinalizerMessages(t *testing.T) {
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	cfg, err := registry.Get("answer-document-skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			state string
			wake  bool
		}{{"S", true}, {"D", true}, {"S", false}} {
			t.Run(fmt.Sprintf("%s/%s/closed=%t", lang, tc.state, tc.wake), func(t *testing.T) {
				ctx := b1645ActualCausalIOContext(t, tc.state, lang, tc.wake)
				ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
				ctx.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
					SourceNavigationOptional: true,
					Artifacts:                []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "completion.ftrace", Carrier: "request_path"}},
				})
				before := b1645Snapshot(t, ctx.Mutable.TurnAArtifacts())
				modelBefore := b1645Snapshot(t, ctx.Mutable.AnswerDocumentV2())
				assembler := DefaultPromptAssembler()
				messages := assembler.RenderMessages(assembler.AssembleContext(ctx, cfg))
				messages = AppendDynamicInstruction(messages, &answerDocumentEvaluator{}, ctx, cfg)
				var rendered strings.Builder
				for _, message := range messages {
					rendered.WriteString(message.Content)
					rendered.WriteByte('\n')
				}
				prompt := rendered.String()
				if !strings.Contains(prompt, "TRACE ANSWER SKELETON:") || !strings.Contains(prompt, "## Final Trace Decision Boundary") {
					t.Fatal("test must reach both the registered trace skill and actual finalizer supplement")
				}
				for _, want := range []string{
					"separate measured scheduler-state occupancy from proven dependencies and unresolved candidates",
					"S alone proves no wait mechanism",
					"an absent IO marker does not prove non-IO waiting",
					"calling a wait normal or cooperative requires independent business/protocol evidence",
					"③ Then the top eliminable causes with their repair directions",
					"④ Point everything else at the report's own deterministic faces",
					"request_residence=`0.100`",
					fmt.Sprintf("completion_woke_issuer=`%t`", tc.wake),
				} {
					if !strings.Contains(prompt, want) {
						t.Errorf("actual finalizer messages missing %q", want)
					}
				}
				if strings.Contains(prompt, "which waiting is designed-in cooperation") {
					t.Error("system skeleton still presupposes a normal/cooperative wait mechanism")
				}
				if tc.wake != strings.Contains(prompt, "issuer_blocked=`0.090`") {
					t.Fatal("teaching must neither lose a proven S/D IO wait nor invent closure without a wakeup")
				}
				if before != b1645Snapshot(t, ctx.Mutable.TurnAArtifacts()) || modelBefore != b1645Snapshot(t, ctx.Mutable.AnswerDocumentV2()) {
					t.Fatal("prompt teaching mutated evidence or the model-owned answer")
				}
			})
		}
	}
}

func TestTraceSleepMechanismSkeletonRemainsTraceOnlySoftTeaching(t *testing.T) {
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	cfg, err := registry.Get("answer-document-skill")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range cfg.WorkflowTierB {
		if strings.HasPrefix(item.Body, "TRACE ANSWER SKELETON:") {
			found = true
			if !item.AppliesTo.RequiresTrace || len(item.OnViolation) != 0 {
				t.Fatal("the skeleton must remain trace-only teaching, not a new reject or retry condition")
			}
		}
	}
	if !found {
		t.Fatal("trace skeleton missing")
	}
	ctx := &types.AgentContext{AgentName: types.AgentFinalizer, Stage: types.StageFinalize}
	assembler := DefaultPromptAssembler()
	for _, message := range assembler.RenderMessages(assembler.AssembleContext(ctx, cfg)) {
		if strings.Contains(message.Content, "TRACE ANSWER SKELETON:") {
			t.Fatal("trace-only wait teaching leaked into non-trace composition")
		}
	}
}
