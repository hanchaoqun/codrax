package types

import (
	"strings"
	"testing"
)

func a2RequiredCurrentSourceRequestModel() RequestModel {
	return RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"结合当前源码解释"},
			Confidence:                          0.9,
		},
	}
}

func a2AggregateFact(refs ...string) AnswerAggregateFact {
	return AnswerAggregateFact{
		Kind:        AnswerAggregateScalar,
		Label:       "model-authored source fact",
		Value:       "1",
		SupportRefs: refs,
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: "current_source"},
		},
	}
}

func a2AggregateRecord(t *testing.T, ledger ObservationLedger) ObservationRecord {
	t.Helper()
	for _, record := range ledger.Records {
		if strings.HasPrefix(record.ID, "aggregate:0#") {
			return record
		}
	}
	t.Fatalf("aggregate record missing: %+v", ledger.Records)
	return ObservationRecord{}
}

func TestCompileObservationLedger_A2BareSupportRefIsAdvisory(t *testing.T) {
	rm := a2RequiredCurrentSourceRequestModel()
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{a2AggregateFact("internal/types/observation_ledger.go:44")},
		RequestModel:   &rm,
	})
	record := a2AggregateRecord(t, ledger)
	if record.Origin != AnswerEvidenceOriginSystemInference || record.SourceRef.Kind != ObservationSourceModelClaim {
		t.Fatalf("bare model file:line minted proof lane: %+v", record)
	}
	if len(record.SupportRefs) != 1 || record.SupportRefs[0] != "internal/types/observation_ledger.go:44" {
		t.Fatalf("advisory demotion must retain the declared coordinate losslessly: %+v", record)
	}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{RequestModel: &rm, Ledger: ledger})
	if authority.CurrentSourceSatisfied || authority.CurrentSourceRecordCount != 0 {
		t.Fatalf("bare SupportRef still satisfied current source: %+v", authority)
	}
}

func TestCompileObservationLedger_A2GroundedAndRecoveredEvidenceBindExactCoordinate(t *testing.T) {
	rm := a2RequiredCurrentSourceRequestModel()
	for _, status := range []GroundingStatus{GroundingGrounded, GroundingRecovered} {
		t.Run(string(status), func(t *testing.T) {
			ledger := CompileObservationLedger(ObservationLedgerInput{
				EvidenceItems: []EvidenceItem{{
					ID:              "coordinate-witness",
					Kind:            EvidenceDirect,
					Scope:           ScopeLineRange,
					Source:          "internal/types/observation_ledger.go",
					LineStart:       40,
					LineEnd:         48,
					GroundingStatus: status,
				}},
				AggregateFacts: []AnswerAggregateFact{a2AggregateFact("observation_ledger.go:44-45")},
				RequestModel:   &rm,
			})
			record := a2AggregateRecord(t, ledger)
			if record.Origin != AnswerEvidenceOriginCurrentSource || record.SourceRef.Kind != ObservationSourceCurrentSource {
				t.Fatalf("verified coordinate was over-demoted: %+v", record)
			}
			if record.SourceRef.Path != "internal/types/observation_ledger.go" ||
				record.Span.LineStart != 44 || record.Span.LineEnd != 45 || record.GroundingStatus != status {
				t.Fatalf("aggregate record did not publish canonical witness binding: %+v", record)
			}
		})
	}
}

func TestCompileObservationLedger_A2RejectedGroundingAndCoordinateMismatchDemote(t *testing.T) {
	rm := a2RequiredCurrentSourceRequestModel()
	for name, item := range map[string]EvidenceItem{
		"ungrounded": {
			Kind: EvidenceDirect, Scope: ScopeLine, Source: "internal/types/a.go", LineStart: 44,
			GroundingStatus: GroundingUngrounded,
		},
		"legacy-empty": {
			Kind: EvidenceDirect, Scope: ScopeLine, Source: "internal/types/a.go", LineStart: 44,
		},
		"wrong-line": {
			Kind: EvidenceDirect, Scope: ScopeLine, Source: "internal/types/a.go", LineStart: 45,
			GroundingStatus: GroundingGrounded,
		},
		"wrong-file": {
			Kind: EvidenceDirect, Scope: ScopeLine, Source: "internal/types/b.go", LineStart: 44,
			GroundingStatus: GroundingGrounded,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ledger := CompileObservationLedger(ObservationLedgerInput{
				EvidenceItems:  []EvidenceItem{item},
				AggregateFacts: []AnswerAggregateFact{a2AggregateFact("internal/types/a.go:44")},
				RequestModel:   &rm,
			})
			record := a2AggregateRecord(t, ledger)
			if record.Origin != AnswerEvidenceOriginSystemInference || record.SourceRef.Kind != ObservationSourceModelClaim {
				t.Fatalf("%s evidence certified a mismatched/unverified aggregate coordinate: %+v", name, record)
			}
		})
	}
}

