package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceCPUIdentityGuideUsesTypedAttachmentAcrossStages(t *testing.T) {
	ctx := &types.AgentContext{
		AttachedHitrace: "raw-21 (20) [005] .... 3.003000: perf_sample: cpu=5 cpu_known=true",
	}
	for name, prompt := range map[string]string{
		"perf_triager": (&perfTriagerEvaluator{}).BuildInitialInstruction(ctx, nil),
		"analyzer":     prependAnswerPitfalls(ctx, "analyze"),
		"explorer":     (&explorerEvaluator{}).buildExplicitRuntimeTracePathStartInstruction(ctx),
	} {
		for _, want := range []string{
			"parenthesized `tgid`",
			"bracketed `[NNN]` field is the event-row CPU",
			"`cpu_known=true`",
			"Do not infer a CPU migration",
			"final conclusion model-authored",
		} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("%s prompt missing %q:\n%s", name, want, prompt)
			}
		}
	}
}

func TestRuntimeTraceCPUIdentityGuideAbsentWithoutTypedTraceCarrier(t *testing.T) {
	ctx := &types.AgentContext{}
	if got := (&perfTriagerEvaluator{}).BuildInitialInstruction(ctx, nil); got != "" {
		t.Fatalf("non-trace perf prompt=%q, want empty", got)
	}
	if got := prependAnswerPitfalls(ctx, "analyze"); got != "analyze" {
		t.Fatalf("non-trace analyzer prompt=%q", got)
	}
}
