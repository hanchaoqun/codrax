package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHMC281CommandAggregateValuePreservedWithoutLabelAuthority(t *testing.T) {
	for _, tc := range []struct{ label, value, unit string }{
		{"trace 时间轴到实际时间的固定偏移", "1787106904.242698", "s"},
		{"measured hardware bandwidth", "4096", "bytes/s"},
		{"物理资源占用比例", "0.25", "ratio"},
		{"matching revision count", "7", "count"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			fact := AnswerAggregateFact{Kind: AnswerAggregateScalar, Label: tc.label, Value: tc.value, Unit: tc.unit,
				Role: AnswerAggregateRolePrincipalAnswer, Provenance: "exec_command arithmetic",
				Dimensions:  []AnswerAggregateDimension{{Name: "origin", Value: "command_measurement"}},
				SupportRefs: []string{"capture.systrace:429168"}, MemberNotes: []string{"retained explanation, not an independent physical witness"}}
			before, _ := json.Marshal(fact)
			bindings := CompileAnswerClaimBindingsFromAggregateFacts([]AnswerAggregateFact{fact}, nil, nil)
			if !AnswerAggregateFactHasCommandValueOnlySupport(fact) {
				t.Fatal("explicit command-derived aggregate lacks shared qualification")
			}
			if len(bindings) == 0 {
				t.Fatal("command-derived value disappeared")
			}
			for _, binding := range bindings {
				if binding.GroundingPolicy != ClaimGroundingSoft || binding.AuthorityCeiling != AuthorityIllustrative {
					t.Errorf("model label inherited whole-claim authority: %+v", binding)
				}
				if !binding.ValueOnlySupport {
					t.Errorf("binding omitted the value-versus-interpretation distinction: %+v", binding)
				}
				if binding.Value != tc.value || binding.Label != tc.label || !binding.AggregateRole.IsPrincipal() ||
					!reflect.DeepEqual(binding.SupportRefs, fact.SupportRefs) {
					t.Errorf("lawful numeric/support payload changed: %+v", binding)
				}
			}
			if !AnswerAggregateFactAuthorizesPrincipalContract(fact, nil) {
				t.Error("scalar value lost its principal presentation contract")
			}
			ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: []AnswerAggregateFact{fact}})
			for _, record := range ledger.Records {
				if record.ClaimAuthority != ObservationClaimAuthorityModelInference || record.GroundingPolicy != ClaimGroundingSoft {
					t.Errorf("ledger disagrees with command label boundary: %+v", record)
				}
				if record.Value != tc.value || record.Unit != tc.unit {
					t.Errorf("raw numeric/unit text changed: %+v", record)
				}
			}
			after, _ := json.Marshal(fact)
			if string(before) != string(after) {
				t.Fatal("qualification rewrote the accepted aggregate")
			}
		})
	}
}

func TestHMC281CommandAggregateCannotBorrowSiblingOriginOrSourceCoordinate(t *testing.T) {
	fact := AnswerAggregateFact{Kind: AnswerAggregateScalar, Label: "model-named relation", Value: "7", Role: AnswerAggregateRolePrincipalAnswer,
		Dimensions:  []AnswerAggregateDimension{{Name: "origin", Value: "vcs_metadata"}, {Name: "measurement_origin", Value: "command_measurement"}},
		SupportRefs: []string{"current.go:42"}}
	bindings := CompileAnswerClaimBindingsFromAggregateFacts([]AnswerAggregateFact{fact}, nil, nil)
	if len(bindings) < 2 {
		t.Fatalf("mixed origin payload lost: %+v", bindings)
	}
	for _, binding := range bindings {
		if binding.GroundingPolicy != ClaimGroundingSoft || AnswerClaimBindingAuthorityCeiling(binding) != AuthorityIllustrative ||
			!binding.ValueOnlySupport {
			t.Errorf("sibling origin laundered the same model label: %+v", binding)
		}
	}
	if aggregateFactHasIndependentTypedAuthority(fact, nil) {
		t.Fatal("operand coordinate independently certified an arbitrary calculation label")
	}
	if !AnswerAggregateFactAuthorizesPrincipalContract(fact, nil) {
		t.Fatal("mixed-origin count value lost its presentation contract")
	}
}