func TestCompileObservationLedger_A2EveryDeclaredSourceRefMustBind(t *testing.T) {
	rm := a2RequiredCurrentSourceRequestModel()
	ledger := CompileObservationLedger(ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{{
			Kind: EvidenceDirect, Scope: ScopeLine, Source: "internal/types/a.go", LineStart: 10,
			GroundingStatus: GroundingGrounded,
		}},
		AggregateFacts: []AnswerAggregateFact{a2AggregateFact(
			"internal/types/a.go:10",
			"internal/types/invented.go:20",
		)},
		RequestModel: &rm,
	})
	record := a2AggregateRecord(t, ledger)
	if record.Origin != AnswerEvidenceOriginSystemInference {
		t.Fatalf("one real coordinate laundered another unbound SupportRef: %+v", record)
	}
}

func TestCompileObservationLedger_A2ShortPathAmbiguityFailsClosed(t *testing.T) {
	rm := a2RequiredCurrentSourceRequestModel()
	ledger := CompileObservationLedger(ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{
			{Kind: EvidenceDirect, Scope: ScopeLine, Source: "internal/a/shared.go", LineStart: 7, GroundingStatus: GroundingGrounded},
			{Kind: EvidenceDirect, Scope: ScopeLine, Source: "internal/b/shared.go", LineStart: 7, GroundingStatus: GroundingGrounded},
		},
		AggregateFacts: []AnswerAggregateFact{a2AggregateFact("shared.go:7")},
		RequestModel:   &rm,
	})
	record := a2AggregateRecord(t, ledger)
	if record.Origin != AnswerEvidenceOriginSystemInference {
		t.Fatalf("ambiguous basename guessed a proof coordinate: %+v", record)
	}
}

func TestCompileObservationLedger_A2TypedReadCoverageCanBindButSummaryCannot(t *testing.T) {
	rm := a2RequiredCurrentSourceRequestModel()
	for name, result := range map[string]ToolResult{
		"typed-read-coverage": {
			ToolName: "read_file", Success: true,
			ReadCoverage: &ToolReadCoverage{Path: "internal/types/a.go", LineStart: 1, LineEnd: 80},
		},
		"summary-only": {
			ToolName: "read_file", Success: true,
			Summary: "[file content] internal/types/a.go:1-80",
		},
		"failed-typed-read": {
			ToolName: "read_file", Success: false,
			ReadCoverage: &ToolReadCoverage{Path: "internal/types/a.go", LineStart: 1, LineEnd: 80},
		},
	} {
		t.Run(name, func(t *testing.T) {
			ledger := CompileObservationLedger(ObservationLedgerInput{
				AggregateFacts: []AnswerAggregateFact{a2AggregateFact("internal/types/a.go:44")},
				ToolResults:    []ToolResult{result},
				RequestModel:   &rm,
			})
			record := a2AggregateRecord(t, ledger)
			if name == "typed-read-coverage" {
				if record.Origin != AnswerEvidenceOriginCurrentSource || record.GroundingStatus != GroundingRecovered ||
					record.SourceRef.Path != "internal/types/a.go" || record.Span.LineStart != 44 {
					t.Fatalf("typed read coverage did not bind coordinate: %+v", record)
				}
				return
			}
			if record.Origin != AnswerEvidenceOriginSystemInference {
				t.Fatalf("%s minted proof without a successful typed carrier: %+v", name, record)
			}
		})
	}
}
