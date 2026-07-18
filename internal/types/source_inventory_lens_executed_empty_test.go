package types

import (
	"reflect"
	"testing"
)

// §29.122 LENSBURN 病B pins: "lens executed" is a first-class typed fact
// independent of result emptiness. A successfully executed empty lens carries
// the LensExecutedEmpty credential through clone/normalize/merge and is
// accepted by SourceInventoryLensExecuted; a failed, never-executed, or
// analyzer-stage lens can never impersonate execution.

func lensExecutedEmptyCarrier(provenance ...string) SourceInventoryObservation {
	return SourceInventoryObservation{
		Active:     true,
		Complete:   true,
		Scopes:     []string{"."},
		Provenance: provenance,
		Lens:       []string{"lens_executed_empty"},
		Execution:  &SourceInventoryExecutionState{LensExecutedEmpty: true},
	}
}

func TestSourceInventoryLensExecutedEmpty_CarrierSurvivesCloneAndNormalize(t *testing.T) {
	carrier := lensExecutedEmptyCarrier(
		SourceInventoryProvenanceRepoLensToolQuery,
		SourceInventoryProvenanceStageExplore,
	)
	cloned := CloneSourceInventoryObservation(carrier)
	if !cloned.LensExecutedEmpty() {
		t.Fatalf("executed-empty carrier must survive clone/normalize, got %+v", cloned)
	}
	if cloned.IsActive() {
		t.Fatalf("executed-empty carrier must stay non-IsActive (no row content), got %+v", cloned)
	}
	if !SourceInventoryLensExecuted(cloned) {
		t.Fatalf("successfully executed empty lens must count as executed, got %+v", cloned)
	}
}

func TestSourceInventoryLensExecutedEmpty_ZeroObservationStaysZero(t *testing.T) {
	// 无关形字节恒等负臂: without the engine-minted execution marker, an
	// Active-but-empty observation still collapses to the zero value exactly
	// as before — Active alone can never impersonate lens execution.
	bare := SourceInventoryObservation{
		Active:     true,
		Complete:   true,
		Scopes:     []string{"."},
		Provenance: []string{SourceInventoryProvenanceRepoLensToolQuery, SourceInventoryProvenanceStageExplore},
	}
	if got := CloneSourceInventoryObservation(bare); !reflect.DeepEqual(got, SourceInventoryObservation{}) {
		t.Fatalf("markerless empty observation must normalize to the zero value, got %+v", got)
	}
	if SourceInventoryLensExecuted(bare) {
		t.Fatalf("markerless empty observation must not count as executed")
	}
	if got := MergeSourceInventoryObservation(SourceInventoryObservation{}, SourceInventoryObservation{}); !reflect.DeepEqual(got, SourceInventoryObservation{}) {
		t.Fatalf("zero-merge must stay the zero value, got %+v", got)
	}
}

func TestSourceInventoryLensExecutedEmpty_AnalyzeStageLensDoesNotCount(t *testing.T) {
	// 负臂: the provenance discipline is unchanged — an analyzer-stage lens
	// (or a carrier with no execution provenance at all) never mints the
	// execution credential, empty result or not.
	analyzeStage := lensExecutedEmptyCarrier(
		SourceInventoryProvenanceRepoLensToolQuery,
		SourceInventoryProvenanceStageAnalyze,
	)
	if SourceInventoryLensExecuted(analyzeStage) {
		t.Fatalf("analyzer-stage executed-empty carrier must not count as executed")
	}
	if SourceInventoryLensExecuted(lensExecutedEmptyCarrier()) {
		t.Fatalf("executed-empty carrier without execution provenance must not count as executed")
	}
	inactive := lensExecutedEmptyCarrier(SourceInventoryProvenanceRepoLensToolQuery)
	inactive.Active = false
	if inactive.LensExecutedEmpty() || SourceInventoryLensExecuted(inactive) {
		t.Fatalf("inactive carrier must not count as executed")
	}
}