func TestHMC281RequestInferredCommandLaneDoesNotDemoteIndependentSourceFact(t *testing.T) {
	rm := &RequestModel{Predicates: SemanticPredicates{IsCountQuestion: true, IsScalarAnswer: true}}
	fact := AnswerAggregateFact{Kind: AnswerAggregateScalar, Label: "source constant", Value: "7", Role: AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "current_source"}}, SupportRefs: []string{"current.go:42"}}
	if AnswerAggregateFactHasCommandValueOnlySupport(fact) || !aggregateFactHasIndependentTypedAuthority(fact, rm) {
		t.Fatal("count-shaped request changed an independently source-backed fact into a command derivation")
	}
	bindings := CompileAnswerClaimBindingsFromAggregateFacts([]AnswerAggregateFact{fact}, rm, nil)
	if len(bindings) != 2 {
		t.Fatalf("count compatibility bindings changed: %+v", bindings)
	}
	for _, binding := range bindings {
		switch binding.Origin {
		case AnswerEvidenceOriginCurrentSource:
			if binding.GroundingPolicy != ClaimGroundingHard || binding.AuthorityCeiling != AuthorityFactual || binding.ValueOnlySupport {
				t.Errorf("source witness was demoted by inferred sibling: %+v", binding)
			}
		case AnswerEvidenceOriginCommandMeasurement:
			if binding.GroundingPolicy != ClaimGroundingSoft || !binding.ValueOnlySupport {
				t.Errorf("request shape minted command proof: %+v", binding)
			}
		}
	}
}

func TestHMC281CommandValueBoundaryPreservesNativeAndTypedRelationAuthority(t *testing.T) {
	native := []ToolResult{{ToolName: "exec_command", Success: true, RawRef: "count-output.txt",
		CommandMeasurement: &ToolCommandMeasurement{Kind: ToolCommandMeasurementKindCount, Value: 7, Command: "printf 7"}},
		{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{{
			ID: "native:wakeup", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: AnswerAggregateRolePrincipalAnswer, GroundingPolicy: ClaimGroundingHard,
			ClaimAuthority: ObservationClaimAuthorityDirectObservation, ProvenanceLane: ObservationProvenanceObservedDirectCause,
			SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "capture.systrace"},
			Predicate: "wakeup_chain", Subject: "waker-12", Object: "target-41", Value: "0.0005", Unit: "s",
		}}}}
	before := CompileObservationLedger(ObservationLedgerInput{ToolResults: native})
	fact := AnswerAggregateFact{Kind: AnswerAggregateScalar, Label: "model explanation", Value: "7", Role: AnswerAggregateRolePrincipalAnswer,
		Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "command_measurement"}}}
	after := CompileObservationLedger(ObservationLedgerInput{ToolResults: native, AggregateFacts: []AnswerAggregateFact{fact}})
	for _, want := range before.Records {
		found := false
		for _, got := range after.Records {
			if got.ID == want.ID {
				found = true
				if !reflect.DeepEqual(got, want) {
					t.Errorf("aggregate qualification changed native witness: before=%+v after=%+v", want, got)
				}
			}
		}
		if !found {
			t.Errorf("native witness lost: %s", want.ID)
		}
	}
	for _, provenance := range []string{TypedRelationPrincipalMemberSetAggregateProvenance, SourceInventoryPrincipalRowSetAggregateProvenance} {
		system := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Label: "verified members", Value: "1", Members: []string{"Worker"},
			Role: AnswerAggregateRolePrincipalAnswer, Provenance: provenance,
			Dimensions: []AnswerAggregateDimension{{Name: "origin", Value: "command_measurement"}}}
		binding := CompileAnswerClaimBindingsFromAggregateFacts([]AnswerAggregateFact{system}, nil, nil)[0]
		if binding.GroundingPolicy != ClaimGroundingHard || binding.AuthorityCeiling != AuthorityFactual ||
			!aggregateFactHasIndependentTypedAuthority(system, nil) || AnswerAggregateFactHasCommandValueOnlySupport(system) || binding.ValueOnlySupport {
			t.Errorf("independently verified system carrier demoted: %+v", binding)
		}
	}
}
