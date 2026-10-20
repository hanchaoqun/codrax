package types

import (
	"encoding/json"
	"testing"
)

func TestHMC224SourceInventoryAllowCannotCertifyScope(t *testing.T) {
	for _, carrier := range []string{"log", "trace", "attached"} {
		for _, tc := range []struct {
			name string
			want bool
			edit func(*RequestModel)
		}{
			{"unquoted_all", false, nil},
			{"unquoted_production", false, func(r *RequestModel) { r.SourceScopeProfile.RequestedScope = SourceScopeProduction }},
			{"anchored_scope", false, func(r *RequestModel) { r.SourceScopeProfile.SourceQuotes = []string{"production sources"} }},
			{"exact_target", false, func(r *RequestModel) { r.AnalyzerHints.ExactTargets = []string{"src/owner.go"} }},
			{"required_file", false, func(r *RequestModel) {
				r.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "src/owner.go", Confidence: 1}}
			}},
			{"declared_inventory", true, func(r *RequestModel) {
				r.SourceInventoryProfile = &SourceInventoryProfile{IsSourceInventory: true, DeclarationOrigin: SourceInventoryDeclarationModelProvided,
					TargetRoles: []AnswerCandidateRole{AnswerCandidateRoleFunction}, SourceQuotes: []string{"public functions"}}
			}},
			{"synthesized_inventory", false, func(r *RequestModel) {
				r.SourceInventoryProfile = &SourceInventoryProfile{IsSourceInventory: true, DeclarationOrigin: SourceInventoryDeclarationSynthesized,
					TargetRoles: []AnswerCandidateRole{AnswerCandidateRoleFunction}, SourceQuotes: []string{"recorded events"}}
			}},
			{"source_excluded", false, func(r *RequestModel) {
				r.AnalyzerHints.ExactTargets = []string{"src/owner.go"}
				r.ExternalObservationPolicy.CurrentSourceMode = ExternalObservationCurrentSourceExclude
				r.ExternalObservationPolicy.ExclusionKind = ExternalObservationSourceExclusionExplicitUserBoundary
				r.ExternalObservationPolicy.SourceQuotes = []string{"do not inspect source"}
			}},
		} {
			t.Run(carrier+"/"+tc.name, func(t *testing.T) {
				rm := RequestModel{Intent: IntentEnumerate, Predicates: SemanticPredicates{HasPerMemberTable: true},
					SourceScopeProfile:        &SourceScopeProfile{RequestedScope: SourceScopeAll},
					ExternalObservationPolicy: &ExternalObservationPolicy{CurrentSourceMode: ExternalObservationCurrentSourceAllow}}
				if carrier == "log" {
					rm.LogTriage = &LogBundle{Errors: []LogError{{Type: "ObservedFailure"}}}
				} else if carrier == "trace" {
					rm.PerfTrace = &PerfBundle{Observations: []PerfObservation{{Kind: "stage", Subject: "observed-stage"}}}
				}
				if tc.edit != nil {
					tc.edit(&rm)
				}
				before, _ := json.Marshal(rm)
				hint := TurnRouteHint{Route: "repo", Source: "artifact", NeedsRepoAccess: true,
					CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceOptional, RequiredOutcomes: TurnOutcomeSourceExplanation}
				if carrier == "attached" {
					// Exercise attachment-only legacy contexts without a typed
					// external route; public tests cover routed preflight too.
					hint.Source = ""
				}
				if got := SourceInventoryCurrentSourceApplicableInContext(rm, hint, carrier == "attached"); got != tc.want {
					t.Fatalf("applicable=%v want %v", got, tc.want)
				}
				if tc.name == "anchored_scope" || tc.name == "exact_target" || tc.name == "required_file" {
					domainView := rm
					domainView.runtimeMaterialDomain = carrier == "attached"
					if got := RuntimeSourceRequestCurrentSourceRequirementPrecision(&domainView, hint); got != RuntimeSourceRequirementPrecise {
						t.Fatalf("source-inventory boundary revoked independent source obligation: %s", got)
					}
				}
				after, _ := json.Marshal(rm)
				if string(before) != string(after) || !hint.RequiresCurrentSourceEvidence() {
					t.Fatal("inventory projection mutated permissions or the route obligation")
				}
			})
		}
	}
	ordinary := RequestModel{Intent: IntentEnumerate, SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeAll},
		ExternalObservationPolicy: &ExternalObservationPolicy{CurrentSourceMode: ExternalObservationCurrentSourceAllow}}
	if !SourceInventoryCurrentSourceApplicable(ordinary) {
		t.Fatal("ordinary source inventory behavior changed")
	}
}
