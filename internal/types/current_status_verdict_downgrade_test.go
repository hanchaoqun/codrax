package types

import "testing"

// SPR #72 pins (RTC ledger §8.3). Mutation contract: removing the
// downgrade compute lane, the shared ledger predicate, or the clone/audit
// round-trip turns these red.

func downgradeTestDoc(verdict CurrentStatusVerdict) *AnswerDocumentV2 {
	return &AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:                   "d1",
			Kind:                 BlockDecision,
			SurfaceRole:          SurfacePrincipal,
			Text:                 "是。无法判断该问题在最新代码中是否已修复。",
			CurrentStatusVerdict: verdict,
		}},
	}
}

func TestComputeCurrentStatusVerdictDowngrade_SidePickedVerdictZeroEvidence(t *testing.T) {
	for _, verdict := range []CurrentStatusVerdict{CurrentStatusStillPresent, CurrentStatusFixed} {
		doc := downgradeTestDoc(verdict)
		d := ComputeCurrentStatusVerdictDowngrade(doc, nil, false)
		if d == nil {
			t.Fatalf("%s + zero current_source evidence must downgrade", verdict)
		}
		if d.OriginalVerdict != verdict || d.BlockID != "d1" {
			t.Fatalf("downgrade must carry the original verdict in the audit position, got %+v", d)
		}
		if d.Reason != CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence {
			t.Fatalf("unexpected reason %q", d.Reason)
		}
		// Audit position: the block's own typed verdict is never modified.
		if doc.Blocks[0].CurrentStatusVerdict != verdict {
			t.Fatalf("block verdict must stay untouched, got %q", doc.Blocks[0].CurrentStatusVerdict)
		}
	}
}

func TestComputeCurrentStatusVerdictDowngrade_EvidenceRunNeverFires(t *testing.T) {
	contract := &CurrentStatusDiagnosticContract{Required: true}
	for _, verdict := range []CurrentStatusVerdict{
		CurrentStatusStillPresent, CurrentStatusFixed, CurrentStatusNotEnoughEvidence, CurrentStatusUnknown,
	} {
		if d := ComputeCurrentStatusVerdictDowngrade(downgradeTestDoc(verdict), contract, true); d != nil {
			t.Fatalf("runs with current_source evidence must keep pre-SPR behavior (verdict %q), got %+v", verdict, d)
		}
	}
}

func TestComputeCurrentStatusVerdictDowngrade_NotEnoughEvidenceIsConsistent(t *testing.T) {
	contract := &CurrentStatusDiagnosticContract{Required: true}
	if d := ComputeCurrentStatusVerdictDowngrade(downgradeTestDoc(CurrentStatusNotEnoughEvidence), contract, false); d != nil {
		t.Fatalf("not_enough_evidence is consistent with a zero-evidence ledger; must not downgrade, got %+v", d)
	}
}

func TestComputeCurrentStatusVerdictDowngrade_MissingVerdictDemandWaiver(t *testing.T) {
	doc := downgradeTestDoc(CurrentStatusUnknown)
	contract := &CurrentStatusDiagnosticContract{Required: true}
	d := ComputeCurrentStatusVerdictDowngrade(doc, contract, false)
	if d == nil {
		t.Fatal("required contract + zero evidence + no verdict must stamp the demand waiver")
	}
	if d.OriginalVerdict != CurrentStatusUnknown {
		t.Fatalf("demand-waiver stamp must not invent a verdict, got %q", d.OriginalVerdict)
	}
	// Without a required contract there is no obligation to waive.
	if d := ComputeCurrentStatusVerdictDowngrade(doc, nil, false); d != nil {
		t.Fatalf("no contract + no verdict must not stamp, got %+v", d)
	}
	if d := ComputeCurrentStatusVerdictDowngrade(doc, &CurrentStatusDiagnosticContract{Required: false}, false); d != nil {
		t.Fatalf("non-required contract + no verdict must not stamp, got %+v", d)
	}
}

