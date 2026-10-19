package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHMC266NegativeClaimCompilerIdentityCannotBeSelfCertified(t *testing.T) {
	for _, origin := range []AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact, AnswerEvidenceOriginRepoNegativeSearch, AnswerEvidenceOriginMCPResource} {
		for _, provenance := range []string{"model", SourceInventoryPrincipalRowSetAggregateProvenance} {
			t.Run(string(origin)+"/"+provenance, func(t *testing.T) {
				fact := AnswerAggregateFact{Kind: AnswerAggregateNegativeObservation, Label: "unexecuted absence", Value: "0", Provenance: provenance,
					Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: string(origin)}, {Name: "target", Value: "not-found"}, {Name: "scope", Value: "bounded-source"}, {Name: "result_count", Value: "0"}}}
				ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: []AnswerAggregateFact{fact}})
				before, _ := json.Marshal(ledger)
				rows := ProjectObservationPromptRecords(ledger.Records, nil, nil, DefaultObservationPromptProjectionOptions(10))
				if len(rows) != 1 || rows[0].Value != "" || rows[0].ResultCount != nil || rows[0].ClaimAuthority != ObservationClaimAuthorityModelInference || !strings.Contains(strings.Join(rows[0].Notes, " "), "zero_result_authority=`not_established`") {
					t.Fatalf("model-selected identity certified zero: %+v", rows)
				}
				after, _ := json.Marshal(ledger)
				if string(before) != string(after) {
					t.Fatal("display qualification changed the ledger")
				}
			})
		}
	}
}

func TestHMC266NegativeSearchCompilerIdentityKeepsItsSeparateContract(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: []AnswerAggregateFact{{Kind: AnswerAggregateNegativeSearch, Label: "repo search", Value: "0",
		Dimensions: []AnswerAggregateDimension{{Name: "repo", Value: "."}, {Name: "scope", Value: "settings.go"}, {Name: "pattern", Value: "old_option"}, {Name: "result_count", Value: "0"}},
	}}})
	rows := ProjectObservationPromptRecords(ledger.Records, nil, nil, DefaultObservationPromptProjectionOptions(10))
	if len(rows) != 1 || rows[0].Value != "0" || rows[0].ResultCount == nil || *rows[0].ResultCount != 0 {
		t.Fatalf("negative_search compatibility changed: %+v", rows)
	}
}

func TestHMC266NegativeModelCopyCannotMergeAwayIndependentProducerZero(t *testing.T) {
	zero := 0
	fact := AnswerAggregateFact{Kind: AnswerAggregateNegativeObservation, Label: "absence", Value: "0",
		Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: string(AnswerEvidenceOriginWebPage)}, {Name: "target", Value: "entry"}, {Name: "scope", Value: "https://example.test/entries"}, {Name: "tool_result", Value: "fetch[0]"}, {Name: "payload_ref", Value: "blob://exact-payload"}}}
	native := ObservationRecord{ID: "native-zero", Origin: AnswerEvidenceOriginWebPage, Producer: "fetch", ClaimAuthority: ObservationClaimAuthorityDirectObservation,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceKindForOrigin(AnswerEvidenceOriginWebPage), URL: "https://example.test/entries", ToolCallID: "fetch[0]", PayloadRef: "blob://exact-payload", RawRef: "blob://exact-payload"},
		ClaimKey:  "entry", Subject: "entry", Value: "0", Negative: true, ResultCount: &zero, Scope: "https://example.test/entries"}
	duplicate := native
	duplicate.ID = "native-zero-copy"
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: []AnswerAggregateFact{fact}, ToolResults: []ToolResult{{ToolName: "fetch", Success: true, Observations: []ObservationRecord{native, duplicate}}}})
	if len(ledger.Records) != 2 {
		t.Fatalf("model claim must stay separate; identical native observations must still dedupe: %+v", ledger.Records)
	}
	rows := ProjectObservationPromptRecords(ledger.Records, nil, nil, DefaultObservationPromptProjectionOptions(10))
	var measured, advisory int
	for _, row := range rows {
		if row.Value == "0" && row.ResultCount != nil && *row.ResultCount == 0 {
			measured++
		} else if row.Value == "" && row.ResultCount == nil && strings.Contains(strings.Join(row.Notes, " "), "zero_result_authority=`not_established`") {
			advisory++
		}
	}
	if measured != 1 || advisory != 1 {
		t.Fatalf("lost native zero or promoted its model copy: %+v", rows)
	}
}