func TestSourceInventoryLensExecutedEmpty_RowContentDisablesShapeAccessor(t *testing.T) {
	withRows := lensExecutedEmptyCarrier(SourceInventoryProvenanceRepoLensToolQuery)
	withRows.Sets = []SourceInventoryObservationSet{{
		Role: AnswerCandidateRoleFunction,
		Members: []SourceInventoryObservationMember{{
			Name: "run", Key: "src/app.py::run", Role: AnswerCandidateRoleFunction, File: "src/app.py",
		}},
	}}
	if withRows.LensExecutedEmpty() {
		t.Fatalf("row content must disable the executed-empty shape accessor (IsActive lane governs)")
	}
	if !withRows.IsActive() || !SourceInventoryLensExecuted(withRows) {
		t.Fatalf("row-ful lens observation keeps the historic IsActive execution lane")
	}
}

func TestSourceInventoryLensExecutedEmpty_MergePreservesCredential(t *testing.T) {
	carrier := lensExecutedEmptyCarrier(
		SourceInventoryProvenanceRepoLensToolQuery,
		SourceInventoryProvenanceStageExplore,
	)
	rows := SourceInventoryObservation{
		Active: true,
		Sets: []SourceInventoryObservationSet{{
			Role: AnswerCandidateRoleFile,
			Members: []SourceInventoryObservationMember{{
				Name: "big.systrace", Key: "big.systrace", Role: AnswerCandidateRoleFile,
				File: "big.systrace", Provenance: []string{"tool:list_files:direct"},
			}},
		}},
	}
	// Historic bug shape: the early-return merge arms dropped the non-IsActive
	// executed-empty side, so a later list_files universe silently destroyed
	// the execution credential.
	merged := MergeSourceInventoryObservation(carrier, rows)
	if !merged.IsActive() {
		t.Fatalf("merged view must keep the row content, got %+v", merged)
	}
	if !SourceInventoryLensExecuted(merged) {
		t.Fatalf("merge(executed-empty, rows) must preserve the execution credential, got %+v", merged)
	}
	merged = MergeSourceInventoryObservation(rows, carrier)
	if !merged.IsActive() || !SourceInventoryLensExecuted(merged) {
		t.Fatalf("merge(rows, executed-empty) must preserve the execution credential, got %+v", merged)
	}
	// An unqualified donor (analyzer-stage) must NOT smuggle a credential in.
	analyzeStage := lensExecutedEmptyCarrier(
		SourceInventoryProvenanceRepoLensToolQuery,
		SourceInventoryProvenanceStageAnalyze,
	)
	if merged := MergeSourceInventoryObservation(rows, analyzeStage); SourceInventoryLensExecuted(merged) {
		t.Fatalf("analyzer-stage empty lens must not credential the merged view, got %+v", merged)
	}
}

