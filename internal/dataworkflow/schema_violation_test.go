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

	canonical := ApplyResolutionCanonicalFieldGuardResult(ApplyResolutionCanonicalFieldGuardInput{
		Action:          action,
		ActionIndex:     3,
		SpecIndex:       1,
		ResolutionPath:  "mapping",
		CanonicalFields: []string{"canonical_id", "canonical_label"},
		AvailableFields: []string{"source_value", "match_status"},
	})
	if canonical.Code != "apply_resolution_canonical_field_contract" || !strings.Contains(canonical.ErrorText(), "canonical value field") {
		t.Fatalf("canonical=%+v, want canonical field guard", canonical)
	}
	if got := canonical.Violations[0].MissingFields; len(got) != 2 || got[0] != "canonical_id" || got[1] != "canonical_label" {
		t.Fatalf("canonical violation=%+v, want canonical missing fields", canonical.Violations[0])
	}
}

func TestActionDependencyGuardResultsAreReducerOwned(t *testing.T) {
	consumer := dataquery.DataAction{ID: "join", Kind: dataquery.DataActionJoinRecords, InputPaths: []string{"left", "future"}}
	producer := dataquery.DataAction{ID: "derive", Kind: dataquery.DataActionDeriveFields, OutputArtifact: "future"}

	intra := IntraBatchDependencyGuardResult(IntraBatchDependencyGuardInput{
		Action:        consumer,
		ActionIndex:   1,
		ConsumedInput: "future",
		Producer:      producer,
	})
	if intra.Code != "intra_batch_dependency" || !strings.Contains(intra.ErrorText(), "deferred queue") {
		t.Fatalf("intra=%+v, want intra-batch dependency guard", intra)
	}

	unavailable := UnavailableActionInputGuardResult(UnavailableActionInputGuardInput{
		Action:      consumer,
		ActionIndex: 0,
		Missing:     []string{"missing_alias"},
	})
	if unavailable.Code != "unavailable_action_input" || !strings.Contains(unavailable.ErrorText(), "missing_alias") {
		t.Fatalf("unavailable=%+v, want unavailable input guard", unavailable)
	}
}

func TestInputPathAndSpecGuardResultsAreReducerOwned(t *testing.T) {
	compute := dataquery.DataAction{ID: "compute", Kind: dataquery.DataActionComputeContribs}
	missingInput := InputPathContractGuardResult(InputPathContractGuardInput{
		Action:      compute,
		ActionIndex: 0,
		Contract:    ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	})
	if missingInput.Code != "missing_action_inputs" || !strings.Contains(missingInput.ErrorText(), "contribution computation") {
		t.Fatalf("missingInput=%+v", missingInput)
	}

	tooMany := InputPathContractGuardResult(InputPathContractGuardInput{
		Action:      dataquery.DataAction{ID: "filter", Kind: dataquery.DataActionFilterRecords, InputPaths: []string{"a", "b"}},
		ActionIndex: 0,
		Contract:    ActionInputPathContract{Min: 1, Max: 1, SingleRecordSet: true},
	})
	if tooMany.Code != "too_many_action_inputs" || !strings.Contains(tooMany.ErrorText(), "single-record-set action") {
		t.Fatalf("tooMany=%+v", tooMany)
	}

	missingSpec := MissingActionSpecGuardResult(MissingActionSpecGuardInput{
		Action:      dataquery.DataAction{ID: "derive", Kind: dataquery.DataActionDeriveFields},
		ActionIndex: 0,
	})
	if missingSpec.Code != "missing_action_spec" || !strings.Contains(missingSpec.ErrorText(), "field specification") {
		t.Fatalf("missingSpec=%+v", missingSpec)
	}
}

func TestMissingUpstreamLedgerGuardResult(t *testing.T) {
	reconcile := MissingUpstreamLedgerGuardResult(MissingUpstreamLedgerGuardInput{
		Action:      dataquery.DataAction{ID: "reconcile", Kind: dataquery.DataActionReconcile},
		ActionIndex: 0,
		Ledger:      LedgerContributions,
	})
	if reconcile.Code != "action_dependency_violation" || !strings.Contains(reconcile.ErrorText(), "compute_contributions") {
		t.Fatalf("reconcile=%+v", reconcile)
	}
	if len(reconcile.Violations) != 1 ||
		reconcile.Violations[0].Role != string(LedgerContributions) ||
		reconcile.Violations[0].Operation != "reconcile" ||
		reconcile.Violations[0].RepairActionHints[0] != string(dataquery.DataActionComputeContribs) {
		t.Fatalf("reconcile violations=%+v, want typed dependency details", reconcile.Violations)
	}

	assemble := MissingUpstreamLedgerGuardResult(MissingUpstreamLedgerGuardInput{
		Action:      dataquery.DataAction{ID: "assemble", Kind: dataquery.DataActionAssembleAnswer},
		ActionIndex: 1,
		Ledger:      LedgerReconcile,
	})
	if assemble.Code != "action_dependency_violation" || !strings.Contains(assemble.ErrorText(), "reconcile_artifacts") {
		t.Fatalf("assemble=%+v", assemble)
	}
	if len(assemble.Violations) != 1 ||
		assemble.Violations[0].Role != string(LedgerReconcile) ||
		assemble.Violations[0].Operation != "answer_projection" ||
		assemble.Violations[0].RepairActionHints[0] != string(dataquery.DataActionReconcile) {
		t.Fatalf("assemble violations=%+v, want typed dependency details", assemble.Violations)
	}
}

