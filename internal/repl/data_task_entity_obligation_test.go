package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
)

// Witness-isomorphic pins for DATAGATE-1 (audit GAP-3/G10, campaign ledger
// §29.140/§29.142; witness eval/results/
// data_multifile_reference_projection-20260719-034625/run-2): the workflow
// took the join_records path, dataTaskWorkflowEntityStageMaterialized turned
// true, the stage machine advanced past normalize_or_enrich_entities, and the
// result validator — reading the obligation as bare records>0 — kept
// demanding entity_resolutions the emit-stage action set could not produce.
// Six repair rounds were all hard-rejected and the already-computed correct
// answer ("17,0,5", round 8) was withheld; terminal failed. The fixtures
// below reproduce the two run shapes (run-2 join path / run-1 normalize path)
// against the workflow validation throat that raised the witness error
// ("validate data workflow result: … entity_resolutions is empty").
// Deliberate reduction vs the witness contract: rule/decision flags are
// omitted — their satisfaction was never contested in G10 and they would only
// add unrelated linkage plumbing.

func entityObligationWitnessContract() dataquery.CoverageContract {
	return dataquery.CoverageContract{
		EntityResolutionRequired:   true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}
}

// entityObligationWitnessResult mirrors the round-8 witness result: correct
// answer computed, contributions and passing reconcile present, zero
// entity_resolution records.
func entityObligationWitnessResult() dataquery.Result {
	return dataquery.Result{
		Answer:         "17,0,5",
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		Contributions: []dataquery.ContributionRecord{{
			ItemID:        dataquery.LooseText("item-1"),
			Source:        dataquery.LooseText("observations.csv"),
			SourceLocator: dataquery.LooseText("row 2"),
			GroupKey:      dataquery.LooseText("matched"),
			Metric:        dataquery.LooseText("count"),
			Value:         dataquery.LooseText("17"),
			Operation:     dataquery.LooseText("add"),
			Role:          dataquery.LooseText("target"),
		}},
		Reconcile: &dataquery.ReconcileReport{
			Status:         dataquery.LooseText("pass"),
			ExpectedAnswer: dataquery.LooseText("17,0,5"),
			ActualAnswer:   dataquery.LooseText("17,0,5"),
			Groups: []dataquery.ReconcileGroup{{
				GroupKey:   dataquery.LooseText("matched"),
				Metric:     dataquery.LooseText("count"),
				Expected:   dataquery.LooseText("17"),
				Actual:     dataquery.LooseText("17"),
				Difference: dataquery.LooseText("0"),
			}},
		},
	}
}

func entityObligationWitnessRecords(withJoinArtifact bool) []dataTaskWorkflowRecord {
	rec := dataTaskWorkflowRecord{
		Plan: dataquery.TaskPlan{CoverageContract: entityObligationWitnessContract()},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{{
				ID:          "labels_joined",
				Kind:        "extract_records/csv",
				SourcePaths: []string{"labels.csv"},
			}},
		},
	}
	if withJoinArtifact {
		// Engine-real emission shape: the runner materializes join outputs
		// with artifact Kind "join_records" (witness run-2 artifact census).
		rec.Result.Artifacts = append(rec.Result.Artifacts, dataquery.DataArtifact{
			ID:          "observations_with_labels",
			Kind:        "join_records",
			SourcePaths: []string{"observations.csv", "labels.csv"},
		})
	}
	return []dataTaskWorkflowRecord{rec}
}

