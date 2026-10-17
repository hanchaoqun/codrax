package types

import (
	"reflect"
	"testing"
)

func TestHMC257IndependentCurrentSourcePromptBudgetCompetition(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*RuntimeSourceAnswerAuthoritySnapshot, *RequestModel)
		want bool
	}{
		{name: "runtime-only"},
		{name: "legacy-soft-default", set: func(s *RuntimeSourceAnswerAuthoritySnapshot, _ *RequestModel) {
			s.CurrentSourceRequired, s.NeedsCurrentSourceEvidence, s.CanDowngradeToCaveat = true, true, true
			s.CurrentSourceRequirement = RuntimeSourceRequirementSoft
		}},
		{name: "ordinary-diagnostic", set: func(_ *RuntimeSourceAnswerAuthoritySnapshot, rm *RequestModel) {
			rm.DiagnosticProfile.IsDiagnostic = true
			rm.Predicates.IsDiagnosticQuestion, rm.Predicates.IsCrossComponent = true, true
		}},
		{name: "requested-source", want: true, set: func(s *RuntimeSourceAnswerAuthoritySnapshot, _ *RequestModel) {
			s.CurrentSourceExplanationRequested = true
		}},
		{name: "landed-source", want: true, set: func(s *RuntimeSourceAnswerAuthoritySnapshot, _ *RequestModel) {
			s.CurrentSourceRecordCount = 1
		}},
		{name: "precise-obligation", want: true, set: func(s *RuntimeSourceAnswerAuthoritySnapshot, _ *RequestModel) {
			s.CurrentSourceRequirement = RuntimeSourceRequirementPrecise
		}},
		{name: "satisfied-source", want: true, set: func(s *RuntimeSourceAnswerAuthoritySnapshot, _ *RequestModel) {
			s.CurrentSourceSatisfied = true
		}},
		{name: "current-version", want: true, set: func(_ *RuntimeSourceAnswerAuthoritySnapshot, rm *RequestModel) {
			rm.DiagnosticProfile.CurrentVersionCheck = true
		}},
		{name: "current-version-excluded", set: func(_ *RuntimeSourceAnswerAuthoritySnapshot, rm *RequestModel) {
			rm.DiagnosticProfile.CurrentVersionCheck = true
			rm.ExternalObservationPolicy = &ExternalObservationPolicy{
				CurrentSourceMode: ExternalObservationCurrentSourceExclude,
				ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"do not analyze current source"},
			}
		}},
		{name: "inactive", set: func(s *RuntimeSourceAnswerAuthoritySnapshot, _ *RequestModel) {
			s.Active, s.CurrentSourceExplanationRequested = false, true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := RuntimeSourceAnswerAuthoritySnapshot{Active: true, RuntimeObservationCount: 5, RuntimeOnlySufficient: true, CanUseRuntimeOnlyWithCaveat: true}
			rm := RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet}}
			if tc.set != nil {
				tc.set(&s, &rm)
			}
			before := s
			if got := s.HasIndependentCurrentSourcePromptBudgetCompetition(&rm); got != tc.want {
				t.Fatalf("budget competition=%t want %t; %+v", got, tc.want, s)
			}
			if !reflect.DeepEqual(s, before) {
				t.Fatal("budget projection mutated source authority")
			}
		})
	}
}