func TestUpstreamLedgerGuardResultUsesWorkflowRecordsAndPriorActions(t *testing.T) {
	reconcileAction := dataquery.DataAction{ID: "reconcile", Kind: dataquery.DataActionReconcile}
	guard := UpstreamLedgerGuardResult(MissingUpstreamLedgerGuardInput{
		Action:      reconcileAction,
		ActionIndex: 0,
		Ledger:      LedgerContributions,
	}, []WorkflowRecord{{
		Result: &dataquery.Result{
			Contributions: []dataquery.ContributionRecord{{GroupKey: dataquery.LooseText("group"), Metric: dataquery.LooseText("value"), Value: dataquery.LooseText("1")}},
		},
	}}, dataquery.TaskPlan{})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want no guard when contribution ledger exists in records", guard)
	}

	assembleAction := dataquery.DataAction{ID: "assemble", Kind: dataquery.DataActionAssembleAnswer}
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{
		{ID: "reconcile_first", Kind: dataquery.DataActionReconcile},
		assembleAction,
	}}
	guard = UpstreamLedgerGuardResult(MissingUpstreamLedgerGuardInput{
		Action:      assembleAction,
		ActionIndex: 1,
		Ledger:      LedgerReconcile,
	}, nil, plan)
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want no guard when a prior typed action produces reconcile", guard)
	}

	guard = UpstreamLedgerGuardResult(MissingUpstreamLedgerGuardInput{
		Action:      assembleAction,
		ActionIndex: 0,
		Ledger:      LedgerReconcile,
	}, nil, plan)
	if guard.Code != "action_dependency_violation" {
		t.Fatalf("guard=%+v, want dependency guard when no upstream ledger is ready", guard)
	}
}

func TestEmptySetDiagnosticGuardResultsAreReducerOwned(t *testing.T) {
	action := dataquery.DataAction{ID: "compute", Kind: dataquery.DataActionComputeContribs, InputPaths: []string{"filtered"}}

	zeroMatch := ZeroMatchFilterGuardResult(ZeroMatchFilterGuardInput{
		Action:       action,
		ActionIndex:  0,
		InputAlias:   "filtered",
		ArtifactID:   "filtered",
		InputRows:    10,
		OutputRows:   0,
		SourcePath:   "rows",
		FilterFields: []string{"status"},
	})
	if zeroMatch.Code != "zero_match_filter" || !strings.Contains(zeroMatch.ErrorText(), "zero-match filter artifact filtered") {
		t.Fatalf("zeroMatch=%+v", zeroMatch)
	}
	if len(zeroMatch.Violations) != 1 || zeroMatch.Violations[0].InputAlias != "filtered" || !strings.Contains(strings.Join(zeroMatch.Violations[0].MissingFields, ","), "status") {
		t.Fatalf("zeroMatch violation=%+v", zeroMatch.Violations)
	}

	unmatched := UnmatchedResolutionGuardResult(UnmatchedResolutionGuardInput{
		Action:       action,
		ActionIndex:  1,
		InputAlias:   "resolved",
		ArtifactID:   "resolved",
		BasePath:     "rows",
		BaseRows:     5,
		TargetFields: []string{"canonical_id"},
	})
	if unmatched.Code != "unmatched_resolution" || !strings.Contains(unmatched.ErrorText(), "all-unmatched resolution artifact resolved") {
		t.Fatalf("unmatched=%+v", unmatched)
	}

	zeroEligible := ZeroEligibleGuardResult(ZeroEligibleGuardInput{
		Action:       action,
		ActionIndex:  2,
		InputAlias:   "eligible",
		ArtifactID:   "eligible",
		InputRows:    7,
		EligibleRows: 0,
	})
	if zeroEligible.Code != "zero_eligible_records" || !strings.Contains(zeroEligible.ErrorText(), "zero-eligible qualification artifact eligible") {
		t.Fatalf("zeroEligible=%+v", zeroEligible)
	}
}
