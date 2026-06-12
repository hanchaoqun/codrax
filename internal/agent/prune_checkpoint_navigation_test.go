package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func navigationCheckpointTestContext(stage types.PipelineStage) *types.AgentContext {
	mut := types.NewMutableState("which payment handlers exist")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active: true,
		Scopes: []string{"internal/payments"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFile,
			Complete: true,
			Count:    2,
			Members: []types.SourceInventoryObservationMember{
				{Name: "card.go", Key: "internal/payments/card.go"},
				{Name: "wallet.go", Key: "internal/payments/wallet.go"},
			},
		}},
	})
	ir := &types.AnalysisIR{}
	ir.RequestModel.AnalyzerHints.ExactTargets = []string{"PaymentGateway"}
	return &types.AgentContext{Stage: stage, Mutable: mut, AnalysisIR: ir}
}

// TestPruneCheckpointCarriesNavigationDigest pins A3: after tool-history
// pruning, the checkpoint re-injects the typed repo_map route plan and the
// already-observed candidate-universe summary so post-prune windows do not
// re-invoke broad navigation.
func TestPruneCheckpointCarriesNavigationDigest(t *testing.T) {
	got := buildToolHistoryPruneCheckpoint(navigationCheckpointTestContext(types.StageExplore))
	if !strings.HasPrefix(got, toolHistoryPruneCheckpointPrefix) {
		t.Fatalf("checkpoint prefix missing:\n%s", got)
	}
	for _, want := range []string{
		"Navigation context already computed",
		"Suggested exact query surfaces: `PaymentGateway`",
		"Recommended repo_map lens order:",
		`view="task_map"`,
		"Candidate universe already observed (role=file, 2 member(s), complete for the observed scope)",
		"`internal/payments`",
		"`card.go`, `wallet.go`",
		"Reuse this checklist instead of re-listing.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint missing %q:\n%s", want, got)
		}
	}
}

// TestPruneCheckpointNavigationDigestScopedToInvestigationWindows pins the
// precise stage gate: windows that cannot run navigation tools get no digest.
func TestPruneCheckpointNavigationDigestScopedToInvestigationWindows(t *testing.T) {
	got := buildToolHistoryPruneCheckpoint(navigationCheckpointTestContext(types.StageExtract))
	if strings.Contains(got, "Navigation context already computed") {
		t.Fatalf("navigation digest leaked into a non-investigation window:\n%s", got)
	}
}

// TestPruneCheckpointNavigationDigestIsStableForReinjectionHash pins that the
// digest is deterministic for identical typed state, so the existing
// stable-hash re-injection mechanics replace the checkpoint in place instead
// of appending a new message each prune pass.
func TestPruneCheckpointNavigationDigestIsStableForReinjectionHash(t *testing.T) {
	a := buildToolHistoryPruneCheckpoint(navigationCheckpointTestContext(types.StageExplore))
	b := buildToolHistoryPruneCheckpoint(navigationCheckpointTestContext(types.StageExplore))
	if a != b {
		t.Fatalf("navigation digest is not deterministic:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}
