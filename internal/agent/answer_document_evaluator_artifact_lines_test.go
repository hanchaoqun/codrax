package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocReferencedArtifactLines(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{}}
	if got := renderAnswerDocReferencedArtifactLines(ctx); got != "" {
		t.Fatalf("no refs → empty section, got %q", got)
	}
	ctx.AnalysisIR.RequestModel.ReferencedArtifactLines = []types.ArtifactLineRef{
		{Source: "log", StartLine: 3, EndLine: 3},
		{Source: "trace", StartLine: 5, EndLine: 6},
	}
	got := renderAnswerDocReferencedArtifactLines(ctx)
	if !strings.Contains(got, "log line 3") || !strings.Contains(got, "trace lines 5-6") {
		t.Fatalf("section must list every referenced span, got %q", got)
	}
	if !strings.Contains(got, "NOT repository citations") {
		t.Fatalf("section must keep the two-rail rule, got %q", got)
	}
}
