package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SPR #72 pins (RTC ledger §8.3) — the post-emit contract gate must not
// demand a current-status verdict, nor consume a side-picked one as a
// satisfied decision signal, when the persist-time stamp records zero
// current_source lane evidence. Mutation contract: removing the
// downgrade early-out re-fires the missing-verdict violation on the
// synthetic zero-evidence doc and turns these red.

func currentStatusGateView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
	}
}

func TestValidateCurrentStatusVerdict_DowngradeWaivesObligation(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "d1",
			Kind:        types.BlockDecision,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "本轮没有源码证据，无法给出现状判定。",
		}},
		CurrentStatusVerdictDowngrade: &types.CurrentStatusVerdictDowngrade{
			Reason: types.CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
		},
	}
	if v := validateCurrentStatusVerdict(doc, currentStatusGateView()); len(v) != 0 {
		t.Fatalf("stamped zero-evidence run must not demand a verdict (no forced side-pick), got %+v", v)
	}
	// Side-picked verdict under the stamp: not consumed as a satisfied
	// decision signal — the gate stays inert either way, and the verdict
	// stays only as the audit record.
	doc.Blocks[0].CurrentStatusVerdict = types.CurrentStatusStillPresent
	doc.CurrentStatusVerdictDowngrade = &types.CurrentStatusVerdictDowngrade{
		BlockID:         "d1",
		OriginalVerdict: types.CurrentStatusStillPresent,
		Reason:          types.CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
	}
	if v := validateCurrentStatusVerdict(doc, currentStatusGateView()); len(v) != 0 {
		t.Fatalf("stamped side-picked verdict must not produce violations, got %+v", v)
	}
}

func TestValidateCurrentStatusVerdict_EvidenceRunBehaviorUnchanged(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "d1",
			Kind:        types.BlockDecision,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "prose only",
		}},
	}
	v := validateCurrentStatusVerdict(doc, currentStatusGateView())
	if len(v) != 1 || v[0].Kind != types.ViolCurrentStatusVerdictMissing {
		t.Fatalf("unstamped missing verdict must keep the pre-SPR demand, got %+v", v)
	}
	doc.Blocks[0].CurrentStatusVerdict = types.CurrentStatusFixed
	if v := validateCurrentStatusVerdict(doc, currentStatusGateView()); len(v) != 0 {
		t.Fatalf("unstamped allowed verdict must keep passing, got %+v", v)
	}
}
