package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC344RuntimeSearchRoleReachesRetryRouting(t *testing.T) {
	for _, origin := range []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginUnknown, types.AnswerEvidenceOriginCurrentSource, types.AnswerEvidenceOriginRuntimeArtifact} {
		r := types.ToolResult{ToolName: "grep", Success: true, PathDiscovery: &types.ToolPathDiscovery{InputOrigin: origin, Path: "opaque-result"}}
		if got := toolResultCarriesRuntimeArtifactSignal(r); got != (origin == types.AnswerEvidenceOriginRuntimeArtifact) {
			t.Fatalf("typed role lost or guessed from a filename: origin=%s got=%t", origin, got)
		}
	}
	page := types.ToolResult{ToolName: "read_file", Success: true, RuntimeArtifactRead: &types.ToolRuntimeArtifactRead{RequestedPath: "opaque-result"}}
	if !toolResultCarriesRuntimeArtifactSignal(page) {
		t.Fatal("reader's established runtime marker lost at retry routing")
	}
}
