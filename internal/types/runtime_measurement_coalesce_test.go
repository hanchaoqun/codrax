package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeMeasurementAggregateKeepsPrivateCoverageConflicts(t *testing.T) {
	input, receipt := memberCoverageFixture(t)
	original := BuildRuntimeMeasurementContract(input).Choices()[0]
	if !original.CoversMemberSet(input.RequestModel) {
		t.Fatal("fixture has no private member coverage")
	}
	for _, changed := range []string{"", "scope_removed", "scope_changed", "value", "member_id", "default", "source", "missing_view", "extra_view"} {
		t.Run(changed, func(t *testing.T) {
			other := original.Clone()
			switch changed {
			case "scope_removed":
				other.coverageScope = nil
			case "scope_changed":
				other.coverageScope.windowEnd++
			case "value":
				other.Rows[0][0] = "different"
			case "member_id":
				other.MemberSet.RowIDs[0] = "different"
			case "default":
				other.DefaultPresentation = !other.DefaultPresentation
			}
			if changed == "scope_removed" || changed == "scope_changed" {
				a, _ := json.Marshal(original)
				b, _ := json.Marshal(other)
				if string(a) != string(b) {
					t.Fatal("private-scope negative does not have identical public bytes")
				}
			}
			sibling := original.Clone()
			sibling.View, sibling.MemberSet, sibling.coverageScope = RuntimeMeasurementSummary, nil, nil
			publication := RuntimeMeasurementPublication{Version: 1, ObservationID: original.ObservationID,
				Source: input.ToolResults[0].Observations[0].SourceRef, Tables: []RuntimeMeasurementTable{original, sibling}}
			altered := publication
			altered.Tables = []RuntimeMeasurementTable{other, sibling.Clone()}
			switch changed {
			case "source":
				altered.Source.Path += ".different"
			case "missing_view":
				altered.Tables = altered.Tables[:1]
			case "extra_view":
				extra := sibling.Clone()
				extra.View = RuntimeMeasurementTimeline
				altered.Tables = append(altered.Tables, extra)
			}
			c := &RuntimeMeasurementContract{Tables: coalesceRuntimeMeasurementPublications([]RuntimeMeasurementPublication{publication, altered, publication})}
			bound := BindRuntimeMeasurementReceipt(&receipt, c)
			if bound != (changed == "") {
				t.Fatalf("conflicting display/completion authority %q: bound=%t", changed, bound)
			}
			if changed == "" && (!reflect.DeepEqual(c.Tables[0], original) || !c.Tables[0].CoversMemberSet(input.RequestModel)) {
				t.Fatal("exact repeat changed original reference or private scope")
			}
			if changed != "" && c.Active() {
				t.Fatal("publication conflict left an unchanged sibling view selectable")
			}
		})
	}
	// Exercise public collection too: the publication JSON is identical, but
	// a differently qualified observation cannot inherit the earlier scope.
	other := input.ToolResults[0].Observations[0]
	other.ClaimAuthority = ObservationClaimAuthorityModelInference
	input.SystemTraceSupplementResults = measurementProviderInput(other).ToolResults
	if BuildRuntimeMeasurementContract(input).Active() {
		t.Fatal("collection coalesced public bytes with conflicting private authority")
	}
}