func TestObservationLedgerHasCurrentSourceEvidence_SharedLanePredicate(t *testing.T) {
	sourceRecord := ObservationRecord{
		ID:     "obs:1",
		Origin: AnswerEvidenceOriginCurrentSource,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceCurrentSource,
			Path: "internal/foo/bar.go",
		},
		Span: ObservationSpan{LineStart: 10, LineEnd: 12},
	}
	if !ObservationLedgerHasCurrentSourceEvidence(ObservationLedger{Records: []ObservationRecord{sourceRecord}}) {
		t.Fatal("current_source record with line span must count as lane evidence")
	}
	runtimeOnly := ObservationRecord{
		ID:     "obs:2",
		Origin: AnswerEvidenceOriginRuntimeArtifact,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceRuntimeArtifact,
			Path: "trace.systrace",
		},
		Span: ObservationSpan{LineStart: 100},
	}
	if ObservationLedgerHasCurrentSourceEvidence(ObservationLedger{Records: []ObservationRecord{runtimeOnly}}) {
		t.Fatal("runtime-artifact records must not satisfy the current_source lane")
	}
	ungrounded := sourceRecord
	ungrounded.GroundingStatus = GroundingUngrounded
	if ObservationLedgerHasCurrentSourceEvidence(ObservationLedger{Records: []ObservationRecord{ungrounded}}) {
		t.Fatal("ungrounded current_source records must not satisfy the lane (mirror of the completion-gate arm)")
	}
	if ObservationLedgerHasCurrentSourceEvidence(ObservationLedger{}) {
		t.Fatal("empty ledger has no lane evidence")
	}
}

func TestCurrentStatusVerdictDowngrade_SurvivesMutableStateRoundTrip(t *testing.T) {
	doc := downgradeTestDoc(CurrentStatusStillPresent)
	doc.CurrentStatusVerdictDowngrade = &CurrentStatusVerdictDowngrade{
		BlockID:         "d1",
		OriginalVerdict: CurrentStatusStillPresent,
		Reason:          CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
	}
	m := NewMutableState("t")
	m.SetAnswerDocumentV2WithMutation(MutationReplaceAll, doc)
	got := m.AnswerDocumentV2()
	if got == nil || got.CurrentStatusVerdictDowngrade == nil {
		t.Fatal("downgrade stamp must survive the MutableState deep-clone round-trip (audit position)")
	}
	if *got.CurrentStatusVerdictDowngrade != *doc.CurrentStatusVerdictDowngrade {
		t.Fatalf("stamp mutated in round-trip: %+v", got.CurrentStatusVerdictDowngrade)
	}
	got.CurrentStatusVerdictDowngrade.OriginalVerdict = CurrentStatusFixed
	if fresh := m.AnswerDocumentV2(); fresh.CurrentStatusVerdictDowngrade.OriginalVerdict != CurrentStatusStillPresent {
		t.Fatal("clone must be defensive: mutating a reader copy must not leak into stored state")
	}
}

func TestCurrentStatusDowngradeForBlock_Matching(t *testing.T) {
	doc := downgradeTestDoc(CurrentStatusStillPresent)
	doc.CurrentStatusVerdictDowngrade = &CurrentStatusVerdictDowngrade{
		BlockID:         "d1",
		OriginalVerdict: CurrentStatusStillPresent,
		Reason:          CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
	}
	if CurrentStatusDowngradeForBlock(doc, doc.Blocks[0]) == nil {
		t.Fatal("stamp must match its target block")
	}
	other := doc.Blocks[0]
	other.ID = "d2"
	if CurrentStatusDowngradeForBlock(doc, other) != nil {
		t.Fatal("stamp must not match a different block id")
	}
	mismatch := doc.Blocks[0]
	mismatch.CurrentStatusVerdict = CurrentStatusFixed
	if CurrentStatusDowngradeForBlock(doc, mismatch) != nil {
		t.Fatal("stamp must not match a different verdict")
	}
	// Demand-waiver stamps (no original verdict) never target a rendered block.
	doc.CurrentStatusVerdictDowngrade = &CurrentStatusVerdictDowngrade{
		Reason: CurrentStatusVerdictDowngradeZeroCurrentSourceEvidence,
	}
	if CurrentStatusDowngradeForBlock(doc, doc.Blocks[0]) != nil {
		t.Fatal("demand-waiver stamp must not attach to any block surface")
	}
}
