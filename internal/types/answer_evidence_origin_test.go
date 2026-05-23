package types

import (
	"reflect"
	"strings"
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

func TestAnswerAggregateFactEvidenceOrigins_ExecCommandGitHistory(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:  AnswerAggregateScalar,
		Label: "history count",
		Value: "3",
		Dimensions: []AnswerAggregateDimension{
			{Name: "proof_source", Value: "exec_command_git_history"},
			{Name: "measurement_origin", Value: "command_measurement"},
		},
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, nil)
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

func TestAnswerAggregateFactEvidenceOrigins_SourceRefToolTokens(t *testing.T) {
	cases := []struct {
		name string
		dim  AnswerAggregateDimension
		want AnswerEvidenceOrigin
	}{
		{
			name: "source_ref exact git history tool",
			dim:  AnswerAggregateDimension{Name: "source_ref", Value: "git_history_search"},
			want: AnswerEvidenceOriginVCSMetadata,
		},
		{
			name: "tool_result bracketed git log call",
			dim:  AnswerAggregateDimension{Name: "tool_result", Value: "git_log[0]"},
			want: AnswerEvidenceOriginVCSMetadata,
		},
		{
			name: "producer runtime triage",
			dim:  AnswerAggregateDimension{Name: "producer", Value: "emit_log_triage"},
			want: AnswerEvidenceOriginRuntimeArtifact,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AnswerAggregateFactEvidenceOrigins(AnswerAggregateFact{
				Kind:       AnswerAggregateNegativeObservation,
				Label:      "no-hit",
				Value:      "0",
				Dimensions: []AnswerAggregateDimension{tc.dim},
			}, nil)
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("origins = %+v, want [%s]", got, tc.want)
			}
		})
	}
}

func TestAnswerEvidenceOriginsAreOriginSpecificOnly(t *testing.T) {
	if !AnswerEvidenceOriginsAreOriginSpecificOnly([]AnswerEvidenceOrigin{
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginCommandMeasurement,
	}) {
		t.Fatal("mixed external observation origins should be origin-specific only")
	}
	if AnswerEvidenceOriginsAreOriginSpecificOnly([]AnswerEvidenceOrigin{
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginCurrentSource,
	}) {
		t.Fatal("current_source mixed with external support must not be origin-specific only")
	}
	if AnswerEvidenceOriginsAreOriginSpecificOnly([]AnswerEvidenceOrigin{
		AnswerEvidenceOriginUnknown,
	}) {
		t.Fatal("unknown-only origin set must not suppress current-source fallbacks")
	}
	if AnswerEvidenceOriginsAreOriginSpecificOnly([]AnswerEvidenceOrigin{
		AnswerEvidenceOriginSystemInference,
	}) {
		t.Fatal("system inference is display support, not origin-specific evidence support")
	}
}

func TestNormalizeAnswerAggregateFacts_AddsStructuredOriginDimensionsFromLegacyProvenance(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:       AnswerAggregateMemberSet,
		Label:      "changed symbols",
		Value:      "1",
		Provenance: "git_diff",
		Members:    []string{"OldSymbol"},
	}})
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("normalized facts = %d, want 1", len(got))
	}
	if value := aggregateTestDimensionValue(got[0].Dimensions, "origin"); value != string(AnswerEvidenceOriginVCSDiff) {
		t.Fatalf("origin dimension = %q, want %q; dims=%+v", value, AnswerEvidenceOriginVCSDiff, got[0].Dimensions)
	}
	origins := AnswerAggregateFactEvidenceOrigins(got[0], nil)
	want := []AnswerEvidenceOrigin{AnswerEvidenceOriginVCSDiff}
	if !reflect.DeepEqual(origins, want) {
		t.Fatalf("origins mismatch after normalization\ngot:  %#v\nwant: %#v", origins, want)
	}
}

func TestNormalizeAnswerAggregateFacts_PreservesExistingOriginDimensions(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:       AnswerAggregateScalar,
		Label:      "history count",
		Value:      "3",
		Provenance: "git_history_search",
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: string(AnswerEvidenceOriginVCSMetadata)},
			{Name: "measurement_origin", Value: string(AnswerEvidenceOriginCommandMeasurement)},
		},
	}})
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("normalized facts = %d, want 1", len(got))
	}
	var originCount int
	for _, dim := range got[0].Dimensions {
		if strings.EqualFold(dim.Name, "origin") {
			originCount++
		}
	}
	if originCount != 1 {
		t.Fatalf("origin dimension duplicated: %+v", got[0].Dimensions)
	}
	if value := aggregateTestDimensionValue(got[0].Dimensions, "measurement_origin"); value != string(AnswerEvidenceOriginCommandMeasurement) {
		t.Fatalf("measurement_origin = %q, want %q", value, AnswerEvidenceOriginCommandMeasurement)
	}
}

func aggregateTestDimensionValue(dims []AnswerAggregateDimension, name string) string {
	for _, dim := range dims {
		if strings.EqualFold(dim.Name, name) {
			return dim.Value
		}
	}
	return ""
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

func TestAnswerAggregateFactEvidenceOrigins_NegativeObservation(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:  AnswerAggregateNegativeObservation,
		Label: "trace window has no long GC span",
		Value: "0",
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: "runtime_artifact"},
			{Name: "target", Value: "GC span > 50ms"},
			{Name: "scope", Value: "attached trace"},
		},
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, nil)
	want := []AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact}
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

func TestAnswerAggregateFactEvidenceOrigins_RuntimeBehaviorOutcome(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:  AnswerAggregateBehaviorOutcome,
		Label: "崩溃根因",
		Value: "UserCard 接收到了 undefined 并触发 ArkTS panic",
	}
	got := AnswerAggregateFactEvidenceOrigins(fact, &RequestModel{
		LogTriage: &LogBundle{Errors: []LogError{{Type: "TypeError"}}},
	})
	want := []AnswerEvidenceOrigin{AnswerEvidenceOriginRuntimeArtifact}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime behavior outcome origins mismatch\ngot:  %#v\nwant: %#v", got, want)
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