// TestEntityObligationJoinMaterializedPathReleasesAnswer is the run-2 shape:
// join_records materialized the entity stage, zero explicit resolution
// records. Pre-fix this exact throat raised "coverage_contract.
// entity_resolution_required=true but result.entity_resolutions is empty" and
// the workflow deadlocked; post-fix the validator reads the same predicate the
// stage machine routes on, the completion ledger guard stays quiet, and the
// terminal answer selection releases the computed answer unchanged (the fix
// releases the withheld value — it never edits it).
func TestEntityObligationJoinMaterializedPathReleasesAnswer(t *testing.T) {
	records := entityObligationWitnessRecords(true)
	current := dataquery.TaskPlan{CoverageContract: entityObligationWitnessContract()}
	result := entityObligationWitnessResult()

	if err := validateDataTaskWorkflowResult(records, current, result); err != nil {
		t.Fatalf("validateDataTaskWorkflowResult on the materialized join path must accept the discharged entity obligation (GAP-3/G10 deadlock throat): %v", err)
	}
	graph := dataTaskWorkflowCompletionLedgerGraph(records, current, result)
	if !graph.EntityResolutions.Present || graph.EntityResolutions.Count != 0 {
		t.Fatalf("ledger graph must publish entity_resolutions Present with count 0 on the materialized path: %+v", graph.EntityResolutions)
	}
	if guard := dataTaskWorkflowCompletionLedgerGuardResult(records, current, result); !guard.Empty() {
		t.Fatalf("completion ledger guard must stay quiet on the materialized path: %s", guard.ErrorText())
	}
	sel := selectDataTaskTerminalAnswer(records, current, result, entityObligationWitnessContract(), result.OutputContract)
	if sel.Contested || sel.FromFallback {
		t.Fatalf("terminal answer selection=%+v, want the direct candidate", sel)
	}
	if sel.Result.Answer != "17,0,5" {
		t.Fatalf("released answer=%q, want the withheld witness value 17,0,5 released byte-identical", sel.Result.Answer)
	}
	if len(sel.Result.EntityResolutions) != 0 {
		t.Fatalf("the fix must not fabricate entity_resolution records: %+v", sel.Result.EntityResolutions)
	}
}

// TestEntityObligationNormalizePathUnchanged is the run-1 shape (normalize
// path, explicit resolution records): zero regression — the validator accepts
// exactly as before the fix.
func TestEntityObligationNormalizePathUnchanged(t *testing.T) {
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{CoverageContract: entityObligationWitnessContract()},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{{
				ID:          "label_mappings",
				Kind:        "normalize_entities",
				SourcePaths: []string{"labels.csv"},
			}},
		},
	}}
	current := dataquery.TaskPlan{CoverageContract: entityObligationWitnessContract()}
	result := entityObligationWitnessResult()
	result.EntityResolutions = []dataquery.EntityResolutionRecord{{
		SourceValue: dataquery.LooseText("lbl-a"),
		CanonicalID: dataquery.LooseText("L-A"),
		Status:      dataquery.LooseText("resolved"),
	}}
	if err := validateDataTaskWorkflowResult(records, current, result); err != nil {
		t.Fatalf("normalize path with explicit resolutions must keep passing: %v", err)
	}
	if guard := dataTaskWorkflowCompletionLedgerGuardResult(records, current, result); !guard.Empty() {
		t.Fatalf("normalize path completion guard must stay quiet: %s", guard.ErrorText())
	}
}

// TestEntityObligationGenuinelyMissingStillFailsLoud is the honest-failure
// negative arm: no resolution records AND no materializing artifact anywhere
// in the history — the obligation is genuinely unpaid, the validator must keep
// rejecting, and the completion guard must keep demanding the entity ledger.
// The GAP-3 fix removes the false deadlock, not the fail-loud contract.
func TestEntityObligationGenuinelyMissingStillFailsLoud(t *testing.T) {
	records := entityObligationWitnessRecords(false)
	current := dataquery.TaskPlan{CoverageContract: entityObligationWitnessContract()}
	result := entityObligationWitnessResult()

	err := validateDataTaskWorkflowResult(records, current, result)
	if err == nil {
		t.Fatal("validateDataTaskWorkflowResult must keep rejecting a genuinely unpaid entity obligation")
	}
	if !strings.Contains(err.Error(), "result.entity_resolutions") {
		t.Fatalf("err=%v, want the entity ledger named", err)
	}
	guard := dataTaskWorkflowCompletionLedgerGuardResult(records, current, result)
	if guard.Empty() {
		t.Fatal("completion ledger guard must fire for a genuinely unpaid entity obligation")
	}
	if !strings.Contains(guard.ErrorText(), string(dataworkflow.LedgerEntityResolutions)) {
		t.Fatalf("guard=%q, want the entity ledger named", guard.ErrorText())
	}
	graph := dataTaskWorkflowCompletionLedgerGraph(records, current, result)
	if graph.EntityResolutions.Present {
		t.Fatalf("ledger graph must not publish Present without records or materialization: %+v", graph.EntityResolutions)
	}
}
