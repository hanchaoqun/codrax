package tool

import (
	"reflect"
	"testing"
)

func TestRuntimeDiagramPresentationSamplingIsNotAuthority(t *testing.T) {
	rows := []RuntimeDiagramRelation{
		{FromIdentity: "narrow-a", presentationKey: "physical-a", presentationQuery: "narrow"},
		{FromIdentity: "wide-a", presentationKey: "physical-a", presentationQuery: "wide"},
		{FromIdentity: "wide-b", presentationKey: "physical-b", presentationQuery: "wide"},
		{FromIdentity: "other-file-a", presentationKey: "other-file-a", presentationQuery: "other"},
		{FromIdentity: "unknown"}, {FromIdentity: "unknown"},
	}
	before := append([]RuntimeDiagramRelation(nil), rows...)
	selected := runtimeDiagramPresentationRows(rows)
	if len(selected) != 5 || selected[0].FromIdentity != "wide-a" || selected[1].FromIdentity != "wide-b" || !reflect.DeepEqual(rows, before) {
		t.Fatalf("sampling changed authority or lost distinct/unknown rows: %+v", selected)
	}
	// The covered event's discarded query still proves exactly its own pair.
	rows[0].ToIdentity = "narrow-target"
	rows[1].ToIdentity = "wide-target"
	if !runtimeDiagramRelationProved(rows, "narrow-a", "narrow-target", rows[0].Kind) || runtimeDiagramRelationProved(rows, "narrow-a", "wide-target", rows[0].Kind) {
		t.Fatal("presentation selection changed query-local authority")
	}
}
