package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func sourceInventoryOptionalRuntimeRequest() RequestModel {
	return RequestModel{
		Intent:     IntentEnumerate,
		Predicates: SemanticPredicates{HasPerMemberTable: true},
		PerfTrace:  &PerfBundle{Observations: []PerfObservation{{Kind: "stage", Subject: "renamed-stage"}}},
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction, AnswerCandidateRoleType},
			SourceQuotes:      []string{"renamed-stage"},
			Confidence:        1,
		},
	}
}

func TestSourceInventoryRuntimeDeclarationDoesNotReplacePrecision(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		origin                 SourceInventoryDeclarationOrigin
		mutate                 func(*RequestModel)
		wantHard, wantNavigate bool
	}{
		{"provided_low_precision", SourceInventoryDeclarationModelProvided, nil, false, true},
		{"provided_missing_quote", SourceInventoryDeclarationModelProvided, func(r *RequestModel) { r.SourceInventoryProfile.SourceQuotes = nil }, false, false},
		{"provided_scope", SourceInventoryDeclarationModelProvided, func(r *RequestModel) { r.SourceScopeProfile = &SourceScopeProfile{RequestedScope: SourceScopeTest} }, true, true},
		{"provided_const", SourceInventoryDeclarationModelProvided, func(r *RequestModel) { r.SourceInventoryProfile.RequiresConstSet = true }, true, true},
		{"provided_complete", SourceInventoryDeclarationModelProvided, func(r *RequestModel) {
			r.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "renamed-stage"}
		}, true, true},
		{"synthesized_copied_quote", SourceInventoryDeclarationSynthesized, func(r *RequestModel) { r.SourceInventoryProfile.RequiresConstSet = true }, false, false},
		{"legacy_restored_quote", "", func(r *RequestModel) { r.SourceInventoryProfile.RequiresConstSet = true }, false, false},
		{"old_precise_source_hint", "", func(r *RequestModel) {
			r.AnalyzerHints.RequiredFileHints = []RequiredFileHint{{Path: "src/owner.go", Confidence: 1}}
		}, true, true},
		{"old_precise_target", "", func(r *RequestModel) { r.AnalyzerHints.ExactTargets = []string{"src/owner.go"} }, true, true},
		{"source_excluded", SourceInventoryDeclarationModelProvided, func(r *RequestModel) {
			r.SourceInventoryProfile.RequiresConstSet = true
			r.ExternalObservationPolicy = &ExternalObservationPolicy{CurrentSourceMode: ExternalObservationCurrentSourceExclude, ExclusionKind: ExternalObservationSourceExclusionExplicitUserBoundary, SourceQuotes: []string{"renamed-stage"}}
		}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rm := sourceInventoryOptionalRuntimeRequest()
			rm.SourceInventoryProfile.DeclarationOrigin = tc.origin
			rm.SourceInventoryProfile.Confidence = 0.5
			if tc.mutate != nil {
				tc.mutate(&rm)
			}
			if tc.wantHard && !SourceInventoryProfileHasPrincipalPrecision(rm) {
				// Existing precise code anchors identify the source lane, while
				// the original principal precision rule still owns the inventory.
				rm.SourceInventoryProfile.Confidence = 1
			}
			if got := SourceInventoryCurrentSourceApplicable(rm); got != tc.wantHard {
				t.Errorf("applicable=%v want %v", got, tc.wantHard)
			}
			if got := SourceInventoryPrincipalAuthorityActive(rm); got != tc.wantHard {
				t.Errorf("authority=%v want %v", got, tc.wantHard)
			}
			if got := SourceInventoryPrincipalNavigationActive(rm); got != tc.wantNavigate {
				t.Errorf("navigation=%v want %v", got, tc.wantNavigate)
			}
			if got := SourceInventoryRequiredFileCoverageShape(rm); got != tc.wantHard {
				t.Errorf("coverage=%v want %v", got, tc.wantHard)
			}
		})
	}
}

func TestSourceInventoryDeclarationCloneAndPersistence(t *testing.T) {
	rm := sourceInventoryOptionalRuntimeRequest()
	rm.SourceInventoryProfile.DeclarationOrigin = SourceInventoryDeclarationModelProvided
	rm.SourceInventoryProfile.RequestedFields = []SourceInventoryRequestedField{SourceInventoryFieldName}
	before, _ := json.Marshal(rm)
	mut := NewMutableState("renamed-stage")
	mut.SetRequestModel(rm)
	rm.SourceInventoryProfile.SourceQuotes[0] = "caller mutation"
	first := mut.RequestModel()
	if first.SourceInventoryProfile.SourceQuotes[0] != "renamed-stage" {
		t.Fatal("setter aliased source quotes")
	}
	fork := mut.ForkForExploreDispatch()
	first.SourceInventoryProfile.TargetRoles[0] = AnswerCandidateRoleUnknown
	first.SourceInventoryProfile.RequestedFields[0] = SourceInventoryFieldSummary
	if got, _ := json.Marshal(mut.RequestModel()); string(got) != string(before) {
		t.Fatal("getter aliased inventory declaration")
	}
	forked := fork.RequestModel()
	if !reflect.DeepEqual(forked, mut.RequestModel()) {
		t.Fatal("fork lost declaration")
	}
	var restored RequestModel
	if err := json.Unmarshal(before, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.SourceInventoryProfile.DeclarationOrigin != SourceInventoryDeclarationModelProvided {
		t.Fatal("persisted declaration origin lost")
	}
	legacyRM := sourceInventoryOptionalRuntimeRequest()
	legacy, _ := json.Marshal(legacyRM)
	// Decode into a fresh IR, as the durable restore path does.
	var old RequestModel
	if err := json.Unmarshal(legacy, &old); err != nil {
		t.Fatal(err)
	}
	// Runtime bundles are held separately from RequestModel JSON and restored
	// through the accepted current-run context, not manufactured by decoding.
	old.PerfTrace = legacyRM.PerfTrace
	if old.SourceInventoryProfile.DeclarationOrigin != "" || SourceInventoryPrincipalAuthorityActive(old) {
		t.Fatal("legacy profile inferred a model declaration")
	}
}

func TestSourceInventoryOrdinarySourceKeepsLegacyShape(t *testing.T) {
	rm := sourceInventoryOptionalRuntimeRequest()
	rm.PerfTrace = nil
	before, _ := json.Marshal(rm)
	if !SourceInventoryCurrentSourceApplicable(rm) || !SourceInventoryPrincipalAuthorityActive(rm) || !SourceInventoryRequiredFileCoverageShape(rm) {
		t.Fatal("ordinary source inventory lost old authority")
	}
	after, _ := json.Marshal(rm)
	if string(before) != string(after) {
		t.Fatal("applicability changed the request")
	}
}

func TestSourceInventoryRuntimeOptionalUnknownProfileDoesNotGrantSource(t *testing.T) {
	rm := sourceInventoryOptionalRuntimeRequest()
	if got := RuntimeSourceRequestCurrentSourceRequirementPrecision(&rm, TurnRouteHint{}); got != RuntimeSourceRequirementNone {
		t.Fatalf("fixture should have optional current source: %q", got)
	}
	if SourceInventoryRequiredFileCoverageShape(rm) {
		t.Error("runtime table/profile minted required-file coverage without an independent source request")
	}
	if SourceInventoryPrincipalNavigationActive(rm) || SourceInventoryPrincipalAuthorityActive(rm) {
		t.Error("unknown profile provenance minted a source-inventory lane")
	}
}
