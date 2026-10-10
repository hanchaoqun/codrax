package types

import "testing"

func TestRuntimeNativeFactDefaultSelectionBoundaries(t *testing.T) {
	table := RuntimeMeasurementTable{ObservationID: "native", View: RuntimeMeasurementMembers, Columns: []string{"value"}, Rows: [][]string{{"unknown"}}, DefaultPresentation: true}
	contract := &RuntimeMeasurementContract{Tables: []RuntimeMeasurementTable{table}}
	rm := RequestModel{RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
			{Required: true, Role: RequestedAnswerDimensionObservedValue}, {Required: true, Role: RequestedAnswerDimensionRelationPath}, {Required: true, Role: RequestedAnswerDimensionCurrentKeyCode}}}}
	for _, scope := range AllRuntimeQuestionScopes() {
		rm.RuntimeQuestionProfile.Scope = scope
		want := scope != RuntimeQuestionScopeNotApplicable && scope != RuntimeQuestionScopeUnspecified
		if (len(RuntimeNativeFactDisplaySelections(&rm, contract)) == 1) != want {
			t.Fatalf("scope %s changed independent relationship/source/fact dimensions", scope)
		}
	}
	rm.RuntimeQuestionProfile.Scope = RuntimeQuestionScopeCausalDiagnosis
	// Defaults are producer intent, not a guessed choice among multiple views.
	duplicate := table
	duplicate.View = RuntimeMeasurementTimeline
	contract.Tables = append(contract.Tables, duplicate)
	if len(RuntimeNativeFactDisplaySelections(&rm, contract)) != 0 {
		t.Fatal("ambiguous defaults were silently arbitrated")
	}
	contract.Tables = contract.Tables[:1]
	for _, role := range []RequestedAnswerDimensionRole{RequestedAnswerDimensionRelationPath, RequestedAnswerDimensionCausalAttribution, RequestedAnswerDimensionCurrentKeyCode, RequestedAnswerDimensionDiagram} {
		rm.RequestedAnswerDimensions.Dimensions = []RequestedAnswerDimension{{Required: true, Role: role}}
		if len(RuntimeNativeFactDisplaySelections(&rm, contract)) != 0 {
			t.Fatalf("role %s gained unrelated fact table", role)
		}
	}
	rm.RequestedAnswerDimensions.Dimensions = []RequestedAnswerDimension{{Required: false, Role: RequestedAnswerDimensionObservedValue}}
	if len(RuntimeNativeFactDisplaySelections(&rm, contract)) != 0 {
		t.Fatal("optional guidance became an automatic answer")
	}
	rm.RequestedAnswerDimensions.Dimensions[0].Required = true
	contract.Tables[0].DefaultPresentation = false
	if len(RuntimeNativeFactDisplaySelections(&rm, contract)) != 0 {
		t.Fatal("unmarked provider table was guessed as default")
	}
	contract.Tables[0].DefaultPresentation = true
	for _, scope := range []RuntimeQuestionScope{"", "invalid"} {
		rm.RuntimeQuestionProfile.Scope = scope
		if len(RuntimeNativeFactDisplaySelections(&rm, contract)) != 0 {
			t.Fatal("undeclared scope gained a default display")
		}
	}
	rm.RuntimeQuestionProfile.Scope = RuntimeQuestionScopeCausalDiagnosis
	rm.SourceInventoryProfile = &SourceInventoryProfile{IsSourceInventory: true, TargetRoles: []AnswerCandidateRole{AnswerCandidateRoleFunction}}
	if len(RuntimeNativeFactDisplaySelections(&rm, contract)) != 0 {
		t.Fatal("runtime records took over the source inventory")
	}
}
