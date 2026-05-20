package types

import (
	"reflect"
	"testing"
)

func TestAnswerAggregateFactEvidenceOrigins_GitHistorySearch(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:       AnswerAggregateScalar,
		Label:      "direct commits",
		Value:      "0",
		Provenance: "git_history_search",
		Dimensions: []AnswerAggregateDimension{
			{Name: "proof_source", Value: "git_history_search"},
			{Name: "measurement_kind", Value: "vcs_history_count"},
		},
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, &RequestModel{
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
			IsCountQuestion: true,
			IsScalarAnswer:  true,
		},
	})
	want := []AnswerEvidenceOrigin{
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginCommandMeasurement,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestAnswerAggregateFactEvidenceOrigins_GitDiff(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind: AnswerAggregateMemberSet,
		Dimensions: []AnswerAggregateDimension{
			{Name: "proof_source", Value: "git_diff"},
		},
		Members: []string{"oldSymbol"},
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, nil)
	want := []AnswerEvidenceOrigin{AnswerEvidenceOriginVCSDiff}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestAnswerAggregateFactEvidenceOrigins_GitShowMetadataAndDiff(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind: AnswerAggregateMemberSet,
		Dimensions: []AnswerAggregateDimension{
			{Name: "evidence_origin", Value: "vcs_metadata"},
			{Name: "diff_origin", Value: "vcs_diff"},
		},
		Members: []string{"abc1234 latest feature"},
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, nil)
	want := []AnswerEvidenceOrigin{
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestAnswerAggregateFactEvidenceOrigins_CommandMeasurement(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:       AnswerAggregateTotalCount,
		Label:      "non-test LOC",
		Value:      "70421",
		Provenance: "command:find internal/tool -name '*.go' | xargs wc -l",
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, &RequestModel{
		Predicates: SemanticPredicates{
			IsCountQuestion: true,
			IsScalarAnswer:  true,
		},
	})
	want := []AnswerEvidenceOrigin{AnswerEvidenceOriginCommandMeasurement}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestAnswerAggregateFactEvidenceOrigins_NegativeSearch(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:  AnswerAggregateNegativeSearch,
		Label: "missing config key",
		Value: "0",
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, nil)
	want := []AnswerEvidenceOrigin{AnswerEvidenceOriginRepoNegativeSearch}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestAnswerAggregateFactEvidenceOrigins_RuntimeArtifact(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:  AnswerAggregateScalar,
		Label: "panic type",
		Value: "SIGSEGV",
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, &RequestModel{
		LogTriage: &LogBundle{Errors: []LogError{{Type: "panic"}}},
	})
	want := []AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestAnswerAggregateFactEvidenceOrigins_DefaultsToCurrentSourceForOrdinaryAggregate(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:    AnswerAggregateMemberSet,
		Label:   "public functions",
		Value:   "2",
		Members: []string{"Eval", "EvalAll"},
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, nil)
	want := []AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("origins mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}
