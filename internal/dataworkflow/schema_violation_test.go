package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestFieldContractCandidateArtifactsRanksCompleteMatches(t *testing.T) {
	projections := []ArtifactSchemaProjection{
		{ID: "base", Aliases: []string{"base.json"}, Fields: []string{"id"}},
		{ID: "partial", Aliases: []string{"partial.json"}, Fields: []string{"currency"}},
		{ID: "complete", Aliases: []string{"complete.json"}, Fields: []string{"currency", "amount"}},
	}

	candidates := FieldContractCandidateArtifacts(projections, "base.json", []string{"currency", "amount"}, 4)
	if len(candidates) < 2 {
		t.Fatalf("candidates=%+v, want complete and partial matches", candidates)
	}
	if candidates[0].Alias != "complete.json" || !candidates[0].MatchesAll {
		t.Fatalf("first candidate=%+v, want complete match first", candidates[0])
	}
	labels := FieldContractCandidateLabels(candidates)
	if len(labels) == 0 || !strings.Contains(labels[0], "complete.json has [currency, amount]") {
		t.Fatalf("labels=%v", labels)
	}
}

func TestFieldContractRepairHintsFollowAllowedActions(t *testing.T) {
	hints := strings.Join(FieldContractRepairHints([]string{"derive_fields", "compute_contributions"}), "\n")
	for _, want := range []string{"derive_fields", "compute_contributions"} {
		if !strings.Contains(hints, want) {
			t.Fatalf("hints=%q, want %q", hints, want)
		}
	}
	if strings.Contains(hints, "join_records") {
		t.Fatalf("hints=%q should not mention disallowed join_records", hints)
	}
}

func TestFieldContractGuardResultUsesTypedViolation(t *testing.T) {
	violation := NewFieldContractViolation(FieldContractViolationInput{
		Action: dataquery.DataAction{
			ID:         "compute",
			Kind:       dataquery.DataActionComputeContribs,
			InputPaths: []string{"rows.json"},
		},
		InputAlias:        "rows.json",
		MissingFields:     []string{"amount"},
		AvailableFields:   []string{"id", "currency"},
		SchemaProjections: []ArtifactSchemaProjection{{ID: "candidate", Aliases: []string{"candidate.json"}, Fields: []string{"amount"}}},
	})
	guard := FieldContractGuardResult(FieldContractGuardInput{
		Action:      dataquery.DataAction{ID: "compute", Kind: dataquery.DataActionComputeContribs},
		ActionIndex: 1,
		Violation:   violation,
	})
	if guard.Code != "field_contract_violation" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed field_contract_violation", guard)
	}
	msg := guard.ErrorText()
	for _, want := range []string{"action 2", "amount", "rows.json", "candidate.json"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message=%q, want %q", msg, want)
		}
	}
}

func TestActionInputContractGuardResultUsesTypedAction(t *testing.T) {
	guard := ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:          "apply_resolution_contract",
		Action:        dataquery.DataAction{ID: "apply_vendor", Kind: dataquery.DataActionApplyResolutions, InputPaths: []string{"rows", "mapping"}},
		InputAlias:    "mapping",
		MissingFields: []string{"canonical_id"},
		Message:       "mapping is missing canonical_id",
	})
	if guard.Code != "apply_resolution_contract" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed action input contract guard", guard)
	}
	v := guard.Violations[0]
	if v.ActionID != "apply_vendor" || v.ActionKind != string(dataquery.DataActionApplyResolutions) || v.InputAlias != "mapping" {
		t.Fatalf("violation=%+v, want action/input metadata", v)
	}
	if !strings.Contains(guard.ErrorText(), "canonical_id") {
		t.Fatalf("message=%q, want canonical_id", guard.ErrorText())
	}
}

func TestApplyResolutionGuardResultsAreReducerOwned(t *testing.T) {
	action := dataquery.DataAction{ID: "apply_vendor", Kind: dataquery.DataActionApplyResolutions, InputPaths: []string{"rows", "mapping"}}

	diagnostic := ApplyResolutionDiagnosticInputGuardResult(ApplyResolutionDiagnosticInputGuardInput{
		Action:         action,
		ActionIndex:    0,
		ResolutionPath: "mapping#source",
	})
	if diagnostic.Code != "apply_resolution_diagnostic_input" || len(diagnostic.Violations) != 1 {
		t.Fatalf("diagnostic=%+v, want diagnostic guard", diagnostic)
	}
	if !strings.Contains(diagnostic.ErrorText(), "diagnostic artifact mapping#source") {
		t.Fatalf("diagnostic message=%q", diagnostic.ErrorText())
	}

	lineage := ApplyResolutionLineageGuardResult(ApplyResolutionLineageGuardInput{
		Action:                  action,
		ActionIndex:             1,
		ResolutionPath:          "mapping",
		BasePath:                "other_rows",
		ResolutionSourceLineage: "rows",
		BaseLineage:             "other_rows, source.csv",
	})
	if lineage.Code != "apply_resolution_lineage_contract" || !strings.Contains(lineage.ErrorText(), "not compatible") {
		t.Fatalf("lineage=%+v, want lineage guard", lineage)
	}

	noProgress := ApplyResolutionNoProgressGuardResult(ApplyResolutionNoProgressGuardInput{
		Action:         action,
		ActionIndex:    2,
		ResolutionPath: "mapping",
		BasePath:       "rows_with_vendor",
		TargetFields:   []string{"vendor_id", "vendor_status"},
	})
	if noProgress.Code != "apply_resolution_no_progress" || !strings.Contains(noProgress.ErrorText(), "idempotent") {
		t.Fatalf("noProgress=%+v, want no-progress guard", noProgress)
	}
}