func TestSourceInventoryLensExecutedEmpty_FromMutableAdvisoryReplacementKeepsCredential(t *testing.T) {
	// Fix-round item 2 pin: the Turn A advisory replacement arm in
	// sourceInventoryObservationFromMutableFields. Reachability note: the arm
	// fires only when FromAdvisory(turnA.SourceInventoryAdvisory) is NOT
	// IsActive while the advisory itself is — SetTurnAArtifacts materializes
	// turnA.SourceInventoryObservation from any FromAdvisory-active advisory,
	// and that active observation then wins the merge before the arm. So the
	// regression shape is a projection-inert advisory (here: a zero-candidate
	// set whose role fails NormalizeAnswerCandidateRole, e.g. a legacy blob
	// restore): the historic bare SourceInventoryObservationFromAdvisory
	// assignment replaced the durable executed-empty carrier with the ZERO
	// observation and destroyed the execution credential. The ensure wrapper
	// must keep the carrier as the durable view instead.
	carrier := lensExecutedEmptyCarrier(
		SourceInventoryProvenanceRepoLensToolQuery,
		SourceInventoryProvenanceStageExplore,
	)
	advisory := SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Scopes:       []string{"."},
		Provenance:   []string{"handoff:turn_a"},
		Sets:         []SourceInventoryAdvisorySet{{Role: "legacy_role_shape", Total: 3}},
	}
	if !advisory.IsActive() {
		t.Fatalf("fixture drift: advisory must be active (Active + non-empty Sets)")
	}
	if projected := SourceInventoryObservationFromAdvisory(advisory); projected.IsActive() || SourceInventoryLensExecuted(projected) {
		t.Fatalf("fixture drift: the advisory projection must be inert (that is the only shape that reaches the replacement arm), got %+v", projected)
	}
	mut := NewMutableState("source inventory")
	mut.SetSourceInventoryObservation(carrier)
	mut.SetTurnAArtifacts(TurnAArtifacts{SourceInventoryAdvisory: advisory})
	view := SourceInventoryObservationFromMutable(mut)
	if !view.LensExecutedEmpty() {
		t.Fatalf("advisory replacement arm must keep the durable executed-empty carrier, got %+v", view)
	}
	if !SourceInventoryLensExecuted(view) {
		t.Fatalf("advisory replacement arm must preserve the executed-empty lens credential, got %+v", view)
	}

	// Companion (merge-arm lane, not the replacement arm): a row-ful advisory is
	// materialized into turnA.SourceInventoryObservation by SetTurnAArtifacts,
	// and the credential is grafted by the merge early arm instead.
	rowful := SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Scopes:       []string{"."},
		Provenance:   []string{"handoff:turn_a"},
		Sets: []SourceInventoryAdvisorySet{{
			Role: AnswerCandidateRoleFile,
			Candidates: []SourceInventoryAdvisoryCandidate{{
				Member: "big.systrace", Key: "big.systrace",
				Role: AnswerCandidateRoleFile, File: "big.systrace",
			}},
		}},
	}
	mut = NewMutableState("source inventory")
	mut.SetSourceInventoryObservation(carrier)
	mut.SetTurnAArtifacts(TurnAArtifacts{SourceInventoryAdvisory: rowful})
	view = SourceInventoryObservationFromMutable(mut)
	if !view.IsActive() || !SourceInventoryLensExecuted(view) {
		t.Fatalf("row-ful Turn A advisory must keep rows and credential via the merge arm, got %+v", view)
	}
}

func TestSourceInventoryLensExecutedEmpty_FromMutableHealsSchedulerPredicate(t *testing.T) {
	// 调度侧 pin: orchestrator env.SourceInventoryLensExecuted is exactly
	// SourceInventoryLensExecuted(SourceInventoryObservationFromMutable(...)).
	// Persisting the executed-empty carrier must heal it so the lens window
	// stops re-forming every round.
	carrier := lensExecutedEmptyCarrier(
		SourceInventoryProvenanceRepoLensToolQuery,
		SourceInventoryProvenanceStageExplore,
	)
	mut := NewMutableState("source inventory")
	mut.SetSourceInventoryObservation(carrier)
	if !SourceInventoryLensExecuted(SourceInventoryObservationFromMutable(mut)) {
		t.Fatalf("persisted executed-empty carrier must heal the scheduler-side lens predicate")
	}
	// Even when the closure later contributes an unrelated active universe,
	// the merged durable view keeps the credential.
	mut.EvidenceClosure().RecordSourceInventoryObservation(SourceInventoryObservation{
		Active: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role: SourcePathRoleThirdParty, Count: 2, Complete: true,
			Provenance: []string{"source_class_universe:git_tracked_or_filesystem"},
		}},
	})
	merged := SourceInventoryObservationFromMutable(mut)
	if !merged.IsActive() || !SourceInventoryLensExecuted(merged) {
		t.Fatalf("closure universe merge must not destroy the execution credential, got %+v", merged)
	}
}
