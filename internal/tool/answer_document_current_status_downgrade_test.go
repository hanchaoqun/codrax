package tool

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SPR #72 pins (RTC ledger §8.3) — persist-time stamping + pre-emit
// demand waiver. Mutation contract: removing the persist stamp call, the
// stale-stamp reset, or the zero-evidence demand waiver turns these red.

func newDowngradeDecisionDoc(verdict types.CurrentStatusVerdict) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:                   "d1",
			Kind:                 types.BlockDecision,
			SurfaceRole:          types.SurfacePrincipal,
			Text:                 "是。无法判断该问题在最新代码中是否已修复。",
			CurrentStatusVerdict: verdict,
		}},
	}
}

func busWithCurrentSourceEvidence() *types.BusContext {
	bus := newBusForMutationTest()
	bus.EvidenceItems = []types.EvidenceItem{{
		ID:        "e1",
		Source:    "internal/foo/bar.go",
		LineStart: 10,
		LineEnd:   12,
		Summary:   "guard present at call site",
	}}
	return bus
}

func TestApplyAndPersistMutation_StampsZeroEvidenceVerdictDowngrade(t *testing.T) {
	bus := newBusForMutationTest()
	doc := newDowngradeDecisionDoc(types.CurrentStatusStillPresent)
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist failed: %v / %+v", err, res)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || got.CurrentStatusVerdictDowngrade == nil {
		t.Fatal("zero-evidence run with side-picked verdict must persist the downgrade stamp")
	}
	d := got.CurrentStatusVerdictDowngrade
	if d.OriginalVerdict != types.CurrentStatusStillPresent || d.BlockID != "d1" ||
		d.Reason != types.CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence {
		t.Fatalf("stamp shape wrong: %+v", d)
	}
	// Audit position: the model-emitted verdict field survives untouched.
	if got.Blocks[0].CurrentStatusVerdict != types.CurrentStatusStillPresent {
		t.Fatalf("block verdict must stay untouched, got %q", got.Blocks[0].CurrentStatusVerdict)
	}
}

func TestApplyAndPersistMutation_EvidenceRunDoesNotStamp(t *testing.T) {
	bus := busWithCurrentSourceEvidence()
	doc := newDowngradeDecisionDoc(types.CurrentStatusStillPresent)
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist failed: %v / %+v", err, res)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || got.CurrentStatusVerdictDowngrade != nil {
		t.Fatalf("runs with current_source evidence must not stamp (byte-identical pre-SPR behavior), got %+v", got.CurrentStatusVerdictDowngrade)
	}
}

func TestApplyAndPersistMutation_ClearsStaleDowngradeStampOnReEmit(t *testing.T) {
	bus := busWithCurrentSourceEvidence()
	doc := newDowngradeDecisionDoc(types.CurrentStatusStillPresent)
	doc.CurrentStatusVerdictDowngrade = &types.CurrentStatusVerdictDowngrade{
		BlockID:         "d1",
		OriginalVerdict: types.CurrentStatusStillPresent,
		Reason:          types.CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
	}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist failed: %v / %+v", err, res)
	}
	if got := bus.Mutable.AnswerDocumentV2(); got.CurrentStatusVerdictDowngrade != nil {
		t.Fatal("a stale stamp must be recomputed away once the run holds current_source evidence")
	}
}

func TestPreCheckCurrentStatusVerdict_ZeroEvidenceWaivesDemand(t *testing.T) {
	view := &types.AnswerSemanticView{
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "d1",
			Kind:        types.BlockDecision,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "本轮没有读取任何源码，无法给出现状判定。",
		}},
	}
	// Zero-evidence run: the obligation is not evaluable — no forced
	// side-pick, no retry burn.
	if hints := preCheckCurrentStatusVerdict(doc, view, newPreEmitCheckContext(newBusForMutationTest())); len(hints) != 0 {
		t.Fatalf("zero-evidence run must waive the verdict demand, got %+v", hints)
	}
	// Evidence run: the demand stays byte-identical.
	if hints := preCheckCurrentStatusVerdict(doc, view, newPreEmitCheckContext(busWithCurrentSourceEvidence())); len(hints) != 1 {
		t.Fatalf("evidence run must keep the verdict demand, got %+v", hints)
	}
}
