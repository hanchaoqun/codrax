package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestCompletionGateGuardResultUsesLedgerGraph(t *testing.T) {
	guard := CompletionGateGuardResult(CompletionGateGuardInput{
		ValidationErr: dataquery.DataValidationError{Violations: []dataquery.DataTaskViolation{{
			Code:    "missing_required_ledger",
			Summary: "contributions missing",
		}}},
		LedgerGraph: LedgerGraph{
			Dependencies: []LedgerDependency{{
				Ledger:          string(LedgerContributions),
				Required:        true,
				Status:          LedgerStatusMissing,
				ProducesActions: []string{string(dataquery.DataActionComputeContribs)},
			}},
		},
	})
	if guard.Code != "missing_workflow_ledger" {
		t.Fatalf("guard=%+v, want missing_workflow_ledger", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].ActionKind != string(dataquery.DataActionComputeContribs) {
		t.Fatalf("violations=%+v, want contribution producer action hint", guard.Violations)
	}
}

func TestCompletionGateGuardResultPrefersMissingLedgerOverRuleLinkSymptom(t *testing.T) {
	guard := CompletionGateGuardResult(CompletionGateGuardInput{
		ValidationErr: dataquery.DataValidationError{Violations: []dataquery.DataTaskViolation{{
			Code:    "unlinked_source_rule_coverage",
			Summary: "source-backed rules are not linked from item ledgers",
		}}},
		LedgerGraph: LedgerGraph{
			Dependencies: []LedgerDependency{{
				Ledger:          string(LedgerDecisions),
				Required:        true,
				Status:          LedgerStatusMissing,
				ProducesActions: []string{string(dataquery.DataActionQualifyRecords)},
			}, {
				Ledger:               string(LedgerContributions),
				Required:             true,
				Status:               LedgerStatusBlockedByPrerequisite,
				MissingPrerequisites: []string{string(LedgerDecisions)},
				ProducesActions:      []string{string(dataquery.DataActionComputeContribs)},
			}},
		},
	})
	if guard.Code != "missing_workflow_ledger" {
		t.Fatalf("guard=%+v, want missing_workflow_ledger", guard)
	}
	if !strings.Contains(guard.ErrorText(), "decisions") {
		t.Fatalf("message=%q, want first missing ledger detail", guard.ErrorText())
	}
	if len(guard.Violations) != 1 || guard.Violations[0].ActionKind != string(dataquery.DataActionQualifyRecords) {
		t.Fatalf("violations=%+v, want decision producer action hint", guard.Violations)
	}
}

func TestCompletionGateGuardResultUsesValidationViolation(t *testing.T) {
	guard := CompletionGateGuardResult(CompletionGateGuardInput{
		ValidationErr: dataquery.DataValidationError{Violations: []dataquery.DataTaskViolation{{
			Code:       "workflow_material_coverage_incomplete",
			Summary:    "material missing",
			RepairHint: "inspect missing material",
		}}},
	})
	if guard.Code != "workflow_material_coverage_incomplete" {
		t.Fatalf("guard=%+v, want typed validation code", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].RepairActionHints[0] != "inspect missing material" {
		t.Fatalf("violations=%+v, want repair hint carried", guard.Violations)
	}
}

func TestCompletionGateGuardResultKeepsDirectValidationOverLedger(t *testing.T) {
	guard := CompletionGateGuardResult(CompletionGateGuardInput{
		ValidationErr: dataquery.DataValidationError{Violations: []dataquery.DataTaskViolation{{
			Code:    "artifact_schema_incompatible",
			Summary: "input is not a record artifact",
		}}},
		LedgerGraph: LedgerGraph{
			Dependencies: []LedgerDependency{{
				Ledger:          string(LedgerContributions),
				Required:        true,
				Status:          LedgerStatusMissing,
				ProducesActions: []string{string(dataquery.DataActionComputeContribs)},
			}},
		},
	})
	if guard.Code != "artifact_schema_incompatible" {
		t.Fatalf("guard=%+v, want direct schema validation blocker", guard)
	}
}

func TestCompletionGateGuardResultUsesLedgerWithoutValidationError(t *testing.T) {
	guard := CompletionGateGuardResult(CompletionGateGuardInput{
		LedgerGraph: LedgerGraph{
			Dependencies: []LedgerDependency{{
				Ledger:          string(LedgerContributions),
				Required:        true,
				Status:          LedgerStatusMissing,
				ProducesActions: []string{string(dataquery.DataActionComputeContribs)},
			}},
		},
	})
	if guard.Code != "missing_workflow_ledger" {
		t.Fatalf("guard=%+v, want missing workflow ledger", guard)
	}
}

func TestCompletionGateGuardResultUsesOutputProjectionGraph(t *testing.T) {
	guard := CompletionGateGuardResult(CompletionGateGuardInput{
		OutputGraph: OutputProjectionGraph{
			Status:          OutputProjectionStatusMissingProjection,
			ProducesActions: []string{string(dataquery.DataActionAssembleAnswer)},
		},
	})
	if guard.Code != "output_projection_missing_projection" {
		t.Fatalf("guard=%+v, want output projection guard", guard)
	}
	if !strings.Contains(guard.ErrorText(), "final answer projection") {
		t.Fatalf("message=%q, want projection detail", guard.ErrorText())
	}
}

func TestCompletionGateGuardResultBlocksMissingAnswer(t *testing.T) {
	guard := CompletionGateGuardResult(CompletionGateGuardInput{
		OutputGraph: OutputProjectionGraph{
			Status:          OutputProjectionStatusMissingAnswer,
			ProducesActions: []string{string(dataquery.DataActionAssembleAnswer)},
		},
	})
	if guard.Code != "output_projection_missing_answer" {
		t.Fatalf("guard=%+v, want missing answer projection guard", guard)
	}
	if !strings.Contains(guard.ErrorText(), "final answer candidate") {
		t.Fatalf("message=%q, want final answer candidate detail", guard.ErrorText())
	}
	if len(guard.Violations) != 1 || guard.Violations[0].ActionKind != string(dataquery.DataActionAssembleAnswer) {
		t.Fatalf("violations=%+v, want assemble_answer repair hint", guard.Violations)
	}
}

func TestCompletionGateGuardResultReportsReferenceGap(t *testing.T) {
	guard := CompletionGateGuardResult(CompletionGateGuardInput{
		OutputGraph: OutputProjectionGraph{
			Status:          OutputProjectionStatusIncompleteReference,
			AnswerItemCount: 2,
			ProducesActions: []string{string(dataquery.DataActionAssembleAnswer)},
		},
		ReferenceGap: ReferenceProjectionGap{
			Present: true,
			Candidate: dataquery.ReferenceKeyCandidate{
				Path:     "targets.csv",
				Field:    "target_id",
				KeyCount: 4,
			},
		},
	})
	if guard.Code != "output_projection_incomplete_reference" {
		t.Fatalf("guard=%+v, want reference projection guard", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].InputAlias != "targets.csv" || guard.Violations[0].MissingFields[0] != "target_id" {
		t.Fatalf("violations=%+v, want reference path/field carried", guard.Violations)
	}
}
